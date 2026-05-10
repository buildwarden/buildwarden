package main

import (
	"io"
	"sync"
	"time"
)

// quantum is the number of bytes each stream may read per scheduling round.
const quantum = 128 * 1024 // 128 KiB

// drrScheduler implements Deficit Round Robin scheduling across all active
// response streams. Each stream gets a quota of bytes it may read before
// yielding to others. Unlike a strict turn-based ring, streams read
// independently and the scheduler only governs how much each stream may
// read before waiting — this avoids blocking the entire ring when one
// stream's upstream is slow.
type drrScheduler struct {
	mu      sync.Mutex
	streams []*drrStream
}

type drrStream struct {
	mu      sync.Mutex
	cond    *sync.Cond
	quota   int  // bytes remaining before this stream must wait
	waiting bool // true if this stream is blocked waiting for quota
}

var scheduler = newDRRScheduler()

func newDRRScheduler() *drrScheduler {
	s := &drrScheduler{}
	go s.run()
	return s
}

// join adds a stream to the scheduler with an initial quota.
func (s *drrScheduler) join() *drrStream {
	st := &drrStream{quota: quantum}
	st.cond = sync.NewCond(&st.mu)
	s.mu.Lock()
	s.streams = append(s.streams, st)
	s.mu.Unlock()
	return st
}

// leave removes a stream from the scheduler.
func (s *drrScheduler) leave(st *drrStream) {
	s.mu.Lock()
	for i, v := range s.streams {
		if v == st {
			s.streams = append(s.streams[:i], s.streams[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	// Wake the stream in case it's blocked waiting for quota.
	st.cond.Signal()
}

// run periodically replenishes quota for all streams. This is the
// "round" in round-robin: every tick, each stream gets a fresh quantum.
// Streams that haven't used their previous quota don't accumulate
// unbounded credit (capped at 2x quantum for burst tolerance).
func (s *drrScheduler) run() {
	for {
		s.mu.Lock()
		for _, st := range s.streams {
			st.mu.Lock()
			st.quota += quantum
			if st.quota > quantum*2 {
				st.quota = quantum * 2
			}
			if st.waiting {
				st.cond.Signal()
			}
			st.mu.Unlock()
		}
		n := len(s.streams)
		s.mu.Unlock()

		// If no streams or only one stream, replenish fast (no contention).
		// With multiple streams, pace replenishment so total throughput is
		// not artificially limited but fairness is enforced.
		if n <= 1 {
			// Single stream: replenish immediately when it exhausts quota.
			sleepUntilNeeded(s)
		} else {
			// Multiple streams: replenish at intervals that create natural
			// pacing. With 128KB quantum and say 100MB/s target per-stream,
			// that's ~1.3ms per quantum. We use 1ms as a reasonable tick.
			time.Sleep(time.Millisecond)
		}
	}
}

// sleepUntilNeeded blocks until at least one stream is waiting for quota.
func sleepUntilNeeded(s *drrScheduler) {
	for {
		time.Sleep(time.Millisecond)
		s.mu.Lock()
		for _, st := range s.streams {
			st.mu.Lock()
			if st.waiting {
				st.mu.Unlock()
				s.mu.Unlock()
				return
			}
			st.mu.Unlock()
		}
		s.mu.Unlock()
	}
}

// acquireQuota blocks until the stream has quota available, then deducts
// the requested amount. Returns the number of bytes granted.
func (st *drrStream) acquireQuota(requested int) int {
	st.mu.Lock()
	for st.quota <= 0 {
		st.waiting = true
		st.cond.Wait()
		st.waiting = false
	}
	grant := requested
	if grant > st.quota {
		grant = st.quota
	}
	st.quota -= grant
	st.mu.Unlock()
	return grant
}

// fairReader wraps an upstream response body and participates in the DRR
// scheduler to ensure fair bandwidth sharing across all concurrent streams.
// Crucially, streams read independently — a slow upstream does NOT block
// other streams from making progress.
type fairReader struct {
	source io.ReadCloser
	stream *drrStream
	sched  *drrScheduler
}

func newFairReader(source io.ReadCloser, _ int64) io.ReadCloser {
	return &fairReader{
		source: source,
		sched:  scheduler,
		stream: scheduler.join(),
	}
}

func (fr *fairReader) Read(p []byte) (int, error) {
	// Acquire quota (may block if this stream has exhausted its share).
	allowed := fr.stream.acquireQuota(len(p))

	// Read from upstream — this may block on I/O but does NOT hold any
	// shared lock or ring position. Other streams proceed independently.
	n, err := fr.source.Read(p[:allowed])
	return n, err
}

func (fr *fairReader) Close() error {
	fr.sched.leave(fr.stream)
	return fr.source.Close()
}

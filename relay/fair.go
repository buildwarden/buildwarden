package relay

import (
	"io"
	"sync"
	"time"
)

const quantum = 128 * 1024

type drrScheduler struct {
	mu      sync.Mutex
	streams []*drrStream
}

type drrStream struct {
	mu      sync.Mutex
	cond    *sync.Cond
	quota   int
	waiting bool
}

var scheduler = newDRRScheduler()

func newDRRScheduler() *drrScheduler {
	s := &drrScheduler{}
	go s.run()
	return s
}

func (s *drrScheduler) join() *drrStream {
	st := &drrStream{quota: quantum}
	st.cond = sync.NewCond(&st.mu)
	s.mu.Lock()
	s.streams = append(s.streams, st)
	s.mu.Unlock()
	return st
}

func (s *drrScheduler) leave(st *drrStream) {
	s.mu.Lock()
	for i, v := range s.streams {
		if v == st {
			s.streams = append(s.streams[:i], s.streams[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	st.cond.Signal()
}

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

		if n <= 1 {
			sleepUntilNeeded(s)
		} else {
			time.Sleep(time.Millisecond)
		}
	}
}

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
	allowed := fr.stream.acquireQuota(len(p))
	n, err := fr.source.Read(p[:allowed])
	return n, err
}

func (fr *fairReader) Close() error {
	fr.sched.leave(fr.stream)
	return fr.source.Close()
}

package relay

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testScheduler creates an isolated scheduler for testing (avoids global state).
func testScheduler() *drrScheduler {
	s := &drrScheduler{}
	go s.run()
	return s
}

func newTestFairReader(s *drrScheduler, source io.ReadCloser) io.ReadCloser {
	st := s.join()
	return &fairReader{
		source: source,
		sched:  s,
		stream: st,
	}
}

func TestDRR_SingleStreamFullSpeed(t *testing.T) {
	s := testScheduler()
	data := make([]byte, 512*1024) // 512KB
	for i := range data {
		data[i] = byte(i)
	}

	fr := newTestFairReader(s, io.NopCloser(bytes.NewReader(data)))

	buf := make([]byte, 32*1024)
	var total int
	start := time.Now()
	for {
		n, err := fr.Read(buf)
		total += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	elapsed := time.Since(start)

	if total != len(data) {
		t.Fatalf("expected %d bytes, got %d", len(data), total)
	}
	// Single stream should complete quickly (quota replenishes immediately
	// when no contention).
	if elapsed > 500*time.Millisecond {
		t.Fatalf("single stream took %v, expected fast completion", elapsed)
	}
	fr.Close()
}

func TestDRR_TwoStreamsShareFairly(t *testing.T) {
	s := testScheduler()
	size := 256 * 1024 // 256KB each

	dataA := make([]byte, size)
	dataB := make([]byte, size)

	frA := newTestFairReader(s, io.NopCloser(bytes.NewReader(dataA)))
	frB := newTestFairReader(s, io.NopCloser(bytes.NewReader(dataB)))

	var totalA, totalB atomic.Int64
	var wg sync.WaitGroup
	wg.Add(2)

	readAll := func(fr io.ReadCloser, total *atomic.Int64) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := fr.Read(buf)
			total.Add(int64(n))
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
		}
		fr.Close()
	}

	go readAll(frA, &totalA)
	go readAll(frB, &totalB)
	wg.Wait()

	if totalA.Load() != int64(size) {
		t.Fatalf("A: expected %d, got %d", size, totalA.Load())
	}
	if totalB.Load() != int64(size) {
		t.Fatalf("B: expected %d, got %d", size, totalB.Load())
	}
}

func TestDRR_SlowStreamDoesNotBlockOthers(t *testing.T) {
	s := testScheduler()

	// Stream A: fast (data immediately available)
	fastData := make([]byte, 256*1024)
	frFast := newTestFairReader(s, io.NopCloser(bytes.NewReader(fastData)))

	// Stream B: slow (blocks for 200ms before returning data)
	slowSource := &blockingReader{
		data:       make([]byte, 64*1024),
		blockUntil: time.Now().Add(200 * time.Millisecond),
	}
	frSlow := newTestFairReader(s, io.NopCloser(slowSource))

	var fastDone atomic.Bool
	var wg sync.WaitGroup
	wg.Add(2)

	// Read fast stream
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, _ := frFast.Read(buf)
			if n == 0 {
				break
			}
		}
		fastDone.Store(true)
		frFast.Close()
	}()

	// Read slow stream
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := frSlow.Read(buf)
			_ = n
			if err == io.EOF {
				break
			}
		}
		frSlow.Close()
	}()

	// The fast stream should complete well before the slow stream's 200ms block.
	time.Sleep(100 * time.Millisecond)
	if !fastDone.Load() {
		t.Fatal("fast stream should have completed while slow " +
			"stream was blocked, but it didn't")
	}

	wg.Wait()
}

func TestDRR_ManyStreams(t *testing.T) {
	s := testScheduler()
	numStreams := 8
	size := 128 * 1024

	var wg sync.WaitGroup
	wg.Add(numStreams)

	for i := 0; i < numStreams; i++ {
		go func() {
			defer wg.Done()
			data := make([]byte, size)
			fr := newTestFairReader(s, io.NopCloser(bytes.NewReader(data)))
			buf := make([]byte, 32*1024)
			var total int
			for {
				n, err := fr.Read(buf)
				total += n
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
			}
			if total != size {
				t.Errorf("expected %d, got %d", size, total)
			}
			fr.Close()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: 8 concurrent streams did not complete within 5s")
	}
}

func TestDRR_StreamJoinsMidTransfer(t *testing.T) {
	s := testScheduler()

	dataA := make([]byte, 256*1024)
	dataB := make([]byte, 128*1024)

	frA := newTestFairReader(s, io.NopCloser(bytes.NewReader(dataA)))

	var totalA, totalB atomic.Int64
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := frA.Read(buf)
			totalA.Add(int64(n))
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("A: %v", err)
				return
			}
		}
		frA.Close()
	}()

	// Let A get a head start, then join B.
	time.Sleep(2 * time.Millisecond)

	frB := newTestFairReader(s, io.NopCloser(bytes.NewReader(dataB)))
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := frB.Read(buf)
			totalB.Add(int64(n))
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("B: %v", err)
				return
			}
		}
		frB.Close()
	}()

	wg.Wait()

	if totalA.Load() != int64(len(dataA)) {
		t.Fatalf("A: expected %d, got %d", len(dataA), totalA.Load())
	}
	if totalB.Load() != int64(len(dataB)) {
		t.Fatalf("B: expected %d, got %d", len(dataB), totalB.Load())
	}
}

// blockingReader simulates a slow upstream that blocks until a given time.
type blockingReader struct {
	data       []byte
	offset     int
	blockUntil time.Time
	blocked    bool
}

func (r *blockingReader) Read(p []byte) (int, error) {
	if !r.blocked {
		r.blocked = true
		time.Sleep(time.Until(r.blockUntil))
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

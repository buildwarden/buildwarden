package firecracker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// fakeSession stands in for driver.Session.Wait: it returns once complete is
// closed, or when its context ends, with the completion if one has landed
// (as Session.Wait does when completion races the tunnel teardown).
type fakeSession struct {
	complete chan struct{}
	result   error
}

func (s *fakeSession) wait(ctx context.Context) error {
	select {
	case <-s.complete:
		return s.result
	case <-ctx.Done():
		select {
		case <-s.complete:
			return s.result
		default:
			return ctx.Err()
		}
	}
}

// waitHarness runs waitBuild against channels the test closes in order. Its
// grace reports when waitBuild starts it and ends only when the test says, so
// no case depends on the clock (TEST-04).
type waitHarness struct {
	s            *fakeSession
	build, relay chan struct{}
	graceStarted chan struct{}
	graceEnd     chan time.Time
	result       chan error
}

func startWait(ctx context.Context, result error) *waitHarness {
	h := &waitHarness{
		s:     &fakeSession{complete: make(chan struct{}), result: result},
		build: make(chan struct{}), relay: make(chan struct{}),
		graceStarted: make(chan struct{}), graceEnd: make(chan time.Time),
		result: make(chan error, 1),
	}
	grace := func() <-chan time.Time {
		close(h.graceStarted)
		return h.graceEnd
	}
	go func() { h.result <- waitBuild(ctx, h.s.wait, h.build, h.relay, grace) }()
	return h
}

func TestWaitBuildExitOrdering(t *testing.T) {
	exit3 := &driver.ExitError{Code: 3}
	for _, tt := range []struct {
		name string
		// run closes the harness's channels in the case's order.
		run    func(h *waitHarness)
		result error
		want   error
	}{
		{"completion before the build VM exits", func(h *waitHarness) {
			close(h.s.complete)
		}, nil, nil},
		{"completion just after the build VM exits", func(h *waitHarness) {
			close(h.build)
			<-h.graceStarted
			close(h.s.complete)
		}, nil, nil},
		{"non-zero exit just after the build VM exits", func(h *waitHarness) {
			close(h.build)
			<-h.graceStarted
			close(h.s.complete)
		}, exit3, exit3},
		{"no completion after the build VM exits", func(h *waitHarness) {
			close(h.build)
			<-h.graceStarted
			close(h.graceEnd)
		}, nil, errBuildVMExited},
		// The grace never ends here, so waiting for it would hang the test.
		{"relay exits first", func(h *waitHarness) { close(h.relay) }, nil,
			errRelayVMExited},
		{"relay exits during the grace", func(h *waitHarness) {
			close(h.build)
			<-h.graceStarted
			close(h.relay)
		}, nil, errRelayVMExited},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := startWait(context.Background(), tt.result)
			tt.run(h)
			err := <-h.result
			if !errors.Is(err, tt.want) && err != tt.want {
				t.Fatalf("waitBuild = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWaitBuildParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := startWait(ctx, nil)
	cancel()
	if err := <-h.result; !errors.Is(err, context.Canceled) {
		t.Fatalf("waitBuild = %v, want the parent's cancellation", err)
	}
}

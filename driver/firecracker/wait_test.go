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

func TestWaitBuildExitOrdering(t *testing.T) {
	const grace = 200 * time.Millisecond
	exit3 := &driver.ExitError{Code: 3}
	for _, tt := range []struct {
		name string
		// steps run in order, each after the previous, on the test's clock.
		buildExit, relayExit, complete time.Duration // < 0: never
		result                         error
		want                           error
	}{
		{"completion before the build VM exits", 40 * time.Millisecond, -1,
			10 * time.Millisecond, nil, nil},
		{"completion just after the build VM exits", 10 * time.Millisecond, -1,
			60 * time.Millisecond, nil, nil},
		{"non-zero exit just after the build VM exits", 10 * time.Millisecond, -1,
			60 * time.Millisecond, exit3, exit3},
		{"no completion after the build VM exits", 10 * time.Millisecond, -1, -1, nil,
			errBuildVMExited},
		{"relay exits first", -1, 10 * time.Millisecond, -1, nil, errRelayVMExited},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &fakeSession{complete: make(chan struct{}), result: tt.result}
			build, relay := make(chan struct{}), make(chan struct{})
			after := func(d time.Duration, ch chan struct{}) {
				if d >= 0 {
					time.AfterFunc(d, func() { close(ch) })
				}
			}
			after(tt.buildExit, build)
			after(tt.relayExit, relay)
			after(tt.complete, s.complete)
			start := time.Now()
			err := waitBuild(context.Background(), s.wait, build, relay, grace)
			if !errors.Is(err, tt.want) && err != tt.want {
				t.Fatalf("waitBuild = %v, want %v", err, tt.want)
			}
			if tt.want == errRelayVMExited && time.Since(start) > grace {
				t.Fatal("a relay exit waited for the grace")
			}
		})
	}
}

func TestWaitBuildParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &fakeSession{complete: make(chan struct{})}
	time.AfterFunc(10*time.Millisecond, cancel)
	err := waitBuild(ctx, s.wait, make(chan struct{}), make(chan struct{}), time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitBuild = %v, want the parent's cancellation", err)
	}
}

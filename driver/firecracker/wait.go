package firecracker

import (
	"context"
	"errors"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// completionGrace is how long a build may still complete after its build
// VM's process exits (Decision 8). A successful build exits that way: the
// guest resets once warden-io has posted its completion, which can land just
// after the VMM is gone.
const completionGrace = 10 * time.Second

// afterGrace is waitBuild's grace in a real build: completionGrace from the
// build VM's exit.
func afterGrace() <-chan time.Time { return time.After(completionGrace) }

// Why waitBuild stopped the session's wait itself.
var (
	errBuildVMExited = errors.New("the build VM stopped before the build completed")
	errRelayVMExited = errors.New("the relay VM stopped before the build completed")
)

// waitBuild runs wait (the session's Wait) until the build completes, and
// watches both VMMs: when the build VM's process exits, a completion before
// the channel grace() returns fires still wins; when the relay VM's exits,
// the build fails at once. wait itself returns the completion when it has
// one, even if its context ends,
// so cancelling it after the grace yields a completion that raced the exit.
// A stop waitBuild caused is reported as errBuildVMExited or
// errRelayVMExited.
func waitBuild(ctx context.Context, wait func(context.Context) error,
	buildExited, relayExited <-chan struct{}, grace func() <-chan time.Time) error {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- wait(wctx) }()

	var cause error
	var graceEnd <-chan time.Time
	for {
		select {
		case err := <-done:
			var exit *driver.ExitError
			if err != nil && cause != nil && ctx.Err() == nil && !errors.As(err,
				&exit) {
				return cause
			}
			return err
		case <-buildExited:
			buildExited = nil
			graceEnd = grace()
		case <-graceEnd:
			graceEnd = nil
			cause = errBuildVMExited
			cancel()
		case <-relayExited:
			relayExited = nil
			if cause == nil {
				cause = errRelayVMExited
			}
			cancel()
		}
	}
}

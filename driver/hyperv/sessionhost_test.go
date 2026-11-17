package hyperv

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// sessionHost is the warden side of a build for the driver's tests: a
// driver.Host that claims the relay it is given with a real driver.Session,
// as warden's build sequence does, and asks the driver to stop once the
// collector has the build's completion or the driver reports an exit.
type sessionHost struct {
	t         *testing.T
	outputDir string
	token     string
	// claimTimeout bounds the claim; zero is driver.ClaimTimeout.
	claimTimeout time.Duration

	mu      sync.Mutex
	sess    *driver.Session
	relay   driver.RelayStarted
	started bool
	exits   []string
	// result is the build's completion: nil or a *driver.ExitError, or why
	// it ended without one.
	result error
	stop   chan struct{}
	once   sync.Once
}

func newSessionHost(t *testing.T, outputDir, token string) *sessionHost {
	t.Helper()
	h := &sessionHost{t: t, outputDir: outputDir, token: token,
		stop: make(chan struct{})}
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.sess != nil {
			h.sess.Close()
		}
	})
	return h
}

func (h *sessionHost) ClaimRelay(ctx context.Context, r driver.RelayStarted) error {
	sess, err := driver.StartSession(driver.SessionConfig{OutputDir: h.outputDir,
		GuestOS: r.GuestOS})
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.sess, h.relay = sess, r
	h.mu.Unlock()
	req := &driver.BuildRequest{UpstreamSystemCA: true}
	for _, s := range r.ClaimValues.UpstreamDNS {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return fmt.Errorf("upstream resolver %q: %w", s, err)
		}
		req.DNS = append(req.DNS, a)
	}
	if h.claimTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.claimTimeout)
		defer cancel()
	}
	_, err = sess.ClaimRelay(ctx, r.Claim.Address, h.token, req,
		r.ClaimValues.Environment, r.ClaimValues.BuildLink, r.OldRelayFix)
	return err
}

func (h *sessionHost) Started() error {
	h.mu.Lock()
	h.started = true
	sess := h.sess
	h.mu.Unlock()
	go func() {
		err := sess.Wait(context.Background())
		h.end(err)
	}()
	return nil
}

func (h *sessionHost) Exited(which driver.ExitedPart, detail string) {
	h.mu.Lock()
	h.exits = append(h.exits, string(which)+": "+detail)
	h.mu.Unlock()
	h.end(fmt.Errorf("%s exited: %s", which, detail))
}

// end records how the build ended, the first cause winning, and asks the
// driver to stop.
func (h *sessionHost) end(err error) {
	h.once.Do(func() {
		h.mu.Lock()
		h.result = err
		h.mu.Unlock()
		close(h.stop)
	})
}

func (h *sessionHost) Wait(ctx context.Context) string {
	select {
	case <-h.stop:
		return driver.StopCompleted
	case <-ctx.Done():
		return driver.StopInterrupted
	case <-time.After(30 * time.Second):
		h.t.Error("the build never ended")
		return driver.StopTimeout
	}
}

// outcome is how the build ended, and the exits the driver reported.
func (h *sessionHost) outcome() ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.exits...), h.result
}

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CompletionSignal is the decoded body of POST /v1/complete.
type CompletionSignal struct {
	ExitCode   int       `json:"exit_code"`
	Message    string    `json:"message"`
	Error      string    `json:"error"`
	ReceivedAt time.Time `json:"received_at"`
}

func (c *Collector) handleReady(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	c.logf("collector: ready")
	c.readyOnce.Do(func() { close(c.ready) })
	w.WriteHeader(http.StatusOK)
}

func (c *Collector) handleHeartbeat(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	c.MarkHeartbeat()
	w.WriteHeader(http.StatusOK)
}

func (c *Collector) handleComplete(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ExitCode int    `json:"exit_code"`
		Message  string `json:"message"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	sig := CompletionSignal{
		ExitCode:   body.ExitCode,
		Message:    body.Message,
		Error:      body.Error,
		ReceivedAt: time.Now(),
	}
	c.recordCompletion(sig)
	c.logf("collector: complete exit=%d msg=%q", sig.ExitCode, sig.Message)
	w.WriteHeader(http.StatusOK)
}

func (c *Collector) handleStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if sig, ok := c.Status(); ok {
		_ = enc.Encode(sig)
		return
	}
	_ = enc.Encode(map[string]string{"state": "running"})
}

func (c *Collector) recordCompletion(sig CompletionSignal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.completion == nil {
		s := sig
		c.completion = &s
	}
	if !c.doneClosed {
		c.doneClosed = true
		c.done <- sig
		close(c.done)
	}
}

// Status returns the first completion signal received, if any.
func (c *Collector) Status() (CompletionSignal, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.completion == nil {
		return CompletionSignal{}, false
	}
	return *c.completion, true
}

// Ready is closed when the relay posts /v1/ready.
func (c *Collector) Ready() <-chan struct{} { return c.ready }

// Done delivers the first completion signal, then is closed.
func (c *Collector) Done() <-chan CompletionSignal { return c.done }

// ErrUnresponsive is returned by Wait when the guest's heartbeat goes stale.
type ErrUnresponsive struct{ Silence time.Duration }

func (e *ErrUnresponsive) Error() string {
	return fmt.Sprintf("build guest unresponsive (no heartbeat for %s)", e.Silence)
}

// Wait blocks until the build completes, the guest's heartbeat goes stale for
// longer than heartbeatTimeout (only once a first heartbeat has arrived, so a
// slow boot is never mistaken for a hang), or ctx ends. heartbeatTimeout <= 0
// disables the liveness check.
func (c *Collector) Wait(ctx context.Context, heartbeatTimeout time.Duration) (CompletionSignal, error) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case sig := <-c.done:
			return sig, nil
		case <-ctx.Done():
			return CompletionSignal{}, ctx.Err()
		case <-tick.C:
			if heartbeatTimeout <= 0 {
				continue
			}
			last := c.LastHeartbeat()
			if !last.IsZero() && time.Since(last) > heartbeatTimeout {
				// A completion may have raced the tick.
				if sig, ok := c.Status(); ok {
					return sig, nil
				}
				return CompletionSignal{}, &ErrUnresponsive{Silence: heartbeatTimeout}
			}
		}
	}
}

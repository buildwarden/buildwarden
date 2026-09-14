package collector

import (
	"encoding/json"
	"net/http"
	"time"
)

// CompletionSignal is the terminal status a relay reports to the collector.
// The Hyper-V driver's relay reports readiness and build completion over this
// HTTP channel (replacing a Hyper-V COM named pipe), so an embedding driver
// can react in-process via Config.OnComplete or Collector.Done.
type CompletionSignal struct {
	ExitCode   int       `json:"exit_code"`
	Message    string    `json:"message"`
	Error      string    `json:"error"`
	ReceivedAt time.Time `json:"received_at"`
}

// handleReady serves POST /v1/ready: the relay is up. It carries no body,
// fires Config.OnReady if set, and always responds 200.
func (c *Collector) handleReady(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	c.logf("collector: ready")
	if c.cfg.OnReady != nil {
		c.cfg.OnReady()
	}
	w.WriteHeader(http.StatusOK)
}

// completeBody is the wire shape POST /v1/complete accepts.
type completeBody struct {
	ExitCode int    `json:"exit_code"`
	Message  string `json:"message"`
	Error    string `json:"error"`
}

// handleComplete serves POST /v1/complete: the build finished. It decodes the
// JSON body into a CompletionSignal (ReceivedAt = now), stores it as the last
// status, wakes any embedder waiting on Done, then fires Config.OnComplete if
// set. Malformed JSON yields 400.
func (c *Collector) handleComplete(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body completeBody
	dec := json.NewDecoder(req.Body)
	if err := dec.Decode(&body); err != nil {
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
	if c.cfg.OnComplete != nil {
		c.cfg.OnComplete(sig)
	}
	w.WriteHeader(http.StatusOK)
}

// handleStatus serves GET /v1/status. When a completion has been received it
// returns the CompletionSignal as JSON; otherwise it returns
// {"state":"running"}. It matches /healthz's unauthenticated stance.
func (c *Collector) handleStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	sig, ok := c.completed()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	if ok {
		_ = enc.Encode(sig)
		return
	}
	_ = enc.Encode(map[string]string{"state": "running"})
}

// recordCompletion stores the first completion signal and wakes a Done waiter.
// Only the first completion is delivered to Done and reflected by Status; later
// signals still fire OnComplete but do not overwrite the recorded status.
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

// completed returns the recorded completion signal, if any.
func (c *Collector) completed() (CompletionSignal, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.completion == nil {
		return CompletionSignal{}, false
	}
	return *c.completion, true
}

// Status reports the last completion signal, if one has been received.
func (c *Collector) Status() (CompletionSignal, bool) { return c.completed() }

// Done returns a channel that yields the first CompletionSignal received and
// is then closed. It is meant for a SINGLE embedder that owns the collector's
// lifecycle: read it once, e.g.
//
//	select {
//	case sig := <-c.Done():
//		// build finished; sig.ExitCode etc. are set
//	case <-ctx.Done():
//		// gave up waiting
//	}
//
// The channel is buffered (size 1), so the signal is delivered even if the
// completion arrives before the embedder starts waiting. After the value is
// consumed the channel is closed, so any further receive returns the zero
// CompletionSignal with ok == false — use Status to re-read the recorded
// signal instead of receiving twice.
func (c *Collector) Done() <-chan CompletionSignal { return c.done }

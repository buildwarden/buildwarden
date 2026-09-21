// Package relaycfg serves a relay VM its per-build configuration -- the
// collector sink URL and bearer token -- which the relay pulls once at boot.
//
// It is the host side of the "pull-at-boot" delivery used by the Hyper-V driver,
// where the relay VM has no data disk and the UKI kernel cmdline is static, so
// there is no in-image injection point for per-build config. The driver mints a
// per-build token (the collector separates concurrent builds by token), runs one
// Responder per build bound to that build's isolated NAT gateway address, and
// closes it once the relay reports ready. The relay's init does a single HTTP
// GET, parses OUTPUT_SINK_URL / OUTPUT_SINK_TOKEN, and starts the relay with them
// in its environment.
//
// The response is strict KEY=VALUE env lines (not JSON) so the minimal Alpine
// init can parse it without a JSON tool and, crucially, without sourcing the
// body -- init matches only known keys, so a response can never execute shell in
// the relay. The config is served exactly once as defense in depth: the relay
// fetches a single time at boot, so any later GET (from anything else that can
// reach the port) gets 410 Gone.
package relaycfg

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Responder serves the per-build relay config exactly once.
type Responder struct {
	body []byte

	mu     sync.Mutex
	served bool
}

// New builds a Responder that serves the given collector sink URL and token.
// It rejects values containing CR or LF, which would let a caller inject extra
// env lines into the response body.
func New(sinkURL, token string) (*Responder, error) {
	if strings.ContainsAny(sinkURL, "\r\n") || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("relaycfg: sink URL and token must not contain CR/LF")
	}
	if sinkURL == "" {
		return nil, fmt.Errorf("relaycfg: sink URL is required")
	}
	body := fmt.Sprintf("OUTPUT_SINK_URL=%s\nOUTPUT_SINK_TOKEN=%s\n", sinkURL, token)
	return &Responder{body: []byte(body)}, nil
}

// Handler returns the HTTP handler serving GET /config. The caller binds a
// server using it to the build's NAT gateway address (never 0.0.0.0) and closes
// that server once the relay is up.
func (r *Responder) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/config", r.handleConfig)
	return mux
}

func (r *Responder) handleConfig(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.served {
		http.Error(w, "config already served", http.StatusGone)
		return
	}
	r.served = true
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(r.body)
}

// Served reports whether the config has been fetched. A driver can use this (or
// the collector's ready signal) to decide when to close the responder.
func (r *Responder) Served() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.served
}

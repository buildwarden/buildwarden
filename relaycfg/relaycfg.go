// Package relaycfg is the claim protocol between an orchestrator and a relay.
//
// A relay boots unconfigured and listens for a single claim. The orchestrator
// pushes the per-build Config (where the collector is, how to authenticate to
// it, capture and upstream-TLS settings, the environment record) and gets back
// the relay's identity: its ephemeral MITM CA certificate and its ledger
// signing key. After one successful claim the relay closes the listener.
//
// Pushing (rather than having the relay fetch its config at boot) keeps the
// relay ignorant of the host's topology: the orchestrator chooses every
// address, a relay can be started ahead of time and wait warm until a build
// claims it, and concurrent relays never race for a fixed config endpoint.
//
// Transport: the relay never dials the host. Before claiming, the
// orchestrator opens a tunnel on the claim port (POST /v1/tunnel, upgraded to
// a raw connection) and serves the collector over it as HTTP/2, so all of the
// relay's collector traffic rides a connection the host initiated. Any
// topology that lets the host reach the relay's claim port therefore works,
// with no host listener, firewall rule or host-reachability assumption. A
// claim may instead name a CollectorURL the relay dials directly (e.g. a
// remote collector service); TunnelURL selects the tunnel.
//
// The claim endpoint is only ever exposed host-locally (a forwarded or
// published host-only port, or a host-only switch), and the build guest is
// not started until the claim has completed and the listener has closed. An
// optional per-instance claim token, fixed when the relay is created,
// additionally stops another local process from claiming a warm relay first.
package relaycfg

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultPort is the relay's claim port.
const DefaultPort = 8301

// TokenEnv names the environment variable carrying the claim token.
const TokenEnv = "WARDEN_CLAIM_TOKEN"

const (
	claimPath     = "/v1/claim"
	tunnelPath    = "/v1/tunnel"
	tunnelUpgrade = "warden-tunnel"
)

// TunnelURL as Config.CollectorURL tells the relay to reach its collector over
// the tunnel the orchestrator opened before claiming.
const TunnelURL = "http://collector.tunnel"

// Config is everything a relay needs for one build.
type Config struct {
	// CollectorURL is the collector's base URL as the relay reaches it, or
	// TunnelURL to use the orchestrator's tunnel. Required.
	CollectorURL string `json:"collector_url"`
	// CollectorToken is the bearer token for every collector request.
	CollectorToken string `json:"collector_token"`
	// CaptureMode is "", "none", "headers", "bodies" or "all".
	CaptureMode string `json:"capture_mode,omitempty"`
	// UpstreamCACerts are extra PEM bundles trusted for upstream TLS.
	UpstreamCACerts []string `json:"upstream_ca_certs,omitempty"`
	// UpstreamSystemCA, when false, excludes the system pool (only honored
	// together with UpstreamCACerts).
	UpstreamSystemCA *bool `json:"upstream_system_ca,omitempty"`
	// Environment, when set, is recorded as the ledger's first entry.
	Environment *Environment `json:"environment,omitempty"`
}

// Environment describes the build's base environment (e.g. an OCI manifest).
type Environment struct {
	Payload  []byte `json:"payload"`
	Metadata []byte `json:"metadata"` // CBOR
}

// Identity is what a claimed relay returns.
type Identity struct {
	// CACert is the PEM-encoded ephemeral MITM CA the guest must trust.
	CACert string `json:"ca_cert"`
	// LedgerPublicKey is the hex Ed25519 key that signs this build's ledger.
	LedgerPublicKey string `json:"ledger_public_key"`
}

// Serve accepts exactly one authorized claim on ln, calls start with its
// Config and the collector transport (the tunnel's HTTP/2 client when the
// claim selects TunnelURL, else nil for a direct CollectorURL), replies with
// the returned Identity, and then closes ln. Requests with a wrong token are
// refused and do not consume the claim. If start fails, the error is reported
// to the claimant and returned.
func Serve(ln net.Listener, token string, start func(Config, http.RoundTripper) (Identity, error)) error {
	var (
		mu      sync.Mutex
		claimed bool
		tunnel  http.RoundTripper
		result  = make(chan error, 1)
	)
	authorized := func(w http.ResponseWriter, req *http.Request) bool {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return false
		}
		got := strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer"))
		if token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
		return true
	}
	release := func() {
		mu.Lock()
		claimed = false // a refused claim does not consume the relay
		mu.Unlock()
	}

	mux := http.NewServeMux()
	mux.HandleFunc(tunnelPath, func(w http.ResponseWriter, req *http.Request) {
		if !authorized(w, req) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if tunnel != nil || claimed {
			http.Error(w, "tunnel already open", http.StatusConflict)
			return
		}
		if rt, err := acceptTunnel(w); err == nil {
			tunnel = rt
		}
	})
	mux.HandleFunc(claimPath, func(w http.ResponseWriter, req *http.Request) {
		if !authorized(w, req) {
			return
		}
		mu.Lock()
		if claimed {
			mu.Unlock()
			http.Error(w, "already claimed", http.StatusConflict)
			return
		}
		claimed = true
		rt := tunnel
		mu.Unlock()

		var cfg Config
		dec := json.NewDecoder(io.LimitReader(req.Body, 16<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			release()
			http.Error(w, "invalid claim: "+err.Error(), http.StatusBadRequest)
			return
		}
		switch {
		case cfg.CollectorURL == "":
			release()
			http.Error(w, "invalid claim: collector_url is required", http.StatusBadRequest)
			return
		case cfg.CollectorURL == TunnelURL && rt == nil:
			release()
			http.Error(w, "invalid claim: collector_url selects the tunnel, but none is open",
				http.StatusBadRequest)
			return
		case cfg.CollectorURL != TunnelURL:
			rt = nil
		}
		id, err := start(cfg, rt)
		if err != nil {
			http.Error(w, "relay start failed: "+err.Error(), http.StatusInternalServerError)
			result <- err
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(id)
		result <- nil
	})

	// Shutdown leaves the hijacked tunnel connection open.
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-result:
		// Let the reply flush before closing the listener.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		return err
	case err := <-served:
		return fmt.Errorf("relaycfg: claim listener: %w", err)
	}
}

// ErrRejected wraps a definitive refusal from the relay (bad token, already
// claimed, invalid config, failed start); Claim does not retry these.
var ErrRejected = errors.New("relaycfg: claim rejected")

// Claim pushes cfg to the relay listening at addr (host:port), retrying
// connection failures until the relay answers or ctx ends.
func Claim(ctx context.Context, addr, token string, cfg Config) (Identity, error) {
	body, err := json.Marshal(cfg)
	if err != nil {
		return Identity{}, err
	}
	url := "http://" + addr + claimPath
	client := &http.Client{Timeout: 60 * time.Second}
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return Identity{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				return Identity{}, fmt.Errorf("%w: HTTP %d: %s",
					ErrRejected, resp.StatusCode, strings.TrimSpace(string(msg)))
			}
			var id Identity
			if err := json.NewDecoder(resp.Body).Decode(&id); err != nil {
				return Identity{}, fmt.Errorf("relaycfg: decoding identity: %w", err)
			}
			if id.CACert == "" {
				return Identity{}, fmt.Errorf("relaycfg: relay returned no CA certificate")
			}
			return id, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return Identity{}, fmt.Errorf("relaycfg: relay at %s never accepted the claim: %w (last error: %v)",
				addr, ctx.Err(), lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// oldRelayConfig is relaycfg.Config as a relay that predates build_link
// decodes it.
type oldRelayConfig struct {
	CollectorURL     string                `json:"collector_url"`
	CollectorToken   string                `json:"collector_token"`
	CaptureMode      string                `json:"capture_mode,omitempty"`
	UpstreamCACerts  []string              `json:"upstream_ca_certs,omitempty"`
	UpstreamSystemCA *bool                 `json:"upstream_system_ca,omitempty"`
	Environment      *relaycfg.Environment `json:"environment,omitempty"`
}

// oldRelay serves the claim protocol the way a relay built before build_link
// does: it accepts the tunnel and decodes the claim strictly into the old
// Config.
func oldRelay(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tunnel", func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
			"Connection: Upgrade\r\nUpgrade: warden-tunnel\r\n\r\n")
		_ = rw.Flush()
		t.Cleanup(func() { _ = conn.Close() })
	})
	mux.HandleFunc("/v1/claim", func(w http.ResponseWriter, r *http.Request) {
		var c oldRelayConfig
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			http.Error(w, "invalid claim: "+err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "this fake only refuses", http.StatusTeapot)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func newSession(t *testing.T) *Session {
	t.Helper()
	s, err := StartSession(SessionConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// TestClaimRelayOldRelay is the old relay, new warden row: the refusal is a
// FixableError carrying the caller's fix.
func TestClaimRelayOldRelay(t *testing.T) {
	s := newSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := s.ClaimRelay(ctx, oldRelay(t), "", &BuildRequest{}, nil,
		buildlink.Default().Claim(), "refresh the relay")
	var fe *FixableError
	if !errors.As(err, &fe) || fe.Fix != "refresh the relay" ||
		!strings.Contains(fe.Msg, `unknown field "build_link"`) {
		t.Fatalf("err = %#v, want a FixableError with the caller's fix", err)
	}
}

// TestClaimRelayLinks covers what the claim carries: the link when given, no
// field for nil, and a relay's refusal of a claim with no link on an
// unaddressed build interface (the new relay, old warden row) reaching the
// caller as the relay's own message.
func TestClaimRelayLinks(t *testing.T) {
	configured, _ := buildlink.Parse("100.126.255.252/30")
	tests := []struct {
		name string
		link *relaycfg.BuildLink
		want string
	}{
		{"default", buildlink.Default().Claim(), "100.127.255.252/30"},
		{"configured", configured.Claim(), "100.126.255.252/30"},
		{"none", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make(chan *relaycfg.BuildLink, 1)
			addr, _ := fakeRelayWith(t, "", func(c relaycfg.Config) error {
				got <- c.BuildLink
				return nil
			})
			s := newSession(t)
			if _, err := s.ClaimRelay(context.Background(), addr, "",
				&BuildRequest{UpstreamSystemCA: true}, nil, tt.link,
				"fix"); err != nil {
				t.Fatalf("ClaimRelay: %v", err)
			}
			b := <-got
			if tt.want == "" && b != nil || tt.want != "" && (b == nil ||
				b.Prefix != tt.want) {
				t.Fatalf("relay received build_link %+v, want %q", b, tt.want)
			}
		})
	}

	addr, _ := fakeRelayWith(t, "", func(c relaycfg.Config) error {
		if c.BuildLink == nil {
			return fmt.Errorf("%w: build_link is required: the build interface eth0 "+
				"has no address", relaycfg.ErrRefused)
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := newSession(t).ClaimRelay(ctx, addr, "", &BuildRequest{}, nil, nil, "fix")
	if !errors.Is(err, relaycfg.ErrRejected) || !strings.Contains(err.Error(),
		"build_link is required") {
		t.Fatalf("err = %v, want the relay's build_link is required refusal", err)
	}
}

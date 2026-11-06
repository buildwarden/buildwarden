package relaycfg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var defaultLink = BuildLink{
	Prefix: "100.127.255.252/30", Relay: "100.127.255.253", Guest: "100.127.255.254",
}

func TestBuildLinkParse(t *testing.T) {
	const (
		net30 = "100.127.255.252/30"
		r30   = "100.127.255.253"
		g30   = "100.127.255.254"
	)
	tests := []struct {
		name string
		link BuildLink
		want string // substring of the error; empty for valid
	}{
		{"default", defaultLink, ""},
		{"wider segment", BuildLink{"192.0.2.0/24", "192.0.2.1", "192.0.2.77"}, ""},
		{"slash 8", BuildLink{"100.0.0.0/8", "100.0.0.1", "100.0.0.2"}, ""},
		{"bad prefix", BuildLink{"nope", "1.1.1.1", "1.1.1.2"}, "prefix"},
		{"ipv6", BuildLink{"fd00::/126", "fd00::1", "fd00::2"}, "not IPv4"},
		{"not network", BuildLink{r30 + "/30", r30, g30}, "did you mean " + net30},
		{"too long", BuildLink{g30 + "/31", g30, "100.127.255.255"},
			"length must be 8 to 30"},
		{"too short", BuildLink{"96.0.0.0/7", "96.0.0.1", "96.0.0.2"},
			"length must be 8 to 30"},
		{"relay outside", BuildLink{net30, "100.127.255.1", g30},
			"relay 100.127.255.1 is outside"},
		{"guest network", BuildLink{net30, r30, "100.127.255.252"},
			"guest 100.127.255.252 is the network or broadcast"},
		{"guest broadcast", BuildLink{net30, r30, "100.127.255.255"},
			"network or broadcast"},
		{"same", BuildLink{net30, r30, r30}, "both"},
		{"guest ipv6", BuildLink{net30, r30, "::1"}, "not IPv4"},
		{"zero net", BuildLink{"0.0.0.0/30", "0.0.0.1", "0.0.0.2"}, "0.0.0.0/8"},
		{"loopback", BuildLink{"127.0.0.0/30", "127.0.0.1", "127.0.0.2"}, "loopback"},
		{"link-local", BuildLink{"169.254.1.0/30", "169.254.1.1", "169.254.1.2"},
			"link-local"},
		{"multicast", BuildLink{"224.0.0.0/30", "224.0.0.1", "224.0.0.2"}, "multicast"},
		{"class e", BuildLink{"240.0.0.0/30", "240.0.0.1", "240.0.0.2"}, "class E"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, r, g, err := tt.link.Parse()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if p.String() != tt.link.Prefix || r.String() != tt.link.Relay ||
					g.String() != tt.link.Guest {
					t.Fatalf("Parse = %v %v %v", p, r, g)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// serveOnce runs Serve with start in the background and returns its address
// and the channel Serve's result arrives on.
func serveOnce(t *testing.T,
	start func(Config, http.RoundTripper) (Identity, error)) (string, <-chan error) {
	t.Helper()
	ln := listen(t)
	served := make(chan error, 1)
	go func() { served <- Serve(ln, "", start) }()
	return ln.Addr().String(), served
}

func claimCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestClaimBuildLinkRoundTrip(t *testing.T) {
	got := make(chan Config, 1)
	addr, served := serveOnce(t, func(c Config, _ http.RoundTripper) (Identity, error) {
		got <- c
		return Identity{CACert: "PEM"}, nil
	})
	link := defaultLink
	if _, err := Claim(claimCtx(t), addr, "",
		Config{CollectorURL: "http://x", BuildLink: &link}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c := <-got; c.BuildLink == nil || *c.BuildLink != defaultLink {
		t.Fatalf("relay received build_link %+v", c.BuildLink)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

// TestClaimRefusalsLeaveRelayClaimable covers each refusal that must not
// consume the relay: an invalid build_link, and a start step that refuses
// with ErrRefused. A valid claim succeeds afterwards.
func TestClaimRefusalsLeaveRelayClaimable(t *testing.T) {
	tests := []struct {
		name string
		link BuildLink
		want string
	}{
		{"invalid prefix",
			BuildLink{"100.127.255.253/30", "100.127.255.253", "100.127.255.254"},
			"invalid claim: build_link: prefix"},
		{"invalid guest",
			BuildLink{"100.127.255.252/30", "100.127.255.253", "192.0.2.9"},
			"invalid claim: build_link: guest"},
		{"start refuses", BuildLink{"192.0.2.0/30", "192.0.2.1", "192.0.2.2"},
			"claim refused: overlaps eth1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, served := serveOnce(t, func(
				c Config, _ http.RoundTripper,
			) (Identity, error) {
				if c.BuildLink != nil && c.BuildLink.Prefix == "192.0.2.0/30" {
					return Identity{},
						fmt.Errorf("%w: overlaps eth1", ErrRefused)
				}
				return Identity{CACert: "PEM"}, nil
			})
			ctx := claimCtx(t)
			link := tt.link
			_, err := Claim(ctx, addr, "",
				Config{CollectorURL: "http://x", BuildLink: &link})
			if !errors.Is(err, ErrRejected) ||
				!strings.Contains(err.Error(), "HTTP 400") ||
				!strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want a 400 containing %q", err, tt.want)
			}
			good := defaultLink
			if _, err := Claim(ctx, addr, "",
				Config{CollectorURL: "http://x", BuildLink: &good}); err != nil {
				t.Fatalf("valid claim after a refused one: %v", err)
			}
			if err := <-served; err != nil {
				t.Fatalf("Serve: %v", err)
			}
		})
	}
}

// TestClaimWithoutBuildLink is the new relay, old orchestrator row: a claim
// with no build_link decodes and starts as before.
func TestClaimWithoutBuildLink(t *testing.T) {
	got := make(chan Config, 1)
	addr, _ := serveOnce(t, func(c Config, _ http.RoundTripper) (Identity, error) {
		got <- c
		return Identity{CACert: "PEM"}, nil
	})
	if _, err := Claim(claimCtx(t), addr, "", Config{CollectorURL: "http://x"}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c := <-got; c.BuildLink != nil {
		t.Fatalf("build_link = %+v, want nil", c.BuildLink)
	}
}

// oldConfig is Config as it was before build_link, decoded the way an old
// relay decodes it.
type oldConfig struct {
	CollectorURL     string       `json:"collector_url"`
	CollectorToken   string       `json:"collector_token"`
	CaptureMode      string       `json:"capture_mode,omitempty"`
	UpstreamCACerts  []string     `json:"upstream_ca_certs,omitempty"`
	UpstreamSystemCA *bool        `json:"upstream_system_ca,omitempty"`
	Environment      *Environment `json:"environment,omitempty"`
}

// TestOldRelayRefusesBuildLink is the old relay, new orchestrator row: the
// refusal text is what driver.Session.ClaimRelay maps to a fix.
func TestOldRelayRefusesBuildLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c oldConfig
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			http.Error(w, "invalid claim: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(Identity{CACert: "PEM"})
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	link := defaultLink
	_, err := Claim(claimCtx(t), addr, "",
		Config{CollectorURL: "http://x", BuildLink: &link})
	if !errors.Is(err, ErrRejected) ||
		!strings.Contains(err.Error(), `unknown field "build_link"`) {
		t.Fatalf("err = %v, want an ErrRejected naming the unknown field", err)
	}
	if _, err := Claim(claimCtx(t), addr, "", Config{CollectorURL: "http://x"}); err != nil {
		t.Fatalf("claim without build_link to an old relay: %v", err)
	}
}

// TestReplyWaitsForStart pins the ordering rule: the claimant gets no reply
// until the start callback (where a mode configures its segment) returns.
func TestReplyWaitsForStart(t *testing.T) {
	release := make(chan struct{})
	addr, _ := serveOnce(t, func(Config, http.RoundTripper) (Identity, error) {
		<-release
		return Identity{CACert: "PEM"}, nil
	})
	done := make(chan error, 1)
	go func() {
		link := defaultLink
		_, err := Claim(claimCtx(t), addr, "",
			Config{CollectorURL: "http://x", BuildLink: &link})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Claim returned before start did: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Claim: %v", err)
	}
}

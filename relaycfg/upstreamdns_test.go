package relaycfg

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestClaimUpstreamDNSRoundTrip(t *testing.T) {
	got := make(chan Config, 1)
	addr, served := serveOnce(t, func(c Config, _ http.RoundTripper) (Identity, error) {
		got <- c
		return Identity{CACert: "PEM"}, nil
	})
	want := []string{"10.20.0.53", "10.20.0.54"}
	link := defaultLink
	if _, err := Claim(claimCtx(t), addr, "", Config{
		CollectorURL: "http://x", BuildLink: &link, UpstreamDNS: want,
	}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c := <-got; !slices.Equal(c.UpstreamDNS, want) {
		t.Fatalf("relay received upstream_dns %v, want %v", c.UpstreamDNS, want)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestUpstreamDNSOmittedWhenEmpty(t *testing.T) {
	b, err := json.Marshal(Config{CollectorURL: "http://x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "upstream_dns") {
		t.Fatalf("marshalled %s, want no upstream_dns", b)
	}
}

func TestParseUpstreamDNS(t *testing.T) {
	link := defaultLink
	tests := []struct {
		name string
		list []string
		link *BuildLink
		want string // substring of the error; empty for valid
	}{
		{"one", []string{"10.20.0.53"}, &link, ""},
		{"ordered", []string{"9.9.9.9", "1.1.1.1"}, nil, ""},
		{"docker embedded", []string{"127.0.0.11"}, &link, ""},
		{"empty", []string{}, nil, "empty list"},
		{"ipv6", []string{"2001:db8::53"}, nil, "not IPv4"},
		{"name", []string{"dns.example"}, nil, "not an IP address"},
		{"unspecified", []string{"0.0.0.0"}, nil, "unspecified"},
		{"multicast", []string{"224.0.0.251"}, nil, "multicast"},
		{"broadcast", []string{"255.255.255.255"}, nil, "broadcast"},
		{"inside link", []string{"10.20.0.53", "100.127.255.253"}, &link,
			"entry 1 100.127.255.253 is inside the build link"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUpstreamDNS(tt.list, tt.link)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("ParseUpstreamDNS: %v", err)
				}
				var strs []string
				for _, a := range got {
					strs = append(strs, a.String())
				}
				if !slices.Equal(strs, tt.list) {
					t.Fatalf("ParseUpstreamDNS = %v, want %v in order", strs, tt.list)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestClaimUpstreamDNSRefusalsLeaveRelayClaimable checks that every invalid
// upstream_dns is a 400 that does not consume the relay.
func TestClaimUpstreamDNSRefusalsLeaveRelayClaimable(t *testing.T) {
	tests := []struct {
		name string
		list []string
		want string
	}{
		{"empty", []string{}, "empty list"},
		{"ipv6", []string{"2001:db8::53"}, "not IPv4"},
		{"name", []string{"dns.example"}, "not an IP address"},
		{"unspecified", []string{"0.0.0.0"}, "unspecified"},
		{"multicast", []string{"239.1.1.1"}, "multicast"},
		{"broadcast", []string{"255.255.255.255"}, "broadcast"},
		{"inside link", []string{"100.127.255.254"}, "inside the build link"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, served := serveOnce(t, func(Config, http.RoundTripper) (Identity, error) {
				return Identity{CACert: "PEM"}, nil
			})
			ctx := claimCtx(t)
			link := defaultLink
			cfg := Config{CollectorURL: "http://x", BuildLink: &link, UpstreamDNS: tt.list}
			_, err := claimRaw(t, addr, cfg)
			if !errors.Is(err, ErrRejected) ||
				!strings.Contains(err.Error(), "HTTP 400") ||
				!strings.Contains(err.Error(), "invalid claim: upstream_dns: ") ||
				!strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want a 400 containing %q", err, tt.want)
			}
			if _, err := Claim(ctx, addr, "", Config{
				CollectorURL: "http://x", BuildLink: &link,
				UpstreamDNS: []string{"10.20.0.53"},
			}); err != nil {
				t.Fatalf("valid claim after a refused one: %v", err)
			}
			if err := <-served; err != nil {
				t.Fatalf("Serve: %v", err)
			}
		})
	}
}

// claimRaw posts cfg with upstream_dns always present: omitempty would drop
// an empty list, which must reach the relay as a present, empty field.
func claimRaw(t *testing.T, addr string, cfg Config) (Identity, error) {
	t.Helper()
	type alias Config
	body, err := json.Marshal(struct {
		alias
		UpstreamDNS []string `json:"upstream_dns"`
	}{alias(cfg), cfg.UpstreamDNS})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post("http://"+addr+claimPath, "application/json",
		strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	return parseClaimResponse(resp)
}

// TestClaimWithoutUpstreamDNS is the new relay, claimant-sends-no-list row: a
// claim with no upstream_dns decodes and starts.
func TestClaimWithoutUpstreamDNS(t *testing.T) {
	got := make(chan Config, 1)
	addr, _ := serveOnce(t, func(c Config, _ http.RoundTripper) (Identity, error) {
		got <- c
		return Identity{CACert: "PEM"}, nil
	})
	if _, err := Claim(claimCtx(t), addr, "", Config{CollectorURL: "http://x"}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c := <-got; c.UpstreamDNS != nil {
		t.Fatalf("upstream_dns = %v, want nil", c.UpstreamDNS)
	}
}

// preUpstreamDNSConfig is Config as it was before upstream_dns, decoded the
// way a relay of that age decodes it.
type preUpstreamDNSConfig struct {
	oldConfig
	BuildLink *BuildLink `json:"build_link,omitempty"`
}

// TestOldRelayRefusesUpstreamDNS is the old relay, new orchestrator row: the
// refusal text is what driver.Session.ClaimRelay maps to a fix.
func TestOldRelayRefusesUpstreamDNS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c preUpstreamDNSConfig
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
	_, err := Claim(claimCtx(t), addr, "", Config{
		CollectorURL: "http://x", BuildLink: &link, UpstreamDNS: []string{"10.20.0.53"},
	})
	if !errors.Is(err, ErrRejected) ||
		!strings.Contains(err.Error(), `unknown field "upstream_dns"`) {
		t.Fatalf("err = %v, want an ErrRejected naming the unknown field", err)
	}
	if _, err := Claim(claimCtx(t), addr, "",
		Config{CollectorURL: "http://x", BuildLink: &link}); err != nil {
		t.Fatalf("claim without upstream_dns to an old relay: %v", err)
	}
}

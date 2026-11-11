package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/relay"
	"github.com/buildwarden/buildwarden/relaycfg"
)

func confReader(conf string, err error) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(conf), err }
}

func TestUpstreamResolvers(t *testing.T) {
	link := &relaycfg.BuildLink{
		Prefix: "100.127.255.252/30", Relay: "100.127.255.253", Guest: "100.127.255.254",
	}
	base := relay.Config{
		SelfIP:    net.ParseIP("100.127.255.253"),
		BuildLink: netip.MustParsePrefix("100.127.255.252/30"),
	}
	tests := []struct {
		name       string
		claim      relaycfg.Config
		conf       func() ([]byte, error)
		want       string // the list, space-separated
		wantSource string
		wantErr    string
	}{
		{"claim list used exclusively",
			relaycfg.Config{BuildLink: link, UpstreamDNS: []string{"10.20.0.53", "10.20.0.54"}},
			confReader("nameserver 9.9.9.9\n", nil),
			"10.20.0.53 10.20.0.54", relay.SourceClaim, ""},
		{"absent: resolv.conf",
			relaycfg.Config{BuildLink: link},
			confReader("nameserver 100.127.255.254\nnameserver 127.0.0.11\nnameserver 10.0.0.2\n", nil),
			"127.0.0.11 10.0.0.2", relay.SourceResolvConf, ""},
		{"absent: nothing usable",
			relaycfg.Config{BuildLink: link},
			confReader("nameserver 127.0.0.1\nnameserver 100.127.255.253\n", nil),
			"", "", "upstream_dns is absent and /etc/resolv.conf names no usable nameserver"},
		{"absent: no file",
			relaycfg.Config{BuildLink: link},
			confReader("", errors.New("no such file")),
			"", "", "upstream_dns is absent and the relay's default cannot be read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, source, err := upstreamResolvers(tt.claim, base, tt.conf)
			if tt.wantErr != "" {
				if !errors.Is(err, relaycfg.ErrRefused) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want ErrRefused containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("upstreamResolvers: %v", err)
			}
			var got []string
			for _, a := range list {
				got = append(got, a.String())
			}
			if strings.Join(got, " ") != tt.want || source != tt.wantSource {
				t.Fatalf("got %v (%s), want %s (%s)", got, source, tt.want, tt.wantSource)
			}
		})
	}
}

// TestAbsentUpstreamDNSRefusalLeavesRelayClaimable checks the absent-field
// default's refusal is a 400 naming upstream_dns that does not consume the
// relay: a claim carrying the field succeeds afterwards.
func TestAbsentUpstreamDNSRefusalLeavesRelayClaimable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sources := make(chan string, 2)
	go func() {
		_ = relaycfg.Serve(ln, "", func(c relaycfg.Config,
			_ http.RoundTripper) (relaycfg.Identity, error) {
			_, source, err := upstreamResolvers(c, relay.Config{},
				confReader("nameserver 127.0.0.1\n", nil))
			if err != nil {
				return relaycfg.Identity{}, err
			}
			sources <- source
			return relaycfg.Identity{CACert: "PEM"}, nil
		})
	}()
	addr := ln.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = relaycfg.Claim(ctx, addr, "", relaycfg.Config{CollectorURL: "http://x"})
	if !errors.Is(err, relaycfg.ErrRejected) || !strings.Contains(err.Error(), "HTTP 400") ||
		!strings.Contains(err.Error(), "upstream_dns") {
		t.Fatalf("err = %v, want a 400 naming upstream_dns", err)
	}
	if _, err := relaycfg.Claim(ctx, addr, "", relaycfg.Config{
		CollectorURL: "http://x", UpstreamDNS: []string{"10.20.0.53"},
	}); err != nil {
		t.Fatalf("claim with upstream_dns after the refusal: %v", err)
	}
	if s := <-sources; s != relay.SourceClaim {
		t.Fatalf("source = %q, want %q", s, relay.SourceClaim)
	}
}

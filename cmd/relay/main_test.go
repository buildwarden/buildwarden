package main

import (
	"net"
	"testing"

	"github.com/buildwarden/buildwarden/relay"
	"github.com/buildwarden/buildwarden/relaycfg"
)

func TestApplyClaimOverlaysBuildConfig(t *testing.T) {
	gw := net.ParseIP("10.100.0.1")
	base := relay.Config{SelfIP: gw, SSRF: true}
	f := false
	cfg := applyClaim(base, relaycfg.Config{
		CollectorURL:     "http://127.0.0.1:9",
		CollectorToken:   "tok",
		CaptureMode:      "all",
		UpstreamCACerts:  []string{"PEM-A", "PEM-B"},
		UpstreamSystemCA: &f,
		Environment:      &relaycfg.Environment{Payload: []byte("p")},
	})
	if !cfg.SSRF || !cfg.SelfIP.Equal(gw) {
		t.Fatal("mode base settings were lost")
	}
	if cfg.CollectorURL != "http://127.0.0.1:9" || cfg.CollectorToken != "tok" ||
		cfg.CaptureMode != "all" || cfg.Environment == nil {
		t.Fatalf("claim not applied: %+v", cfg)
	}
	if len(cfg.UpstreamCACerts) != 2 || string(cfg.UpstreamCACerts[1]) != "PEM-B" {
		t.Fatalf("upstream CAs = %q", cfg.UpstreamCACerts)
	}
	if cfg.UpstreamSystemCA == nil || *cfg.UpstreamSystemCA {
		t.Fatal("UpstreamSystemCA=false not applied")
	}
}

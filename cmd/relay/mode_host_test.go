//go:build unix

package main

import (
	"net"
	"testing"

	"github.com/buildwarden/buildwarden/relaycfg"
)

type fakeIngress struct {
	cfg    FDIngressConfig
	closed bool
}

func (f *fakeIngress) ListenUDP(uint16) (net.PacketConn, error) { return nil, nil }
func (f *fakeIngress) Close()                                   { f.closed = true }

func TestHostLinkStart(t *testing.T) {
	fallback, err := subnetLink("10.100.0.0/30")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name                    string
		link                    *relaycfg.BuildLink
		gw, guest, mask, prefix string
	}{
		{"from the claim", defaultLink,
			"100.127.255.253", "100.127.255.254", "fffffffc", "100.127.255.252/30"},
		{"from a configured claim", otherLink,
			"100.126.255.253", "100.126.255.254", "fffffffc", "100.126.255.252/30"},
		{"from --subnet", nil, "10.100.0.1", "10.100.0.2", "fffffffc", "10.100.0.0/30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ing *fakeIngress
			h := hostLink{fd: 3, fallback: fallback,
				newIngress: func(c FDIngressConfig) (hostIngress, error) {
					ing = &fakeIngress{cfg: c}
					return ing, nil
				}}
			cfg, cleanup, err := h.start(relaycfg.Config{BuildLink: tt.link})
			if err != nil {
				t.Fatal(err)
			}
			if ing.cfg.GatewayIP.String() != tt.gw ||
				ing.cfg.GuestIP.String() != tt.guest ||
				ing.cfg.SubnetMask.String() != tt.mask {
				t.Fatalf("ingress = %+v", ing.cfg)
			}
			if !cfg.SSRF || cfg.SelfIP.String() != tt.gw ||
				cfg.BuildLink.String() != tt.prefix {
				t.Fatalf("relay config SSRF %v SelfIP %s BuildLink %s",
					cfg.SSRF, cfg.SelfIP, cfg.BuildLink)
			}
			if cfg.HTTPListener == nil || cfg.HTTPSListener == nil ||
				cfg.ControlListener == nil {
				t.Fatal("channel listeners missing")
			}
			cleanup()
			if !ing.closed {
				t.Fatal("cleanup left the ingress open")
			}
		})
	}
}

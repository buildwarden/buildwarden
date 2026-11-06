package buildlink

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/relaycfg"
)

func TestDefault(t *testing.T) {
	if defaultErr != nil {
		t.Fatalf("DefaultCIDR does not parse: %v", defaultErr)
	}
	l := Default()
	if l.Net.String() != DefaultCIDR || l.Relay.String() != "100.127.255.253" ||
		l.Guest.String() != "100.127.255.254" {
		t.Fatalf("Default() = %+v", l)
	}
	if _, _, _, err := l.Claim().Parse(); err != nil {
		t.Fatalf("default claim does not validate: %v", err)
	}
	if (Link{}).OrDefault() != l || !(Link{}).IsZero() || l.IsZero() {
		t.Fatal("zero Link does not resolve to the default")
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want string // error substring; empty when valid
	}{
		{"100.127.255.252/30", ""},
		{" 100.126.255.252/30 ", ""},
		{"192.0.2.8/30", ""},
		{"100.127.255.248/29", "must be a /30"},
		{"100.127.255.252/31", "must be a /30"},
		{"100.126.0.1/30", "did you mean 100.126.0.0/30"},
		{"fd00::/126", "not IPv4"},
		{"100.126.0.0", "not a CIDR"},
		{"0.0.0.0/30", "0.0.0.0/8"},
		{"127.0.0.0/30", "loopback"},
		{"169.254.1.0/30", "link-local"},
		{"224.0.0.0/30", "multicast"},
		{"240.0.0.0/30", "class E"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			l, err := Parse(tt.in)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if l.Relay != l.Net.Addr().Next() || l.Guest != l.Relay.Next() {
					t.Fatalf("hosts = %v %v", l.Relay, l.Guest)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestHostOverlap(t *testing.T) {
	l := Default()
	pp := func(s string) netip.Prefix { return netip.MustParsePrefix(s) }
	refuse, warn := l.HostOverlap([]netip.Prefix{
		pp("192.168.1.10/24"),    // elsewhere
		pp("100.127.255.254/24"), // an address inside the link
		pp("100.100.1.2/10"),     // a CGNAT network containing it
		pp("100.127.255.0/30"),   // a neighbouring /30
		pp("fe80::1/64"),         // IPv6 ignored
		pp("::ffff:100.127.255.253/120"),
	})
	if len(refuse) != 2 || refuse[0] != pp("100.127.255.254/24") ||
		refuse[1].Addr().String() != "100.127.255.253" {
		t.Errorf("refuse = %v", refuse)
	}
	if len(warn) != 1 || warn[0] != pp("100.100.1.2/10") {
		t.Errorf("warn = %v", warn)
	}
	if r, w := l.HostOverlap([]netip.Prefix{pp("10.0.0.2/16")}); r != nil || w != nil {
		t.Errorf("clean host: refuse %v warn %v", r, w)
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		cidr                   string
		claim                  relaycfg.BuildLink
		boot, guest, relay, mk string
	}{
		{DefaultCIDR,
			relaycfg.BuildLink{Prefix: DefaultCIDR, Relay: "100.127.255.253",
				Guest: "100.127.255.254"},
			"warden.link_ip=100.127.255.254/30 warden.link_gw=100.127.255.253",
			"100.127.255.254/30", "100.127.255.253/30", "255.255.255.252"},
		{"100.126.255.252/30",
			relaycfg.BuildLink{Prefix: "100.126.255.252/30", Relay: "100.126.255.253",
				Guest: "100.126.255.254"},
			"warden.link_ip=100.126.255.254/30 warden.link_gw=100.126.255.253",
			"100.126.255.254/30", "100.126.255.253/30", "255.255.255.252"},
	}
	for _, tt := range tests {
		l, err := Parse(tt.cidr)
		if err != nil {
			t.Fatal(err)
		}
		if got := *l.Claim(); got != tt.claim {
			t.Errorf("%s Claim = %+v", tt.cidr, got)
		}
		if got := l.BootArgs(); got != tt.boot {
			t.Errorf("%s BootArgs = %q", tt.cidr, got)
		}
		if l.GuestCIDR() != tt.guest || l.RelayCIDR() != tt.relay || l.Mask() != tt.mk {
			t.Errorf("%s = %s %s %s", tt.cidr, l.GuestCIDR(), l.RelayCIDR(), l.Mask())
		}
		if l.String() != tt.cidr {
			t.Errorf("String = %s", l)
		}
	}
}

func TestStamp(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name, path, state string
		missing           bool
	}{
		{"missing", filepath.Join(dir, "none"), "missing", true},
		{"malformed", write("bad", "WARDEN_LINK_CHANNEL=one\n"), "stale", false},
		{"no key", write("nokey", "# empty\n"), "stale", false},
		{"different", write("old", "WARDEN_LINK_CHANNEL=0\n"), "stale", false},
		{"equal", write("ok", "# written by build-initramfs.sh\nWARDEN_LINK_CHANNEL=1\n"),
			"current", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckStamp(tt.path)
			if errors.Is(err, ErrNoStamp) != tt.missing {
				t.Errorf("CheckStamp = %v, missing %v", err, tt.missing)
			}
			if (err == nil) != (tt.state == "current") {
				t.Errorf("CheckStamp = %v", err)
			}
			if got := StampState(tt.path); got != tt.state {
				t.Errorf("StampState = %q, want %q", got, tt.state)
			}
		})
	}
	if n, err := ReadStamp(filepath.Join(dir, "ok")); n != Channel || err != nil {
		t.Errorf("ReadStamp = %d, %v", n, err)
	}
}

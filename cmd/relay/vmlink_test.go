package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/relaycfg"
)

// events records, in order, what the stubs below were asked to do.
type events struct {
	mu  sync.Mutex
	got []string
}

func (e *events) add(s string) {
	e.mu.Lock()
	e.got = append(e.got, s)
	e.mu.Unlock()
}

func (e *events) list() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.got...)
}

// stubIface is a buildIface over plain slices.
type stubIface struct {
	ev             *events
	addrs          []netip.Prefix
	others, routes []netip.Prefix
}

func (s *stubIface) Name() string                   { return "eth0" }
func (s *stubIface) Addrs() ([]netip.Prefix, error) { return s.addrs, nil }
func (s *stubIface) Flush() error {
	s.ev.add("flush")
	s.addrs = nil
	return nil
}

func (s *stubIface) Add(p netip.Prefix) error {
	s.ev.add("add " + p.String())
	s.addrs = append(s.addrs, p)
	return nil
}

func (s *stubIface) Others() ([]netip.Prefix, []netip.Prefix, error) {
	return s.others, s.routes, nil
}

var (
	pfx         = netip.MustParsePrefix
	defaultLink = &relaycfg.BuildLink{
		Prefix: "100.127.255.252/30", Relay: "100.127.255.253", Guest: "100.127.255.254",
	}
	otherLink = &relaycfg.BuildLink{
		Prefix: "100.126.255.252/30", Relay: "100.126.255.253", Guest: "100.126.255.254",
	}
)

// stubDHCP records the responder's start and stop against ev.
func stubDHCP(ev *events, leases *[]dhcpLease) func(string, dhcpLease) (func(), error) {
	return func(name string, l dhcpLease) (func(), error) {
		ev.add("dhcp " + name)
		*leases = append(*leases, l)
		return func() { ev.add("dhcp stop") }, nil
	}
}

func TestVMLinkStart(t *testing.T) {
	oldInit := []netip.Prefix{pfx("10.0.0.1/30")}
	upstream := []netip.Prefix{pfx("10.0.2.15/24")}
	defRoute := []netip.Prefix{pfx("0.0.0.0/0"), pfx("10.0.2.0/24")}
	tests := []struct {
		name           string
		link           *relaycfg.BuildLink
		addrs          []netip.Prefix
		others, routes []netip.Prefix
		wantSelf       string
		wantBlock      string
		wantEvents     []string
		refused        string
	}{
		{"link on an unaddressed interface", defaultLink, nil, upstream, defRoute,
			"100.127.255.253", "100.127.255.252/30",
			[]string{"flush", "add 100.127.255.253/30", "dhcp eth0"}, ""},
		{"old init's address flushed", otherLink, oldInit, upstream, defRoute,
			"100.126.255.253", "100.126.255.252/30",
			[]string{"flush", "add 100.126.255.253/30", "dhcp eth0"}, ""},
		{"a CGNAT route containing the link is fine", defaultLink, nil,
			[]netip.Prefix{pfx("100.100.0.5/10")},
			[]netip.Prefix{pfx("100.64.0.0/10")},
			"100.127.255.253", "100.127.255.252/30",
			[]string{"flush", "add 100.127.255.253/30", "dhcp eth0"}, ""},
		{"no link keeps the init's address", nil, oldInit, upstream, defRoute,
			"10.0.0.1", "10.0.0.0/30", nil, ""},
		{"no link, no address", nil, nil, upstream, defRoute, "", "", nil,
			"build_link is required: the build interface eth0 has no address"},
		{"an upstream address inside the link", defaultLink, nil,
			[]netip.Prefix{pfx("100.127.255.254/24")}, nil, "", "", nil,
			"holds 100.127.255.254/24"},
		{"an upstream route equal to the link", defaultLink, nil, upstream,
			[]netip.Prefix{pfx("100.127.255.252/30")}, "", "", nil,
			"overlaps the route 100.127.255.252/30"},
		{"an upstream route inside the link", defaultLink, nil, upstream,
			[]netip.Prefix{pfx("100.127.255.254/32")}, "", "", nil,
			"overlaps the route 100.127.255.254/32"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &events{}
			var leases []dhcpLease
			iface := &stubIface{ev: ev, addrs: tt.addrs, others: tt.others,
				routes: tt.routes}
			v := vmLink{iface: iface, serveDHCP: stubDHCP(ev, &leases)}
			cfg, stop, err := v.start(relaycfg.Config{BuildLink: tt.link})
			if tt.refused != "" {
				if !errors.Is(err, relaycfg.ErrRefused) ||
					!strings.Contains(err.Error(), tt.refused) {
					t.Fatalf("err = %v, want ErrRefused containing %q", err,
						tt.refused)
				}
				if got := ev.list(); len(got) != 0 {
					t.Fatalf("a refused claim changed the interface: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if cfg.SelfIP.String() != tt.wantSelf ||
				cfg.BuildLink.String() != tt.wantBlock {
				t.Fatalf("SelfIP %s BuildLink %s", cfg.SelfIP, cfg.BuildLink)
			}
			got, want := strings.Join(ev.list(), ","), strings.Join(tt.wantEvents, ",")
			if got != want {
				t.Fatalf("events = %v, want %v", got, want)
			}
			checkResponder(t, tt.link, stop, leases, ev)
		})
	}
}

// checkResponder checks the vm step started the responder with the claim's
// lease, and that its cleanup stops it; or that it started none without a
// claim link.
func checkResponder(t *testing.T, link *relaycfg.BuildLink, stop func(),
	leases []dhcpLease, ev *events) {
	t.Helper()
	if link == nil {
		if stop != nil || len(leases) != 0 {
			t.Fatal("a responder started without a claim link")
		}
		return
	}
	want := newDHCPLease(pfx(link.Prefix),
		netip.MustParseAddr(link.Relay), netip.MustParseAddr(link.Guest))
	if len(leases) != 1 || leases[0] != want {
		t.Fatalf("leases = %+v, want %+v", leases, want)
	}
	stop()
	if got := ev.list(); got[len(got)-1] != "dhcp stop" {
		t.Fatalf("cleanup did not stop the responder: %v", got)
	}
}

func TestVMLinkDHCPErrorFailsStart(t *testing.T) {
	ev := &events{}
	v := vmLink{iface: &stubIface{ev: ev}, serveDHCP: func(string, dhcpLease) (func(), error) {
		return nil, errors.New("bind: address in use")
	}}
	_, _, err := v.start(relaycfg.Config{BuildLink: defaultLink})
	if err == nil || errors.Is(err, relaycfg.ErrRefused) {
		t.Fatalf("err = %v, want a plain start error", err)
	}
}

// serveVMClaims runs relaycfg.Serve with the vm step as its start callback,
// recording "step done" when the step returns.
func serveVMClaims(t *testing.T, v vmLink, ev *events) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = relaycfg.Serve(ln, "", func(c relaycfg.Config,
			_ http.RoundTripper) (relaycfg.Identity, error) {
			_, _, err := v.start(c)
			ev.add("step done")
			if err != nil {
				return relaycfg.Identity{}, err
			}
			return relaycfg.Identity{CACert: "PEM"}, nil
		})
	}()
	return ln.Addr().String()
}

// TestVMLinkRefusalLeavesRelayClaimable checks a refused claim does not
// consume the relay, and that the responder starts after the address is added
// and before the claim is answered.
func TestVMLinkRefusalLeavesRelayClaimable(t *testing.T) {
	ev := &events{}
	var leases []dhcpLease
	iface := &stubIface{ev: ev, routes: []netip.Prefix{pfx("100.127.255.252/30")}}
	addr := serveVMClaims(t, vmLink{iface: iface, serveDHCP: stubDHCP(ev, &leases)}, ev)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := relaycfg.Claim(ctx, addr, "",
		relaycfg.Config{CollectorURL: "http://x", BuildLink: defaultLink})
	if !errors.Is(err, relaycfg.ErrRejected) || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("overlapping link: err = %v, want a 400", err)
	}
	if _, err := relaycfg.Claim(ctx, addr, "",
		relaycfg.Config{CollectorURL: "http://x", BuildLink: otherLink}); err != nil {
		t.Fatalf("valid claim after a refused one: %v", err)
	}
	ev.add("reply")
	want := "step done,flush,add 100.126.255.253/30,dhcp eth0,step done,reply"
	if got := strings.Join(ev.list(), ","); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

// TestVMLinkUnaddressedRefusal is the new relay, old orchestrator row on a
// new init: no build_link and nothing on the interface is a 400, not a crash.
func TestVMLinkUnaddressedRefusal(t *testing.T) {
	ev := &events{}
	var leases []dhcpLease
	iface := &stubIface{ev: ev}
	addr := serveVMClaims(t, vmLink{iface: iface, serveDHCP: stubDHCP(ev, &leases)}, ev)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := relaycfg.Claim(ctx, addr, "", relaycfg.Config{CollectorURL: "http://x"})
	if !errors.Is(err, relaycfg.ErrRejected) ||
		!strings.Contains(err.Error(), "build_link is required") {
		t.Fatalf("err = %v, want the build_link is required refusal", err)
	}
}

func TestContainerRefusesBuildLink(t *testing.T) {
	if _, _, err := containerStart(relaycfg.Config{BuildLink: defaultLink}); !errors.Is(
		err, relaycfg.ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	cfg, stop, err := containerStart(relaycfg.Config{})
	if err != nil || stop != nil || cfg.BuildLink.IsValid() || cfg.SelfIP != nil {
		t.Fatalf("container without a link: %+v %v", cfg, err)
	}
}

func TestPickBuildIf(t *testing.T) {
	names := []string{"eth0", "eth1", "lo"}
	if got, _ := pickBuildIf([]string{"lo", "eth0", "eth1"}, ""); got != "eth0" {
		t.Errorf("default pick = %q", got)
	}
	if got, _ := pickBuildIf(names, "eth1"); got != "eth1" {
		t.Errorf("named pick = %q", got)
	}
	if _, err := pickBuildIf(names, "eth9"); err == nil {
		t.Error("a missing -build-if was accepted")
	}
	if _, err := pickBuildIf([]string{"lo"}, ""); err == nil {
		t.Error("lo alone was accepted")
	}
}

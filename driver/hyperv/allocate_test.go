package hyperv

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func TestRelayNotesRoundTrip(t *testing.T) {
	ip := netip.MustParseAddr("10.200.0.7")
	gotIP, gotSw := parseRelayNote(relayNotes(ip, "warden-dev-3"))
	if gotIP != ip || gotSw != "warden-dev-3" {
		t.Errorf("parsed %v %q", gotIP, gotSw)
	}
	if ip, sw := parseRelayNote("edited by hand warden-relay-ip=bogus"); ip.IsValid() ||
		sw != "" {
		t.Errorf("malformed notes parsed as %v %q", ip, sw)
	}
}

// TestAllocRelayIP: the lowest host of the prefix that is not the network,
// broadcast, host IP or recorded in a relay VM's Notes.
func TestAllocRelayIP(t *testing.T) {
	pfx := netip.MustParsePrefix("192.168.240.0/20")
	host := netip.MustParseAddr("192.168.240.1")
	tests := []struct {
		name  string
		notes map[string]string
		want  string
	}{
		{"empty host", nil, "192.168.240.2"},
		{"recorded skipped", map[string]string{
			"warden-relay-a": relayNotes(netip.MustParseAddr("192.168.240.2"), "w-a"),
			"warden-relay-b": relayNotes(netip.MustParseAddr("192.168.240.3"), "w-b"),
		}, "192.168.240.4"},
		{"a gap is reused", map[string]string{
			"warden-relay-b": relayNotes(netip.MustParseAddr("192.168.240.3"), "w-b"),
		}, "192.168.240.2"},
		{"outside the prefix ignored", map[string]string{
			"warden-relay-x": relayNotes(netip.MustParseAddr("172.31.0.2"), "w-x"),
		}, "192.168.240.2"},
	}
	for _, tt := range tests {
		got, err := allocRelayIP(pfx, host, tt.notes)
		if err != nil || got != netip.MustParseAddr(tt.want) {
			t.Errorf("%s: got %v, %v; want %s", tt.name, got, err, tt.want)
		}
	}
	// The host not at .1: the network address is never taken.
	got, err := allocRelayIP(netip.MustParsePrefix("10.200.0.0/24"),
		netip.MustParseAddr("10.200.0.9"), nil)
	if err != nil || got != netip.MustParseAddr("10.200.0.1") {
		t.Errorf("host at .9: got %v, %v", got, err)
	}
}

// TestAllocRelayIPFull: a /30 holds two hosts, the host IP and one relay; a
// second relay is refused naming the first's VM and `warden clean`, and the
// broadcast address is never handed out.
func TestAllocRelayIPFull(t *testing.T) {
	pfx := netip.MustParsePrefix("10.200.0.0/30")
	host := netip.MustParseAddr("10.200.0.1")
	got, err := allocRelayIP(pfx, host, nil)
	if err != nil || got != netip.MustParseAddr("10.200.0.2") {
		t.Fatalf("got %v, %v", got, err)
	}
	notes := map[string]string{"warden-relay-held": relayNotes(got, "warden-held")}
	_, err = allocRelayIP(pfx, host, notes)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "warden-relay-held (10.200.0.2)") ||
		!strings.Contains(fe.Fix, "warden clean --driver hyperv") ||
		!strings.Contains(fe.Fix, "--nat-prefix") {
		t.Fatalf("err = %v", err)
	}
}

// TestAllocSlot: the lowest slot with no adapter attached; a build VM alone
// marks a slot busy; all busy is refused naming each slot's holders,
// `warden clean` and --slots.
func TestAllocSlot(t *testing.T) {
	slots := []string{"warden-dev-0", "warden-dev-1", "warden-dev-2"}
	got, err := allocSlot(slots, map[string][]string{
		"warden-dev-0": {"warden-relay-a", "warden-build-a"},
		"warden-dev-1": {"warden-build-kept"},
	})
	if err != nil || got != "warden-dev-2" {
		t.Errorf("got %q, %v; want warden-dev-2", got, err)
	}
	if got, err := allocSlot(slots, nil); err != nil || got != "warden-dev-0" {
		t.Errorf("free host: got %q, %v", got, err)
	}
	_, err = allocSlot(slots[:2], map[string][]string{
		"warden-dev-0": {"warden-relay-a", "warden-build-a"},
		"warden-dev-1": {"warden-build-kept"},
	})
	var fe *driver.FixableError
	if !errors.As(err, &fe) ||
		!strings.Contains(fe.Msg, "warden-dev-0 (warden-relay-a, warden-build-a)") ||
		!strings.Contains(fe.Msg, "warden-dev-1 (warden-build-kept)") ||
		!strings.Contains(fe.Fix, "--slots <more>") ||
		!strings.Contains(fe.Fix, "warden clean --driver hyperv") ||
		!strings.Contains(fe.Fix, "elevated") {
		t.Fatalf("err = %v", err)
	}
}

// TestAllocateRelay: durable builds read the slots' adapters; ephemeral ones
// keep their own switch and read none.
func TestAllocateRelay(t *testing.T) {
	ctx := context.Background()
	pfx := netip.MustParsePrefix("192.168.240.0/20")
	host := netip.MustParseAddr("192.168.240.1")
	fp := &buildFake{attached: map[string][]string{"warden-dev-0": {"warden-relay-z"}},
		notes: map[string]string{"warden-relay-z": relayNotes(
			netip.MustParseAddr("192.168.240.2"), "warden-dev-0")}}
	got, err := allocateRelay(ctx, fp, pfx, host, []string{"warden-dev-0", "warden-dev-1"}, "")
	if err != nil || got.IP != netip.MustParseAddr("192.168.240.3") ||
		got.BuildSwitch != "warden-dev-1" {
		t.Errorf("durable: %+v, %v", got, err)
	}
	fp.adapterCalls.Store(0)
	got, err = allocateRelay(ctx, fp, pfx, host, nil, "warden-abc")
	if err != nil || got.BuildSwitch != "warden-abc" || fp.adapterCalls.Load() != 0 {
		t.Errorf("ephemeral: %+v, %v (adapter reads %d)", got, err, fp.adapterCalls.Load())
	}
	if _, err := allocateRelay(ctx, fp, pfx, host, nil, ""); err == nil {
		t.Error("no switch and no slots allocated")
	}
}

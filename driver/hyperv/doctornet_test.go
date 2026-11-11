package hyperv

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

var durableCaps = Capabilities{Supported: true, HypervisorPresent: true, HyperVFeature: true,
	HyperVAdmin: true, DevSwitch: "warden-dev", DevSwitchPresent: true}

// TestWriteBuildNetwork: the NAT read back, the resolvers and their source,
// and each slot free or held (a relay VM named with its recorded address).
func TestWriteBuildNetwork(t *testing.T) {
	dn := healthyNet(durableCaps)
	dn.Attached = map[string][]string{"warden-dev-1": {"warden-relay-a", "warden-build-a"}}
	dn.Notes = map[string]string{"warden-relay-a": relayNotes(
		netip.MustParseAddr("192.168.240.2"), "warden-dev-1")}
	var buf bytes.Buffer
	if blocked := writeBuildNetwork(&buf, durableCaps, dn, buildlink.Link{}, nil); blocked {
		t.Fatalf("blocked:\n%s", buf.String())
	}
	for _, want := range []string{"warden-dev-nat (192.168.240.0/20, host 192.168.240.1)",
		"10.9.0.53 (from interface Ethernet)", "Slot warden-dev-0 ", "free",
		"in use by warden-relay-a (192.168.240.2), warden-build-a"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}
}

// TestWriteBuildNetworkBlocked: an overlap with the link, an unusable [vm]
// dns entry and a durable network with no slots each block, with their fix.
func TestWriteBuildNetworkBlocked(t *testing.T) {
	link, _ := buildlink.Parse("192.168.240.4/30")
	for name, tc := range map[string]struct {
		caps  Capabilities
		dn    doctorNet
		link  buildlink.Link
		dns   []netip.Addr
		wants []string
	}{
		"overlap": {durableCaps, healthyNet(durableCaps), link, nil,
			[]string{"NAT and build link", "BLOCKED", "--nat-prefix"}},
		"vm dns": {durableCaps, healthyNet(durableCaps), buildlink.Link{},
			addrs("127.0.0.1"), []string{"Relay resolvers", "BLOCKED", "loopback"}},
		"no slots": {durableCaps, doctorNet{ReadBack: parseNetReadback(
			"NAT=warden-dev-nat\nPREFIX=192.168.240.0/20\nHOSTIP=192.168.240.1\n")},
			buildlink.Link{}, nil, []string{"has no build switches",
				"to create the build switches"}},
	} {
		var buf bytes.Buffer
		if !writeBuildNetwork(&buf, tc.caps, tc.dn, tc.link, tc.dns) {
			t.Errorf("%s: not blocked:\n%s", name, buf.String())
		}
		for _, w := range tc.wants {
			if !strings.Contains(buf.String(), w) {
				t.Errorf("%s: report lacks %q:\n%s", name, w, buf.String())
			}
		}
	}
}

// TestWriteBuildNetworkEphemeral: an elevated build with no NAT yet reports
// the default it would make and checks against it.
func TestWriteBuildNetworkEphemeral(t *testing.T) {
	elevated := durableCaps
	elevated.Elevated = true
	dn := doctorNet{ReadBack: parseNetReadback(""), DNS: healthyNet(elevated).DNS,
		Host: []netip.Addr{}}
	var buf bytes.Buffer
	if writeBuildNetwork(&buf, elevated, dn, buildlink.Link{}, nil) {
		t.Fatalf("blocked:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "none yet (a build makes one on 192.168.240.0/20") {
		t.Errorf("report:\n%s", buf.String())
	}
}

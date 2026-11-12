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

// TestCheckLinkAhead: before any per-build work, the link is checked against
// the NAT the build would use: the durable network's, the one EnsureNetwork
// would take, or the default prefix, named, when the host has none.
func TestCheckLinkAhead(t *testing.T) {
	const host = "NAT=warden-dev-nat\nPREFIX=10.200.0.0/24\nHOSTIP=10.200.0.1\n"
	inDefault := mustLink(t, "192.168.240.4/30")
	inHost := mustLink(t, "10.200.0.4/30")
	for name, tc := range map[string]struct {
		out     string
		durable bool
		link    buildlink.Link
		want    string // "" accepts
	}{
		"ephemeral, no NAT, link in the default": {"", false, inDefault,
			"the Hyper-V default NAT prefix " + DefaultNATPrefix},
		"ephemeral, no NAT, link elsewhere": {"", false, inHost, ""},
		"ephemeral, host NAT, link in it": {host, false, inHost,
			`NAT "warden-dev-nat" on this host (10.200.0.0/24)`},
		"ephemeral, host NAT, link in the default": {host, false, inDefault, ""},
		"durable, link in its NAT": {host + "SLOT=warden-dev-0\tPrivate\n", true,
			inHost, `NAT "warden-dev-nat" on this host`},
		"durable, no slots": {host, true, inDefault, "has no build switches"},
	} {
		err := checkLinkAhead(parseNetReadback(tc.out), "warden-dev", tc.durable, tc.link)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: refused: %v", name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

// TestWriteBuildNetworkStrayHostIP (native F5): an address an earlier setup
// left on the NAT's vNIC is named, with the command that removes it.
func TestWriteBuildNetworkStrayHostIP(t *testing.T) {
	elevated := durableCaps
	elevated.Elevated = true
	dn := doctorNet{ReadBack: parseNetReadback("NAT=warden-dev-nat\n" +
		"PREFIX=192.168.250.0/24\nHOSTIP=192.168.240.1\nHOSTIP=192.168.250.1\n"),
		DNS: healthyNet(elevated).DNS, Host: []netip.Addr{}}
	var buf bytes.Buffer
	if writeBuildNetwork(&buf, elevated, dn, buildlink.Link{}, nil) {
		t.Fatalf("blocked:\n%s", buf.String())
	}
	for _, want := range []string{"warden-dev-nat (192.168.250.0/24, host 192.168.250.1)",
		"Other NAT vNIC IPs", "192.168.240.1 (warning",
		"Remove-NetIPAddress -InterfaceAlias 'vEthernet (warden-dev-nat)' " +
			"-IPAddress 192.168.240.1 -Confirm:$false"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}
}

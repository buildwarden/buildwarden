package hyperv

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

// TestCheckNATOverlap: a NAT prefix overlapping the build link is refused
// before any VM, naming both fixes.
func TestCheckNATOverlap(t *testing.T) {
	nat := netip.MustParsePrefix("100.127.0.0/16")
	err := checkNATOverlap(nat, buildlink.Default())
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "[vm] build_link") ||
		!strings.Contains(fe.Fix, "--nat-prefix") ||
		!strings.Contains(fe.Msg, "100.127.0.0/16") {
		t.Fatalf("err = %v", err)
	}
	if err := checkNATOverlap(netip.MustParsePrefix(DefaultNATPrefix),
		buildlink.Default()); err != nil {
		t.Errorf("the defaults overlap: %v", err)
	}
}

// hostDNS is a host source returning list from "interface Ethernet".
func hostDNS(list ...string) hostDNSSource {
	return func() ([]netip.Addr, string, error) {
		return addrs(list...), "interface Ethernet (the IPv4 default route's)", nil
	}
}

// TestResolverRule: from the host's list Hyper-V's relay drops loopback,
// link-local, the host's own addresses, the NAT prefix and the build link,
// keeping the rest in order.
func TestResolverRule(t *testing.T) {
	nat := netip.MustParsePrefix(DefaultNATPrefix)
	link := buildlink.Default()
	list, source, err := relayResolvers(hostDNS("127.0.0.1", "10.9.0.53", "169.254.1.1",
		"10.9.0.7", "192.168.240.1", "100.127.255.253", "10.9.0.54"),
		addrs("10.9.0.7"), nat, link, nil)
	if err != nil || resolvers.Join(list) != "10.9.0.53, 10.9.0.54" ||
		!strings.Contains(source, "Ethernet") {
		t.Fatalf("got %v from %q, %v", list, source, err)
	}
	_, _, err = relayResolvers(hostDNS("127.0.0.1", "10.9.0.7", "100.127.255.253"),
		addrs("10.9.0.7"), nat, link, nil)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "127.0.0.1 (loopback)") ||
		!strings.Contains(fe.Msg, "10.9.0.7 (assigned to this host)") ||
		!strings.Contains(fe.Fix, "[vm] dns") ||
		!strings.Contains(fe.Fix, "[vm] build_link") ||
		!strings.Contains(fe.Fix, "Ethernet") {
		t.Fatalf("none usable: %v", err)
	}
}

// TestVMDNSUsedExclusively: [vm] dns, checked by the same rule, is the list
// and the host source is never read; an unusable entry is refused, not
// dropped.
func TestVMDNSUsedExclusively(t *testing.T) {
	nat := netip.MustParsePrefix(DefaultNATPrefix)
	never := func() ([]netip.Addr, string, error) {
		t.Fatal("the host source was read with [vm] dns set")
		return nil, "", nil
	}
	list, source, err := relayResolvers(never, nil, nat, buildlink.Default(),
		addrs("10.20.0.54", "10.20.0.53"))
	if err != nil || resolvers.Join(list) != "10.20.0.54, 10.20.0.53" ||
		source != resolvers.VMDNS {
		t.Fatalf("got %v from %q, %v", list, source, err)
	}
	for _, bad := range []string{"127.0.0.53", "169.254.0.1", "192.168.240.9",
		"100.127.255.253"} {
		_, _, err := relayResolvers(never, nil, nat, buildlink.Default(),
			addrs("10.20.0.53", bad))
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, bad) {
			t.Errorf("%s: err = %v", bad, err)
		}
	}
	req := &driver.BuildRequest{DNS: addrs("10.20.0.53")}
	if err := prepareBuildNetwork(req, nat, never); err != nil ||
		resolvers.Join(req.DNS) != "10.20.0.53" {
		t.Errorf("prepareBuildNetwork: %v, DNS %v", err, req.DNS)
	}
}

// TestPrepareBuildNetwork: the overlap refusal comes first, and the host's
// resolvers land in req.DNS for the claim.
func TestPrepareBuildNetwork(t *testing.T) {
	req := &driver.BuildRequest{}
	if err := prepareBuildNetwork(req, netip.MustParsePrefix("100.127.0.0/16"),
		hostDNS("10.9.0.53")); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("overlap: %v", err)
	}
	if err := prepareBuildNetwork(req, netip.MustParsePrefix(DefaultNATPrefix),
		hostDNS("203.0.113.53")); err != nil || resolvers.Join(req.DNS) != "203.0.113.53" {
		t.Errorf("host's: %v, DNS %v", err, req.DNS)
	}
}

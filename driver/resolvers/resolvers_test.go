package resolvers

import (
	"net/netip"
	"testing"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

// TestSelect is Firecracker's selection case, moved with the selection: a
// loopback resolver and one assigned to the host are excluded, the rest kept
// in order.
func TestSelect(t *testing.T) {
	local := func(a netip.Addr) string {
		if a == netip.MustParseAddr("192.168.1.10") {
			return "assigned to this host"
		}
		return ""
	}
	use, excluded := Select(addrs("127.0.0.53", "192.168.1.10", "192.168.1.1", "8.8.8.8"),
		Rules(Loopback, local, BuildLink(buildlink.Default())))
	if Join(use) != "192.168.1.1, 8.8.8.8" ||
		JoinExcluded(excluded) != "127.0.0.53 (loopback), 192.168.1.10 (assigned to "+
			"this host)" {
		t.Fatalf("use %v excluded %v", use, excluded)
	}
}

func TestParseResolvConfIPv4Only(t *testing.T) {
	got := ParseResolvConf("# c\nnameserver 1.1.1.1\nnameserver 2606:4700::1111\nsearch " +
		"x\nnameserver 9.9.9.9 \n")
	if Join(got) != "1.1.1.1, 9.9.9.9" {
		t.Fatalf("nameservers = %v", got)
	}
}

package vz

import (
	"errors"
	"net/netip"
	"os"
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

func resolvConf(conf string) resolvers.ReadFile {
	return func(p string) ([]byte, error) {
		if p == resolvers.ResolvConf {
			return []byte(conf), nil
		}
		return nil, os.ErrNotExist
	}
}

// TestResolverExclusions: the host relay drops only the build link from the
// Mac's list; loopback and link-local stay, since the relay runs on the Mac.
func TestResolverExclusions(t *testing.T) {
	link, _ := buildlink.Parse("10.1.2.0/30")
	got, source, err := relayResolvers(resolvConf("# macOS Notice\nsearch lan\n"+
		"nameserver 127.0.0.1\nnameserver 10.1.2.2\nnameserver 169.254.1.1\n"+
		"nameserver 10.0.2.3\n"), link, nil)
	if err != nil || resolvers.Join(got) != "127.0.0.1, 169.254.1.1, 10.0.2.3" ||
		source != resolvers.ResolvConf {
		t.Fatalf("%v %q %v", got, source, err)
	}
	_, _, err = relayResolvers(resolvConf("nameserver 10.1.2.2\n"), link, nil)
	var fe *driver.FixableError
	if !errors.As(err, &fe) ||
		!strings.Contains(err.Error(), "10.1.2.2 (inside the build link") ||
		!strings.Contains(fe.Fix, "[vm] build_link") ||
		!strings.Contains(fe.Fix, "[vm] dns") {
		t.Fatalf("in-link only: %v", err)
	}
	_, _, err = relayResolvers(resolvConf("# macOS Notice\n"), link, nil)
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "[vm] dns") {
		t.Fatalf("none listed: %v", err)
	}
}

// TestVMDNS: [vm] dns is checked by the same rule (a loopback entry is
// accepted), never filtered, and placed in req.DNS with no host file read.
func TestVMDNS(t *testing.T) {
	noRead := func(p string) ([]byte, error) {
		t.Fatalf("read %s with [vm] dns set", p)
		return nil, nil
	}
	req := &driver.BuildRequest{DNS: addrs("127.0.0.1", "10.20.0.53")}
	if err := setRelayDNS(req, noRead); err != nil ||
		resolvers.Join(req.DNS) != "127.0.0.1, 10.20.0.53" {
		t.Fatalf("%v %v", req.DNS, err)
	}
	req = &driver.BuildRequest{DNS: addrs("10.20.0.53", "100.127.255.253")}
	err := setRelayDNS(req, noRead)
	var fe *driver.FixableError
	if !errors.As(err, &fe) ||
		!strings.Contains(err.Error(), "[vm] dns lists 100.127.255.253") ||
		!strings.Contains(fe.Fix, "[vm] build_link") || len(req.DNS) != 2 {
		t.Fatalf("in-link entry: %v %v", req.DNS, err)
	}
}

// TestHostResolversPlacedInRequest: unset [vm] dns, the Mac's resolvers
// become req.DNS.
func TestHostResolversPlacedInRequest(t *testing.T) {
	req := &driver.BuildRequest{}
	if err := setRelayDNS(req, resolvConf("nameserver 192.168.1.1\n")); err != nil ||
		resolvers.Join(req.DNS) != "192.168.1.1" {
		t.Fatalf("%v %v", req.DNS, err)
	}
}

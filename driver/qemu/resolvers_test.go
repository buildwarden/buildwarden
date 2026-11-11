package qemu

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

// TestResolverExclusions: from the host's list the qemu relay VM drops
// loopback and link-local (they would be its own), user-mode NAT's network
// and the build link, and keeps the rest in order.
func TestResolverExclusions(t *testing.T) {
	link, _ := buildlink.Parse("10.1.2.0/30")
	conf := "nameserver 127.0.0.1\nnameserver 169.254.169.254\nnameserver 10.0.2.3\n" +
		"nameserver 10.1.2.2\nnameserver 192.168.1.1\nnameserver 1.1.1.1\n"
	for _, goos := range []string{"darwin", "linux"} {
		got, source, err := relayResolvers(goos, resolvConf(conf), link, nil)
		if err != nil || resolvers.Join(got) != "192.168.1.1, 1.1.1.1" ||
			source != resolvers.ResolvConf {
			t.Errorf("%s: %v %q %v", goos, got, source, err)
		}
	}
	_, _, err := relayResolvers("darwin",
		resolvConf("nameserver 127.0.0.1\nnameserver 10.0.2.3\nnameserver 10.1.2.2\n"),
		link, nil)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "off-host DNS server") ||
		!strings.Contains(fe.Fix, "set [vm] dns") ||
		!strings.Contains(fe.Fix, "[vm] build_link") {
		t.Fatalf("none usable: %v", err)
	}
	for _, want := range []string{"127.0.0.1 (loopback)",
		"10.0.2.3 (inside qemu's user-mode network 10.0.2.0/24)",
		"10.1.2.2 (inside the build link 10.1.2.0/30)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal lacks %q: %v", want, err)
		}
	}
}

// TestVMDNS: [vm] dns is checked by the same rule, never filtered, and
// placed in req.DNS with no host file read, on any host.
func TestVMDNS(t *testing.T) {
	noRead := func(p string) ([]byte, error) {
		t.Fatalf("read %s with [vm] dns set", p)
		return nil, nil
	}
	req := &driver.BuildRequest{DNS: addrs("10.20.0.54", "10.20.0.53")}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if err := setRelayDNS(req, goos, noRead); err != nil ||
			resolvers.Join(req.DNS) != "10.20.0.54, 10.20.0.53" {
			t.Fatalf("%s: %v %v", goos, req.DNS, err)
		}
	}
	for _, bad := range []string{"127.0.0.1", "169.254.0.1", "10.0.2.3",
		"100.127.255.253"} {
		req := &driver.BuildRequest{DNS: addrs("10.20.0.53", bad)}
		err := setRelayDNS(req, "darwin", noRead)
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(err.Error(), "[vm] dns lists "+bad) ||
			!strings.Contains(fe.Fix, "[vm] dns") || len(req.DNS) != 2 {
			t.Errorf("%s: %v %v", bad, req.DNS, err)
		}
	}
}

// TestHostResolversPlacedInRequest: unset [vm] dns, the host's usable
// resolvers become req.DNS.
func TestHostResolversPlacedInRequest(t *testing.T) {
	req := &driver.BuildRequest{}
	if err := setRelayDNS(req, "linux", resolvConf("nameserver 10.0.0.2\n")); err != nil ||
		resolvers.Join(req.DNS) != "10.0.0.2" {
		t.Fatalf("%v %v", req.DNS, err)
	}
	var fe *driver.FixableError
	if err := setRelayDNS(&driver.BuildRequest{}, "windows",
		resolvConf("")); !errors.As(err, &fe) || !strings.Contains(fe.Fix, "[vm] dns") {
		t.Fatalf("windows host: %v", err)
	}
}

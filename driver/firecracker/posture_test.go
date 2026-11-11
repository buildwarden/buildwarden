package firecracker

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// nftDropJSON is `nft -j list chains` on a host running both iptables-nft
// (whose FORWARD drop is the posture setup wants) and Debian's default
// nftables.conf, whose inet filter forward chain drops by default.
const nftDropJSON = `{"nftables":[{"metainfo":{"version":"1.0.6"}},
{"chain":{"family":"ip","table":"filter","name":"FORWARD",
"handle":1,"type":"filter","hook":"forward","prio":0,"policy":"drop"}},
{"chain":{"family":"bridge","table":"warden_fc","name":"forward",
"handle":2,"type":"filter","hook":"forward","prio":0,"policy":"accept"}},
{"chain":{"family":"inet","table":"filter","name":"forward",
"handle":3,"type":"filter","hook":"forward","prio":0,"policy":"drop"}},
{"chain":{"family":"inet","table":"filter","name":"input",
"handle":4,"type":"filter","hook":"input","prio":0,"policy":"drop"}}]}`

func TestNftForwardDrops(t *testing.T) {
	drops, err := nftForwardDrops([]byte(nftDropJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(drops) != 1 || drops[0].Family != "inet" || drops[0].Name != "forward" {
		t.Fatalf("drops = %+v, want only inet filter forward", drops)
	}
	fix := nftDropFix(drops)
	if !strings.Contains(fix,
		`nft insert rule inet filter forward oifname "wfc*u" ct state `+
			`established,related accept`) {
		t.Fatalf("fix = %q", fix)
	}
	if _, err := nftForwardDrops([]byte("not json")); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestForwardPolicyAccept(t *testing.T) {
	if !forwardPolicyAccept("-P FORWARD ACCEPT\n-A FORWARD -j DOCKER-USER\n") {
		t.Fatal("ACCEPT policy not seen")
	}
	if forwardPolicyAccept("-P FORWARD DROP\n-A FORWARD -j ACCEPT\n") {
		t.Fatal("an ACCEPT rule read as the policy")
	}
}

func TestRelayResolvers(t *testing.T) {
	none := func(net.IP) bool { return false }
	def := buildlink.Default()
	files := func(m map[string]string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if s, ok := m[p]; ok {
				return []byte(s), nil
			}
			return nil, os.ErrNotExist
		}
	}
	// systemd-resolved's stub is followed to its upstream list.
	got, _, err := relayResolvers(files(map[string]string{
		resolvers.ResolvConf:       "nameserver 127.0.0.53\noptions edns0\n",
		resolvers.ResolvedUpstream: "nameserver 10.1.2.3\n",
	}), none, def, nil)
	if err != nil || resolvers.Join(got) != "10.1.2.3" {
		t.Fatalf("resolved: %v %v", got, err)
	}
	// WSL's DNS-tunnelling resolver is on the host itself: refused, and the
	// fix says how to change it.
	wsl := func(ip net.IP) bool { return ip.Equal(net.ParseIP(wslTunnelResolver)) }
	_, _, err = relayResolvers(files(map[string]string{
		resolvers.ResolvConf: "nameserver 10.255.255.254\n",
	}), wsl, def, nil)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "generateResolvConf=false") ||
		!strings.Contains(fe.Fix, "set [vm] dns") ||
		!strings.Contains(err.Error(), "10.255.255.254") {
		t.Fatalf("wsl: %v", err)
	}
	// An AWS VPC resolver at 10.0.0.2 (the PR #90 failure) is outside the
	// default link and now used as is.
	got, _, err = relayResolvers(files(map[string]string{
		resolvers.ResolvConf:       "nameserver 127.0.0.53\n",
		resolvers.ResolvedUpstream: "nameserver 10.0.0.2\n",
	}), none, def, nil)
	if err != nil || resolvers.Join(got) != "10.0.0.2" {
		t.Fatalf("VPC resolver: %v %v", got, err)
	}
	// A local dnsmasq on a LAN address is refused too.
	_, _, err = relayResolvers(files(map[string]string{
		resolvers.ResolvConf: "nameserver 127.0.0.1\n",
	}), none, def, nil)
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "/etc/resolv.conf") ||
		!strings.Contains(fe.Fix, "set [vm] dns") {
		t.Fatalf("loopback: %v", err)
	}
}

// TestRelayResolversConfiguredLink: a resolver inside a configured link
// would be routed to the build VM, so it is excluded with a fix naming both
// remedies.
func TestRelayResolversConfiguredLink(t *testing.T) {
	none := func(net.IP) bool { return false }
	files := func(m map[string]string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if s, ok := m[p]; ok {
				return []byte(s), nil
			}
			return nil, os.ErrNotExist
		}
	}
	var fe *driver.FixableError
	conf, _ := buildlink.Parse("10.1.2.0/30")
	_, _, err := relayResolvers(files(map[string]string{
		resolvers.ResolvConf: "nameserver 10.1.2.2\n",
	}), none, conf, nil)
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "10.1.2.0/30") ||
		!strings.Contains(fe.Fix, "[vm] build_link") ||
		!strings.Contains(fe.Fix, "set [vm] dns") ||
		!strings.Contains(err.Error(), "10.1.2.2") {
		t.Fatalf("build link: %v", err)
	}
	got, _, err := relayResolvers(files(map[string]string{
		resolvers.ResolvConf: "nameserver 10.1.2.2\nnameserver 10.1.2.4\n",
	}), none, conf, nil)
	if err != nil || resolvers.Join(got) != "10.1.2.4" {
		t.Fatalf("build link mixed: %v %v", got, err)
	}
}

// TestRelayResolversVMDNS: a set [vm] dns is the list, in order, with no
// host file read; an entry the rule refuses refuses the build (it is never
// dropped), and the fix names [vm] dns.
func TestRelayResolversVMDNS(t *testing.T) {
	noRead := func(p string) ([]byte, error) {
		t.Fatalf("read %s with [vm] dns set", p)
		return nil, nil
	}
	local := func(ip net.IP) bool { return ip.Equal(net.ParseIP("192.168.1.10")) }
	conf, _ := buildlink.Parse("10.1.2.0/30")
	got, source, err := relayResolvers(noRead, local, conf,
		addrs("10.20.0.54", "10.20.0.53"))
	if err != nil || resolvers.Join(got) != "10.20.0.54, 10.20.0.53" ||
		source != resolvers.VMDNS {
		t.Fatalf("[vm] dns: %v %q %v", got, source, err)
	}
	for _, tt := range []struct{ bad, why, fix string }{
		{"127.0.0.1", "loopback", "[vm] dns"},
		{"192.168.1.10", "assigned to this host", "[vm] dns"},
		{"169.254.169.254", "link-local", "[vm] dns"},
		{"10.1.2.2", "inside the build link 10.1.2.0/30", "[vm] build_link"},
	} {
		got, _, err := relayResolvers(noRead, local, conf, addrs("10.20.0.53", tt.bad))
		var fe *driver.FixableError
		if got != nil || !errors.As(err, &fe) ||
			!strings.Contains(err.Error(), tt.bad) ||
			!strings.Contains(err.Error(), tt.why) ||
			!strings.Contains(fe.Fix, tt.fix) {
			t.Errorf("[vm] dns with %s: %v %v", tt.bad, got, err)
		}
	}
}

// TestRelayResolversLinkLocal: a link-local host resolver (a cloud metadata
// resolver) is not reachable from behind the slot's NAT, so it is excluded;
// with nothing else left the refusal names it and [vm] dns.
func TestRelayResolversLinkLocal(t *testing.T) {
	none := func(net.IP) bool { return false }
	files := func(conf string) resolvers.ReadFile {
		return func(p string) ([]byte, error) {
			if p == resolvers.ResolvConf {
				return []byte(conf), nil
			}
			return nil, os.ErrNotExist
		}
	}
	got, source, err := relayResolvers(
		files("nameserver 169.254.169.254\nnameserver 10.0.0.2\n"), none,
		buildlink.Default(), nil)
	if err != nil || resolvers.Join(got) != "10.0.0.2" || source != resolvers.ResolvConf {
		t.Fatalf("mixed: %v %q %v", got, source, err)
	}
	_, _, err = relayResolvers(files("nameserver 169.254.169.254\n"), none,
		buildlink.Default(), nil)
	var fe *driver.FixableError
	if !errors.As(err, &fe) ||
		!strings.Contains(err.Error(), "169.254.169.254 (link-local)") ||
		!strings.Contains(fe.Fix, "set [vm] dns") {
		t.Fatalf("link-local only: %v", err)
	}
}

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

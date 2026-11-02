package firecracker

import (
	"errors"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
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

func TestSelectResolvers(t *testing.T) {
	local := func(ip net.IP) bool { return ip.Equal(net.ParseIP("192.168.1.10")) }
	use, excluded := selectResolvers(
		[]string{"127.0.0.53", "192.168.1.10", "192.168.1.1", "8.8.8.8"}, local)
	if strings.Join(use, ",") != "192.168.1.1,8.8.8.8" ||
		strings.Join(excluded, ",") != "127.0.0.53,192.168.1.10" {
		t.Fatalf("use %v excluded %v", use, excluded)
	}
}

func TestParseNameserversIPv4Only(t *testing.T) {
	got := parseNameservers("# c\nnameserver 1.1.1.1\nnameserver 2606:4700::1111\nsearch " +
		"x\nnameserver 9.9.9.9 \n")
	if strings.Join(got, ",") != "1.1.1.1,9.9.9.9" {
		t.Fatalf("nameservers = %v", got)
	}
}

func TestUpstreamResolvers(t *testing.T) {
	none := func(net.IP) bool { return false }
	files := func(m map[string]string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if s, ok := m[p]; ok {
				return []byte(s), nil
			}
			return nil, os.ErrNotExist
		}
	}
	// systemd-resolved's stub is followed to its upstream list.
	got, err := upstreamResolvers(files(map[string]string{
		resolvConf:       "nameserver 127.0.0.53\noptions edns0\n",
		resolvedUpstream: "nameserver 10.1.2.3\n",
	}), none)
	if err != nil || strings.Join(got, ",") != "10.1.2.3" {
		t.Fatalf("resolved: %v %v", got, err)
	}
	// WSL's DNS-tunnelling resolver is on the host itself: refused, and the
	// fix says how to change it.
	wsl := func(ip net.IP) bool { return ip.Equal(net.ParseIP(wslTunnelResolver)) }
	_, err = upstreamResolvers(files(map[string]string{
		resolvConf: "nameserver 10.255.255.254\n",
	}), wsl)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "generateResolvConf=false") ||
		!strings.Contains(err.Error(), "10.255.255.254") {
		t.Fatalf("wsl: %v", err)
	}
	// A local dnsmasq on a LAN address is refused too.
	_, err = upstreamResolvers(files(map[string]string{resolvConf: "nameserver 127.0.0.1\n"}),
		none)
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "/etc/resolv.conf") {
		t.Fatalf("loopback: %v", err)
	}
}

package firecracker

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// Host posture: the firewall states setup cannot make safe by itself (a11),
// and the upstream resolver the relay is given (Decision 5).

const firewalldFix = "put the upstream TAPs in firewalld's trusted zone: " +
	"firewall-cmd --permanent --zone=trusted --add-interface=wfc+u && firewall-cmd --reload"

const forwardPolicyFix = "set the filter FORWARD policy to DROP and keep it across reboots: " +
	"iptables -P FORWARD DROP && ip6tables -P FORWARD DROP"

// nftChain is the part of `nft -j list chains` posture reads.
type nftChain struct {
	Family string `json:"family"`
	Table  string `json:"table"`
	Name   string `json:"name"`
	Hook   string `json:"hook"`
	Policy string `json:"policy"`
}

// nftForwardDrops returns the base chains in `nft -j list chains` output that
// hook forward with a drop policy and are not iptables' own FORWARD chain (on
// an iptables-nft host) or warden's bridge table. Such a chain drops the
// relays' forwarded traffic whatever WARDEN-FC accepts, since every base
// chain on a hook must accept a packet.
func nftForwardDrops(out []byte) ([]nftChain, error) {
	var doc struct {
		Nftables []struct {
			Chain *nftChain `json:"chain"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("parsing nft chains: %w", err)
	}
	var drops []nftChain
	for _, e := range doc.Nftables {
		c := e.Chain
		if c == nil || c.Hook != "forward" || c.Policy != "drop" {
			continue
		}
		iptablesOwn := (c.Family == "ip" || c.Family == "ip6") && c.Table == "filter" &&
			c.Name == "FORWARD"
		if iptablesOwn || (c.Family == "bridge" && c.Table == nftTable) {
			continue
		}
		drops = append(drops, *c)
	}
	return drops, nil
}

// nftDropFix is the commands that let slot traffic through the chains drops
// names, without opening anything else.
func nftDropFix(drops []nftChain) string {
	var cmds []string
	for _, c := range drops {
		at := c.Family + " " + c.Table + " " + c.Name
		cmds = append(cmds,
			"nft insert rule "+at+` iifname "wfc*u" accept`,
			"nft insert rule "+at+` oifname "wfc*u" ct state established,related `+
				`accept`)
	}
	return "let the relays' upstream traffic through your nftables forward chain(s): " +
		strings.Join(cmds, " && ")
}

// parseNameservers returns the IPv4 nameservers of a resolv.conf.
func parseNameservers(conf string) []string {
	var out []string
	for _, line := range strings.Split(conf, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" {
			if ip := net.ParseIP(f[1]); ip != nil && ip.To4() != nil {
				out = append(out, ip.String())
			}
		}
	}
	return out
}

// selectResolvers splits nameservers into those the relay can use and those
// it cannot: a loopback address or one assigned to the host is reached from
// the relay as NEW traffic to the host, which WARDEN-FC drops.
func selectResolvers(servers []string, local func(net.IP) bool) (use, excluded []string) {
	for _, s := range servers {
		ip := net.ParseIP(s)
		if ip == nil || ip.IsLoopback() || local(ip) {
			excluded = append(excluded, s)
			continue
		}
		use = append(use, s)
	}
	return use, excluded
}

const (
	resolvConf         = "/etc/resolv.conf"
	resolvedUpstream   = "/run/systemd/resolve/resolv.conf"
	resolvedStubServer = "127.0.0.53"
	wslTunnelResolver  = "10.255.255.254"
)

// upstreamResolvers is the host's off-host resolvers for the relay: those of
// /etc/resolv.conf, or of systemd-resolved's upstream list when its stub
// fronts them, minus loopback and host-local addresses. None left is a
// fixable error naming what was excluded.
func upstreamResolvers(readFile func(string) ([]byte, error), local func(net.IP) bool) ([]string,
	error) {
	b, err := readFile(resolvConf)
	if err != nil {
		return nil, driver.Fixable("list an off-host DNS server as a nameserver in "+
			""+resolvConf,
			"reading %s: %v", resolvConf, err)
	}
	servers := parseNameservers(string(b))
	if len(servers) == 1 && servers[0] == resolvedStubServer {
		if up, err := readFile(resolvedUpstream); err == nil {
			servers = parseNameservers(string(up))
		}
	}
	use, excluded := selectResolvers(servers, local)
	if len(use) > 0 {
		return use, nil
	}
	fix := "point the host's resolver at an off-host DNS server: list one as a nameserver " +
		"in " + resolvConf + " (under systemd-resolved, set DNS= in " +
		"/etc/systemd/resolved.conf)"
	for _, s := range excluded {
		if s == wslTunnelResolver {
			fix = "WSL's only resolver is its DNS-tunnelling address " +
				wslTunnelResolver +
				" on the host itself: set generateResolvConf=false under " +
				"[network] in " +
				"/etc/wsl.conf and list the Windows host's DNS servers as " +
				"nameservers in " +
				resolvConf + " (or set dnsTunneling=false in .wslconfig)"
		}
	}
	what := "none listed"
	if len(excluded) > 0 {
		what = "only local resolvers: " + strings.Join(excluded, ", ")
	}
	return nil, driver.Fixable(fix,
		"the relay needs an off-host upstream DNS server, and the host has %s", what)
}

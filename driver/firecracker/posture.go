package firecracker

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// Host posture: the firewall states setup cannot make safe by itself (a11),
// and the upstream resolvers the relay is given in its claim (upstream_dns).

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

// wslTunnelResolver is WSL's DNS-tunnelling resolver, an address on the host
// itself.
const wslTunnelResolver = "10.255.255.254"

// vmDNSFix is the remedy every host-resolver refusal also names.
const vmDNSFix = "or set [vm] dns in warden.toml to DNS servers the relay can reach"

// resolverRule is Firecracker's reachability rule: a loopback address or one
// assigned to the host is reached from the relay as NEW traffic to the host,
// which WARDEN-FC drops; a link-local one is not reachable from behind the
// slot's NAT; and one inside the build link is routed by the relay VM's
// connected route to the build VM.
func resolverRule(local func(net.IP) bool, link buildlink.Link) resolvers.Rule {
	return resolvers.Rules(resolvers.Loopback,
		func(a netip.Addr) string {
			if local(net.IP(a.AsSlice())) {
				return "assigned to this host"
			}
			return ""
		},
		resolvers.LinkLocal, resolvers.BuildLink(link))
}

// relayResolvers is the relay's upstream resolvers and where they came from:
// [vm] dns when set, each entry checked by the rule and no host file read;
// otherwise the host's (resolvers.Linux), minus those the rule excludes. An
// unusable [vm] dns entry, or no usable host resolver, is a fixable error.
func relayResolvers(read resolvers.ReadFile, local func(net.IP) bool,
	link buildlink.Link, vmDNS []netip.Addr) (list []netip.Addr, source string, err error) {
	rule := resolverRule(local, link)
	if len(vmDNS) > 0 {
		list, err := resolvers.Check(vmDNS, rule)
		if err != nil {
			return nil, resolvers.VMDNS, vmDNSRefusal(err, link)
		}
		return list, resolvers.VMDNS, nil
	}
	host, source, err := resolvers.Linux(read)
	if err != nil {
		return nil, source, driver.Fixable("list an off-host DNS server as a nameserver "+
			"in "+resolvers.ResolvConf+", "+vmDNSFix, "%v", err)
	}
	use, excluded := resolvers.Select(host, rule)
	if len(use) > 0 {
		return use, source, nil
	}
	fix := "point the host's resolver at an off-host DNS server: list one as a nameserver " +
		"in " + resolvers.ResolvConf + " (under systemd-resolved, set DNS= in " +
		"/etc/systemd/resolved.conf), " + vmDNSFix
	for _, e := range excluded {
		if resolvers.InLink(link, e.Addr) {
			fix = "the relay's link to the build VM is " + link.String() +
				", so it cannot reach a resolver there: list an off-host DNS " +
				"server outside it as a nameserver in " + resolvers.ResolvConf +
				" (under systemd-resolved, set DNS= in " +
				"/etc/systemd/resolved.conf), or set [vm] build_link in " +
				"warden.toml (or --build-link) to a /30 that holds none of " +
				"the host's resolvers, " + vmDNSFix
		}
		if e.Addr.String() == wslTunnelResolver {
			fix = "WSL's only resolver is its DNS-tunnelling address " +
				wslTunnelResolver +
				" on the host itself: set generateResolvConf=false under " +
				"[network] in " +
				"/etc/wsl.conf and list the Windows host's DNS servers as " +
				"nameservers in " +
				resolvers.ResolvConf +
				" (or set dnsTunneling=false in .wslconfig), " + vmDNSFix
		}
	}
	what := "none listed in " + source
	if len(excluded) > 0 {
		what = "only resolvers it cannot reach in " + source + ": " +
			resolvers.JoinExcluded(excluded)
	}
	return nil, source, driver.Fixable(fix,
		"the relay needs an off-host upstream DNS server, and the host has %s", what)
}

// vmDNSRefusal is the fixable form of resolvers.Check's refusal.
func vmDNSRefusal(err error, link buildlink.Link) error {
	fix := "change or remove that entry in [vm] dns"
	var ue *resolvers.UnusableError
	if errors.As(err, &ue) && resolvers.InLink(link, ue.Addr) {
		fix += ", or set [vm] build_link in warden.toml (or --build-link) to a /30 " +
			"that holds none of [vm] dns"
	}
	return driver.Fixable(fix, "%v", err)
}

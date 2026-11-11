package hyperv

import (
	"errors"
	"net/netip"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// buildNetwork is the network a Hyper-V build runs on, decided before any VM:
// the NAT the relay's upstream NIC attaches to, read back from the host, and
// either the ephemeral strategy's per-build switch or the durable strategy's
// slot switches, one of which the allocator picks under the host lock.
type buildNetwork struct {
	NAT       string
	NATPrefix netip.Prefix
	HostIP    netip.Addr
	// BuildSwitch is the ephemeral strategy's per-build Private switch; empty
	// on the durable strategy.
	BuildSwitch string
	// Slots are the durable strategy's build switches; empty on the
	// ephemeral strategy.
	Slots []string
}

// checkNATOverlap refuses a build whose NAT prefix overlaps the build link,
// before any VM boots: the relay's upstream and its build segment would
// share addresses.
func checkNATOverlap(nat netip.Prefix, link buildlink.Link) error {
	if !nat.Overlaps(link.Net) {
		return nil
	}
	return driver.Fixable(
		"set [vm] build_link in warden.toml (or pass --build-link) to a /30 outside "+
			nat.String()+", or move the NAT with `warden setup --driver hyperv "+
			"--nat-prefix <prefix>` from an elevated shell",
		"the Hyper-V NAT prefix %s overlaps the build link %s", nat, link)
}

// vmDNSFix is the remedy every host-resolver refusal also names.
const vmDNSFix = "or set [vm] dns in warden.toml to DNS servers the relay can reach"

// resolverRule is Hyper-V's reachability rule for the relay VM's upstream
// resolvers: a loopback or link-local address would be the relay VM's own; one
// assigned to the host, or inside the NAT prefix, is the host side of WinNAT,
// which forwards no DNS; and one inside the build link is routed to the build
// VM.
func resolverRule(host []netip.Addr, nat netip.Prefix, link buildlink.Link) resolvers.Rule {
	return resolvers.Rules(resolvers.Loopback, resolvers.LinkLocal,
		func(a netip.Addr) string {
			for _, h := range host {
				if h == a.Unmap() {
					return "assigned to this host"
				}
			}
			return ""
		},
		resolvers.InPrefix(nat, "the Hyper-V NAT prefix"), resolvers.BuildLink(link))
}

// hostDNSSource reads the host's resolvers and names where they came from.
type hostDNSSource func() (list []netip.Addr, source string, err error)

// relayResolvers is the relay's upstream resolvers and where they came from:
// [vm] dns when set, each entry checked by the rule and the host never read;
// otherwise the host's, from hostDNS, minus those the rule excludes. An
// unusable [vm] dns entry, or no usable host resolver, is a fixable error.
func relayResolvers(hostDNS hostDNSSource, host []netip.Addr, nat netip.Prefix,
	link buildlink.Link, vmDNS []netip.Addr) (list []netip.Addr, source string, err error) {
	rule := resolverRule(host, nat, link)
	if len(vmDNS) > 0 {
		list, err := resolvers.Check(vmDNS, rule)
		if err != nil {
			return nil, resolvers.VMDNS, vmDNSRefusal(err, link)
		}
		return list, resolvers.VMDNS, nil
	}
	found, source, err := hostDNS()
	if err != nil {
		return nil, source, err
	}
	use, excluded := resolvers.Select(found, rule)
	if len(use) > 0 {
		return use, source, nil
	}
	fix := "give the host an off-host DNS server (on " + source + "), " + vmDNSFix
	for _, e := range excluded {
		if resolvers.InLink(link, e.Addr) {
			fix = "give the host an off-host DNS server outside the build " +
				"link " + link.String() + " (on " + source + "), or set [vm] " +
				"build_link in warden.toml (or --build-link) to a /30 that " +
				"holds none of the host's resolvers, " + vmDNSFix
		}
	}
	what := "none on " + source
	if len(excluded) > 0 {
		what = "only resolvers it cannot reach on " + source + ": " +
			resolvers.JoinExcluded(excluded)
	}
	return nil, source, driver.Fixable(fix,
		"the relay VM needs an upstream DNS server it can reach through WinNAT, "+
			"and the host has %s", what)
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

// hostAddrs is the host's interface addresses, the rule's "assigned to this
// host" set.
func hostAddrs() ([]netip.Addr, error) {
	prefixes, err := driver.HostPrefixes()
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, len(prefixes))
	for i, p := range prefixes {
		out[i] = p.Addr()
	}
	return out, nil
}

// prepareBuildNetwork runs the checks a build's network takes before any VM:
// the NAT prefix against the build link, and the relay's resolvers, which it
// sets into req.DNS for the claim's upstream_dns.
func prepareBuildNetwork(req *driver.BuildRequest, nat netip.Prefix,
	hostDNS hostDNSSource) error {
	link := req.Link()
	if err := checkNATOverlap(nat, link); err != nil {
		return err
	}
	host, err := hostAddrs()
	if err != nil {
		return err
	}
	dns, _, err := relayResolvers(hostDNS, host, nat, link, req.DNS)
	if err != nil {
		return err
	}
	req.DNS = dns
	return nil
}

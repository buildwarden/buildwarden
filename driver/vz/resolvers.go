package vz

import (
	"net/netip"
	"os"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// resolverRule is vz's reachability rule for the relay's upstream resolvers:
// only the build link, which the relay routes to the build VM. The relay is a
// process on the Mac, so a loopback resolver there is reachable.
func resolverRule(link buildlink.Link) resolvers.Rule {
	return resolvers.BuildLink(link)
}

// relayResolvers is the relay's upstream resolvers and where they came from:
// [vm] dns when set, each entry checked by the rule and no host file read;
// otherwise the Mac's /etc/resolv.conf, minus those the rule excludes. An
// unusable [vm] dns entry, or no usable host resolver, is a fixable error.
func relayResolvers(read resolvers.ReadFile, link buildlink.Link,
	vmDNS []netip.Addr) (list []netip.Addr, source string, err error) {
	rule := resolverRule(link)
	if len(vmDNS) > 0 {
		list, err := resolvers.Check(vmDNS, rule)
		if err != nil {
			return nil, resolvers.VMDNS, driver.Fixable("change or remove that "+
				"entry in [vm] dns, or set [vm] build_link in warden.toml (or "+
				"--build-link) to a /30 that holds none of [vm] dns", "%v", err)
		}
		return list, resolvers.VMDNS, nil
	}
	const vmDNSFix = "or set [vm] dns in warden.toml to the DNS servers the relay " +
		"should use"
	host, source, err := resolvers.Mac(read)
	if err != nil {
		return nil, source, driver.Fixable("give the Mac a DNS server (a network "+
			"connection that lists one in "+resolvers.ResolvConf+"), "+vmDNSFix,
			"%v", err)
	}
	use, excluded := resolvers.Select(host, rule)
	if len(use) > 0 {
		return use, source, nil
	}
	if len(excluded) == 0 {
		return nil, source, driver.Fixable("give the Mac a DNS server (a network "+
			"connection that lists one in "+source+"), "+vmDNSFix,
			"the relay needs an upstream DNS server, and the Mac lists none in %s",
			source)
	}
	return nil, source, driver.Fixable("give the Mac an off-host DNS server outside "+
		"the build link "+link.String()+", or set [vm] build_link in warden.toml (or "+
		"--build-link) to a /30 that holds none of the Mac's resolvers, "+vmDNSFix,
		"the relay needs an upstream DNS server, and the Mac has only resolvers "+
			"inside the build link in %s: %s", source,
		resolvers.JoinExcluded(excluded))
}

// setRelayDNS replaces req.DNS ([vm] dns) with the relay's resolvers, which
// the claim carries as upstream_dns.
func setRelayDNS(req *driver.BuildRequest, read resolvers.ReadFile) error {
	dns, _, err := relayResolvers(read, req.Link(), req.DNS)
	if err != nil {
		return err
	}
	req.DNS = dns
	return nil
}

// Resolvers is the relay's upstream resolvers on this Mac and their source
// ([vm] dns, or /etc/resolv.conf), or the refusal a build would get: what
// `warden doctor --driver vz` prints. vmDNS is [vm] dns.
func Resolvers(link buildlink.Link, vmDNS []netip.Addr) ([]netip.Addr, string, error) {
	return relayResolvers(os.ReadFile, link, vmDNS)
}

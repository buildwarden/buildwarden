package qemu

import (
	"errors"
	"net/netip"
	"os"
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// vmDNSFix is the remedy every host-resolver refusal also names.
const vmDNSFix = "or set [vm] dns in warden.toml to DNS servers the relay can reach"

// resolverRule is qemu's reachability rule for the relay VM's upstream
// resolvers: a loopback or link-local address would be the relay VM's own,
// one inside user-mode NAT's network is qemu's (or nothing), and one inside
// the build link is routed to the build VM.
func resolverRule(link buildlink.Link) resolvers.Rule {
	return resolvers.Rules(resolvers.Loopback, resolvers.LinkLocal,
		resolvers.InPrefix(netip.MustParsePrefix(UserNetCIDR), "qemu's user-mode network"),
		resolvers.BuildLink(link))
}

// hostSource is the host resolver source for goos: the Mac's /etc/resolv.conf
// on macOS and the Linux source on Linux. The qemu driver is not a Windows
// host driver, so there is none there.
func hostSource(goos string) (func(resolvers.ReadFile) ([]netip.Addr, string, error), error) {
	switch goos {
	case "darwin":
		return resolvers.Mac, nil
	case "linux":
		return resolvers.Linux, nil
	}
	return nil, driver.Fixable("set [vm] dns in warden.toml to the DNS servers the relay "+
		"should use, or use --driver hyperv on a Windows host",
		"the qemu driver reads the host's resolvers on macOS and Linux, and this host "+
			"is %s", goos)
}

// relayResolvers is the relay's upstream resolvers and where they came from:
// [vm] dns when set, each entry checked by the rule and no host file read;
// otherwise the host's for goos, minus those the rule excludes. An unusable
// [vm] dns entry, or no usable host resolver, is a fixable error.
func relayResolvers(goos string, read resolvers.ReadFile, link buildlink.Link,
	vmDNS []netip.Addr) (list []netip.Addr, source string, err error) {
	rule := resolverRule(link)
	if len(vmDNS) > 0 {
		list, err := resolvers.Check(vmDNS, rule)
		if err != nil {
			return nil, resolvers.VMDNS, vmDNSRefusal(err, link)
		}
		return list, resolvers.VMDNS, nil
	}
	src, err := hostSource(goos)
	if err != nil {
		return nil, "", err
	}
	host, source, err := src(read)
	if err != nil {
		return nil, source, driver.Fixable("give the host an off-host DNS server (a "+
			"nameserver in "+resolvers.ResolvConf+"), "+vmDNSFix, "%v", err)
	}
	use, excluded := resolvers.Select(host, rule)
	if len(use) > 0 {
		return use, source, nil
	}
	fix := "give the host an off-host DNS server (a nameserver in " + source + "), " +
		vmDNSFix
	for _, e := range excluded {
		if resolvers.InLink(link, e.Addr) {
			fix = "give the host an off-host DNS server outside the build link " +
				link.String() + " (a nameserver in " + source + "), or set " +
				"[vm] build_link in warden.toml (or --build-link) to a /30 that " +
				"holds none of the host's resolvers, " + vmDNSFix
		}
	}
	what := "none listed in " + source
	if len(excluded) > 0 {
		what = "only resolvers it cannot reach in " + source + ": " +
			resolvers.JoinExcluded(excluded)
	}
	return nil, source, driver.Fixable(fix,
		"the relay VM needs an upstream DNS server it can reach through qemu's "+
			"user-mode NAT, and the host has %s", what)
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

// setRelayDNS replaces req.DNS ([vm] dns) with the relay's resolvers on a
// goos host, which the claim carries as upstream_dns.
func setRelayDNS(req *driver.BuildRequest, goos string, read resolvers.ReadFile) error {
	dns, _, err := relayResolvers(goos, read, req.Link(), req.DNS)
	if err != nil {
		return err
	}
	req.DNS = dns
	return nil
}

// Resolvers is the relay's upstream resolvers on this host and their source
// ([vm] dns, or the host file they came from), or the refusal a build would
// get: what `warden doctor --driver qemu` prints. vmDNS is [vm] dns.
func Resolvers(link buildlink.Link, vmDNS []netip.Addr) ([]netip.Addr, string, error) {
	return relayResolvers(runtime.GOOS, os.ReadFile, link, vmDNS)
}

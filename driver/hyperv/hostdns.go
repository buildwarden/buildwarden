package hyperv

import (
	"net/netip"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// The host's resolvers on Windows, the default the relay's upstream_dns takes
// when [vm] dns is unset (Decision 6): the IPv4 DNS servers of the interface
// that holds the lowest-metric IPv4 default route, the interface the host's
// own lookups leave by. hostdns_windows.go queries them; this parser is pure.

// parseDefaultRouteDNS reads the default-route DNS script's lines:
// ROUTE=none when the host has no IPv4 default route, else ALIAS=<interface>
// and one DNS=<address> per IPv4 server it lists. The source names the
// interface, and so does each refusal.
func parseDefaultRouteDNS(out string) (list []netip.Addr, source string, err error) {
	alias, noRoute := "", false
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "ROUTE":
			noRoute = v == "none"
		case "ALIAS":
			alias = v
		case "DNS":
			if a, err := netip.ParseAddr(v); err == nil && a.Is4() {
				list = append(list, a)
			}
		}
	}
	if noRoute || alias == "" {
		return nil, "", driver.Fixable("give the host an IPv4 default route, "+vmDNSFix,
			"the host has no IPv4 default route, so it has no resolvers to give the relay")
	}
	source = "interface " + alias + " (the IPv4 default route's)"
	if len(list) == 0 {
		return nil, source, driver.Fixable(
			"give interface "+alias+" an IPv4 DNS server, "+vmDNSFix,
			"interface %s, which holds the host's IPv4 default route, lists no IPv4 "+
				"DNS server", alias)
	}
	return list, source, nil
}

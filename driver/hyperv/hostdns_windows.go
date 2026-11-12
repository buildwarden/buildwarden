//go:build windows

package hyperv

import (
	"fmt"
	"net/netip"
)

// defaultRouteDNS queries the IPv4 DNS servers of the interface holding the
// host's lowest-metric IPv4 default route (the route's metric plus its
// interface's, as Windows ranks them).
func defaultRouteDNS() ([]netip.Addr, string, error) {
	out, err := runPS(`
$r = Get-NetRoute -DestinationPrefix 0.0.0.0/0 -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Sort-Object { $_.RouteMetric + $_.InterfaceMetric } | Select-Object -First 1
if (-not $r) { 'ROUTE=none'; return }
"ALIAS=$($r.InterfaceAlias)"
(Get-DnsClientServerAddress -InterfaceIndex $r.ifIndex -AddressFamily IPv4).ServerAddresses |
    ForEach-Object { "DNS=$_" }
`)
	if err != nil {
		return nil, "", fmt.Errorf("reading the host's DNS servers: %w", err)
	}
	return parseDefaultRouteDNS(out)
}

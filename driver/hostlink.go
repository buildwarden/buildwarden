package driver

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// HostPrefixes returns this host's interface addresses with their lengths,
// the host side of the build-link overlap check (buildlink.Link.HostOverlap).
func HostPrefixes() ([]netip.Prefix, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("listing host interfaces: %w", err)
	}
	var out []netip.Prefix
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipn.IP)
		if !ok {
			continue
		}
		ones, _ := ipn.Mask.Size()
		if ip.Is4In6() {
			ip = ip.Unmap()
			if ones >= 96 {
				ones -= 96
			}
		}
		out = append(out, netip.PrefixFrom(ip, ones))
	}
	return out, nil
}

// LinkFix is the fix every link-overlap refusal names.
const LinkFix = "set [vm] build_link in warden.toml (or pass --build-link) to a /30 " +
	"no host network uses, e.g. build_link = \"100.126.255.252/30\""

// CheckHostOverlap refuses a build link that holds an address of one of the
// host's interfaces, before any VM boots. A host network that merely
// contains the link is not refused (doctor warns about it).
func CheckHostOverlap(link buildlink.Link, host []netip.Prefix) error {
	refuse, _ := link.HostOverlap(host)
	if len(refuse) == 0 {
		return nil
	}
	names := make([]string, len(refuse))
	for i, p := range refuse {
		names[i] = p.String()
	}
	return Fixable(LinkFix,
		"the build link %s holds a host interface address (%s)",
		link, strings.Join(names, ", "))
}

// CheckLauncherStamp refuses a launcher asset built before its launcher read
// the build link per build: stamp is the asset's link-channel file, what
// names the asset, and rebuild the one-time command that replaces it.
func CheckLauncherStamp(stamp, what, rebuild string) error {
	if err := buildlink.CheckStamp(stamp); err != nil {
		return Fixable(rebuild,
			"%s predates the configurable build link and must be rebuilt once (%v)",
			what, err)
	}
	return nil
}

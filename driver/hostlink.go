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

// CheckHostLink is CheckHostOverlap against this host's interfaces, the
// build-start check every VM driver runs before anything boots.
func CheckHostLink(link buildlink.Link) error {
	host, err := HostPrefixes()
	if err != nil {
		return err
	}
	return CheckHostOverlap(link, host)
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

// LauncherStamp names one launcher asset's link-channel stamp, for doctor and
// the build-start check.
type LauncherStamp struct {
	// What names the asset, e.g. "qemu direct-boot initramfs".
	What string
	// Path is the stamp file; empty when the asset itself was not found.
	Path string
	// Rebuild is the one-time command that replaces the asset.
	Rebuild string
	// Required is true when every build of the driver uses the asset, so a
	// missing or stale stamp refuses every build and doctor fails on it.
	// Direct-boot assets, which only some builds use, leave it false.
	Required bool
}

// Check refuses the asset when its stamp is missing or different.
func (s LauncherStamp) Check() error {
	return CheckLauncherStamp(s.Path, s.What, s.Rebuild)
}

// Blocks reports whether doctor fails on the stamp: the asset is required
// by every build, was found, and its stamp is missing or stale.
func (s LauncherStamp) Blocks() bool {
	return s.Required && s.Path != "" && s.Check() != nil
}

// Report is the stamp's doctor line.
func (s LauncherStamp) Report() string {
	if s.Path == "" {
		return s.What + ": not found"
	}
	state := buildlink.StampState(s.Path)
	if state == "current" {
		return s.What + ": link-channel stamp current"
	}
	prefix := ""
	if s.Required {
		prefix = "BLOCKED: "
	}
	return prefix + s.What + ": link-channel stamp " + state +
		"; rebuild it once: " + s.Rebuild
}

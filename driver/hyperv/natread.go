package hyperv

import (
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// The NAT a build uses is read back from the host (Decision 5): its
// InternalIPInterfaceAddressPrefix and the host vNIC's address in it, never a
// constant, so `warden setup --driver hyperv --nat-prefix/--host-ip` takes
// effect on every build. The scripts in provision_local_windows.go print one
// KEY=value per line; this file reads them.

// setupFix is the fix for a durable network that setup must (re)make.
const setupFix = "run `warden setup --driver hyperv` from an elevated shell"

// netReadback is what the network scripts print.
type netReadback struct {
	NAT     string   // NAT=<name>, "" when the NAT is missing
	Reused  bool     // REUSED=1
	Prefix  string   // PREFIX=<InternalIPInterfaceAddressPrefix>
	HostIPs []string // HOSTIP=<address> per IPv4 address of the NAT's host vNIC
	// NATSwitchMissing is NATSWITCH=missing: the NAT exists, its vSwitch not.
	NATSwitchMissing bool
	Slots            map[string]string // SLOT=<name>\t<switch type>
}

// parseNetReadback reads the network scripts' KEY=value lines, ignoring
// anything else they print.
func parseNetReadback(out string) netReadback {
	r := netReadback{Slots: map[string]string{}}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "NAT":
			r.NAT = v
		case "REUSED":
			r.Reused = v == "1"
		case "PREFIX":
			r.Prefix = v
		case "HOSTIP":
			r.HostIPs = append(r.HostIPs, v)
		case "NATSWITCH":
			r.NATSwitchMissing = v == "missing"
		case "SLOT":
			name, typ, _ := strings.Cut(v, "\t")
			r.Slots[name] = typ
		}
	}
	return r
}

// natAddressing is the NAT's prefix and the host's address in it, from the
// read-back. A NAT whose host vNIC holds no address in its prefix is a
// fixable error naming setup.
func (r netReadback) natAddressing() (netip.Prefix, netip.Addr, error) {
	prefix, err := netip.ParsePrefix(r.Prefix)
	if err != nil || !prefix.Addr().Is4() {
		return netip.Prefix{}, netip.Addr{}, driver.Fixable(setupFix,
			"the NAT %q has no IPv4 internal prefix (read %q)", r.NAT, r.Prefix)
	}
	prefix = prefix.Masked()
	for _, s := range r.HostIPs {
		if ip, err := netip.ParseAddr(s); err == nil && prefix.Contains(ip) {
			return prefix, ip, nil
		}
	}
	held := "none"
	if len(r.HostIPs) > 0 {
		held = strings.Join(r.HostIPs, ", ")
	}
	return netip.Prefix{}, netip.Addr{}, driver.Fixable(setupFix,
		"the host vNIC of NAT %q has no address in its prefix %s (it has: %s)",
		r.NAT, prefix, held)
}

// ensureResources turns EnsureNetwork's read-back into its result.
func (r netReadback) ensureResources(buildSwitch string) (*NetworkResources, error) {
	if r.NAT == "" {
		return nil, fmt.Errorf("EnsureNetwork: the script named no NAT")
	}
	prefix, hostIP, err := r.natAddressing()
	if err != nil {
		return nil, err
	}
	return &NetworkResources{BuildSwitch: buildSwitch, NATName: r.NAT, Reused: r.Reused,
		NATPrefix: prefix, HostIP: hostIP}, nil
}

// slotSuffix matches a slot switch's index after "<network>-".
var slotSuffix = regexp.MustCompile(`^[0-9]+$`)

// slotIndex is name's slot index under network, or -1 when name is not one
// of network's slot switches.
func slotIndex(network, name string) int {
	rest, ok := strings.CutPrefix(name, network+"-")
	if !ok || !slotSuffix.MatchString(rest) {
		return -1
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return -1
	}
	return n
}

// durableResources turns VerifyNetwork's read-back of the durable network
// named network into the NAT a durable build uses and its slot switches, in
// index order. A missing NAT or NAT switch, a NAT with no host address in its
// prefix and a host with no slot switches (set up by an older warden) are
// fixable errors naming setup; a slot switch that is not Private is refused
// as compromised isolation.
func (r netReadback) durableResources(network string) (*NetworkResources, error) {
	switch {
	case r.NAT == "":
		return nil, driver.Fixable(setupFix,
			"the durable network's NAT %q is missing", network+"-nat")
	case r.NATSwitchMissing:
		return nil, driver.Fixable(setupFix,
			"the durable network's NAT %q has no vSwitch", r.NAT)
	}
	prefix, hostIP, err := r.natAddressing()
	if err != nil {
		return nil, err
	}
	var slots []string
	for name, typ := range r.Slots {
		if slotIndex(network, name) < 0 {
			continue
		}
		if typ != "Private" {
			return nil, driver.Fixable(
				"remove it (Remove-VMSwitch -Name '"+name+"'), then "+setupFix,
				"build switch %q is %s, expected Private (isolation compromised)",
				name, typ)
		}
		slots = append(slots, name)
	}
	if len(slots) == 0 {
		return nil, driver.Fixable(setupFix+" to create the build switches",
			"the durable network %q has no build switches (%s-0 and up)", network,
			network)
	}
	sort.Slice(slots, func(i, j int) bool {
		return slotIndex(network, slots[i]) < slotIndex(network, slots[j])
	})
	return &NetworkResources{NATName: r.NAT, Reused: true, NATPrefix: prefix,
		HostIP: hostIP, Slots: slots}, nil
}

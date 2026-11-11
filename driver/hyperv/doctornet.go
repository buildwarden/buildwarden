package hyperv

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// doctorNet is what doctor reads of the host's build network (probeDoctorNet).
type doctorNet struct {
	ReadBack netReadback
	// Attached maps each slot switch to the VMs attached to it, and Notes
	// each relay VM to its Notes: who holds which slot and address.
	Attached map[string][]string
	Notes    map[string]string
	DNS      hostDNSSource
	// Host is the host's addresses, the resolver rule's "assigned to this
	// host"; nil reads them.
	Host []netip.Addr
	Err  error
}

// writeBuildNetwork reports the network a build would use, as a build checks
// it before any VM: the NAT read back (prefix and host IP), its overlap with
// the build link, the relay's resolvers and their source, and each durable
// slot free or its holders. It reports whether any of it blocks a build.
func writeBuildNetwork(w io.Writer, c Capabilities, dn doctorNet, link buildlink.Link,
	vmDNS []netip.Addr) (blocked bool) {
	fmt.Fprintln(w, "\nBuild network:")
	if dn.Err != nil {
		fmt.Fprintf(w, "  %sprobe failed: %v\n", dotted("NAT"), dn.Err)
		return true
	}
	durable := c.NetworkStrategy() == NetReuseDurable
	prefix, ok := writeNAT(w, c, dn.ReadBack, durable)
	blocked = !ok
	if prefix.IsValid() {
		if err := checkNATOverlap(prefix, link); err != nil {
			writeBlocked(w, "NAT and build link", err)
			blocked = true
		}
		host := dn.Host
		if host == nil {
			var err error
			if host, err = hostAddrs(); err != nil {
				writeBlocked(w, "Relay resolvers", err)
				return true
			}
		}
		list, source, err := relayResolvers(dn.DNS, host, prefix, link, vmDNS)
		if err != nil {
			writeBlocked(w, "Relay resolvers", err)
			blocked = true
		} else {
			fmt.Fprintf(w, "  %s%s (from %s)\n", dotted("Relay resolvers"),
				resolvers.Join(list), source)
		}
	}
	writeSlots(w, c.DevSwitch, dn)
	return blocked
}

// writeNAT reports the NAT a build would use and returns its prefix (the
// default prefix when a build would make one), and false when a durable build
// would be refused on it.
func writeNAT(w io.Writer, c Capabilities, r netReadback, durable bool) (netip.Prefix, bool) {
	if durable {
		res, err := r.durableResources(c.DevSwitch)
		if err != nil {
			writeBlocked(w, "NAT", err)
			return netip.Prefix{}, false
		}
		fmt.Fprintf(w, "  %s%s (%s, host %s)\n", dotted("NAT"), res.NATName, res.NATPrefix,
			res.HostIP)
		return res.NATPrefix, true
	}
	if r.NAT == "" {
		prefix := netip.MustParsePrefix(DefaultNATPrefix)
		fmt.Fprintf(w, "  %snone yet (a build makes one on %s, host %s)\n", dotted("NAT"),
			prefix, DefaultHostIP)
		return prefix, true
	}
	prefix, hostIP, err := r.natAddressing()
	if err != nil {
		writeBlocked(w, "NAT", err)
		return netip.Prefix{}, false
	}
	fmt.Fprintf(w, "  %s%s (%s, host %s)\n", dotted("NAT"), r.NAT, prefix, hostIP)
	return prefix, true
}

// writeSlots reports each durable build switch free or the VMs holding it,
// naming a relay VM's recorded address, or that there are none.
func writeSlots(w io.Writer, network string, dn doctorNet) {
	slots := dn.ReadBack.slotNames(network)
	if len(slots) == 0 {
		fmt.Fprintf(w, "  %snone: run `warden setup --driver hyperv` (elevated) to "+
			"create the build switches\n", dotted("Durable build switches"))
		return
	}
	for _, sw := range slots {
		vms := dn.Attached[sw]
		if len(vms) == 0 {
			fmt.Fprintf(w, "  %sfree\n", dotted("Slot "+sw))
			continue
		}
		holders := make([]string, len(vms))
		for i, vm := range vms {
			holders[i] = vm
			if ip, _ := parseRelayNote(dn.Notes[vm]); ip.IsValid() {
				holders[i] += " (" + ip.String() + ")"
			}
		}
		fmt.Fprintf(w, "  %sin use by %s\n", dotted("Slot "+sw), strings.Join(holders, ", "))
	}
}

// writeBlocked is a BLOCKED row with the error and, for a fixable one, its fix.
func writeBlocked(w io.Writer, label string, err error) {
	fmt.Fprintf(w, "  %sBLOCKED: %v\n", dotted(label), err)
	var fe *driver.FixableError
	if errors.As(err, &fe) && fe.Fix != "" {
		fmt.Fprintf(w, "    -> %s\n", fe.Fix)
	}
}

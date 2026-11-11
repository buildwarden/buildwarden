package hyperv

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// The per-build relay address and durable build switch (Decisions 4, 7). Both
// are read from Hyper-V itself, so every warden process sees the same state
// whether a relay VM is running, stopped, kept by --keep-vms or left by a
// crash: a relay VM's Notes record its address and switch, and an attached
// adapter holds a switch. StartRelay runs the allocation and creates the relay
// VM under driver.HostLock(networkLock), so two builds never pick the same
// address or switch.

// networkLock is the host lock the allocation and the relay VM's creation
// run under.
const networkLock = "hyperv-network"

const (
	notesIPKey     = "warden-relay-ip="
	notesSwitchKey = "warden-relay-switch="
)

// relayNotes is the Notes a relay VM is created with.
func relayNotes(ip netip.Addr, buildSwitch string) string {
	return notesIPKey + ip.String() + " " + notesSwitchKey + buildSwitch
}

// parseRelayNote reads a relay VM's Notes; a missing or malformed address is
// the zero Addr.
func parseRelayNote(notes string) (ip netip.Addr, buildSwitch string) {
	for _, f := range strings.Fields(notes) {
		switch {
		case strings.HasPrefix(f, notesIPKey):
			ip, _ = netip.ParseAddr(strings.TrimPrefix(f, notesIPKey))
		case strings.HasPrefix(f, notesSwitchKey):
			buildSwitch = strings.TrimPrefix(f, notesSwitchKey)
		}
	}
	return ip, buildSwitch
}

// allocRelayIP is the lowest host address of prefix that is not its network
// or broadcast address, the host's address on the NAT, or recorded in a relay
// VM's Notes. A prefix with none free is a fixable error naming the VMs that
// hold its addresses.
func allocRelayIP(prefix netip.Prefix, hostIP netip.Addr,
	notes map[string]string) (netip.Addr, error) {
	prefix = prefix.Masked()
	taken := map[netip.Addr]bool{hostIP: true}
	var holders []string
	for vm, n := range notes {
		if ip, _ := parseRelayNote(n); ip.IsValid() && prefix.Contains(ip) {
			taken[ip] = true
			holders = append(holders, vm+" ("+ip.String()+")")
		}
	}
	base := prefix.Addr().As4()
	last := binary.BigEndian.Uint32(base[:]) | (1<<(32-prefix.Bits()) - 1)
	var bcast [4]byte
	binary.BigEndian.PutUint32(bcast[:], last)
	broadcast := netip.AddrFrom4(bcast)
	for a := prefix.Addr().Next(); a.IsValid() && a.Less(broadcast); a = a.Next() {
		if !taken[a] {
			return a, nil
		}
	}
	sort.Strings(holders)
	held := "none"
	if len(holders) > 0 {
		held = strings.Join(holders, ", ")
	}
	return netip.Addr{}, driver.Fixable(
		"wait for a running build to finish, remove a relay VM kept with "+
			"--keep-vms (Remove-VM -Name <name>), or run `warden setup --driver "+
			"hyperv --nat-prefix <a larger prefix>` from an elevated shell",
		"the NAT prefix %s has no free address for this build's relay "+
			"(host %s; relay VMs holding addresses: %s)", prefix, hostIP, held)
}

// allocSlot is the first of slots, in order, with no adapter attached: a
// relay VM or a build VM alone marks a slot busy. All busy is a fixable error
// naming each slot's holders.
func allocSlot(slots []string, attached map[string][]string) (string, error) {
	busy := make([]string, 0, len(slots))
	for _, sw := range slots {
		vms := attached[sw]
		if len(vms) == 0 {
			return sw, nil
		}
		busy = append(busy, sw+" ("+strings.Join(vms, ", ")+")")
	}
	return "", driver.Fixable(
		"wait for a running build to finish, remove a VM kept with --keep-vms "+
			"(Remove-VM -Name <name>), or run `warden setup --driver hyperv "+
			"--slots <more>` from an elevated shell",
		"all %d durable build switches are in use: %s", len(slots),
		strings.Join(busy, "; "))
}

// relayAllocation is where a build's relay goes: its address on the NAT and
// the build switch its build NIC attaches to.
type relayAllocation struct {
	IP          netip.Addr
	BuildSwitch string
}

// allocateRelay reads the host's relay VMs' Notes and, for a durable build
// (slots non-empty), the slots' adapters, and picks the relay's address and
// build switch. An ephemeral build passes its own buildSwitch and no slots.
// The caller holds networkLock until the relay VM exists.
func allocateRelay(ctx context.Context, prov Provisioner, prefix netip.Prefix,
	hostIP netip.Addr, slots []string, buildSwitch string) (relayAllocation, error) {
	if !prefix.Addr().Is4() {
		return relayAllocation{}, fmt.Errorf("relay allocation: NAT prefix %v is not IPv4",
			prefix)
	}
	if len(slots) == 0 && buildSwitch == "" {
		return relayAllocation{}, fmt.Errorf(
			"relay allocation: no build switch and no slots")
	}
	notes, err := prov.RelayNotes(ctx)
	if err != nil {
		return relayAllocation{}, err
	}
	ip, err := allocRelayIP(prefix, hostIP, notes)
	if err != nil {
		return relayAllocation{}, err
	}
	if len(slots) == 0 {
		return relayAllocation{IP: ip, BuildSwitch: buildSwitch}, nil
	}
	attached, err := prov.SwitchAdapters(ctx, slots)
	if err != nil {
		return relayAllocation{}, err
	}
	sw, err := allocSlot(slots, attached)
	if err != nil {
		return relayAllocation{}, err
	}
	return relayAllocation{IP: ip, BuildSwitch: sw}, nil
}

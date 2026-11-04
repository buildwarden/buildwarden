package relaycfg

import (
	"errors"
	"fmt"
	"net/netip"
)

// BuildLink is the private segment between the relay and the build
// environment, chosen by the orchestrator and pushed in the claim. A vm-mode
// relay addresses its build interface with Relay inside Prefix, a host-mode
// relay builds its netstack and DHCP lease from it, and every mode that has
// it refuses upstream dials into Prefix.
type BuildLink struct {
	// Prefix is the segment's network, e.g. "192.0.2.0/30".
	Prefix string `json:"prefix"`
	// Relay is the relay's own address on the segment.
	Relay string `json:"relay"`
	// Guest is the build environment's address on the segment.
	Guest string `json:"guest"`
}

// ErrRefused, wrapped by a Serve start callback's error, marks a claim the
// relay refuses for what it asks (a build link the relay cannot apply, a field
// its mode does not take). Serve answers it with 400 and leaves the relay
// claimable, as it does a malformed claim, instead of consuming the claim.
var ErrRefused = errors.New("claim refused")

// Parse validates the link and returns its parts. The rules are the same for
// every claimant and mode: IPv4; Prefix is its own network address with a
// length from 8 to 30; Relay and Guest are distinct hosts inside it, neither
// its network nor its broadcast address; and no address is unspecified,
// loopback, link-local, multicast or class E. An orchestrator may apply a
// stricter rule of its own.
func (b BuildLink) Parse() (prefix netip.Prefix, relay, guest netip.Addr, err error) {
	prefix, err = netip.ParsePrefix(b.Prefix)
	if err != nil {
		return netip.Prefix{}, netip.Addr{}, netip.Addr{}, fmt.Errorf("prefix: %w", err)
	}
	if !prefix.Addr().Is4() {
		return fail("prefix %s is not IPv4", b.Prefix)
	}
	if prefix.Masked() != prefix {
		return fail("prefix %s is not a network address (did you mean %s?)",
			b.Prefix, prefix.Masked())
	}
	if prefix.Bits() < 8 || prefix.Bits() > 30 {
		return fail("prefix %s: length must be 8 to 30", b.Prefix)
	}
	if err := checkUnicast("prefix", prefix.Addr()); err != nil {
		return netip.Prefix{}, netip.Addr{}, netip.Addr{}, err
	}
	relay, err = parseHost("relay", b.Relay, prefix)
	if err != nil {
		return netip.Prefix{}, netip.Addr{}, netip.Addr{}, err
	}
	guest, err = parseHost("guest", b.Guest, prefix)
	if err != nil {
		return netip.Prefix{}, netip.Addr{}, netip.Addr{}, err
	}
	if relay == guest {
		return fail("relay and guest are both %s", relay)
	}
	return prefix, relay, guest, nil
}

func fail(format string, a ...any) (netip.Prefix, netip.Addr, netip.Addr, error) {
	return netip.Prefix{}, netip.Addr{}, netip.Addr{}, fmt.Errorf(format, a...)
}

func parseHost(name, s string, prefix netip.Prefix) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%s: %w", name, err)
	}
	if !a.Is4() {
		return netip.Addr{}, fmt.Errorf("%s %s is not IPv4", name, s)
	}
	if !prefix.Contains(a) {
		return netip.Addr{}, fmt.Errorf("%s %s is outside prefix %s", name, s, prefix)
	}
	if a == prefix.Addr() || a == broadcast(prefix) {
		return netip.Addr{}, fmt.Errorf(
			"%s %s is the network or broadcast address of %s", name, s, prefix)
	}
	return a, checkUnicast(name, a)
}

// checkUnicast refuses the ranges no build segment may use.
func checkUnicast(name string, a netip.Addr) error {
	switch {
	case a.IsUnspecified() || a.As4()[0] == 0:
		return fmt.Errorf("%s %s is in 0.0.0.0/8", name, a)
	case a.IsLoopback():
		return fmt.Errorf("%s %s is a loopback address", name, a)
	case a.IsLinkLocalUnicast():
		return fmt.Errorf("%s %s is a link-local address", name, a)
	case a.IsMulticast():
		return fmt.Errorf("%s %s is a multicast address", name, a)
	case a.As4()[0] >= 240:
		return fmt.Errorf("%s %s is a class E address", name, a)
	}
	return nil
}

// broadcast returns the last address of an IPv4 prefix.
func broadcast(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().As4()
	host := uint32(1)<<(32-p.Bits()) - 1
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]) | host
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

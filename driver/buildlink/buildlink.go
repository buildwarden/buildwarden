// Package buildlink is the one definition of the VM drivers' build link: the
// private segment between the relay and the build VM. The default lives here
// and nowhere else; a configured value ([vm] build_link, --build-link) is
// parsed and checked here. Every consumer takes the link per build: the relay
// from the claim (Claim), the direct-boot inits from their kernel command
// line (BootArgs), and the seeded guests from their seed. No boot asset bakes
// a link value.
//
// What can still be stale is a launcher built before the link moved to a
// per-build channel, which bakes the old address. Each launcher-carrying
// asset has a stamp beside it, written by the tool that builds it, holding
// WARDEN_LINK_CHANNEL=<Channel>; CheckStamp compares it.
//
// buildlink imports only relaycfg and the standard library, so the relay and
// the driver packages can both use it without a cycle; its errors are plain
// for callers to wrap in driver.Fixable with their own fix.
package buildlink

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/relaycfg"
)

// DefaultCIDR is the default build link: the last /30 of RFC 6598 shared
// address space, which no LAN, VPC or resolver plan uses. The relay is its
// first host and the build VM its second.
const DefaultCIDR = "100.127.255.252/30"

// Channel is the launcher stamp value. A change to how a launcher reads the
// link bumps it, so assets built before the change are refused.
const Channel = 1

// StampKey is the key a launcher stamp file sets to Channel.
const StampKey = "WARDEN_LINK_CHANNEL"

// ErrNoStamp reports a launcher asset with no stamp beside it: one built
// before its launcher read the link per build.
var ErrNoStamp = errors.New("no link-channel stamp")

// Link is a resolved build link.
type Link struct {
	// Net is the /30, e.g. 100.127.255.252/30.
	Net netip.Prefix
	// Relay is the relay's address on the link, Net's first host.
	Relay netip.Addr
	// Guest is the build VM's address on the link, Net's second host.
	Guest netip.Addr
}

// defaultLink is DefaultCIDR parsed once; TestDefault fails if it does not
// parse.
var defaultLink, defaultErr = Parse(DefaultCIDR)

// Default returns the default link, DefaultCIDR.
func Default() Link { return defaultLink }

// IsZero reports whether l is the zero Link (no link resolved).
func (l Link) IsZero() bool { return !l.Net.IsValid() }

// OrDefault returns l, or the default when l is zero.
func (l Link) OrDefault() Link {
	if l.IsZero() {
		return defaultLink
	}
	return l
}

// String returns the link's network, e.g. "100.127.255.252/30".
func (l Link) String() string { return l.Net.String() }

// Parse parses a configured link: an IPv4 network address with prefix
// exactly /30, outside 0.0.0.0/8, loopback, link-local, multicast and class E.
func Parse(cidr string) (Link, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return Link{}, fmt.Errorf("%q is not a CIDR network such as %s", cidr, DefaultCIDR)
	}
	if !p.Addr().Is4() {
		return Link{}, fmt.Errorf("%s is not IPv4", p)
	}
	if p.Bits() != 30 {
		return Link{}, fmt.Errorf("%s is a /%d; the build link must be a /30", p, p.Bits())
	}
	if p.Masked() != p {
		return Link{}, fmt.Errorf("%s is not a network address; did you mean %s?",
			p, p.Masked())
	}
	a := p.Addr()
	b := a.As4()
	switch {
	case b[0] == 0:
		return Link{}, fmt.Errorf("%s is in 0.0.0.0/8", p)
	case a.IsLoopback():
		return Link{}, fmt.Errorf("%s is a loopback network", p)
	case a.IsLinkLocalUnicast():
		return Link{}, fmt.Errorf("%s is a link-local network", p)
	case a.IsMulticast():
		return Link{}, fmt.Errorf("%s is a multicast network", p)
	case b[0] >= 240:
		return Link{}, fmt.Errorf("%s is a class E network", p)
	}
	relay := a.Next()
	return Link{Net: p, Relay: relay, Guest: relay.Next()}, nil
}

// Range is an address range the link must not overlap, named for the error.
type Range struct {
	Name string
	CIDR string
}

// Disjoint returns an error naming the first range that overlaps l.
func (l Link) Disjoint(ranges []Range) error {
	for _, r := range ranges {
		p, err := netip.ParsePrefix(r.CIDR)
		if err != nil {
			return fmt.Errorf("%s: %w", r.Name, err)
		}
		if p.Overlaps(l.Net) {
			return fmt.Errorf("%s overlaps %s (%s)", l.Net, r.Name, p)
		}
	}
	return nil
}

// HostOverlap sorts a host's interface addresses against l. refuse holds
// those whose address lies inside the link, a real collision since no host
// holds a link address by design; warn holds networks that merely contain
// the link (a /10 CGNAT WAN, say), which shadow nothing beyond its four
// addresses.
func (l Link) HostOverlap(addrs []netip.Prefix) (refuse, warn []netip.Prefix) {
	for _, a := range addrs {
		if !a.Addr().Unmap().Is4() {
			continue
		}
		a = netip.PrefixFrom(a.Addr().Unmap(), a.Bits())
		switch {
		case l.Net.Contains(a.Addr()):
			refuse = append(refuse, a)
		case a.Bits() <= l.Net.Bits() && a.Contains(l.Net.Addr()):
			warn = append(warn, a)
		}
	}
	return refuse, warn
}

// Claim is the link as the relay claim carries it.
func (l Link) Claim() *relaycfg.BuildLink {
	return &relaycfg.BuildLink{
		Prefix: l.Net.String(), Relay: l.Relay.String(), Guest: l.Guest.String(),
	}
}

// BootArgs are the kernel command-line keys a direct-boot init reads.
func (l Link) BootArgs() string {
	return "warden.link_ip=" + l.GuestCIDR() + " warden.link_gw=" + l.Relay.String()
}

// GuestCIDR is the build VM's address with the link's length, e.g.
// "100.127.255.254/30".
func (l Link) GuestCIDR() string {
	return netip.PrefixFrom(l.Guest, l.Net.Bits()).String()
}

// RelayCIDR is the relay's address with the link's length.
func (l Link) RelayCIDR() string {
	return netip.PrefixFrom(l.Relay, l.Net.Bits()).String()
}

// Mask is the link's netmask in dotted form, e.g. "255.255.255.252".
func (l Link) Mask() string {
	m := uint32(0xffffffff) << (32 - l.Net.Bits())
	b := [4]byte{byte(m >> 24), byte(m >> 16), byte(m >> 8), byte(m)}
	return netip.AddrFrom4(b).String()
}

// ReadStamp reads a launcher stamp file's channel. A missing file is
// ErrNoStamp.
func ReadStamp(path string) (int, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("%w at %s", ErrNoStamp, path)
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || k != StampKey {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("%s: malformed %s %q", path, StampKey, v)
		}
		return n, nil
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("%s: no %s line", path, StampKey)
}

// CheckStamp returns nil when the stamp at path holds Channel, and otherwise
// an error saying why the asset beside it is stale (ErrNoStamp when there is
// no stamp at all).
func CheckStamp(path string) error {
	n, err := ReadStamp(path)
	if err != nil {
		return err
	}
	if n != Channel {
		return fmt.Errorf("%s holds link channel %d; this warden needs %d",
			path, n, Channel)
	}
	return nil
}

// StampState is CheckStamp's result as one doctor word: "current",
// "missing" or "stale".
func StampState(path string) string {
	err := CheckStamp(path)
	switch {
	case err == nil:
		return "current"
	case errors.Is(err, ErrNoStamp):
		return "missing"
	}
	return "stale"
}

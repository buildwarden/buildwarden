package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"

	"github.com/buildwarden/buildwarden/relay"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// buildIface is the vm-mode relay's view of its build interface and of the
// interfaces beside it. IPv4 only; nothing here changes a route or a rule.
type buildIface interface {
	// Name is the interface's name.
	Name() string
	// Addrs are the interface's IPv4 addresses, with their lengths.
	Addrs() ([]netip.Prefix, error)
	// Flush removes every IPv4 address from the interface.
	Flush() error
	// Add adds addr (the relay's address with the link's length).
	Add(addr netip.Prefix) error
	// Others are the IPv4 addresses and route destinations on every other
	// interface, loopback included.
	Others() (addrs, routes []netip.Prefix, err error)
}

// vmLink is vm mode's start step: the relay addresses its own build interface
// from the claim and serves the build VM's DHCP lease on it.
type vmLink struct {
	iface buildIface
	// serveDHCP starts the responder bound to the named interface and returns
	// its stop function (openVMDHCP outside tests).
	serveDHCP func(ifName string, l dhcpLease) (func(), error)
}

// start configures the build segment for one claim. With a build link it
// refuses one that collides with another interface, replaces the build
// interface's IPv4 addresses with the relay's (an old init's 10.0.0.1/30
// included), and starts the DHCP responder there, all before the claim is
// answered. Without one it keeps the address already on the interface and
// starts no responder.
func (v vmLink) start(c relaycfg.Config) (relay.Config, func(), error) {
	if c.BuildLink == nil {
		return v.startUnlinked()
	}
	seg, err := claimSegment(c.BuildLink)
	if err != nil {
		return relay.Config{}, nil,
			fmt.Errorf("%w: build_link: %w", relaycfg.ErrRefused, err)
	}
	addrs, routes, err := v.iface.Others()
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("listing interfaces: %w", err)
	}
	if err := linkCollision(seg.prefix, addrs, routes); err != nil {
		return relay.Config{}, nil, fmt.Errorf("%w: %w", relaycfg.ErrRefused, err)
	}
	if err := v.iface.Flush(); err != nil {
		return relay.Config{}, nil, fmt.Errorf("flushing %s: %w", v.iface.Name(), err)
	}
	self := netip.PrefixFrom(seg.relay, seg.prefix.Bits())
	if err := v.iface.Add(self); err != nil {
		return relay.Config{}, nil,
			fmt.Errorf("adding %s to %s: %w", self, v.iface.Name(), err)
	}
	stop, err := v.serveDHCP(v.iface.Name(), newDHCPLease(seg.prefix, seg.relay, seg.guest))
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("DHCP on %s: %w", v.iface.Name(), err)
	}
	log.Printf("relay: build-if %s %s from the claim; DHCP offers %s",
		v.iface.Name(), self, seg.guest)
	return relay.Config{SelfIP: net.IP(seg.relay.AsSlice()), BuildLink: seg.prefix}, stop, nil
}

func (v vmLink) startUnlinked() (relay.Config, func(), error) {
	addrs, err := v.iface.Addrs()
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("reading %s: %w", v.iface.Name(), err)
	}
	if len(addrs) == 0 {
		return relay.Config{}, nil, fmt.Errorf(
			"%w: build_link is required: the build interface %s has no address",
			relaycfg.ErrRefused, v.iface.Name())
	}
	a := addrs[0]
	log.Printf("relay: build-if %s keeps %s (no build_link in the claim)", v.iface.Name(), a)
	return relay.Config{SelfIP: net.IP(a.Addr().AsSlice()), BuildLink: a.Masked()}, nil, nil
}

// linkCollision refuses a link another interface already uses: an address
// inside it, or a route inside it or equal to it. A route that strictly
// contains the link (the default route, a CGNAT /10) is not a collision: the
// link's connected route is more specific and shadows only its own addresses.
func linkCollision(link netip.Prefix, addrs, routes []netip.Prefix) error {
	for _, a := range addrs {
		if link.Contains(a.Addr()) {
			return fmt.Errorf(
				"build_link %s holds %s, an address on another interface", link, a)
		}
	}
	for _, r := range routes {
		if r.Bits() >= link.Bits() && link.Contains(r.Addr()) {
			return fmt.Errorf(
				"build_link %s overlaps the route %s on another interface",
				link, r)
		}
	}
	return nil
}

// pickBuildIf chooses the build interface the way the relay VM init does:
// the named one, or the first entry of /sys/class/net (sorted) that is not lo.
func pickBuildIf(names []string, want string) (string, error) {
	for _, n := range names {
		if want != "" && n == want || want == "" && n != "lo" {
			return n, nil
		}
	}
	if want != "" {
		return "", fmt.Errorf("no interface %q", want)
	}
	return "", fmt.Errorf("no interface other than lo")
}

package main

import (
	"net/netip"

	"github.com/buildwarden/buildwarden/relaycfg"
)

// segment is a resolved build segment: the relay's address, the guest's,
// and the network.
type segment struct {
	prefix       netip.Prefix
	relay, guest netip.Addr
}

// subnetLink resolves --subnet to a segment: gateway first host, guest second.
func subnetLink(cidr string) (segment, error) {
	gw, guest, mask, err := parseSubnet(cidr)
	if err != nil {
		return segment{}, err
	}
	ones, _ := mask.Size()
	r := netip.AddrFrom4([4]byte(gw.To4()))
	return segment{
		prefix: netip.PrefixFrom(r, ones).Masked(),
		relay:  r,
		guest:  netip.AddrFrom4([4]byte(guest.To4())),
	}, nil
}

// claimSegment returns the claim's build link as a segment. relaycfg has
// already validated it.
func claimSegment(b *relaycfg.BuildLink) (segment, error) {
	p, r, g, err := b.Parse()
	return segment{prefix: p, relay: r, guest: g}, err
}

//go:build unix

package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/buildwarden/buildwarden/relay"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// runHostMode starts the relay with a gvisor netstack reading Ethernet
// frames from an inherited socketpair FD. This is the required mode for
// VZ driver (no vmnet entitlement). SSRF filtering is active.
//
// In host mode, all network ingress comes through the FD-based netstack.
// The relay does not bind any ports on the host — the TCP forwarder
// dispatches connections and a netstack UDP listener handles DNS.
func runHostMode(fd int, subnetCIDR, claimAddr string) int {
	fallback, err := subnetLink(subnetCIDR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid subnet: %v\n", err)
		return 1
	}
	h := hostLink{fd: fd, fallback: fallback, newIngress: newFDIngress}
	return serveClaim(claimAddr, h.start)
}

// hostIngress is what host mode needs from its netstack.
type hostIngress interface {
	ListenUDP(port uint16) (net.PacketConn, error)
	Close()
}

// hostLink is host mode's start step.
type hostLink struct {
	fd       int
	fallback segment
	// newIngress builds the netstack on fd (NewFDIngress outside tests).
	newIngress func(FDIngressConfig) (hostIngress, error)
}

func newFDIngress(cfg FDIngressConfig) (hostIngress, error) { return NewFDIngress(cfg) }

// start builds the netstack, ARP and DHCP answers, channel listeners and
// udp/53 on the claim's build link, or on --subnet without one, and returns
// the relay base with SSRF on and the link blocked.
func (h hostLink) start(c relaycfg.Config) (relay.Config, func(), error) {
	seg := h.fallback
	if c.BuildLink != nil {
		var err error
		if seg, err = claimSegment(c.BuildLink); err != nil {
			return relay.Config{}, nil,
				fmt.Errorf("%w: build_link: %w", relaycfg.ErrRefused, err)
		}
	}
	gwIP := net.IP(seg.relay.AsSlice())
	guestIP := net.IP(seg.guest.AsSlice())

	// Channel-based listeners: the FD ingress TCP forwarder delivers
	// connections to these based on destination port.
	httpLn := newChanListener(gwIP, 80)
	httpsLn := newChanListener(gwIP, 443)
	ctrlLn := newChanListener(gwIP, 8300)

	ing, err := h.newIngress(FDIngressConfig{
		FD:         h.fd,
		GatewayIP:  gwIP,
		GuestIP:    guestIP,
		SubnetMask: net.CIDRMask(seg.prefix.Bits(), 32),
		ConnHandler: func(conn net.Conn, dstPort uint16) {
			switch dstPort {
			case 443:
				httpsLn.deliver(conn)
			case 8300:
				ctrlLn.deliver(conn)
			default:
				httpLn.deliver(conn)
			}
		},
	})
	if err != nil {
		return relay.Config{}, nil, fmt.Errorf("creating fd ingress: %w", err)
	}
	dnsConn, err := ing.ListenUDP(53)
	if err != nil {
		ing.Close()
		return relay.Config{}, nil, fmt.Errorf("listening udp/53: %w", err)
	}

	log.Printf("relay: host mode, fd %d, link %s, relay %s, guest %s",
		h.fd, seg.prefix, gwIP, guestIP)
	return relay.Config{
		SelfIP:          gwIP,
		BuildLink:       seg.prefix,
		SSRF:            true,
		DNSPacketConn:   dnsConn,
		HTTPListener:    httpLn,
		HTTPSListener:   httpsLn,
		ControlListener: ctrlLn,
	}, ing.Close, nil
}

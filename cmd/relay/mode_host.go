//go:build unix

package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/buildwarden/buildwarden/relay"
)

// runHostMode starts the relay with a gvisor netstack reading Ethernet
// frames from an inherited socketpair FD. This is the required mode for
// VZ driver (no vmnet entitlement). SSRF filtering is active.
//
// In host mode, all network ingress comes through the FD-based netstack.
// The relay does not bind any ports on the host — the TCP forwarder
// dispatches connections and a netstack UDP listener handles DNS.
func runHostMode(fd int, subnetCIDR, claimAddr string) int {
	gwIP, guestIP, mask, err := parseSubnet(subnetCIDR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid subnet: %v\n", err)
		return 1
	}

	// Channel-based listeners: the FD ingress TCP forwarder delivers
	// connections to these based on destination port.
	httpLn := newChanListener(gwIP, 80)
	httpsLn := newChanListener(gwIP, 443)
	ctrlLn := newChanListener(gwIP, 8300)

	// Create FD ingress first so we can get the DNS PacketConn.
	ing, err := NewFDIngress(FDIngressConfig{
		FD:         fd,
		GatewayIP:  gwIP,
		GuestIP:    guestIP,
		SubnetMask: mask,
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
		fmt.Fprintf(os.Stderr, "error creating fd ingress: %v\n", err)
		return 1
	}

	dnsConn, err := ing.ListenUDP(53)
	if err != nil {
		ing.Close()
		fmt.Fprintf(os.Stderr, "error listening udp/53: %v\n", err)
		return 1
	}

	log.Printf("relay: host mode, fd %d, gateway %s, guest %s", fd, gwIP, guestIP)
	return serveClaim(claimAddr, relay.Config{
		SelfIP:          gwIP,
		BlockedSelfIP:   gwIP,
		SSRF:            true,
		DNSPacketConn:   dnsConn,
		HTTPListener:    httpLn,
		HTTPSListener:   httpsLn,
		ControlListener: ctrlLn,
	}, func() { ing.Close() })
}

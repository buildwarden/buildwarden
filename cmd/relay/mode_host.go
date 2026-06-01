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
func runHostMode(
	fd int, subnetCIDR, outDir, ctxDir, sigDir, captureMode string,
) int {
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
	defer ing.Close()

	dnsConn, err := ing.ListenUDP(53)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listening udp/53: %v\n", err)
		return 1
	}

	cfg := relay.Config{
		LedgerDir:       outDir,
		ContextDir:      ctxDir,
		CaptureMode:     captureMode,
		SignalDir:        sigDir,
		SelfIP:          gwIP,
		BlockedSelfIP:   gwIP,
		SSRF:            true,
		OutputWriter:    os.Stderr,
		DNSPacketConn:   dnsConn,
		HTTPListener:    httpLn,
		HTTPSListener:   httpsLn,
		ControlListener: ctrlLn,
	}

	if err := loadUpstreamCA(outDir, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error configuring upstream TLS: %v\n", err)
		return 1
	}

	// Start relay with injected listeners — no port binding on the host.
	r, err := relay.Start(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting relay: %v\n", err)
		return 1
	}

	log.Printf("relay: host mode, fd %d, gateway %s, guest %s",
		fd, gwIP, guestIP)

	if err := r.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		return 1
	}
	return 0
}

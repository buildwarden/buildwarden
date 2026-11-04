package main

import (
	"log"
	"net"
)

// packetConn is the part of net.PacketConn the vm-mode responder uses.
type packetConn interface {
	ReadFrom(b []byte) (int, net.Addr, error)
	WriteTo(b []byte, addr net.Addr) (int, error)
	Close() error
}

// dhcpBroadcast is where vm mode sends every reply: the one client has no
// address until it takes the lease.
var dhcpBroadcast = &net.UDPAddr{IP: net.IPv4bcast, Port: 68}

// openVMDHCP starts vm mode's DHCP responder on the build interface and
// returns its stop function.
func openVMDHCP(ifName string, l dhcpLease) (func(), error) {
	return startVMDHCP(openVMDHCPConn, ifName, l)
}

// startVMDHCP opens the responder's socket on ifName and serves the lease on
// it until the returned stop function closes it.
func startVMDHCP(open func(string) (packetConn, error), ifName string,
	l dhcpLease) (func(), error) {
	conn, err := open(ifName)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveVMDHCP(conn, l)
	}()
	return func() {
		_ = conn.Close()
		<-done
	}, nil
}

// serveVMDHCP answers each datagram through the shared core and broadcasts
// the reply, until the connection fails or is closed.
func serveVMDHCP(conn packetConn, l dhcpLease) {
	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		rep, ok := dhcpReply(buf[:n], l)
		if !ok {
			continue
		}
		if _, err := conn.WriteTo(rep, dhcpBroadcast); err != nil {
			log.Printf("relay: DHCP reply: %v", err)
			continue
		}
		log.Printf("relay: %s", dhcpLogLine(rep))
	}
}

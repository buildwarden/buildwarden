package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
)

// The relay's DHCP responder, shared by host mode (inside its netstack's
// frame loop) and vm mode (a UDP socket bound to the build interface). There
// is exactly one client on the segment and one answer for it, fixed by the
// claim, so the responder keeps no state: every DISCOVER gets the same OFFER
// and every REQUEST the same ACK, whatever address or client it names. A
// flood of requests therefore exhausts nothing, and the guest can only ever
// be told its own address.

// DHCP message types
const (
	dhcpDiscover = 1
	dhcpOffer    = 2
	dhcpRequest  = 3
	dhcpAck      = 5
)

// DHCP option codes
const (
	optSubnetMask  = 1
	optRouter      = 3
	optDNS         = 6
	optLeaseTime   = 51
	optMessageType = 53
	optServerID    = 54
	optEnd         = 255
)

const (
	bootpFixedLen = 236
	dhcpCookie    = 0x63825363
	// dhcpInfinite is the "infinite" lease time (RFC 2131 §3.3), so a long
	// build never renews.
	dhcpInfinite = 0xffffffff
)

// dhcpLease is the one answer the responder gives: the guest's address, the
// segment's mask, and the relay as server identifier, router and DNS.
type dhcpLease struct {
	relay, guest [4]byte
	mask         [4]byte
}

// newDHCPLease builds the lease for a segment.
func newDHCPLease(prefix netip.Prefix, relay, guest netip.Addr) dhcpLease {
	m := uint32(0xffffffff) << (32 - prefix.Bits())
	l := dhcpLease{relay: relay.As4(), guest: guest.As4()}
	binary.BigEndian.PutUint32(l.mask[:], m)
	return l
}

// dhcpReply answers one BOOTP payload: an OFFER for a DISCOVER, an ACK for a
// REQUEST, both of the lease's guest address, echoing the request's xid and
// chaddr. Anything else (a BOOTREPLY, RELEASE, DECLINE, INFORM, a missing
// magic cookie, a short packet, a truncated option) gets no reply.
func dhcpReply(req []byte, l dhcpLease) ([]byte, bool) {
	if len(req) < bootpFixedLen+4 || req[0] != 1 { // BOOTREQUEST
		return nil, false
	}
	if binary.BigEndian.Uint32(req[bootpFixedLen:]) != dhcpCookie {
		return nil, false
	}
	msgType, ok := dhcpMessageType(req[bootpFixedLen+4:])
	if !ok {
		return nil, false
	}
	var replyType byte
	switch msgType {
	case dhcpDiscover:
		replyType = dhcpOffer
	case dhcpRequest:
		replyType = dhcpAck
	default:
		return nil, false
	}

	// BOOTP fixed header (236 bytes) + DHCP options + padding to 300.
	rep := make([]byte, 300)
	rep[0] = 2                   // BOOTREPLY
	rep[1] = 1                   // Ethernet
	rep[2] = 6                   // HW addr length
	copy(rep[4:8], req[4:8])     // xid
	copy(rep[16:20], l.guest[:]) // yiaddr
	copy(rep[20:24], l.relay[:]) // siaddr
	copy(rep[28:34], req[28:34]) // chaddr

	opts := rep[bootpFixedLen:]
	binary.BigEndian.PutUint32(opts, dhcpCookie)
	i := 4
	put := func(code byte, val []byte) {
		opts[i], opts[i+1] = code, byte(len(val))
		copy(opts[i+2:], val)
		i += 2 + len(val)
	}
	var lease [4]byte
	binary.BigEndian.PutUint32(lease[:], dhcpInfinite)
	put(optMessageType, []byte{replyType})
	put(optServerID, l.relay[:])
	put(optLeaseTime, lease[:])
	put(optSubnetMask, l.mask[:])
	put(optRouter, l.relay[:])
	put(optDNS, l.relay[:])
	opts[i] = optEnd
	return rep, true
}

// dhcpMessageType walks the options after the magic cookie and returns
// option 53's value. A truncated option, or no option 53, is not a message.
func dhcpMessageType(options []byte) (byte, bool) {
	msgType, found := byte(0), false
	for len(options) > 0 {
		switch options[0] {
		case optEnd:
			return msgType, found
		case 0: // padding
			options = options[1:]
			continue
		}
		if len(options) < 2 || len(options) < 2+int(options[1]) {
			return 0, false
		}
		code, length := options[0], int(options[1])
		if code == optMessageType && length >= 1 {
			msgType, found = options[2], true
		}
		options = options[2+length:]
	}
	return msgType, found
}

// dhcpLogLine describes a reply dhcpReply built, for the console.
func dhcpLogLine(rep []byte) string {
	kind := "OFFER"
	if rep[bootpFixedLen+6] == dhcpAck {
		kind = "ACK"
	}
	return fmt.Sprintf("DHCP %s %s to %s", kind,
		net.IP(rep[16:20]), net.HardwareAddr(rep[28:34]))
}

package main

import (
	"encoding/binary"
	"log"
	"net"
)

// DHCP message types
const (
	dhcpDiscover = 1
	dhcpOffer    = 2
	dhcpRequest  = 3
	dhcpAck      = 5
)

// DHCP option codes
const (
	optSubnetMask    = 1
	optRouter        = 3
	optDNS           = 6
	optLeaseTime     = 51
	optMessageType   = 53
	optServerID      = 54
	optEnd           = 255
)

// handleDHCP responds to DHCP discover/request from the guest VM.
// We always offer the same IP (cfg.GuestIP) since there is exactly one client.
func (ing *FDIngress) handleDHCP(frame []byte) {
	if len(frame) < 14+28+236 { // eth + ip + udp + min BOOTP
		return
	}

	ethHeader := frame[:14]
	ipHeader := frame[14:]
	ihl := int(ipHeader[0]&0x0f) * 4
	udpStart := ihl
	bootpStart := udpStart + 8

	if len(ipHeader) < bootpStart+236 {
		return
	}

	bootp := ipHeader[bootpStart:]
	op := bootp[0]
	if op != 1 { // BOOTREQUEST
		return
	}

	xid := bootp[4:8]
	clientMAC := net.HardwareAddr(bootp[28:34])

	// Parse options to find message type
	msgType := byte(0)
	options := bootp[236:]
	if len(options) < 4 || binary.BigEndian.Uint32(options[:4]) != 0x63825363 {
		return // no magic cookie
	}
	options = options[4:]
	for len(options) > 0 {
		if options[0] == optEnd {
			break
		}
		if options[0] == 0 { // padding
			options = options[1:]
			continue
		}
		if len(options) < 2 {
			break
		}
		code := options[0]
		length := int(options[1])
		if len(options) < 2+length {
			break
		}
		if code == optMessageType && length >= 1 {
			msgType = options[2]
		}
		options = options[2+length:]
	}

	var replyType byte
	switch msgType {
	case dhcpDiscover:
		replyType = dhcpOffer
		log.Printf("netstack: DHCP Discover from %s, offering %s",
			clientMAC, ing.cfg.GuestIP)
	case dhcpRequest:
		replyType = dhcpAck
		log.Printf("netstack: DHCP Request from %s, ACK %s",
			clientMAC, ing.cfg.GuestIP)
	default:
		return
	}

	reply := ing.buildDHCPReply(replyType, xid, clientMAC)
	ing.sendEthernetFrame(clientMAC, ethHeader, reply)
}

// buildDHCPReply constructs a DHCP Offer or Ack response.
func (ing *FDIngress) buildDHCPReply(
	msgType byte, xid []byte, clientMAC net.HardwareAddr,
) []byte {
	gwIP := ing.cfg.GatewayIP.To4()
	guestIP := ing.cfg.GuestIP.To4()
	mask := ing.cfg.SubnetMask

	// BOOTP fixed header (236 bytes) + DHCP options + padding to 300.
	bootp := make([]byte, 300)
	bootp[0] = 2 // BOOTREPLY
	bootp[1] = 1 // Ethernet
	bootp[2] = 6 // HW addr length
	copy(bootp[4:8], xid)
	copy(bootp[16:20], guestIP) // yiaddr (your IP)
	copy(bootp[20:24], gwIP)    // siaddr (server IP)
	copy(bootp[28:34], clientMAC)

	// DHCP options start at offset 236 (after fixed BOOTP header)
	opts := bootp[236:]
	i := 0
	// Magic cookie
	copy(opts[i:], []byte{0x63, 0x82, 0x53, 0x63})
	i += 4
	// Message type
	opts[i], opts[i+1], opts[i+2] = optMessageType, 1, msgType
	i += 3
	// Server identifier
	opts[i], opts[i+1] = optServerID, 4
	i += 2
	copy(opts[i:], gwIP)
	i += 4
	// Lease time (1 hour)
	opts[i], opts[i+1] = optLeaseTime, 4
	i += 2
	opts[i], opts[i+1], opts[i+2], opts[i+3] = 0, 0, 0x0e, 0x10
	i += 4
	// Subnet mask
	opts[i], opts[i+1] = optSubnetMask, 4
	i += 2
	copy(opts[i:], mask)
	i += 4
	// Router (gateway)
	opts[i], opts[i+1] = optRouter, 4
	i += 2
	copy(opts[i:], gwIP)
	i += 4
	// DNS server (point to ourselves)
	opts[i], opts[i+1] = optDNS, 4
	i += 2
	copy(opts[i:], gwIP)
	i += 4
	// End
	opts[i] = optEnd

	// Wrap in UDP (src 67, dst 68)
	udpLen := 8 + len(bootp)
	udpHeader := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHeader[0:2], 67)
	binary.BigEndian.PutUint16(udpHeader[2:4], 68)
	udpLenU16 := uint16(udpLen)
	binary.BigEndian.PutUint16(udpHeader[4:6], udpLenU16)

	// Wrap in IPv4
	ipLen := 20 + udpLen
	ipHeader := make([]byte, 20)
	ipHeader[0] = 0x45 // version + IHL
	binary.BigEndian.PutUint16(ipHeader[2:4], uint16(ipLen))
	ipHeader[8] = 64  // TTL
	ipHeader[9] = 17  // UDP
	copy(ipHeader[12:16], gwIP)
	copy(ipHeader[16:20], net.IPv4bcast.To4()) // broadcast
	// Compute IP header checksum
	binary.BigEndian.PutUint16(ipHeader[10:12], ipChecksum(ipHeader))

	var packet []byte
	packet = append(packet, ipHeader...)
	packet = append(packet, udpHeader...)
	packet = append(packet, bootp...)
	return packet
}

// handleARP responds to ARP requests for the gateway IP.
func (ing *FDIngress) handleARP(frame []byte) {
	if len(frame) < 42 { // 14 eth + 28 ARP
		return
	}

	arp := frame[14:]
	opcode := binary.BigEndian.Uint16(arp[6:8])
	if opcode != 1 { // ARP request
		return
	}

	targetIP := net.IP(arp[24:28])
	if !targetIP.Equal(ing.cfg.GatewayIP) {
		return
	}

	senderMAC := net.HardwareAddr(arp[8:14])
	senderIP := net.IP(arp[14:18])

	// Learn guest MAC from ARP
	ing.guestMACMu.Lock()
	if ing.guestMAC == nil {
		ing.guestMAC = make(net.HardwareAddr, 6)
		copy(ing.guestMAC, senderMAC)
		log.Printf("netstack: learned guest MAC %s from ARP", senderMAC)
	}
	ing.guestMACMu.Unlock()

	// Build ARP reply
	reply := make([]byte, 28)
	binary.BigEndian.PutUint16(reply[0:2], 1)    // hardware type: Ethernet
	binary.BigEndian.PutUint16(reply[2:4], 0x0800) // protocol type: IPv4
	reply[4] = 6                                    // hardware size
	reply[5] = 4                                    // protocol size
	binary.BigEndian.PutUint16(reply[6:8], 2)    // opcode: reply
	copy(reply[8:14], ing.cfg.GatewayMAC)        // sender MAC (us)
	copy(reply[14:18], ing.cfg.GatewayIP.To4())  // sender IP (us)
	copy(reply[18:24], senderMAC)                // target MAC
	copy(reply[24:28], senderIP)                 // target IP

	// Wrap in Ethernet frame
	ethFrame := make([]byte, 14+28)
	copy(ethFrame[0:6], senderMAC)
	copy(ethFrame[6:12], ing.cfg.GatewayMAC)
	binary.BigEndian.PutUint16(ethFrame[12:14], 0x0806) // ARP
	copy(ethFrame[14:], reply)

	if _, err := ing.file.Write(ethFrame); err != nil {
		log.Printf("netstack: ARP reply write error: %v", err)
	}
}

// sendEthernetFrame wraps an IP packet in an Ethernet frame and writes to FD.
func (ing *FDIngress) sendEthernetFrame(
	dstMAC net.HardwareAddr, _ []byte, ipPacket []byte,
) {
	frame := make([]byte, 14+len(ipPacket))
	copy(frame[0:6], dstMAC)
	copy(frame[6:12], ing.cfg.GatewayMAC)
	binary.BigEndian.PutUint16(frame[12:14], 0x0800) // IPv4
	copy(frame[14:], ipPacket)

	if _, err := ing.file.Write(frame); err != nil {
		log.Printf("netstack: DHCP reply write error: %v", err)
	}
}

// ipChecksum computes the IPv4 header checksum.
func ipChecksum(header []byte) uint16 {
	var sum uint32
	for i := 0; i < len(header)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i : i+2]))
	}
	if len(header)%2 == 1 {
		sum += uint32(header[len(header)-1]) << 8
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

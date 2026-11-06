//go:build unix

package main

import (
	"encoding/binary"
	"log"
	"net"
)

// handleDHCP answers a DHCP DISCOVER or REQUEST from the guest VM through
// the shared core (dhcpReply), framing the reply as a UDP broadcast from the
// gateway.
func (ing *FDIngress) handleDHCP(frame []byte) {
	if len(frame) < 14+28 {
		return
	}
	ipHeader := frame[14:]
	bootpStart := int(ipHeader[0]&0x0f)*4 + 8
	if len(ipHeader) < bootpStart {
		return
	}
	rep, ok := dhcpReply(ipHeader[bootpStart:], ing.lease)
	if !ok {
		return
	}
	log.Printf("netstack: %s", dhcpLogLine(rep))
	ing.sendEthernetFrame(net.HardwareAddr(rep[28:34]), nil, ing.wrapDHCPReply(rep))
}

// wrapDHCPReply wraps a BOOTP reply in UDP (67 to 68) and an IPv4 broadcast
// from the gateway.
func (ing *FDIngress) wrapDHCPReply(bootp []byte) []byte {
	gwIP := ing.cfg.GatewayIP.To4()

	udpLen := 8 + len(bootp)
	udpHeader := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHeader[0:2], 67)
	binary.BigEndian.PutUint16(udpHeader[2:4], 68)
	udpLenU16 := uint16(udpLen)
	binary.BigEndian.PutUint16(udpHeader[4:6], udpLenU16)

	ipLen := 20 + udpLen
	ipHeader := make([]byte, 20)
	ipHeader[0] = 0x45 // version + IHL
	binary.BigEndian.PutUint16(ipHeader[2:4], uint16(ipLen))
	ipHeader[8] = 64 // TTL
	ipHeader[9] = 17 // UDP
	copy(ipHeader[12:16], gwIP)
	copy(ipHeader[16:20], net.IPv4bcast.To4()) // broadcast
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
	binary.BigEndian.PutUint16(reply[0:2], 1)      // hardware type: Ethernet
	binary.BigEndian.PutUint16(reply[2:4], 0x0800) // protocol type: IPv4
	reply[4] = 6                                   // hardware size
	reply[5] = 4                                   // protocol size
	binary.BigEndian.PutUint16(reply[6:8], 2)      // opcode: reply
	copy(reply[8:14], ing.cfg.GatewayMAC)          // sender MAC (us)
	copy(reply[14:18], ing.cfg.GatewayIP.To4())    // sender IP (us)
	copy(reply[18:24], senderMAC)                  // target MAC
	copy(reply[24:28], senderIP)                   // target IP

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

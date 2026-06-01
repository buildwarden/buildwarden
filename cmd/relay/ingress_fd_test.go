package main

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFDIngressARP(t *testing.T) {
	// Create a socketpair to simulate the VM <-> host link.
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	vmFD := fds[0]   // "VM side" — we write frames here
	hostFD := fds[1] // "host side" — FDIngress reads from here

	defer unix.Close(vmFD)

	vmFile := os.NewFile(uintptr(vmFD), "vm-end")
	defer vmFile.Close()

	ing, err := NewFDIngress(FDIngressConfig{
		FD:         hostFD,
		GatewayIP:  net.IPv4(10, 100, 0, 1),
		GuestIP:    net.IPv4(10, 100, 0, 2),
		SubnetMask: net.CIDRMask(30, 32),
		GatewayMAC: net.HardwareAddr{0x02, 0xBD, 0x00, 0x00, 0x00, 0x01},
	})
	if err != nil {
		t.Fatalf("NewFDIngress: %v", err)
	}
	defer ing.Close()

	// Send an ARP request from "VM" asking for the gateway MAC.
	guestMAC := net.HardwareAddr{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}
	arpReq := buildARPRequest(guestMAC, net.IPv4(10, 100, 0, 2), net.IPv4(10, 100, 0, 1))
	if _, err := vmFile.Write(arpReq); err != nil {
		t.Fatalf("write ARP request: %v", err)
	}

	// Read the ARP reply from the host side.
	buf := make([]byte, 1600)
	_ = vmFile.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := vmFile.Read(buf)
	if err != nil {
		t.Fatalf("read ARP reply: %v", err)
	}

	reply := buf[:n]
	if n < 42 {
		t.Fatalf("reply too short: %d bytes", n)
	}

	// Verify it's an ARP reply (ethertype 0x0806, opcode 2)
	etherType := binary.BigEndian.Uint16(reply[12:14])
	if etherType != 0x0806 {
		t.Fatalf("expected ARP ethertype 0x0806, got 0x%04x", etherType)
	}

	opcode := binary.BigEndian.Uint16(reply[14+6 : 14+8])
	if opcode != 2 {
		t.Fatalf("expected ARP reply (opcode 2), got %d", opcode)
	}

	// Verify the sender MAC in the ARP reply is our gateway MAC
	senderMAC := net.HardwareAddr(reply[14+8 : 14+14])
	expectedMAC := net.HardwareAddr{0x02, 0xBD, 0x00, 0x00, 0x00, 0x01}
	if !macEqual(senderMAC, expectedMAC) {
		t.Fatalf("ARP reply sender MAC = %s, want %s", senderMAC, expectedMAC)
	}

	// Verify the sender IP is the gateway
	senderIP := net.IP(reply[14+14 : 14+18])
	if !senderIP.Equal(net.IPv4(10, 100, 0, 1)) {
		t.Fatalf("ARP reply sender IP = %s, want 10.100.0.1", senderIP)
	}
}

func TestFDIngressDHCP(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	vmFD := fds[0]
	hostFD := fds[1]

	defer unix.Close(vmFD)

	vmFile := os.NewFile(uintptr(vmFD), "vm-end")
	defer vmFile.Close()

	ing, err := NewFDIngress(FDIngressConfig{
		FD:         hostFD,
		GatewayIP:  net.IPv4(10, 100, 0, 1),
		GuestIP:    net.IPv4(10, 100, 0, 2),
		SubnetMask: net.CIDRMask(30, 32),
		GatewayMAC: net.HardwareAddr{0x02, 0xBD, 0x00, 0x00, 0x00, 0x01},
	})
	if err != nil {
		t.Fatalf("NewFDIngress: %v", err)
	}
	defer ing.Close()

	// Send a DHCP Discover from the VM
	guestMAC := net.HardwareAddr{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}
	discover := buildDHCPDiscover(guestMAC)
	if _, err := vmFile.Write(discover); err != nil {
		t.Fatalf("write DHCP discover: %v", err)
	}

	// Read frames until we get the DHCP Offer (skip kickstart ARPs)
	buf := make([]byte, 1600)
	var reply []byte
	for i := 0; i < 20; i++ {
		_ = vmFile.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := vmFile.Read(buf)
		if err != nil {
			t.Fatalf("read: %v (after %d frames)", err, i)
		}
		frame := buf[:n]
		if n >= 14 && binary.BigEndian.Uint16(frame[12:14]) == 0x0800 {
			reply = make([]byte, n)
			copy(reply, frame)
			break
		}
	}
	if reply == nil {
		t.Fatal("no IPv4 frame received")
	}

	ipStart := 14
	protocol := reply[ipStart+9]
	if protocol != 17 { // UDP
		t.Fatalf("expected UDP (17), got %d", protocol)
	}

	ihl := int(reply[ipStart]&0x0f) * 4
	srcPort := binary.BigEndian.Uint16(reply[ipStart+ihl : ipStart+ihl+2])
	dstPort := binary.BigEndian.Uint16(reply[ipStart+ihl+2 : ipStart+ihl+4])
	if srcPort != 67 || dstPort != 68 {
		t.Fatalf("expected ports 67->68, got %d->%d", srcPort, dstPort)
	}

	// Check yiaddr (offered IP) in the BOOTP payload
	bootpStart := ipStart + ihl + 8
	yiaddr := net.IP(reply[bootpStart+16 : bootpStart+20])
	if !yiaddr.Equal(net.IPv4(10, 100, 0, 2)) {
		t.Fatalf("DHCP offer yiaddr = %s, want 10.100.0.2", yiaddr)
	}
}

func TestFDIngressTCPListener(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	vmFD := fds[0]
	hostFD := fds[1]

	defer unix.Close(vmFD)

	ing, err := NewFDIngress(FDIngressConfig{
		FD:         hostFD,
		GatewayIP:  net.IPv4(10, 100, 0, 1),
		GuestIP:    net.IPv4(10, 100, 0, 2),
		SubnetMask: net.CIDRMask(30, 32),
		GatewayMAC: net.HardwareAddr{0x02, 0xBD, 0x00, 0x00, 0x00, 0x01},
	})
	if err != nil {
		t.Fatalf("NewFDIngress: %v", err)
	}
	defer ing.Close()

	// Verify we can create TCP listeners on the virtual stack.
	ln, err := ing.ListenTCP(80)
	if err != nil {
		t.Fatalf("ListenTCP(80): %v", err)
	}
	defer ln.Close()

	ln443, err := ing.ListenTCP(443)
	if err != nil {
		t.Fatalf("ListenTCP(443): %v", err)
	}
	defer ln443.Close()

	// Verify we can create a UDP listener.
	udp, err := ing.ListenUDP(53)
	if err != nil {
		t.Fatalf("ListenUDP(53): %v", err)
	}
	defer udp.Close()
}

// --- test helpers ---

func buildARPRequest(srcMAC net.HardwareAddr, srcIP, targetIP net.IP) []byte {
	frame := make([]byte, 42)
	// Ethernet header
	copy(frame[0:6], net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) // dst: broadcast
	copy(frame[6:12], srcMAC)
	binary.BigEndian.PutUint16(frame[12:14], 0x0806) // ARP

	// ARP payload
	arp := frame[14:]
	binary.BigEndian.PutUint16(arp[0:2], 1)      // hardware type: Ethernet
	binary.BigEndian.PutUint16(arp[2:4], 0x0800)  // protocol type: IPv4
	arp[4] = 6                                     // hardware size
	arp[5] = 4                                     // protocol size
	binary.BigEndian.PutUint16(arp[6:8], 1)       // opcode: request
	copy(arp[8:14], srcMAC)                        // sender MAC
	copy(arp[14:18], srcIP.To4())                  // sender IP
	// target MAC = 0 (unknown)
	copy(arp[24:28], targetIP.To4()) // target IP
	return frame
}

func buildDHCPDiscover(clientMAC net.HardwareAddr) []byte {
	// BOOTP fixed header (236 bytes) + options, pre-padded to 300.
	bootp := make([]byte, 300)
	bootp[0] = 1 // BOOTREQUEST
	bootp[1] = 1 // Ethernet
	bootp[2] = 6 // HW addr len
	bootp[4] = 0x12
	bootp[5] = 0x34
	bootp[6] = 0x56
	bootp[7] = 0x78
	copy(bootp[28:34], clientMAC)

	// DHCP magic cookie + options at offset 236
	copy(bootp[236:], []byte{
		0x63, 0x82, 0x53, 0x63, // magic cookie
		53, 1, 1, // message type: discover
		255, // end
	})

	// UDP header (src 68, dst 67)
	udpLen := 8 + len(bootp)
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], 68)
	binary.BigEndian.PutUint16(udpHdr[2:4], 67)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(udpLen))

	// IPv4 header
	ipLen := 20 + udpLen
	ipHdr := make([]byte, 20)
	ipHdr[0] = 0x45
	binary.BigEndian.PutUint16(ipHdr[2:4], uint16(ipLen))
	ipHdr[8] = 64  // TTL
	ipHdr[9] = 17  // UDP
	copy(ipHdr[12:16], net.IPv4zero.To4())   // src: 0.0.0.0
	copy(ipHdr[16:20], net.IPv4bcast.To4())  // dst: 255.255.255.255
	binary.BigEndian.PutUint16(ipHdr[10:12], ipChecksum(ipHdr))

	// Ethernet frame
	frame := make([]byte, 14+ipLen)
	copy(frame[0:6], net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(frame[6:12], clientMAC)
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	copy(frame[14:], ipHdr)
	copy(frame[14+20:], udpHdr)
	copy(frame[14+20+8:], bootp)
	return frame
}

func macEqual(a, b net.HardwareAddr) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestIsDHCPFrame(t *testing.T) {
	guestMAC := net.HardwareAddr{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}
	frame := buildDHCPDiscover(guestMAC)
	t.Logf("frame length: %d", len(frame))
	t.Logf("ethertype: 0x%04x", binary.BigEndian.Uint16(frame[12:14]))
	t.Logf("ip protocol: %d", frame[14+9])
	ihl := int(frame[14]&0x0f) * 4
	t.Logf("ihl: %d", ihl)
	t.Logf("udp src port: %d", binary.BigEndian.Uint16(frame[14+ihl:14+ihl+2]))
	t.Logf("udp dst port: %d", binary.BigEndian.Uint16(frame[14+ihl+2:14+ihl+4]))

	if !isDHCPFrame(frame) {
		t.Fatal("isDHCPFrame returned false for a DHCP discover frame")
	}
}

//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
)

// probe sends the DISCOVER as a raw Ethernet/IPv4/UDP broadcast frame on -if
// and collects BOOTREPLYs seen on that interface until wait elapses.
func probe(payload, mac, xid []byte, wait time.Duration) ([][]byte, error) {
	if *ifName == "" {
		return nil, fmt.Errorf("-if is required on linux")
	}
	ifi, err := net.InterfaceByName(*ifName)
	if err != nil {
		return nil, err
	}
	const ethPAll = 0x0003
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(ethPAll)))
	if err != nil {
		return nil, fmt.Errorf("raw socket: %w", err)
	}
	defer syscall.Close(fd)
	sa := &syscall.SockaddrLinklayer{Protocol: htons(ethPAll), Ifindex: ifi.Index}
	if err := syscall.Bind(fd, sa); err != nil {
		return nil, fmt.Errorf("bind %s: %w", *ifName, err)
	}
	frame := frameFor(payload, mac)
	if err := syscall.Sendto(fd, frame, 0, sa); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}
	fmt.Printf("sent %d bytes on %s (src %s)\n", len(frame), *ifName, net.HardwareAddr(mac))
	tv := syscall.NsecToTimeval(int64(500 * time.Millisecond))
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		return nil, err
	}
	var out [][]byte
	buf := make([]byte, 2048)
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			continue
		}
		if p := bootpPayload(buf[:n]); p != nil && isReplyFor(p, xid) {
			out = append(out, append([]byte(nil), p...))
		}
	}
	return out, nil
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// frameFor wraps the BOOTP payload in Ethernet broadcast, IPv4 0.0.0.0 ->
// 255.255.255.255 and UDP 68 -> 67.
func frameFor(payload, mac []byte) []byte {
	udpLen := 8 + len(payload)
	ipLen := 20 + udpLen
	f := make([]byte, 14+ipLen)
	copy(f[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(f[6:12], mac)
	binary.BigEndian.PutUint16(f[12:14], 0x0800)
	ip := f[14:34]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(ipLen))
	ip[8] = 64
	ip[9] = 17
	copy(ip[16:20], []byte{255, 255, 255, 255})
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))
	udp := f[34:42]
	binary.BigEndian.PutUint16(udp[0:2], 68)
	binary.BigEndian.PutUint16(udp[2:4], 67)
	binary.BigEndian.PutUint16(udp[4:6], uint16(udpLen))
	copy(f[42:], payload)
	return f
}

func checksum(b []byte) uint16 {
	var s uint32
	for i := 0; i+1 < len(b); i += 2 {
		s += uint32(binary.BigEndian.Uint16(b[i : i+2]))
	}
	if len(b)%2 == 1 {
		s += uint32(b[len(b)-1]) << 8
	}
	for s>>16 != 0 {
		s = s&0xffff + s>>16
	}
	return ^uint16(s)
}

// bootpPayload returns the UDP payload of a frame that is IPv4/UDP to port 68.
func bootpPayload(f []byte) []byte {
	if len(f) < 42 || binary.BigEndian.Uint16(f[12:14]) != 0x0800 {
		return nil
	}
	ihl := int(f[14]&0x0f) * 4
	if f[14]>>4 != 4 || f[14+9] != 17 || len(f) < 14+ihl+8 {
		return nil
	}
	udp := f[14+ihl:]
	if binary.BigEndian.Uint16(udp[2:4]) != 68 {
		return nil
	}
	return udp[8:]
}

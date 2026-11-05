//go:build windows

package main

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"
)

// probe binds UDP :68 on -src (SO_REUSEADDR, since the DHCP Client service
// holds the port too), broadcasts the DISCOVER to 255.255.255.255:67 from that
// address, and collects BOOTREPLYs until wait elapses.
func probe(payload, mac, xid []byte, wait time.Duration) ([][]byte, error) {
	if *srcIP == "" {
		return nil, fmt.Errorf("-src is required on windows")
	}
	lc := net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			h := syscall.Handle(fd)
			if e := syscall.SetsockoptInt(h, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); e != nil {
				serr = e
				return
			}
			serr = syscall.SetsockoptInt(h, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		})
		if err != nil {
			return err
		}
		return serr
	}}
	pc, err := lc.ListenPacket(context.Background(), "udp4", net.JoinHostPort(*srcIP, "68"))
	if err != nil {
		return nil, fmt.Errorf("bind %s:68: %w", *srcIP, err)
	}
	defer pc.Close()
	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: 67}
	if _, err := pc.WriteTo(payload, dst); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}
	fmt.Printf("sent %d bytes from %s:68 to 255.255.255.255:67\n", len(payload), *srcIP)
	var out [][]byte
	buf := make([]byte, 2048)
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		_ = pc.SetReadDeadline(deadline)
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			break
		}
		if isReplyFor(buf[:n], xid) {
			fmt.Printf("reply from %s\n", from)
			out = append(out, append([]byte(nil), buf[:n]...))
		}
	}
	return out, nil
}

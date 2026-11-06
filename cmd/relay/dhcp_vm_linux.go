//go:build linux

package main

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// openVMDHCPConn binds udp4 :67 to the build interface alone
// (SO_BINDTODEVICE), so a DHCP request arriving on any other interface (the
// relay VM's upstream) never reaches the responder, and enables broadcast for
// the reply to a client that has no address yet.
func openVMDHCPConn(ifName string) (packetConn, error) {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			if serr = unix.BindToDevice(int(fd), ifName); serr != nil {
				return
			}
			s := int(fd)
			const sol = unix.SOL_SOCKET
			if serr = unix.SetsockoptInt(s, sol, unix.SO_BROADCAST, 1); serr != nil {
				return
			}
			serr = unix.SetsockoptInt(s, sol, unix.SO_REUSEADDR, 1)
		})
		if err != nil {
			return err
		}
		return serr
	}}
	return lc.ListenPacket(context.Background(), "udp4", ":67")
}

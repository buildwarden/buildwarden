package vz

import (
	"fmt"
	"net"
)

// UserspaceBridge provides the network layer between the host (relay) and
// the VM. It uses VZFileHandleNetworkDeviceAttachment to get raw Ethernet
// frames from the VM via a Unix datagram socket pair, then presents them
// through a userspace TCP/IP stack (gVisor netstack) as standard Go
// net.Listener and net.PacketConn interfaces.
//
// This enables the relay to run on the host while the VM's only network
// path is through the bridge — achieving topological isolation without
// iptables.
type UserspaceBridge struct {
	RelayIP net.IP
	VMIP    net.IP

	// socketFD is the host side of the VZFileHandleNetworkDevice socket pair.
	socketFD int

	// TODO: gVisor netstack stack instance
	// stack *stack.Stack
}

// NewUserspaceBridge creates a network bridge with the given relay and VM IPs.
// It creates a Unix datagram socket pair — one end goes to the VM's
// VZFileHandleNetworkDeviceAttachment, the other is used by the userspace
// stack on the host.
func NewUserspaceBridge(relayIP, vmIP string) (*UserspaceBridge, error) {
	relay := net.ParseIP(relayIP)
	vm := net.ParseIP(vmIP)
	if relay == nil || vm == nil {
		return nil, fmt.Errorf("invalid IP addresses: relay=%s vm=%s", relayIP, vmIP)
	}

	// TODO: Create Unix datagram socket pair via syscall.Socketpair
	// TODO: Initialize gVisor netstack with the host-side FD
	// TODO: Configure static ARP entry for VM IP -> VM MAC
	// TODO: Set up IP address on the stack interface

	return &UserspaceBridge{
		RelayIP:  relay,
		VMIP:     vm,
		socketFD: -1,
	}, nil
}

// ListenTCP returns a net.Listener on the relay IP for the given port.
// The listener is backed by the userspace TCP/IP stack.
func (b *UserspaceBridge) ListenTCP(port uint16) (net.Listener, error) {
	// TODO: Create TCP listener on the netstack
	_ = port
	return nil, fmt.Errorf("netstack TCP listener not yet implemented")
}

// ListenUDP returns a net.PacketConn on the relay IP for the given port.
// Used for DNS (port 53).
func (b *UserspaceBridge) ListenUDP(port uint16) (net.PacketConn, error) {
	// TODO: Create UDP listener on the netstack
	_ = port
	return nil, fmt.Errorf("netstack UDP listener not yet implemented")
}

// VMSocketFD returns the file descriptor that should be passed to
// VZFileHandleNetworkDeviceAttachment for the VM's network interface.
func (b *UserspaceBridge) VMSocketFD() int {
	return b.socketFD
}

// Close shuts down the userspace stack and closes the socket pair.
func (b *UserspaceBridge) Close() error {
	// TODO: Close netstack, close socket FDs
	return nil
}

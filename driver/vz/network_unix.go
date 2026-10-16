//go:build unix

package vz

import (
	"fmt"
	"net"
	"syscall"
)

// NewVirtualNetwork creates a Unix datagram socket pair for the isolated
// link between the build VM and relay VM. Each FD will be passed to a
// VZFileHandleNetworkDeviceAttachment.
func NewVirtualNetwork() (*VirtualNetwork, error) {
	// Create socket pair for the private link between relay and build VMs.
	// SOCK_DGRAM preserves message boundaries (each message = one Ethernet frame).
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("creating socket pair: %w", err)
	}

	return &VirtualNetwork{
		BuildSocketFD: fds[0],
		RelaySocketFD: fds[1],
		Subnet: NetworkSubnet{
			RelayIP: net.IPv4(10, 0, 0, 1),
			BuildIP: net.IPv4(10, 0, 0, 2),
			Netmask: net.CIDRMask(30, 32),
			Gateway: net.IPv4(10, 0, 0, 1),
		},
	}, nil
}

// Close releases the socket pair file descriptors.
func (vn *VirtualNetwork) Close() error {
	var firstErr error
	if err := syscall.Close(vn.BuildSocketFD); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := syscall.Close(vn.RelaySocketFD); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

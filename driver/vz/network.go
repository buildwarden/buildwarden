package vz

import (
	"fmt"
	"net"
	"syscall"
)

// VirtualNetwork represents an isolated virtual network connecting two VMs
// via VZFileHandleNetworkDeviceAttachment. Each VM gets one end of a Unix
// datagram socket pair — Ethernet frames flow directly between them with
// no external path.
//
// Topology:
//
//	Build VM (macOS) ←—virtio-net—→ [socket pair] ←—virtio-net—→ Relay VM (Alpine)
//	                                                              ↕ (second interface)
//	                                                           Host/Internet (NAT)
//
// The build VM has only one network interface (the private link to relay).
// The relay VM has two: the private link and a NAT interface for internet.
// Network isolation is topological — no iptables needed.
type VirtualNetwork struct {
	// BuildSocketFD is the file descriptor for the build VM's network device.
	BuildSocketFD int
	// RelaySocketFD is the file descriptor for the relay VM's private interface.
	RelaySocketFD int

	// Subnet holds the IP allocation for this network.
	Subnet NetworkSubnet
}

// NetworkSubnet defines the IP addressing for the isolated network.
type NetworkSubnet struct {
	RelayIP  net.IP
	BuildIP  net.IP
	Netmask  net.IPMask
	Gateway  net.IP // = RelayIP (relay is the gateway for build VM)
}

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

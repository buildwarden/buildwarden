package vz

import (
	"fmt"
	"net"
	"syscall"
)

// VirtualNetwork represents an isolated virtual network connecting the build
// VM to the host-side relay via VZFileHandleNetworkDeviceAttachment. The
// build VM gets one end of a Unix datagram socket pair as its sole NIC
// (no shared directories, no NAT, no other devices). The relay process
// reads from the other end using gvisor netstack.
//
// Topology:
//
//	Build VM (macOS) ←—virtio-net—→ [socket pair] ←—fd—→ Relay (host process)
//	                  (sole NIC)                          ↕ (native host networking)
//	                                                  Internet
//
// Isolation guarantees:
//   - Build VM has exactly one network interface (the socketpair)
//   - No virtio-fs, no shared directory, no clipboard, no NAT
//   - All communication (context, CA, artifacts, signals) goes through the
//     relay's HTTP API over the socketpair
//   - NAT guard: createMacOSVM refuses to combine socketpair + NAT
type VirtualNetwork struct {
	// BuildSocketFD is the file descriptor for the build VM's network device.
	BuildSocketFD int
	// RelaySocketFD is the file descriptor for the host relay process.
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

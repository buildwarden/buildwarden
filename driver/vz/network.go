package vz

import "net"

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

// NewVirtualNetwork and (*VirtualNetwork).Close are implemented per-platform:
// the Unix datagram socket pair backing the isolated link is a Unix-only
// mechanism (see network_unix.go). On Windows the vz driver is unavailable
// (Apple Virtualization.framework is macOS-only), so the Windows variant in
// network_windows.go returns an error.

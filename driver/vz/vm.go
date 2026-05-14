package vz

import "net"

// VMConfig holds the configuration for creating a macOS virtual machine.
type VMConfig struct {
	CPUs     int
	MemoryMB int

	// DiskImage is the path to the bootable macOS disk image.
	DiskImage string

	// SharedDirs are virtio-fs directories shared between host and guest.
	SharedDirs []SharedDir

	// Network configures the VM's network interface.
	Network NetworkConfig
}

// SharedDir represents a virtio-fs shared directory.
type SharedDir struct {
	// Tag is the virtio-fs mount tag. On macOS guests, the automount path
	// is derived from this tag.
	Tag string
	// HostPath is the absolute path on the host.
	HostPath string
	// ReadOnly restricts the share to read-only access.
	ReadOnly bool
}

// NetworkConfig holds network device configuration for the VM.
type NetworkConfig struct {
	// Mode determines how the VM is networked.
	// "filehandle" uses VZFileHandleNetworkDeviceAttachment (for isolation).
	// "nat" uses VZNATNetworkDeviceAttachment (for testing/development).
	Mode string

	// SocketFD is the VM-side file descriptor from the socket pair,
	// used when Mode is "filehandle".
	SocketFD int

	// MACAddress is the VM's MAC address (auto-generated if empty).
	MACAddress net.HardwareAddr
}

// VM represents a running macOS virtual machine.
type VM struct {
	config VMConfig
	// handle is the opaque pointer to VZVirtualMachine (set by cgo layer)
	handle uintptr
}

// NewVM creates a new virtual machine with the given configuration.
// The VM is not started — call Start() to boot it.
func NewVM(cfg VMConfig) (*VM, error) {
	// TODO: Call into cgo layer to create VZVirtualMachineConfiguration
	// with the specified CPU count, memory, disk, shared dirs, and network.
	return &VM{config: cfg}, nil
}

// Start boots the virtual machine.
func (vm *VM) Start() error {
	// TODO: Call VZVirtualMachine.start() via cgo
	return nil
}

// Stop gracefully shuts down the virtual machine.
func (vm *VM) Stop() error {
	// TODO: Call VZVirtualMachine.stop() via cgo
	return nil
}

// WaitForStop blocks until the VM exits.
func (vm *VM) WaitForStop() error {
	// TODO: Wait for VM state change to stopped/error
	return nil
}

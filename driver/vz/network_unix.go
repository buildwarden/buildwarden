//go:build unix

package vz

import (
	"fmt"
	"os"
	"syscall"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// NewVirtualNetwork creates a Unix datagram socket pair for the isolated
// link between the build VM and relay VM, addressed as link. Each FD will be
// passed to a VZFileHandleNetworkDeviceAttachment.
func NewVirtualNetwork(link buildlink.Link) (*VirtualNetwork, error) {
	// Create socket pair for the private link between relay and build VMs.
	// SOCK_DGRAM preserves message boundaries (each message = one Ethernet frame).
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("creating socket pair: %w", err)
	}

	return &VirtualNetwork{
		BuildSocketFD: fds[0],
		RelaySocketFD: fds[1],
		Subnet:        subnetOf(link),
	}, nil
}

// RelaySocket returns a duplicate of the relay end of the socket pair, for
// the relay process to inherit. The caller closes it once the process has
// started. The pair's own descriptors stay the VirtualNetwork's alone, for
// Close: wrapping one of them in an *os.File would close it a second time
// from the File's finalizer, after Close, by when the number may name an
// unrelated file (an output being written).
func (vn *VirtualNetwork) RelaySocket() (*os.File, error) {
	fd, err := syscall.Dup(vn.RelaySocketFD)
	if err != nil {
		return nil, fmt.Errorf("duplicating the relay socket: %w", err)
	}
	syscall.CloseOnExec(fd)
	return os.NewFile(uintptr(fd), "relay-socket"), nil
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

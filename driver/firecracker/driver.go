// Package firecracker is the build driver of Firecracker microVMs on a
// Linux KVM host (docs/design/firecracker-driver.md). It uses the qemu
// driver's two-VM topology: the relay VM boots the unchanged Alpine relay
// initramfs and is claimed over its upstream TAP; the build VM's only NIC is
// on a per-slot host bridge whose only other port is the relay. Slots are made
// once by `warden setup --driver firecracker`, so builds run unprivileged.
//
// Linux (x86_64) only: the build and process code is in *_linux.go, and other
// hosts get a stub whose Preflight and Run return a fixable error.
package firecracker

import (
	"fmt"
	"os"

	"github.com/buildwarden/buildwarden/driver/linuxguest"
)

// Driver is the firecracker driver: its Preflight and Run.
type Driver struct {
	// Verbose shows both VMs' serial consoles on stderr.
	Verbose bool
}

// New returns a Firecracker driver.
func New() *Driver { return &Driver{} }

func (d *Driver) status(msg string) {
	fmt.Fprintf(os.Stderr, "[warden] %s\n", msg)
}

// DefaultImage is the cloud image `warden image prepare --driver firecracker`
// converts when given no source: the one the driver is validated with.
const DefaultImage = "ubuntu:24.04"

// PrepareImage downloads the cloud image ref (default DefaultImage) if it is
// not cached, or takes a local image path, and converts it to the raw image
// builds copy, so the first build pays for neither.
func PrepareImage(ref string) (string, error) {
	if err := hostSupported(); err != nil {
		return "", err
	}
	if ref == "" {
		ref = DefaultImage
	}
	src, err := linuxguest.PrepareImage(ref)
	if err != nil {
		return "", err
	}
	return EnsureRaw(src)
}

// bootInOrder runs the relay step (boot the relay VM and claim it) and only
// once it has returned, with the relay serving the build link, the build VM
// step; a failed claim boots no build VM (BOOT-01's order, pinned by a test).
func bootInOrder(relayStep, buildStep func() error) error {
	if err := relayStep(); err != nil {
		return err
	}
	return buildStep()
}

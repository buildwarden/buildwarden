// Package firecracker implements driver.Driver with Firecracker microVMs on a
// Linux KVM host (docs/design/firecracker-driver.md). It uses the qemu
// driver's two-VM topology: the relay VM boots the unchanged Alpine relay
// initramfs and is claimed over its upstream TAP; the build VM's only NIC is
// on a per-slot host bridge whose only other port is the relay. Slots are made
// once by `warden setup --driver firecracker`, so builds run unprivileged.
//
// Linux (x86_64) only: the build and process code is in *_linux.go, and other
// hosts get a stub whose StartBuild returns a fixable error.
package firecracker

import (
	"context"
	"fmt"
	"os"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
)

// Driver implements driver.Driver using Firecracker.
type Driver struct {
	// Verbose shows both VMs' serial consoles on stderr.
	Verbose bool
}

// New returns a Firecracker driver.
func New() *Driver { return &Driver{} }

// Name implements driver.Driver; it returns "firecracker".
func (d *Driver) Name() string { return "firecracker" }

// Exec implements driver.Driver. The firecracker driver has no interactive
// mode, so it returns driver.ErrExecNotSupported.
func (d *Driver) Exec(context.Context, *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

// Close implements driver.Driver. Each build stops its own VMs, so the driver
// holds nothing to release.
func (d *Driver) Close() error { return nil }

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

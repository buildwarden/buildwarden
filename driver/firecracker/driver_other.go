//go:build !linux

package firecracker

import (
	"context"
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
)

func hostSupported() error { return unsupportedHost(runtime.GOOS, runtime.GOARCH) }

// Preflight is the firecracker driver's probe. Firecracker needs Linux KVM,
// so on this host it returns a fixable error.
func (d *Driver) Preflight(*driver.BuildRequest) error { return hostSupported() }

// Run is the firecracker driver's run. Firecracker needs Linux KVM, so on
// this host it returns a fixable error and reports nothing.
func (d *Driver) Run(context.Context, *driver.BuildRequest, *driver.Start,
	driver.Host) ([]string, error) {
	return nil, hostSupported()
}

// killVMMs has nothing to kill off Linux: no VMM ever ran here.
func killVMMs([]vmmRecord) error { return nil }

// tryLock finds every lock free off Linux: no build ever held one here.
func (realHost) tryLock(string) (func(), bool, error) { return func() {}, false, nil }

// lockAs finds every lock free off Linux, where setup refuses to run.
func (realHost) lockAs(string, string) (func(), bool, error) { return func() {}, false, nil }

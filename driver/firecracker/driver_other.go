//go:build !linux

package firecracker

import (
	"context"
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
)

func hostSupported() error { return unsupportedHost(runtime.GOOS, runtime.GOARCH) }

// StartBuild implements driver.Driver. Firecracker needs Linux KVM, so on
// this host it returns a fixable error.
func (d *Driver) StartBuild(context.Context, *driver.BuildRequest) (*driver.BuildResult, error) {
	return nil, hostSupported()
}

// killVMMs has nothing to kill off Linux: no VMM ever ran here.
func killVMMs([]vmmRecord) error { return nil }

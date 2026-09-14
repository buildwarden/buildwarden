//go:build windows

package hyperv

import (
	"context"
	"fmt"

	"github.com/buildwarden/buildwarden/driver"
)

// StartBuild is not implemented yet. Phase 0 lands the privilege foundation
// (capability detection, `warden hyperv doctor`, the dev-switch setup stub);
// the VM/boot/build path arrives in Phase 1.
func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
	return nil, fmt.Errorf("hyperv driver: build not implemented yet (Phase 1); run `warden hyperv doctor` to check host readiness")
}

// Exec is not supported by the Hyper-V driver.
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

// Close releases resources held by the driver. Nothing to release yet.
func (d *Driver) Close() error { return nil }

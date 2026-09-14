//go:build !windows

package hyperv

import (
	"context"
	"fmt"
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
)

// StartBuild is unavailable off Windows: Hyper-V is a Windows host feature.
func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
	return nil, fmt.Errorf("hyperv driver requires a Windows host (current OS: %s)", runtime.GOOS)
}

// Exec is unavailable off Windows.
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return fmt.Errorf("hyperv driver requires a Windows host (current OS: %s)", runtime.GOOS)
}

// Close is a no-op.
func (d *Driver) Close() error { return nil }

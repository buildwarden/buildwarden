//go:build !windows

package hyperv

import (
	"context"
	"fmt"
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
)

// errNotWindows is the hyperv driver's refusal off Windows: Hyper-V is a
// Windows host feature.
func errNotWindows() error {
	return fmt.Errorf("hyperv driver requires a Windows host (current OS: %s)",
		runtime.GOOS)
}

// Preflight is unavailable off Windows.
func (d *Driver) Preflight(*driver.BuildRequest) error { return errNotWindows() }

// Run is unavailable off Windows: it reports nothing.
func (d *Driver) Run(context.Context, *driver.BuildRequest, *driver.Start,
	driver.Host) ([]string, error) {
	return nil, errNotWindows()
}

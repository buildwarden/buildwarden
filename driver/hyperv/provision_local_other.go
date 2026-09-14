//go:build !windows

package hyperv

import (
	"context"
	"fmt"
	"runtime"
)

// unsupportedProvisioner satisfies Provisioner on non-Windows hosts. Every
// method fails clearly; callers (e.g. RunSetup) preflight Capabilities.Supported
// and error before reaching it.
type unsupportedProvisioner struct{}

func newLocalProvisioner(verbose bool) Provisioner { return unsupportedProvisioner{} }

func errUnsupported() error {
	return fmt.Errorf("hyperv provisioner requires a Windows host (current OS: %s)", runtime.GOOS)
}

func (unsupportedProvisioner) EnsureNetwork(context.Context, NetworkSpec) (*NetworkResources, error) {
	return nil, errUnsupported()
}
func (unsupportedProvisioner) TeardownNetwork(context.Context, string) error { return errUnsupported() }
func (unsupportedProvisioner) CreateVM(context.Context, VMSpec) (*VMHandle, error) {
	return nil, errUnsupported()
}
func (unsupportedProvisioner) StartVM(context.Context, string) error         { return errUnsupported() }
func (unsupportedProvisioner) StopVM(context.Context, string) error          { return errUnsupported() }
func (unsupportedProvisioner) RemoveVM(context.Context, string) error        { return errUnsupported() }
func (unsupportedProvisioner) CreateDiffDisk(context.Context, string, string) error {
	return errUnsupported()
}

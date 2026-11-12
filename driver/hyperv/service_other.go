//go:build !windows

package hyperv

import "context"

// The privileged service and its client are Windows-only (winio named pipes,
// Windows SCM). These stubs let the package cross-compile on non-Windows; the
// driver preflights Capabilities.Supported and errors long before any of them
// would be reached at runtime.

// RunService is unsupported off Windows.
func RunService(context.Context) error { return errUnsupported() }

// InstallService is unsupported off Windows.
func InstallService() error { return errUnsupported() }

// UninstallService is unsupported off Windows.
func UninstallService() error { return errUnsupported() }

// probeService finds no service off Windows.
func probeService() (reachable bool, stale string) { return false, "" }

// newClientProvisioner returns the unsupported provisioner off Windows.
func newClientProvisioner(bool) Provisioner { return unsupportedProvisioner{} }

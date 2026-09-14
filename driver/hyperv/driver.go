// Package hyperv implements a BuildWarden host driver backed by Windows
// Hyper-V. It is the production Windows-host path; the vz and qemu drivers
// target macOS and Linux hosts respectively.
//
// Unlike every other driver, the Hyper-V driver needs host-level OS privilege.
// See docs/design/hyperv-driver-plan.md ("Privilege & Elevation Model") for how
// privilege is detected per operation and acquired. Every privileged operation
// flows through the Provisioner interface (provision.go) so the same logic
// serves both the in-process path and the (later) privileged-service path, and
// so a third-party orchestrator reusing the relay can target the same boundary.
//
// The real implementation is Windows-only (driver_windows.go); other platforms
// get a stub (driver_stub.go) so the warden CLI still builds everywhere.
package hyperv

// DefaultDevSwitch is the fixed name of the durable, reused development vSwitch
// created once by `warden hyperv setup` (dev stub) and reused on every build to
// avoid per-build network standup (which requires full Administrator).
const DefaultDevSwitch = "warden-dev"

// Driver implements driver.Driver using Hyper-V as the backend. Windows only
// (Pro/Enterprise/Server with the Hyper-V role enabled).
type Driver struct {
	// Verbose enables VM console output.
	Verbose bool

	// Switch, when non-empty, names a pre-created durable/dev vSwitch to reuse
	// instead of standing up a network per build. Standing up a network
	// requires full Administrator; reusing a pre-created switch keeps the
	// per-build path within the Hyper-V Administrators tier (no elevation).
	// Populated from --switch / WARDEN_HYPERV_SWITCH.
	Switch string
}

// New returns a Hyper-V driver.
func New() *Driver { return &Driver{} }

// Name implements driver.Driver.
func (d *Driver) Name() string { return "hyperv" }

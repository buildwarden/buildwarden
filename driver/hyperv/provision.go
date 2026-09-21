package hyperv

import "context"

// Provisioner is the single boundary through which every privileged Hyper-V
// operation flows. It is the code-reuse spine of the driver: the same logic
// serves both the in-process path and the privileged-service path, with only
// the transport differing.
//
// Two implementations are planned (see the elevation model in
// docs/design/hyperv-driver-plan.md):
//
//   - localProvisioner  — in-process; calls hcsshim / PowerShell directly.
//     Used whenever the current process already holds the required privilege.
//   - clientProvisioner — marshals the identical calls over a named pipe to
//     the privileged service (Phase 4).
//
// The service host instantiates a localProvisioner and exposes it over the
// pipe, so localProvisioner is the single source of truth for all privileged
// logic. This same boundary is what a third-party Windows orchestrator reusing
// the relay would target.
//
// Methods are grouped by the privilege tier they require:
//   - EnsureNetwork / TeardownNetwork need full Administrator (host network
//     standup: New-VMSwitch, New-NetNat, New-NetIPAddress).
//   - The VM-lifecycle methods need only the Hyper-V Administrators group.
type Provisioner interface {
	// Full-Administrator operations (host network standup).
	EnsureNetwork(ctx context.Context, spec NetworkSpec) (*NetworkResources, error)
	TeardownNetwork(ctx context.Context, id string) error

	// Hyper-V Administrators operations (VM lifecycle).
	CreateVM(ctx context.Context, spec VMSpec) (*VMHandle, error)
	// CreateRelayVM creates the relay VM specifically. It differs from CreateVM
	// (the build VM) in ways that matter for the trust boundary: an unsigned
	// UKI booted with Secure Boot off, a per-build differencing overlay off the
	// static boot VHDX, both the Private build NIC and the NAT upstream NIC, and
	// COM1 wired to a host named pipe.
	CreateRelayVM(ctx context.Context, spec RelayVMSpec) (*VMHandle, error)
	StartVM(ctx context.Context, name string) error
	StopVM(ctx context.Context, name string) error
	RemoveVM(ctx context.Context, name string) error
	CreateDiffDisk(ctx context.Context, overlay, base string) error
}

// NetworkSpec describes the isolated build network to stand up (or reuse).
type NetworkSpec struct {
	// ID is the per-build identifier used to name ephemeral resources.
	ID string
	// SwitchName is the vSwitch to use. When it names a pre-created durable/dev
	// switch, the network is reused rather than created (no full-admin standup).
	SwitchName string
	// SwitchType is "Private" (VM-to-VM only) or "Internal". Private is
	// stronger: the build VM cannot reach the host network stack at all.
	SwitchType string
	// NATPrefix is the CIDR for the relay VM's controlled upstream egress
	// (e.g. 192.168.240.0/20). The build VM never gets a NAT route.
	NATPrefix string
	// HostIP is the host-side gateway address on the NAT switch.
	HostIP string
}

// NetworkResources identifies the network resources backing a build.
type NetworkResources struct {
	BuildSwitch string
	NATName     string
	// Reused is true when an existing durable/dev switch was reused rather than
	// created for this build.
	Reused bool
}

// VMSpec describes a VM to create.
type VMSpec struct {
	Name       string
	Generation int
	MemoryMB   int
	CPUs       int
	SwitchName string
	VHDXPath   string
	// SeedISO / SeedVHDX carry per-build provisioning media (cloud-init for
	// Linux guests, unattend media for Windows guests).
	SeedISO  string
	SeedVHDX string
	IsWindows bool
}

// VMHandle references a created VM for lifecycle operations.
type VMHandle struct {
	Name       string
	Generation int
}

// RelayVMSpec describes the relay VM to create. Unlike VMSpec (the build VM),
// the relay boots an unsigned UKI, so Secure Boot is off and no SecureBoot
// template is set; it boots a per-build differencing overlay off the static,
// read-only boot VHDX so the shared base is never written and concurrent relay
// VMs never collide; it has two NICs (NIC1/eth0 = the Private build link,
// NIC2/eth1 = the NAT upstream); and COM1 is wired to a host named pipe that
// carries init's readiness echo and the relay's stdout.
type RelayVMSpec struct {
	Name        string
	MemoryMB    int    // default 512
	CPUs        int    // default 2
	BootVHDX    string // static, read-only UKI boot disk (differencing parent)
	OverlayVHDX string // per-build differencing child created off BootVHDX
	BuildSwitch string // Private switch; NIC1 = build link (eth0)
	NATSwitch   string // Internal NAT switch; NIC2 = upstream (eth1)
	COMPipePath string // e.g. \\.\pipe\<name>, wired to COM1
}

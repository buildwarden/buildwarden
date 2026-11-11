package hyperv

import (
	"context"
	"net/netip"
	"strings"
)

// relayVMPrefix names every relay VM; StartRelay appends the build ID. The
// allocator reads the Notes of every VM with this prefix.
const relayVMPrefix = "warden-relay-"

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
	// VMState reports a VM's state ("Running", "Off", ...), or "" when no
	// such VM exists.
	VMState(ctx context.Context, name string) (string, error)
	// GuestKVP returns the key-value items the guest published through the
	// Data Exchange integration service (empty while the guest is not up).
	GuestKVP(ctx context.Context, name string) (map[string]string, error)
	// DisksInUse lists every disk file any VM on the host depends on: each
	// attached disk and all of its differencing parents.
	DisksInUse(ctx context.Context) ([]string, error)
	// ListVMs names the host's VMs whose names start with prefix.
	ListVMs(ctx context.Context, prefix string) ([]string, error)
	// RelayNotes maps each relay VM on the host (relayVMPrefix) to its Notes,
	// whichever process created it and whatever its state.
	RelayNotes(ctx context.Context) (map[string]string, error)
	// SwitchAdapters maps each of switches that has an adapter attached to
	// the VMs those adapters belong to, a VM once per adapter.
	SwitchAdapters(ctx context.Context, switches []string) (map[string][]string, error)
}

// parseRelayNotes reads RelayNotes' script output: one "<vm>\t<notes>" line
// per relay VM.
func parseRelayNotes(out string) map[string]string {
	notes := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		name, n, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if name = strings.TrimSpace(name); name != "" {
			notes[name] = strings.TrimSpace(n)
		}
	}
	return notes
}

// parseSwitchAdapters reads SwitchAdapters' script output: one
// "<switch>\t<vm>" line per attached adapter.
func parseSwitchAdapters(out string) map[string][]string {
	attached := map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		sw, vm, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if sw = strings.TrimSpace(sw); ok && sw != "" {
			attached[sw] = append(attached[sw], strings.TrimSpace(vm))
		}
	}
	return attached
}

// NetworkSpec describes the isolated build network to stand up (or reuse).
type NetworkSpec struct {
	// ID is the per-build identifier used to name ephemeral resources.
	ID string
	// SwitchName is the per-build Private switch EnsureNetwork creates; empty
	// creates none (setup, whose build switches are the slots).
	SwitchName string
	// SwitchType is "Private" (VM-to-VM only) or "Internal". Private is
	// stronger: the build VM cannot reach the host network stack at all.
	SwitchType string
	// NATPrefix is the CIDR for the relay VM's controlled upstream egress
	// (e.g. 192.168.240.0/20). The build VM never gets a NAT route.
	NATPrefix string
	// HostIP is the host-side gateway address on the NAT switch.
	HostIP string
	// NATName names the NAT (and its Internal switch) EnsureNetwork creates
	// when none qualifies; empty is <SwitchName>-nat.
	NATName string
	// DevNAT, when set, names the dev switch's NAT (<dev switch>-nat), which a
	// build uses whenever it exists, whatever prefix setup gave it; only when
	// it does not is a warden NAT owning NATPrefix reused or a new one made.
	DevNAT string
}

// NetworkResources identifies the network resources backing a build.
type NetworkResources struct {
	BuildSwitch string
	NATName     string
	// Reused is true when an existing durable/dev switch was reused rather than
	// created for this build.
	Reused bool
	// NATPrefix and HostIP are read back from the NAT: its internal prefix
	// and the host vNIC's address in it, the relay's prefix and gateway.
	NATPrefix netip.Prefix
	HostIP    netip.Addr
	// Slots are the durable network's build switches, in index order
	// (VerifyNetwork only).
	Slots []string
}

// VMSpec describes a VM to create.
type VMSpec struct {
	Name       string
	Generation int
	MemoryMB   int
	CPUs       int
	// SwitchName is the vSwitch for the VM's only NIC; empty means no network
	// adapter at all.
	SwitchName string
	VHDXPath   string
	// SeedISO / SeedVHDX carry per-build provisioning media (cloud-init for
	// Linux guests, unattend media for Windows guests).
	SeedISO   string
	SeedVHDX  string
	IsWindows bool
	// COMPipePath, when set, wires the build VM's COM1 to a host named pipe
	// (e.g. \\.\pipe\<name>). The guest bootstrap echoes its progress there so a
	// failed boot is observable from the host without mounting the guest disk.
	// Empty leaves COM1 unwired.
	COMPipePath string
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
// NIC2/eth1 = the NAT upstream); COM1 is wired to a host named pipe that
// carries init's readiness echo and the relay's stdout; and the per-build
// relay seed is attached as a DVD, the relay's boot channel.
type RelayVMSpec struct {
	Name        string
	MemoryMB    int    // default 512
	CPUs        int    // default 2
	BootVHDX    string // static, read-only UKI boot disk (differencing parent)
	OverlayVHDX string // per-build differencing child created off BootVHDX
	BuildSwitch string // Private switch; NIC1 = build link (eth0)
	NATSwitch   string // Internal NAT switch; NIC2 = upstream (eth1)
	COMPipePath string // e.g. \\.\pipe\<name>, wired to COM1
	// SeedISO is the relay seed (RelaySeedLabel), attached as a DVD.
	SeedISO string
	// Notes is written to the VM's Notes before it is started: the relay's
	// address and build switch, which the allocator of every warden process
	// reads back (relayNotes).
	Notes string
}

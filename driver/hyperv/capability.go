package hyperv

import "runtime"

// Op is a class of privileged Hyper-V operation, grouped by the privilege tier
// it requires. Privilege is resolved per operation, never by a single global
// "am I admin?" check (see Capabilities.Resolve).
type Op int

const (
	// OpNetworkStandup covers host network standup (New-VMSwitch, New-NetNat,
	// New-NetIPAddress) and requires full Administrator.
	OpNetworkStandup Op = iota
	// OpVMLifecycle covers VM lifecycle (New-VM, Start-VM, Stop-VM, Remove-VM,
	// adapters, differencing VHDX) and requires only the local Hyper-V
	// Administrators group.
	OpVMLifecycle
)

func (o Op) String() string {
	switch o {
	case OpNetworkStandup:
		return "network standup"
	case OpVMLifecycle:
		return "VM lifecycle"
	default:
		return "unknown op"
	}
}

// Outcome is how an Op will be satisfied given the current Capabilities.
type Outcome int

const (
	// OutcomeDirect: the process holds the privilege; perform in-process.
	OutcomeDirect Outcome = iota
	// OutcomeDelegate: delegate the op to the privileged service.
	OutcomeDelegate
	// OutcomeBlocked: neither available; needs one-time setup or elevation.
	OutcomeBlocked
)

func (o Outcome) String() string {
	switch o {
	case OutcomeDirect:
		return "direct"
	case OutcomeDelegate:
		return "delegate"
	case OutcomeBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

// Capabilities is a read-only snapshot of the current process's privilege and
// the host's Hyper-V state. Every field comes from a cheap, side-effect-free
// probe; see Detect.
type Capabilities struct {
	// Supported is false when not running on a Windows host.
	Supported bool
	// Platform is runtime.GOOS.
	Platform string
	// TokenScope names which token the elevation/membership checks reflect.
	// The gateway spawns warden non-elevated, so this typically reads
	// "current process (non-elevated)"; an elevated verdict only appears when
	// warden is launched from an elevated terminal (a different token).
	TokenScope string

	HypervisorPresent bool // Win32_ComputerSystem.HypervisorPresent
	HyperVFeature     bool // Hyper-V management stack present (vmms service)
	Elevated          bool // full Administrator (network standup ops)
	HyperVAdmin       bool // Hyper-V Administrators group per THIS process's token (VM lifecycle ops)

	// HyperVAdminByAccount is true when the user account is a member of the
	// Hyper-V Administrators group per the persistent group membership, even
	// though the current process token does not carry it. A true value here
	// with HyperVAdmin false is the stale-token signature: the group was joined
	// after this process's logon token was issued. A long-running parent (the
	// gateway) keeps its token for life, so its spawned warden inherits the old
	// token until the parent is restarted in a session that has the group.
	HyperVAdminByAccount bool

	// DevSwitch is the durable/dev switch name that was probed ("" if none).
	DevSwitch        string
	DevSwitchPresent bool

	// ServiceReachable is true when the privileged service pipe answers.
	ServiceReachable bool

	// Errors collects non-fatal probe failures, surfaced by Doctor.
	Errors []string
}

// Resolve reports how op will be satisfied. It is per-operation on purpose: a
// Hyper-V Administrators member is not "elevated" yet can run every VM op
// directly, so a global boolean would wrongly send them down the network-standup
// path and fail on New-VMSwitch.
func (c Capabilities) Resolve(op Op) Outcome {
	var hasPrivilege bool
	switch op {
	case OpNetworkStandup:
		hasPrivilege = c.Elevated
	case OpVMLifecycle:
		hasPrivilege = c.Elevated || c.HyperVAdmin
	}
	switch {
	case hasPrivilege:
		return OutcomeDirect
	case c.ServiceReachable:
		return OutcomeDelegate
	default:
		return OutcomeBlocked
	}
}

// NetworkStrategy is how the isolated build network will be provisioned given
// the current privilege and environment.
//
// Policy: when the process CAN stand up a network (full Administrator, or the
// privileged service running as LocalSystem), prefer a FRESH ephemeral per-build
// network. That is the cleanest isolation and avoids the collisions and
// side-effects of a shared, long-lived switch. The durable named switch created
// by `warden hyperv setup` is a LOWER-PRIVILEGE fallback: it exists so a
// non-elevated Hyper-V Administrators member (which cannot stand up a network)
// still has an isolated path to reuse.
type NetworkStrategy int

const (
	// NetBlocked: no network path is available.
	NetBlocked NetworkStrategy = iota
	// NetEphemeralPerBuild: full Administrator; create a fresh isolated network
	// per build and tear it down after (cleanest isolation, no reuse).
	NetEphemeralPerBuild
	// NetDelegateService: delegate standup to the privileged service, which
	// runs elevated and likewise creates a fresh per-build network.
	NetDelegateService
	// NetReuseDurable: non-elevated Hyper-V Administrators; reuse the durable
	// named switch created by an earlier elevated `warden hyperv setup`.
	NetReuseDurable
)

func (s NetworkStrategy) String() string {
	switch s {
	case NetEphemeralPerBuild:
		return "ephemeral per-build"
	case NetDelegateService:
		return "delegate to service"
	case NetReuseDurable:
		return "reuse durable switch"
	default:
		return "blocked"
	}
}

// NetworkStrategy resolves how the build network will be provisioned. An
// elevated process never reuses the durable named switch: it stands up a fresh
// isolated network per build instead.
func (c Capabilities) NetworkStrategy() NetworkStrategy {
	switch {
	case c.Elevated:
		return NetEphemeralPerBuild
	case c.ServiceReachable:
		return NetDelegateService
	case c.HyperVAdmin && c.DevSwitchPresent:
		return NetReuseDurable
	default:
		return NetBlocked
	}
}

// Ready reports whether the driver can run builds given these capabilities and,
// if not, a short reason. A build needs a hypervisor, the Hyper-V stack, VM
// lifecycle not blocked, and a usable network strategy.
func (c Capabilities) Ready() (ready bool, reason string) {
	switch {
	case !c.Supported:
		return false, "hyperv driver requires a Windows host"
	case !c.HypervisorPresent:
		return false, "no hypervisor present (enable Hyper-V and reboot)"
	case !c.HyperVFeature:
		return false, "Hyper-V management stack (vmms) not found"
	case c.Resolve(OpVMLifecycle) == OutcomeBlocked:
		return false, "VM lifecycle blocked: join Hyper-V Administrators, launch elevated, or install the service"
	case c.NetworkStrategy() == NetBlocked:
		return false, "no network path: run `warden hyperv setup` (elevated) for a reusable switch, or run this build from an elevated shell for a fresh per-build network"
	default:
		return true, ""
	}
}

// Detect probes the current process and host. switchName is the durable/dev
// switch to check for (from --switch / WARDEN_HYPERV_SWITCH); pass "" to skip
// that probe.
func Detect(switchName string) Capabilities {
	c := Capabilities{Platform: runtime.GOOS, DevSwitch: switchName}
	detectPlatform(&c)
	return c
}

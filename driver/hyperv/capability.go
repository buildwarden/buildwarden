package hyperv

import (
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
)

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
	// OutcomeDirect means the process holds the privilege; perform in-process.
	OutcomeDirect Outcome = iota
	// OutcomeDelegate means delegate the op to the privileged service.
	OutcomeDelegate
	// OutcomeBlocked means neither is available; the op needs one-time setup or
	// elevation.
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
	// Hyper-V Administrators group per THIS process's token (VM lifecycle ops)
	HyperVAdmin bool

	// HyperVAdminByAccount is true when the user account is a member of the
	// Hyper-V Administrators group per the persistent group membership, even
	// though the current process token does not carry it. A true value here
	// with HyperVAdmin false is the stale-token signature: the group was joined
	// after this process's logon token was issued. A long-running parent (the
	// gateway) keeps its token for life, so its spawned warden inherits the old
	// token until the parent is restarted in a session that has the group.
	HyperVAdminByAccount bool

	// DevSwitch names the durable network that was probed ("" if none).
	// DevSwitchPresent is true when its build switches (<DevSwitch>-<i>)
	// exist; DevSwitchLegacy when only the single shared <DevSwitch> switch
	// of an older setup does, which durable builds no longer use.
	DevSwitch        string
	DevSwitchPresent bool
	DevSwitchLegacy  bool

	// ServiceReachable is true when the privileged service pipe answers a
	// ping at this warden's protocol version.
	ServiceReachable bool
	// ServiceStale says why a privileged service whose pipe answers cannot
	// serve this warden (another protocol version, or a failed ping); "" when
	// none answers or it is reachable. A non-elevated process with a stale
	// service is refused, naming the reinstall, and never takes the durable
	// path as if no service were installed (GLOBAL-11).
	ServiceStale string
	// ServiceInstalled is true when the service control manager has the
	// privileged service although its pipe does not open, and ServiceState
	// is its state then ("stopped", "running", ...), so doctor names starting
	// it rather than installing it.
	ServiceInstalled bool
	ServiceState     string
	// ServiceUnread is why the service control manager's answer could not be
	// read, "" when it was. ServiceInstalled is then true only when the SCM
	// found the name (it denied access), and neither installing nor starting
	// is named as certain: a failed read is never "not installed".
	ServiceUnread string

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
// by `warden setup --driver hyperv` is a LOWER-PRIVILEGE fallback: it exists so a
// non-elevated Hyper-V Administrators member (which cannot stand up a network)
// still has an isolated path to reuse.
type NetworkStrategy int

const (
	// NetBlocked means no network path is available.
	NetBlocked NetworkStrategy = iota
	// NetEphemeralPerBuild means full Administrator: create a fresh isolated
	// network per build and tear it down after (cleanest isolation, no reuse).
	NetEphemeralPerBuild
	// NetDelegateService means delegate standup to the privileged service,
	// which runs elevated and likewise creates a fresh per-build network.
	NetDelegateService
	// NetReuseDurable means non-elevated Hyper-V Administrators: reuse the
	// durable named switch created by an earlier elevated
	// `warden setup --driver hyperv`.
	NetReuseDurable
)

func (s NetworkStrategy) String() string {
	switch s {
	case NetEphemeralPerBuild:
		return "ephemeral per-build"
	case NetDelegateService:
		return "delegate to service"
	case NetReuseDurable:
		return "durable build switches"
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
	case c.ServiceStale != "":
		return NetBlocked
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
	case c.serviceBlocks():
		return false, "the privileged service " + c.ServiceStale +
			"; reinstall it: " + serviceReinstallFix
	case c.Resolve(OpVMLifecycle) == OutcomeBlocked:
		return false, "VM lifecycle blocked: join Hyper-V Administrators, " +
			"launch elevated, or " + c.serviceRemedy()
	case c.NetworkStrategy() == NetBlocked && c.HyperVAdmin && c.DevSwitchLegacy:
		return false, "the durable network has no build switches: run `warden setup " +
			"--driver hyperv` (elevated) to create the build switches"
	case c.NetworkStrategy() == NetBlocked:
		return false, "no network path: run `warden setup --driver hyperv` " +
			"(elevated) for a reusable switch, run this build from an " +
			"elevated shell for a fresh per-build network, or " + c.serviceRemedy()
	default:
		return true, ""
	}
}

// serviceDown is the fix for a service whose pipe does not open but which
// the SCM has, or may have: start (or restart) an installed one, check one
// whose state could not be read. "" when none is installed or it answers.
func (c Capabilities) serviceDown() string {
	switch {
	case c.ServiceReachable || c.ServiceStale != "":
		return ""
	case c.ServiceUnread != "" && c.ServiceInstalled:
		return serviceCheckInstalledFix
	case c.ServiceUnread != "":
		return serviceCheckFix
	case !c.ServiceInstalled:
		return ""
	case c.ServiceState == serviceRunning:
		return serviceRestartFix
	default:
		return serviceStartFix
	}
}

// serviceRemedy is the service remedy a blocked path names: check a service
// whose state could not be read, start (or restart) the installed service,
// else install it.
func (c Capabilities) serviceRemedy() string {
	fix := c.serviceDown()
	switch {
	case fix == "":
		return "install the network service (one-time, elevated)"
	case c.ServiceUnread != "":
		return "check the privileged service, whose state could not be read: " + fix
	default:
		return "start the installed privileged service: " + fix
	}
}

// serviceBlocks reports whether a stale privileged service blocks builds: it
// does whenever the process is not elevated, the only case that would use it.
func (c Capabilities) serviceBlocks() bool {
	return !c.Elevated && c.ServiceStale != ""
}

// buildRefusal is the error a build returns when Ready is false, nil when it
// is true: a Fixable whose fix is the reinstall for a stale service, else one
// pointing at doctor for each capability's fix.
func (c Capabilities) buildRefusal() error {
	ready, reason := c.Ready()
	switch {
	case ready:
		return nil
	case c.serviceBlocks():
		return driver.Fixable(serviceReinstallFix,
			"hyperv build: the privileged service %s", c.ServiceStale)
	default:
		return driver.Fixable(
			"see `warden doctor --driver hyperv` for each capability and its fix",
			"hyperv build: %s", reason)
	}
}

// Detect probes the current process and host. switchName is the durable/dev
// switch to check for (from --network / [vm] network); pass "" to skip
// that probe.
func Detect(switchName string) Capabilities {
	c := Capabilities{Platform: runtime.GOOS, DevSwitch: switchName}
	detectPlatform(&c)
	return c
}

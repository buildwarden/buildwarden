//go:build windows

package hyperv

import (
	"context"
	"fmt"
	"strings"
)

// localProvisioner performs privileged Hyper-V operations in-process via
// PowerShell. It is used whenever the current process already holds the
// required privilege (full Administrator for network standup, Hyper-V
// Administrators for VM lifecycle). The privileged service (Phase 4) will host
// this same type behind a named pipe, so this is the single source of truth for
// all privileged logic.
type localProvisioner struct {
	verbose bool
}

func newLocalProvisioner(verbose bool) Provisioner {
	return &localProvisioner{verbose: verbose}
}

// EnsureNetwork idempotently stands up (or verifies) the isolated build network:
//
//   - a Private vSwitch (spec.SwitchName): the build VM's sole NIC attaches
//     here, and Private means it cannot reach the host network stack at all.
//   - an Internal NAT vSwitch (<SwitchName>-nat) + host gateway IP + a NAT rule:
//     this is the RELAY VM's controlled upstream egress ONLY. The build VM is
//     never attached to it, so it never gets a direct route to the internet.
//
// It is safe to call repeatedly: existing resources are verified rather than
// recreated, and a pre-existing NAT on the same prefix is treated as a
// collision and refused rather than clobbered.
func (p *localProvisioner) EnsureNetwork(ctx context.Context, spec NetworkSpec) (*NetworkResources, error) {
	if spec.SwitchName == "" {
		return nil, fmt.Errorf("EnsureNetwork: empty switch name")
	}
	natName := spec.SwitchName + "-nat"
	prefixLen, err := cidrPrefixLen(spec.NATPrefix)
	if err != nil {
		return nil, err
	}

	name := psEscapeSingle(spec.SwitchName)
	nat := psEscapeSingle(natName)
	prefix := psEscapeSingle(spec.NATPrefix)
	hostIP := psEscapeSingle(spec.HostIP)

	// One script so the whole standup is atomic-ish and reports a single error.
	// $ErrorActionPreference=Stop turns cmdlet failures into catchable errors.
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'

# Build (Private) switch: relay <-> build only, no host path.
$build = Get-VMSwitch -Name '%[1]s' -ErrorAction SilentlyContinue
if (-not $build) {
    New-VMSwitch -Name '%[1]s' -SwitchType Private | Out-Null
} elseif ($build.SwitchType -ne 'Private') {
    throw "switch '%[1]s' exists but is $($build.SwitchType); expected Private"
}

# NAT (Internal) switch: relay upstream egress only.
$natSw = Get-VMSwitch -Name '%[2]s' -ErrorAction SilentlyContinue
if (-not $natSw) {
    New-VMSwitch -Name '%[2]s' -SwitchType Internal | Out-Null
} elseif ($natSw.SwitchType -ne 'Internal') {
    throw "switch '%[2]s' exists but is $($natSw.SwitchType); expected Internal"
}

# Host gateway IP on the NAT switch's host vNIC.
$alias = "vEthernet (%[2]s)"
if (-not (Get-NetIPAddress -InterfaceAlias $alias -IPAddress '%[3]s' -ErrorAction SilentlyContinue)) {
    New-NetIPAddress -IPAddress '%[3]s' -PrefixLength %[4]d -InterfaceAlias $alias | Out-Null
}

# NAT rule for the relay's egress, with collision detection: refuse to clobber
# an existing NAT on the same prefix (WSL2 / Docker Desktop / Default Switch).
if (-not (Get-NetNat -Name '%[2]s' -ErrorAction SilentlyContinue)) {
    $clash = Get-NetNat -ErrorAction SilentlyContinue | Where-Object { $_.InternalIPInterfaceAddressPrefix -eq '%[5]s' }
    if ($clash) {
        throw "a NAT for %[5]s already exists (name $($clash.Name)); refusing to clobber - pick a different prefix"
    }
    New-NetNat -Name '%[2]s' -InternalIPInterfaceAddressPrefix '%[5]s' | Out-Null
}
'OK'
`, name, nat, hostIP, prefixLen, prefix)

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("EnsureNetwork: %w", err)
	}
	if !strings.Contains(out, "OK") {
		return nil, fmt.Errorf("EnsureNetwork: unexpected output: %q", strings.TrimSpace(out))
	}
	return &NetworkResources{BuildSwitch: spec.SwitchName, NATName: natName}, nil
}

// TeardownNetwork removes the NAT rule and both switches for the given base
// name. Best-effort: missing resources are not an error (idempotent cleanup).
func (p *localProvisioner) TeardownNetwork(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("TeardownNetwork: empty name")
	}
	name := psEscapeSingle(id)
	nat := psEscapeSingle(id + "-nat")
	script := fmt.Sprintf(`
Remove-NetNat -Name '%[2]s' -Confirm:$false -ErrorAction SilentlyContinue
Remove-VMSwitch -Name '%[2]s' -Force -ErrorAction SilentlyContinue
Remove-VMSwitch -Name '%[1]s' -Force -ErrorAction SilentlyContinue
'OK'
`, name, nat)
	if _, err := runPS(script); err != nil {
		return fmt.Errorf("TeardownNetwork: %w", err)
	}
	return nil
}

// VerifyNetwork re-asserts that the durable network is still safe to reuse:
// the build switch is still Private (isolation intact) and it exists. Callers
// run this before every build rather than trusting that persistence equals
// correctness (a crash or manual edit can silently weaken isolation).
func (p *localProvisioner) VerifyNetwork(ctx context.Context, switchName string) error {
	name := psEscapeSingle(switchName)
	out, err := runPS(fmt.Sprintf(
		`$s = Get-VMSwitch -Name '%[1]s' -ErrorAction SilentlyContinue; if (-not $s) { 'missing' } else { $s.SwitchType }`,
		name))
	if err != nil {
		return fmt.Errorf("VerifyNetwork: %w", err)
	}
	switch strings.TrimSpace(out) {
	case "Private":
		return nil
	case "missing":
		return fmt.Errorf("build switch %q is missing; run `warden hyperv setup`", switchName)
	default:
		return fmt.Errorf("build switch %q is %s, expected Private (isolation compromised)", switchName, strings.TrimSpace(out))
	}
}

// --- VM lifecycle (Hyper-V Administrators tier) -----------------------------
// Implemented in the next Phase 0 increment (paired with relay-VM boot). The
// signatures satisfy Provisioner now so the setup path can land first.

func (p *localProvisioner) CreateVM(ctx context.Context, spec VMSpec) (*VMHandle, error) {
	return nil, fmt.Errorf("hyperv: CreateVM not implemented yet (next Phase 0 increment)")
}

func (p *localProvisioner) StartVM(ctx context.Context, name string) error {
	return fmt.Errorf("hyperv: StartVM not implemented yet (next Phase 0 increment)")
}

func (p *localProvisioner) StopVM(ctx context.Context, name string) error {
	return fmt.Errorf("hyperv: StopVM not implemented yet (next Phase 0 increment)")
}

func (p *localProvisioner) RemoveVM(ctx context.Context, name string) error {
	return fmt.Errorf("hyperv: RemoveVM not implemented yet (next Phase 0 increment)")
}

func (p *localProvisioner) CreateDiffDisk(ctx context.Context, overlay, base string) error {
	return fmt.Errorf("hyperv: CreateDiffDisk not implemented yet (next Phase 0 increment)")
}

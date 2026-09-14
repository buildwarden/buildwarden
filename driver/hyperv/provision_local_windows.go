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

// CreateVM creates a Generation-2 VM attached to spec.SwitchName. With an empty
// VHDXPath it is created diskless (-NoVHD), which is enough to validate the
// create/attach/remove path; a real build passes the overlay VHDX and seed
// media. SecureBoot template is chosen per guest family.
func (p *localProvisioner) CreateVM(ctx context.Context, spec VMSpec) (*VMHandle, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("CreateVM: empty name")
	}
	gen := spec.Generation
	if gen == 0 {
		gen = 2
	}
	mem := spec.MemoryMB
	if mem == 0 {
		mem = 2048
	}
	cpus := spec.CPUs
	if cpus == 0 {
		cpus = 2
	}
	name := psEscapeSingle(spec.Name)

	var b strings.Builder
	b.WriteString("$ErrorActionPreference='Stop'\n")
	if spec.VHDXPath != "" {
		fmt.Fprintf(&b, "New-VM -Name '%s' -Generation %d -MemoryStartupBytes %dMB -VHDPath '%s'",
			name, gen, mem, psEscapeSingle(spec.VHDXPath))
	} else {
		fmt.Fprintf(&b, "New-VM -Name '%s' -Generation %d -MemoryStartupBytes %dMB -NoVHD", name, gen, mem)
	}
	if spec.SwitchName != "" {
		fmt.Fprintf(&b, " -SwitchName '%s'", psEscapeSingle(spec.SwitchName))
	}
	b.WriteString(" | Out-Null\n")
	fmt.Fprintf(&b, "Set-VMProcessor -VMName '%s' -Count %d\n", name, cpus)
	if spec.SeedVHDX != "" {
		fmt.Fprintf(&b, "Add-VMHardDiskDrive -VMName '%s' -Path '%s'\n", name, psEscapeSingle(spec.SeedVHDX))
	}
	if spec.SeedISO != "" {
		fmt.Fprintf(&b, "Add-VMDvdDrive -VMName '%s' -Path '%s'\n", name, psEscapeSingle(spec.SeedISO))
	}
	if gen == 2 {
		tmpl := "MicrosoftUEFICertificateAuthority" // Linux guests
		if spec.IsWindows {
			tmpl = "MicrosoftWindows"
		}
		fmt.Fprintf(&b, "Set-VMFirmware -VMName '%s' -SecureBootTemplate '%s'\n", name, tmpl)
	}
	b.WriteString("'OK'\n")

	if _, err := runPS(b.String()); err != nil {
		return nil, fmt.Errorf("CreateVM %q: %w", spec.Name, err)
	}
	return &VMHandle{Name: spec.Name, Generation: gen}, nil
}

func (p *localProvisioner) StartVM(ctx context.Context, name string) error {
	if _, err := runPS(fmt.Sprintf("Start-VM -Name '%s'", psEscapeSingle(name))); err != nil {
		return fmt.Errorf("StartVM %q: %w", name, err)
	}
	return nil
}

func (p *localProvisioner) StopVM(ctx context.Context, name string) error {
	if _, err := runPS(fmt.Sprintf("Stop-VM -Name '%s' -Force -TurnOff", psEscapeSingle(name))); err != nil {
		return fmt.Errorf("StopVM %q: %w", name, err)
	}
	return nil
}

// RemoveVM force-stops (best-effort) then deletes the VM. Idempotent-ish: a
// missing VM is treated as already removed.
func (p *localProvisioner) RemoveVM(ctx context.Context, name string) error {
	n := psEscapeSingle(name)
	script := fmt.Sprintf(`
$vm = Get-VM -Name '%[1]s' -ErrorAction SilentlyContinue
if (-not $vm) { 'gone'; return }
Stop-VM -Name '%[1]s' -Force -TurnOff -ErrorAction SilentlyContinue
Remove-VM -Name '%[1]s' -Force
'OK'
`, n)
	if _, err := runPS(script); err != nil {
		return fmt.Errorf("RemoveVM %q: %w", name, err)
	}
	return nil
}

// CreateDiffDisk creates a differencing VHDX (the Hyper-V equivalent of a QCOW2
// overlay), so the read-only base image is preserved and each build writes to
// its own thin overlay.
func (p *localProvisioner) CreateDiffDisk(ctx context.Context, overlay, base string) error {
	script := fmt.Sprintf("New-VHD -Path '%s' -ParentPath '%s' -Differencing | Out-Null; 'OK'",
		psEscapeSingle(overlay), psEscapeSingle(base))
	if _, err := runPS(script); err != nil {
		return fmt.Errorf("CreateDiffDisk %q: %w", overlay, err)
	}
	return nil
}

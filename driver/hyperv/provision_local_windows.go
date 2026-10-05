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
//     This is the isolation boundary and is ALWAYS created fresh per build.
//   - an Internal NAT vSwitch + host gateway IP + a NAT rule: this is the RELAY
//     VM's controlled upstream egress ONLY. The build VM is never attached to
//     it, so it never gets a direct route to the internet.
//
// The NAT egress is shared plumbing, not the isolation boundary, so when a
// warden NAT already owns spec.NATPrefix (e.g. a durable `warden hyperv setup`
// switch, or a concurrent build), EnsureNetwork REUSES it rather than colliding
// on the host gateway IP. Only when no NAT owns the prefix does it stand one up
// per build (named <SwitchName>-nat). The actual NAT switch chosen is returned
// in NetworkResources.NATName (Reused reflects which path was taken), and the
// caller attaches the relay's upstream NIC to that name. It is safe to call
// repeatedly: existing resources are verified rather than recreated.
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

# Build (Private) switch: relay <-> build only, no host path. ALWAYS per-build:
# this is the isolation boundary, so it is never shared between builds.
$build = Get-VMSwitch -Name '%[1]s' -ErrorAction SilentlyContinue
if (-not $build) {
    New-VMSwitch -Name '%[1]s' -SwitchType Private | Out-Null
} elseif ($build.SwitchType -ne 'Private') {
    throw "switch '%[1]s' exists but is $($build.SwitchType); expected Private"
}

# NAT egress: the relay's controlled upstream. Reuse an existing warden NAT that
# already owns this prefix (a durable 'warden hyperv setup' switch, or a
# concurrent build) rather than colliding on the host gateway IP; the NAT subnet
# is shared egress plumbing, not the isolation boundary (that is the Private
# switch above). Only stand up a per-build NAT when none owns the prefix.
$reused = 0
$existingNat = Get-NetNat -ErrorAction SilentlyContinue | Where-Object { $_.InternalIPInterfaceAddressPrefix -eq '%[5]s' }
if ($existingNat) {
    $natName = $existingNat.Name
    $reused = 1
    $natSw = Get-VMSwitch -Name $natName -ErrorAction SilentlyContinue
    if (-not $natSw) {
        throw "a NAT '$natName' owns %[5]s but its vSwitch is missing; remove the stale NAT (elevated: Remove-NetNat -Name $natName) or re-run 'warden hyperv setup'"
    }
} else {
    $natName = '%[2]s'
    $natSw = Get-VMSwitch -Name $natName -ErrorAction SilentlyContinue
    if (-not $natSw) {
        New-VMSwitch -Name $natName -SwitchType Internal | Out-Null
    } elseif ($natSw.SwitchType -ne 'Internal') {
        throw "switch '$natName' exists but is $($natSw.SwitchType); expected Internal"
    }
}

# Host gateway IP on the NAT switch's host vNIC (idempotent; scoped to the alias
# so a reused NAT that already has it is left untouched).
$alias = "vEthernet ($natName)"
if (-not (Get-NetIPAddress -InterfaceAlias $alias -IPAddress '%[3]s' -ErrorAction SilentlyContinue)) {
    New-NetIPAddress -IPAddress '%[3]s' -PrefixLength %[4]d -InterfaceAlias $alias | Out-Null
}

# NAT rule for the relay's egress (only when we created the NAT; a reused one
# already has it).
if ($reused -eq 0 -and -not (Get-NetNat -Name $natName -ErrorAction SilentlyContinue)) {
    New-NetNat -Name $natName -InternalIPInterfaceAddressPrefix '%[5]s' | Out-Null
}

# Firewall: allow the relay VM (on this NAT subnet) to reach the host-side
# collector + config responder. The NAT vEthernet sits in the Public profile
# with inbound blocked by default, so without this the relay's boot-time config
# fetch and its output streaming are silently dropped. Scoped to the host IP,
# the NAT subnet, and only the two relay-host ports, so nothing else on the host
# is exposed. Keyed to the chosen NAT so reuse and fresh converge on one rule.
$fwName = "$natName-relay-ingress"
if (-not (Get-NetFirewallRule -DisplayName $fwName -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName $fwName -Direction Inbound -Action Allow -Protocol TCP -LocalPort %[6]d,%[7]d -LocalAddress '%[3]s' -RemoteAddress '%[5]s' | Out-Null
}
"NAT=$natName REUSED=$reused"
`, name, nat, hostIP, prefixLen, prefix, defaultConfigPort, defaultCollectorPort)

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("EnsureNetwork: %w", err)
	}
	chosenNAT, reused := parseEnsureNetworkOutput(out)
	if chosenNAT == "" {
		return nil, fmt.Errorf("EnsureNetwork: could not parse NAT switch from output: %q", strings.TrimSpace(out))
	}
	return &NetworkResources{BuildSwitch: spec.SwitchName, NATName: chosenNAT, Reused: reused}, nil
}

// parseEnsureNetworkOutput extracts the chosen NAT switch name and reuse flag
// from the EnsureNetwork script's trailing "NAT=<name> REUSED=<0|1>" line.
func parseEnsureNetworkOutput(out string) (nat string, reused bool) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "NAT=") {
			continue
		}
		for _, tok := range strings.Fields(line) {
			switch {
			case strings.HasPrefix(tok, "NAT="):
				nat = strings.TrimPrefix(tok, "NAT=")
			case tok == "REUSED=1":
				reused = true
			}
		}
	}
	return nat, reused
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
Remove-NetFirewallRule -DisplayName '%[2]s-relay-ingress' -ErrorAction SilentlyContinue
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
	// Ephemeral build VM: no automatic checkpoints (they spawn an .avhdx layer
	// on top of the overlay, complicating teardown and disk inspection). Matches
	// the relay VM's config.
	fmt.Fprintf(&b, "Set-VM -Name '%s' -AutomaticCheckpointsEnabled $false -CheckpointType Disabled\n", name)
	fmt.Fprintf(&b, "Set-VMProcessor -VMName '%s' -Count %d\n", name, cpus)
	if spec.SeedVHDX != "" {
		fmt.Fprintf(&b, "Add-VMHardDiskDrive -VMName '%s' -Path '%s'\n", name, psEscapeSingle(spec.SeedVHDX))
	}
	if spec.SeedISO != "" {
		fmt.Fprintf(&b, "Add-VMDvdDrive -VMName '%s' -Path '%s'\n", name, psEscapeSingle(spec.SeedISO))
	}
	// COM1 -> host named pipe (Gen2 supports Set-VMComPort): the guest bootstrap
	// echoes progress here for host-visible diagnostics without mounting the disk.
	if spec.COMPipePath != "" {
		fmt.Fprintf(&b, "Set-VMComPort -VMName '%s' -Number 1 -Path '%s'\n", name, psEscapeSingle(spec.COMPipePath))
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

// CreateRelayVM creates the Gen2 relay VM: a per-build differencing overlay off
// the static UKI boot VHDX, Secure Boot off (the UKI is unsigned), both the
// Private build NIC and the NAT upstream NIC, and COM1 on a host named pipe. The
// PowerShell is assembled by buildRelayVMScript (pure, unit-tested); this method
// only runs it. See that builder for the rationale behind each difference from
// CreateVM.
func (p *localProvisioner) CreateRelayVM(ctx context.Context, spec RelayVMSpec) (*VMHandle, error) {
	script, err := buildRelayVMScript(spec)
	if err != nil {
		return nil, err
	}
	if _, err := runPS(script); err != nil {
		return nil, fmt.Errorf("CreateRelayVM %q: %w", spec.Name, err)
	}
	return &VMHandle{Name: spec.Name, Generation: 2}, nil
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

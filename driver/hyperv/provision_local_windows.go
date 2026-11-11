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
// The NAT egress is shared plumbing, not the isolation boundary, and WinNAT
// supports one NAT per host in practice, so the NAT is chosen from what the
// host has, in a fixed order (Decision 5): the dev switch's NAT
// (spec.DevNAT) when setup made one, whatever its prefix; else a warden NAT
// that owns spec.NATPrefix (a concurrent build's); else the NAT named
// spec.NATName (default <SwitchName>-nat) if it exists, on whatever prefix;
// else a new one by that name on spec.NATPrefix with spec.HostIP. A reused
// NAT is never re-addressed: its prefix and the host vNIC's address in it are
// read back and returned, and a host vNIC with no address in its prefix is
// refused. It is safe to call repeatedly: existing resources are verified
// rather than recreated.
func (p *localProvisioner) EnsureNetwork(
	ctx context.Context, spec NetworkSpec,
) (*NetworkResources, error) {
	script, err := ensureNetworkScript(spec)
	if err != nil {
		return nil, err
	}

	out, err := runPS(script)
	if err != nil {
		return nil, fmt.Errorf("EnsureNetwork: %w", err)
	}
	res, err := parseNetReadback(out).ensureResources(spec.SwitchName)
	if err != nil {
		return nil, fmt.Errorf("EnsureNetwork: %w", err)
	}
	return res, nil
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

// VerifyNetwork re-asserts that the durable network named network is still
// safe to use, before every build rather than trusting that persistence
// equals correctness (a crash or manual edit can silently weaken isolation):
// its NAT (<network>-nat) and that NAT's vSwitch exist, the host vNIC holds
// an address in the NAT's prefix, and its slot switches (<network>-<i>)
// exist and are Private. It returns the NAT's prefix and host IP read back,
// and the slots in index order.
func (p *localProvisioner) VerifyNetwork(
	ctx context.Context, network string,
) (*NetworkResources, error) {
	out, err := runPS(netProbeScript(network, ""))
	if err != nil {
		return nil, fmt.Errorf("VerifyNetwork: %w", err)
	}
	return parseNetReadback(out).durableResources(network)
}

// netProbeScript reads, without changing anything, the NAT a build on the
// durable network named network uses (<network>-nat, or with otherPrefix set
// a warden NAT owning that prefix when there is none, as EnsureNetwork
// chooses) and network's slot switches, as parseNetReadback lines.
func netProbeScript(network, otherPrefix string) string {
	return fmt.Sprintf(`
$natName = $null
if (Get-NetNat -Name '%[1]s-nat' -ErrorAction SilentlyContinue) { $natName = '%[1]s-nat' }
elseif ('%[2]s') {
    $e = Get-NetNat -ErrorAction SilentlyContinue |
        Where-Object { $_.InternalIPInterfaceAddressPrefix -eq '%[2]s' } | Select-Object -First 1
    if ($e) { $natName = $e.Name }
}
if ($natName) {
    "NAT=$natName"
    "PREFIX=$((Get-NetNat -Name $natName).InternalIPInterfaceAddressPrefix)"
    if (-not (Get-VMSwitch -Name $natName -ErrorAction SilentlyContinue)) { 'NATSWITCH=missing' }
    $alias = "vEthernet ($natName)"
Get-NetIPAddress -InterfaceAlias $alias -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        ForEach-Object { "HOSTIP=$($_.IPAddress)" }
}
Get-VMSwitch -Name '%[1]s-*' -ErrorAction SilentlyContinue |
    ForEach-Object { "SLOT=$($_.Name)`+"`t"+`$($_.SwitchType)" }
`, psEscapeSingle(network), psEscapeSingle(otherPrefix))
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
		fmt.Fprintf(&b, "New-VM -Name '%s' -Generation %d "+
			"-MemoryStartupBytes %dMB -VHDPath '%s'",
			name, gen, mem, psEscapeSingle(spec.VHDXPath))
	} else {
		fmt.Fprintf(&b, "New-VM -Name '%s' -Generation %d "+
			"-MemoryStartupBytes %dMB -NoVHD", name, gen, mem)
	}
	if spec.SwitchName != "" {
		fmt.Fprintf(&b, " -SwitchName '%s'", psEscapeSingle(spec.SwitchName))
	}
	b.WriteString(" | Out-Null\n")
	// No switch means no network at all (image prepare): New-VM always adds an
	// adapter, so remove it rather than leave it disconnected.
	if spec.SwitchName == "" {
		fmt.Fprintf(&b,
			"Get-VMNetworkAdapter -VMName '%s' | Remove-VMNetworkAdapter\n", name)
	}
	// Ephemeral build VM: no automatic checkpoints (they spawn an .avhdx layer
	// on top of the overlay, complicating teardown and disk inspection). Matches
	// the relay VM's config.
	fmt.Fprintf(&b, "Set-VM -Name '%s' -AutomaticCheckpointsEnabled $false "+
		"-CheckpointType Disabled\n", name)
	fmt.Fprintf(&b, "Set-VMProcessor -VMName '%s' -Count %d\n", name, cpus)
	if spec.SeedVHDX != "" {
		fmt.Fprintf(&b, "Add-VMHardDiskDrive -VMName '%s' -Path '%s'\n",
			name, psEscapeSingle(spec.SeedVHDX))
	}
	if spec.SeedISO != "" {
		fmt.Fprintf(&b, "Add-VMDvdDrive -VMName '%s' -Path '%s'\n",
			name, psEscapeSingle(spec.SeedISO))
	}
	// COM1 -> host named pipe (Gen2 supports Set-VMComPort): the guest bootstrap
	// echoes progress here for host-visible diagnostics without mounting the disk.
	if spec.COMPipePath != "" {
		fmt.Fprintf(&b, "Set-VMComPort -VMName '%s' -Number 1 -Path '%s'\n",
			name, psEscapeSingle(spec.COMPipePath))
	}
	if gen == 2 {
		tmpl := "MicrosoftUEFICertificateAuthority" // Linux guests
		if spec.IsWindows {
			tmpl = "MicrosoftWindows"
		}
		fmt.Fprintf(&b, "Set-VMFirmware -VMName '%s' -SecureBootTemplate '%s'\n",
			name, tmpl)
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
func (p *localProvisioner) CreateRelayVM(
	ctx context.Context, spec RelayVMSpec,
) (*VMHandle, error) {
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
	if _, err := runPS(
		fmt.Sprintf("Start-VM -Name '%s'", psEscapeSingle(name))); err != nil {
		return fmt.Errorf("StartVM %q: %w", name, err)
	}
	return nil
}

func (p *localProvisioner) StopVM(ctx context.Context, name string) error {
	if _, err := runPS(fmt.Sprintf(
		"Stop-VM -Name '%s' -Force -TurnOff", psEscapeSingle(name))); err != nil {
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

// VMState reports the VM's state, or "" when it does not exist.
func (p *localProvisioner) VMState(ctx context.Context, name string) (string, error) {
	out, err := runPS(fmt.Sprintf(
		"$vm = Get-VM -Name '%s' -ErrorAction SilentlyContinue; "+
			"if ($vm) { [string]$vm.State } else { 'missing' }", psEscapeSingle(name)))
	if err != nil {
		return "", fmt.Errorf("VMState %q: %w", name, err)
	}
	state := strings.TrimSpace(out)
	if state == "missing" {
		return "", nil
	}
	return state, nil
}

// GuestKVP reads the guest-published key-value items over WMI (the
// root\virtualization\v2 Msvm_KvpExchangeComponent of the VM). Hyper-V
// Administrators may read it; no elevation is needed.
func (p *localProvisioner) GuestKVP(
	ctx context.Context, name string,
) (map[string]string, error) {
	out, err := runPS(fmt.Sprintf(`
$cs = Get-CimInstance -Namespace root\virtualization\v2 -ClassName Msvm_ComputerSystem `+
		`-Filter "ElementName='%s'"
if (-not $cs) { return }
$kvp = Get-CimAssociatedInstance -InputObject $cs -ResultClassName Msvm_KvpExchangeComponent
foreach ($item in $kvp.GuestExchangeItems) {
  $x = [xml]$item
  $n = ($x.INSTANCE.PROPERTY | Where-Object NAME -eq 'Name').VALUE
  $d = ($x.INSTANCE.PROPERTY | Where-Object NAME -eq 'Data').VALUE
  "$n=$d"
}`, psEscapeSingle(name)))
	if err != nil {
		return nil, fmt.Errorf("GuestKVP %q: %w", name, err)
	}
	return parseKVPLines(out), nil
}

// DisksInUse walks every VM's attached disks up their differencing chains.
func (p *localProvisioner) DisksInUse(ctx context.Context) ([]string, error) {
	out, err := runPS(`
foreach ($d in (Get-VM | Get-VMHardDiskDrive)) {
  $p = $d.Path
  while ($p) {
    $p
    $p = (Get-VHD -Path $p -ErrorAction SilentlyContinue).ParentPath
  }
}`)
	if err != nil {
		return nil, fmt.Errorf("DisksInUse: %w", err)
	}
	return nonEmptyLines(out), nil
}

// ListVMs names the host's VMs whose names start with prefix.
func (p *localProvisioner) ListVMs(ctx context.Context, prefix string) ([]string, error) {
	out, err := runPS(fmt.Sprintf(
		"Get-VM -Name '%s*' -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }",
		psEscapeSingle(prefix)))
	if err != nil {
		return nil, fmt.Errorf("ListVMs: %w", err)
	}
	return nonEmptyLines(out), nil
}

// RelayNotes reads the Notes of every relay VM, one line each (Notes the
// user edited across lines are joined, so a line is always one VM).
func (p *localProvisioner) RelayNotes(ctx context.Context) (map[string]string, error) {
	out, err := runPS(fmt.Sprintf("Get-VM -Name '%s*' -ErrorAction SilentlyContinue | "+
		"ForEach-Object { $_.Name + \"`t\" + ($_.Notes -replace '[\\r\\n]+', ' ') }",
		psEscapeSingle(relayVMPrefix)))
	if err != nil {
		return nil, fmt.Errorf("RelayNotes: %w", err)
	}
	return parseRelayNotes(out), nil
}

// SwitchAdapters lists every VM adapter on the host (Get-VMNetworkAdapter
// -All) attached to one of switches.
func (p *localProvisioner) SwitchAdapters(
	ctx context.Context, switches []string,
) (map[string][]string, error) {
	if len(switches) == 0 {
		return map[string][]string{}, nil
	}
	quoted := make([]string, len(switches))
	for i, sw := range switches {
		quoted[i] = "'" + psEscapeSingle(sw) + "'"
	}
	out, err := runPS(fmt.Sprintf("$sw = @(%s)\n"+
		"Get-VMNetworkAdapter -All | Where-Object { $sw -contains $_.SwitchName } | "+
		"ForEach-Object { $_.SwitchName + \"`t\" + $_.VMName }",
		strings.Join(quoted, ",")))
	if err != nil {
		return nil, fmt.Errorf("SwitchAdapters: %w", err)
	}
	return parseSwitchAdapters(out), nil
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

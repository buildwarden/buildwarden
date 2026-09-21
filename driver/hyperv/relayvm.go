package hyperv

import (
	"fmt"
	"strings"
)

// buildRelayVMScript assembles the PowerShell that creates the relay VM. It is a
// pure function (no I/O), so it is unit-testable on any host; CreateRelayVM runs
// the result via runPS on Windows.
//
// The relay VM is the trust boundary, so its creation is explicit and separate
// from the build VM's (CreateVM). Each difference is deliberate:
//   - A per-build DIFFERENCING overlay is created off the static, read-only UKI
//     boot VHDX. The shared base is never written, and concurrent relay VMs get
//     their own overlays instead of colliding on one disk.
//   - Secure Boot is OFF and NO SecureBootTemplate is set: the UKI is not
//     Microsoft-signed, so a template would refuse to boot it.
//   - Two NICs. NIC1 (eth0 in the guest) attaches to the Private build switch
//     via New-VM -SwitchName and is the build link; NIC2 (eth1) attaches to the
//     Internal NAT switch and is the relay's controlled upstream. The guest
//     maps them by attach order, the same convention the qemu/vz relay init
//     uses.
//   - COM1 is wired to a host named pipe: init echoes readiness there and the
//     relay's stdout/stderr are routed to it for host-visible diagnostics.
//
// The whole thing is one $ErrorActionPreference='Stop' script so a partial
// failure surfaces as a single error rather than a half-created VM without
// signal.
func buildRelayVMScript(spec RelayVMSpec) (string, error) {
	switch {
	case spec.Name == "":
		return "", fmt.Errorf("relay VM: empty name")
	case spec.BootVHDX == "":
		return "", fmt.Errorf("relay VM: empty boot VHDX path")
	case spec.OverlayVHDX == "":
		return "", fmt.Errorf("relay VM: empty overlay VHDX path")
	case spec.BuildSwitch == "":
		return "", fmt.Errorf("relay VM: empty build switch")
	case spec.NATSwitch == "":
		return "", fmt.Errorf("relay VM: empty NAT switch")
	case spec.COMPipePath == "":
		return "", fmt.Errorf("relay VM: empty COM1 pipe path")
	}

	mem := spec.MemoryMB
	if mem == 0 {
		mem = 512
	}
	cpus := spec.CPUs
	if cpus == 0 {
		cpus = 2
	}
	name := psEscapeSingle(spec.Name)

	var b strings.Builder
	b.WriteString("$ErrorActionPreference='Stop'\n")
	// Per-build differencing overlay off the static read-only boot VHDX.
	fmt.Fprintf(&b, "New-VHD -Path '%s' -ParentPath '%s' -Differencing | Out-Null\n",
		psEscapeSingle(spec.OverlayVHDX), psEscapeSingle(spec.BootVHDX))
	// Gen2 VM. NIC1 attaches to the Private build switch via -SwitchName.
	fmt.Fprintf(&b, "New-VM -Name '%s' -Generation 2 -MemoryStartupBytes %dMB -VHDPath '%s' -SwitchName '%s' | Out-Null\n",
		name, mem, psEscapeSingle(spec.OverlayVHDX), psEscapeSingle(spec.BuildSwitch))
	fmt.Fprintf(&b, "Set-VM -Name '%s' -AutomaticCheckpointsEnabled $false -CheckpointType Disabled\n", name)
	fmt.Fprintf(&b, "Set-VMProcessor -VMName '%s' -Count %d\n", name, cpus)
	// NIC2: relay upstream on the Internal NAT switch (eth1 in the guest).
	fmt.Fprintf(&b, "Add-VMNetworkAdapter -VMName '%s' -SwitchName '%s'\n", name, psEscapeSingle(spec.NATSwitch))
	// Secure Boot OFF (unsigned UKI); boot straight off the overlay disk.
	fmt.Fprintf(&b, "$drive = Get-VMHardDiskDrive -VMName '%s'\n", name)
	fmt.Fprintf(&b, "Set-VMFirmware -VMName '%s' -EnableSecureBoot Off -FirstBootDevice $drive\n", name)
	// COM1 -> host named pipe: readiness echo + relay stdout.
	fmt.Fprintf(&b, "Set-VMComPort -VMName '%s' -Number 1 -Path '%s'\n", name, psEscapeSingle(spec.COMPipePath))
	b.WriteString("'OK'\n")
	return b.String(), nil
}

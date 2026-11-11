package hyperv

import "fmt"

// ensureNetworkScript renders EnsureNetwork's PowerShell (see
// localProvisioner.EnsureNetwork for the NAT order it applies); it is pure, so
// the order is tested on any host.
func ensureNetworkScript(spec NetworkSpec) (string, error) {
	natName := spec.NATName
	if natName == "" {
		natName = spec.SwitchName + "-nat"
	}
	if spec.SwitchName == "" && spec.NATName == "" {
		return "", fmt.Errorf("EnsureNetwork: no switch and no NAT name")
	}
	prefixLen, err := cidrPrefixLen(spec.NATPrefix)
	if err != nil {
		return "", err
	}

	name := psEscapeSingle(spec.SwitchName)
	nat := psEscapeSingle(natName)
	prefix := psEscapeSingle(spec.NATPrefix)
	hostIP := psEscapeSingle(spec.HostIP)
	devNAT := psEscapeSingle(spec.DevNAT)

	// One script so the whole standup is atomic-ish and reports a single error.
	// $ErrorActionPreference=Stop turns cmdlet failures into catchable errors.
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'

# Build (Private) switch: relay <-> build only, no host path. ALWAYS per-build:
# this is the isolation boundary, so it is never shared between builds. Setup
# names none: its build switches are the slots.
if ('%[1]s') {
    $build = Get-VMSwitch -Name '%[1]s' -ErrorAction SilentlyContinue
    if (-not $build) {
        New-VMSwitch -Name '%[1]s' -SwitchType Private | Out-Null
    } elseif ($build.SwitchType -ne 'Private') {
        throw "switch '%[1]s' exists but is $($build.SwitchType); expected Private"
    }
}

# NAT egress: the relay's controlled upstream, chosen from what the host has.
$reused = 1
$natName = $null
if ('%[6]s' -and (Get-NetNat -Name '%[6]s' -ErrorAction SilentlyContinue)) {
    $natName = '%[6]s'
} else {
    $existingNat = Get-NetNat -ErrorAction SilentlyContinue |
        Where-Object { $_.InternalIPInterfaceAddressPrefix -eq '%[5]s' } |
        Select-Object -First 1
    if ($existingNat) { $natName = $existingNat.Name }
    elseif (Get-NetNat -Name '%[2]s' -ErrorAction SilentlyContinue) { $natName = '%[2]s' }
}
if ($natName) {
    if (-not (Get-VMSwitch -Name $natName -ErrorAction SilentlyContinue)) {
        throw "a NAT '$natName' exists but its vSwitch is missing; " +
            "remove the stale NAT (elevated: Remove-NetNat -Name $natName) " +
            "or re-run 'warden setup --driver hyperv'"
    }
} else {
    $reused = 0
    $natName = '%[2]s'
    $natSw = Get-VMSwitch -Name $natName -ErrorAction SilentlyContinue
    if (-not $natSw) {
        New-VMSwitch -Name $natName -SwitchType Internal | Out-Null
    } elseif ($natSw.SwitchType -ne 'Internal') {
        throw "switch '$natName' exists but is $($natSw.SwitchType); expected Internal"
    }
    $alias = "vEthernet ($natName)"
    $hasIP = Get-NetIPAddress -InterfaceAlias $alias -IPAddress '%[3]s' -ErrorAction SilentlyContinue
    if (-not $hasIP) {
        New-NetIPAddress -IPAddress '%[3]s' -PrefixLength %[4]d -InterfaceAlias $alias | Out-Null
    }
    if (-not (Get-NetNat -Name $natName -ErrorAction SilentlyContinue)) {
        New-NetNat -Name $natName -InternalIPInterfaceAddressPrefix '%[5]s' | Out-Null
    }
}

# No inbound firewall rule: the host claims the relay and serves its collector
# over that same host-initiated connection, so nothing listens on the host.
"NAT=$natName"
"REUSED=$reused"
"PREFIX=$((Get-NetNat -Name $natName).InternalIPInterfaceAddressPrefix)"
Get-NetIPAddress -InterfaceAlias "vEthernet ($natName)" -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    ForEach-Object { "HOSTIP=$($_.IPAddress)" }
`, name, nat, hostIP, prefixLen, prefix, devNAT), nil
}

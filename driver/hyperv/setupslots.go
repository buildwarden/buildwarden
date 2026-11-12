package hyperv

import (
	"fmt"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// The durable strategy's build switches (Decision 7): `warden setup --driver
// hyperv --slots N` makes the Private switches <network>-0 to <network>-<N-1>
// once, elevated, so each durable build attaches its relay to a switch of its
// own without elevation. Setup also trims slots above N that nothing is
// attached to and retires the single shared <network> Private switch earlier
// setups made. An attached switch is never removed; setup refuses, naming its
// VMs.

// setupSlotsScript renders the PowerShell that makes network's n slot
// switches, trims the unattached ones above n and retires the old shared
// switch. It changes nothing attached; it reports each switch on a line
// parseSetupSlots reads, then psDone.
func setupSlotsScript(network string, n int) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$net = '%[1]s'
$n = %[2]d
function Attached($sw) {
    @(Get-VMNetworkAdapter -All | Where-Object { $_.SwitchName -eq $sw } |
        ForEach-Object { $_.VMName }) -join ', '
}
for ($i = 0; $i -lt $n; $i++) {
    $name = "$net-$i"
    $s = Get-VMSwitch -Name $name -ErrorAction SilentlyContinue
    if (-not $s) {
        New-VMSwitch -Name $name -SwitchType Private | Out-Null
        "SLOT=$name`+"`t"+`created"
    } elseif ($s.SwitchType -ne 'Private') {
        "WRONGTYPE=$name`+"`t"+`$($s.SwitchType)"
    } else {
        "SLOT=$name`+"`t"+`present"
    }
}
$re = '^' + [regex]::Escape($net) + '-([0-9]+)$'
foreach ($s in @(Get-VMSwitch -ErrorAction SilentlyContinue)) {
    if ($s.Name -match $re -and [long]$Matches[1] -ge $n) {
        $vms = Attached $s.Name
        if ($vms) { "BUSY=$($s.Name)`+"`t"+`$vms" }
        else { Remove-VMSwitch -Name $s.Name -Force; "REMOVED=$($s.Name)" }
    }
}
$old = Get-VMSwitch -Name $net -ErrorAction SilentlyContinue
if ($old -and $old.SwitchType -eq 'Private') {
    $vms = Attached $net
    if ($vms) { "OLDBUSY=$net`+"`t"+`$vms" }
    else { Remove-VMSwitch -Name $net -Force; "RETIRED=$net" }
}
'%[3]s'
`, psEscapeSingle(network), n, psDone)
}

// setupSlots is what setupSlotsScript reported.
type setupSlots struct {
	Slots   []string // each slot, "<name> (created)" or "<name> (present)"
	Removed []string // unattached slots above N, removed
	Retired string   // the old shared switch, removed
	// Refusals are the switches setup would not touch, each with why.
	Refusals []string
}

// parseSetupSlots reads setupSlotsScript's lines, refusing output that does
// not end with psDone.
func parseSetupSlots(out string) (setupSlots, error) {
	var r setupSlots
	if err := psDoneCheck(out); err != nil {
		return r, err
	}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		name, detail, _ := strings.Cut(v, "\t")
		switch k {
		case "SLOT":
			r.Slots = append(r.Slots, name+" ("+detail+")")
		case "WRONGTYPE":
			r.Refusals = append(r.Refusals,
				fmt.Sprintf("%s exists as a %s switch, not Private", name, detail))
		case "REMOVED":
			r.Removed = append(r.Removed, name)
		case "BUSY":
			r.Refusals = append(r.Refusals, fmt.Sprintf(
				"%s is above --slots but has VMs attached (%s)", name, detail))
		case "RETIRED":
			r.Retired = name
		case "OLDBUSY":
			r.Refusals = append(r.Refusals, fmt.Sprintf("the old shared build "+
				"switch %s is no longer used but has VMs attached (%s)", name,
				detail))
		}
	}
	return r, nil
}

// err is the refusal for any switch setup would not touch.
func (r setupSlots) err() error {
	if len(r.Refusals) == 0 {
		return nil
	}
	return driver.Fixable("remove those VMs or move their adapters to another switch "+
		"(Connect-VMNetworkAdapter), or rename the switch, then re-run `warden setup "+
		"--driver hyperv`", "setup left these build switches as they are: %s",
		strings.Join(r.Refusals, "; "))
}

// devSwitchProbe reads the capability probe's "SLOTS=<count> OLD=<0|1>" line:
// how many slot switches the durable network has, and whether the old
// shared switch is there.
func devSwitchProbe(out string) (slots int, old bool) {
	for _, f := range strings.Fields(out) {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "SLOTS":
			_, _ = fmt.Sscanf(v, "%d", &slots)
		case "OLD":
			old = v == "1"
		}
	}
	return slots, old
}

package hyperv

import (
	"errors"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// TestSetupSlotsScript: the script makes <network>-0..N-1 Private, trims
// only unattached slots above N, retires only an unattached Private
// <network>, and escapes the network name.
func TestSetupSlotsScript(t *testing.T) {
	s := setupSlotsScript("o'net", 4)
	for _, want := range []string{
		"$net = 'o''net'", "$n = 4", "for ($i = 0; $i -lt $n; $i++)",
		"New-VMSwitch -Name $name -SwitchType Private",
		"[regex]::Escape($net) + '-([0-9]+)$'", "[long]$Matches[1] -ge $n",
		`if ($vms) { "BUSY=`, "Remove-VMSwitch -Name $s.Name -Force",
		"$old.SwitchType -eq 'Private'", `if ($vms) { "OLDBUSY=`,
		"Remove-VMSwitch -Name $net -Force",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
}

func TestParseSetupSlots(t *testing.T) {
	r := parseSetupSlots("SLOT=warden-dev-0\tcreated\r\nSLOT=warden-dev-1\tpresent\n" +
		"REMOVED=warden-dev-7\nRETIRED=warden-dev\n")
	if strings.Join(r.Slots, ",") != "warden-dev-0 (created),warden-dev-1 (present)" ||
		strings.Join(r.Removed, ",") != "warden-dev-7" || r.Retired != "warden-dev" ||
		r.err() != nil {
		t.Errorf("clean setup: %+v", r)
	}
	r = parseSetupSlots("SLOT=warden-dev-0\tpresent\nBUSY=warden-dev-5\twarden-build-k\n" +
		"WRONGTYPE=warden-dev-1\tInternal\nOLDBUSY=warden-dev\twarden-relay-a, warden-build-a\n")
	var fe *driver.FixableError
	err := r.err()
	if !errors.As(err, &fe) || r.Retired != "" {
		t.Fatalf("refusals: %+v, %v", r, err)
	}
	for _, want := range []string{"warden-dev-5 is above --slots but has VMs attached " +
		"(warden-build-k)", "warden-dev-1 exists as a Internal switch",
		"old shared build switch warden-dev", "warden-relay-a, warden-build-a"} {
		if !strings.Contains(fe.Msg, want) {
			t.Errorf("refusal lacks %q: %s", want, fe.Msg)
		}
	}
}

// TestDevSwitchProbe: DevSwitchPresent means slot switches exist; a host
// with only the old shared switch gets the "create the build switches"
// reason.
func TestDevSwitchProbe(t *testing.T) {
	for _, tt := range []struct {
		out   string
		slots int
		old   bool
	}{{"SLOTS=4 OLD=0\r\n", 4, false}, {"SLOTS=0 OLD=1", 0, true}, {"", 0, false}} {
		if slots, old := devSwitchProbe(tt.out); slots != tt.slots || old != tt.old {
			t.Errorf("%q: %d %v", tt.out, slots, old)
		}
	}
	legacy := Capabilities{Supported: true, HypervisorPresent: true, HyperVFeature: true,
		HyperVAdmin: true, DevSwitch: "warden-dev", DevSwitchLegacy: true}
	if ready, reason := legacy.Ready(); ready ||
		!strings.Contains(reason, "to create the build switches") {
		t.Errorf("legacy host: %v %q", ready, reason)
	}
	legacy.DevSwitchPresent, legacy.DevSwitchLegacy = true, false
	if legacy.NetworkStrategy() != NetReuseDurable {
		t.Errorf("slots present: strategy %v", legacy.NetworkStrategy())
	}
}

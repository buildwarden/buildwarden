package hyperv

import (
	"bytes"
	"strings"
	"testing"
)

// TestServiceRowStates renders doctor's privileged-service row, and the
// remedies of a blocked VM lifecycle and network, for each service state.
func TestServiceRowStates(t *testing.T) {
	const row = "Privileged service ................... "
	const install = "install the network service (one-time, elevated)"
	start := "start the installed privileged service: " + serviceStartFix
	restart := "start the installed privileged service: " + serviceRestartFix
	for _, c := range []struct {
		name   string
		caps   Capabilities
		row    string
		remedy string
	}{
		{"not installed", Capabilities{}, row + "not installed\n", install},
		{"stopped", Capabilities{ServiceInstalled: true, ServiceState: serviceStopped},
			row + "installed, but stopped\n    -> " + serviceStartFix + "\n", start},
		{"starting", Capabilities{ServiceInstalled: true, ServiceState: "starting"},
			row + "installed, but starting\n    -> " + serviceStartFix + "\n", start},
		{"state unread", Capabilities{ServiceInstalled: true},
			row + "installed, but in an unknown state\n    -> " + serviceStartFix + "\n",
			start},
		{"running, no pipe",
			Capabilities{ServiceInstalled: true, ServiceState: serviceRunning},
			row + "installed and running, but its pipe does not answer\n    -> " +
				serviceRestartFix + "\n", restart},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeServiceRow(&buf, c.caps)
			if got := buf.String(); got != c.row {
				t.Errorf("row = %q, want %q", got, c.row)
			}

			// Non-elevated, not a Hyper-V Administrator, no durable switches:
			// both paths are blocked, and each names the service remedy.
			caps := c.caps
			caps.Supported, caps.HypervisorPresent, caps.HyperVFeature = true, true, true
			buf.Reset()
			writeVMLifecycle(&buf, caps)
			writeNetworkStrategy(&buf, caps)
			if n := strings.Count(buf.String(), "    -> "+c.remedy+"\n"); n != 2 {
				t.Errorf("remedy %q named %d times, want 2:\n%s", c.remedy, n,
					buf.String())
			}
			if ready, reason := caps.Ready(); ready || !strings.HasSuffix(reason, c.remedy) {
				t.Errorf("Ready = %v, %q; want blocked naming %q", ready, reason, c.remedy)
			}
			if c.caps.ServiceInstalled && strings.Contains(buf.String(), install) {
				t.Errorf("an installed service is told to install:\n%s", buf.String())
			}
		})
	}

	// Reachable and stale services are reported as before, whatever the SCM
	// fields say.
	for caps, want := range map[*Capabilities]string{
		{ServiceReachable: true}: row + "reachable\n",
		{ServiceStale: "speaks protocol 1, this warden 2"}: row +
			"stale: it speaks protocol 1, this warden 2\n    -> " + serviceReinstallFix + "\n",
	} {
		var buf bytes.Buffer
		writeServiceRow(&buf, *caps)
		if buf.String() != want {
			t.Errorf("row = %q, want %q", buf.String(), want)
		}
		if caps.serviceDown() != "" {
			t.Errorf("serviceDown = %q for %+v", caps.serviceDown(), *caps)
		}
	}
}

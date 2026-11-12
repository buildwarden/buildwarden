package hyperv

import (
	"bytes"
	"strings"
	"testing"
)

const (
	serviceRow    = "Privileged service ................... "
	installRemedy = "install the network service (one-time, elevated)"
)

// TestServiceRowStates renders doctor's privileged-service row, and the
// remedies of a blocked VM lifecycle and network, for each state of a service
// whose pipe does not open, including one whose state the SCM would not give.
func TestServiceRowStates(t *testing.T) {
	start := "start the installed privileged service: " + serviceStartFix
	restart := "start the installed privileged service: " + serviceRestartFix
	check := func(fix string) string {
		return "check the privileged service, whose state could not be read: " + fix
	}
	const (
		denied = "open service WardenHyperV: Access is denied."
		failed = "open service WardenHyperV: The RPC server is unavailable."
	)
	stoppedRow := func(state string) string {
		return serviceRow + "installed, but " + state + "\n    -> " +
			serviceStartFix + "\n"
	}
	for _, c := range []struct {
		name   string
		caps   Capabilities
		row    string
		remedy string
	}{
		{"not installed", Capabilities{}, serviceRow + "not installed\n",
			installRemedy},
		{"stopped", Capabilities{ServiceInstalled: true, ServiceState: serviceStopped},
			stoppedRow("stopped"), start},
		{"starting", Capabilities{ServiceInstalled: true, ServiceState: "starting"},
			stoppedRow("starting"), start},
		{"state unread", Capabilities{ServiceInstalled: true},
			stoppedRow("in an unknown state"), start},
		{"access denied",
			Capabilities{ServiceInstalled: true, ServiceUnread: denied},
			serviceRow + "installed, but its state could not be read (" + denied +
				")\n    -> " + serviceCheckInstalledFix + "\n",
			check(serviceCheckInstalledFix)},
		{"read failed", Capabilities{ServiceUnread: failed},
			serviceRow + "state could not be read (" + failed + ")\n    -> " +
				serviceCheckFix + "\n", check(serviceCheckFix)},
		{"running, no pipe",
			Capabilities{ServiceInstalled: true, ServiceState: serviceRunning},
			serviceRow + "installed and running, but its pipe does not answer\n" +
				"    -> " + serviceRestartFix + "\n", restart},
	} {
		t.Run(c.name, func(t *testing.T) {
			checkServiceRemedies(t, c.caps, c.row, c.remedy)
		})
	}
}

// checkServiceRemedies checks doctor's service row for caps, and that a
// non-elevated host that is not a Hyper-V Administrator and has no durable
// switches, where both paths are blocked, names remedy for each and in Ready.
func checkServiceRemedies(t *testing.T, caps Capabilities, row, remedy string) {
	t.Helper()
	var buf bytes.Buffer
	writeServiceRow(&buf, caps)
	if got := buf.String(); got != row {
		t.Errorf("row = %q, want %q", got, row)
	}
	caps.Supported, caps.HypervisorPresent = true, true
	caps.HyperVFeature = true
	buf.Reset()
	writeVMLifecycle(&buf, caps)
	writeNetworkStrategy(&buf, caps)
	out := buf.String()
	if n := strings.Count(out, "    -> "+remedy+"\n"); n != 2 {
		t.Errorf("remedy %q named %d times, want 2:\n%s", remedy, n, out)
	}
	ready, reason := caps.Ready()
	if ready || !strings.HasSuffix(reason, remedy) {
		t.Errorf("Ready = %v, %q; want blocked naming %q", ready, reason, remedy)
	}
	if (caps.ServiceInstalled || caps.ServiceUnread != "") &&
		strings.Contains(out+reason, installRemedy) {
		t.Errorf("an installed or unread service is told to install:\n%s", out)
	}
}

// TestServiceRowAnswering: a reachable or stale service is reported as
// before, and is never told to start.
func TestServiceRowAnswering(t *testing.T) {
	const why = "speaks protocol 1, this warden 2"
	for _, c := range []struct {
		caps Capabilities
		want string
	}{
		{Capabilities{ServiceReachable: true}, serviceRow + "reachable\n"},
		{Capabilities{ServiceStale: why}, serviceRow + "stale: it " + why +
			"\n    -> " + serviceReinstallFix + "\n"},
	} {
		var buf bytes.Buffer
		writeServiceRow(&buf, c.caps)
		if buf.String() != c.want {
			t.Errorf("row = %q, want %q", buf.String(), c.want)
		}
		if fix := c.caps.serviceDown(); fix != "" {
			t.Errorf("serviceDown = %q for %+v", fix, c.caps)
		}
	}
}

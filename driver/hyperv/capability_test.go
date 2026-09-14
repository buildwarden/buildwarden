package hyperv

import (
	"bytes"
	"strings"
	"testing"
)

func TestResolvePerOperation(t *testing.T) {
	tests := []struct {
		name    string
		caps    Capabilities
		op      Op
		want    Outcome
	}{
		{"elevated does network standup", Capabilities{Elevated: true}, OpNetworkStandup, OutcomeDirect},
		{"non-elevated cannot standup network", Capabilities{Elevated: false}, OpNetworkStandup, OutcomeBlocked},
		{"hyperv admin does VM ops directly", Capabilities{HyperVAdmin: true}, OpVMLifecycle, OutcomeDirect},
		{"elevated also does VM ops", Capabilities{Elevated: true}, OpVMLifecycle, OutcomeDirect},
		{"hyperv admin cannot standup network", Capabilities{HyperVAdmin: true}, OpNetworkStandup, OutcomeBlocked},
		{"service delegates VM ops when unprivileged", Capabilities{ServiceReachable: true}, OpVMLifecycle, OutcomeDelegate},
		{"service delegates network standup", Capabilities{ServiceReachable: true}, OpNetworkStandup, OutcomeDelegate},
		{"privilege beats service for VM ops", Capabilities{HyperVAdmin: true, ServiceReachable: true}, OpVMLifecycle, OutcomeDirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.Resolve(tt.op); got != tt.want {
				t.Fatalf("Resolve(%v) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

func TestReady(t *testing.T) {
	base := Capabilities{Supported: true, HypervisorPresent: true, HyperVFeature: true}

	// Hyper-V Admins member with a reusable dev switch: ready without elevation.
	c := base
	c.HyperVAdmin = true
	c.DevSwitchPresent = true
	if ok, reason := c.Ready(); !ok {
		t.Fatalf("expected ready with hyperv-admin + dev switch, got not ready: %s", reason)
	}

	// VM ops available but no switch and no network privilege: not ready.
	c = base
	c.HyperVAdmin = true
	c.DevSwitchPresent = false
	if ok, _ := c.Ready(); ok {
		t.Fatal("expected not ready when network standup blocked and no reusable switch")
	}

	// Fully elevated, no switch: can stand up the network, so ready.
	c = base
	c.Elevated = true
	if ok, reason := c.Ready(); !ok {
		t.Fatalf("expected ready when elevated, got not ready: %s", reason)
	}

	// No VM privilege at all: not ready even with a switch.
	c = base
	c.DevSwitchPresent = true
	if ok, _ := c.Ready(); ok {
		t.Fatal("expected not ready when VM lifecycle blocked")
	}

	// Unsupported platform: not ready.
	if ok, _ := (Capabilities{Supported: false, Platform: "linux"}).Ready(); ok {
		t.Fatal("expected not ready on unsupported platform")
	}
}

func TestNetworkStrategy(t *testing.T) {
	tests := []struct {
		name string
		caps Capabilities
		want NetworkStrategy
	}{
		{"elevated uses fresh per-build even with a durable switch",
			Capabilities{Elevated: true, DevSwitchPresent: true}, NetEphemeralPerBuild},
		{"elevated uses fresh per-build without a switch",
			Capabilities{Elevated: true}, NetEphemeralPerBuild},
		{"non-elevated hyperv-admin reuses the durable switch",
			Capabilities{HyperVAdmin: true, DevSwitchPresent: true}, NetReuseDurable},
		{"non-elevated hyperv-admin without a switch is blocked",
			Capabilities{HyperVAdmin: true, DevSwitchPresent: false}, NetBlocked},
		{"service delegates when unprivileged",
			Capabilities{ServiceReachable: true}, NetDelegateService},
		{"service beats durable reuse",
			Capabilities{ServiceReachable: true, HyperVAdmin: true, DevSwitchPresent: true}, NetDelegateService},
		{"nothing available is blocked", Capabilities{}, NetBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.NetworkStrategy(); got != tt.want {
				t.Fatalf("NetworkStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoctorReport(t *testing.T) {
	// Non-elevated Hyper-V Admins member reusing a durable switch: ready.
	caps := Capabilities{
		Supported:         true,
		Platform:          "windows",
		TokenScope:        "current process (non-elevated)",
		HypervisorPresent: true,
		HyperVFeature:     true,
		HyperVAdmin:       true,
		DevSwitch:         "warden-dev",
		DevSwitchPresent:  true,
	}
	var buf bytes.Buffer
	if blocked := Doctor(&buf, caps); blocked {
		t.Fatalf("expected not blocked; report:\n%s", buf.String())
	}
	out := buf.String()
	for _, want := range []string{"VM lifecycle ......... OK", "reuse durable switch", "warden-dev"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q; got:\n%s", want, out)
		}
	}

	// Elevated with a durable switch present: fresh per-build, switch noted as
	// lower-privilege only.
	buf.Reset()
	elevated := Capabilities{
		Supported: true, Platform: "windows", TokenScope: "current process (elevated)",
		HypervisorPresent: true, HyperVFeature: true, Elevated: true,
		DevSwitch: "warden-dev", DevSwitchPresent: true,
	}
	if blocked := Doctor(&buf, elevated); blocked {
		t.Fatalf("expected not blocked when elevated; report:\n%s", buf.String())
	}
	out = buf.String()
	for _, want := range []string{"fresh per-build", "lower-privilege runs"} {
		if !strings.Contains(out, want) {
			t.Errorf("elevated report missing %q; got:\n%s", want, out)
		}
	}

	// Unsupported platform is blocked and says so.
	buf.Reset()
	if blocked := Doctor(&buf, Capabilities{Supported: false, Platform: "linux"}); !blocked {
		t.Fatal("expected blocked on unsupported platform")
	}
	if !strings.Contains(buf.String(), "unavailable on linux") {
		t.Errorf("expected unavailable message; got:\n%s", buf.String())
	}
}

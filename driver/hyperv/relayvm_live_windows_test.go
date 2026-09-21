//go:build windows

package hyperv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveCreateRelayVM exercises CreateRelayVM against real Hyper-V. It is
// gated on WARDEN_HYPERV_LIVE=1 because it creates and removes a real VM plus a
// differencing overlay, and needs the warden-dev / warden-dev-nat switches and a
// built boot VHDX (WARDEN_RELAY_BOOT_VHDX). It never runs in normal CI.
//
// It is the ground-truth check that Hyper-V accepts the exact cmdlet sequence
// buildRelayVMScript emits -- in particular that New-VHD -Differencing accepts
// the fixed boot VHDX as a parent, and that the VM ends up with two NICs and
// Secure Boot off.
func TestLiveCreateRelayVM(t *testing.T) {
	if os.Getenv("WARDEN_HYPERV_LIVE") != "1" {
		t.Skip("set WARDEN_HYPERV_LIVE=1 to run the live Hyper-V relay VM smoke test")
	}
	boot := os.Getenv("WARDEN_RELAY_BOOT_VHDX")
	if boot == "" {
		t.Skip("set WARDEN_RELAY_BOOT_VHDX to the boot VHDX path")
	}

	p := newLocalProvisioner(false)
	ctx := context.Background()
	name := "warden-relay-livetest"
	overlay := filepath.Join(t.TempDir(), "relay-overlay.vhdx")

	_ = p.RemoveVM(ctx, name) // clear any leftover from a prior run
	t.Cleanup(func() {
		_ = p.RemoveVM(ctx, name)
		_ = os.Remove(overlay)
	})

	spec := RelayVMSpec{
		Name:        name,
		BootVHDX:    boot,
		OverlayVHDX: overlay,
		BuildSwitch: "warden-dev",
		NATSwitch:   "warden-dev-nat",
		COMPipePath: `\\.\pipe\` + name,
	}
	if _, err := p.CreateRelayVM(ctx, spec); err != nil {
		t.Fatalf("CreateRelayVM: %v", err)
	}

	out, err := runPS(
		"$n=(Get-VMNetworkAdapter -VMName '" + name + "').Count; " +
			"$sb=(Get-VMFirmware -VMName '" + name + "').SecureBoot; " +
			"\"nics=$n sb=$sb\"")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(out, "nics=2") {
		t.Errorf("expected 2 NICs, got: %s", strings.TrimSpace(out))
	}
	if !strings.Contains(out, "sb=Off") {
		t.Errorf("expected Secure Boot Off, got: %s", strings.TrimSpace(out))
	}
}

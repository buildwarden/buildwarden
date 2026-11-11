//go:build windows

package hyperv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveRelayHostEndToEnd is the full claim/tunnel integration: it creates and
// boots a real relay VM (unconfigured, listening on :8301), claims it at its
// static NAT address, opens the host-initiated /v1/tunnel and serves the
// collector over it, and asserts the relay came all the way up (StartRelay
// returns once the relay reports ready over the tunnel). This is the first point
// the boot image, claim, tunnel, collector and CreateRelayVM all run together.
//
// Gated on WARDEN_HYPERV_LIVE=1. Needs the durable network from `warden setup
// --driver hyperv` (warden-dev-nat and the warden-dev-<i> slot switches) and a
// built boot VHDX (WARDEN_RELAY_BOOT_VHDX). Never runs in normal CI.
func TestLiveRelayHostEndToEnd(t *testing.T) {
	if os.Getenv("WARDEN_HYPERV_LIVE") != "1" {
		t.Skip("set WARDEN_HYPERV_LIVE=1 to run the live relay-host end-to-end test")
	}
	boot := os.Getenv("WARDEN_RELAY_BOOT_VHDX")
	if boot == "" {
		t.Skip("set WARDEN_RELAY_BOOT_VHDX to the boot VHDX path")
	}

	dir := t.TempDir()
	overlay := filepath.Join(dir, "relay-overlay.vhdx")
	prov := newLocalProvisioner(false)

	// Clear any leftover VM from a prior run before starting.
	_ = prov.RemoveVM(context.Background(), "warden-relay-livee2e")

	lp, ok := prov.(*localProvisioner)
	if !ok {
		t.Fatalf("provisioner %T is not the local one", prov)
	}
	res, err := lp.VerifyNetwork(context.Background(), DefaultDevSwitch)
	if err != nil {
		t.Fatalf("VerifyNetwork: %v", err)
	}
	h, err := StartRelay(context.Background(), prov, RelayHostConfig{
		BuildID:      "livee2e",
		OutputDir:    dir,
		BootVHDX:     boot,
		OverlayVHDX:  overlay,
		Slots:        res.Slots,
		NATSwitch:    res.NATName,
		NATPrefix:    res.NATPrefix,
		HostIP:       res.HostIP,
		SeedDir:      filepath.Join(dir, "relay-seed"),
		ReadyTimeout: 120 * time.Second,
	})
	if err != nil {
		t.Fatalf("StartRelay end-to-end: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	t.Logf("relay host ready: vm=%s (claimed, collector tunnel up)", h.VMName)
	if h.Session.Token == "" {
		t.Error("expected a minted token")
	}
}

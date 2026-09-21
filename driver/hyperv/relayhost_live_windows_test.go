//go:build windows

package hyperv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveRelayHostEndToEnd is the full pull-at-boot integration: it starts the
// collector + config responder on the host, creates and boots a real relay VM,
// and asserts the relay came all the way up -- it fetched its config from the
// responder over the NAT link, selected the httpSink, connected to the collector
// and POSTed /v1/ready (which is what makes StartRelay return). This is the
// first point the boot image, config responder, collector and CreateRelayVM all
// run together.
//
// Gated on WARDEN_HYPERV_LIVE=1. Needs the warden-dev / warden-dev-nat switches
// with the host gateway 192.168.240.1 assigned, and a built boot VHDX
// (WARDEN_RELAY_BOOT_VHDX). Never runs in normal CI.
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

	h, err := StartRelay(context.Background(), prov, RelayHostConfig{
		BuildID:      "livee2e",
		OutputDir:    dir,
		BootVHDX:     boot,
		OverlayVHDX:  overlay,
		BuildSwitch:  "warden-dev",
		NATSwitch:    "warden-dev-nat",
		ReadyTimeout: 120 * time.Second,
	})
	if err != nil {
		t.Fatalf("StartRelay end-to-end: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	t.Logf("relay host ready: vm=%s sink=%s", h.VMName, h.SinkURL)
	if h.Token == "" {
		t.Error("expected a minted token")
	}
}

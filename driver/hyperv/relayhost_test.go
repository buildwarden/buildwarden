package hyperv

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProv is a no-op Provisioner that records the lifecycle calls the relay
// host makes, so a test can assert them without a real Hyper-V host.
type fakeProv struct {
	createRelay atomic.Int32
	start       atomic.Int32
	remove      atomic.Int32
}

func (f *fakeProv) EnsureNetwork(context.Context, NetworkSpec) (*NetworkResources, error) {
	return &NetworkResources{}, nil
}
func (f *fakeProv) TeardownNetwork(context.Context, string) error       { return nil }
func (f *fakeProv) CreateVM(context.Context, VMSpec) (*VMHandle, error) { return &VMHandle{}, nil }
func (f *fakeProv) CreateRelayVM(context.Context, RelayVMSpec) (*VMHandle, error) {
	f.createRelay.Add(1)
	return &VMHandle{Name: "relay"}, nil
}
func (f *fakeProv) StartVM(context.Context, string) error { f.start.Add(1); return nil }
func (f *fakeProv) StopVM(context.Context, string) error  { return nil }
func (f *fakeProv) RemoveVM(context.Context, string) error {
	f.remove.Add(1)
	return nil
}
func (f *fakeProv) CreateDiffDisk(context.Context, string, string) error { return nil }

// With no real relay to POST /v1/ready, StartRelay must hit the ready timeout
// and tear everything down: create+start were attempted, the VM is removed, and
// the per-build overlay file is deleted.
func TestStartRelay_TimeoutTearsDown(t *testing.T) {
	fp := &fakeProv{}
	dir := t.TempDir()
	overlay := filepath.Join(dir, "overlay.vhdx")
	if err := os.WriteFile(overlay, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err := StartRelay(context.Background(), fp, RelayHostConfig{
		BuildID:     "unit",
		OutputDir:   dir,
		BootVHDX:    "boot.vhdx",
		OverlayVHDX: overlay,
		BuildSwitch: "warden-dev",
		NATSwitch:   "warden-dev-nat",
		NATHostIP:   "127.0.0.1", // servers bind to loopback in the test
		ConfigPort:  48299,       // high test ports to avoid colliding with defaults
		CollectorPort: 48390,
		ReadyTimeout:  400 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a ready-timeout error (no relay to signal ready)")
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Errorf("returned in %s, before the ready timeout", elapsed)
	}
	if got := fp.createRelay.Load(); got != 1 {
		t.Errorf("CreateRelayVM calls = %d, want 1", got)
	}
	if got := fp.start.Load(); got != 1 {
		t.Errorf("StartVM calls = %d, want 1", got)
	}
	if got := fp.remove.Load(); got != 1 {
		t.Errorf("RemoveVM (teardown) calls = %d, want 1", got)
	}
	if _, statErr := os.Stat(overlay); !os.IsNotExist(statErr) {
		t.Error("Close should have removed the overlay file")
	}
}

func TestStartRelay_ValidatesConfig(t *testing.T) {
	_, err := StartRelay(context.Background(), &fakeProv{}, RelayHostConfig{
		OutputDir: t.TempDir(), BootVHDX: "b", OverlayVHDX: "o",
		BuildSwitch: "s", NATSwitch: "n", // missing BuildID
	})
	if err == nil {
		t.Error("expected validation error for missing BuildID")
	}
}

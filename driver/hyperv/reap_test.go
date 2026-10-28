package hyperv

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// reapFake reports leftover VMs and disks in use, and records removals.
type reapFake struct {
	fakeProv
	vms     []string
	inUse   []string
	removed []string
}

func (f *reapFake) ListVMs(_ context.Context, prefix string) ([]string, error) {
	var out []string
	for _, vm := range f.vms {
		if len(vm) >= len(prefix) && vm[:len(prefix)] == prefix {
			out = append(out, vm)
		}
	}
	return out, nil
}
func (f *reapFake) RemoveVM(_ context.Context, name string) error {
	f.removed = append(f.removed, name)
	return nil
}
func (f *reapFake) DisksInUse(context.Context) ([]string, error) { return f.inUse, nil }

func TestReapPrepareLeftovers(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	dir := filepath.Join(evalImagesDir(), "prepared")
	if err := os.MkdirAll(filepath.Join(dir, "work-123", "prepare-seed"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := func(n string) string {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	recorded, orphan, busy, other := file("ws-1.vhdx"), file("ws-2.vhdx"), file("ws-3.vhdx"),
		file("notes.txt")
	if err := saveEvalManifest(&evalManifest{
		Images:   map[string]evalManifestEntry{},
		Prepared: []PreparedBase{{Path: recorded}},
	}); err != nil {
		t.Fatal(err)
	}
	f := &reapFake{
		vms:   []string{"warden-prepare-20261027t1", "warden-build-x", "my-vm"},
		inUse: []string{busy},
	}
	reapPrepareLeftovers(context.Background(), f, os.Stderr)

	if !slices.Equal(f.removed, []string{"warden-prepare-20261027t1"}) {
		t.Errorf("removed VMs %v, want only the prepare VM", f.removed)
	}
	for p, want := range map[string]bool{
		recorded: true, busy: true, other: true, orphan: false,
		filepath.Join(dir, "work-123"): false,
	} {
		_, err := os.Stat(p)
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(p), exists, want)
		}
	}
}

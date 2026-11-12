package hyperv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type cleanFake struct {
	vms       []string
	listErr   error
	removeErr map[string]error
	removed   []string
	torn      []string
}

func (f *cleanFake) ListVMs(_ context.Context, prefix string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []string
	for _, vm := range f.vms {
		if strings.HasPrefix(vm, prefix) {
			out = append(out, vm)
		}
	}
	return out, nil
}

func (f *cleanFake) RemoveVM(_ context.Context, name string) error {
	if err := f.removeErr[name]; err != nil {
		return err
	}
	f.removed = append(f.removed, name)
	return nil
}

func (f *cleanFake) TeardownNetwork(_ context.Context, id string) error {
	f.torn = append(f.torn, id)
	return nil
}

// locks returns a tryLock where the named build IDs are held (running).
func locks(running ...string) func(string) (func(), bool, error) {
	return func(name string) (func(), bool, error) {
		for _, id := range running {
			if name == buildLockName(id) {
				return nil, false, nil
			}
		}
		return func() {}, true, nil
	}
}

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCleanRemovesOnlyFinishedBuilds(t *testing.T) {
	tmp := t.TempDir()
	mkdirs(t, tmp, "warden-hyperv-done0001-123", "warden-hyperv-live0001-456",
		"warden-hyperv-dirsonly-789", "warden-firecracker-done0001-1")
	f := &cleanFake{vms: []string{
		"warden-build-done0001", "warden-relay-done0001",
		"warden-build-live0001", "warden-relay-live0001",
		"warden-relay-kept0001",
		"warden-prepare-20261112T183421Z", "warden-dev-0", "podman-machine-default",
	}}
	var out bytes.Buffer
	got := clean(context.Background(), &out, f, tmp, locks("live0001"))

	if got != 3 {
		t.Errorf("cleaned %d builds, want 3 (done0001, kept0001, dirsonly)\n%s",
			got, out.String())
	}
	slices.Sort(f.removed)
	wantVMs := []string{
		"warden-build-done0001", "warden-relay-done0001", "warden-relay-kept0001",
	}
	if !slices.Equal(f.removed, wantVMs) {
		t.Errorf("removed VMs %v, want %v", f.removed, wantVMs)
	}
	slices.Sort(f.torn)
	wantSw := []string{"warden-dirsonly", "warden-done0001", "warden-kept0001"}
	if !slices.Equal(f.torn, wantSw) {
		t.Errorf("tore down %v, want %v", f.torn, wantSw)
	}
	for name, want := range map[string]bool{
		"warden-hyperv-done0001-123":    false,
		"warden-hyperv-dirsonly-789":    false,
		"warden-hyperv-live0001-456":    true,
		"warden-firecracker-done0001-1": true,
	} {
		_, err := os.Stat(filepath.Join(tmp, name))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists=%v, want %v", name, exists, want)
		}
	}
}

func TestCleanKeepsSwitchAndDirWhenAVMStays(t *testing.T) {
	tmp := t.TempDir()
	mkdirs(t, tmp, "warden-hyperv-stuck001-1")
	f := &cleanFake{
		vms:       []string{"warden-build-stuck001", "warden-relay-stuck001"},
		removeErr: map[string]error{"warden-build-stuck001": errors.New("in use")},
	}
	var out bytes.Buffer
	if got := clean(context.Background(), &out, f, tmp, locks()); got != 0 {
		t.Errorf("cleaned %d, want 0", got)
	}
	if len(f.torn) != 0 {
		t.Errorf("tore down %v with a VM left", f.torn)
	}
	if _, err := os.Stat(filepath.Join(tmp, "warden-hyperv-stuck001-1")); err != nil {
		t.Errorf("work dir removed with a VM left: %v", err)
	}
	if !strings.Contains(out.String(), "warden-build-stuck001: in use") {
		t.Errorf("output %q does not name the VM and its error", out.String())
	}
}

func TestCleanListErrorRemovesNothing(t *testing.T) {
	tmp := t.TempDir()
	mkdirs(t, tmp, "warden-hyperv-done0001-1")
	f := &cleanFake{listErr: errors.New("Hyper-V module not found")}
	var out bytes.Buffer
	if got := clean(context.Background(), &out, f, tmp, locks()); got != 0 {
		t.Errorf("cleaned %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(tmp, "warden-hyperv-done0001-1")); err != nil {
		t.Errorf("work dir removed although VMs could not be listed: %v", err)
	}
}

func TestIsBuildID(t *testing.T) {
	for s, want := range map[string]bool{
		"q6h52kvd": true, "dev": false, "dev-0": false, "dev-nat": false,
		"Q6H52KVD": false, "q6h52kv": false, "q6h52kvdx": false, "q6h5-kvd": false,
	} {
		if got := isBuildID(s); got != want {
			t.Errorf("isBuildID(%q) = %v, want %v", s, got, want)
		}
	}
}

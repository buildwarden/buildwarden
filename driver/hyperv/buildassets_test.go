package hyperv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// touch creates an empty file so the resolver's os.Stat readability check passes.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveBuildBaseVHDX_GenerationByFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WARDEN_HYPERV_BUILD_IMAGE", "")
	t.Setenv("WARDEN_HYPERV_BUILD_GEN", "")

	tests := []struct {
		name    string
		file    string
		guestOS string
		wantGen int
		wantWin bool
	}{
		{"vhd is gen1", "eval.vhd", "windows", 1, true},
		{"vhdx is gen2", "cloud.vhdx", "linux", 2, false},
		{"uppercase VHD is gen1", "EVAL.VHD", "windows", 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			img := touch(t, dir, tc.file)
			path, isWin, gen, err := resolveBuildBaseVHDX(&driver.BuildRequest{Image: img, GuestOS: tc.guestOS})
			if err != nil {
				t.Fatalf("resolveBuildBaseVHDX: %v", err)
			}
			if gen != tc.wantGen {
				t.Errorf("generation = %d, want %d", gen, tc.wantGen)
			}
			if isWin != tc.wantWin {
				t.Errorf("isWindows = %v, want %v", isWin, tc.wantWin)
			}
			if !filepath.IsAbs(path) {
				t.Errorf("path %q is not absolute", path)
			}
		})
	}
}

func TestResolveBuildBaseVHDX_GenOverride(t *testing.T) {
	dir := t.TempDir()
	img := touch(t, dir, "cloud.vhdx") // would infer gen2
	t.Setenv("WARDEN_HYPERV_BUILD_GEN", "1")
	_, _, gen, err := resolveBuildBaseVHDX(&driver.BuildRequest{Image: img})
	if err != nil {
		t.Fatalf("resolveBuildBaseVHDX: %v", err)
	}
	if gen != 1 {
		t.Errorf("generation = %d, want 1 (override)", gen)
	}

	t.Setenv("WARDEN_HYPERV_BUILD_GEN", "bogus")
	if _, _, _, err := resolveBuildBaseVHDX(&driver.BuildRequest{Image: img}); err == nil {
		t.Error("expected an error for an invalid WARDEN_HYPERV_BUILD_GEN")
	}
}

func TestResolveBuildBaseVHDX_Rejections(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WARDEN_HYPERV_BUILD_GEN", "")

	// Unsupported format.
	img := touch(t, dir, "disk.qcow2")
	t.Setenv("WARDEN_HYPERV_BUILD_IMAGE", img)
	if _, _, _, err := resolveBuildBaseVHDX(nil); err == nil {
		t.Error("expected an error for a non-vhd/vhdx image")
	}

	// No image at all.
	t.Setenv("WARDEN_HYPERV_BUILD_IMAGE", "")
	if _, _, _, err := resolveBuildBaseVHDX(nil); err == nil {
		t.Error("expected an error when no image is provided")
	}

	// Missing file.
	t.Setenv("WARDEN_HYPERV_BUILD_IMAGE", filepath.Join(dir, "nope.vhd"))
	if _, _, _, err := resolveBuildBaseVHDX(nil); err == nil {
		t.Error("expected an error for a nonexistent image")
	}
}

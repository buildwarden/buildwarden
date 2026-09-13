package qemu

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateWindowsSeedFAT verifies the Windows seed is emitted as a FAT
// filesystem image (Windows reads FAT16 in-box with a drive letter and exact
// long names; our ISO9660 seed showed up as FileSystemType=Unknown). Skips
// when no FAT tool (hdiutil/mtools) is present, e.g. a minimal CI image.
func TestGenerateWindowsSeedFAT(t *testing.T) {
	haveTool := false
	for _, tool := range []string{"hdiutil", "mformat"} {
		if _, err := exec.LookPath(tool); err == nil {
			haveTool = true
			break
		}
	}
	if !haveTool {
		t.Skip("no FAT tool (hdiutil/mtools) available")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(src, "warden-run.ps1"), []byte("run\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(src, "warden-io.exe"), []byte("MZbytes"), 0644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "seed.img")
	if err := generateWindowsSeedFAT(src, out, "WARDEN"); err != nil {
		t.Fatalf("generateWindowsSeedFAT: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("seed image not written: %v", err)
	}

	// When `file` is available, assert the medium is FAT with the label.
	if _, err := exec.LookPath("file"); err == nil {
		desc, _ := exec.Command("file", "-b", out).Output()
		d := string(desc)
		if !strings.Contains(d, "FAT") {
			t.Errorf("expected FAT image, got: %s", d)
		}
		if !strings.Contains(d, "WARDEN") {
			t.Errorf("expected WARDEN volume label, got: %s", d)
		}
	}
}

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

	// When `file` is available, assert the medium is an MBR disk with a
	// partition (the FAT filesystem + WARDEN label live inside the partition,
	// which `file` does not descend into on the whole-disk image). Windows
	// needs the MBR partition to mount the seed on a usb-storage disk.
	if _, err := exec.LookPath("file"); err == nil {
		desc, _ := exec.Command("file", "-b", out).Output()
		d := string(desc)
		if !strings.Contains(d, "MBR boot sector") {
			t.Errorf("expected MBR disk image, got: %s", d)
		}
		if !strings.Contains(d, "partition 1") {
			t.Errorf("expected a partition table, got: %s", d)
		}
	}
}

package hyperv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
)

// Windows guest seeds for the Hyper-V driver. Both are directories packaged as
// a DVD ISO (winseed_windows.go) and found by volume label:
//
//   - The build seed (winguest.SeedLabel) carries warden-io.exe and
//     warden-run.ps1. The prepared image's warden-run startup task finds it and
//     runs warden-run.ps1 as SYSTEM, exactly as on qemu.
//   - The prepare seed (winguest.PrepareSeedLabel, staged by
//     winguest.StagePrepareSeed) carries the OOBE answer file and
//     warden-prepare.ps1, which turns a generalized image into a prepared
//     layer (see prepare.go).
//
// Staging is pure I/O, so it is tested on any platform.

// stageSeedDir writes the per-build seed into <workDir>/seed.
func stageSeedDir(workDir, arch string, _ *driver.BuildRequest) (string, error) {
	seedDir := filepath.Join(workDir, "seed")
	if err := os.MkdirAll(seedDir, 0o755); err != nil {
		return "", fmt.Errorf("seed dir: %w", err)
	}
	ioExe, err := driver.GuestBinary("warden-io", "windows", arch)
	if err != nil {
		return "", err
	}
	if err := driver.CopyFile(ioExe, filepath.Join(seedDir, "warden-io.exe")); err != nil {
		return "", fmt.Errorf("stage warden-io.exe: %w", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "warden-run.ps1"),
		[]byte(winguest.WardenRunPS1()), 0o644); err != nil {
		return "", fmt.Errorf("write warden-run.ps1: %w", err)
	}
	return seedDir, nil
}

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
//   - The prepare seed (winguest.PrepareSeedLabel) carries the OOBE answer file
//     and warden-prepare.ps1, which turns a generalized image into a prepared
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

// stagePrepareSeedDir writes the prepare seed into dir: the OOBE answer file
// under both implicit-search names, and warden-prepare.ps1.
func stagePrepareSeedDir(dir, arch string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prepare seed dir: %w", err)
	}
	unattend := winguest.OOBEUnattend(winguest.OOBEConfig{
		ProcArch:          unattendProcArch(arch),
		AdminPassword:     driver.RandAlphaNum(20),
		FirstLogonCommand: winguest.SeedScriptCommand(winguest.PrepareScriptName),
		Description:       "warden image prepare",
	})
	files := map[string]string{
		"unattend.xml":             unattend,
		"Autounattend.xml":         unattend,
		winguest.PrepareScriptName: winguest.PrepareScript(),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// unattendProcArch maps a Go arch to the unattend processorArchitecture token.
func unattendProcArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

package qemu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
)

// isWindowsGuest reports whether the build target is a Windows guest.
func isWindowsGuest(req *driver.BuildRequest) bool {
	return strings.EqualFold(req.GuestOS, "windows")
}

// windowsSeedDir builds the WARDEN seed directory: warden-io.exe plus the
// first-boot bootstrap the prepared image's startup task runs.
func windowsSeedDir(workDir, goarch string) (string, error) {
	seedDir := filepath.Join(workDir, "wardenseed")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return "", err
	}

	agentBin, err := driver.GuestBinary("warden-io", "windows", goarch)
	if err != nil {
		return "", err
	}
	if err := driver.CopyFile(
		agentBin, filepath.Join(seedDir, "warden-io.exe")); err != nil {
		return "", fmt.Errorf("copying warden-io.exe to seed: %w", err)
	}

	runPS := filepath.Join(seedDir, "warden-run.ps1")
	if err := os.WriteFile(runPS, []byte(winguest.WardenRunPS1()), 0644); err != nil {
		return "", err
	}
	return seedDir, nil
}

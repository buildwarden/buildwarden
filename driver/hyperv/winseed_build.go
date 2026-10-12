package hyperv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// stageSeedDir writes the build-guest seed files into <workDir>/seed and returns
// the directory. It is pure I/O (no Hyper-V / ISO calls), so it is testable on
// any platform; packaging the directory into attachable media is the
// platform-specific step in buildSeed.
//
// Contents (matching the qemu Windows runtime contract):
//   - warden-io.exe   the guest agent
//   - warden-run.ps1  first-boot bootstrap (launched by the OOBE FirstLogonCommand)
//   - unattend.xml + Autounattend.xml  the OOBE answer file, under both implicit
//     -search names so discovery does not hinge on which one OOBE looks for
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

	if err := os.WriteFile(
		filepath.Join(seedDir, "warden-run.ps1"),
		[]byte(generateWardenRunPS1()), 0o644); err != nil {
		return "", fmt.Errorf("write warden-run.ps1: %w", err)
	}

	unattend := generateWindowsUnattend(unattendConfig{
		AdminPassword: driver.RandAlphaNum(20),
		ProcArch:      unattendProcArch(arch),
	})
	for _, name := range []string{"unattend.xml", "Autounattend.xml"} {
		if err := os.WriteFile(
			filepath.Join(seedDir, name), []byte(unattend), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}
	return seedDir, nil
}

// unattendProcArch maps a Go arch to the unattend processorArchitecture token.
func unattendProcArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

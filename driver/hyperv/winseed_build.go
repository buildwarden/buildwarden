package hyperv

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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

	ioExe, err := ensureWardenIOExe(arch)
	if err != nil {
		return "", err
	}
	if err := copyFile(ioExe, filepath.Join(seedDir, "warden-io.exe")); err != nil {
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
		if err := os.WriteFile(filepath.Join(seedDir, name), []byte(unattend), 0o644); err != nil {
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

// ensureWardenIOExe returns a path to warden-io.exe for the guest arch,
// cross-building it into the cache when no prebuilt/env-supplied binary exists.
func ensureWardenIOExe(arch string) (string, error) {
	if p, err := resolveWardenIOExe(arch); err == nil {
		return p, nil
	}
	root, err := moduleRoot()
	if err != nil {
		// No module to build from; surface the resolver's actionable guidance.
		_, rerr := resolveWardenIOExe(arch)
		return "", rerr
	}
	cached := filepath.Join(hypervCacheDir(), "bin", "warden-io-windows-"+arch+".exe")
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", cached, "./cmd/warden-io")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+arch, "CGO_ENABLED=0")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cross-building warden-io.exe: %w", err)
	}
	return cached, nil
}

// moduleRoot walks up from the working directory to the dir containing go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s upward", dir)
		}
		dir = parent
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

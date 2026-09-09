package qemu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// Windows guests attach to the same isolated relay<->build link as Linux
// guests. There is no DHCP server on that link, so the guest is assigned a
// static address; these must match the relay VM's configuration and the Linux
// cloud-init network-config.
const (
	relayGatewayIP  = "10.0.0.1"
	buildGuestCIDR  = "10.0.0.2/30"
	windowsScript   = "build.ps1"
	windowsSeedName = "WARDEN"
)

// isWindowsGuest reports whether the build target is a Windows guest.
func isWindowsGuest(req *driver.BuildRequest) bool {
	return strings.EqualFold(req.GuestOS, "windows")
}

// prepareWindowsAgent places the PowerShell build script into the context
// (served to the guest over the relay at http://cwd/build.ps1) and
// cross-compiles warden-io.exe into agent/ for the seed volume.
func (d *Driver) prepareWindowsAgent(
	sharedDir string, req *driver.BuildRequest,
) error {
	ctxDir := filepath.Join(sharedDir, "context")
	buildScript := filepath.Join(ctxDir, windowsScript)
	if req.Script != "" {
		data, err := os.ReadFile(req.Script)
		if err != nil {
			return fmt.Errorf("reading build script: %w", err)
		}
		if err := os.WriteFile(buildScript, data, 0644); err != nil {
			return err
		}
	} else {
		// No-op default so the pipeline runs end to end.
		if err := os.WriteFile(buildScript, []byte("exit 0\r\n"), 0644); err != nil {
			return err
		}
	}

	agentDir := filepath.Join(sharedDir, "agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("creating agent dir: %w", err)
	}
	return prepareWindowsWardenIO(agentDir)
}

// prepareWindowsWardenIO cross-compiles warden-io.exe (windows/<hostarch>)
// into agentDir, using a prebuilt binary next to the warden executable or a
// cached build when available.
func prepareWindowsWardenIO(agentDir string) error {
	goarch := runtime.GOARCH
	dst := filepath.Join(agentDir, "warden-io.exe")

	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(
			filepath.Dir(exe), "warden-io-windows-"+goarch+".exe")
		if _, err := os.Stat(candidate); err == nil {
			return copyFile(candidate, dst)
		}
	}

	cached := filepath.Join(
		cacheBaseDir(), "warden", "bin", "warden-io-windows-"+goarch+".exe")
	_ = os.MkdirAll(filepath.Dir(cached), 0755)
	if !isCacheStale(cached) {
		return copyFile(cached, dst)
	}

	cmd := exec.Command("go", "build",
		"-ldflags=-s -w", "-o", cached, "./cmd/warden-io")
	cmd.Env = append(os.Environ(),
		"GOOS=windows", "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return copyFile(cached, dst)
}

// windowsSeedDir builds the WARDEN seed directory: warden-io.exe plus the
// first-boot bootstrap the prepared image's startup task runs.
func (d *Driver) windowsSeedDir(sharedDir string) (string, error) {
	seedDir := filepath.Join(sharedDir, "wardenseed")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return "", err
	}

	agentBin := filepath.Join(sharedDir, "agent", "warden-io.exe")
	if _, err := os.Stat(agentBin); err != nil {
		return "", fmt.Errorf("warden-io.exe not found in agent dir: %w", err)
	}
	if err := copyFile(
		agentBin, filepath.Join(seedDir, "warden-io.exe")); err != nil {
		return "", fmt.Errorf("copying warden-io.exe to seed: %w", err)
	}

	runPS := filepath.Join(seedDir, "warden-run.ps1")
	if err := os.WriteFile(runPS, []byte(wardenRunPS1()), 0644); err != nil {
		return "", err
	}
	return seedDir, nil
}

// wardenRunPS1 is executed by the prepared Windows image's boot task. It
// copies warden-io.exe off the seed volume and runs initialize, which
// configures the static network, installs the CA, fetches build.ps1 from the
// relay, runs it, and reports the exit code. The gateway/IP match the
// relay<->build link.
func wardenRunPS1() string {
	return fmt.Sprintf(`# warden-run.ps1 - first-boot bootstrap (see tools/win-image-prep).
# The prepared image's startup task mounts the WARDEN volume and runs this.
$ErrorActionPreference = 'Continue'
$seed = $PSScriptRoot
New-Item -ItemType Directory -Force -Path 'C:\warden' | Out-Null
Copy-Item -Force (Join-Path $seed 'warden-io.exe') 'C:\warden\warden-io.exe'
& 'C:\warden\warden-io.exe' initialize --gateway=%s --ip=%s
Stop-Computer -Force
`, relayGatewayIP, buildGuestCIDR)
}

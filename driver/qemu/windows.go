package qemu

import (
	"fmt"
	"os"
	"path/filepath"
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
	windowsSeedName = "WARDEN"
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
$log = 'C:\warden\warden-run.log'
"warden-run start $(Get-Date -Format o)" | Out-File -FilePath $log -Encoding utf8
& 'C:\warden\warden-io.exe' initialize --gateway=%s --ip=%s *>> $log 2>&1
"warden-io exit=$LASTEXITCODE $(Get-Date -Format o)" | Out-File -FilePath $log -Append -Encoding utf8
Stop-Computer -Force
`, relayGatewayIP, buildGuestCIDR)
}

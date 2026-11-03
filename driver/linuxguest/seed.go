// Package linuxguest is the Linux guest family's code shared by the VM drivers
// that boot cloud images (qemu, firecracker): the cloud-init NoCloud seed that
// delivers and starts warden-io, the ISO writer it is packed with, and
// cloud-image resolution and download.
package linuxguest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// The seed's last runcmd line, which ends the VM once warden-io has finished:
// qemu exits on a poweroff, Firecracker only on a guest reset.
const (
	PowerOff = "poweroff -f"
	Reboot   = "reboot -f"
)

// Seed generates a NoCloud seed directory, <workDir>/cidata, holding
// meta-data, network-config, user-data and the warden-io binary for
// linux/goarch. The caller packs it into a CIDATA volume. last is the final
// runcmd line (PowerOff or Reboot).
//
// The build script is NOT embedded in cloud-init. Instead, warden-io
// initialize fetches it from the relay's context endpoint at runtime.
func Seed(workDir, goarch, last string) (string, error) {
	agentBin, err := driver.GuestBinary("warden-io", "linux", goarch)
	if err != nil {
		return "", err
	}
	return writeSeed(filepath.Join(workDir, "cidata"), agentBin, last)
}

func writeSeed(seedDir, agentBin, last string) (string, error) {
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return "", err
	}
	if err := driver.CopyFile(agentBin, filepath.Join(seedDir, "warden-io")); err != nil {
		return "", fmt.Errorf("copying warden-io to seed: %w", err)
	}

	metaData := "instance-id: warden-build\nlocal-hostname: build\n"
	if err := os.WriteFile(
		filepath.Join(seedDir, "meta-data"),
		[]byte(metaData), 0644); err != nil {
		return "", err
	}

	// network-config (v2 format, match first ethernet device)
	netConfig := `version: 2
ethernets:
  eth0:
    match:
      name: "e*"
    addresses:
      - 10.0.0.2/30
    gateway4: 10.0.0.1
    nameservers:
      addresses:
        - 10.0.0.1
`
	if err := os.WriteFile(
		filepath.Join(seedDir, "network-config"),
		[]byte(netConfig), 0644); err != nil {
		return "", err
	}

	if err := os.WriteFile(
		filepath.Join(seedDir, "user-data"),
		[]byte(userData(last)), 0644); err != nil {
		return "", err
	}
	return seedDir, nil
}

// userData is the cloud-config that copies warden-io off the seed and runs
// initialize, then ends the VM with last.
func userData(last string) string {
	return `#cloud-config
runcmd:
  - mkdir -p /opt/warden /mnt/cidata
  - mount LABEL=CIDATA /mnt/cidata || mount /dev/sr0 /mnt/cidata || true
  - cp /mnt/cidata/warden-io /opt/warden/warden-io && chmod +x /opt/warden/warden-io
  - /opt/warden/warden-io initialize --gateway=10.0.0.1
  - ` + last + `
`
}

package qemu

import (
	"fmt"
	"os"
	"path/filepath"
)

// cloudInitSeed generates a NoCloud seed directory for QCOW2 builds.
// The directory is turned into an ISO and attached as a CDROM drive.
// Contains: meta-data, network-config, user-data, and the warden-io binary.
//
// The build script is NOT embedded in cloud-init. Instead, warden-io
// initialize fetches it from the relay's context endpoint at runtime.
func (d *Driver) cloudInitSeed(sharedDir string) (string, error) {
	seedDir := filepath.Join(sharedDir, "cidata")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return "", err
	}

	// Place warden-io binary on the seed ISO
	agentBin := filepath.Join(sharedDir, "agent", "warden-io")
	if _, err := os.Stat(agentBin); err == nil {
		if err := copyFile(agentBin, filepath.Join(seedDir, "warden-io")); err != nil {
			return "", fmt.Errorf("copying warden-io to seed: %w", err)
		}
	}

	// meta-data
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

	// user-data: cloud-config that delivers warden-io and runs initialize
	userData := cloudInitUserData()
	if err := os.WriteFile(
		filepath.Join(seedDir, "user-data"),
		[]byte(userData), 0644); err != nil {
		return "", err
	}

	return seedDir, nil
}

func cloudInitUserData() string {
	return `#cloud-config
runcmd:
  - mkdir -p /opt/warden /mnt/cidata
  - mount LABEL=CIDATA /mnt/cidata || mount /dev/sr0 /mnt/cidata || true
  - cp /mnt/cidata/warden-io /opt/warden/warden-io && chmod +x /opt/warden/warden-io
  - /opt/warden/warden-io initialize --gateway=10.0.0.1
  - poweroff -f
`
}

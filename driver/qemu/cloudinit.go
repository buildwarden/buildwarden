package qemu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"warden/driver/script"
)

// cloudInitSeed generates a NoCloud seed directory for QCOW2 builds.
// The directory is turned into an ISO and attached as a CDROM drive.
// Contains: meta-data, network-config, user-data, and the warden-io binary.
func (d *Driver) cloudInitSeed(
	sharedDir string, buildScript string, containerfile string,
) (string, error) {
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

	// Resolve the build commands
	var buildCommands string
	if buildScript != "" {
		data, err := os.ReadFile(buildScript)
		if err != nil {
			return "", fmt.Errorf("reading build script: %w", err)
		}
		buildCommands = string(data)
	} else if containerfile != "" {
		result, err := script.Translate(containerfile)
		if err != nil {
			return "", fmt.Errorf("translating containerfile: %w", err)
		}
		buildCommands = result.Script
	} else {
		buildCommands = "#!/bin/sh\ntrue\n"
	}

	// user-data: cloud-config that sets up warden-io and runs the build
	userData := cloudInitUserData(buildCommands)
	if err := os.WriteFile(
		filepath.Join(seedDir, "user-data"),
		[]byte(userData), 0644); err != nil {
		return "", err
	}

	return seedDir, nil
}

func cloudInitUserData(buildScript string) string {
	// Escape the build script for embedding in YAML
	indented := indentScript(buildScript, "        ")

	return fmt.Sprintf(`#cloud-config
write_files:
  - path: /etc/resolv.conf
    permissions: '0644'
    content: |
        nameserver 10.0.0.1
  - path: /opt/warden/build.sh
    permissions: '0755'
    content: |
%s
  - path: /opt/warden/watcher.sh
    permissions: '0755'
    content: |
        #!/bin/sh
        export PATH="/opt/warden:$PATH"
        /opt/warden/build.sh &
        BUILD_PID=$!
        while kill -0 "$BUILD_PID" 2>/dev/null; do
            wget -q -O /dev/null http://artifacts/heartbeat 2>/dev/null || true
            sleep 2
        done
        wait "$BUILD_PID"
        CODE=$?
        wget -q -O /dev/null "http://artifacts/exit?code=$CODE" 2>/dev/null || true

runcmd:
  - mkdir -p /opt/warden /mnt/cidata
  - mount LABEL=CIDATA /mnt/cidata || mount /dev/sr0 /mnt/cidata || true
  - cp /mnt/cidata/warden-io /opt/warden/warden-io && chmod +x /opt/warden/warden-io
  - /opt/warden/warden-io trust
  - /opt/warden/watcher.sh
  - poweroff -f
`, indented)
}

func indentScript(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	var sb strings.Builder
	for _, line := range lines {
		if line == "" {
			sb.WriteString("\n")
		} else {
			sb.WriteString(prefix + line + "\n")
		}
	}
	return sb.String()
}

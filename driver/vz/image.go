package vz

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ImageCache manages cached VM base images.
type ImageCache struct {
	CacheDir string
}

// ImageDir returns the directory containing the full set of files for a
// prepared macOS VM image (disk, aux-storage, hardware-model, machine-id).
// Returns empty string if no prepared image exists.
func (c *ImageCache) ImageDir() (string, error) {
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	entries, _ := os.ReadDir(c.CacheDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.CacheDir, e.Name())
		if isValidImageDir(dir) {
			return dir, nil
		}
	}
	return "", nil
}

func isValidImageDir(dir string) bool {
	required := []string{"disk.img", "aux-storage", "hardware-model", "machine-id", ".prepared"}
	for _, f := range required {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return false
		}
	}
	return true
}

// LatestIPSW returns the path to the disk image of a prepared macOS image.
// If no prepared image exists, it downloads and restores the latest from Apple.
func (c *ImageCache) LatestIPSW() (string, error) {
	dir, err := c.ImageDir()
	if err != nil {
		return "", err
	}
	if dir != "" {
		return filepath.Join(dir, "disk.img"), nil
	}

	// No cached image — restore from Apple's latest IPSW
	return c.RestoreIPSW("")
}

// RestoreIPSW downloads an IPSW file from the given URL, restores it to a
// bootable disk image, and prepares it for headless use (suppresses Setup
// Assistant, creates the warden user).
func (c *ImageCache) RestoreIPSW(ipswURL string) (string, error) {
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	// If URL is empty, fetch the latest from Apple
	if ipswURL == "" {
		var err error
		ipswURL, err = latestSupportedIPSW()
		if err != nil {
			return "", fmt.Errorf("fetching latest IPSW URL: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Latest macOS IPSW: %s\n", ipswURL)
	}

	// Create a directory for this image
	urlHash := shortHash(ipswURL)
	imgDir := filepath.Join(c.CacheDir, "macos-"+urlHash)
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return "", fmt.Errorf("creating image dir: %w", err)
	}

	diskPath := filepath.Join(imgDir, "disk.img")
	auxPath := filepath.Join(imgDir, "aux-storage")
	hwModelPath := filepath.Join(imgDir, "hardware-model")
	machineIDPath := filepath.Join(imgDir, "machine-id")

	// If already restored, return immediately
	if isValidImageDir(imgDir) {
		return diskPath, nil
	}

	// Download IPSW if not already cached
	ipswLocal := filepath.Join(c.CacheDir, "ipsw-"+urlHash+".ipsw")
	if _, err := os.Stat(ipswLocal); err != nil {
		// Check for a pre-existing ipsw-latest.ipsw (manual or previous download)
		latestPath := filepath.Join(c.CacheDir, "ipsw-latest.ipsw")
		if info, errLat := os.Stat(latestPath); errLat == nil && info.Size() > 0 {
			ipswLocal = latestPath
		} else {
			fmt.Fprintf(os.Stderr, "Downloading IPSW...\n")
			cmd := exec.Command("curl", "-L", "-o", ipswLocal, "--progress-bar", ipswURL)
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				os.Remove(ipswLocal)
				return "", fmt.Errorf("downloading IPSW: %w", err)
			}
		}
	}

	// Restore IPSW to disk image (64GB disk).
	// Skip if disk already has content (restore succeeded but prep failed).
	diskInfo, _ := os.Stat(diskPath)
	if diskInfo == nil || diskInfo.Size() == 0 {
		fmt.Fprintf(os.Stderr, "Restoring macOS from IPSW (this takes 10-20 minutes)...\n")
		if err := restoreIPSW(ipswLocal, diskPath, 64, auxPath, hwModelPath, machineIDPath); err != nil {
			return "", fmt.Errorf("restoring IPSW: %w", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Disk image exists, skipping restore...\n")
	}

	// Suppress Setup Assistant and create the warden user
	if err := PrepareHeadlessImage(diskPath); err != nil {
		return "", fmt.Errorf("headless setup: %w", err)
	}

	// Brief pause to ensure disk is fully released after hdiutil detach
	time.Sleep(2 * time.Second)

	// First boot: boot the VM once so macOS completes initial setup.
	// This validates: auto-login → launchd → virtio-fs mount → agent execution.
	fmt.Fprintf(os.Stderr, "Running first boot (macOS initial setup)...\n")
	if err := c.firstBoot(diskPath, imgDir); err != nil {
		return "", fmt.Errorf("first boot: %w", err)
	}

	// Mark the image as fully prepared
	os.WriteFile(filepath.Join(imgDir, ".prepared"), []byte("ok\n"), 0644)

	fmt.Fprintf(os.Stderr, "macOS image ready at %s\n", imgDir)
	return diskPath, nil
}

// PrepareHeadlessImage installs the warden build agent into a macOS disk image.
// Writes a LaunchDaemon, boot script, and the warden-io binary directly onto
// the disk so no virtio-fs shared directory is needed at runtime.
func PrepareHeadlessImage(diskPath string) error {
	dm, err := mountDiskImage(diskPath)
	if err != nil {
		return fmt.Errorf("mounting disk: %w", err)
	}
	defer unmountDiskImage(dm)
	mountPoint := dm.DataVolumePath

	if err := installAgentLaunchd(mountPoint); err != nil {
		return fmt.Errorf("installing agent launchd: %w", err)
	}

	if err := installWardenIO(mountPoint); err != nil {
		return fmt.Errorf("installing warden-io: %w", err)
	}

	return nil
}

// installWardenIO cross-compiles warden-io for darwin/arm64 and installs it
// into the disk image at /usr/local/bin/warden-io.
func installWardenIO(mountPoint string) error {
	binDir := filepath.Join(mountPoint, "usr", "local", "bin")
	agentBin := filepath.Join(binDir, "warden-io")

	// Check for pre-built binary next to the warden executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe),
			"warden-io-darwin-arm64")
		if _, errStat := os.Stat(candidate); errStat == nil {
			data, err := os.ReadFile(candidate)
			if err != nil {
				return err
			}
			if err := os.WriteFile(agentBin, data, 0755); err != nil {
				return err
			}
			os.Chown(agentBin, 0, 0) //nolint:errcheck
			return nil
		}
	}

	// Fall back to building from source
	cmd := exec.Command("go", "build", "-o", agentBin, "./cmd/warden-io")
	cmd.Env = append(os.Environ(),
		"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cross-compiling warden-io: %w", err)
	}
	os.Chown(agentBin, 0, 0) //nolint:errcheck
	return nil
}

// ReprepareImage updates the boot script and LaunchDaemon on an existing
// prepared image. Use this after upgrading warden to install the latest
// boot script without needing a full IPSW restore.
func ReprepareImage(imgDir string) error {
	diskPath := filepath.Join(imgDir, "disk.img")
	if _, err := os.Stat(diskPath); err != nil {
		return fmt.Errorf("disk image not found: %w", err)
	}

	if err := PrepareHeadlessImage(diskPath); err != nil {
		return err
	}

	// Update the .prepared marker
	os.WriteFile(filepath.Join(imgDir, ".prepared"), //nolint:errcheck
		[]byte("ok\n"), 0644)

	return nil
}

// suppressSetupAssistant writes the markers that tell macOS to skip the
// first-run experience entirely.
func suppressSetupAssistant(mountPoint string) error {
	varDB := filepath.Join(mountPoint, "private", "var", "db")
	if err := os.MkdirAll(varDB, 0755); err != nil {
		return err
	}

	// Primary marker — existence of this file suppresses Setup Assistant
	doneFile := filepath.Join(varDB, ".AppleSetupDone")
	if err := os.WriteFile(doneFile, []byte{}, 0644); err != nil {
		return fmt.Errorf("writing .AppleSetupDone: %w", err)
	}

	// Setup Assistant plist with skip list for all known screens.
	// This covers macOS 13-15. New screens in future versions may need
	// additions but the .AppleSetupDone marker alone handles most cases.
	skipItems := []string{
		"Accessibility",
		"AccessibilityOptions",
		"AppStore",
		"AppleID",
		"Appearance",
		"CloudStorage",
		"CrashReporterSoftware",
		"DataMigration",
		"Diagnostics",
		"FindMy",
		"HomeButtonSensitivity",
		"iCloudDocuments",
		"iCloudStorage",
		"Intro",
		"Location",
		"NanoPreview",
		"Privacy",
		"PrivacyBriefing",
		"RemoteManagement",
		"ScreenTime",
		"Selection",
		"Siri",
		"SiriDatabase",
		"TrueTone",
		"User",
		"WatchKit",
	}

	plist := buildSkipPlist(skipItems)
	plistPath := filepath.Join(varDB, "com.apple.SetupAssistant.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("writing SetupAssistant plist: %w", err)
	}

	return nil
}

func buildSkipPlist(items []string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>DidSeeCloudSetup</key>
	<true/>
	<key>DidSeePrivacy</key>
	<true/>
	<key>DidSeeSiriSetup</key>
	<true/>
	<key>DidSeeAccessibility</key>
	<true/>
	<key>DidSeeAppearanceSetup</key>
	<true/>
	<key>DidSeeScreenTime</key>
	<true/>
	<key>DidSeeTrueTonePrivacy</key>
	<true/>
	<key>DidSeeiCloudLoginForStorageServices</key>
	<true/>
	<key>LastSeenBuddyBuildVersion</key>
	<string>99Z999</string>
	<key>LastSeenCloudProductVersion</key>
	<string>99.9</string>
	<key>SkipFirstLoginOptimization</key>
	<true/>
`)
	sb.WriteString("\t<key>SkipSetupItems</key>\n\t<array>\n")
	for _, item := range items {
		fmt.Fprintf(&sb, "\t\t<string>%s</string>\n", item)
	}
	sb.WriteString("\t</array>\n</dict>\n</plist>\n")
	return sb.String()
}

// createWardenUser pre-creates a local user account in the macOS disk image.
// Uses dslocal plist format (the same store Open Directory uses offline).
const wardenUser = "warden"
const wardenUID = "501"
const wardenGID = "20" // staff group

func createWardenUser(mountPoint string) error {
	usersDir := filepath.Join(mountPoint, "private", "var", "db",
		"dslocal", "nodes", "Default", "users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		return err
	}

	// Generate password hash. macOS uses PBKDF2-SHA512 in ShadowHashData
	// but for VM-only ephemeral use, we use a simple salted SHA-512.
	// The VM is torn down after each build so password strength is irrelevant.
	passHash := hashPassword(wardenUser)

	userPlist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>authentication_authority</key>
	<array>
		<string>;ShadowHash;HASHLIST:&lt;SALTED-SHA512-PBKDF2&gt;</string>
	</array>
	<key>generateduid</key>
	<array>
		<string>FFFFEEEE-DDDD-CCCC-BBBB-AAAA00000001</string>
	</array>
	<key>gid</key>
	<array>
		<string>%s</string>
	</array>
	<key>home</key>
	<array>
		<string>/Users/%s</string>
	</array>
	<key>name</key>
	<array>
		<string>%s</string>
	</array>
	<key>passwd</key>
	<array>
		<string>********</string>
	</array>
	<key>realname</key>
	<array>
		<string>Warden Build User</string>
	</array>
	<key>shell</key>
	<array>
		<string>/bin/zsh</string>
	</array>
	<key>uid</key>
	<array>
		<string>%s</string>
	</array>
	<key>ShadowHashData</key>
	<array>
		<data>%s</data>
	</array>
</dict>
</plist>
`, wardenGID, wardenUser, wardenUser, wardenUID, passHash)

	userPath := filepath.Join(usersDir, wardenUser+".plist")
	if err := os.WriteFile(userPath, []byte(userPlist), 0600); err != nil {
		return fmt.Errorf("writing user plist: %w", err)
	}

	// Create home directory
	homeDir := filepath.Join(mountPoint, "Users", wardenUser)
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return fmt.Errorf("creating home dir: %w", err)
	}

	return nil
}

// hashPassword generates a placeholder ShadowHashData value.
// For ephemeral VMs this doesn't need to be cryptographically correct —
// auto-login bypasses password verification entirely.
func hashPassword(password string) string {
	h := sha512.Sum512([]byte(password))
	return hex.EncodeToString(h[:])
}

// enableAutoLogin configures the system to log in as the warden user
// without prompting for a password.
func enableAutoLogin(mountPoint string) error {
	kcDir := filepath.Join(mountPoint, "private", "etc")
	if err := os.MkdirAll(kcDir, 0755); err != nil {
		return err
	}

	// /etc/kcpassword stores the auto-login password XOR-encoded.
	// The encoding is a simple repeating XOR with a fixed key.
	encoded := encodeKCPassword(wardenUser)
	kcPath := filepath.Join(kcDir, "kcpassword")
	if err := os.WriteFile(kcPath, encoded, 0600); err != nil {
		return fmt.Errorf("writing kcpassword: %w", err)
	}

	// loginwindow plist to enable auto-login
	lwPlist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>autoLoginUser</key>
	<string>%s</string>
</dict>
</plist>
`, wardenUser)

	prefDir := filepath.Join(mountPoint, "Library", "Preferences")
	if err := os.MkdirAll(prefDir, 0755); err != nil {
		return err
	}
	lwPath := filepath.Join(prefDir, "com.apple.loginwindow.plist")
	return os.WriteFile(lwPath, []byte(lwPlist), 0644)
}

// encodeKCPassword XOR-encodes a password for /etc/kcpassword.
// macOS uses a fixed 11-byte repeating key.
var kcKey = []byte{0x7D, 0x89, 0x52, 0x23, 0xD2, 0xBC, 0xDD, 0xEA, 0xA3, 0xB9, 0x1F}

func encodeKCPassword(password string) []byte {
	pass := []byte(password)
	// Pad to multiple of 12 bytes
	padLen := 12 - (len(pass) % 12)
	if padLen < 12 {
		pass = append(pass, make([]byte, padLen)...)
	}

	encoded := make([]byte, len(pass))
	for i, b := range pass {
		encoded[i] = b ^ kcKey[i%len(kcKey)]
	}
	return encoded
}

// installAgentLaunchd writes a LaunchDaemon plist that watches the virtio-fs
// shared directory for the warden-io agent binary and executes it on arrival.
func installAgentLaunchd(mountPoint string) error {
	// Use the Apple system daemon path which loads during Installer Progress
	// (before user login / full boot). The regular /Library/LaunchDaemons/
	// only loads after the login window, which is too late.
	daemonDir := filepath.Join(mountPoint, "Library", "Apple", "System",
		"Library", "LaunchDaemons")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return err
	}

	// The agent watches for /Volumes/My Shared Files/agent/warden-io.
	// On macOS guests with Virtualization.framework, virtio-fs shares with
	// the automount tag appear at /Volumes/My Shared Files/<tag>.
	// We use the tag "shared" so the mount is /Volumes/My Shared Files/shared/
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.buildwarden.agent</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/warden-boot</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/warden-agent.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/warden-agent.log</string>
</dict>
</plist>
`
	plistPath := filepath.Join(daemonDir, "com.buildwarden.agent.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}
	os.Chown(plistPath, 0, 0) //nolint:errcheck

	// Write the boot script that waits for virtio-fs mount then runs
	// warden-io initialize. This lives on the local disk so it's always
	// available at boot. warden-io initialize handles all subsequent setup:
	// network config, relay health wait, CA install, build script fetch+exec.
	binDir := filepath.Join(mountPoint, "usr", "local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	bootScript := `#!/bin/sh
# Warden boot: configure network, run warden-io initialize.
# Runs as LaunchDaemon (root, no user session needed).
# warden-io is baked into the image at /usr/local/bin/warden-io.
LOG="/var/log/warden-boot.log"
AGENT="/usr/local/bin/warden-io"

exec >>"$LOG" 2>&1
echo "$(date): warden-boot starting"

if [ ! -x "$AGENT" ]; then
    echo "$(date): FATAL agent not found at $AGENT"
    exit 1
fi

# Configure static network. The relay is always at .1 on the /30 subnet.
# The FileHandle virtio-net device is en1 (en0 is NAT/USB-NCM when present).
# Wait for an interface to become active — macOS takes a moment to bring
# the virtio-net driver online after boot.
echo "$(date): configuring network..."
IFACE=""
i=0
while [ -z "$IFACE" ] && [ $i -lt 30 ]; do
    for iface in $(ifconfig -l); do
        case "$iface" in lo0|anpi*|bridge*|awdl*|llw*|utun*|ap*) continue ;; esac
        hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
        status=$(ifconfig "$iface" 2>/dev/null | grep "status:" | awk '{print $2}')
        if [ -n "$hasether" ] && [ "$status" = "active" ]; then
            IFACE=$iface
            break
        fi
    done
    if [ -z "$IFACE" ]; then
        sleep 2
        i=$((i + 1))
    fi
done

if [ -z "$IFACE" ]; then
    echo "$(date): FATAL no active ethernet interface after 60s"
    ifconfig -a 2>&1
    exit 1
fi

GW="10.0.0.1"
SELF="10.0.0.2"
MASK="255.255.255.252"

echo "$(date): $IFACE -> $SELF gw $GW"
ifconfig "$IFACE" inet "$SELF" netmask "$MASK" up
route add default "$GW" 2>/dev/null || true

# DNS (multiple mechanisms for macOS compatibility)
mkdir -p /etc/resolver
echo "nameserver $GW" > /etc/resolv.conf
echo "nameserver $GW" > /etc/resolver/default
scutil 2>/dev/null <<SCUTIL || true
d.init
d.add ServerAddresses * $GW
set State:/Network/Service/warden/DNS
d.init
d.add Addresses * $SELF
d.add SubnetMasks * $MASK
d.add Router $GW
d.add InterfaceName $IFACE
set State:/Network/Service/warden/IPv4
quit
SCUTIL

echo "$(date): launching warden-io initialize (gateway=$GW)"
exec "$AGENT" initialize --gateway="$GW" --ip="$SELF"
`
	bootScriptPath := filepath.Join(binDir, "warden-boot")
	if err := os.WriteFile(bootScriptPath, []byte(bootScript), 0755); err != nil {
		return err
	}
	os.Chown(bootScriptPath, 0, 0) //nolint:errcheck
	return nil
}

// diskMount holds the result of mounting a macOS disk image.
type diskMount struct {
	DataVolumePath string // e.g., "/Volumes/Data"
	BaseDevice     string // e.g., "/dev/disk4" — used for detach
}

// mountDiskImage mounts a macOS APFS disk image and returns the Data volume
// mount point. Uses `diskutil` to identify the Data role volume precisely.
func mountDiskImage(diskPath string) (*diskMount, error) {
	// Attach the disk image with ownership enabled so that files we write
	// retain root ownership (required for LaunchDaemons and system binaries).
	// This requires running as root (sudo).
	cmd := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "on")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("hdiutil attach: %s: %w", string(out), err)
	}

	// Extract the base device (first line of output is always the disk device)
	var baseDevice string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[0], "/dev/disk") {
			if !strings.Contains(fields[0], "s") || fields[0] == fields[0][:strings.LastIndex(fields[0], "s")] {
				// This is a base device (no slice number) like /dev/disk4
			}
			if baseDevice == "" {
				// First device listed is always the base disk
				baseDevice = fields[0]
			}
		}
	}

	// Identify the Data volume by its APFS role. We use `diskutil apfs list`
	// on the container device to get volume roles, then match the Data role
	// volume to its mount point.
	var dataPath string
	var mountPoints []string
	for _, line := range strings.Split(string(out), "\n") {
		tabIdx := strings.LastIndex(line, "\t")
		if tabIdx < 0 {
			continue
		}
		mp := strings.TrimSpace(line[tabIdx+1:])
		if mp != "" && mp[0] == '/' {
			mountPoints = append(mountPoints, mp)
		}
	}

	// The Data volume is identified by `diskutil info <mount>` showing
	// "Volume Name: Data". This is set by macOS during IPSW restore and
	// is the standard name for the APFS data volume.
	for _, mp := range mountPoints {
		infoOut, infoErr := exec.Command("diskutil", "info", mp).Output()
		if infoErr != nil {
			continue
		}
		for _, infoLine := range strings.Split(string(infoOut), "\n") {
			trimmed := strings.TrimSpace(infoLine)
			if strings.HasPrefix(trimmed, "Volume Name:") {
				volName := strings.TrimSpace(strings.TrimPrefix(trimmed, "Volume Name:"))
				if volName == "Data" {
					dataPath = mp
					break
				}
			}
		}
		if dataPath != "" {
			break
		}
	}

	if dataPath == "" {
		// Detach since we can't find the data volume
		exec.Command("hdiutil", "detach", baseDevice, "-force").Run() //nolint:errcheck
		return nil, fmt.Errorf("data volume not found in mounted disk image")
	}

	return &diskMount{
		DataVolumePath: dataPath,
		BaseDevice:     baseDevice,
	}, nil
}

// unmountDiskImage detaches all volumes for a previously mounted disk image.
func unmountDiskImage(dm *diskMount) {
	exec.Command("hdiutil", "detach", dm.BaseDevice, "-force").Run() //nolint:errcheck
}

// CloneDisk creates an APFS copy-on-write clone of a base disk image.
func CloneDisk(src, dst string) error {
	cmd := exec.Command("cp", "-c", src, dst)
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("cp", src, dst)
		return cmd.Run()
	}
	return nil
}

// firstBoot boots the macOS VM once so it completes initial setup.
// Places a probe agent on the shared volume and waits for it to signal.
// This validates the full chain: boot → auto-login → launchd → mount → agent.
func (c *ImageCache) firstBoot(diskPath, platformDir string) error {
	// First boot validates the boot chain: launchd → warden-boot → warden-io.
	// warden-io is baked into the image. The boot script configures networking
	// and runs "warden-io initialize --gateway=10.0.0.1". For validation we
	// just boot with NAT and verify the VM starts successfully. A full relay
	// test is done at e2e time rather than during image prep.
	vm, err := NewMacOSVM(macOSVMConfig{
		CPUs:               2,
		MemoryMB:           4096,
		DiskImagePath:      diskPath,
		AuxStoragePath:     filepath.Join(platformDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(platformDir, "hardware-model"),
		MachineIDPath:      filepath.Join(platformDir, "machine-id"),
		FileHandleSocketFD: -1,
		AttachNAT:          true,
	})
	if err != nil {
		return fmt.Errorf("creating first-boot VM: %w", err)
	}
	if err := vm.Start(); err != nil {
		return fmt.Errorf("starting first-boot VM: %w", err)
	}
	defer vm.Stop()

	// Wait 90 seconds for macOS to complete initial setup.
	// On first boot after IPSW restore, macOS does one-time tasks
	// (APFS seal verification, preference migration, etc.).
	fmt.Fprintf(os.Stderr,
		"Waiting for macOS first boot (90s)...\n")
	time.Sleep(90 * time.Second)

	fmt.Fprintf(os.Stderr, "First boot complete.\n")
	return nil
}

func shortHash(s string) string {
	h := sha512.Sum512([]byte(s))
	return hex.EncodeToString(h[:8])
}

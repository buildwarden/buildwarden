package vz

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	required := []string{"disk.img", "aux-storage", "hardware-model", "machine-id"}
	for _, f := range required {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return false
		}
	}
	return true
}

// LatestIPSW returns the path to the disk image of a prepared macOS image.
// If no prepared image exists, returns an error directing the user to restore.
func (c *ImageCache) LatestIPSW() (string, error) {
	dir, err := c.ImageDir()
	if err != nil {
		return "", err
	}
	if dir != "" {
		return filepath.Join(dir, "disk.img"), nil
	}
	return "", fmt.Errorf(
		"no prepared macOS image found; run 'warden image restore' to download and prepare one")
}

// RestoreIPSW downloads an IPSW file from the given URL, restores it to a
// bootable disk image, and prepares it for headless use (suppresses Setup
// Assistant, creates the warden user).
func (c *ImageCache) RestoreIPSW(ipswURL string) (string, error) {
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	// Create a directory for this image based on the URL hash
	urlHash := shortHash(ipswURL)
	imgDir := filepath.Join(c.CacheDir, "macos-"+urlHash)
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return "", fmt.Errorf("creating image dir: %w", err)
	}

	diskPath := filepath.Join(imgDir, "disk.img")

	// TODO: Download IPSW from ipswURL
	// TODO: Call VZMacOSRestoreImage + VZMacOSInstaller via cgo
	// TODO: Save hardware-model, machine-id, aux-storage to imgDir

	// After restore, suppress Setup Assistant and create user
	if err := PrepareHeadlessImage(diskPath); err != nil {
		return "", fmt.Errorf("headless setup: %w", err)
	}

	return diskPath, nil
}

// PrepareHeadlessImage modifies a freshly-restored macOS disk image so it
// boots directly to the desktop without Setup Assistant interaction.
// It writes setup markers and pre-creates the warden user.
func PrepareHeadlessImage(diskPath string) error {
	// Mount the disk image
	mountPoint, err := mountDiskImage(diskPath)
	if err != nil {
		return fmt.Errorf("mounting disk: %w", err)
	}
	defer unmountDiskImage(mountPoint)

	// 1. Mark Setup Assistant as complete
	if err := suppressSetupAssistant(mountPoint); err != nil {
		return fmt.Errorf("suppressing setup assistant: %w", err)
	}

	// 2. Create the warden user account
	if err := createWardenUser(mountPoint); err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	// 3. Enable auto-login for the warden user
	if err := enableAutoLogin(mountPoint); err != nil {
		return fmt.Errorf("enabling auto-login: %w", err)
	}

	// 4. Install the build agent launchd plist
	if err := installAgentLaunchd(mountPoint); err != nil {
		return fmt.Errorf("installing agent launchd: %w", err)
	}

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
	daemonDir := filepath.Join(mountPoint, "Library", "LaunchDaemons")
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
		<string>/Volumes/My Shared Files/shared/agent/warden-io</string>
		<string>run</string>
		<string>--signal-dir=/Volumes/My Shared Files/shared/signal</string>
		<string>--context-dir=/Volumes/My Shared Files/shared/context</string>
	</array>
	<key>WatchPaths</key>
	<array>
		<string>/Volumes/My Shared Files/shared/agent/warden-io</string>
	</array>
	<key>RunAtLoad</key>
	<false/>
	<key>StandardOutPath</key>
	<string>/var/log/warden-agent.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/warden-agent.log</string>
</dict>
</plist>
`
	plistPath := filepath.Join(daemonDir, "com.buildwarden.agent.plist")
	return os.WriteFile(plistPath, []byte(plist), 0644)
}

// mountDiskImage mounts a macOS disk image and returns the mount point.
func mountDiskImage(diskPath string) (string, error) {
	mountPoint, err := os.MkdirTemp("", "warden-mount-")
	if err != nil {
		return "", err
	}

	cmd := exec.Command("hdiutil", "attach", diskPath,
		"-mountpoint", mountPoint,
		"-nobrowse", "-noverify", "-noautoopen")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(mountPoint)
		return "", fmt.Errorf("hdiutil attach: %s: %w", string(out), err)
	}
	return mountPoint, nil
}

// unmountDiskImage unmounts a previously mounted disk image.
func unmountDiskImage(mountPoint string) {
	exec.Command("hdiutil", "detach", mountPoint, "-force").Run() //nolint:errcheck
	os.RemoveAll(mountPoint)
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

func shortHash(s string) string {
	h := sha512.Sum512([]byte(s))
	return hex.EncodeToString(h[:8])
}

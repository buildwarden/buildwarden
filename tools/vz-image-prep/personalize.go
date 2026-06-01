//go:build ignore

// Image personalization: boots a freshly IPSW-restored macOS VM with NAT
// internet so macOS can complete its one-time personalization. After that,
// writes .AppleSetupDone + user account + boot script markers to skip Setup
// Assistant on subsequent boots.
//
// This is a one-time operation per image. After personalization, the image's
// network driver works with any VZ attachment type.
//
// Run:
//   go build -o .dev/vz-personalize .dev/vz-personalize.go && \
//   codesign --force --sign "Developer ID Application: Jeffrey Edwards (WGWKU4C782)" \
//     --entitlements entitlements.plist .dev/vz-personalize && \
//   sudo ./.dev/vz-personalize
//
// Requires sudo for hdiutil -owners on (to set root ownership on files).

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"github.com/buildwarden/buildwarden/driver/vz"
)

func init() {
	runtime.LockOSThread()
}

func logf(format string, args ...interface{}) {
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", ts, fmt.Sprintf(format, args...))
}

func main() {
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "Must run as root (for hdiutil -owners on)\n")
		os.Exit(1)
	}

	startPhase := 1
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--from-phase=") {
			fmt.Sscanf(arg, "--from-phase=%d", &startPhase)
		}
		if arg == "--phase3-only" {
			startPhase = 3 // backwards compat
		}
	}

	imgDir := "/Users/jeffe/.cache/warden/images/macos-289afe17c389a911"
	diskPath := filepath.Join(imgDir, "disk.img")

	logf("Image: %s", imgDir)
	logf("Starting from phase %d", startPhase)

	if startPhase <= 1 {
		// Phase 1: Boot with NAT for personalization
		logf("=== Phase 1: Personalization boot (NAT internet) ===")
		logf("Booting macOS VM with NAT (10 min timeout)...")

		vm, err := vz.NewMacOSVM(vz.MacOSVMConfig{
			CPUs:               4,
			MemoryMB:           8192,
			DiskImagePath:      diskPath,
			AuxStoragePath:     filepath.Join(imgDir, "aux-storage"),
			HardwareModelPath:  filepath.Join(imgDir, "hardware-model"),
			MachineIDPath:      filepath.Join(imgDir, "machine-id"),
			FileHandleSocketFD: -1,
			AttachNAT:          true,
		})
		if err != nil {
			logf("ERROR creating VM: %v", err)
			os.Exit(1)
		}
		if err := vm.Start(); err != nil {
			logf("ERROR starting VM: %v", err)
			os.Exit(1)
		}

		logf("Waiting 10 minutes for personalization...")
		for i := 0; i < 20; i++ {
			time.Sleep(30 * time.Second)
			state := vm.State()
			logf("  [%ds] VM state: %d", (i+1)*30, state)
			if state == vz.StateStopped || state == vz.StateError {
				logf("  VM stopped/errored")
				break
			}
		}

		vm.Stop()
		time.Sleep(3 * time.Second)
		logf("Phase 1 complete.")
	}

	if startPhase <= 2 {
		// Phase 2: Write Setup Assistant bypass + user account to disk
		logf("=== Phase 2: Writing setup bypass markers ===")
		if err := writeSetupBypass(diskPath); err != nil {
			logf("ERROR: %v", err)
			os.Exit(1)
		}
		logf("Phase 2 complete.")
	}

	if startPhase <= 3 {
		// Phase 3: Boot again with NAT to verify network works
		logf("=== Phase 3: Verification boot ===")
	logf("Booting macOS VM to verify en0 is active...")

	vm2, err := vz.NewMacOSVM(vz.MacOSVMConfig{
		CPUs:               4,
		MemoryMB:           8192,
		DiskImagePath:      diskPath,
		AuxStoragePath:     filepath.Join(imgDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(imgDir, "hardware-model"),
		MachineIDPath:      filepath.Join(imgDir, "machine-id"),
		FileHandleSocketFD: -1,
		AttachNAT:          true,
	})
	if err != nil {
		logf("ERROR creating VM: %v", err)
		os.Exit(1)
	}
	if err := vm2.Start(); err != nil {
		logf("ERROR starting VM: %v", err)
		os.Exit(1)
	}

		logf("Waiting 90s for boot + network check...")
		time.Sleep(90 * time.Second)
		vm2.Stop()
		time.Sleep(3 * time.Second)

		// Check boot log from phase 3
		logf("=== Checking Phase 3 boot log ===")
		checkBootLog(diskPath)
	}

	if startPhase <= 4 {
		// Phase 4: Install Xcode Command Line Tools.
		// CLT provides cc/ld (linker), system headers, /usr/bin/git,
		// /usr/bin/python3 (real binary, not the shim), make, and the macOS SDK.
		// Without CLT, virtually no native compilation is possible since even
		// basic C linking requires the SDK's libSystem.tbd.
		// Must be done with NAT (direct internet) because Apple's softwareupdate
		// uses certificate pinning that MITM relays cannot intercept.
		logf("=== Phase 4: Installing Xcode Command Line Tools ===")
		if err := installCLT(imgDir, diskPath); err != nil {
			logf("ERROR: %v", err)
			os.Exit(1)
		}
		logf("Phase 4 complete.")
	}

	logf("Image preparation complete.")
}

func writeSetupBypass(diskPath string) error {
	// Mount with ownership enabled so we can chown
	out, err := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "on").CombinedOutput()
	if err != nil {
		return fmt.Errorf("hdiutil attach: %s: %w", string(out), err)
	}

	// Find Data volume mount point
	var dataPath string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "/Volumes/Data") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.HasPrefix(f, "/Volumes/") {
					dataPath = f
				}
			}
		}
	}
	if dataPath == "" {
		dataPath = "/Volumes/Data"
	}

	baseDevice := strings.Fields(strings.Split(string(out), "\n")[0])[0]
	defer exec.Command("hdiutil", "detach", baseDevice, "-force").Run()

	logf("  Data volume: %s", dataPath)

	// 1. .AppleSetupDone
	varDB := filepath.Join(dataPath, "private", "var", "db")
	os.MkdirAll(varDB, 0755)
	doneFile := filepath.Join(varDB, ".AppleSetupDone")
	if err := os.WriteFile(doneFile, []byte{}, 0644); err != nil {
		return fmt.Errorf("writing .AppleSetupDone: %w", err)
	}
	logf("  Wrote .AppleSetupDone")

	// 2. SetupAssistant plist with all skip items
	skipPlist := `<?xml version="1.0" encoding="UTF-8"?>
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
</dict>
</plist>
`
	plistPath := filepath.Join(varDB, "com.apple.SetupAssistant.plist")
	if err := os.WriteFile(plistPath, []byte(skipPlist), 0644); err != nil {
		return fmt.Errorf("writing SetupAssistant plist: %w", err)
	}
	logf("  Wrote SetupAssistant plist")

	// 3. LaunchDaemon at Apple system path (works in Installer Progress)
	daemonDir := filepath.Join(dataPath, "Library", "Apple", "System",
		"Library", "LaunchDaemons")
	os.MkdirAll(daemonDir, 0755)

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
	plistFile := filepath.Join(daemonDir, "com.buildwarden.agent.plist")
	if err := os.WriteFile(plistFile, []byte(plist), 0644); err != nil {
		return fmt.Errorf("writing daemon plist: %w", err)
	}
	exec.Command("chown", "0:0", plistFile).Run()
	exec.Command("chmod", "644", plistFile).Run()
	logf("  Wrote LaunchDaemon (Apple system path)")

	// 4. Boot script that checks network status
	binDir := filepath.Join(dataPath, "usr", "local", "bin")
	os.MkdirAll(binDir, 0755)

	bootScript := `#!/bin/sh
LOG="/var/log/warden-boot.log"
exec >>"$LOG" 2>&1
echo "$(date): warden-boot starting (personalization check)"

# Find the first active interface (skip lo0, anpi*, etc.)
echo "$(date): interface list: $(ifconfig -l)"
IFACE=""
for iface in $(ifconfig -l); do
    case "$iface" in
        lo0|anpi*|bridge*|awdl*|llw*|utun*|ap*) continue ;;
    esac
    status=$(ifconfig "$iface" 2>/dev/null | grep "status:" | awk '{print $2}')
    echo "$(date): $iface: status=$status"
    if [ "$status" = "active" ]; then
        IFACE=$iface
        break
    fi
done

if [ -z "$IFACE" ]; then
    echo "$(date): no active interface found, trying all en*"
    for iface in $(ifconfig -l); do
        case "$iface" in en*) ;; *) continue ;; esac
        hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
        if [ -n "$hasether" ]; then
            IFACE=$iface
            break
        fi
    done
fi

echo "$(date): selected: $IFACE"

if [ -z "$IFACE" ]; then
    echo "$(date): FATAL no usable interface"
    ifconfig -a 2>&1
    exit 1
fi

# Request DHCP on the active interface
echo "$(date): requesting DHCP on $IFACE..."
ipconfig set "$IFACE" DHCP 2>/dev/null || true
sleep 15

IP=$(ifconfig "$IFACE" 2>/dev/null | grep "inet " | awk '{print $2}')
GW=$(netstat -rn 2>/dev/null | grep "^default" | head -1 | awk '{print $2}')

echo "$(date): $IFACE result: ip=$IP gw=$GW"
ifconfig "$IFACE" 2>&1

# Test internet connectivity
if [ -n "$IP" ]; then
    echo "$(date): testing connectivity..."
    curl -sf --max-time 5 http://captive.apple.com/ 2>&1 && \
        echo "$(date): INTERNET WORKS" || \
        echo "$(date): no internet"
fi

echo "$(date): done"
`
	bootPath := filepath.Join(binDir, "warden-boot")
	if err := os.WriteFile(bootPath, []byte(bootScript), 0755); err != nil {
		return fmt.Errorf("writing boot script: %w", err)
	}
	exec.Command("chown", "0:0", bootPath).Run()
	logf("  Wrote boot script")

	// 5. Clear old logs
	os.Remove(filepath.Join(dataPath, "private", "var", "log", "warden-boot.log"))
	os.Remove(filepath.Join(dataPath, "private", "var", "log", "warden-agent.log"))

	return nil
}

func installCLT(imgDir, diskPath string) error {
	// Force-detach any existing mount of this disk (from previous runs)
	exec.Command("hdiutil", "detach", "/Volumes/Data", "-force").Run()
	exec.Command("hdiutil", "detach", "/Volumes/Macintosh HD 1", "-force").Run()
	time.Sleep(1 * time.Second)

	// Write a CLT-install boot script to disk
	out, err := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "on").CombinedOutput()
	if err != nil {
		return fmt.Errorf("attach: %s: %w", string(out), err)
	}
	baseDevice := strings.Fields(strings.Split(string(out), "\n")[0])[0]

	dataPath := "/Volumes/Data"
	bootPath := filepath.Join(dataPath, "usr/local/bin/warden-boot")

	// Save original boot script to restore after CLT install
	origBoot, _ := os.ReadFile(bootPath)

	// The boot script writes progress to /Volumes/shared/progress.log (virtio-fs)
	// so the host can tail it in real time.
	cltScript := `#!/bin/sh
# Write progress to disk log. Virtio-fs is blocked by macOS sandbox for
# LaunchDaemons, so we use the disk log and the host polls it after boot.
LOG="/var/log/warden-clt-install.log"
: > "$LOG"

log() { echo "$(date '+%H:%M:%S'): $*" >> "$LOG"; }

log "CLT install starting"

# Configure network — use the same approach as warden-boot (static + DNS)
IFACE=""
for iface in $(ifconfig -l); do
    case "$iface" in lo0|anpi*|bridge*|awdl*|llw*|utun*|ap*) continue ;; esac
    hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
    status=$(ifconfig "$iface" 2>/dev/null | grep "status:" | awk '{print $2}')
    if [ -n "$hasether" ] && [ "$status" = "active" ]; then IFACE=$iface; break; fi
done
if [ -z "$IFACE" ]; then
    for iface in $(ifconfig -l); do
        case "$iface" in en*) ;; *) continue ;; esac
        hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
        [ -n "$hasether" ] && { IFACE=$iface; break; }
    done
fi
log "interface=$IFACE"

# Static IP config (same subnet as netfwd)
ifconfig "$IFACE" inet 10.0.0.2 netmask 255.255.255.252 up
route add default 10.0.0.1 2>/dev/null || true

# DNS — point to gateway AND configure scutil for system resolver
echo "nameserver 10.0.0.1" > /etc/resolv.conf
mkdir -p /etc/resolver
echo "nameserver 10.0.0.1" > /etc/resolver/default
scutil 2>/dev/null <<SCUTIL || true
d.init
d.add ServerAddresses * 10.0.0.1
set State:/Network/Service/netfwd/DNS
d.init
d.add Addresses * 10.0.0.2
d.add SubnetMasks * 255.255.255.252
d.add Router 10.0.0.1
d.add InterfaceName $IFACE
set State:/Network/Service/netfwd/IPv4
d.init
d.add PrimaryService State:/Network/Service/netfwd
set State:/Network/Global/IPv4
quit
SCUTIL

# Start mDNSResponder if needed (macOS uses it for all DNS)
if ! pgrep -x mDNSResponder >/dev/null 2>&1; then
    launchctl load /System/Library/LaunchDaemons/com.apple.mDNSResponder.plist 2>/dev/null || true
    sleep 2
fi

sleep 3
log "network configured: $(ifconfig $IFACE | grep inet | head -1)"

# Test DNS first (separate from HTTP)
log "testing DNS..."
i=0
while ! nslookup captive.apple.com 10.0.0.1 >/dev/null 2>&1; do
    log "DNS not working yet ($i)..."
    sleep 3
    i=$((i + 1))
    if [ $i -gt 10 ]; then
        log "DNS failed after 10 tries, trying direct curl anyway"
        nslookup captive.apple.com 10.0.0.1 2>&1 >> "$LOG"
        break
    fi
done
log "DNS result: $(nslookup captive.apple.com 10.0.0.1 2>&1 | grep Address | tail -1)"

# Test full HTTP connectivity
i=0
while ! curl -sf --max-time 10 http://captive.apple.com/ >/dev/null 2>&1; do
    log "waiting for internet... ($i)"
    sleep 3
    i=$((i + 1))
    [ $i -gt 20 ] && { log "TIMEOUT waiting for network"; curl -v --max-time 5 http://captive.apple.com/ >> "$LOG" 2>&1; echo "FAIL" >> "$LOG"; exit 1; }
done
log "internet confirmed"

# Check if already installed
if xcode-select -p >/dev/null 2>&1; then
    log "CLT already installed at $(xcode-select -p)"
    echo "DONE" >> "$LOG"
    exit 0
fi

# Trigger CLT install via softwareupdate
touch /tmp/.com.apple.dt.CommandLineTools.installondemand.in-progress
log "searching for CLT package..."
softwareupdate -l 2>&1 | tee -a "$LOG"

# Extract the label value (after "* Label: ")
CLT_LABEL=$(softwareupdate -l 2>&1 | grep '* Label:.*Command Line Tools' | head -1 | sed 's/.*\* Label: //')
log "found: '$CLT_LABEL'"
CLT_PKG="$CLT_LABEL"

if [ -z "$CLT_PKG" ]; then
    log "ERROR: CLT package not found in softwareupdate"
    echo "FAIL" >> "$LOG"
    exit 1
fi

log "installing '$CLT_PKG' (this downloads ~1.5 GB)..."
softwareupdate -i "$CLT_PKG" --agree-to-license 2>&1 | tee -a "$LOG"
RC=$?
log "softwareupdate exit=$RC"
rm -f /tmp/.com.apple.dt.CommandLineTools.installondemand.in-progress

if [ $RC -eq 0 ] && xcode-select -p >/dev/null 2>&1; then
    log "SUCCESS — CLT installed at $(xcode-select -p)"
    echo "DONE" >> "$LOG"
else
    log "FAILED (exit=$RC)"
    echo "FAIL" >> "$LOG"
fi
`
	if err := os.WriteFile(bootPath, []byte(cltScript), 0755); err != nil {
		exec.Command("hdiutil", "detach", baseDevice, "-force").Run()
		return fmt.Errorf("writing CLT script: %w", err)
	}
	exec.Command("chown", "0:0", bootPath).Run()

	// Verify the write took effect
	verify, _ := os.ReadFile(bootPath)
	if !strings.Contains(string(verify), "CLT install starting") {
		exec.Command("hdiutil", "detach", baseDevice, "-force").Run()
		return fmt.Errorf("boot script verification failed — write didn't persist")
	}
	logf("  Boot script written and verified (%d bytes)", len(cltScript))

	// Clear old boot log so we can detect fresh output
	os.Remove(filepath.Join(dataPath, "private/var/log/warden-boot.log"))
	os.Remove(filepath.Join(dataPath, "private/var/log/warden-clt-install.log"))

	exec.Command("hdiutil", "detach", baseDevice, "-force").Run()
	time.Sleep(2 * time.Second)

	// Create socketpair for VM networking (vz-netfwd provides transparent internet)
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		return fmt.Errorf("socketpair: %w", err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	// Start vz-netfwd as a transparent TCP forwarder (no MITM)
	netfwdBin := filepath.Join(findModuleRoot(), ".dev", "vz-netfwd")
	if _, err := os.Stat(netfwdBin); err != nil {
		return fmt.Errorf("vz-netfwd not found at %s — build it first", netfwdBin)
	}
	netfwdCmd := exec.Command(netfwdBin, "--fd=3", "--subnet=10.0.0.0/30")
	netfwdCmd.Stderr = os.Stderr
	netfwdCmd.ExtraFiles = []*os.File{os.NewFile(uintptr(fds[1]), "netfwd-socket")}
	if err := netfwdCmd.Start(); err != nil {
		return fmt.Errorf("starting vz-netfwd: %w", err)
	}
	defer func() {
		netfwdCmd.Process.Signal(os.Interrupt)
		netfwdCmd.Wait()
	}()
	time.Sleep(1 * time.Second) // let netfwd initialize

	// Boot VM with socketpair (to netfwd) for internet access.
	// Progress is written to /var/log on the disk (virtio-fs is blocked by
	// macOS's LaunchDaemon sandbox). We poll the disk log after VM exits.
	logf("  Booting VM with netfwd for CLT install...")
	vm, err := vz.NewMacOSVM(vz.MacOSVMConfig{
		CPUs:               4,
		MemoryMB:           8192,
		DiskImagePath:      diskPath,
		AuxStoragePath:     filepath.Join(imgDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(imgDir, "hardware-model"),
		MachineIDPath:      filepath.Join(imgDir, "machine-id"),
		FileHandleSocketFD: fds[0],
		AttachNAT:          false,
	})
	if err != nil {
		return fmt.Errorf("creating VM: %w", err)
	}
	if err := vm.Start(); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}

	// Wait up to 15 minutes for CLT install.
	// Can't tail progress in real-time (macOS LaunchDaemon sandbox blocks
	// virtio-fs writes). We check the disk log after the VM stops.
	logf("  Waiting up to 15 min for CLT install (logs on disk)...")
	success := false
	for i := 0; i < 30; i++ {
		time.Sleep(30 * time.Second)
		logf("  [%d min] VM running...", (i+1)/2)
		if vm.State() == vz.StateStopped || vm.State() == vz.StateError {
			logf("  VM stopped early")
			break
		}
	}

	vm.Stop()
	time.Sleep(3 * time.Second)

	// Mount disk to check results and restore boot script
	out, err = exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "on").CombinedOutput()
	if err != nil {
		return fmt.Errorf("re-attach: %s: %w", string(out), err)
	}
	baseDevice = strings.Fields(strings.Split(string(out), "\n")[0])[0]
	defer exec.Command("hdiutil", "detach", baseDevice, "-force").Run()

	// Print CLT install log
	if logData, err := os.ReadFile(filepath.Join(dataPath, "private/var/log/warden-clt-install.log")); err == nil && len(logData) > 0 {
		logf("  === CLT install log ===\n%s", string(logData))
		if strings.Contains(string(logData), "SUCCESS") {
			success = true
		}
	} else {
		logf("  WARNING: no CLT install log found")
		// Check agent log for errors
		if agentLog, err := os.ReadFile(filepath.Join(dataPath, "private/var/log/warden-agent.log")); err == nil {
			logf("  Agent log:\n%s", string(agentLog))
		}
	}

	// Restore original boot script
	if origBoot != nil && len(origBoot) > 0 {
		os.WriteFile(bootPath, origBoot, 0755)
		exec.Command("chown", "0:0", bootPath).Run()
		logf("  Restored original boot script")
	}

	if !success {
		return fmt.Errorf("CLT install did not succeed")
	}
	return nil
}

func findModuleRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

func checkBootLog(diskPath string) {
	out, err := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "off").CombinedOutput()
	if err != nil {
		logf("Could not mount: %v", err)
		return
	}
	baseDevice := strings.Fields(strings.Split(string(out), "\n")[0])[0]
	defer exec.Command("hdiutil", "detach", baseDevice, "-force").Run()

	fmt.Fprintf(os.Stderr, "\n--- warden-boot.log ---\n")
	data, err := os.ReadFile("/Volumes/Data/private/var/log/warden-boot.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "(not found: %v)\n", err)
	} else {
		os.Stderr.Write(data)
	}
	fmt.Fprintf(os.Stderr, "\n")
}

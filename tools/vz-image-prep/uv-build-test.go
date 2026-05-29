//go:build ignore

// Milestone-1 → relay: Build the `uv` Python package wheel inside a macOS VM
// using the full relay path. Relay handles DNS, HTTPS MITM, and stdout streaming.
//
// Topology:
//   Host (this process)
//   ├── Shared volume: CA output, ledger
//   ├── Relay (host process, --ingress=fd) — reads frames from socketpair
//   └── Build VM (macOS) — private link only (10.0.0.2/30)
//
// Run:
//   go build -o .dev/vz-uv-build .dev/vz-uv-build.go && \
//   codesign --force --sign "Developer ID Application: Jeffrey Edwards (WGWKU4C782)" \
//     --entitlements entitlements.plist .dev/vz-uv-build && \
//   ./.dev/vz-uv-build --skip-prep

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"warden/driver/vz"
)

func init() {
	runtime.LockOSThread()
}

// Boot script for the macOS build VM. Configures static networking on the
// isolated subnet, fetches CA from relay, then runs the build.
const macBootScript = `#!/bin/sh
LOG="/var/log/warden-boot.log"
exec >>"$LOG" 2>&1
echo "$(date): warden-boot starting"

# Static network config — relay is at 10.0.0.1, we are 10.0.0.2.
echo "$(date): configuring network..."
IFACE=""
for iface in $(ifconfig -l); do
    hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
    status=$(ifconfig "$iface" 2>/dev/null | grep "status: active")
    if [ -n "$hasether" ] && [ -n "$status" ]; then
        IFACE=$iface
        break
    fi
done

if [ -z "$IFACE" ]; then
    echo "$(date): no active interface, trying all ethernet"
    for iface in $(ifconfig -l); do
        hasether=$(ifconfig "$iface" 2>/dev/null | grep "ether")
        if [ -n "$hasether" ] && [ "$iface" != "lo0" ]; then
            IFACE=$iface
            break
        fi
    done
fi

if [ -z "$IFACE" ]; then
    echo "$(date): FATAL no ethernet interface"
    ifconfig -a 2>&1
    exit 1
fi

echo "$(date): using $IFACE"
ifconfig "$IFACE" inet 10.0.0.2 netmask 255.255.255.252 up
route add default 10.0.0.1 2>/dev/null || true

# DNS: macOS uses mDNSResponder as the system resolver, not /etc/resolv.conf.
# In the Installer Progress boot environment, mDNSResponder may not be running
# or configured. We set up DNS via multiple mechanisms:
mkdir -p /etc /var/run/mDNSResponder /etc/resolver

# resolv.conf (for tools that read it directly)
echo "nameserver 10.0.0.1" > /etc/resolv.conf

# /etc/resolver/ directory (macOS libresolv checks this)
echo "nameserver 10.0.0.1" > /etc/resolver/default

# Configure via scutil (pokes configd's DNS state if it's running)
scutil 2>/dev/null <<SCUTIL || true
d.init
d.add ServerAddresses * 10.0.0.1
set State:/Network/Service/warden/DNS
d.init
d.add Addresses * 10.0.0.2
d.add SubnetMasks * 255.255.255.252
d.add Router 10.0.0.1
d.add InterfaceName $IFACE
set State:/Network/Service/warden/IPv4
d.init
d.add PrimaryService State:/Network/Service/warden
set State:/Network/Global/IPv4
quit
SCUTIL

# Start mDNSResponder if not already running
if ! pgrep -x mDNSResponder >/dev/null 2>&1; then
    echo "$(date): starting mDNSResponder"
    launchctl load /System/Library/LaunchDaemons/com.apple.mDNSResponder.plist 2>/dev/null || \
        /usr/sbin/mDNSResponder 2>/dev/null &
    sleep 2
fi

# Verify DNS works
echo "$(date): testing DNS..."
if nslookup sh.rustup.rs 10.0.0.1 >/dev/null 2>&1; then
    echo "$(date): DNS via relay works (direct query)"
elif host sh.rustup.rs >/dev/null 2>&1; then
    echo "$(date): DNS via system resolver works"
else
    echo "$(date): DNS not working, trying dig"
    dig @10.0.0.1 sh.rustup.rs 2>&1 || true
fi

echo "$(date): waiting for relay..."
i=0
while ! curl -sf "http://10.0.0.1:8300/health" >/dev/null 2>&1 && [ $i -lt 60 ]; do
    sleep 1
    i=$((i + 1))
done

if ! curl -sf "http://10.0.0.1:8300/health" >/dev/null 2>&1; then
    echo "$(date): relay not reachable"
    exit 1
fi
echo "$(date): relay ready"

# Fetch CA certificate and install for curl/pip/cargo
echo "$(date): fetching CA cert..."
curl -sf "http://10.0.0.1:8300/ca.pem" -o /tmp/warden-ca.pem
if [ ! -s /tmp/warden-ca.pem ]; then
    echo "$(date): failed to fetch CA"
    exit 1
fi

# Create a combined CA bundle: system roots + our MITM CA.
# Write to /etc/ssl/cert.pem (the default OpenSSL/LibreSSL lookup path on macOS).
# This ensures ALL tools find our CA regardless of which env vars they honor.
echo "$(date): building combined CA bundle..."
security find-certificate -a -p /System/Library/Keychains/SystemRootCertificates.keychain \
    > /tmp/warden-ca-bundle.pem
cat /tmp/warden-ca.pem >> /tmp/warden-ca-bundle.pem
echo "$(date): bundle has $(grep -c 'BEGIN CERTIFICATE' /tmp/warden-ca-bundle.pem) certs"

# Install as the system default cert bundle (Rust openssl-probe reads this path)
cp /tmp/warden-ca-bundle.pem /etc/ssl/cert.pem
# Also place at Homebrew OpenSSL path in case any tool checks there
mkdir -p /usr/local/etc/openssl
cp /tmp/warden-ca-bundle.pem /usr/local/etc/openssl/cert.pem

# Set env vars as belt-and-suspenders (some tools check these first)
export SSL_CERT_FILE=/etc/ssl/cert.pem
export REQUESTS_CA_BUNDLE=/etc/ssl/cert.pem
export CARGO_HTTP_CAINFO=/etc/ssl/cert.pem
export CURL_CA_BUNDLE=/etc/ssl/cert.pem
export NODE_EXTRA_CA_CERTS=/tmp/warden-ca.pem
export RUSTUP_USE_CURL=1

# Trust the CA for SSL/TLS. macOS requires authorization for SecTrustSettings
# changes even headlessly. Fix: pre-authorize via authorizationdb, then set
# admin-domain trust with -p ssl (required for native-tls/SecTrust).
echo "$(date): authorizing trust settings..."
security authorizationdb write com.apple.trust-settings.admin allow 2>&1

echo "$(date): trusting CA (admin domain, ssl policy)..."
security add-trusted-cert -d -r trustRoot -p ssl \
    -k /Library/Keychains/System.keychain /tmp/warden-ca.pem 2>&1
echo "$(date): add-trusted-cert exit=$?"

# Verify with an HTTPS fetch that exercises SecTrustEvaluateWithError
# (the same code path rustup/native-tls uses)
echo "$(date): verifying HTTPS (SecureTransport)..."
if curl -sf --max-time 10 https://static.rust-lang.org/ -o /dev/null; then
    echo "$(date): HTTPS verification PASSED"
else
    echo "$(date): HTTPS FAILED"
    security verify-cert -c /tmp/warden-ca.pem 2>&1 || true
    curl -v --max-time 5 https://static.rust-lang.org/ 2>&1 | tail -10
fi

echo "$(date): CA setup complete"

# Verify Xcode Command Line Tools are installed.
# CLT provides: cc/ld (linker), system headers (/usr/include via SDKs),
# /usr/bin/git, /usr/bin/python3 (real, not the shim), make, and the
# macOS SDK. Without CLT, virtually no native compilation is possible
# since even basic C linking requires the SDK's libSystem.tbd.
# NOTE: CLT must be installed during image preparation (with direct NAT
# internet) because Apple's softwareupdate uses certificate pinning that
# the MITM relay cannot intercept.
if /usr/bin/xcode-select -p >/dev/null 2>&1; then
    echo "$(date): CLT installed at $(xcode-select -p)"
else
    echo "$(date): ERROR: CLT not installed. Run image prep with --install-clt"
    exit 1
fi

# Create output directory
mkdir -p /var/warden/output

# Run build, logging locally and streaming to relay
echo "$(date): launching build"
/bin/sh /usr/local/bin/warden-build > /var/log/warden-build.log 2>&1
EXIT=$?

# Stream the build log to relay for visibility
curl -sf -X POST --data-binary @/var/log/warden-build.log \
    "http://10.0.0.1:8300/v1/output" || true

curl -sf -X POST "http://10.0.0.1:8300/v1/complete?code=$EXIT" || true
echo "$(date): done (exit $EXIT)"
exit $EXIT
`

const buildScript = `#!/bin/sh
set -ex

export HOME=/var/root

echo "=== uv wheel build starting at $(date) ==="

# 1. Install Rust via rustup
echo "--- Installing rustup ---"
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | \
    sh -s -- -y --default-toolchain stable
. "$HOME/.cargo/env"
echo "rustc: $(rustc --version)"
echo "cargo: $(cargo --version)"

# 2. Create Python venv using system python3
echo "--- Creating Python venv ---"
/usr/bin/python3 -m venv /tmp/buildenv
. /tmp/buildenv/bin/activate
echo "python: $(python3 --version)"

# 3. Install build tools
echo "--- Installing build + maturin ---"
pip install --upgrade pip
pip install build maturin

# 4. Download uv source distribution from PyPI
echo "--- Downloading uv sdist ---"
pip download --no-binary :all: --no-deps uv -d /tmp/uv-sdist

# 5. Extract and build
echo "--- Extracting sdist ---"
cd /tmp/uv-sdist
SDIST=$(ls uv-*.tar.gz | head -1)
echo "Building from: $SDIST"
tar xzf "$SDIST"
SRCDIR=$(ls -d uv-*/ | head -1)
cd "$SRCDIR"

echo "--- Building wheel (this may take 20-30 minutes) ---"
python -m build --wheel --no-isolation -o /var/warden/output/

# 6. Done
echo "--- Build complete! ---"
ls -la /var/warden/output/
echo "=== uv wheel build finished at $(date) ==="
`

func logf(format string, args ...interface{}) {
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", ts, fmt.Sprintf(format, args...))
}

func main() {
	skipPrep := len(os.Args) > 1 && os.Args[1] == "--skip-prep"

	imgDir := "/Users/jeffe/.cache/warden/images/macos-289afe17c389a911"
	diskPath := filepath.Join(imgDir, "disk.img")

	logf("Image: %s", imgDir)

	if !skipPrep {
		logf("Installing build scripts on disk...")
		if err := installScripts(diskPath); err != nil {
			logf("ERROR: %v", err)
			os.Exit(1)
		}
	} else {
		logf("Skipping disk prep (--skip-prep)")
	}

	// Prepare shared volume for relay
	sharedDir, err := os.MkdirTemp("", "warden-relay-shared-")
	if err != nil {
		logf("ERROR: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(sharedDir)

	for _, sub := range []string{"ledger", "context"} {
		os.MkdirAll(filepath.Join(sharedDir, sub), 0755)
	}

	// Build and sign relay binary (native darwin/arm64 — runs on host).
	// CGO_ENABLED=1 is required so net.LookupHost uses the system resolver
	// (mDNSResponder) instead of pure-Go DNS which may be blocked by
	// endpoint security on macOS.
	logf("Building relay binary...")
	relayBin := filepath.Join(sharedDir, "relay")
	cmd := exec.Command("go", "build", "-o", relayBin, "./cmd/relay")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logf("ERROR building relay: %v", err)
		os.Exit(1)
	}
	signCmd := exec.Command("codesign", "--force", "--sign",
		"Developer ID Application: Jeffrey Edwards (WGWKU4C782)", relayBin)
	if out, err := signCmd.CombinedOutput(); err != nil {
		logf("WARN: codesign relay: %s: %v", string(out), err)
	}

	// Create isolated virtual network (socket pair)
	logf("Creating virtual network...")
	vnet, err := vz.NewVirtualNetwork()
	if err != nil {
		logf("ERROR: %v", err)
		os.Exit(1)
	}
	defer vnet.Close()

	// Start host-side relay process with --ingress=fd.
	// Relay reads raw Ethernet frames from the socketpair, provides DHCP/DNS/
	// HTTP(S) proxy to the build VM, and has native internet access.
	logf("Starting host relay (--ingress=fd)...")
	relayCmd := exec.Command(relayBin,
		"--ingress=fd",
		"--fd=3",
		"--subnet=10.0.0.0/30",
	)
	relayCmd.Env = append(os.Environ(),
		"LEDGER_DIR="+filepath.Join(sharedDir, "ledger"),
		"CONTEXT_DIR="+filepath.Join(sharedDir, "context"),
	)
	relayCmd.Stdout = os.Stdout
	relayCmd.Stderr = os.Stderr
	relayCmd.ExtraFiles = []*os.File{
		os.NewFile(uintptr(vnet.RelaySocketFD), "relay-socket"),
	}
	if err := relayCmd.Start(); err != nil {
		logf("ERROR starting relay: %v", err)
		os.Exit(1)
	}
	defer func() {
		relayCmd.Process.Signal(os.Interrupt)
		relayCmd.Wait()
	}()

	// Wait for relay to write CA cert (signals it's ready)
	logf("Waiting for relay to start...")
	caPath := filepath.Join(sharedDir, "ledger", "ca.cert.pem")
	ctx := context.Background()
	if err := waitForFile(ctx, caPath, 30); err != nil {
		logf("ERROR: relay did not start: %v", err)
		os.Exit(1)
	}
	logf("Relay ready (CA written)")

	// Boot macOS build VM — private link only (no NAT)
	logf("Booting macOS build VM (8 cores, 16GB)...")
	buildVM, err := vz.NewMacOSVM(vz.MacOSVMConfig{
		CPUs:               8,
		MemoryMB:           16384,
		DiskImagePath:      diskPath,
		AuxStoragePath:     filepath.Join(imgDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(imgDir, "hardware-model"),
		MachineIDPath:      filepath.Join(imgDir, "machine-id"),
		SharedDirPath:      "",
		SharedDirTag:       "",
		FileHandleSocketFD: vnet.BuildSocketFD,
		AttachNAT:          false,
	})
	if err != nil {
		logf("ERROR creating build VM: %v", err)
		os.Exit(1)
	}
	if err := buildVM.Start(); err != nil {
		logf("ERROR starting build VM: %v", err)
		os.Exit(1)
	}
	defer buildVM.Stop()
	logf("Build VM started. Tailing output...")

	// Tail the build output file that the relay writes to
	outputPath := filepath.Join(sharedDir, "ledger", "build-output.log")
	go tailFile(outputPath)

	// Handle ctrl-c
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	timeout := 90 * time.Minute
	timer := time.NewTimer(timeout)

	select {
	case <-timer.C:
		logf("TIMEOUT after %s", timeout)
		_ = buildVM.Stop()
		relayCmd.Process.Signal(os.Interrupt)
		checkLogs(diskPath)
		os.Exit(1)

	case sig := <-sigCh:
		logf("Caught %s, stopping...", sig)
		_ = buildVM.Stop()
		relayCmd.Process.Signal(os.Interrupt)
		checkLogs(diskPath)
		os.Exit(130)
	}
}

func installScripts(diskPath string) error {
	out, err := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen").CombinedOutput()
	if err != nil {
		return fmt.Errorf("attach: %s: %w", string(out), err)
	}

	data := "/Volumes/Data"
	baseDevice := strings.Fields(strings.Split(string(out), "\n")[0])[0]
	defer exec.Command("hdiutil", "detach", baseDevice, "-force").Run()

	// Write build script
	buildPath := filepath.Join(data, "usr/local/bin/warden-build")
	os.WriteFile(buildPath, []byte(buildScript), 0755)

	// Write boot script
	bootPath := filepath.Join(data, "usr/local/bin/warden-boot")
	os.WriteFile(bootPath, []byte(macBootScript), 0755)

	// Clear old logs
	os.Remove(filepath.Join(data, "private/var/log/warden-boot.log"))
	os.Remove(filepath.Join(data, "private/var/log/warden-build.log"))

	// Verify daemon plist exists
	plist := filepath.Join(data, "Library/Apple/System/Library/LaunchDaemons",
		"com.buildwarden.agent.plist")
	if _, err := os.Stat(plist); err != nil {
		logf("  WARNING: daemon plist missing: %v", err)
	}

	// Ensure output dir
	os.MkdirAll(filepath.Join(data, "var/warden/output"), 0755)

	logf("  Installed: warden-build, warden-boot")
	return nil
}


func waitForFile(ctx context.Context, path string, timeoutSec int) error {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", filepath.Base(path))
		}
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
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

func tailFile(path string) {
	var offset int64
	for {
		info, err := os.Stat(path)
		if err == nil && info.Size() > offset {
			f, err := os.Open(path)
			if err == nil {
				f.Seek(offset, 0)
				buf := make([]byte, info.Size()-offset)
				n, _ := f.Read(buf)
				if n > 0 {
					os.Stderr.Write(buf[:n])
				}
				offset += int64(n)
				f.Close()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func checkLogs(diskPath string) {
	time.Sleep(2 * time.Second)
	out, _ := exec.Command("hdiutil", "attach", diskPath,
		"-nobrowse", "-noverify", "-noautoopen", "-owners", "off").CombinedOutput()
	if len(out) == 0 {
		return
	}
	baseDevice := strings.Fields(strings.Split(string(out), "\n")[0])[0]
	defer exec.Command("hdiutil", "detach", baseDevice, "-force").Run()

	fmt.Fprintf(os.Stderr, "\n=== warden-boot.log ===\n")
	bootLog, _ := os.ReadFile("/Volumes/Data/private/var/log/warden-boot.log")
	os.Stderr.Write(bootLog)

	fmt.Fprintf(os.Stderr, "\n=== warden-build.log (last 80 lines) ===\n")
	buildLog, _ := os.ReadFile("/Volumes/Data/private/var/log/warden-build.log")
	lines := strings.Split(string(buildLog), "\n")
	if len(lines) > 80 {
		lines = lines[len(lines)-80:]
	}
	fmt.Fprintf(os.Stderr, "%s\n", strings.Join(lines, "\n"))
}

package qemu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// virtioWinStableURL is the signed virtio-win driver ISO (stable channel).
// Unlike Windows media, this has a stable, freely-downloadable URL.
const virtioWinStableURL = "https://fedorapeople.org/groups/virt/" +
	"virtio-win/direct-downloads/stable-virtio/virtio-win.iso"

// WindowsPrepOptions configures Windows image preparation.
type WindowsPrepOptions struct {
	// ISO is the Windows install media: a local path or an http(s) URL
	// (downloaded + cached + sha256-verified). Required.
	ISO string
	// VirtioISO is the virtio-win driver ISO (path or URL). Empty uses the
	// stable channel URL.
	VirtioISO string
	// Edition is the install.wim image name (default "Windows 11 Pro").
	Edition string
	// Arch is the guest arch, "arm64" or "amd64" (default host arch).
	Arch string
	// AdminUser is the ephemeral autologon admin (default "warden").
	AdminUser string
	// VirtioDriveLetter is where virtio-win mounts during setup (default
	// "E:"); tuned against the real installer in 3b.
	VirtioDriveLetter string
	// WorkDir is where the Autounattend + answer ISO are materialized
	// (default <cache>/warden/winprep).
	WorkDir string
}

// WindowsMedia holds the resolved artifacts needed to run an unattended install.
type WindowsMedia struct {
	InstallISO      string
	VirtioISO       string
	AutounattendISO string
}

// AcquireWindowsMedia resolves/caches the install and virtio-win ISOs and
// materializes an Autounattend answer ISO. This is the deterministic half of
// Windows image prep; the live install is InstallWindowsImage (3b).
func AcquireWindowsMedia(opts WindowsPrepOptions) (*WindowsMedia, error) {
	installISO, err := resolveWindowsISO(opts.ISO)
	if err != nil {
		return nil, err
	}
	virtioISO, err := resolveVirtioWin(opts.VirtioISO)
	if err != nil {
		return nil, err
	}

	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	edition := opts.Edition
	if edition == "" {
		edition = "Windows 11 Pro"
	}
	adminUser := opts.AdminUser
	if adminUser == "" {
		adminUser = "warden"
	}
	driveLetter := opts.VirtioDriveLetter
	if driveLetter == "" {
		driveLetter = "E:"
	}
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = filepath.Join(cacheBaseDir(), "warden", "winprep")
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, err
	}

	xml := generateAutounattend(autounattendConfig{
		Arch:              arch,
		Edition:           edition,
		AdminUser:         adminUser,
		AdminPassword:     driver.RandAlphaNum(16),
		VirtioDriveLetter: driveLetter,
		SeedLabel:         windowsSeedName,
	})
	answerDir := filepath.Join(workDir, "answer")
	if err := os.MkdirAll(answerDir, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(
		filepath.Join(answerDir, "Autounattend.xml"),
		[]byte(xml), 0644); err != nil {
		return nil, err
	}
	answerISO := filepath.Join(workDir, "autounattend.iso")
	if err := generateSeedISO(answerDir, answerISO, "UNATTEND"); err != nil {
		return nil, fmt.Errorf("building autounattend ISO: %w", err)
	}

	return &WindowsMedia{
		InstallISO:      installISO,
		VirtioISO:       virtioISO,
		AutounattendISO: answerISO,
	}, nil
}

// InstallWindowsImage runs the unattended install headlessly under qemu and
// returns the captured base image path. The Autounattend powers the VM off
// when prep completes and -no-reboot makes qemu exit, which is how the host
// detects completion.
//
// FIRST CUT (chunk 3b): device details — USB-mounted install/driver/answer
// ISOs, virtio target disk, pflash UEFI vars, boot order, and the virtio-win
// drive letter the Autounattend references — are tuned against the real
// installer. Validate on a Win11 Arm64 ISO before marking Supported.
func InstallWindowsImage(m *WindowsMedia, opts WindowsPrepOptions) (string, error) {
	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	qarch := qemuArchOf(arch)

	d := &Driver{}
	binary := d.qemuBinary(qarch)
	if _, err := exec.LookPath(binary); err != nil {
		return "", fmt.Errorf("qemu binary %s not found in PATH: %w", binary, err)
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return "", fmt.Errorf("qemu-img not found in PATH: %w", err)
	}
	accel := d.detectAccel()

	imagesDir := filepath.Join(cacheBaseDir(), "warden", "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return "", err
	}
	base := filepath.Join(imagesDir, "windows-"+arch+".qcow2")

	// Fresh target disk for the install.
	_ = os.Remove(base)
	if out, err := exec.Command(
		"qemu-img", "create", "-f", "qcow2", base, "64G").CombinedOutput(); err != nil {
		return "", fmt.Errorf("creating base disk: %s: %w", string(out), err)
	}

	// Writable UEFI vars (per-image copy) so the boot entry persists across
	// the install's several reboots; fall back to read-only -bios.
	codeFD := efiCodePath(qarch)
	varsFD := ""
	if tmpl := efiVarsPath(qarch); tmpl != "" {
		varsFD = filepath.Join(imagesDir, "windows-"+arch+"-vars.fd")
		if err := copyFile(tmpl, varsFD); err != nil {
			varsFD = ""
		}
	}

	args := windowsInstallArgs(qarch, accel, base, codeFD, varsFD, m)
	// Control channel: a QMP socket the driver owns, used to send the first
	// "press any key to boot from CD" keypress and to eject the install ISO
	// after WinPE so the guest's reboots boot the installed disk instead of
	// dropping to the UEFI shell. Separate from the WARDEN_QEMU_MONITOR debug
	// HMP socket below.
	qmpSock := filepath.Join(imagesDir, "windows-"+arch+"-qmp.sock")
	_ = os.Remove(qmpSock)
	args = append(args, "-qmp", "unix:"+qmpSock+",server,nowait")
	// Debug knob: WARDEN_QEMU_VNC=127.0.0.1:0 attaches a loopback VNC server so
	// a stuck headless boot can be watched. Unset keeps the install headless.
	if vnc := os.Getenv("WARDEN_QEMU_VNC"); vnc != "" {
		args = append(args, "-vnc", vnc)
	}
	// Debug knob: WARDEN_QEMU_MONITOR=/path/to.sock exposes the HMP monitor on
	// a unix socket (e.g. for `screendump`), for headless diagnosis.
	if mon := os.Getenv("WARDEN_QEMU_MONITOR"); mon != "" {
		args = append(args, "-monitor", "unix:"+mon+",server,nowait")
	}

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	fmt.Fprintf(os.Stderr,
		"[warden] running unattended Windows install (headless, up to %s)...\n",
		installTimeout())
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("windows install (qemu start): %w", err)
	}
	// Drive the install over QMP (best-effort; never fails the install).
	driveDone := make(chan struct{})
	go func() {
		defer close(driveDone)
		driveWindowsInstall(ctx, qmpSock)
	}()
	err := cmd.Wait()
	cancel()
	<-driveDone
	_ = os.Remove(qmpSock)
	if err != nil {
		return "", fmt.Errorf("windows install (qemu): %w", err)
	}
	return base, nil
}

// driveWindowsInstall connects to the install VM's QMP socket and performs the
// two hands-off actions the unattended flow needs from outside the guest:
//
//  1. clears the firmware's "Press any key to boot from CD" prompt with a short
//     burst of Enter keypresses over the first few seconds of boot, and
//  2. ejects the bootable install ISO (device usbcd0) on the first guest reset
//     — which happens when WinPE finishes applying the image and reboots — so
//     that reboot and every later OOBE reboot boot the installed disk's Windows
//     Boot Manager instead of dropping to the UEFI interactive shell.
//
// It is best-effort: any failure is logged and the install proceeds unaided.
func driveWindowsInstall(ctx context.Context, sockPath string) {
	conn := dialQMP(ctx, sockPath)
	if conn == nil {
		fmt.Fprintf(os.Stderr,
			"[warden] QMP control channel unavailable; install runs unaided\n")
		return
	}
	defer conn.Close()
	// Unblock a pending Decode when the install ends.
	go func() { <-ctx.Done(); _ = conn.Close() }()

	var wmu sync.Mutex
	enc := json.NewEncoder(conn)
	send := func(v any) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = enc.Encode(v)
	}

	dec := json.NewDecoder(conn)
	var greeting map[string]json.RawMessage
	if err := dec.Decode(&greeting); err != nil {
		return // socket closed before the QMP banner
	}
	send(map[string]any{"execute": "qmp_capabilities"})

	// The loopback VNC debug knob needs no password (nothing off-host can reach
	// 127.0.0.1). Only when WARDEN_QEMU_VNC_PASSWORD is set — e.g. to use macOS
	// Screen Sharing, which refuses no-auth servers — do we enable+set it. The
	// `-vnc ...,password=on` option only ENABLES auth; the secret is set here.
	if pw := os.Getenv("WARDEN_QEMU_VNC_PASSWORD"); pw != "" {
		send(map[string]any{
			"execute":   "set_password",
			"arguments": map[string]any{"protocol": "vnc", "password": pw},
		})
	}

	// Clear "Press any key to boot from CD". With the OS disk at bootindex=0
	// (empty on first boot) the firmware probes it first, so the prompt appears
	// only after that probe — we wait ~4s, then send a key every 1.5s for ~18s.
	// The key is Down-arrow, NOT Enter: the prompt accepts any key, but if a
	// press lands on Setup's UI (timing varies) Enter/Space would activate the
	// Cancel button and open a modal "quit?" dialog that PAUSES the install,
	// whereas an arrow key is inert on Setup's controls (and the answer file
	// overrides any locale-dropdown selection).
	bootKey := map[string]any{
		"execute": "send-key",
		"arguments": map[string]any{
			"keys": []any{map[string]any{"type": "qcode", "data": "down"}},
		},
	}
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(4 * time.Second):
		}
		for i := 0; i < 12; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			send(bootKey)
			time.Sleep(1500 * time.Millisecond)
		}
	}()

	// Watch events; eject the install ISO on the first guest reset.
	ejected := false
	for {
		var msg struct {
			Event string `json:"event"`
		}
		if err := dec.Decode(&msg); err != nil {
			return
		}
		if msg.Event == "RESET" && !ejected {
			ejected = true
			send(map[string]any{
				"execute":   "device_del",
				"arguments": map[string]any{"id": "usbcd0"},
			})
			fmt.Fprintf(os.Stderr,
				"[warden] ejected install ISO after WinPE reboot; "+
					"reboots now boot the installed disk\n")
		}
	}
}

// dialQMP connects to the QMP unix socket, retrying until it appears (qemu
// creates it at startup) or the deadline/context elapses.
func dialQMP(ctx context.Context, sockPath string) net.Conn {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("unix", sockPath); err == nil {
			return c
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
	return nil
}

// installTimeout is the wall-clock cap on the qemu install. Win11 (especially
// ARM64 under HVF) can take well over an hour to apply the image, run bcdboot,
// reboot several times, and complete OOBE, so the default is deliberately
// generous. WARDEN_QEMU_INSTALL_TIMEOUT_MIN overrides it (in minutes).
func installTimeout() time.Duration {
	if v := os.Getenv("WARDEN_QEMU_INSTALL_TIMEOUT_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 240 * time.Minute
}

// windowsInstallArgs builds the qemu argument list for the unattended install,
// modelled on the virtio-win project's known-good ARM64 recipe: ramfb display,
// USB keyboard/tablet, a user NIC, and the install/driver/answer media as
// usb-storage CD-ROMs with explicit format=raw,media=cdrom (WinPE reads USB
// mass storage in-box, and format=raw makes qemu present them as CD-ROMs).
func windowsInstallArgs(
	qarch, accel, base, codeFD, varsFD string, m *WindowsMedia,
) []string {
	args := []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, qarch),
		"-m", "8192",
		"-smp", "8",
		"-nodefaults",
		"-display", "none",
		"-device", "ramfb",
		"-device", "qemu-xhci,id=xhci",
		"-device", "usb-kbd,bus=xhci.0",
		"-device", "usb-tablet,bus=xhci.0",
		"-nic", "user,model=virtio-net-pci",
		"-device", "virtio-rng-pci",
	}
	// Firmware: pflash code (ro) + writable vars, else read-only -bios.
	if varsFD != "" {
		args = append(args,
			"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", codeFD),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", varsFD),
		)
	} else {
		args = append(args, "-bios", codeFD)
	}
	// Install ISO, virtio-win drivers, and the answer ISO as USB CD-ROMs. Each
	// gets a device id (usbcd0..2) so the control channel can eject the bootable
	// install ISO (usbcd0) after WinPE, and a bootindex so the install ISO
	// (bootindex=1) sits just behind the OS disk (bootindex=0) in boot order.
	for i, iso := range []string{m.InstallISO, m.VirtioISO, m.AutounattendISO} {
		id := fmt.Sprintf("cd%d", i)
		args = append(args,
			"-drive", fmt.Sprintf(
				"if=none,id=%s,format=raw,media=cdrom,readonly=on,file=%s", id, iso),
			"-device", fmt.Sprintf(
				"usb-storage,bus=xhci.0,drive=%s,id=usb%s,bootindex=%d", id, id, i+1),
		)
	}
	// OS install disk on emulated NVMe with bootindex=0. NVMe (stornvme.sys) is
	// in-box on both win-64 and win-arm64, so Setup detects the disk with no
	// driver injection — unlike virtio-blk, whose viostor driver is injected
	// from the virtio-win ISO via Autounattend DriverPaths, an injection whose
	// ISO drive letter is nondeterministic and so fails intermittently (the
	// empty disk-selection screen). The explicit device also carries
	// bootindex=0, so the firmware boots the installed Windows Boot Manager
	// first on every reboot instead of dropping to the UEFI shell (edk2 ignores
	// -boot order; per-device bootindex is the working lever).
	args = append(args,
		"-drive", fmt.Sprintf("if=none,id=osdisk,file=%s,format=qcow2", base),
		"-device", "nvme,drive=osdisk,serial=wardenwin,bootindex=0")
	return args
}

// qemuArchOf maps a Go arch to the qemu-system-<arch> token.
func qemuArchOf(goarch string) string {
	switch goarch {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	default:
		return goarch
	}
}

// goArchOf maps a qemu-system-<arch> token back to the Go arch used for cache
// filenames like windows-<arch>.qcow2 (the inverse of qemuArchOf).
func goArchOf(qarch string) string {
	switch qarch {
	case "aarch64":
		return "arm64"
	case "x86_64":
		return "amd64"
	default:
		return qarch
	}
}

// efiVarsPath locates a writable UEFI vars template for the arch, or "".
func efiVarsPath(arch string) string {
	var candidates []string
	switch arch {
	case "aarch64":
		candidates = []string{
			// brew's qemu pairs edk2-aarch64-code.fd with edk2-arm-vars.fd.
			"/opt/homebrew/share/qemu/edk2-arm-vars.fd",
			"/usr/share/qemu/edk2-arm-vars.fd",
			"/opt/homebrew/share/qemu/edk2-aarch64-vars.fd",
			"/usr/share/qemu/edk2-aarch64-vars.fd",
			"/usr/share/AAVMF/AAVMF_VARS.fd",
		}
	case "x86_64":
		candidates = []string{
			"/opt/homebrew/share/qemu/edk2-i386-vars.fd",
			"/usr/share/qemu/edk2-i386-vars.fd",
			"/usr/share/OVMF/OVMF_VARS.fd",
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// resolveWindowsISO resolves Windows install media to a local path.
func resolveWindowsISO(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf(
			"windows install media required: pass --iso <path-or-url>")
	}
	return resolveISORef(ref, "windows iso")
}

// resolveVirtioWin resolves the virtio-win ISO; empty ref uses the stable URL.
func resolveVirtioWin(ref string) (string, error) {
	return resolveISORef(virtioWinSource(ref), "virtio-win iso")
}

// virtioWinSource returns the stable-channel URL for an empty ref.
func virtioWinSource(ref string) string {
	if ref == "" {
		return virtioWinStableURL
	}
	return ref
}

func resolveISORef(ref, what string) (string, error) {
	if isURL(ref) {
		return downloadCachedISO(ref)
	}
	if _, err := os.Stat(ref); err != nil {
		return "", fmt.Errorf("%s not found: %s", what, ref)
	}
	abs, _ := filepath.Abs(ref)
	return abs, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// downloadCachedISO downloads (curl) + caches an ISO with a sha256 sidecar,
// reusing the cached copy when its hash still verifies.
func downloadCachedISO(url string) (string, error) {
	cached := cachedISOPath(url)
	if _, err := os.Stat(cached); err == nil {
		if verifyImageIntegrity(cached) {
			return cached, nil
		}
		_ = os.Remove(cached)
		_ = os.Remove(cached + ".sha256")
	}
	if err := downloadImage(url, cached); err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	return cached, nil
}

// cachedISOPath derives a stable cache filename from an ISO reference.
func cachedISOPath(ref string) string {
	safe := strings.NewReplacer(
		"/", "_", ":", "_", ".", "_", "?", "_", "&", "_", "=", "_",
	).Replace(ref)
	if len(safe) > 100 {
		sum := sha256.Sum256([]byte(ref))
		safe = safe[:80] + "_" + hex.EncodeToString(sum[:6])
	}
	dir := filepath.Join(cacheBaseDir(), "warden", "images")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, safe+".iso")
}

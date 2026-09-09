package qemu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	fmt.Fprintf(os.Stderr,
		"[warden] running unattended Windows install (headless, up to 90m)...\n")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("windows install (qemu): %w", err)
	}
	return base, nil
}

// windowsInstallArgs builds the qemu argument list for the unattended install.
func windowsInstallArgs(
	qarch, accel, base, codeFD, varsFD string, m *WindowsMedia,
) []string {
	args := []string{
		"-machine", fmt.Sprintf("virt,accel=%s", accel),
		"-cpu", cpuForAccel(accel, qarch),
		"-m", "4096",
		"-smp", "4",
		"-nodefaults",
		"-display", "none",
		"-no-reboot",
		"-device", "virtio-rng-pci",
		"-device", "virtio-gpu-pci",
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
	// Target base disk on virtio; Setup installs the virtio storage driver
	// (via the Autounattend DriverPaths) so it can write here.
	args = append(args,
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", base))
	// Install media, virtio-win drivers, and the answer disk as USB storage:
	// WinPE reads USB mass storage in-box, avoiding the virtio chicken-and-egg
	// for the boot media.
	args = append(args, "-device", "qemu-xhci,id=xhci")
	for i, iso := range []string{m.InstallISO, m.VirtioISO, m.AutounattendISO} {
		id := fmt.Sprintf("cd%d", i)
		args = append(args,
			"-drive", fmt.Sprintf(
				"file=%s,id=%s,media=cdrom,readonly=on,if=none", iso, id),
			"-device", fmt.Sprintf("usb-storage,bus=xhci.0,drive=%s", id),
		)
	}
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

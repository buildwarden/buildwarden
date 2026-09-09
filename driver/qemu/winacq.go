package qemu

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// virtioWinStableURL is the signed virtio-win driver ISO (stable channel).
// Unlike Windows media, this has a stable, freely-downloadable URL.
const virtioWinStableURL = "https://fedorapeople.org/groups/virt/" +
	"virtio-win/direct-downloads/stable-virtio/virtio-win.iso"

// errWindowsInstallNotImplemented marks the live unattended-install
// orchestration (boot qemu, run Setup, capture the qcow2) as the 3b milestone.
var errWindowsInstallNotImplemented = errors.New(
	"windows unattended install orchestration is not yet wired (chunk 3b)")

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
// captures the prepared base image. Not yet wired (chunk 3b): it needs real
// media to validate device attach order, install-completion detection, and
// image capture.
func InstallWindowsImage(_ *WindowsMedia, _ WindowsPrepOptions) (string, error) {
	return "", errWindowsInstallNotImplemented
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

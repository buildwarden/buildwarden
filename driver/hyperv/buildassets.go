package hyperv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// Asset resolution for a Hyper-V build. These are pure path/env lookups (no
// Hyper-V calls), so they live outside the Windows build tag and are testable
// on any platform. StartBuild composes them, then hands the resolved paths to
// runBuild.

// hypervCacheDir is where `warden setup --driver hyperv` and `warden image
// prepare --driver hyperv` land pinned, verified boot/image artifacts (per the
// design: built once in CI, downloaded + verified, never built per build).
// WARDEN_HYPERV_CACHE_DIR overrides the location (ops flexibility; hermetic
// tests).
func hypervCacheDir() string {
	if d := os.Getenv("WARDEN_HYPERV_CACHE_DIR"); d != "" {
		return d
	}
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "warden", "hyperv")
	}
	return filepath.Join(".", ".warden-cache", "hyperv")
}

// resolveRelayBootVHDX locates the static, read-only relay boot VHDX (the UKI
// disk). Resolution order: WARDEN_RELAY_BOOT_VHDX, then the pinned path under the
// hyperv cache. Returns an actionable error when neither is present.
func resolveRelayBootVHDX() (string, error) {
	if p := os.Getenv("WARDEN_RELAY_BOOT_VHDX"); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", driver.Fixable(
				"point WARDEN_RELAY_BOOT_VHDX at the relay boot VHDX, or unset it",
				"WARDEN_RELAY_BOOT_VHDX=%q not readable: %v", p, err)
		}
		return p, nil
	}
	pinned := filepath.Join(hypervCacheDir(), "relay-boot.vhdx")
	if _, err := os.Stat(pinned); err == nil {
		return pinned, nil
	}
	return "", driver.Fixable(
		"build it from the source tree: tools/relay-vm/hyperv/build.sh, "+
			"then build-boot-vhdx.sh",
		"relay boot VHDX not found (looked at $WARDEN_RELAY_BOOT_VHDX and %s)", pinned)
}

// resolveBuildBaseVHDX locates the read-only disk chain the build VM's
// per-build overlay is created off, base first; the last element is the
// overlay's parent. Resolution: --image (or WARDEN_HYPERV_BUILD_IMAGE), a disk
// that is already prepared, so a chain of one; otherwise the active prepared
// layer from `warden image prepare --driver hyperv`, a chain of its
// generalized base and the layer.
//
// The disk's format decides the VM generation, and the caller MUST honour it: a
// Gen1 (BIOS/MBR) disk cannot boot on a Gen2 (UEFI/GPT) VM, and vice versa.
//
//   - .vhd  -> Generation 1 (Microsoft's Windows Server eval VHD shape).
//   - .vhdx -> Generation 2 (UEFI, Secure Boot capable).
//
// WARDEN_HYPERV_BUILD_GEN (1 or 2) overrides the inferred generation for the
// uncommon case of a .vhdx that must boot Gen1, or a .vhd wrapped for Gen2.
func resolveBuildBaseVHDX(
	req *driver.BuildRequest,
) (chain []string, isWindows bool, generation int, err error) {
	isWindows = req != nil && req.GuestOS == "windows"

	candidate := ""
	if req != nil {
		candidate = req.Image
	}
	if candidate == "" {
		candidate = os.Getenv("WARDEN_HYPERV_BUILD_IMAGE")
	}
	if candidate == "" {
		return resolvePreparedChain(isWindows)
	}

	switch strings.ToLower(filepath.Ext(candidate)) {
	case ".vhd":
		generation = 1
	case ".vhdx":
		generation = 2
	default:
		return nil, isWindows, 0, fmt.Errorf(
			"build-guest image %q must be a .vhd (Gen1) or .vhdx (Gen2); "+
				"Hyper-V boots no other format "+
				"(convert a qcow2/raw with `qemu-img convert -O vhdx`)", candidate)
	}
	if v := os.Getenv("WARDEN_HYPERV_BUILD_GEN"); v != "" {
		switch v {
		case "1":
			generation = 1
		case "2":
			generation = 2
		default:
			return nil, isWindows, 0, fmt.Errorf(
				"WARDEN_HYPERV_BUILD_GEN=%q invalid (want 1 or 2)", v)
		}
	}
	if _, err := os.Stat(candidate); err != nil {
		return nil, isWindows, 0, fmt.Errorf(
			"build-guest image %q not readable: %w", candidate, err)
	}
	abs, _ := filepath.Abs(candidate)
	return []string{abs}, isWindows, generation, nil
}

// resolvePreparedChain returns the active prepared layer and its base. A
// pinned base that was never prepared, or a chain whose files have moved, is
// a fixable error: builds boot only prepared images.
func resolvePreparedChain(isWindows bool) ([]string, bool, int, error) {
	const fix = "warden image prepare --driver hyperv"
	pb, base, ok := activePrepared()
	if !ok {
		if _, pinned := ActiveImage(); pinned {
			return nil, isWindows, 0, driver.Fixable(fix,
				"the Hyper-V base image has not been prepared for builds yet")
		}
		return nil, isWindows, 0, driver.Fixable(
			fix+" --source <eval-image-url-or-path> (or pass --image <prepared disk>)",
			"no Hyper-V build-guest image")
	}
	for _, p := range []string{base.Path, pb.Path} {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return nil, isWindows, 0, driver.Fixable(fix,
				"prepared Hyper-V image chain is incomplete: %s is missing "+
					"(was it moved or deleted?)", p)
		} else if err != nil {
			return nil, isWindows, 0, err
		}
	}
	return []string{base.Path, pb.Path}, true, pb.Generation, nil
}

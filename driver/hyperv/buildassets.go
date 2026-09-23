package hyperv

import (
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

// hypervCacheDir is where `warden hyperv setup` and `warden image fetch` land
// pinned, verified boot/image artifacts (per the design: built once in CI,
// downloaded + verified, never built per build). WARDEN_HYPERV_CACHE_DIR
// overrides the location (ops flexibility; hermetic tests).
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
			return "", fmt.Errorf("WARDEN_RELAY_BOOT_VHDX=%q not readable: %w", p, err)
		}
		return p, nil
	}
	pinned := filepath.Join(hypervCacheDir(), "relay-boot.vhdx")
	if _, err := os.Stat(pinned); err == nil {
		return pinned, nil
	}
	return "", fmt.Errorf(
		"relay boot VHDX not found (looked at $WARDEN_RELAY_BOOT_VHDX and %s); "+
			"build it with tools/relay-vm/hyperv/build.sh + build-boot-vhdx.sh, "+
			"or wait for `warden hyperv setup` to download the pinned disk", pinned)
}

// resolveBuildBaseVHDX locates the read-only build-guest base image (the
// differencing parent for the per-build overlay). Until Hyper-V build-image
// tooling exists, a caller brings their own bootable disk via req.Image or
// WARDEN_HYPERV_BUILD_IMAGE.
//
// The disk's format decides the VM generation, and the caller MUST honour it: a
// Gen1 (BIOS/MBR) disk cannot boot on a Gen2 (UEFI/GPT) VM, and vice versa.
//
//   - .vhd  -> Generation 1. This is the shape Microsoft's Windows Server
//     Evaluation image ships in (MBR/BIOS), so that image boots as-is with no
//     custom construction and no mbr2gpt conversion. This is the deliberate
//     starting path for Windows builds: a real, signed OS with zero image
//     surgery, so any boot issue is ours, not our disk assembly.
//   - .vhdx -> Generation 2 (UEFI, Secure Boot capable): a Linux cloud image,
//     or a Windows image already converted to Gen2 by `warden image` prep.
//
// WARDEN_HYPERV_BUILD_GEN (1 or 2) overrides the inferred generation for the
// uncommon case of a .vhdx that must boot Gen1, or a .vhd wrapped for Gen2.
func resolveBuildBaseVHDX(req *driver.BuildRequest) (path string, isWindows bool, generation int, err error) {
	isWindows = req != nil && req.GuestOS == "windows"

	candidate := ""
	if req != nil {
		candidate = req.Image
	}
	if candidate == "" {
		candidate = os.Getenv("WARDEN_HYPERV_BUILD_IMAGE")
	}
	if candidate == "" {
		// Fall back to the image pinned by `warden image fetch` (its manifest
		// carries the format-derived generation and guest OS, so honour those
		// rather than re-inferring from the extension).
		if p, isWin, gen, ok := resolveActiveEvalImage(); ok {
			return p, isWin, gen, nil
		}
		return "", isWindows, 0, fmt.Errorf(
			"no build-guest base image: run `warden image fetch windows-server-2025` "+
				"to pin Microsoft's eval VHD, or pass --image <disk.vhd|.vhdx> / set "+
				"WARDEN_HYPERV_BUILD_IMAGE")
	}

	switch strings.ToLower(filepath.Ext(candidate)) {
	case ".vhd":
		generation = 1
	case ".vhdx":
		generation = 2
	default:
		return "", isWindows, 0, fmt.Errorf(
			"build-guest image %q must be a .vhd (Gen1, e.g. the Windows Server "+
				"eval image) or .vhdx (Gen2); Hyper-V boots no other format "+
				"(convert a qcow2/raw with `qemu-img convert -O vhdx`)", candidate)
	}
	if v := os.Getenv("WARDEN_HYPERV_BUILD_GEN"); v != "" {
		switch v {
		case "1":
			generation = 1
		case "2":
			generation = 2
		default:
			return "", isWindows, 0, fmt.Errorf(
				"WARDEN_HYPERV_BUILD_GEN=%q invalid (want 1 or 2)", v)
		}
	}
	if _, err := os.Stat(candidate); err != nil {
		return "", isWindows, 0, fmt.Errorf("build-guest image %q not readable: %w", candidate, err)
	}
	abs, _ := filepath.Abs(candidate)
	return abs, isWindows, generation, nil
}

// buildSeed generates the per-build provisioning media the build VM boots with:
// a cloud-init NoCloud (CIDATA) volume for Linux, or unattend media for Windows.
// It must carry the static build-network config (10.0.0.2/30, gateway
// 10.0.0.1), the relay CA trust step, and the warden-io agent that fetches the
// build script from the relay and reports completion.
//
// It is deliberately not implemented yet: the seed's cloud-init flavour is
// coupled to the chosen build-guest base image (Alpine tiny-cloud vs Ubuntu
// cloud-init differ, and the NoCloud datasource must be validated against a real
// Gen2 boot), so building it before that image is chosen would be unverifiable.
// This is the single remaining piece of the build-VM path.
func buildSeed(workDir, buildID string, isWindows bool, req *driver.BuildRequest) (seedISO, seedVHDX string, err error) {
	return "", "", fmt.Errorf(
		"build-VM seed generation not implemented yet: choose the build-guest base " +
			"image first (the cloud-init/unattend seed is built to match it), then " +
			"the CIDATA/unattend media, warden-io injection, and static-network + " +
			"CA-trust provisioning land here")
}

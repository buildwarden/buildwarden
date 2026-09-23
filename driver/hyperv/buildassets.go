package hyperv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// Asset resolution for a Hyper-V build. These are pure path/env lookups (no
// Hyper-V calls), so they live outside the Windows build tag and are testable
// on any platform. StartBuild composes them, then hands the resolved paths to
// runBuild.

// hypervCacheDir is where `warden hyperv setup` is expected to land pinned,
// verified boot/image artifacts (per the design: built once in CI, downloaded +
// verified, never built per build).
func hypervCacheDir() string {
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
// tooling exists, a caller brings their own bootable Gen2 VHDX via req.Image or
// WARDEN_HYPERV_BUILD_IMAGE. Returns the path and whether it is a Windows guest.
func resolveBuildBaseVHDX(req *driver.BuildRequest) (path string, isWindows bool, err error) {
	isWindows = req != nil && req.GuestOS == "windows"

	candidate := ""
	if req != nil {
		candidate = req.Image
	}
	if candidate == "" {
		candidate = os.Getenv("WARDEN_HYPERV_BUILD_IMAGE")
	}
	if candidate == "" {
		return "", isWindows, fmt.Errorf(
			"no build-guest base image: pass --image <bootable-gen2.vhdx> or set "+
				"WARDEN_HYPERV_BUILD_IMAGE. Hyper-V build-image tooling (a `warden "+
				"image` path producing a cloud-init-ready VHDX) is not built yet")
	}
	if filepath.Ext(candidate) != ".vhdx" {
		return "", isWindows, fmt.Errorf(
			"build-guest image %q is not a .vhdx; Hyper-V Gen2 boots VHDX only "+
				"(convert with `qemu-img convert -O vhdx -o subformat=fixed`)", candidate)
	}
	if _, err := os.Stat(candidate); err != nil {
		return "", isWindows, fmt.Errorf("build-guest image %q not readable: %w", candidate, err)
	}
	abs, _ := filepath.Abs(candidate)
	return abs, isWindows, nil
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

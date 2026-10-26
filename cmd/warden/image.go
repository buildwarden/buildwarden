package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

// `warden image` prepares and lists build-guest base images. One verb,
// `prepare`, covers every driver: it acquires the base (download, restore or
// install), verifies it, readies it for headless builds, and records it where
// `warden build --driver <d>` finds it. Every input a driver needs is a flag;
// nothing is prompted for. Re-running prepare is safe: cached work is reused.

var (
	flagImgOS             string
	flagImgSource         string
	flagImgSHA256         string
	flagImgAcceptUnpinned bool
	flagImgForce          bool
	flagImgName           string
	flagImgEdition        string
	flagImgVirtio         string
	flagImgArch           string
	flagImgKeepVMs        bool
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Prepare and list build-guest base images (VM drivers)",
}

var imagePrepareCmd = &cobra.Command{
	Use:   "prepare [source]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Acquire, verify and ready a build-guest base image",
	Long: `Acquire, verify and ready a build-guest base image for a VM driver.

The driver comes from --driver (or warden.toml); --os defaults per driver
(vz: macos, qemu: linux, hyperv: windows). [source] (or --source) is a local
path or URL:

  --driver vz     --os macos    macOS IPSW (default: latest supported).
                                An already-restored image has its warden
                                boot agent refreshed instead.
  --driver qemu   --os linux    cloud image reference (alpine:3.21,
                                ubuntu:24.04, debian:bookworm) or a qcow2.
  --driver qemu   --os windows  Windows install ISO (required), installed
                                unattended (--edition, --virtio, --arch).
  --driver hyperv --os windows  Windows Server evaluation VHD/VHDX: a URL,
                                or a local file registered in place (needed
                                on first prepare; --name picks the image).
                                Then prepares it: one network-free boot
                                through OOBE and tuning, producing the
                                layer builds boot from. With no source,
                                re-prepares the pinned base (the refresh).

--sha256 pins the expected digest (verified, and recorded for every later
prepare); --accept-unpinned uses an image with no known digest; --force
re-acquires even when a valid cached copy exists.`,
	Example: `  warden image prepare --driver vz
  warden image prepare --driver qemu ubuntu:24.04
  warden image prepare --driver qemu --os windows --source Win11_Arm64.iso
  warden image prepare --driver hyperv --source https://.../server2025.vhd
  warden image prepare --driver hyperv --sha256 <hash-printed-on-first-prepare>
  warden image prepare --driver hyperv --source D:\images\server2025.vhdx --accept-unpinned
  warden image prepare --driver hyperv`,
	RunE: runImagePrepare,
}

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List prepared build-guest base images for every driver",
	Args:  cobra.NoArgs,
	RunE:  runImageList,
}

func init() {
	f := imagePrepareCmd.Flags()
	f.StringVar(&flagImgOS, "os", "", "guest OS: macos, linux or windows (default per driver)")
	f.StringVar(&flagImgSource, "source", "",
		"image source: local path or URL (same as [source])")
	f.StringVar(&flagImgSHA256, "sha256", "", "expected SHA256; verified and pinned")
	f.BoolVar(&flagImgAcceptUnpinned, "accept-unpinned", false,
		"use an image with no known digest (no authenticity check)")
	f.BoolVar(&flagImgForce, "force", false, "re-acquire even if a valid cached copy exists")
	f.StringVar(&flagImgName, "name", "",
		"registry name of the image (hyperv; default windows-server-2025)")
	f.StringVar(&flagImgEdition, "edition", "",
		"Windows edition / install.wim image name (qemu windows)")
	f.BoolVar(&flagImgKeepVMs, "keep-vms", false,
		"leave the prepare VM and its disk for inspection (hyperv)")
	f.StringVar(&flagImgVirtio, "virtio", "",
		"virtio-win driver ISO path or URL (qemu windows)")
	f.StringVar(&flagImgArch, "arch", "", "guest arch: arm64 or amd64 (default host arch)")

	imageCmd.AddCommand(imagePrepareCmd)
	imageCmd.AddCommand(imageListCmd)
	rootCmd.AddCommand(imageCmd)
}

func defaultImageOS(drv string) string {
	switch drv {
	case "vz":
		return "macos"
	case "hyperv":
		return "windows"
	default:
		return "linux"
	}
}

func runImagePrepare(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	drv := cfg.Runtime.Driver
	osName := flagImgOS
	if osName == "" {
		osName = defaultImageOS(drv)
	}
	source, err := imageSource(args)
	if err != nil {
		return err
	}

	var path string
	switch drv + "/" + osName {
	case "vz/macos":
		path, err = prepareVZMacos(source)
	case "qemu/linux":
		path, err = qemu.PrepareLinuxImage(source)
	case "qemu/windows":
		path, err = prepareQemuWindows(source)
	case "hyperv/windows":
		path, err = prepareHyperVWindows(cmd.Context(), source)
	default:
		return driver.Fixable("",
			"no %s image for --driver %q "+
				"(vz: macos; qemu: linux, windows; hyperv: windows)",
			osName, drv)
	}
	if err != nil {
		return err
	}
	// An empty path is the "already prepared" short-circuit: the helper has
	// printed its own message, so there is nothing more to report.
	if path != "" {
		fmt.Fprintf(os.Stderr, "Base image ready: %s\n", path)
	}
	return nil
}

// imageSource resolves the image source from --source and the positional [source]
// argument, rejecting a conflicting pair.
func imageSource(args []string) (string, error) {
	source := flagImgSource
	if len(args) > 0 {
		if source != "" && source != args[0] {
			return "", driver.Fixable("", "give the source once: [source] or --source")
		}
		source = args[0]
	}
	return source, nil
}

// prepareVZMacos restores (or refreshes) the macOS base image for the vz driver.
func prepareVZMacos(source string) (string, error) {
	// Every vz prepare path writes root-owned files into the image, and
	// restoring needs the virtualization entitlement: check both before
	// anything is downloaded.
	if err := vz.Preflight(); err != nil {
		return "", err
	}
	if err := vz.RequireRoot(); err != nil {
		return "", err
	}
	cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}
	existing, _ := cache.ImageDir()
	path, err := cache.RestoreIPSW(source)
	if err == nil && existing != "" && filepath.Dir(path) == existing {
		// Already restored: refresh the warden boot agent to this build.
		fmt.Fprintf(os.Stderr, "Refreshing boot agent on %s\n", existing)
		err = vz.ReprepareImage(existing)
	}
	return path, err
}

// prepareQemuWindows readies the qemu Windows image in its two stages: the
// install from ISO (ending in sysprep /generalize) when --source asks for it
// or there is no base yet, then the prepared layer builds boot from. With no
// source it only re-prepares the layer, which is what the periodic refresh
// runs.
func prepareQemuWindows(source string) (string, error) {
	arch := flagImgArch
	if arch == "" {
		arch = runtime.GOARCH
	}
	_, statErr := os.Stat(qemu.WindowsImagePath(arch))
	installed := statErr == nil
	switch {
	case source == "" && !installed:
		return "", driver.Fixable(
			"warden image prepare --driver qemu --os windows --source <path-or-url>",
			"--driver qemu --os windows needs the Windows install ISO")
	case source != "" && installed && !flagImgForce:
		fmt.Fprintf(os.Stderr, "Windows base already installed: %s "+
			"(--force reinstalls it); preparing it\n", qemu.WindowsImagePath(arch))
	case source != "":
		opts := qemu.WindowsPrepOptions{
			ISO: source, VirtioISO: flagImgVirtio, Edition: flagImgEdition, Arch: arch,
		}
		media, err := qemu.AcquireWindowsMedia(opts)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(os.Stderr, "Running unattended Windows install (headless)...\n")
		if _, err := qemu.InstallWindowsImage(media, opts); err != nil {
			return "", err
		}
	}
	layer, err := qemu.PrepareWindowsLayer(arch, flagImgKeepVMs, os.Stderr)
	if errors.Is(err, qemu.ErrPrepareRunning) {
		fmt.Fprintln(os.Stderr, "A qemu Windows image prepare is already running; "+
			"its result will become the active image when it finishes.")
		return "", nil
	}
	return layer, err
}

// prepareHyperVWindows readies the hyperv Windows base: it fetches (or
// registers, or pins) the generalized eval image when a source, digest or
// --force asks for it or none is pinned yet, then prepares the layer builds
// boot from. With nothing to fetch it only re-prepares, which is what the
// periodic refresh runs.
func prepareHyperVWindows(ctx context.Context, source string) (string, error) {
	name := strings.TrimSpace(flagImgName)
	if name == "" {
		name = hyperv.EvalImageNames()[0]
	}
	if _, pinned := hyperv.ActiveImage(); source != "" || flagImgSHA256 != "" ||
		flagImgForce || !pinned {
		if _, err := hyperv.FetchEvalImage(ctx, hyperv.FetchOptions{
			Name:           name,
			URL:            source,
			SHA256:         flagImgSHA256,
			AcceptUnpinned: flagImgAcceptUnpinned,
			Force:          flagImgForce,
			Progress:       os.Stderr,
		}); err != nil {
			return "", err
		}
	}
	pb, err := hyperv.PrepareBase(ctx, hyperv.PrepareOptions{
		KeepVM: flagImgKeepVMs, Progress: os.Stderr,
	})
	if errors.Is(err, hyperv.ErrPrepareRunning) {
		fmt.Fprintln(os.Stderr, "A Hyper-V image prepare is already running; "+
			"its result will become the active base when it finishes.")
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return pb.Path, nil
}

func runImageList(_ *cobra.Command, _ []string) error {
	found := false
	if runtime.GOOS == "darwin" {
		cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}
		if dir, err := cache.ImageDir(); err == nil && dir != "" {
			fmt.Printf("vz      macos    %s\n", dir)
			found = true
		}
	}
	qemuImages, _ := filepath.Glob(filepath.Join(driver.CacheDir(), "images", "*.qcow2"))
	for _, p := range qemuImages {
		osName := "linux"
		if strings.HasPrefix(filepath.Base(p), "windows-") {
			osName = "windows"
		}
		fmt.Printf("qemu    %-8s %s\n", osName, p)
		found = true
	}
	if st := hyperv.Status(); st.Base != "" {
		pin := "pinned"
		if !st.Pinned {
			pin = "UNPINNED"
		}
		fmt.Printf("hyperv  windows  base     %s (%s)\n", st.Base, pin)
		fmt.Printf("hyperv  windows  prepared %s\n", st.Summary(time.Now()))
		for _, p := range st.Older {
			fmt.Printf("hyperv  windows  older    %s (deleted by the next prepare)\n",
				p.Path)
		}
		found = true
	}
	if !found {
		fmt.Println("No prepared images. Run 'warden image prepare --driver <driver>'.")
	}
	return nil
}

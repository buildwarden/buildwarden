package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
  --driver hyperv --os windows  Windows Server evaluation VHD URL (needed
                                on first prepare; --name picks the image).

--sha256 pins the expected digest (verified, and recorded for every later
prepare); --accept-unpinned uses an image with no known digest; --force
re-acquires even when a valid cached copy exists.`,
	Example: `  warden image prepare --driver vz
  warden image prepare --driver qemu ubuntu:24.04
  warden image prepare --driver qemu --os windows --source Win11_Arm64.iso
  warden image prepare --driver hyperv --source https://.../server2025.vhd
  warden image prepare --driver hyperv --sha256 <hash-printed-on-first-prepare>`,
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
	f.StringVar(&flagImgSource, "source", "", "image source: local path or URL (same as [source])")
	f.StringVar(&flagImgSHA256, "sha256", "", "expected SHA256; verified and pinned")
	f.BoolVar(&flagImgAcceptUnpinned, "accept-unpinned", false,
		"use an image with no known digest (no authenticity check)")
	f.BoolVar(&flagImgForce, "force", false, "re-acquire even if a valid cached copy exists")
	f.StringVar(&flagImgName, "name", "", "registry name of the image (hyperv; default windows-server-2025)")
	f.StringVar(&flagImgEdition, "edition", "", "Windows edition / install.wim image name (qemu windows)")
	f.StringVar(&flagImgVirtio, "virtio", "", "virtio-win driver ISO path or URL (qemu windows)")
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
	source := flagImgSource
	if len(args) > 0 {
		if source != "" && source != args[0] {
			return fmt.Errorf("give the source once: [source] or --source")
		}
		source = args[0]
	}

	var path string
	switch drv + "/" + osName {
	case "vz/macos":
		cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}
		existing, _ := cache.ImageDir()
		path, err = cache.RestoreIPSW(source)
		if err == nil && existing != "" && filepath.Dir(path) == existing {
			// Already restored: refresh the warden boot agent to this build.
			fmt.Fprintf(os.Stderr, "Refreshing boot agent on %s\n", existing)
			err = vz.ReprepareImage(existing)
		}

	case "qemu/linux":
		path, err = qemu.PrepareLinuxImage(source)

	case "qemu/windows":
		if source == "" {
			return fmt.Errorf("--driver qemu --os windows needs the Windows install ISO: " +
				"warden image prepare --driver qemu --os windows --source <path-or-url>")
		}
		if existing := qemu.WindowsImagePath(flagImgArch); !flagImgForce {
			if _, statErr := os.Stat(existing); statErr == nil {
				fmt.Fprintf(os.Stderr, "Windows base image already prepared: %s "+
					"(--force reinstalls it)\n", existing)
				return nil
			}
		}
		opts := qemu.WindowsPrepOptions{
			ISO: source, VirtioISO: flagImgVirtio, Edition: flagImgEdition, Arch: flagImgArch,
		}
		var media *qemu.WindowsMedia
		if media, err = qemu.AcquireWindowsMedia(opts); err == nil {
			fmt.Fprintf(os.Stderr, "Running unattended Windows install (headless)...\n")
			path, err = qemu.InstallWindowsImage(media, opts)
		}

	case "hyperv/windows":
		name := strings.TrimSpace(flagImgName)
		if name == "" {
			name = hyperv.EvalImageNames()[0]
		}
		path, err = hyperv.FetchEvalImage(cmd.Context(), hyperv.FetchOptions{
			Name:           name,
			URL:            source,
			SHA256:         flagImgSHA256,
			AcceptUnpinned: flagImgAcceptUnpinned,
			Force:          flagImgForce,
			Progress:       os.Stderr,
		})

	default:
		return fmt.Errorf("no %s image for --driver %q (vz: macos; qemu: linux, windows; hyperv: windows)",
			osName, drv)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Base image ready: %s\n", path)
	return nil
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
	if p, ok := hyperv.ActiveImage(); ok {
		fmt.Printf("hyperv  windows  %s (active)\n", p)
		found = true
	}
	if !found {
		fmt.Println("No prepared images. Run 'warden image prepare --driver <driver>'.")
	}
	return nil
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

var (
	flagImageOS      string
	flagImageISO     string
	flagImageVirtio  string
	flagImageEdition string
	flagImageArch    string

	flagFetchURL            string
	flagFetchSHA256         string
	flagFetchAcceptUnpinned bool
	flagFetchForce          bool
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage VM base images for the vz driver",
}

var imageRestoreCmd = &cobra.Command{
	Use:   "restore [ipsw-url]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Download and restore a macOS IPSW to a VM image",
	Long: `Downloads a macOS IPSW restore image and installs it to a local
VM disk image suitable for use with the vz driver.

If no URL is provided, the latest supported IPSW is fetched from Apple.
The restored image is prepared for headless operation (no Setup Assistant,
auto-login warden user).

The image is cached and reused for all subsequent builds — each build
gets an instant copy-on-write clone.`,
	Example: `  warden image restore
  warden image restore https://updates.cdn-apple.com/.../Restore.ipsw`,
	RunE: runImageRestore,
}

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached VM images",
	RunE:  runImageList,
}

var imagePrepareCmd = &cobra.Command{
	Use:   "prepare",
	Short: "Re-install boot scripts on an existing image",
	Long: `Updates the warden-boot script and LaunchDaemon on an existing
prepared macOS VM image. Use after upgrading warden to pick up
the latest boot logic without doing a full IPSW restore.`,
	RunE: runImagePrepare,
}

func init() {
	imageRestoreCmd.Flags().StringVar(&flagImageOS, "os", "",
		"guest OS to prepare: macos (default, vz) or windows (qemu)")
	imageRestoreCmd.Flags().StringVar(&flagImageISO, "iso", "",
		"Windows install media (path or URL); required for --os windows")
	imageRestoreCmd.Flags().StringVar(&flagImageVirtio, "virtio", "",
		"virtio-win driver ISO (path or URL); default stable channel")
	imageRestoreCmd.Flags().StringVar(&flagImageEdition, "edition", "",
		"Windows edition / install.wim image name (default \"Windows 11 Pro\")")
	imageRestoreCmd.Flags().StringVar(&flagImageArch, "arch", "",
		"guest arch: arm64 or amd64 (default host arch)")

	imageFetchCmd.Flags().StringVar(&flagFetchURL, "url", "",
		"download URL for the image (required on first fetch of an eval VHD)")
	imageFetchCmd.Flags().StringVar(&flagFetchSHA256, "sha256", "",
		"expected SHA256 to verify against; pins the image when it matches")
	imageFetchCmd.Flags().BoolVar(&flagFetchAcceptUnpinned, "accept-unpinned", false,
		"use the image as the active base even without a pinned hash (no authenticity check)")
	imageFetchCmd.Flags().BoolVar(&flagFetchForce, "force", false,
		"re-download even if a valid cached copy exists")

	imageCmd.AddCommand(imageRestoreCmd)
	imageCmd.AddCommand(imageListCmd)
	imageCmd.AddCommand(imagePrepareCmd)
	imageCmd.AddCommand(imageFetchCmd)
	rootCmd.AddCommand(imageCmd)
}

var imageFetchCmd = &cobra.Command{
	Use:   "fetch [image-name]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Download, verify, and pin a Hyper-V build-guest base image",
	Long: `Downloads a build-guest base image (e.g. Microsoft's Windows Server
Evaluation VHD), verifies it against a pinned SHA256, and records it as the
active Hyper-V build-guest base so 'warden build' finds it automatically.

The Windows Server evaluation VHD is a Gen1 (BIOS/MBR) disk that boots as-is
with no conversion. It sits behind the Microsoft Evaluation Center (free
registration) and has no publicly published checksum, so on the first fetch you
supply its URL with --url; the tool prints the SHA256 it computed. Pin that hash
(re-run with --sha256 <hash>, which verifies the already-cached bytes without
re-downloading) to get an authenticity check on every future fetch.

Run with no arguments to list the known images.`,
	Example: `  warden image fetch
  warden image fetch windows-server-2025 --url https://.../server2025.vhd
  warden image fetch windows-server-2025 --sha256 <hash-printed-on-first-fetch>`,
	RunE: runImageFetch,
}

func runImageFetch(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Known build-guest base images:")
		for _, n := range hyperv.EvalImageNames() {
			if spec, ok := hyperv.LookupEvalImage(n); ok {
				fmt.Fprintf(os.Stderr, "  %-22s %s\n", n, spec.Version)
			}
		}
		fmt.Fprintln(os.Stderr, "\nFetch one with:\n  warden image fetch <name> --url <download-url>")
		return nil
	}
	path, err := hyperv.FetchEvalImage(cmd.Context(), hyperv.FetchOptions{
		Name:           strings.TrimSpace(args[0]),
		URL:            flagFetchURL,
		SHA256:         flagFetchSHA256,
		AcceptUnpinned: flagFetchAcceptUnpinned,
		Force:          flagFetchForce,
		Progress:       os.Stderr,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Build-guest base image ready: %s\n", path)
	return nil
}

func runImageRestore(_ *cobra.Command, args []string) error {
	if flagImageOS == "windows" {
		return runWindowsImageRestore()
	}
	if flagImageOS != "" && flagImageOS != "macos" {
		return fmt.Errorf(
			"unknown --os %q (want \"macos\" or \"windows\")", flagImageOS)
	}

	cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}

	ipswURL := ""
	if len(args) > 0 {
		ipswURL = args[0]
	}

	diskPath, err := cache.RestoreIPSW(ipswURL)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Image ready: %s\n", diskPath)
	return nil
}

// runWindowsImageRestore acquires Windows install media and materializes the
// Autounattend answer ISO. The live unattended install + image capture is the
// 3b milestone (see qemu.InstallWindowsImage).
func runWindowsImageRestore() error {
	opts := qemu.WindowsPrepOptions{
		ISO:       flagImageISO,
		VirtioISO: flagImageVirtio,
		Edition:   flagImageEdition,
		Arch:      flagImageArch,
	}
	media, err := qemu.AcquireWindowsMedia(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Windows install media prepared:\n")
	fmt.Fprintf(os.Stderr, "  install ISO:  %s\n", media.InstallISO)
	fmt.Fprintf(os.Stderr, "  virtio-win:   %s\n", media.VirtioISO)
	fmt.Fprintf(os.Stderr, "  Autounattend: %s\n", media.AutounattendISO)

	fmt.Fprintf(os.Stderr, "\nRunning unattended install (headless)...\n")
	image, err := qemu.InstallWindowsImage(media, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Windows base image ready: %s\n", image)
	return nil
}

func runImagePrepare(_ *cobra.Command, _ []string) error {
	cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}

	dir, err := cache.ImageDir()
	if err != nil {
		return err
	}
	if dir == "" {
		// No prepared image — look for one without .prepared marker
		entries, _ := os.ReadDir(cache.CacheDir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			d := filepath.Join(cache.CacheDir, e.Name())
			disk := filepath.Join(d, "disk.img")
			if info, err := os.Stat(disk); err == nil && info.Size() > 0 {
				dir = d
				break
			}
		}
	}
	if dir == "" {
		return fmt.Errorf("no image found — run 'warden image restore' first")
	}

	fmt.Fprintf(os.Stderr, "Re-preparing image: %s\n", dir)
	if err := vz.ReprepareImage(dir); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Done.\n")
	return nil
}

func runImageList(_ *cobra.Command, _ []string) error {
	cache := &vz.ImageCache{CacheDir: vz.DefaultCacheDir()}

	dir, err := cache.ImageDir()
	if err != nil {
		return err
	}
	if dir == "" {
		fmt.Println("No cached VM images.")
		fmt.Println("Run 'warden image restore' to download and prepare one.")
		return nil
	}

	fmt.Printf("Cached image: %s\n", dir)
	return nil
}

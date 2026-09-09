package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

var (
	flagImageOS      string
	flagImageISO     string
	flagImageVirtio  string
	flagImageEdition string
	flagImageArch    string
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

	imageCmd.AddCommand(imageRestoreCmd)
	imageCmd.AddCommand(imageListCmd)
	imageCmd.AddCommand(imagePrepareCmd)
	rootCmd.AddCommand(imageCmd)
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

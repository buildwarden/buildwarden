package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"warden/driver/vz"
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
	imageCmd.AddCommand(imageRestoreCmd)
	imageCmd.AddCommand(imageListCmd)
	imageCmd.AddCommand(imagePrepareCmd)
	rootCmd.AddCommand(imageCmd)
}

func runImageRestore(_ *cobra.Command, args []string) error {
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

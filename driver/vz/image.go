package vz

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ImageCache manages cached VM base images.
type ImageCache struct {
	CacheDir string
}

// LatestIPSW returns the path to the latest cached macOS IPSW restore image.
// If no cached image exists, it downloads and restores the latest from Apple.
func (c *ImageCache) LatestIPSW() (string, error) {
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	// Check for existing restored image
	entries, _ := os.ReadDir(c.CacheDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".img" && !e.IsDir() {
			return filepath.Join(c.CacheDir, e.Name()), nil
		}
	}

	// TODO: Download latest IPSW from Apple's software update catalog
	// and restore it using VZMacOSRestoreImage + VZMacOSInstaller
	return "", fmt.Errorf("no cached macOS image found; IPSW restore not yet implemented")
}

// RestoreIPSW downloads an IPSW file from the given URL and restores it
// to a bootable disk image in the cache.
func (c *ImageCache) RestoreIPSW(ipswURL string) (string, error) {
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}
	// TODO: implement IPSW download + VZMacOSInstaller restore
	_ = ipswURL
	return "", fmt.Errorf("IPSW restore not yet implemented")
}

// CloneDisk creates an APFS copy-on-write clone of a base disk image.
// This is effectively instant and uses no additional disk space until
// the clone diverges from the base.
func CloneDisk(src, dst string) error {
	cmd := exec.Command("cp", "-c", src, dst)
	if err := cmd.Run(); err != nil {
		// Fallback to regular copy if APFS cloning not available
		cmd = exec.Command("cp", src, dst)
		return cmd.Run()
	}
	return nil
}

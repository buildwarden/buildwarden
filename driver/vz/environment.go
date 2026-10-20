package vz

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// vzPlatformFiles are the files beside disk.img the build VM boots with: the
// NVRAM (auxiliary storage), then the platform identity files.
var vzPlatformFiles = []struct{ name, role string }{
	{"aux-storage", ledger.LayerRoleNVRAM},
	{"hardware-model", ledger.LayerRolePlatform},
	{"machine-id", ledger.LayerRolePlatform},
}

// LayerCache is the vz driver's layer-hash cache, in its image cache.
func (c *ImageCache) LayerCache() *driver.LayerCache {
	return &driver.LayerCache{
		Path:     filepath.Join(c.CacheDir, driver.LayerCacheFile),
		Progress: os.Stderr,
	}
}

// describeBuild describes what the build VM boots from as a vm-disk-chain/v1
// environment: the base disk.img, its NVRAM and platform files, and the build
// script (warden-io is baked into the prepared image, so it is part of the
// disk, not the seed).
func describeBuild(
	cache *driver.LayerCache, diskImage, buildScript string,
) (*relaycfg.Environment, error) {
	b := driver.NewDiskChain(cache, "vz", "macos", "arm64", 0)
	if err := b.AddLayer(ledger.LayerRoleDisk, diskImage, "raw"); err != nil {
		return nil, err
	}
	dir := filepath.Dir(diskImage)
	for _, f := range vzPlatformFiles {
		p := filepath.Join(dir, f.name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := b.AddLayer(f.role, p, ""); err != nil {
			return nil, err
		}
	}
	if buildScript != "" {
		if err := b.AddSeedFile(driver.SeedBuildScript, buildScript); err != nil {
			return nil, err
		}
	}
	return b.Environment()
}

// recordPreparedImage records a prepared image directory's boot files as
// warden-prepared layers whose inputs are the IPSW; nil inputs keep whatever
// was recorded before (a re-prepare of the same image).
func (c *ImageCache) recordPreparedImage(imgDir string, inputs []ledger.PreparationInput) error {
	cache := c.LayerCache()
	disk := filepath.Join(imgDir, "disk.img")
	if inputs == nil {
		if prev, ok := cache.Source(disk); ok && prev.Kind == ledger.SourcePrepared {
			inputs = prev.Inputs
		}
	}
	src := ledger.LayerSource{
		Kind: ledger.SourcePrepared, PreparedBy: driver.PreparedBy(), Inputs: inputs,
	}
	files := []string{disk}
	for _, f := range vzPlatformFiles {
		files = append(files, filepath.Join(imgDir, f.name))
	}
	for _, p := range files {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "Hashing %s...\n", filepath.Base(p))
		sum, err := driver.HashFileSHA256(p)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", p, err)
		}
		if err := cache.Record(p, sum, "", src); err != nil {
			return err
		}
	}
	return nil
}

// ipswInput describes the IPSW an image was restored from.
func ipswInput(ipswURL, ipswLocal string) []ledger.PreparationInput {
	in := ledger.PreparationInput{Name: "ipsw", URL: ipswURL}
	if fi, err := os.Stat(ipswLocal); err == nil {
		fmt.Fprintf(os.Stderr, "Hashing %s...\n", filepath.Base(ipswLocal))
		if sum, err := driver.HashFileSHA256(ipswLocal); err == nil {
			in.SHA256, in.Size = sum, fi.Size()
		}
	}
	return []ledger.PreparationInput{in}
}

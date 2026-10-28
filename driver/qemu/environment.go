package qemu

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// LayerCache is the qemu driver's layer-hash cache, beside its cached images.
func LayerCache() *driver.LayerCache {
	return &driver.LayerCache{
		Path:     filepath.Join(driver.CacheDir(), "images", driver.LayerCacheFile),
		Progress: os.Stderr,
	}
}

// buildEnvironment describes what the build VM boots from as a
// vm-disk-chain/v1 environment: the base disk (or the direct-boot kernel and
// initrd), the Windows UEFI vars, and the per-build seed plus build script.
func buildEnvironment(cfg *buildVMConfig, buildScript string) (*relaycfg.Environment, error) {
	return describeBuild(LayerCache(), cfg, buildScript)
}

func describeBuild(
	cache *driver.LayerCache, cfg *buildVMConfig, buildScript string,
) (*relaycfg.Environment, error) {
	guestOS := "linux"
	if cfg.Windows {
		guestOS = "windows"
	}
	b := driver.NewDiskChain(cache, "qemu", guestOS, goArchOf(cfg.Arch), 0)
	if cfg.BaseImage != "" {
		chain := cfg.BaseChain
		if len(chain) == 0 {
			chain = []string{cfg.BaseImage}
		}
		for _, disk := range chain {
			if err := b.AddLayer(ledger.LayerRoleDisk, disk, "qcow2"); err != nil {
				return nil, err
			}
		}
	} else {
		if err := b.AddLayer(ledger.LayerRoleKernel, cfg.Kernel, ""); err != nil {
			return nil, err
		}
		if err := b.AddLayer(ledger.LayerRoleInitrd, cfg.Initrd, ""); err != nil {
			return nil, err
		}
	}
	if cfg.BaseVarsFD != "" {
		if err := b.AddLayer(ledger.LayerRoleNVRAM, cfg.BaseVarsFD, "raw"); err != nil {
			return nil, err
		}
	}
	if cfg.SeedDir != "" {
		if err := b.AddSeedDir(cfg.SeedDir); err != nil {
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

// recordDownloadedImage records a freshly downloaded cloud image's hash and
// URL, so builds describe it without re-hashing and with its provenance.
func recordDownloadedImage(path, sha256Hex, url string) {
	err := LayerCache().Record(path, sha256Hex, "",
		ledger.LayerSource{Kind: ledger.SourceURL, URL: url})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warden] warning: %v\n", err)
	}
}

// recordPreparedWindows records the installed Windows base and its UEFI vars
// as warden-prepared layers whose inputs are the install media.
func recordPreparedWindows(base, varsFD string, m *WindowsMedia) error {
	var inputs []ledger.PreparationInput
	for _, in := range []struct{ name, path string }{
		{"install-iso", m.InstallISO},
		{"virtio-win-iso", m.VirtioISO},
		{"autounattend-iso", m.AutounattendISO},
	} {
		if in.path == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "[warden] hashing %s...\n", filepath.Base(in.path))
		sum, err := driver.HashFileSHA256(in.path)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", in.name, err)
		}
		inputs = append(inputs, ledger.PreparationInput{Name: in.name, SHA256: sum})
	}
	src := ledger.LayerSource{
		Kind: ledger.SourcePrepared, PreparedBy: driver.PreparedBy(), Inputs: inputs,
	}
	cache := LayerCache()
	for _, p := range []string{base, varsFD} {
		if p == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "[warden] hashing %s...\n", filepath.Base(p))
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

package firecracker

import (
	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Seed entry names beyond driver.SeedBuildScript.
const (
	seedKernelCmdline = "kernel-cmdline"
	seedAgent         = "warden-io"
)

// buildRecord is what the build VM boots from, for its environment record.
type buildRecord struct {
	Kernel string
	// Disk path: the cached raw image (the per-build copy is the writable
	// child, not a layer) and the CIDATA seed directory.
	Raw     string
	SeedDir string
	// Direct boot: the base initramfs and the warden-io appended to it.
	Initrd string
	Agent  string
	// Script is the staged build script; Cmdline a file holding the build
	// VM's kernel command line, which carries no secret.
	Script  string
	Cmdline string
}

// describeBuild is the build's vm-disk-chain/v1 record (Decision 14).
func describeBuild(cache *driver.LayerCache, r buildRecord) (*relaycfg.Environment, error) {
	b := driver.NewDiskChain(cache, "firecracker", "linux", "amd64", 0)
	if err := b.AddLayer(ledger.LayerRoleKernel, r.Kernel, ""); err != nil {
		return nil, err
	}
	if r.Raw != "" {
		if err := b.AddLayer(ledger.LayerRoleDisk, r.Raw, "raw"); err != nil {
			return nil, err
		}
		if err := b.AddSeedDir(r.SeedDir); err != nil {
			return nil, err
		}
	} else {
		if err := b.AddLayer(ledger.LayerRoleInitrd, r.Initrd, ""); err != nil {
			return nil, err
		}
		if err := b.AddSeedFile(seedAgent, r.Agent); err != nil {
			return nil, err
		}
	}
	if r.Script != "" {
		if err := b.AddSeedFile(driver.SeedBuildScript, r.Script); err != nil {
			return nil, err
		}
	}
	if err := b.AddSeedFile(seedKernelCmdline, r.Cmdline); err != nil {
		return nil, err
	}
	return b.Environment()
}

package driver

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// WardenVersion is the orchestrator's version, stamped on layers warden
// prepares. cmd/warden sets it at startup.
var WardenVersion = "dev"

// PreparedBy is the prepared_by value for a layer this warden produced.
func PreparedBy() string { return "warden " + WardenVersion }

// SeedBuildScript is the seed entry name for the staged build script.
const SeedBuildScript = "build-script"

// DiskChain assembles a vm-disk-chain/v1 environment descriptor for a VM
// driver: the files the build VM boots from (hashes via a LayerCache) and the
// per-build seed files handed to the guest (hashed every build).
type DiskChain struct {
	cache       *LayerCache
	desc        ledger.VMDiskChain
	lastDiskSHA string
}

// NewDiskChain starts a descriptor. generation is the Hyper-V VM generation
// (0 for drivers without one).
func NewDiskChain(cache *LayerCache, driverName, guestOS, arch string, generation int) *DiskChain {
	return &DiskChain{cache: cache, desc: ledger.VMDiskChain{
		Type: ledger.EnvTypeVMDiskChain, Driver: driverName,
		GuestOS: guestOS, Arch: arch, Generation: generation,
	}}
}

// AddLayer appends a boot file. Disk layers are added base-first: each is
// recorded as the child of the previous disk, so a cached hash is only reused
// while its parent is unchanged.
func (b *DiskChain) AddLayer(role, path, format string) error {
	parent := ""
	if role == ledger.LayerRoleDisk {
		parent = b.lastDiskSHA
	}
	info, err := b.cache.Lookup(path, parent)
	if err != nil {
		return err
	}
	if role == ledger.LayerRoleDisk {
		b.lastDiskSHA = info.SHA256
	}
	b.desc.Layers = append(b.desc.Layers, ledger.DiskLayer{
		Role: role, Name: filepath.Base(info.Path), Format: format, Path: info.Path,
		SHA256: info.SHA256, Size: info.Size, Source: info.Source,
	})
	return nil
}

// AddSeedFile hashes one per-build guest file under the given name.
func (b *DiskChain) AddSeedFile(name, path string) error {
	sum, err := HashFileSHA256(path)
	if err != nil {
		return fmt.Errorf("hashing seed %s: %w", name, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	b.desc.Seed = append(b.desc.Seed, ledger.SeedFile{
		Name: name, SHA256: sum, Size: fi.Size(),
	})
	return nil
}

// AddSeedDir hashes every regular file under dir, named by its slash-separated
// path relative to dir.
func (b *DiskChain) AddSeedDir(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		return b.AddSeedFile(filepath.ToSlash(rel), p)
	})
}

// Descriptor returns the assembled descriptor with its seed sorted by name.
func (b *DiskChain) Descriptor() *ledger.VMDiskChain {
	d := b.desc
	d.Seed = append([]ledger.SeedFile(nil), b.desc.Seed...)
	sort.Slice(d.Seed, func(i, j int) bool { return d.Seed[i].Name < d.Seed[j].Name })
	return &d
}

// Environment encodes the descriptor as the claim's environment record.
func (b *DiskChain) Environment() (*relaycfg.Environment, error) {
	d := b.Descriptor()
	payload, err := d.Marshal()
	if err != nil {
		return nil, err
	}
	meta, err := cbor.Marshal(map[string]any{"type": d.Type, "driver": d.Driver})
	if err != nil {
		return nil, fmt.Errorf("encoding environment metadata: %w", err)
	}
	return &relaycfg.Environment{Type: d.Type, Payload: payload, Metadata: meta}, nil
}

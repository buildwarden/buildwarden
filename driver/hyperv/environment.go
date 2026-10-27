package hyperv

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// LayerCache is the Hyper-V driver's layer-hash cache, beside the eval image
// manifest.
func LayerCache() *driver.LayerCache {
	return &driver.LayerCache{
		Path:     filepath.Join(evalImagesDir(), driver.LayerCacheFile),
		Progress: os.Stderr,
	}
}

// describeBuild describes what the build VM boots from as a vm-disk-chain/v1
// environment: the disk chain base first (the generalized base, then the
// prepared layer with its preparation inputs), the per-build seed (warden-io,
// warden-run.ps1) and the build script. AddLayer binds each child's cached
// hash to its parent's.
func describeBuild(
	cache *driver.LayerCache, chain []string, isWindows bool, generation int,
	seedDir, buildScript string,
) (*relaycfg.Environment, error) {
	guestOS := "linux"
	if isWindows {
		guestOS = "windows"
	}
	b := driver.NewDiskChain(cache, "hyperv", guestOS, runtime.GOARCH, generation)
	for _, disk := range chain {
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(disk)), ".")
		if err := b.AddLayer(ledger.LayerRoleDisk, disk, format); err != nil {
			return nil, err
		}
	}
	if seedDir != "" {
		if err := b.AddSeedDir(seedDir); err != nil {
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

// recordEvalLayer records a fetched eval image's provenance in the layer
// cache. A fresh download's streamed hash is recorded as is; a reused cached
// file is hashed (or its still-valid cache entry used) rather than trusting
// its .sha256 sidecar, and is not given the URL provenance if they disagree.
func recordEvalLayer(dest, gotHash, url string, pinned, fresh bool, progress io.Writer) {
	cache := LayerCache()
	cache.Progress = progress
	src := ledger.LayerSource{Kind: ledger.SourceURL, URL: url, Pinned: pinned}
	if url == "" {
		src = ledger.LayerSource{Kind: ledger.SourceLocal, Pinned: pinned}
	}
	if !fresh {
		info, err := cache.Lookup(dest, "")
		if err != nil {
			logf(progress, "warning: layer cache: %v\n", err)
			return
		}
		if info.SHA256 != gotHash {
			logf(progress, "warning: %s hashes to %s..., not its recorded %s...; "+
				"builds will describe it as a local file\n",
				filepath.Base(dest), short(info.SHA256), short(gotHash))
			return
		}
	}
	if err := cache.Record(dest, gotHash, "", src); err != nil {
		logf(progress, "warning: %v\n", err)
	}
}

// buildSeedDir is where stageSeedDir put this build's guest seed, or "" when
// there is none (the Linux guest path stages no seed today).
func buildSeedDir(workDir string) string {
	dir := filepath.Join(workDir, "seed")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	return dir
}

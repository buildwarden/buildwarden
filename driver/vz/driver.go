package vz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"warden/driver"
)

// Driver implements driver.Driver using Apple's Virtualization.framework
// for macOS/arm64 targets.
type Driver struct {
	Cache *ImageCache
}

func New() *Driver {
	cacheDir := defaultCacheDir()
	return &Driver{
		Cache: &ImageCache{CacheDir: cacheDir},
	}
}

func (d *Driver) Name() string { return "vz" }

func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
	if err := checkPlatform(); err != nil {
		return nil, err
	}

	buildID := driver.RandAlphaNum(8)
	subnet := driver.AllocateVMSubnet()

	// Prepare ledger directory
	ledgerDir := req.LedgerDir
	if ledgerDir == "" {
		var err error
		ledgerDir, err = os.MkdirTemp("", "warden-ledger-"+buildID+"-")
		if err != nil {
			return nil, fmt.Errorf("creating ledger dir: %w", err)
		}
	}

	// Prepare shared directory for host <-> VM communication
	sharedDir, err := os.MkdirTemp("", "warden-shared-"+buildID+"-")
	if err != nil {
		return nil, fmt.Errorf("creating shared dir: %w", err)
	}
	defer os.RemoveAll(sharedDir)

	// Place warden-io binary in shared directory
	if err := d.prepareAgent(sharedDir); err != nil {
		return nil, fmt.Errorf("preparing agent: %w", err)
	}

	// Place build context and script in shared directory
	if err := d.prepareContext(sharedDir, req); err != nil {
		return nil, fmt.Errorf("preparing context: %w", err)
	}

	// Resolve and clone disk image
	diskImage, err := d.resolveImage(req.Image)
	if err != nil {
		return nil, fmt.Errorf("resolving image: %w", err)
	}

	clonePath := filepath.Join(sharedDir, "disk.img")
	if err := CloneDisk(diskImage, clonePath); err != nil {
		return nil, fmt.Errorf("cloning disk: %w", err)
	}

	// Create userspace network bridge
	bridge, err := NewUserspaceBridge(subnet.RelayIP, subnet.BuildIP)
	if err != nil {
		return nil, fmt.Errorf("creating network bridge: %w", err)
	}
	defer bridge.Close()

	// Start relay on bridge listeners
	_ = ctx       // TODO: wire context for cancellation
	_ = ledgerDir // TODO: start relay with these listeners
	_ = bridge    // TODO: wire bridge to relay

	// Boot VM
	// TODO: create VM config, attach virtio-fs (sharedDir) and virtio-net (bridge)
	// TODO: start VM, wait for warden-io to signal completion

	return &driver.BuildResult{
		OutputDir: req.OutputDir,
	}, nil
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	if err := checkPlatform(); err != nil {
		return err
	}
	// TODO: implement interactive shell via serial console
	return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }

func (d *Driver) prepareAgent(sharedDir string) error {
	// warden-io for macOS is cross-compiled by the build system.
	// For dev mode, build it on the fly.
	agentPath := filepath.Join(sharedDir, "warden-io")
	// TODO: locate or build warden-io for darwin/arm64
	_ = agentPath
	return nil
}

func (d *Driver) prepareContext(sharedDir string, req *driver.BuildRequest) error {
	ctxDst := filepath.Join(sharedDir, "context")
	if err := os.MkdirAll(ctxDst, 0755); err != nil {
		return err
	}
	// TODO: copy or symlink context into shared directory
	_ = req
	return nil
}

func (d *Driver) resolveImage(image string) (string, error) {
	if image == "" {
		return d.Cache.LatestIPSW()
	}
	// If it's a path, use directly
	if _, err := os.Stat(image); err == nil {
		return image, nil
	}
	// Otherwise treat as IPSW URL
	return d.Cache.RestoreIPSW(image)
}

func checkPlatform() error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("vz driver requires macOS on Apple Silicon (got %s/%s)",
			runtime.GOOS, runtime.GOARCH)
	}
	return nil
}

func defaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "warden", "images")
}

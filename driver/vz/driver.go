package vz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"warden/driver"
)

// Driver implements driver.Driver using Apple's Virtualization.framework
// for macOS/arm64 targets.
//
// Architecture (two-VM topology):
//
//	Host (orchestrator)
//	├── Prepares shared volume: relay binary, config, context, ledger dir
//	├── Creates isolated virtual network (socket pair)
//	├── Boots Relay VM (Alpine Linux, static relay binary)
//	│   ├── Private interface → socket pair → Build VM
//	│   └── NAT interface → host/internet
//	├── Boots Build VM (macOS from IPSW)
//	│   └── Single interface → socket pair → Relay VM (sole gateway)
//	└── Waits for build completion, collects ledger + artifacts
//
// Network isolation is topological: the build VM's only network path is
// through the relay VM. No iptables, no host-side proxy needed.
type Driver struct {
	Cache    *ImageCache
	RelayImg string // Path to relay Alpine VM image (built/cached)
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

	// Prepare output directory
	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = "warden-output"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	// Shared volume: accessible by both relay VM and build VM via virtio-fs.
	// Contains relay binary, ledger output, build context, and agent binary.
	sharedDir, err := os.MkdirTemp("", "warden-shared-"+buildID+"-")
	if err != nil {
		return nil, fmt.Errorf("creating shared dir: %w", err)
	}
	defer os.RemoveAll(sharedDir)

	// Create shared volume structure:
	//   shared/
	//   ├── relay          (static relay binary for linux/arm64)
	//   ├── relay.env      (LEDGER_DIR, CONTEXT_DIR, CAPTURE_MODE)
	//   ├── context/       (build context files)
	//   ├── ledger/        (relay writes here)
	//   ├── agent/         (warden-io binary + build script for macOS)
	//   └── signal/        (completion signaling between VMs)
	for _, sub := range []string{"context", "ledger", "agent", "signal"} {
		if err := os.MkdirAll(filepath.Join(sharedDir, sub), 0755); err != nil {
			return nil, fmt.Errorf("creating shared/%s: %w", sub, err)
		}
	}

	// Place relay binary on shared volume
	if err := d.prepareRelay(sharedDir); err != nil {
		return nil, fmt.Errorf("preparing relay: %w", err)
	}

	// Write relay config (env vars it reads at startup)
	relayEnv := fmt.Sprintf("LEDGER_DIR=/shared/ledger\nCONTEXT_DIR=/shared/context\n")
	if req.CaptureMode != "" && req.CaptureMode != "none" {
		relayEnv += fmt.Sprintf("CAPTURE_MODE=%s\n", req.CaptureMode)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "relay.env"), []byte(relayEnv), 0644); err != nil {
		return nil, fmt.Errorf("writing relay.env: %w", err)
	}

	// Place build context on shared volume
	if err := d.prepareContext(sharedDir, req); err != nil {
		return nil, fmt.Errorf("preparing context: %w", err)
	}

	// Place build agent (warden-io + script) on shared volume
	if err := d.prepareAgent(sharedDir, req); err != nil {
		return nil, fmt.Errorf("preparing agent: %w", err)
	}

	// Resolve and clone macOS disk image for build VM
	diskImage, err := d.resolveImage(req.Image)
	if err != nil {
		return nil, fmt.Errorf("resolving image: %w", err)
	}

	buildDisk := filepath.Join(sharedDir, "build-disk.img")
	if err := CloneDisk(diskImage, buildDisk); err != nil {
		return nil, fmt.Errorf("cloning build disk: %w", err)
	}

	// Create isolated virtual network (socket pair)
	vnet, err := NewVirtualNetwork()
	if err != nil {
		return nil, fmt.Errorf("creating virtual network: %w", err)
	}
	defer vnet.Close()

	// Boot Relay VM: Alpine Linux with shared volume + two network interfaces
	// Interface 1: private link (socket pair) — connected to build VM
	// Interface 2: NAT — connected to host/internet for upstream requests
	relayVM, err := d.bootRelayVM(sharedDir, vnet)
	if err != nil {
		return nil, fmt.Errorf("booting relay VM: %w", err)
	}
	defer relayVM.Stop()

	// Wait for relay to write ca.cert.pem (signals it's ready)
	caPath := filepath.Join(sharedDir, "ledger", "ca.cert.pem")
	if err := waitForFile(ctx, caPath, 30); err != nil {
		return nil, fmt.Errorf("relay did not start: %w", err)
	}

	// Boot Build VM: macOS with shared volume + one network interface
	// Single interface: private link (socket pair) — relay is sole gateway
	buildVM, err := d.bootBuildVM(buildDisk, sharedDir, vnet)
	if err != nil {
		return nil, fmt.Errorf("booting build VM: %w", err)
	}
	defer buildVM.Stop()

	// Wait for build completion via heartbeat protocol
	signalDir := filepath.Join(sharedDir, "signal")
	isTTY := req.Stdin != nil
	exitCode, err := WaitForBuild(ctx, signalDir, isTTY)
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("build exited with code %d", exitCode)
	}

	// Collect outputs from shared volume to output directory
	if err := collectOutputs(sharedDir, outputDir); err != nil {
		return nil, fmt.Errorf("collecting outputs: %w", err)
	}

	return &driver.BuildResult{
		OutputDir: outputDir,
	}, nil
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	if err := checkPlatform(); err != nil {
		return err
	}
	// TODO: boot environment, expose serial console for interactive use
	return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }

// prepareRelay places the static relay binary on the shared volume.
// The relay is the same linux/arm64 binary used in container mode.
func (d *Driver) prepareRelay(sharedDir string) error {
	relayDst := filepath.Join(sharedDir, "relay")
	// TODO: locate pre-built relay binary or cross-compile from source
	// For dev: go build -o relayDst -GOOS=linux -GOARCH=arm64 ./cmd/relay
	_ = relayDst
	return nil
}

// prepareContext copies or symlinks the build context into the shared volume.
func (d *Driver) prepareContext(sharedDir string, req *driver.BuildRequest) error {
	if req.ContextDir == "" {
		return nil
	}
	ctxDst := filepath.Join(sharedDir, "context")
	// TODO: copy context files into ctxDst
	// For now, symlink works on same filesystem
	_ = ctxDst
	return nil
}

// prepareAgent places warden-io (macOS build) and the build/watcher scripts
// on the shared volume. The launchd plist in the base macOS image watches
// for the agent binary and executes it.
func (d *Driver) prepareAgent(sharedDir string, req *driver.BuildRequest) error {
	agentDir := filepath.Join(sharedDir, "agent")

	// warden-io binary (darwin/arm64)
	// TODO: locate pre-built or cross-compile warden-io for darwin/arm64
	// agentBin := filepath.Join(agentDir, "warden-io")

	// Write the build script
	buildScript := filepath.Join(agentDir, "build.sh")
	if req.Script != "" {
		data, err := os.ReadFile(req.Script)
		if err != nil {
			return fmt.Errorf("reading build script: %w", err)
		}
		if err := os.WriteFile(buildScript, data, 0755); err != nil {
			return fmt.Errorf("writing build script: %w", err)
		}
	} else {
		// Default: run nothing (useful for shell mode)
		if err := os.WriteFile(buildScript, []byte("#!/bin/sh\ntrue\n"), 0755); err != nil {
			return fmt.Errorf("writing default build script: %w", err)
		}
	}

	// Write the watcher script that wraps the build with heartbeat monitoring
	watcherContent := WatcherScript("/Volumes/My\\ Shared\\ Files/shared/agent/build.sh")
	watcherPath := filepath.Join(agentDir, "watcher.sh")
	if err := os.WriteFile(watcherPath, []byte(watcherContent), 0755); err != nil {
		return fmt.Errorf("writing watcher script: %w", err)
	}

	return nil
}

func (d *Driver) resolveImage(image string) (string, error) {
	if image == "" {
		return d.Cache.LatestIPSW()
	}
	if _, err := os.Stat(image); err == nil {
		return image, nil
	}
	return d.Cache.RestoreIPSW(image)
}

// bootRelayVM creates and starts the relay VM (Alpine Linux).
// Two interfaces: private link to build VM + NAT for internet.
func (d *Driver) bootRelayVM(sharedDir string, vnet *VirtualNetwork) (*VM, error) {
	kernelPath, initrdPath, err := d.resolveRelayVMAssets()
	if err != nil {
		return nil, fmt.Errorf("resolving relay VM assets: %w", err)
	}

	vm, err := NewLinuxVM(linuxVMConfig{
		CPUs:               2,
		MemoryMB:           512,
		KernelPath:         kernelPath,
		InitrdPath:         initrdPath,
		Cmdline:            "console=hvc0",
		SharedDirPath:      sharedDir,
		SharedDirTag:       "shared",
		FileHandleSocketFD: vnet.RelaySocketFD,
		AttachNAT:          true,
	})
	if err != nil {
		return nil, err
	}
	if err := vm.Start(); err != nil {
		return nil, err
	}
	return vm, nil
}

// resolveRelayVMAssets locates the kernel and initramfs for the relay VM.
// Looks in the cache directory, then falls back to the embedded build tooling.
func (d *Driver) resolveRelayVMAssets() (kernel, initrd string, err error) {
	cacheDir := d.Cache.CacheDir
	kernel = filepath.Join(cacheDir, "relay-vm", "vmlinuz")
	initrd = filepath.Join(cacheDir, "relay-vm", "initramfs.cpio.gz")

	if _, err := os.Stat(kernel); err == nil {
		if _, err := os.Stat(initrd); err == nil {
			return kernel, initrd, nil
		}
	}

	return "", "", fmt.Errorf(
		"relay VM assets not found; run tools/relay-vm/build-initramfs.sh " +
			"and copy output to %s/relay-vm/",
		cacheDir)
}

// bootBuildVM creates and starts the build VM (macOS).
// Single interface: private link to relay VM (sole network path).
func (d *Driver) bootBuildVM(diskImage, sharedDir string, vnet *VirtualNetwork) (*VM, error) {
	// Platform state files live alongside the disk image
	imgDir := filepath.Dir(diskImage)

	vm, err := NewMacOSVM(macOSVMConfig{
		CPUs:               4,
		MemoryMB:           8192,
		DiskImagePath:      diskImage,
		AuxStoragePath:     filepath.Join(imgDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(imgDir, "hardware-model"),
		MachineIDPath:      filepath.Join(imgDir, "machine-id"),
		SharedDirPath:      sharedDir,
		SharedDirTag:       "shared",
		FileHandleSocketFD: vnet.BuildSocketFD,
	})
	if err != nil {
		return nil, err
	}
	if err := vm.Start(); err != nil {
		return nil, err
	}
	return vm, nil
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

// waitForFile polls for a file to exist with non-zero size.
func waitForFile(ctx context.Context, path string, timeoutSec int) error {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", filepath.Base(path))
		}
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// collectOutputs moves ledger and artifacts from the shared volume to
// the final output directory.
func collectOutputs(sharedDir, outputDir string) error {
	ledgerSrc := filepath.Join(sharedDir, "ledger")
	entries, err := os.ReadDir(ledgerSrc)
	if err != nil {
		return fmt.Errorf("reading ledger dir: %w", err)
	}

	for _, e := range entries {
		src := filepath.Join(ledgerSrc, e.Name())
		dst := filepath.Join(outputDir, e.Name())
		if e.IsDir() {
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("moving %s: %w", e.Name(), err)
			}
		} else {
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("moving %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

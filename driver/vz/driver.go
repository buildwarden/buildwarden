package vz

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"warden/driver"
)

// Driver implements driver.Driver using Apple's Virtualization.framework
// for macOS/arm64 targets.
//
// Architecture (host relay + build VM):
//
//	Host (orchestrator)
//	├── Prepares shared volume: relay binary, config, context, ledger dir
//	├── Creates isolated virtual network (socket pair)
//	├── Starts Relay (host process with --ingress=fd)
//	│   ├── Reads raw Ethernet frames from socketpair via gvisor netstack
//	│   └── Has native internet access (host networking)
//	├── Boots Build VM (macOS from IPSW)
//	│   └── Single interface → socket pair → Relay (sole gateway)
//	└── Waits for build completion, collects ledger + artifacts
//
// Network isolation is topological: the build VM's only network path is
// through the relay process. No iptables, no vmnet needed.
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

	// Start host-side relay process.
	// The relay reads raw Ethernet frames from its end of the socketpair
	// using gvisor netstack (--ingress=fd). It has native internet access
	// since it runs on the host. Trust boundary is preserved: the build VM
	// can only communicate through the socketpair, and the relay controls
	// all traffic (DNS, HTTP/HTTPS proxy, ledger).
	relayProc, err := d.startHostRelay(sharedDir, vnet)
	if err != nil {
		return nil, fmt.Errorf("starting relay: %w", err)
	}
	defer relayProc.Stop()

	// Wait for relay to write ca.cert.pem (signals it's ready)
	caPath := filepath.Join(sharedDir, "ledger", "ca.cert.pem")
	if err := waitForFile(ctx, caPath, 30); err != nil {
		return nil, fmt.Errorf("relay did not start: %w", err)
	}

	// Boot Build VM: macOS with single network interface (socketpair to relay).
	// No shared directory — all communication goes through the relay HTTP API.
	// Platform state files (hardware-model, machine-id, aux-storage) live
	// alongside the original disk image, not the COW clone.
	buildVM, err := d.bootBuildVM(buildDisk, filepath.Dir(diskImage), vnet)
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

// prepareRelay places the relay binary on the shared volume.
// For the VZ driver, the relay runs on the host (darwin/arm64).
func (d *Driver) prepareRelay(sharedDir string) error {
	relayDst := filepath.Join(sharedDir, "relay")

	// Check for a pre-built relay binary next to the warden executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "warden-relay")
		if _, err := os.Stat(candidate); err == nil {
			return copyFile(candidate, relayDst)
		}
	}

	// Fall back to building from source (native, no cross-compilation needed)
	cmd := exec.Command("go", "build", "-o", relayDst, "./cmd/relay")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building relay: %w", err)
	}
	return nil
}

// prepareContext recursively copies the build context into the shared volume.
func (d *Driver) prepareContext(sharedDir string, req *driver.BuildRequest) error {
	if req.ContextDir == "" {
		return nil
	}
	ctxDst := filepath.Join(sharedDir, "context")
	cmd := exec.Command("cp", "-R", req.ContextDir+"/.", ctxDst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copying context: %s: %w", string(out), err)
	}
	return nil
}

// prepareAgent places warden-io (macOS build) and the build/watcher scripts
// on the shared volume. The launchd plist in the base macOS image watches
// for the agent binary and executes it.
func (d *Driver) prepareAgent(sharedDir string, req *driver.BuildRequest) error {
	agentDir := filepath.Join(sharedDir, "agent")

	// Cross-compile warden-io for the macOS build VM
	agentBin := filepath.Join(agentDir, "warden-io")
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "warden-io-darwin-arm64")
		if _, errStat := os.Stat(candidate); errStat == nil {
			if err := copyFile(candidate, agentBin); err != nil {
				return fmt.Errorf("copying warden-io: %w", err)
			}
			goto agentReady
		}
	}
	{
		cmd := exec.Command("go", "build", "-o", agentBin, "./cmd/warden-io")
		cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
		cmd.Dir = findModuleRoot()
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cross-compiling warden-io: %w", err)
		}
	}
agentReady:

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

// RelayProcess wraps a host-side relay subprocess.
type RelayProcess struct {
	cmd *exec.Cmd
}

// Stop sends SIGTERM and waits for the relay to exit.
func (rp *RelayProcess) Stop() {
	if rp.cmd != nil && rp.cmd.Process != nil {
		rp.cmd.Process.Signal(os.Interrupt)
		rp.cmd.Wait()
	}
}

// startHostRelay starts the relay binary as a host process with --ingress=fd.
// The relay reads raw Ethernet frames from vnet.RelaySocketFD using gvisor
// netstack, providing DHCP, DNS, HTTP/HTTPS proxy to the build VM.
func (d *Driver) startHostRelay(sharedDir string, vnet *VirtualNetwork) (*RelayProcess, error) {
	relayBin := filepath.Join(sharedDir, "relay")
	if _, err := os.Stat(relayBin); err != nil {
		return nil, fmt.Errorf("relay binary not found at %s", relayBin)
	}

	subnet := fmt.Sprintf("%s/%d",
		vnet.Subnet.RelayIP.Mask(vnet.Subnet.Netmask),
		maskBits(vnet.Subnet.Netmask))

	// The relay FD will be passed as fd 3 (first extra FD after stdin/out/err).
	cmd := exec.Command(relayBin,
		"--ingress=fd",
		"--fd=3",
		"--subnet="+subnet,
	)
	cmd.Env = append(os.Environ(),
		"LEDGER_DIR="+filepath.Join(sharedDir, "ledger"),
		"CONTEXT_DIR="+filepath.Join(sharedDir, "context"),
		"SIGNAL_DIR="+filepath.Join(sharedDir, "signal"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Pass the relay socketpair FD as ExtraFiles[0] → becomes fd 3 in child.
	cmd.ExtraFiles = []*os.File{
		os.NewFile(uintptr(vnet.RelaySocketFD), "relay-socket"),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting relay process: %w", err)
	}

	return &RelayProcess{cmd: cmd}, nil
}

func maskBits(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}

// bootBuildVM creates and starts the build VM (macOS).
// Single interface: private link to relay (sole network path).
// No shared directory — all runtime communication goes through the relay's
// HTTP API (context files, CA cert, artifacts, signals).
// platformDir contains the hardware-model, machine-id, and aux-storage files
// from the original IPSW restore (separate from the COW clone disk path).
func (d *Driver) bootBuildVM(diskImage, platformDir string, vnet *VirtualNetwork) (*VM, error) {
	imgDir := platformDir

	vm, err := NewMacOSVM(macOSVMConfig{
		CPUs:               4,
		MemoryMB:           8192,
		DiskImagePath:      diskImage,
		AuxStoragePath:     filepath.Join(imgDir, "aux-storage"),
		HardwareModelPath:  filepath.Join(imgDir, "hardware-model"),
		MachineIDPath:      filepath.Join(imgDir, "machine-id"),
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

// DefaultCacheDir returns the default path for cached VM images.
func DefaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "warden", "images")
}

func defaultCacheDir() string {
	return DefaultCacheDir()
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
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("moving %s: %w", e.Name(), err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	// Preserve executable permission
	info, err := in.Stat()
	if err == nil {
		out.Chmod(info.Mode())
	}
	return out.Close()
}

// findModuleRoot locates the go module root by walking up from the
// executable location or current directory looking for go.mod.
func findModuleRoot() string {
	// Try from executable location first
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Fall back to current directory
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

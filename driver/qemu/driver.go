package qemu

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"warden/driver"
)

// Driver implements driver.Driver using QEMU as the virtualization backend.
// Works on all host platforms (macOS, Linux, Windows) with optional hardware
// acceleration (HVF on macOS, KVM on Linux, WHPX on Windows).
//
// Architecture (same two-VM topology as vz):
//
//	Host (orchestrator)
//	├── Prepares shared volume (via virtio-9p or virtiofsd)
//	├── Starts Relay VM (Alpine, kernel+initrd, direct boot)
//	│   ├── Interface: socket → Build VM (isolated)
//	│   └── Interface: user-net → internet (upstream requests)
//	├── Starts Build VM (target OS, disk image)
//	│   └── Interface: socket → Relay VM (sole network path)
//	└── Waits for build completion via heartbeat
type Driver struct {
	// Accel is the QEMU acceleration backend ("hvf", "kvm", "whpx", "tcg").
	// Empty string means auto-detect.
	Accel string
	// QEMUBinary overrides the QEMU binary path. If empty, auto-detected
	// based on target architecture.
	QEMUBinary string
}

func New() *Driver {
	return &Driver{}
}

func (d *Driver) Name() string { return "qemu" }

func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
	buildID := driver.RandAlphaNum(8)

	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = "warden-output"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	// Shared volume structure (same as vz driver)
	sharedDir, err := os.MkdirTemp("", "warden-shared-"+buildID+"-")
	if err != nil {
		return nil, fmt.Errorf("creating shared dir: %w", err)
	}
	defer os.RemoveAll(sharedDir)

	for _, sub := range []string{"context", "ledger", "agent", "signal"} {
		if err := os.MkdirAll(filepath.Join(sharedDir, sub), 0755); err != nil {
			return nil, fmt.Errorf("creating shared/%s: %w", sub, err)
		}
	}

	// Prepare relay binary
	if err := d.prepareRelay(sharedDir); err != nil {
		return nil, fmt.Errorf("preparing relay: %w", err)
	}

	// Write relay config
	relayEnv := "LEDGER_DIR=/shared/ledger\nCONTEXT_DIR=/shared/context\n"
	if req.CaptureMode != "" && req.CaptureMode != "none" {
		relayEnv += fmt.Sprintf("CAPTURE_MODE=%s\n", req.CaptureMode)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "relay.env"), []byte(relayEnv), 0644); err != nil {
		return nil, fmt.Errorf("writing relay.env: %w", err)
	}

	// Copy build context
	if err := d.prepareContext(sharedDir, req); err != nil {
		return nil, fmt.Errorf("preparing context: %w", err)
	}

	// Prepare build agent and watcher script
	if err := d.prepareAgent(sharedDir, req); err != nil {
		return nil, fmt.Errorf("preparing agent: %w", err)
	}

	// Resolve relay VM assets (kernel + initrd)
	kernelPath, initrdPath, err := d.resolveRelayAssets()
	if err != nil {
		return nil, fmt.Errorf("resolving relay assets: %w", err)
	}

	// Create socket pair for the isolated relay↔build network
	sockDir, err := os.MkdirTemp("", "warden-net-"+buildID+"-")
	if err != nil {
		return nil, fmt.Errorf("creating socket dir: %w", err)
	}
	defer os.RemoveAll(sockDir)
	socketPath := filepath.Join(sockDir, "vlan.sock")

	// Start Relay VM
	relayProc, err := d.startRelayVM(kernelPath, initrdPath, sharedDir, socketPath)
	if err != nil {
		return nil, fmt.Errorf("starting relay VM: %w", err)
	}
	defer relayProc.stop()

	// Wait for relay to be ready
	caPath := filepath.Join(sharedDir, "ledger", "ca.cert.pem")
	if err := waitForFile(ctx, caPath, 30); err != nil {
		return nil, fmt.Errorf("relay did not start: %w", err)
	}

	// Resolve build VM disk image
	diskImage := req.Image
	if diskImage == "" {
		return nil, fmt.Errorf("build image required for QEMU driver (set image in config or --image flag)")
	}

	// Start Build VM
	buildCfg := &buildVMConfig{
		Arch:      targetArch(req),
		CPUs:      4,
		MemoryMB:  4096,
		DiskImage: diskImage,
	}
	buildProc, err := d.startBuildVM(buildCfg, sharedDir, socketPath)
	if err != nil {
		return nil, fmt.Errorf("starting build VM: %w", err)
	}
	defer buildProc.stop()

	// Wait for build via heartbeat
	signalDir := filepath.Join(sharedDir, "signal")
	isTTY := req.Stdin != nil
	exitCode, err := waitForBuild(ctx, signalDir, isTTY)
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("build exited with code %d", exitCode)
	}

	// Collect outputs
	if err := collectOutputs(sharedDir, outputDir); err != nil {
		return nil, fmt.Errorf("collecting outputs: %w", err)
	}

	return &driver.BuildResult{OutputDir: outputDir}, nil
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }

// detectAccel returns the best available acceleration for this host.
func (d *Driver) detectAccel() string {
	if d.Accel != "" {
		return d.Accel
	}
	switch runtime.GOOS {
	case "darwin":
		return "hvf"
	case "linux":
		if _, err := os.Stat("/dev/kvm"); err == nil {
			return "kvm"
		}
		return "tcg"
	case "windows":
		return "whpx"
	default:
		return "tcg"
	}
}

// qemuBinary returns the QEMU system binary for the given architecture.
func (d *Driver) qemuBinary(arch string) string {
	if d.QEMUBinary != "" {
		return d.QEMUBinary
	}
	return fmt.Sprintf("qemu-system-%s", arch)
}

// targetArch determines the QEMU architecture string from the build request.
func targetArch(req *driver.BuildRequest) string {
	// TODO: derive from req.Image metadata or explicit config
	// Default to host architecture for now
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	default:
		return runtime.GOARCH
	}
}

func (d *Driver) prepareRelay(sharedDir string) error {
	relayDst := filepath.Join(sharedDir, "relay")
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "warden-relay-linux-arm64")
		if _, err := os.Stat(candidate); err == nil {
			return copyFile(candidate, relayDst)
		}
	}
	cmd := exec.Command("go", "build", "-o", relayDst, "./cmd/relay")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

func (d *Driver) prepareAgent(sharedDir string, req *driver.BuildRequest) error {
	agentDir := filepath.Join(sharedDir, "agent")

	buildScript := filepath.Join(agentDir, "build.sh")
	if req.Script != "" {
		data, err := os.ReadFile(req.Script)
		if err != nil {
			return fmt.Errorf("reading build script: %w", err)
		}
		if err := os.WriteFile(buildScript, data, 0755); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(buildScript, []byte("#!/bin/sh\ntrue\n"), 0755); err != nil {
			return err
		}
	}

	watcherContent := watcherScript("/shared/agent/build.sh")
	watcherPath := filepath.Join(agentDir, "watcher.sh")
	return os.WriteFile(watcherPath, []byte(watcherContent), 0755)
}

func (d *Driver) resolveRelayAssets() (kernel, initrd string, err error) {
	// Check alongside the warden binary
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		k := filepath.Join(dir, "relay-vm-vmlinuz")
		i := filepath.Join(dir, "relay-vm-initramfs.cpio.gz")
		if _, err := os.Stat(k); err == nil {
			if _, err := os.Stat(i); err == nil {
				return k, i, nil
			}
		}
	}

	// Check the build output directory
	root := findModuleRoot()
	k := filepath.Join(root, "tools", "relay-vm", "output", "vmlinuz")
	i := filepath.Join(root, "tools", "relay-vm", "output", "initramfs.cpio.gz")
	if _, err := os.Stat(k); err == nil {
		if _, err := os.Stat(i); err == nil {
			return k, i, nil
		}
	}

	return "", "", fmt.Errorf(
		"relay VM assets not found; run tools/relay-vm/build-initramfs.sh")
}

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

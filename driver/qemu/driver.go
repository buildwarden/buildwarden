package qemu

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"warden/driver"
	"warden/driver/script"
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
	// Verbose enables kernel/VM console output on stderr.
	Verbose bool
}

func New() *Driver {
	return &Driver{}
}

func (d *Driver) Name() string { return "qemu" }

func (d *Driver) status(msg string) {
	fmt.Fprintf(os.Stderr, "[warden] %s\n", msg)
}

func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Cancel context on interrupt for graceful cleanup
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	for _, sub := range []string{"context", "ledger", "signal"} {
		if err := os.MkdirAll(filepath.Join(sharedDir, sub), 0755); err != nil {
			return nil, fmt.Errorf("creating shared/%s: %w", sub, err)
		}
	}

	d.status("preparing build environment...")

	// Compile relay and warden-io in parallel
	type prepResult struct {
		name string
		err  error
	}
	prepCh := make(chan prepResult, 2)
	go func() {
		prepCh <- prepResult{"relay", d.prepareRelay(sharedDir)}
	}()
	go func() {
		prepCh <- prepResult{"agent", d.prepareAgent(sharedDir, req)}
	}()
	for i := 0; i < 2; i++ {
		r := <-prepCh
		if r.err != nil {
			return nil, fmt.Errorf("preparing %s: %w", r.name, r.err)
		}
	}

	// Write relay config
	relayEnv := "LEDGER_DIR=/shared/ledger\nCONTEXT_DIR=/shared/context\n" +
		"SIGNAL_DIR=/shared/signal\n"
	if req.CaptureMode != "" && req.CaptureMode != "none" {
		relayEnv += fmt.Sprintf("CAPTURE_MODE=%s\n", req.CaptureMode)
	}
	envPath := filepath.Join(sharedDir, "relay.env")
	if err := os.WriteFile(envPath, []byte(relayEnv), 0644); err != nil {
		return nil, fmt.Errorf("writing relay.env: %w", err)
	}

	// Copy build context
	if err := d.prepareContext(sharedDir, req); err != nil {
		return nil, fmt.Errorf("preparing context: %w", err)
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

	d.status("starting relay...")
	relayProc, err := d.startRelayVM(kernelPath, initrdPath, sharedDir, socketPath)
	if err != nil {
		return nil, fmt.Errorf("starting relay VM: %w", err)
	}
	defer relayProc.stop()

	caPath := filepath.Join(sharedDir, "ledger", "ca.cert.pem")
	if err := waitForFile(ctx, caPath, 30); err != nil {
		return nil, fmt.Errorf("relay did not start: %w", err)
	}

	// Resolve build VM boot method
	buildCfg := &buildVMConfig{
		Arch:     targetArch(req),
		CPUs:     4,
		MemoryMB: 4096,
	}

	// Resolve image: explicit --image, FROM in Containerfile, or direct-boot
	image := req.Image
	if image == "" && req.Containerfile != "" {
		result, err := script.Translate(req.Containerfile)
		if err == nil && result.Image != "" && result.Image != "scratch" {
			resolved, err := resolveImage(result.Image)
			if err == nil && resolved != "" {
				image = resolved
			}
		}
	}

	if image != "" {
		if _, err := os.Stat(image); err != nil {
			return nil, fmt.Errorf("build image: %w", err)
		}
		// Create COW overlay so base image is never modified
		overlay := filepath.Join(sharedDir, "build-overlay.qcow2")
		cmd := exec.Command("qemu-img", "create",
			"-f", "qcow2", "-b", image, "-F", "qcow2", overlay)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf(
				"creating overlay: %s: %w", string(out), err)
		}
		buildCfg.DiskImage = overlay

		// Generate cloud-init seed ISO
		seedDir, err := d.cloudInitSeed(sharedDir)
		if err != nil {
			return nil, fmt.Errorf("generating cloud-init: %w", err)
		}
		seedISO := filepath.Join(sharedDir, "seed.iso")
		if err := generateSeedISO(seedDir, seedISO); err != nil {
			return nil, fmt.Errorf("generating seed ISO: %w", err)
		}
		buildCfg.SeedISO = seedISO
	} else {
		k, i, err := d.resolveBuildAssets()
		if err != nil {
			return nil, fmt.Errorf("resolving build assets: %w", err)
		}
		buildCfg.Kernel = k
		buildCfg.Initrd = i
	}
	d.status("starting build...")
	buildProc, err := d.startBuildVM(buildCfg, socketPath)
	if err != nil {
		return nil, fmt.Errorf("starting build VM: %w", err)
	}
	defer buildProc.stop()

	d.status("running...")
	// Wait for build via heartbeat (relay writes signal/exit_code when
	// warden-io reports completion via HTTP)
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

func hostQEMUArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	default:
		return runtime.GOARCH
	}
}

// targetArch determines the QEMU architecture string from the build request.
func targetArch(req *driver.BuildRequest) string {
	return hostQEMUArch()
}

func (d *Driver) prepareRelay(sharedDir string) error {
	relayDst := filepath.Join(sharedDir, "relay")
	goarch := runtime.GOARCH

	// Check next to the warden binary
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(
			filepath.Dir(exe), "warden-relay-linux-"+goarch)
		if _, err := os.Stat(candidate); err == nil {
			return copyFile(candidate, relayDst)
		}
	}

	// Check build cache
	cached := cachedBinaryPath("relay", goarch)
	if !isCacheStale(cached) {
		return copyFile(cached, relayDst)
	}

	// Build and cache
	cmd := exec.Command("go", "build",
		"-ldflags=-s -w", "-o", cached, "./cmd/relay")
	cmd.Env = append(os.Environ(),
		"GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return copyFile(cached, relayDst)
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
	// Place build script into context/ — warden-io initialize fetches it
	// from the relay's HTTP context endpoint at runtime.
	ctxDir := filepath.Join(sharedDir, "context")
	buildScript := filepath.Join(ctxDir, "build.sh")
	if req.Script != "" {
		data, err := os.ReadFile(req.Script)
		if err != nil {
			return fmt.Errorf("reading build script: %w", err)
		}
		if err := os.WriteFile(buildScript, data, 0755); err != nil {
			return err
		}
	} else if req.Containerfile != "" {
		result, err := script.Translate(req.Containerfile)
		if err != nil {
			return fmt.Errorf("translating containerfile: %w", err)
		}
		if err := os.WriteFile(buildScript, []byte(result.Script), 0755); err != nil {
			return err
		}
	} else {
		noop := []byte("#!/bin/sh\ntrue\n")
		if err := os.WriteFile(buildScript, noop, 0755); err != nil {
			return err
		}
	}

	// Cross-compile warden-io for the build VM (placed in agent/ for the
	// seed ISO to pick up)
	agentDir := filepath.Join(sharedDir, "agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("creating agent dir: %w", err)
	}
	if err := d.prepareWardenIO(agentDir); err != nil {
		return fmt.Errorf("preparing warden-io: %w", err)
	}

	return nil
}

func (d *Driver) prepareWardenIO(agentDir string) error {
	goarch := runtime.GOARCH
	dst := filepath.Join(agentDir, "warden-io")

	// Check next to the warden binary
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(
			filepath.Dir(exe), "warden-io-linux-"+goarch)
		if _, err := os.Stat(candidate); err == nil {
			return copyFile(candidate, dst)
		}
	}

	// Check build cache
	cached := cachedBinaryPath("warden-io", goarch)
	if !isCacheStale(cached) {
		return copyFile(cached, dst)
	}

	// Build and cache
	cmd := exec.Command("go", "build",
		"-ldflags=-s -w", "-o", cached, "./cmd/warden-io")
	cmd.Env = append(os.Environ(),
		"GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Dir = findModuleRoot()
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return copyFile(cached, dst)
}

func (d *Driver) resolveBuildAssets() (kernel, initrd string, err error) {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		k := filepath.Join(dir, "build-vm-vmlinuz")
		i := filepath.Join(dir, "build-vm-initramfs.cpio.gz")
		if _, err := os.Stat(k); err == nil {
			if _, err := os.Stat(i); err == nil {
				return k, i, nil
			}
		}
	}

	root := findModuleRoot()
	k := filepath.Join(root, "tools", "relay-vm", "output", "vmlinuz")
	i := filepath.Join(root, "tools", "build-vm", "output", "initramfs.cpio.gz")
	if _, err := os.Stat(k); err == nil {
		if _, err := os.Stat(i); err == nil {
			return k, i, nil
		}
	}

	return "", "", fmt.Errorf(
		"build VM assets not found; run tools/build-vm/build-initramfs.sh")
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
		_ = os.RemoveAll(dst)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("moving %s: %w", e.Name(), err)
		}
	}
	return nil
}

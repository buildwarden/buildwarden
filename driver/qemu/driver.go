package qemu

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/script"
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

func (d *Driver) StartBuild(
	ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
	if err := d.checkPrereqs(); err != nil {
		return nil, err
	}
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

	workDir, err := os.MkdirTemp("", "warden-qemu-"+buildID+"-")
	if err != nil {
		return nil, fmt.Errorf("creating work dir: %w", err)
	}
	defer driver.RemoveWorkDir(req, workDir)

	d.status("preparing build environment...")
	buildScript, err := driver.StageBuildScript(req, workDir)
	if err != nil {
		return nil, fmt.Errorf("staging build script: %w", err)
	}

	// The relay's claim port is forwarded to host loopback (hostfwd); the
	// collector rides the tunnel the session opens over it.
	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   outputDir,
		ContextDir:  req.ContextDir,
		BuildScript: buildScript,
		OutputTee:   driver.ConsoleTee(req),
		Verbose:     d.Verbose,
	})
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	kernelPath, initrdPath, err := d.relayInitrd(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving relay assets: %w", err)
	}

	// Socket for the isolated relay<->build network.
	socketPath := filepath.Join(workDir, "vlan.sock")

	claimPort, err := driver.FreeLoopbackPort()
	if err != nil {
		return nil, err
	}
	claimToken, err := driver.RandomToken()
	if err != nil {
		return nil, err
	}

	d.status("starting relay...")
	relayProc, err := d.startRelayVM(kernelPath, initrdPath, socketPath, claimPort, claimToken)
	if err != nil {
		return nil, fmt.Errorf("starting relay VM: %w", err)
	}
	defer relayProc.stop()

	claimAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(claimPort))
	if _, err := sess.ClaimRelay(ctx, claimAddr, claimToken, req, nil); err != nil {
		return nil, err
	}

	buildCfg, err := d.resolveBuildVMConfig(req, workDir)
	if err != nil {
		return nil, err
	}
	d.status("starting build...")
	buildProc, err := d.startBuildVM(buildCfg, socketPath)
	if err != nil {
		return nil, fmt.Errorf("starting build VM: %w", err)
	}
	defer buildProc.stop()

	d.status("running...")
	if err := sess.Wait(ctx); err != nil {
		return nil, err
	}
	driver.FinalizeOutput(outputDir, req.Compress)
	return &driver.BuildResult{OutputDir: outputDir}, nil
}

// resolveBuildVMConfig determines how to boot the build VM: either from
// a disk image (with COW overlay + cloud-init) or via direct kernel boot.
func (d *Driver) resolveBuildVMConfig(
	req *driver.BuildRequest, workDir string,
) (*buildVMConfig, error) {
	cfg := &buildVMConfig{
		Arch:     targetArch(req),
		CPUs:     driver.OrDefault(req.CPUs, 4),
		MemoryMB: driver.OrDefault(req.MemoryMB, 4096),
	}

	image := req.Image
	if image == "" && req.Containerfile != "" {
		result, err := script.Translate(req.Containerfile)
		if err == nil && result.Image != "" &&
			result.Image != "scratch" {
			resolved, err := resolveImage(result.Image)
			if err == nil && resolved != "" {
				image = resolved
			}
		}
	}

	if image == "" {
		if isWindowsGuest(req) {
			// Use the base image prepared by `warden image prepare --driver
			// qemu --os windows` (parity with the Linux kernel/initrd resolution). It is
			// overlaid below, so the cached base is never mutated.
			ga := goArchOf(cfg.Arch)
			base := WindowsImagePath(ga)
			if _, err := os.Stat(base); err != nil {
				return nil, fmt.Errorf(
					"no prepared Windows base image at %s (run "+
						"`warden image prepare --driver qemu --os windows --arch %s` first, "+
						"or pass --image <path>)", base, ga)
			}
			image = base
		} else {
			k, i, err := d.resolveBuildAssets()
			if err != nil {
				return nil, fmt.Errorf("resolving build assets: %w", err)
			}
			cfg.Kernel = k
			cfg.Initrd = i
			return cfg, nil
		}
	}

	if _, err := os.Stat(image); err != nil {
		return nil, fmt.Errorf("build image: %w", err)
	}
	image, _ = filepath.Abs(image)

	overlay := filepath.Join(workDir, "build-overlay.qcow2")
	cmd := exec.Command("qemu-img", "create",
		"-f", "qcow2", "-b", image, "-F", "qcow2", overlay)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf(
			"creating overlay: %s: %w", string(out), err)
	}
	cfg.DiskImage = overlay

	// Windows guests use a WARDEN provisioning seed and a static network;
	// Linux guests use a cloud-init NoCloud (CIDATA) seed.
	if isWindowsGuest(req) {
		cfg.Windows = true
		// Writable UEFI vars carrying the Windows Boot Manager entry that
		// bcdboot wrote during image prep; copied per-build so the base's
		// vars store is never mutated.
		ga := goArchOf(cfg.Arch)
		srcVars := filepath.Join(driver.CacheDir(), "images",
			"windows-"+ga+"-vars.fd")
		if _, err := os.Stat(srcVars); err != nil {
			return nil, fmt.Errorf(
				"windows base UEFI vars not found at %s (re-run "+
					"`warden image prepare --driver qemu --os windows --arch %s`)", srcVars, ga)
		}
		dstVars := filepath.Join(workDir, "build-vars.fd")
		if err := driver.CopyFile(srcVars, dstVars); err != nil {
			return nil, fmt.Errorf("copying UEFI vars: %w", err)
		}
		cfg.VarsFD = dstVars
		seedDir, err := windowsSeedDir(workDir, goArchOf(cfg.Arch))
		if err != nil {
			return nil, fmt.Errorf("generating windows seed: %w", err)
		}
		// FAT image (not ISO): Windows reads FAT16 natively via usb-storage
		// with exact long names and a drive letter; our ISO9660 seed shows up
		// as FileSystemType=Unknown with no letter, so the startup task can't
		// run warden-run.ps1.
		seedImg := filepath.Join(workDir, "seed.img")
		if err := generateWindowsSeedFAT(
			seedDir, seedImg, windowsSeedName); err != nil {
			return nil, fmt.Errorf("generating windows seed FAT: %w", err)
		}
		cfg.SeedISO = seedImg
		return cfg, nil
	}

	seedDir, err := cloudInitSeed(workDir, goArchOf(cfg.Arch))
	if err != nil {
		return nil, fmt.Errorf("generating cloud-init: %w", err)
	}
	seedISO := filepath.Join(workDir, "seed.iso")
	if err := generateSeedISO(seedDir, seedISO, "CIDATA"); err != nil {
		return nil, fmt.Errorf("generating seed ISO: %w", err)
	}
	cfg.SeedISO = seedISO
	return cfg, nil
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }

func (d *Driver) checkPrereqs() error {
	binary := d.qemuBinary(hostQEMUArch())
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf(
			"qemu driver requires %s but it was not found in PATH\n"+
				"  macOS:  brew install qemu\n"+
				"  Linux:  apt install qemu-system (or equivalent)\n"+
				"  Windows: scoop install qemu", binary)
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return fmt.Errorf(
			"qemu driver requires qemu-img but it was not found in PATH\n" +
				"  It is typically included with the qemu package")
	}
	return nil
}

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
		fmt.Fprintf(os.Stderr,
			"[warden] WARNING: /dev/kvm not available; "+
				"using software emulation (much slower)\n")
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

	root, _ := driver.ModuleRoot()
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
	root, _ := driver.ModuleRoot()
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

// Preflight checks the host can run qemu builds: the qemu binaries are on
// PATH and the relay VM assets exist. It is `warden doctor --driver qemu`.
func Preflight() error {
	d := New()
	if err := d.checkPrereqs(); err != nil {
		return err
	}
	_, _, err := d.resolveRelayAssets()
	return err
}

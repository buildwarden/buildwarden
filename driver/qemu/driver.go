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
	"github.com/buildwarden/buildwarden/relaycfg"
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

	workDir, err := driver.NewWorkDir(req, "qemu-"+buildID)
	if err != nil {
		return nil, err
	}
	defer driver.RemoveWorkDir(req, workDir)

	d.status("preparing build environment...")
	buildScript, err := driver.StageBuildScript(req, workDir)
	if err != nil {
		return nil, fmt.Errorf("staging build script: %w", err)
	}

	// Resolve both VMs' boot assets before anything boots, so a missing
	// image or asset fails in milliseconds rather than after the relay is up.
	buildCfg, err := d.resolveBuildVMConfig(req, workDir)
	if err != nil {
		return nil, err
	}
	kernelPath, initrdPath, err := d.relayInitrd(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving relay assets: %w", err)
	}
	env, err := buildEnvironment(buildCfg, buildScript)
	if err != nil {
		return nil, fmt.Errorf("describing build environment: %w", err)
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

	// Socket for the isolated relay<->build network.
	socketPath := filepath.Join(workDir, "vlan.sock")

	relayProc, err := d.startAndClaimRelay(
		ctx, sess, req, env, kernelPath, initrdPath, socketPath)
	if err != nil {
		return nil, err
	}
	defer relayProc.stop()

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

// startAndClaimRelay boots the relay VM and claims it over the loopback-
// forwarded claim port, returning the running relay process. On a claim
// failure it stops the relay before returning, so the caller only needs to
// defer stop on success.
func (d *Driver) startAndClaimRelay(
	ctx context.Context, sess *driver.Session, req *driver.BuildRequest,
	env *relaycfg.Environment, kernelPath, initrdPath, socketPath string,
) (*vmProcess, error) {
	claimPort, err := driver.FreeLoopbackPort()
	if err != nil {
		return nil, err
	}
	claimToken, err := driver.RandomToken()
	if err != nil {
		return nil, err
	}

	d.status("starting relay...")
	relayProc, err := d.startRelayVM(
		kernelPath, initrdPath, socketPath, claimPort, claimToken)
	if err != nil {
		return nil, fmt.Errorf("starting relay VM: %w", err)
	}

	claimAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(claimPort))
	if _, err := sess.ClaimRelay(ctx, claimAddr, claimToken, req, env); err != nil {
		relayProc.stop()
		return nil, err
	}
	return relayProc, nil
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

	image := resolveContainerfileImage(req)

	if image == "" {
		if isWindowsGuest(req) {
			base, err := resolveWindowsBaseImage(cfg.Arch)
			if err != nil {
				return nil, err
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
		return nil, driver.Fixable("", "build image: %v", err)
	}
	image, _ = filepath.Abs(image)
	cfg.BaseImage = image

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
		if err := configureWindowsBuildVM(cfg, workDir); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	seedDir, err := cloudInitSeed(workDir, goArchOf(cfg.Arch))
	if err != nil {
		return nil, fmt.Errorf("generating cloud-init: %w", err)
	}
	cfg.SeedDir = seedDir
	seedISO := filepath.Join(workDir, "seed.iso")
	if err := generateSeedISO(seedDir, seedISO, "CIDATA"); err != nil {
		return nil, fmt.Errorf("generating seed ISO: %w", err)
	}
	cfg.SeedISO = seedISO
	return cfg, nil
}

// resolveContainerfileImage returns req.Image, or when it is empty, the base
// image a Containerfile FROM resolves to (empty when there is none or it is
// "scratch").
func resolveContainerfileImage(req *driver.BuildRequest) string {
	if req.Image != "" || req.Containerfile == "" {
		return req.Image
	}
	result, err := script.Translate(req.Containerfile)
	if err != nil || result.Image == "" || result.Image == "scratch" {
		return ""
	}
	resolved, err := resolveImage(result.Image)
	if err != nil || resolved == "" {
		return ""
	}
	return resolved
}

// resolveWindowsBaseImage returns the prepared Windows base image for the arch,
// or an actionable error when `warden image prepare` has not produced one.
func resolveWindowsBaseImage(arch string) (string, error) {
	// Use the base image prepared by `warden image prepare --driver qemu --os
	// windows` (parity with the Linux kernel/initrd resolution). It is overlaid
	// below, so the cached base is never mutated.
	ga := goArchOf(arch)
	base := WindowsImagePath(ga)
	if _, err := os.Stat(base); err != nil {
		return "", driver.Fixable(
			fmt.Sprintf("warden image prepare --driver qemu --os windows "+
				"--arch %s --source <install ISO>, or pass --image <path>", ga),
			"no prepared Windows base image at %s", base)
	}
	return base, nil
}

// configureWindowsBuildVM copies the writable UEFI vars and builds the Windows
// provisioning seed (FAT image) for the build VM, filling cfg.VarsFD and
// cfg.SeedISO.
func configureWindowsBuildVM(cfg *buildVMConfig, workDir string) error {
	// Writable UEFI vars carrying the Windows Boot Manager entry that
	// bcdboot wrote during image prep; copied per-build so the base's
	// vars store is never mutated.
	ga := goArchOf(cfg.Arch)
	srcVars := filepath.Join(driver.CacheDir(), "images",
		"windows-"+ga+"-vars.fd")
	if _, err := os.Stat(srcVars); err != nil {
		return driver.Fixable(
			fmt.Sprintf("warden image prepare --driver qemu --os windows "+
				"--arch %s --force --source <install ISO>", ga),
			"windows base UEFI vars not found at %s", srcVars)
	}
	dstVars := filepath.Join(workDir, "build-vars.fd")
	if err := driver.CopyFile(srcVars, dstVars); err != nil {
		return fmt.Errorf("copying UEFI vars: %w", err)
	}
	cfg.VarsFD = dstVars
	cfg.BaseVarsFD = srcVars
	seedDir, err := windowsSeedDir(workDir, goArchOf(cfg.Arch))
	if err != nil {
		return fmt.Errorf("generating windows seed: %w", err)
	}
	cfg.SeedDir = seedDir
	// FAT image (not ISO): Windows reads FAT16 natively via usb-storage
	// with exact long names and a drive letter; our ISO9660 seed shows up
	// as FileSystemType=Unknown with no letter, so the startup task can't
	// run warden-run.ps1.
	seedImg := filepath.Join(workDir, "seed.img")
	if err := generateWindowsSeedFAT(
		seedDir, seedImg, windowsSeedName); err != nil {
		return fmt.Errorf("generating windows seed FAT: %w", err)
	}
	cfg.SeedISO = seedImg
	return nil
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }

func (d *Driver) checkPrereqs() error {
	binary := d.qemuBinary(hostQEMUArch())
	if _, err := exec.LookPath(binary); err != nil {
		return driver.Fixable(qemuInstallHint(),
			"qemu driver requires %s but it was not found in PATH", binary)
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return driver.Fixable(qemuInstallHint()+" (qemu-img ships with qemu)",
			"qemu driver requires qemu-img but it was not found in PATH")
	}
	return nil
}

// qemuInstallHint is how to install qemu on this OS.
func qemuInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "install it: brew install qemu"
	case "windows":
		return "install it: winget install " +
			"SoftwareFreedomConservancy.QEMU (or scoop install qemu)"
	default:
		return "install it: apt install qemu-system qemu-utils " +
			"(or your distro's equivalent)"
	}
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

	return "", "", driver.Fixable(
		"build them from the source tree: "+
			"tools/relay-vm/build-initramfs.sh && "+
			"tools/build-vm/build-initramfs.sh, or pass --image <qcow2> "+
			"(warden image prepare --driver qemu)",
		"qemu build VM boot assets (kernel + initramfs) not found")
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

	return "", "", driver.Fixable(
		"build them from the source tree: tools/relay-vm/build-initramfs.sh",
		"qemu relay VM assets (kernel + initramfs) not found")
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

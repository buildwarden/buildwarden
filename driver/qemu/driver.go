package qemu

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
	"github.com/buildwarden/buildwarden/driver/relayvm"
	"github.com/buildwarden/buildwarden/driver/winguest"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// UserNetCIDR is qemu's built-in user-mode (slirp) network, the relay VM's
// upstream: qemu's default, which the driver does not override.
const UserNetCIDR = "10.0.2.0/24"

// Driver is the qemu driver, with QEMU as the virtualization backend.
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

	// relayStep and buildStep replace runVMs's two boot steps in tests.
	relayStep, buildStep func() (stopper, error)
}

// New returns a Driver that auto-detects its QEMU binary and acceleration
// backend.
func New() *Driver {
	return &Driver{}
}

func (d *Driver) status(msg string) {
	fmt.Fprintf(os.Stderr, "[warden] %s\n", msg)
}

// Preflight is the qemu driver's probe, and the checks its Run makes before
// it creates anything: the qemu binaries, a build link no host interface
// holds, and the relay's resolvers, which replace req.DNS.
func (d *Driver) Preflight(req *driver.BuildRequest) error {
	if err := d.checkPrereqs(); err != nil {
		return err
	}
	if err := driver.CheckHostLink(req.Link()); err != nil {
		return err
	}
	return setRelayDNS(req, runtime.GOOS, os.ReadFile)
}

// Run is the qemu driver's run of one build (specs/driver-contract/v1.md,
// The build): it boots the relay VM with the claim token and reports it
// through host, boots the build VM, whose only network path is the relay,
// once warden has claimed it, and stops both when host.Wait returns. Under
// --keep-vms it leaves the work directory, with the disks, and returns it as
// kept.
func (d *Driver) Run(ctx context.Context, req *driver.BuildRequest, start *driver.Start,
	host driver.Host) (kept []string, err error) {
	if err := d.Preflight(req); err != nil {
		return nil, err
	}
	workDir, err := driver.NewWorkDir(req, "qemu-"+start.BuildID)
	if err != nil {
		return nil, err
	}
	defer driver.RemoveWorkDir(req, workDir)
	if req.KeepVMs {
		kept = []string{workDir}
	}

	d.status("preparing build environment...")
	// Resolve both VMs' boot assets before anything boots, so a missing
	// image or asset fails in milliseconds rather than after the relay is up.
	buildCfg, err := d.resolveBuildVMConfig(req, workDir)
	if err != nil {
		return kept, err
	}
	kernelPath, initrdPath, err := d.relayInitrd(workDir)
	if err != nil {
		return kept, fmt.Errorf("resolving relay assets: %w", err)
	}
	env, err := buildEnvironment(buildCfg, req.BuildScript)
	if err != nil {
		return kept, fmt.Errorf("describing build environment: %w", err)
	}

	// Socket for the isolated relay<->build network.
	socketPath := filepath.Join(workDir, "vlan.sock")
	relay := relayBoot{env: env, kernel: kernelPath, initrd: initrdPath,
		socket: socketPath, token: start.ClaimToken, guestOS: start.GuestOS}
	return kept, d.runVMs(ctx, host, req, relay, buildCfg)
}

// stopper is a started VM.
type stopper interface{ stop() }

// relayBoot is what the relay VM boots with and reports.
type relayBoot struct {
	env                    *relaycfg.Environment
	kernel, initrd, socket string
	token, guestOS         string
}

// runVMs boots the relay VM and reports it, and only once warden has claimed
// it (the relay has addressed the build link and is serving its DHCP lease)
// boots the build VM, then waits for warden's stop. The two steps go through
// d.relayStep and d.buildStep when set, so a test can pin the order.
func (d *Driver) runVMs(ctx context.Context, host driver.Host, req *driver.BuildRequest,
	relay relayBoot, buildCfg *buildVMConfig) error {
	relayStep := func() (stopper, error) {
		return d.startAndClaimRelay(ctx, host, req, relay)
	}
	buildStep := func() (stopper, error) { return d.startBuildVM(buildCfg, relay.socket) }
	if d.relayStep != nil {
		relayStep = d.relayStep
	}
	if d.buildStep != nil {
		buildStep = d.buildStep
	}

	relayProc, err := relayStep()
	if err != nil {
		return err
	}
	defer relayProc.stop()

	d.status("starting build...")
	buildProc, err := buildStep()
	if err != nil {
		return fmt.Errorf("starting build VM: %w", err)
	}
	defer buildProc.stop()

	if err := host.Started(); err != nil {
		return err
	}
	d.status("running...")
	host.Wait(ctx)
	return nil
}

// startAndClaimRelay boots the relay VM and reports it at its loopback-
// forwarded claim port, returning the running relay process once warden has
// claimed it. On a claim failure it stops the relay before returning, so the
// caller only needs to defer stop on success.
func (d *Driver) startAndClaimRelay(ctx context.Context, host driver.Host,
	req *driver.BuildRequest, relay relayBoot) (*vmProcess, error) {
	claimPort, err := driver.FreeLoopbackPort()
	if err != nil {
		return nil, err
	}

	d.status("starting relay...")
	relayProc, err := d.startRelayVM(
		relay.kernel, relay.initrd, relay.socket, claimPort, relay.token)
	if err != nil {
		return nil, fmt.Errorf("starting relay VM: %w", err)
	}

	claimAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(claimPort))
	if err := host.ClaimRelay(ctx, driver.RelayStarted{
		Claim: driver.ClaimEndpoint{Network: driver.ClaimTCP, Address: claimAddr},
		ClaimValues: driver.ClaimValues{Environment: relay.env,
			BuildLink:   req.Link().Claim(),
			UpstreamDNS: driver.UpstreamDNS(req.DNS)},
		GuestOS:     relay.guestOS,
		OldRelayFix: driver.OldRelayFix,
	}); err != nil {
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

	image := linuxguest.RequestImage(req)

	if image == "" {
		if isWindowsGuest(req) {
			chain, vars, err := resolveWindowsBaseImage(cfg.Arch, req)
			if err != nil {
				return nil, err
			}
			image = chain[len(chain)-1]
			cfg.BaseChain, cfg.BaseVarsFD = chain, vars
		} else {
			k, i, err := d.resolveBuildAssets()
			if err != nil {
				return nil, fmt.Errorf("resolving build assets: %w", err)
			}
			cfg.Kernel = k
			cfg.Initrd = i
			cfg.BootArgs = req.Link().BootArgs()
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
		if err := configureWindowsBuildVM(cfg, workDir, req.Link()); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	seedDir, err := linuxguest.Seed(workDir, goArchOf(cfg.Arch), linuxguest.PowerOff,
		req.Link())
	if err != nil {
		return nil, fmt.Errorf("generating cloud-init: %w", err)
	}
	cfg.SeedDir = seedDir
	seedISO := filepath.Join(workDir, "seed.iso")
	if err := linuxguest.GenerateSeedISO(seedDir, seedISO, "CIDATA"); err != nil {
		return nil, fmt.Errorf("generating seed ISO: %w", err)
	}
	cfg.SeedISO = seedISO
	return cfg, nil
}

// resolveWindowsBaseImage returns the disk chain a Windows build boots from,
// base first, and the UEFI vars to copy for it: the active prepared layer over
// the generalized base, or, for a base installed before the generalizing
// install, that base on its own. A generalized base with no prepared layer is
// a fixable error. Using a prepared layer runs the freshness rules, which may
// start a background refresh.
func resolveWindowsBaseImage(
	arch string, req *driver.BuildRequest,
) (chain []string, vars string, err error) {
	ga := goArchOf(arch)
	base := WindowsImagePath(ga)
	if _, err := os.Stat(base); err != nil {
		return nil, "", driver.Fixable(
			fmt.Sprintf("warden image prepare --driver qemu --os windows "+
				"--arch %s --source <install ISO>, or pass --image <path>", ga),
			"no prepared Windows base image at %s", base)
	}
	if _, err := os.Stat(generalizedMarker(ga)); err != nil {
		return []string{base}, windowsVarsPath(ga), nil // installed before the split
	}
	l, ok := activeWindowsLayer(ga)
	if !ok {
		return nil, "", driver.Fixable(prepareFix(ga),
			"the Windows base for %s has not been prepared for builds yet", ga)
	}
	for _, p := range []string{l.Path, l.VarsFD} {
		if _, err := os.Stat(p); err != nil {
			return nil, "", driver.Fixable(prepareFix(ga),
				"prepared Windows layer is incomplete (%s: %v)", p, err)
		}
	}
	if err := ensureFreshWindowsLayer(l, ga, req, os.Stderr); err != nil {
		return nil, "", err
	}
	return []string{base, l.Path}, l.VarsFD, nil
}

// ensureFreshWindowsLayer applies winguest.CheckFreshness and starts a
// detached re-prepare when one is due, or reports on progress that it did not
// start; the build continues on l either way.
func ensureFreshWindowsLayer(
	l PreparedLayer, ga string, req *driver.BuildRequest, progress io.Writer,
) error {
	now := time.Now()
	f := winguest.Freshness{PreparedAt: l.PreparedAt, GraceEnd: l.GraceEnd}
	refresh, err := winguest.CheckFreshness(
		f, now, req.ImageRefreshAfter, req.ImageMinGrace, prepareFix(ga))
	if err != nil || !refresh {
		return err
	}
	age := winguest.RoundAge(f.Age(now))
	unlock, free, err := tryHostLock(prepareLockName(ga))
	if err != nil {
		reportRefreshNotStarted(progress, age, err, ga)
		return nil
	}
	if !free {
		return nil
	}
	unlock()
	logPath := filepath.Join(driver.CacheDir(), "images", "prepare-windows-"+ga+".log")
	if err := startDetached(logPath, "image", "prepare", "--driver", "qemu",
		"--os", "windows", "--arch", ga); err != nil {
		reportRefreshNotStarted(progress, age, err, ga)
		return nil
	}
	fmt.Fprintf(progress, "[warden] prepared Windows layer is %s old; refreshing "+
		"it in the background (log: %s)\n", age, logPath)
	return nil
}

func reportRefreshNotStarted(progress io.Writer, age string, err error, ga string) {
	fmt.Fprintf(progress, "[warden] prepared Windows layer is %s old, and its "+
		"background refresh did not start: %v; run `%s` to refresh it\n",
		age, err, prepareFix(ga))
}

// startDetached and tryHostLock are driver's; tests replace them.
var (
	startDetached = driver.StartDetached
	tryHostLock   = driver.TryHostLock
)

// configureWindowsBuildVM copies the writable UEFI vars and builds the Windows
// provisioning seed (FAT image) for the build VM, filling cfg.VarsFD and
// cfg.SeedISO.
func configureWindowsBuildVM(cfg *buildVMConfig, workDir string, link buildlink.Link) error {
	// Writable UEFI vars carrying the Windows Boot Manager entry that
	// bcdboot wrote during image prep; copied per-build so the base's
	// vars store is never mutated.
	ga := goArchOf(cfg.Arch)
	srcVars := cfg.BaseVarsFD
	if srcVars == "" { // an explicit --image: the installed base's vars
		srcVars = windowsVarsPath(ga)
	}
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
	seedDir, err := windowsSeedDir(workDir, goArchOf(cfg.Arch), link)
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
		seedDir, seedImg, winguest.SeedLabel); err != nil {
		return fmt.Errorf("generating windows seed FAT: %w", err)
	}
	cfg.SeedISO = seedImg
	return nil
}

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

// rebuildDirectBoot is the one-time rebuild of the direct-boot initramfs.
const rebuildDirectBoot = "tools/build-vm/build-initramfs.sh"

// directBootStamp describes the direct-boot initramfs's link-channel stamp,
// with Path empty when the assets are not found.
func directBootStamp() driver.LauncherStamp {
	st := driver.LauncherStamp{What: "qemu direct-boot initramfs", Rebuild: rebuildDirectBoot}
	if paths, ok := relayvm.Find(relayvm.QEMUBuild); ok {
		st.Path = relayvm.StampPath(relayvm.QEMUBuild[1], paths[1])
	}
	return st
}

// LauncherStamps lists the qemu launcher assets with a link-channel stamp:
// the direct-boot initramfs (only builds without a disk image use it).
func LauncherStamps() []driver.LauncherStamp {
	return []driver.LauncherStamp{directBootStamp()}
}

// resolveBuildAssets finds the direct-boot kernel and initramfs, refusing an
// initramfs whose init predates the per-build link.
func (d *Driver) resolveBuildAssets() (kernel, initrd string, err error) {
	if paths, ok := relayvm.Find(relayvm.QEMUBuild); ok {
		if err := directBootStamp().Check(); err != nil {
			return "", "", err
		}
		return paths[0], paths[1], nil
	}
	return "", "", driver.Fixable(
		"build them from the source tree: "+
			"tools/relay-vm/build-initramfs.sh && "+
			"tools/build-vm/build-initramfs.sh, or pass --image <qcow2> "+
			"(warden image prepare --driver qemu)",
		"qemu build VM boot assets (kernel + initramfs) not found")
}

func (d *Driver) resolveRelayAssets() (kernel, initrd string, err error) {
	paths, ok := relayvm.Find(relayvm.QEMURelay)
	if !ok {
		return "", "", driver.Fixable(
			"build them from the source tree: tools/relay-vm/build-initramfs.sh",
			"qemu relay VM assets (kernel + initramfs) not found")
	}
	return paths[0], paths[1], nil
}

// relayInitrd returns the relay VM kernel and the per-build relay initrd
// (relayvm.RelayInitrd) in workDir.
func (d *Driver) relayInitrd(workDir string) (kernel, initrd string, err error) {
	kernel, base, err := d.resolveRelayAssets()
	if err != nil {
		return "", "", err
	}
	initrd, err = relayvm.RelayInitrd(base, workDir, runtime.GOARCH)
	if err != nil {
		return "", "", err
	}
	return kernel, initrd, nil
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

package vz

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Driver is the vz driver, on Apple's Virtualization.framework for
// macOS/arm64 targets.
//
// Architecture (host relay + build VM):
//
//	Host (orchestrator)
//	├── Creates isolated virtual network (socket pair)
//	├── Starts Relay (host process, --mode=host, frames on fd 3) and claims it
//	│   ├── Reads raw Ethernet frames from socketpair via gvisor netstack
//	│   └── Has native internet access (host networking)
//	├── Boots Build VM (macOS from IPSW)
//	│   └── Single interface → socket pair → Relay (sole gateway)
//	└── Waits for build completion; outputs arrive through the collector
//
// Network isolation is topological: the build VM's only network path is
// through the relay process. No iptables, no vmnet needed.
type Driver struct {
	// Cache is the image cache; nil is the default cache directory's.
	Cache *ImageCache
	// Verbose echoes the host relay's log to stderr as well as relay.log.
	Verbose bool
}

// New returns a Driver whose image cache is the default cache directory's.
// It does no host work: the directory is resolved when the driver first
// needs it.
func New() *Driver { return &Driver{} }

// cache is the driver's image cache, the default cache directory's unless
// one was set.
func (d *Driver) cache() *ImageCache {
	if d.Cache == nil {
		d.Cache = &ImageCache{CacheDir: defaultCacheDir()}
	}
	return d.Cache
}

// Preflight is the vz driver's probe: the platform, and, for a base image
// that needs no restore, the image, its link stamp, the host link and the
// relay's resolvers (which replace req.DNS). An IPSW URL is restored, and
// so checked, only by Run. It fails with a fixable platform error on any
// host but macOS/arm64, and on macOS/arm64 when the binary was built without
// the Virtualization.framework bindings (cgo) or lacks the virtualization
// entitlement (see checkPlatform).
func (d *Driver) Preflight(req *driver.BuildRequest) error {
	if err := checkPlatform(); err != nil {
		return err
	}
	if strings.Contains(req.Image, "://") {
		return nil
	}
	diskImage, err := d.resolveImage(req.Image)
	if err != nil {
		return fmt.Errorf("resolving image: %w", err)
	}
	return checkRequest(diskImage, d.cache(), req)
}

// Run is the vz driver's run of one build (specs/driver-contract/v1.md, The
// build): it starts the host relay with the claim token and reports it
// through host, with guest darwin, boots the macOS build VM with the relay as
// its only network path once warden has claimed it, and stops both when
// host.Wait returns. Its work directory and relay child are named from the
// build ID; the relay writes relay.log into the diagnostics directory. Under
// --keep-vms it leaves the work directory, with the disk clones, and returns
// it as kept.
func (d *Driver) Run(ctx context.Context, req *driver.BuildRequest, start *driver.Start,
	host driver.Host) (kept []string, err error) {
	if err := checkPlatform(); err != nil {
		return nil, err
	}
	// The base image is checked first: without one nothing else can run.
	diskImage, err := d.resolveImage(req.Image)
	if err != nil {
		return nil, fmt.Errorf("resolving image: %w", err)
	}
	if err := checkRequest(diskImage, d.cache(), req); err != nil {
		return nil, err
	}

	workDir, err := driver.NewWorkDir(req, "vz-"+start.BuildID)
	if err != nil {
		return nil, err
	}
	defer driver.RemoveWorkDir(req, workDir)
	if req.KeepVMs {
		kept = []string{workDir}
	}

	// macOS guests run a shell script: warden dropped the Containerfile
	// before it staged this one.
	env, err := describeBuild(d.cache().LayerCache(), diskImage, req.BuildScript)
	if err != nil {
		return kept, fmt.Errorf("describing build environment: %w", err)
	}

	buildDisk, buildAux, err := cloneBuildDisks(workDir, diskImage)
	if err != nil {
		return kept, err
	}

	// Create the isolated virtual network (socket pair), start the host-side
	// relay (it reads raw Ethernet frames from its end of the socketpair via
	// gvisor netstack and has native internet access; the build VM can only
	// reach the network through it), and report it to be claimed with the
	// build's config.
	var vnet *VirtualNetwork
	relayStep := func() (func(), error) {
		v, relayProc, err := d.setupNetworkAndRelay(ctx, host, req, env, workDir,
			start)
		if err != nil {
			return nil, err
		}
		vnet = v
		return func() {
			relayProc.Stop()
			_ = v.Close()
		}, nil
	}
	// Boot Build VM: macOS with single network interface (socketpair to relay).
	// No shared directory — all communication goes through the relay HTTP API.
	// Platform state files (hardware-model, machine-id, aux-storage) live
	// alongside the original disk image, not the COW clone.
	buildStep := func() (func(), error) {
		buildVM, err := d.bootBuildVM(
			buildDisk, buildAux, filepath.Dir(diskImage), vnet,
			driver.OrDefault(req.CPUs, 4), driver.OrDefault(req.MemoryMB, 8192))
		if err != nil {
			return nil, bootError(err)
		}
		return func() { _ = buildVM.Stop() }, nil
	}
	return kept, runVMs(ctx, host, relayStep, buildStep)
}

// runVMs runs the host relay step (virtual network, relay, claim) and only
// once it has returned, with the relay serving the build link's DHCP lease,
// the build VM step, then reports the environment started and waits for
// warden's stop. Each step returns its stop.
func runVMs(ctx context.Context, host driver.Host,
	relayStep, buildStep func() (func(), error)) error {
	stopRelay, err := relayStep()
	if err != nil {
		return err
	}
	defer stopRelay()
	stopBuild, err := buildStep()
	if err != nil {
		return err
	}
	defer stopBuild()
	if err := host.Started(); err != nil {
		return err
	}
	host.Wait(ctx)
	return nil
}

// bootError explains a build VM that failed to boot.
func bootError(err error) error {
	// VZErrorDomain code 6: macOS caps running macOS guests at two.
	if strings.Contains(err.Error(), "VZErrorDomain code=6") {
		return driver.Fixable(
			"wait for another vz build, or another macOS VM "+
				"(UTM, Tart, ...), to finish, then retry",
			"booting build VM: macOS allows at most two running "+
				"macOS virtual machines, and two are running")
	}
	return fmt.Errorf("booting build VM: %w", err)
}

// cloneBuildDisks makes per-build copy-on-write clones of the base disk and
// its auxiliary storage. The auxiliary storage (NVRAM) is written at boot and
// locked by the running VM, so each build boots from its own clone too;
// otherwise a second concurrent build fails with "Failed to lock auxiliary
// storage".
func cloneBuildDisks(workDir, diskImage string) (buildDisk, buildAux string, err error) {
	buildDisk = filepath.Join(workDir, "build-disk.img")
	if err = CloneDisk(diskImage, buildDisk); err != nil {
		return "", "", fmt.Errorf("cloning build disk: %w", err)
	}
	buildAux = filepath.Join(workDir, "aux-storage")
	auxSrc := filepath.Join(filepath.Dir(diskImage), "aux-storage")
	if err = CloneDisk(auxSrc, buildAux); err != nil {
		return "", "", fmt.Errorf("cloning auxiliary storage: %w", err)
	}
	return buildDisk, buildAux, nil
}

// setupNetworkAndRelay creates the isolated virtual network, starts the
// host-side relay with the claim token, and reports it through host, which
// returns once warden has claimed it. On any failure it tears down whatever
// it already created; on success the caller owns vnet and relayProc and must
// close/stop them.
func (d *Driver) setupNetworkAndRelay(
	ctx context.Context, host driver.Host, req *driver.BuildRequest,
	env *relaycfg.Environment, workDir string, start *driver.Start,
) (*VirtualNetwork, *RelayProcess, error) {
	vnet, err := NewVirtualNetwork(req.Link())
	if err != nil {
		return nil, nil, fmt.Errorf("creating virtual network: %w", err)
	}
	claimPort, err := driver.FreeLoopbackPort()
	if err != nil {
		_ = vnet.Close()
		return nil, nil, err
	}
	relayProc, err := d.startHostRelay(workDir, start, vnet, claimPort)
	if err != nil {
		_ = vnet.Close()
		return nil, nil, fmt.Errorf("starting relay: %w", err)
	}
	claimAddr := net.JoinHostPort("127.0.0.1", fmt.Sprint(claimPort))
	if err := host.ClaimRelay(ctx, driver.RelayStarted{
		Claim: driver.ClaimEndpoint{Network: driver.ClaimTCP, Address: claimAddr},
		ClaimValues: driver.ClaimValues{Environment: env,
			BuildLink:   req.Link().Claim(),
			UpstreamDNS: driver.UpstreamDNS(req.DNS)},
		GuestOS:     "darwin",
		OldRelayFix: driver.OldRelayFix,
	}); err != nil {
		relayProc.Stop()
		_ = vnet.Close()
		return nil, nil, err
	}
	return vnet, relayProc, nil
}

// PrepareCommand is how a user prepares the vz base image. It needs root:
// readying the image writes root-owned system files into it.
const PrepareCommand = "sudo warden image prepare --driver vz"

// resolveImage returns the base disk for a build. A build never restores an
// image itself: that is a 25+ GB download, needs root, and belongs to
// `warden image prepare`, so a missing image fails immediately with that
// command.
func (d *Driver) resolveImage(image string) (string, error) {
	if image == "" {
		dir, err := d.cache().ImageDir()
		if err != nil {
			return "", err
		}
		if dir == "" {
			return "", driver.Fixable(PrepareCommand,
				"no prepared macOS base image in %s", d.cache().CacheDir)
		}
		return filepath.Join(dir, "disk.img"), nil
	}
	if _, err := os.Stat(image); err == nil {
		return image, nil
	}
	if !strings.Contains(image, "://") {
		return "", driver.Fixable("",
			"--image %s: no such file (give a disk.img path or an IPSW URL)", image)
	}
	return d.cache().RestoreIPSW(image)
}

// RelayProcess wraps a host-side relay subprocess.
type RelayProcess struct {
	cmd *exec.Cmd
	log *os.File
}

// Stop sends SIGTERM and waits for the relay to exit.
func (rp *RelayProcess) Stop() {
	if rp.cmd != nil && rp.cmd.Process != nil {
		_ = rp.cmd.Process.Signal(os.Interrupt)
		_ = rp.cmd.Wait()
	}
	if rp.log != nil {
		_ = rp.log.Close()
	}
}

// startHostRelay starts the relay as a host process in fd-ingress mode, named
// warden-relay-<build ID>, with the claim token. It reads raw Ethernet frames
// from vnet.RelaySocketFD (gvisor netstack) and waits, unconfigured, for the
// claim on 127.0.0.1:claimPort. Its log is relay.log in the diagnostics
// directory.
func (d *Driver) startHostRelay(
	workDir string, start *driver.Start, vnet *VirtualNetwork, claimPort int,
) (*RelayProcess, error) {
	built, err := driver.GuestBinary("relay", "darwin", "arm64")
	if err != nil {
		return nil, err
	}
	// macOS may block unsigned binaries from network operations via Endpoint
	// Security, so run an ad-hoc signed copy.
	relayBin := filepath.Join(workDir, "warden-relay-"+start.BuildID)
	if err := driver.CopyFile(built, relayBin); err != nil {
		return nil, err
	}
	_ = exec.Command("codesign", "--force", "--sign", "-", relayBin).Run()

	// The relay FD will be passed as fd 3 (first extra FD after stdin/out/err).
	// It takes the build link from the claim, not --subnet.
	cmd := exec.Command(relayBin,
		"--mode=host",
		"--fd=3",
		"--claim-addr="+net.JoinHostPort("127.0.0.1", fmt.Sprint(claimPort)),
	)
	cmd.Env = append(os.Environ(), relaycfg.TokenEnv+"="+start.ClaimToken)
	// The relay's own log goes to <output>/relay.log, as on the container
	// driver; it reaches the terminal only with --verbose.
	logFile, err := os.Create(filepath.Join(start.DiagnosticsDir, "relay.log"))
	if err != nil {
		return nil, fmt.Errorf("creating relay.log: %w", err)
	}
	var logOut io.Writer = logFile
	if d.Verbose {
		logOut = io.MultiWriter(logFile, os.Stderr)
	}
	cmd.Stdout = logOut
	cmd.Stderr = logOut

	// Pass the relay socketpair FD as ExtraFiles[0] → becomes fd 3 in child.
	// The child inherits its own copy at Start, so the parent's is closed here.
	relaySock, err := vnet.RelaySocket()
	if err != nil {
		logFile.Close()
		return nil, err
	}
	defer relaySock.Close()
	cmd.ExtraFiles = []*os.File{relaySock}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("starting relay process: %w", err)
	}

	return &RelayProcess{cmd: cmd, log: logFile}, nil
}

// bootBuildVM creates and starts the build VM (macOS).
// Single network interface: private link to relay (sole network path).
// No shared directory — warden-io is baked into the disk image and all
// runtime communication goes through the relay's HTTP API.
func (d *Driver) bootBuildVM(
	diskImage, auxStorage, platformDir string, vnet *VirtualNetwork, cpus, memoryMB int,
) (*VM, error) {
	imgDir := platformDir

	vm, err := NewMacOSVM(macOSVMConfig{
		CPUs:               cpus,
		MemoryMB:           memoryMB,
		DiskImagePath:      diskImage,
		AuxStoragePath:     auxStorage,
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

// checkPlatform refuses the driver unless this host and this build can run
// it: macOS on Apple Silicon, a build with the bindings, and a warden signed
// with the virtualization entitlement. It is the first thing every entry
// point runs, before any relay, download or VM.
func checkPlatform() error {
	if err := platformError(runtime.GOOS, runtime.GOARCH, hasBindings); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	out, err := exec.Command(
		"codesign", "-d", "--entitlements", ":-", exe).Output()
	if err != nil || !strings.Contains(string(out),
		"com.apple.security.virtualization") {
		return driver.Fixable(
			"sign it: codesign --sign - --entitlements entitlements.plist "+exe+
				" (make build does this)",
			"vz driver requires warden to be signed with the "+
				"com.apple.security.virtualization entitlement")
	}
	return nil
}

// platformError is checkPlatform's verdict on the target and on whether the
// build compiled the bindings, as a pure function so every row is testable
// on any host. A build without the bindings is refused even on darwin/arm64,
// where its stub would otherwise hand a build a nil VM and no error.
func platformError(goos, goarch string, bindings bool) error {
	if goos != "darwin" || goarch != "arm64" {
		return driver.Fixable("use --driver qemu or --driver container on this host",
			"vz driver requires macOS on Apple Silicon (got %s/%s)", goos, goarch)
	}
	if !bindings {
		return driver.Fixable("build warden from source on this Mac "+
			"(xcode-select --install, then make build), "+
			"or use --driver container or --driver qemu",
			"this warden was built without cgo, so it has no vz driver "+
				"(released binaries are built this way)")
	}
	return nil
}

// DefaultCacheDir returns the default path for cached VM images. Under sudo
// it is the invoking user's, not root's, so `sudo warden image prepare`
// prepares the image that user's builds use.
func DefaultCacheDir() string {
	home, _ := os.UserHomeDir()
	if u := sudoUser(); u != nil {
		home = u.HomeDir
	}
	return filepath.Join(home, ".cache", "warden", "images")
}

// sudoUser is the user who ran warden through sudo, or nil when warden is
// not running as root on someone's behalf.
func sudoUser() *user.User {
	if os.Geteuid() != 0 {
		return nil
	}
	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" {
		return nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return nil
	}
	return u
}

func defaultCacheDir() string {
	return DefaultCacheDir()
}

// checkLink refuses, before anything boots, an image whose warden-boot
// predates the per-build link and a link a host interface uses. cache is the
// image cache, which decides the fix the refusal names.
func checkLink(diskImage string, cache *ImageCache, req *driver.BuildRequest) error {
	if err := imageStamp(filepath.Dir(diskImage), cache).Check(); err != nil {
		return err
	}
	return driver.CheckHostLink(req.Link())
}

// checkRequest is checkLink, then the relay's resolvers ([vm] dns, or the
// Mac's), which replace req.DNS for the claim (upstream_dns).
func checkRequest(diskImage string, cache *ImageCache, req *driver.BuildRequest) error {
	if err := checkLink(diskImage, cache, req); err != nil {
		return err
	}
	return setRelayDNS(req, os.ReadFile)
}

// imageStamp describes a prepared image's link-channel stamp. An image
// prepared before warden-boot took the build link by DHCP bakes the old
// address and must be re-prepared once (in place, no IPSW restore). The fix
// depends on where the image is: `warden image prepare` with no source
// refreshes the active image, with the directory as its source refreshes
// another cache image (and makes it active), and cannot re-prepare an
// --image disk outside the cache.
func imageStamp(imgDir string, cache *ImageCache) driver.LauncherStamp {
	rebuild := PrepareCommand
	switch active, _ := cache.ImageDir(); {
	case !insideDir(imgDir, cache.CacheDir):
		rebuild = customImageFix(cache.CacheDir)
	case !sameDir(imgDir, active):
		rebuild = PrepareCommand + " " + imgDir +
			" (refreshes it and makes it active), " +
			"or omit --image to build from the active image"
	}
	return driver.LauncherStamp{What: "vz prepared macOS image (warden-boot)",
		Path: filepath.Join(imgDir, linkStampName), Rebuild: rebuild,
		Required: true}
}

// customImageFix is the remedy for a stale --image disk outside the image
// cache: build from the cache's image, which PrepareCommand prepares or
// refreshes in place.
func customImageFix(cacheDir string) string {
	return "omit --image to build from the prepared image in " + cacheDir +
		" (`" + PrepareCommand + "` prepares or refreshes it); " +
		"an image outside that cache cannot be re-prepared in place"
}

// insideDir reports whether path is dir or below it.
func insideDir(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// LauncherStamps lists the vz assets with a link-channel stamp: the active
// prepared image in the default cache, when there is one.
func LauncherStamps() []driver.LauncherStamp {
	return launcherStamps(&ImageCache{CacheDir: defaultCacheDir()})
}

func launcherStamps(cache *ImageCache) []driver.LauncherStamp {
	dir, _ := cache.ImageDir()
	if dir == "" {
		return []driver.LauncherStamp{{What: "vz prepared macOS image (warden-boot)",
			Rebuild: PrepareCommand, Required: true}}
	}
	return []driver.LauncherStamp{imageStamp(dir, cache)}
}

// Preflight checks the host can run vz builds (Apple Silicon macOS, a build
// with the Virtualization.framework bindings (cgo), and a binary signed with
// the virtualization entitlement); checkPlatform is the check. It is
// `warden doctor --driver vz`.
func Preflight() error { return checkPlatform() }

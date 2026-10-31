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

// Driver implements driver.Driver using Apple's Virtualization.framework
// for macOS/arm64 targets.
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
	Cache *ImageCache
	// Verbose echoes the host relay's log to stderr as well as relay.log.
	Verbose bool
}

// New returns a Driver whose image cache lives in the default cache
// directory.
func New() *Driver {
	cacheDir := defaultCacheDir()
	return &Driver{
		Cache: &ImageCache{CacheDir: cacheDir},
	}
}

// Name implements driver.Driver; it returns "vz".
func (d *Driver) Name() string { return "vz" }

// StartBuild implements driver.Driver. It starts the host relay, boots the
// macOS build VM with the relay as its only network path, and waits for the
// build to finish. It fails with a fixable platform error on any host but
// macOS/arm64, and on macOS/arm64 when the binary lacks the virtualization
// entitlement.
func (d *Driver) StartBuild(
	ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
	if err := checkPlatform(); err != nil {
		return nil, err
	}
	// The base image is checked first: without one nothing else can run.
	diskImage, err := d.resolveImage(req.Image)
	if err != nil {
		return nil, fmt.Errorf("resolving image: %w", err)
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = "warden-output"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	workDir, err := driver.NewWorkDir(req, "vz")
	if err != nil {
		return nil, err
	}
	defer driver.RemoveWorkDir(req, workDir)

	// macOS guests run a shell script; a Containerfile is not translated.
	scriptReq := *req
	scriptReq.Containerfile = ""
	buildScript, err := driver.StageBuildScript(&scriptReq, workDir)
	if err != nil {
		return nil, fmt.Errorf("staging build script: %w", err)
	}
	env, err := describeBuild(d.Cache.LayerCache(), diskImage, buildScript)
	if err != nil {
		return nil, fmt.Errorf("describing build environment: %w", err)
	}

	// The relay is a host process here, so the collector is on loopback.
	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   outputDir,
		ContextDir:  req.ContextDir,
		BuildScript: buildScript,
		GuestOS:     "darwin",
		OutputTee:   driver.ConsoleTee(req),
	})
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	buildDisk, buildAux, err := cloneBuildDisks(workDir, diskImage)
	if err != nil {
		return nil, err
	}

	// Create the isolated virtual network (socket pair), start the host-side
	// relay (it reads raw Ethernet frames from its end of the socketpair via
	// gvisor netstack and has native internet access; the build VM can only
	// reach the network through it), and claim it with the build's config.
	vnet, relayProc, err := d.setupNetworkAndRelay(ctx, sess, req, env, workDir, outputDir)
	if err != nil {
		return nil, err
	}
	defer vnet.Close()
	defer relayProc.Stop()

	// Boot Build VM: macOS with single network interface (socketpair to relay).
	// No shared directory — all communication goes through the relay HTTP API.
	// Platform state files (hardware-model, machine-id, aux-storage) live
	// alongside the original disk image, not the COW clone.
	buildVM, err := d.bootBuildVM(
		buildDisk, buildAux, filepath.Dir(diskImage), vnet,
		driver.OrDefault(req.CPUs, 4), driver.OrDefault(req.MemoryMB, 8192))
	if err != nil {
		// VZErrorDomain code 6: macOS caps running macOS guests at two.
		if strings.Contains(err.Error(), "VZErrorDomain code=6") {
			return nil, driver.Fixable(
				"wait for another vz build, or another macOS VM "+
					"(UTM, Tart, ...), to finish, then retry",
				"booting build VM: macOS allows at most two running "+
					"macOS virtual machines, and two are running")
		}
		return nil, fmt.Errorf("booting build VM: %w", err)
	}
	defer func() { _ = buildVM.Stop() }()

	if err := sess.Wait(ctx); err != nil {
		return nil, err
	}
	driver.FinalizeOutput(outputDir, req.Compress)
	return &driver.BuildResult{OutputDir: outputDir}, nil
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
// host-side relay, and claims it with the build's configuration. On any
// failure it tears down whatever it already created; on success the caller
// owns vnet and relayProc and must close/stop them.
func (d *Driver) setupNetworkAndRelay(
	ctx context.Context, sess *driver.Session, req *driver.BuildRequest,
	env *relaycfg.Environment, workDir, outputDir string,
) (*VirtualNetwork, *RelayProcess, error) {
	vnet, err := NewVirtualNetwork()
	if err != nil {
		return nil, nil, fmt.Errorf("creating virtual network: %w", err)
	}
	claimPort, err := driver.FreeLoopbackPort()
	if err != nil {
		_ = vnet.Close()
		return nil, nil, err
	}
	claimToken, err := driver.RandomToken()
	if err != nil {
		_ = vnet.Close()
		return nil, nil, err
	}
	relayProc, err := d.startHostRelay(workDir, outputDir, vnet, claimPort, claimToken)
	if err != nil {
		_ = vnet.Close()
		return nil, nil, fmt.Errorf("starting relay: %w", err)
	}
	claimAddr := net.JoinHostPort("127.0.0.1", fmt.Sprint(claimPort))
	if _, err := sess.ClaimRelay(ctx, claimAddr, claimToken, req, env); err != nil {
		relayProc.Stop()
		_ = vnet.Close()
		return nil, nil, err
	}
	return vnet, relayProc, nil
}

// Exec implements driver.Driver. Interactive mode is not implemented for vz:
// it returns driver.ErrExecNotSupported once the platform checks pass, and
// otherwise the same fixable platform error as StartBuild (another host, or a
// binary without the virtualization entitlement).
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	if err := checkPlatform(); err != nil {
		return err
	}
	// TODO: boot environment, expose serial console for interactive use
	return driver.ErrExecNotSupported
}

// Close implements driver.Driver. Each build stops its own VM and relay, so
// the driver holds nothing to release.
func (d *Driver) Close() error { return nil }

// PrepareCommand is how a user prepares the vz base image. It needs root:
// readying the image writes root-owned system files into it.
const PrepareCommand = "sudo warden image prepare --driver vz"

// resolveImage returns the base disk for a build. A build never restores an
// image itself: that is a 25+ GB download, needs root, and belongs to
// `warden image prepare`, so a missing image fails immediately with that
// command.
func (d *Driver) resolveImage(image string) (string, error) {
	if image == "" {
		dir, err := d.Cache.ImageDir()
		if err != nil {
			return "", err
		}
		if dir == "" {
			return "", driver.Fixable(PrepareCommand,
				"no prepared macOS base image in %s", d.Cache.CacheDir)
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
	return d.Cache.RestoreIPSW(image)
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

// startHostRelay starts the relay as a host process in fd-ingress mode. It
// reads raw Ethernet frames from vnet.RelaySocketFD (gvisor netstack) and
// waits, unconfigured, for the claim on 127.0.0.1:claimPort.
func (d *Driver) startHostRelay(
	workDir, outputDir string, vnet *VirtualNetwork,
	claimPort int, claimToken string,
) (*RelayProcess, error) {
	built, err := driver.GuestBinary("relay", "darwin", "arm64")
	if err != nil {
		return nil, err
	}
	// macOS may block unsigned binaries from network operations via Endpoint
	// Security, so run an ad-hoc signed copy.
	relayBin := filepath.Join(workDir, "relay")
	if err := driver.CopyFile(built, relayBin); err != nil {
		return nil, err
	}
	_ = exec.Command("codesign", "--force", "--sign", "-", relayBin).Run()

	subnet := fmt.Sprintf("%s/%d",
		vnet.Subnet.RelayIP.Mask(vnet.Subnet.Netmask),
		maskBits(vnet.Subnet.Netmask))

	// The relay FD will be passed as fd 3 (first extra FD after stdin/out/err).
	cmd := exec.Command(relayBin,
		"--mode=host",
		"--fd=3",
		"--subnet="+subnet,
		"--claim-addr="+net.JoinHostPort("127.0.0.1", fmt.Sprint(claimPort)),
	)
	cmd.Env = append(os.Environ(), relaycfg.TokenEnv+"="+claimToken)
	// The relay's own log goes to <output>/relay.log, as on the container
	// driver; it reaches the terminal only with --verbose.
	logFile, err := os.Create(filepath.Join(outputDir, "relay.log"))
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
	cmd.ExtraFiles = []*os.File{
		os.NewFile(uintptr(vnet.RelaySocketFD), "relay-socket"),
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("starting relay process: %w", err)
	}

	return &RelayProcess{cmd: cmd, log: logFile}, nil
}

func maskBits(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
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

func checkPlatform() error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return driver.Fixable("use --driver qemu or --driver container on this host",
			"vz driver requires macOS on Apple Silicon (got %s/%s)",
			runtime.GOOS, runtime.GOARCH)
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

// Preflight checks the host can run vz builds (Apple Silicon macOS and a
// binary signed with the virtualization entitlement). It is
// `warden doctor --driver vz`.
func Preflight() error { return checkPlatform() }

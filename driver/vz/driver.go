package vz

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
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

func (d *Driver) StartBuild(
	ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
	if err := checkPlatform(); err != nil {
		return nil, err
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

	workDir, err := os.MkdirTemp("", "warden-vz-")
	if err != nil {
		return nil, fmt.Errorf("creating work dir: %w", err)
	}
	defer driver.RemoveWorkDir(req, workDir)

	// macOS guests run a shell script; a Containerfile is not translated.
	scriptReq := *req
	scriptReq.Containerfile = ""
	buildScript, err := driver.StageBuildScript(&scriptReq, workDir)
	if err != nil {
		return nil, fmt.Errorf("staging build script: %w", err)
	}

	// The relay is a host process here, so the collector is on loopback.
	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   outputDir,
		ContextDir:  req.ContextDir,
		BuildScript: buildScript,
		OutputTee:   driver.ConsoleTee(req),
	})
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	// Resolve and clone macOS disk image for build VM
	diskImage, err := d.resolveImage(req.Image)
	if err != nil {
		return nil, fmt.Errorf("resolving image: %w", err)
	}

	buildDisk := filepath.Join(workDir, "build-disk.img")
	if err := CloneDisk(diskImage, buildDisk); err != nil {
		return nil, fmt.Errorf("cloning build disk: %w", err)
	}

	// Create isolated virtual network (socket pair)
	vnet, err := NewVirtualNetwork()
	if err != nil {
		return nil, fmt.Errorf("creating virtual network: %w", err)
	}
	defer vnet.Close()

	// Start the host-side relay. It reads raw Ethernet frames from its end
	// of the socketpair (gvisor netstack) and has native internet access;
	// the build VM can only reach the network through it.
	claimPort, err := driver.FreeLoopbackPort()
	if err != nil {
		return nil, err
	}
	claimToken, err := driver.RandomToken()
	if err != nil {
		return nil, err
	}
	relayProc, err := d.startHostRelay(workDir, vnet, claimPort, claimToken)
	if err != nil {
		return nil, fmt.Errorf("starting relay: %w", err)
	}
	defer relayProc.Stop()

	claimAddr := net.JoinHostPort("127.0.0.1", fmt.Sprint(claimPort))
	if _, err := sess.ClaimRelay(ctx, claimAddr, claimToken, req, nil); err != nil {
		return nil, err
	}

	// Boot Build VM: macOS with single network interface (socketpair to relay).
	// No shared directory — all communication goes through the relay HTTP API.
	// Platform state files (hardware-model, machine-id, aux-storage) live
	// alongside the original disk image, not the COW clone.
	buildVM, err := d.bootBuildVM(
		buildDisk, filepath.Dir(diskImage), vnet,
		driver.OrDefault(req.CPUs, 4), driver.OrDefault(req.MemoryMB, 8192))
	if err != nil {
		return nil, fmt.Errorf("booting build VM: %w", err)
	}
	defer func() { _ = buildVM.Stop() }()

	if err := sess.Wait(ctx); err != nil {
		return nil, err
	}
	driver.FinalizeOutput(outputDir, req.Compress)
	return &driver.BuildResult{OutputDir: outputDir}, nil
}

func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	if err := checkPlatform(); err != nil {
		return err
	}
	// TODO: boot environment, expose serial console for interactive use
	return driver.ErrExecNotSupported
}

func (d *Driver) Close() error { return nil }

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
		_ = rp.cmd.Process.Signal(os.Interrupt)
		_ = rp.cmd.Wait()
	}
}

// startHostRelay starts the relay as a host process in fd-ingress mode. It
// reads raw Ethernet frames from vnet.RelaySocketFD (gvisor netstack) and
// waits, unconfigured, for the claim on 127.0.0.1:claimPort.
func (d *Driver) startHostRelay(workDir string, vnet *VirtualNetwork, claimPort int, claimToken string) (*RelayProcess, error) {
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
	cmd.Stdout = os.Stderr
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
// Single network interface: private link to relay (sole network path).
// No shared directory — warden-io is baked into the disk image and all
// runtime communication goes through the relay's HTTP API.
func (d *Driver) bootBuildVM(
	diskImage, platformDir string, vnet *VirtualNetwork, cpus, memoryMB int,
) (*VM, error) {
	imgDir := platformDir

	vm, err := NewMacOSVM(macOSVMConfig{
		CPUs:               cpus,
		MemoryMB:           memoryMB,
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
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	out, err := exec.Command(
		"codesign", "-d", "--entitlements", ":-", exe).Output()
	if err != nil || !strings.Contains(string(out),
		"com.apple.security.virtualization") {
		return fmt.Errorf(
			"vz driver requires the com.apple.security.virtualization "+
				"entitlement\n"+
				"  Sign the binary: codesign --sign - "+
				"--entitlements entitlements.plist %s", exe)
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

// Preflight checks the host can run vz builds (Apple Silicon macOS and a
// binary signed with the virtualization entitlement). It is
// `warden doctor --driver vz`.
func Preflight() error { return checkPlatform() }

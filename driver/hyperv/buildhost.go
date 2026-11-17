package hyperv

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Build VM defaults. The build VM is the untrusted guest: its sole NIC is on the
// Private build switch, so its only peer is the relay on the build link and it
// has no route to the host network stack or the internet.
const (
	defaultBuildMemoryMB = 4096
	defaultBuildCPUs     = 4
)

// buildConfig is the fully-resolved input to runBuild: every asset path is
// already located and the network already chosen. runBuild is pure
// orchestration over the Provisioner, StartRelay and the driver.Host, so it is
// unit-testable with a fake Provisioner and never touches asset resolution,
// image acquisition, or seed generation itself.
type buildConfig struct {
	BuildID string
	// Token is warden's claim token, for the relay's seed.
	Token string
	// OutputDir is the build's output directory, which the exit report names
	// for the guest's last output.
	OutputDir string

	// Relay assets (relay VM). RelayOverlay is the per-build differencing child
	// created off the static, read-only RelayBootVHDX.
	RelayBootVHDX string
	RelayOverlay  string

	// Build-guest assets (build VM). BuildOverlay is the per-build differencing
	// child created off the read-only BuildBaseVHDX. Exactly one of SeedISO
	// (Linux cloud-init CIDATA) or SeedVHDX (Windows unattend) carries the
	// per-build provisioning media.
	BuildBaseVHDX string
	BuildOverlay  string
	SeedISO       string
	SeedVHDX      string
	IsWindows     bool

	// Generation is the build VM's Hyper-V generation, dictated by the base
	// image format: 1 for a BIOS/MBR .vhd (the Windows Server eval image boots
	// this way with no conversion), 2 for a UEFI/GPT .vhdx. It MUST match the
	// base disk or the VM won't boot. Zero defaults to 2.
	Generation int

	// Network. Both VMs share a Private build switch (relay <-> build only):
	// the ephemeral strategy's BuildSwitch, or the lowest free of the durable
	// strategy's Slots, which StartRelay allocates. The relay additionally
	// attaches the NAT switch for controlled egress, at an address it
	// allocates in NATPrefix with HostIP as its gateway. The build VM never
	// attaches to the NAT switch.
	BuildSwitch string
	Slots       []string
	NATSwitch   string
	NATPrefix   netip.Prefix
	HostIP      netip.Addr
	// RelaySeedDir is where the relay seed is staged (under the work dir).
	RelaySeedDir string

	MemoryMB int  // build VM; default 4096
	CPUs     int  // build VM; default 4
	KeepVMs  bool // leave both VMs and their overlays for inspection

	// Request carries the build link and the relay's resolvers.
	Request *driver.BuildRequest
	// Environment is the ledger's environment record (what the build VM
	// boots from); nil records none.
	Environment *relaycfg.Environment

	// Test hooks (zero = production defaults): the claim port, the relay
	// seed ISO builder, and the build VM's state poll.
	claimPort   int
	makeSeedISO func(seedDir, iso string) error
	vmPoll      time.Duration // build VM state poll (zero = 3 s)
}

// validateBuildConfig rejects a buildConfig that is missing a required field or
// whose overlay format cannot match its base image.
func validateBuildConfig(cfg buildConfig) error {
	switch {
	case cfg.BuildID == "":
		return fmt.Errorf("runBuild: empty build ID")
	case cfg.OutputDir == "":
		return fmt.Errorf("runBuild: empty output dir")
	case cfg.RelayBootVHDX == "":
		return fmt.Errorf("runBuild: empty relay boot VHDX")
	case cfg.RelayOverlay == "":
		return fmt.Errorf("runBuild: empty relay overlay path")
	case cfg.BuildBaseVHDX == "":
		return fmt.Errorf("runBuild: empty build base VHDX")
	case cfg.BuildOverlay == "":
		return fmt.Errorf("runBuild: empty build overlay path")
	case cfg.SeedISO == "" && cfg.SeedVHDX == "":
		return fmt.Errorf(
			"runBuild: no seed media " +
				"(need SeedISO for Linux or SeedVHDX for Windows)")
	case (cfg.BuildSwitch == "") == (len(cfg.Slots) == 0):
		return fmt.Errorf("runBuild: want exactly one of a build switch and slots")
	case cfg.NATSwitch == "":
		return fmt.Errorf("runBuild: empty NAT switch")
	case !cfg.NATPrefix.IsValid() || !cfg.HostIP.IsValid():
		return fmt.Errorf("runBuild: no NAT prefix and host IP")
	case cfg.RelaySeedDir == "":
		return fmt.Errorf("runBuild: empty relay seed dir")
	}
	// A differencing child must share its parent's on-disk format, so the build
	// overlay's extension must match the base image's (.vhd child off a .vhd
	// base, .vhdx off .vhdx). A mismatch fails opaquely inside New-VHD.
	if !strings.EqualFold(filepath.Ext(cfg.BuildBaseVHDX), filepath.Ext(cfg.BuildOverlay)) {
		return fmt.Errorf(
			"runBuild: build overlay %q must share the base image's extension %q "+
				"(a differencing child must match its parent's format)",
			cfg.BuildOverlay, filepath.Ext(cfg.BuildBaseVHDX))
	}
	return nil
}

// applyBuildDefaults fills unset buildConfig fields with their defaults.
func applyBuildDefaults(cfg *buildConfig) {
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = defaultBuildMemoryMB
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = defaultBuildCPUs
	}
	if cfg.Generation == 0 {
		cfg.Generation = 2
	}
}

// runBuild composes the full Hyper-V build flow against an already-resolved
// buildConfig, as the driver's side of warden's build sequence:
//
//  1. StartRelay boots the relay VM with warden's claim token and reports it
//     through host, returning once warden has claimed it.
//  2. A per-build differencing overlay is created off the read-only build-guest
//     base image, so the base is never mutated and concurrent builds don't
//     collide.
//  3. The build VM is created on the Private build switch (its sole NIC) with
//     the overlay + seed media, then started, and reported started.
//  4. It waits for warden's stop, reporting the build VM's power-off as
//     exited meanwhile. The collector and the build's completion are
//     warden's.
//
// Everything created is torn down on return (both VMs removed, both overlays
// deleted), success or failure, unless KeepVMs keeps it, which runBuild then
// returns as kept.
func runBuild(
	ctx context.Context, prov Provisioner, host driver.Host, cfg buildConfig,
) (kept []string, err error) {
	if err := validateBuildConfig(cfg); err != nil {
		return nil, err
	}
	applyBuildDefaults(&cfg)

	// 1. Relay VM booted, reported and claimed. StartRelay tears its own half
	//    down on failure.
	rh, err := StartRelay(ctx, prov, host, RelayHostConfig{
		BuildID:     cfg.BuildID,
		Token:       cfg.Token,
		BootVHDX:    cfg.RelayBootVHDX,
		OverlayVHDX: cfg.RelayOverlay,
		NATSwitch:   cfg.NATSwitch,
		NATPrefix:   cfg.NATPrefix,
		HostIP:      cfg.HostIP,
		BuildSwitch: cfg.BuildSwitch,
		Slots:       cfg.Slots,
		SeedDir:     cfg.RelaySeedDir,
		MakeSeedISO: cfg.makeSeedISO,
		GuestOS:     guestOS(cfg.IsWindows),
		Request:     cfg.Request,
		Environment: cfg.Environment,
		KeepVM:      cfg.KeepVMs,
		ClaimPort:   cfg.claimPort,
	})
	if err != nil {
		return nil, fmt.Errorf("runBuild: %w", err)
	}
	// rh.Close removes the relay VM + relay overlay (unless --keep-vms).
	defer func() { kept = append(kept, rh.Close()...) }()

	// 2. Per-build differencing overlay off the read-only build-guest base.
	if err := prov.CreateDiffDisk(ctx, cfg.BuildOverlay, cfg.BuildBaseVHDX); err != nil {
		return nil, fmt.Errorf("runBuild: build overlay: %w", err)
	}

	buildVM := "warden-build-" + cfg.BuildID
	// Tear the build VM + its overlay down regardless of outcome. Uses a
	// background context so cleanup still runs when ctx is already cancelled.
	// --keep-vms skips teardown so a failed build VM can be inspected
	// (console, logs); the caller cleans it up manually.
	defer func() {
		if cfg.KeepVMs {
			fmt.Fprintf(os.Stderr,
				"warden: --keep-vms: leaving build VM %q "+
					"(overlay %q) for inspection\n",
				buildVM, cfg.BuildOverlay)
			kept = append(kept, "build VM "+buildVM, cfg.BuildOverlay)
			return
		}
		_ = prov.RemoveVM(context.Background(), buildVM)
		_ = os.Remove(cfg.BuildOverlay)
	}()

	// 3. Create + start the build VM on the relay's Private build switch (its
	//    SOLE NIC; no NAT route). Generation follows the base image: a Gen1 (.vhd) guest
	//    such as the Windows Server eval image has no Secure Boot; a Gen2 (.vhdx)
	//    guest keeps Secure Boot on via the per-family template CreateVM selects.
	//    Either way the build guest is isolated on the Private switch.
	//
	//    COM1 is wired to a host named pipe so the guest bootstrap's progress is
	//    observable from the host without mounting the guest disk. The default
	//    pipe name is derived from the build VM name; WARDEN_HYPERV_BUILD_COM_PIPE
	//    overrides it for a deterministic path a reader can attach to in advance.
	comPipe := buildCOMPipe(buildVM)
	fmt.Fprintf(os.Stderr,
		"warden: build VM %q COM1 -> %s "+
			"(attach a serial reader to observe guest boot)\n",
		buildVM, comPipe)
	if _, err := prov.CreateVM(ctx, VMSpec{
		Name:        buildVM,
		Generation:  cfg.Generation,
		MemoryMB:    cfg.MemoryMB,
		CPUs:        cfg.CPUs,
		SwitchName:  rh.BuildSwitch,
		VHDXPath:    cfg.BuildOverlay,
		SeedISO:     cfg.SeedISO,
		SeedVHDX:    cfg.SeedVHDX,
		IsWindows:   cfg.IsWindows,
		COMPipePath: comPipe,
	}); err != nil {
		return nil, fmt.Errorf("runBuild: create build VM: %w", err)
	}
	if err := verifySlot(ctx, prov, cfg, rh, buildVM); err != nil {
		return nil, fmt.Errorf("runBuild: %w", err)
	}
	if err := prov.StartVM(ctx, buildVM); err != nil {
		return nil, fmt.Errorf("runBuild: start build VM: %w", err)
	}
	if err := host.Started(); err != nil {
		return nil, err
	}

	// 4. A guest that powers off (or crashes) without reporting completion
	//    would otherwise leave the build waiting until its heartbeat timeout
	//    or forever: heartbeat silence is only judged once heartbeats have
	//    started. The watch reports the VM's power-off as exited, and warden
	//    gives a completion on its way its grace.
	watchCtx, stopWatch := context.WithCancel(ctx)
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		watchBuildVM(watchCtx, prov, buildVM, host, cfg.vmPoll, fmt.Sprintf(
			"build VM %q powered off without reporting an exit code; see %s for "+
				"the last thing the guest printed", buildVM,
			filepath.Join(cfg.OutputDir, "build-output.log")))
	}()
	defer func() {
		stopWatch()
		<-watched
	}()
	host.Wait(ctx)
	return nil, nil
}

// buildCOMPipe is the build VM's COM1 pipe: WARDEN_HYPERV_BUILD_COM_PIPE, or
// one named after the VM.
func buildCOMPipe(buildVM string) string {
	if p := os.Getenv("WARDEN_HYPERV_BUILD_COM_PIPE"); p != "" {
		return p
	}
	return `\\.\pipe\` + buildVM
}

// verifySlot re-reads a durable build's slot before its build VM runs: the
// slot is shared host infrastructure, so its segment must hold this build's
// two VMs and nothing else (BOOT-01: the relay is the build VM's sole peer).
// An ephemeral build's switch is its own and is not re-read.
func verifySlot(ctx context.Context, prov Provisioner, cfg buildConfig, rh *RelayHost,
	buildVM string) error {
	if len(cfg.Slots) == 0 {
		return nil
	}
	attached, err := prov.SwitchAdapters(ctx, []string{rh.BuildSwitch})
	if err != nil {
		return err
	}
	return checkSlotAdapters(rh.BuildSwitch, attached[rh.BuildSwitch], rh.VMName, buildVM)
}

// checkSlotAdapters refuses a durable build switch whose attached adapters
// are not exactly the relay VM's build NIC and the build VM's NIC.
func checkSlotAdapters(sw string, attached []string, relayVM, buildVM string) error {
	want := map[string]int{relayVM: 1, buildVM: 1}
	var foreign []string
	for _, vm := range attached {
		if want[vm] > 0 {
			want[vm]--
			continue
		}
		foreign = append(foreign, vm)
	}
	var missing []string
	for _, vm := range []string{relayVM, buildVM} {
		if want[vm] > 0 {
			missing = append(missing, vm)
		}
	}
	if len(foreign) == 0 && len(missing) == 0 {
		return nil
	}
	return driver.Fixable(
		"disconnect the other adapters from "+sw+" (Get-VMNetworkAdapter -All | "+
			"Where-Object SwitchName -eq '"+sw+"'), then re-run the build",
		"durable build switch %s must hold only this build's VMs (%s, %s); "+
			"also attached: %s; missing: %s", sw, relayVM, buildVM,
		joinOrNone(foreign), joinOrNone(missing))
}

// joinOrNone is names comma-joined, or "none".
func joinOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// watchBuildVM polls the build VM's state and reports it through host as
// exited, with detail, once the VM is off (or gone), until ctx ends.
func watchBuildVM(
	ctx context.Context, prov Provisioner, vm string, host driver.Host,
	poll time.Duration, detail string,
) {
	if poll <= 0 {
		poll = 3 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
		state, err := prov.VMState(ctx, vm)
		if err != nil || (state != "Off" && state != "") {
			continue
		}
		host.Exited(driver.ExitedBuild, detail)
		return
	}
}

// guestOS names the build guest's OS family for the bootstrap config.
func guestOS(windows bool) string {
	if windows {
		return "windows"
	}
	return "linux"
}

package hyperv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// Build VM defaults. The build VM is the untrusted guest: its sole NIC is on the
// Private build switch, so its only peer is the relay at 10.0.0.1 and it has no
// route to the host network stack or the internet.
const (
	defaultBuildMemoryMB = 4096
	defaultBuildCPUs     = 4
)

// buildConfig is the fully-resolved input to runBuild: every asset path is
// already located and the network already chosen. runBuild is pure
// orchestration over the Provisioner + StartRelay, so it is unit-testable with a
// fake Provisioner and never touches asset resolution, image acquisition, or
// seed generation itself.
type buildConfig struct {
	BuildID   string
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

	// Network. Both VMs share the Private build switch (relay <-> build only);
	// the relay additionally attaches the NAT switch for controlled egress. The
	// build VM never attaches to the NAT switch.
	BuildSwitch string
	NATSwitch   string

	MemoryMB int           // build VM; default 4096
	CPUs     int           // build VM; default 4
	Timeout  time.Duration // whole-build ceiling; zero means no limit

	// Host-binding overrides for the relay host servers, mirrored onto
	// RelayHostConfig. Empty/zero use the production defaults (NAT gateway
	// 192.168.240.1, ports 8299/8390). Tests set these to loopback + high ports.
	natHostIP     string
	configPort    int
	collectorPort int
	readyTimeout  time.Duration
}

// runBuild composes the full Hyper-V build flow against an already-resolved
// buildConfig:
//
//  1. StartRelay stands up the collector + config responder on the host and
//     boots the relay VM, blocking until the relay signals ready.
//  2. A per-build differencing overlay is created off the read-only build-guest
//     base image, so the base is never mutated and concurrent builds don't
//     collide.
//  3. The build VM is created on the Private build switch (its sole NIC) with
//     the overlay + seed media, then started.
//  4. It blocks until the relay POSTs /v1/complete (warden-io in the guest
//     reported the build finished) or the timeout/context fires. Outputs are
//     streamed to OutputDir by the collector as the build runs, so on success
//     there is nothing left to copy.
//
// Everything created is torn down on return (both VMs removed, both overlays
// deleted, host servers stopped), success or failure.
func runBuild(ctx context.Context, prov Provisioner, cfg buildConfig) (*driver.BuildResult, error) {
	switch {
	case cfg.BuildID == "":
		return nil, fmt.Errorf("runBuild: empty build ID")
	case cfg.OutputDir == "":
		return nil, fmt.Errorf("runBuild: empty output dir")
	case cfg.RelayBootVHDX == "":
		return nil, fmt.Errorf("runBuild: empty relay boot VHDX")
	case cfg.RelayOverlay == "":
		return nil, fmt.Errorf("runBuild: empty relay overlay path")
	case cfg.BuildBaseVHDX == "":
		return nil, fmt.Errorf("runBuild: empty build base VHDX")
	case cfg.BuildOverlay == "":
		return nil, fmt.Errorf("runBuild: empty build overlay path")
	case cfg.SeedISO == "" && cfg.SeedVHDX == "":
		return nil, fmt.Errorf("runBuild: no seed media (need SeedISO for Linux or SeedVHDX for Windows)")
	case cfg.BuildSwitch == "":
		return nil, fmt.Errorf("runBuild: empty build switch")
	case cfg.NATSwitch == "":
		return nil, fmt.Errorf("runBuild: empty NAT switch")
	}
	// A differencing child must share its parent's on-disk format, so the build
	// overlay's extension must match the base image's (.vhd child off a .vhd
	// base, .vhdx off .vhdx). A mismatch fails opaquely inside New-VHD.
	if !strings.EqualFold(filepath.Ext(cfg.BuildBaseVHDX), filepath.Ext(cfg.BuildOverlay)) {
		return nil, fmt.Errorf(
			"runBuild: build overlay %q must share the base image's extension %q "+
				"(a differencing child must match its parent's format)",
			cfg.BuildOverlay, filepath.Ext(cfg.BuildBaseVHDX))
	}
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = defaultBuildMemoryMB
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = defaultBuildCPUs
	}
	if cfg.Generation == 0 {
		cfg.Generation = 2
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("runBuild: output dir: %w", err)
	}

	// 1. Relay host up: collector + config responder on the host, relay VM
	//    booted and ready. StartRelay tears its own half down on failure.
	rh, err := StartRelay(ctx, prov, RelayHostConfig{
		BuildID:       cfg.BuildID,
		OutputDir:     cfg.OutputDir,
		BootVHDX:      cfg.RelayBootVHDX,
		OverlayVHDX:   cfg.RelayOverlay,
		BuildSwitch:   cfg.BuildSwitch,
		NATSwitch:     cfg.NATSwitch,
		NATHostIP:     cfg.natHostIP,
		ConfigPort:    cfg.configPort,
		CollectorPort: cfg.collectorPort,
		ReadyTimeout:  cfg.readyTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("runBuild: %w", err)
	}
	// rh.Close removes the relay VM + relay overlay and stops the host servers.
	defer func() { _ = rh.Close() }()

	// 2. Per-build differencing overlay off the read-only build-guest base.
	if err := prov.CreateDiffDisk(ctx, cfg.BuildOverlay, cfg.BuildBaseVHDX); err != nil {
		return nil, fmt.Errorf("runBuild: build overlay: %w", err)
	}

	buildVM := "warden-build-" + cfg.BuildID
	// Tear the build VM + its overlay down regardless of outcome. Uses a
	// background context so cleanup still runs when ctx is already cancelled.
	defer func() {
		_ = prov.RemoveVM(context.Background(), buildVM)
		_ = os.Remove(cfg.BuildOverlay)
	}()

	// 3. Create + start the build VM on the Private build switch (its SOLE NIC;
	//    no NAT route). Generation follows the base image: a Gen1 (.vhd) guest
	//    such as the Windows Server eval image has no Secure Boot; a Gen2 (.vhdx)
	//    guest keeps Secure Boot on via the per-family template CreateVM selects.
	//    Either way the build guest is isolated on the Private switch.
	if _, err := prov.CreateVM(ctx, VMSpec{
		Name:       buildVM,
		Generation: cfg.Generation,
		MemoryMB:   cfg.MemoryMB,
		CPUs:       cfg.CPUs,
		SwitchName: cfg.BuildSwitch,
		VHDXPath:   cfg.BuildOverlay,
		SeedISO:    cfg.SeedISO,
		SeedVHDX:   cfg.SeedVHDX,
		IsWindows:  cfg.IsWindows,
	}); err != nil {
		return nil, fmt.Errorf("runBuild: create build VM: %w", err)
	}
	if err := prov.StartVM(ctx, buildVM); err != nil {
		return nil, fmt.Errorf("runBuild: start build VM: %w", err)
	}

	// 4. Wait for the build to finish. The relay POSTs /v1/complete to the
	//    collector when warden-io in the guest reports completion; the collector
	//    delivers that (with the exit code) on Done. Artifacts/ledger/output are
	//    streamed to OutputDir as the build runs.
	var timeoutCh <-chan time.Time
	if cfg.Timeout > 0 {
		t := time.NewTimer(cfg.Timeout)
		defer t.Stop()
		timeoutCh = t.C
	}
	select {
	case sig := <-rh.Collector.Done():
		if sig.ExitCode != 0 {
			msg := sig.Message
			if sig.Error != "" {
				msg = sig.Error
			}
			return nil, fmt.Errorf("runBuild: build exited with code %d: %s", sig.ExitCode, msg)
		}
		return &driver.BuildResult{OutputDir: cfg.OutputDir}, nil
	case <-timeoutCh:
		return nil, fmt.Errorf("runBuild: build %q did not complete within %s", buildVM, cfg.Timeout)
	case <-ctx.Done():
		return nil, fmt.Errorf("runBuild: %w", ctx.Err())
	}
}

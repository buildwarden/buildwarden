//go:build windows

package hyperv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// StartBuild runs a build on Hyper-V: it verifies the isolated network, brings
// up the relay host (collector + claimed relay VM) and the build VM,
// waits for completion, and tears everything down. It composes the resolved
// assets and delegates the orchestration to runBuild.
//
// It reuses a durable dev switch (from `warden setup --driver hyperv`) so the whole
// per-build path stays within the Hyper-V Administrators tier (no elevation).
func (d *Driver) StartBuild(
	ctx context.Context, req *driver.BuildRequest,
) (*driver.BuildResult, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperv build: nil request")
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	sw := req.Network
	if sw == "" {
		sw = DefaultDevSwitch
	}

	// Privilege and network readiness first: a missing group membership or
	// elevation is the most common blocker, and it costs nothing to check
	// before any disk or seed work. Detect reflects THIS process's token.
	caps := Detect(sw)
	if ready, reason := caps.Ready(); !ready {
		return nil, driver.Fixable(
			"see `warden doctor --driver hyperv` for each capability and its fix",
			"hyperv build: %s", reason)
	}

	buildID := driver.RandAlphaNum(8)

	relayBoot, chain, isWindows, generation, err := resolveBuildDisks(req)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}
	base := chain[len(chain)-1]

	workDir, err := driver.NewWorkDir(req, "hyperv-"+buildID)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}
	defer driver.RemoveWorkDir(req, workDir)
	if err := checkDirVisible(workDir, "the build's work directory",
		"pass --tmpdir a folder outside AppData"); err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}

	seedISO, seedVHDX, err := buildSeed(workDir, buildID, isWindows, req)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}

	// Same build-script precedence as every VM driver: --script, then
	// build.ps1/build.sh in the context, then a no-op.
	buildScript, err := driver.StageBuildScript(req, workDir)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}
	env, err := describeBuild(LayerCache(), chain, isWindows, generation,
		buildSeedDir(workDir), buildScript)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: describing build environment: %w", err)
	}

	p := newLocalProvisioner(d.Verbose)

	// Network provisioning per the capability model (see capability.go),
	// using the capabilities detected above: the gateway's non-elevated
	// warden lands on reuse-durable or delegate-to-service; an elevated
	// shell lands on ephemeral-per-build.
	p, buildSwitch, natSwitch, cleanupNet, err := d.provisionBuildNetwork(
		ctx, p, caps, sw, buildID, req)
	if err != nil {
		return nil, err
	}
	defer cleanupNet()

	res, err := runBuild(ctx, p, buildConfig{
		BuildID:       buildID,
		OutputDir:     req.OutputDir,
		RelayBootVHDX: relayBoot,
		RelayOverlay:  filepath.Join(workDir, "relay.vhdx"),
		BuildBaseVHDX: base,
		// The differencing overlay must share the base disk's format (.vhd off a
		// Gen1 eval VHD, .vhdx off a Gen2 image).
		BuildOverlay:    filepath.Join(workDir, "build"+filepath.Ext(base)),
		SeedISO:         seedISO,
		SeedVHDX:        seedVHDX,
		IsWindows:       isWindows,
		Generation:      generation,
		BuildScriptPath: buildScript,
		BuildSwitch:     buildSwitch,
		NATSwitch:       natSwitch,
		Timeout:         req.Timeout,
		MemoryMB:        req.MemoryMB,
		CPUs:            req.CPUs,
		KeepVMs:         req.KeepVMs,
		OutputTee:       driver.ConsoleTee(req),
		Request:         req,
		ContextDir:      req.ContextDir,
		Environment:     env,
	})
	if err != nil {
		return nil, err
	}
	driver.FinalizeOutput(req.OutputDir, req.Compress)
	if err := driver.RecordInputs(req.OutputDir, req, buildScript); err != nil {
		return nil, fmt.Errorf("hyperv build: recording inputs: %w", err)
	}
	return res, nil
}

// provisionBuildNetwork resolves the build and NAT switches per the capability
// model, standing up a fresh isolated network when the privilege allows it. It
// returns the (possibly updated) provisioner, the chosen switch names, and a
// cleanup to run on build teardown.
// resolveBuildDisks resolves the relay boot disk and the build guest's disk
// chain, checks Hyper-V can see them, and keeps an active prepared base fresh.
func resolveBuildDisks(req *driver.BuildRequest) (
	relayBoot string, chain []string, isWindows bool, generation int, err error,
) {
	if relayBoot, err = resolveRelayBootVHDX(); err != nil {
		return "", nil, false, 0, err
	}
	if chain, isWindows, generation, err = resolveBuildBaseVHDX(req); err != nil {
		return "", nil, false, 0, err
	}
	err = checkFilesVisible(cacheVisibleFix, append([]string{relayBoot}, chain...)...)
	if err != nil {
		return "", nil, false, 0, err
	}
	if len(chain) > 1 { // the active prepared base, not an explicit --image
		if err := ensureFreshBase(req, os.Stderr); err != nil {
			return "", nil, false, 0, err
		}
	}
	return relayBoot, chain, isWindows, generation, nil
}

func (d *Driver) provisionBuildNetwork(
	ctx context.Context, p Provisioner, caps Capabilities,
	sw, buildID string, req *driver.BuildRequest,
) (prov Provisioner, buildSwitch, natSwitch string, cleanup func(), err error) {
	prov = p
	cleanup = func() {}
	switch strat := caps.NetworkStrategy(); strat {
	case NetReuseDurable:
		// Non-elevated Hyper-V Administrators: reuse the durable switch from
		// `warden setup --driver hyperv`. Re-assert isolation every build rather
		// than trusting persistence (a crash or manual edit can weaken it).
		if v, ok := prov.(interface {
			VerifyNetwork(context.Context, string) error
		}); ok {
			if verr := v.VerifyNetwork(ctx, sw); verr != nil {
				return nil, "", "", nil, fmt.Errorf("hyperv build: %w", verr)
			}
		}
		buildSwitch = sw
		natSwitch = sw + "-nat"

	case NetEphemeralPerBuild, NetDelegateService:
		// Stand up a FRESH isolated network for this build and tear it down
		// after. Elevated -> in-process; non-elevated with the service reachable
		// -> delegate ONLY the standup to the privileged service (VM ops still
		// run in-process via the embedded localProvisioner).
		if strat == NetDelegateService {
			prov = newClientProvisioner(d.Verbose)
		}
		buildSwitch = "warden-" + buildID
		res, nerr := prov.EnsureNetwork(ctx, NetworkSpec{
			ID:         buildID,
			SwitchName: buildSwitch,
			SwitchType: "Private",
			NATPrefix:  DefaultNATPrefix,
			HostIP:     DefaultHostIP,
		})
		if nerr != nil {
			// A standup that failed partway can leave the Private build switch
			// (created first) behind; best-effort clean it so a retry is not
			// blocked by orphaned switches.
			_ = prov.TeardownNetwork(context.Background(), buildSwitch)
			return nil, "", "", nil, fmt.Errorf(
				"hyperv build: network standup: %w", nerr)
		}
		// The relay's upstream NIC attaches to whichever NAT switch EnsureNetwork
		// chose: a reused durable/concurrent NAT, or a per-build one it created.
		natSwitch = res.NATName
		bs, bp := buildSwitch, prov
		cleanup = func() {
			if req.KeepVMs {
				fmt.Fprintf(os.Stderr,
					"warden: --keep-vms: leaving ephemeral network %q\n", bs)
				return
			}
			// Background ctx so teardown runs even when ctx is already cancelled.
			// Teardown is by per-build name convention, so a reused NAT (a
			// different name) is never removed - only the per-build build switch
			// and, when created, the per-build NAT.
			_ = bp.TeardownNetwork(context.Background(), bs)
		}

	default: // NetBlocked
		_, reason := caps.Ready()
		return nil, "", "", nil, fmt.Errorf("hyperv build: %s", reason)
	}
	return prov, buildSwitch, natSwitch, cleanup, nil
}

// Exec is not supported by the Hyper-V driver.
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

// Close releases resources held by the driver. Nothing to release yet.
func (d *Driver) Close() error { return nil }

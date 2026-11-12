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
	if err := caps.buildRefusal(); err != nil {
		return nil, err
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

	// Same build-script precedence as every VM driver: --script, then (Linux
	// guests) the translated Dockerfile, then build.ps1/build.sh in the
	// context, then a no-op.
	buildScript, err := driver.StageBuildScript(req, workDir)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}
	if err := driver.RecordInputs(req.OutputDir, req, buildScript); err != nil {
		return nil, fmt.Errorf("hyperv build: recording inputs: %w", err)
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
	p, bnet, cleanupNet, err := d.provisionBuildNetwork(ctx, p, caps, sw, buildID, req)
	if err != nil {
		return nil, err
	}
	defer cleanupNet()
	if err := prepareBuildNetwork(req, bnet.NATPrefix, defaultRouteDNS); err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}

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
		BuildSwitch:     bnet.BuildSwitch,
		Slots:           bnet.Slots,
		NATSwitch:       bnet.NAT,
		NATPrefix:       bnet.NATPrefix,
		HostIP:          bnet.HostIP,
		RelaySeedDir:    filepath.Join(workDir, "relay-seed"),
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
	return res, nil
}

// resolveBuildDisks resolves the relay boot disk and the build guest's disk
// chain, checks Hyper-V can see them, and keeps an active prepared base fresh.
func resolveBuildDisks(req *driver.BuildRequest) (
	relayBoot string, chain []string, isWindows bool, generation int, err error,
) {
	if relayBoot, err = resolveRelayBootVHDX(); err != nil {
		return "", nil, false, 0, err
	}
	st := relayBootStamp()
	if err = st.Check(); err != nil {
		return "", nil, false, 0, err
	}
	if err = driver.CheckHostLink(req.Link()); err != nil {
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

// provisionBuildNetwork resolves the build's network per the capability
// model: the durable strategy verifies the network setup made and reads its
// NAT and slot switches back, naming no build switch (the allocator picks a
// slot); the ephemeral strategies stand up a per-build Private switch and take
// the NAT EnsureNetwork chose, the dev switch's when setup made one. It
// returns the (possibly updated) provisioner, the network, and a cleanup to
// run on build teardown.
func (d *Driver) provisionBuildNetwork(
	ctx context.Context, p Provisioner, caps Capabilities,
	sw, buildID string, req *driver.BuildRequest,
) (prov Provisioner, bnet buildNetwork, cleanup func(), err error) {
	prov = p
	cleanup = func() {}
	switch strat := caps.NetworkStrategy(); strat {
	case NetReuseDurable:
		// Non-elevated Hyper-V Administrators: the durable network from
		// `warden setup --driver hyperv`. Re-assert isolation every build rather
		// than trusting persistence (a crash or manual edit can weaken it).
		v, ok := prov.(interface {
			VerifyNetwork(context.Context, string) (*NetworkResources, error)
		})
		if !ok {
			return nil, buildNetwork{}, nil, fmt.Errorf(
				"hyperv build: the provisioner cannot verify the durable network")
		}
		res, verr := v.VerifyNetwork(ctx, sw)
		if verr != nil {
			return nil, buildNetwork{}, nil, fmt.Errorf("hyperv build: %w", verr)
		}
		bnet = buildNetwork{NAT: res.NATName, NATPrefix: res.NATPrefix,
			HostIP: res.HostIP, Slots: res.Slots}

	case NetEphemeralPerBuild, NetDelegateService:
		// Stand up a FRESH isolated network for this build and tear it down
		// after. Elevated -> in-process; non-elevated with the service reachable
		// -> delegate ONLY the standup to the privileged service (VM ops still
		// run in-process via the embedded localProvisioner).
		if strat == NetDelegateService {
			prov = newClientProvisioner(d.Verbose)
		}
		buildSwitch := "warden-" + buildID
		res, nerr := prov.EnsureNetwork(ctx, NetworkSpec{
			ID:         buildID,
			SwitchName: buildSwitch,
			SwitchType: "Private",
			NATPrefix:  DefaultNATPrefix,
			HostIP:     DefaultHostIP,
			DevNAT:     sw + "-nat",
		})
		if nerr != nil {
			// A standup that failed partway can leave the Private build switch
			// (created first) behind; best-effort clean it so a retry is not
			// blocked by orphaned switches.
			_ = prov.TeardownNetwork(context.Background(), buildSwitch)
			return nil, buildNetwork{}, nil, fmt.Errorf(
				"hyperv build: network standup: %w", nerr)
		}
		bnet = buildNetwork{NAT: res.NATName, NATPrefix: res.NATPrefix,
			HostIP: res.HostIP, BuildSwitch: buildSwitch}
		bp := prov
		cleanup = func() {
			if req.KeepVMs {
				fmt.Fprintf(os.Stderr, "warden: --keep-vms: leaving ephemeral "+
					"network %q\n", buildSwitch)
				return
			}
			// Background ctx so teardown runs even when ctx is already cancelled.
			// Teardown is by per-build name convention, so a reused NAT (a
			// different name) is never removed - only the per-build build switch
			// and, when created, the per-build NAT.
			_ = bp.TeardownNetwork(context.Background(), buildSwitch)
		}

	default: // NetBlocked, which Ready never passes
		return nil, buildNetwork{}, nil, caps.buildRefusal()
	}
	return prov, bnet, cleanup, nil
}

// Exec is not supported by the Hyper-V driver.
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

// Close releases resources held by the driver. Nothing to release yet.
func (d *Driver) Close() error { return nil }

//go:build windows

package hyperv

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// StartBuild runs a build on Hyper-V: it verifies the isolated network, brings
// up the relay host (collector + config responder + relay VM) and the build VM,
// waits for completion, and tears everything down. It composes the resolved
// assets and delegates the orchestration to runBuild.
//
// It reuses a durable dev switch (from `warden hyperv setup`) so the whole
// per-build path stays within the Hyper-V Administrators tier (no elevation).
func (d *Driver) StartBuild(ctx context.Context, req *driver.BuildRequest) (*driver.BuildResult, error) {
	if req == nil {
		return nil, fmt.Errorf("hyperv build: nil request")
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	sw := d.Switch
	if sw == "" {
		sw = DefaultDevSwitch
	}

	buildID := driver.RandAlphaNum(8)

	relayBoot, err := resolveRelayBootVHDX()
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}
	base, isWindows, generation, err := resolveBuildBaseVHDX(req)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}

	workDir, err := os.MkdirTemp("", "warden-hyperv-"+buildID+"-")
	if err != nil {
		return nil, fmt.Errorf("hyperv build: work dir: %w", err)
	}
	defer func() {
		if os.Getenv("WARDEN_HYPERV_KEEP_VMS") == "1" {
			fmt.Fprintf(os.Stderr, "warden: WARDEN_HYPERV_KEEP_VMS set; leaving work dir %q\n", workDir)
			return
		}
		os.RemoveAll(workDir)
	}()

	seedISO, seedVHDX, err := buildSeed(workDir, buildID, isWindows, req)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}

	// The build script is served to the guest via the collector's
	// /v1/build-script endpoint. Resolution: explicit --script, else the script
	// in the build context (build.ps1 for Windows, build.sh otherwise -- what
	// resolveWindowsContext requires and the qemu/vz drivers use), else a no-op
	// default so the pipeline still runs end to end.
	buildScript := req.Script
	if buildScript == "" {
		name := "build.sh"
		if isWindows {
			name = "build.ps1"
		}
		if cand := filepath.Join(req.ContextDir, name); req.ContextDir != "" {
			if _, statErr := os.Stat(cand); statErr == nil {
				buildScript = cand
			}
		}
	}
	if buildScript == "" {
		def := filepath.Join(workDir, "build-default")
		content := "exit 0\r\n"
		if !isWindows {
			content = "#!/bin/sh\ntrue\n"
		}
		if err := os.WriteFile(def, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("hyperv build: default script: %w", err)
		}
		buildScript = def
	}

	p := newLocalProvisioner(d.Verbose)

	// Resolve how this build's isolated network is provisioned, per the
	// capability model (see capability.go). Detect reflects THIS process's
	// token, so the gateway's non-elevated warden lands on reuse-durable or
	// delegate-to-service; an elevated shell lands on ephemeral-per-build.
	caps := Detect(sw)
	var buildSwitch, natSwitch string
	switch strat := caps.NetworkStrategy(); strat {
	case NetReuseDurable:
		// Non-elevated Hyper-V Administrators: reuse the durable switch from
		// `warden hyperv setup`. Re-assert isolation every build rather than
		// trusting persistence (a crash or manual edit can weaken it).
		if v, ok := p.(interface {
			VerifyNetwork(context.Context, string) error
		}); ok {
			if err := v.VerifyNetwork(ctx, sw); err != nil {
				return nil, fmt.Errorf("hyperv build: %w", err)
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
			p = newClientProvisioner(d.Verbose)
		}
		buildSwitch = "warden-" + buildID
		res, err := p.EnsureNetwork(ctx, NetworkSpec{
			ID:         buildID,
			SwitchName: buildSwitch,
			SwitchType: "Private",
			NATPrefix:  DefaultNATPrefix,
			HostIP:     DefaultHostIP,
		})
		if err != nil {
			// A standup that failed partway can leave the Private build switch
			// (created first) behind; best-effort clean it so a retry is not
			// blocked by orphaned switches.
			_ = p.TeardownNetwork(context.Background(), buildSwitch)
			return nil, fmt.Errorf("hyperv build: network standup: %w", err)
		}
		// The relay's upstream NIC attaches to whichever NAT switch EnsureNetwork
		// chose: a reused durable/concurrent NAT, or a per-build one it created.
		natSwitch = res.NATName
		defer func() {
			if os.Getenv("WARDEN_HYPERV_KEEP_VMS") == "1" {
				fmt.Fprintf(os.Stderr, "warden: WARDEN_HYPERV_KEEP_VMS set; leaving ephemeral network %q\n", buildSwitch)
				return
			}
			// Background ctx so teardown runs even when ctx is already cancelled.
			// Teardown is by per-build name convention, so a reused NAT (a
			// different name) is never removed - only the per-build build switch
			// and, when created, the per-build NAT.
			_ = p.TeardownNetwork(context.Background(), buildSwitch)
		}()

	default: // NetBlocked
		_, reason := caps.Ready()
		return nil, fmt.Errorf("hyperv build: %s", reason)
	}

	// Forward the guest build console to the user unless --quiet. The CLI sets
	// req.Stdout to os.Stdout; fall back to it when a caller leaves it nil.
	var outTee io.Writer
	if !req.Quiet {
		outTee = req.Stdout
		if outTee == nil {
			outTee = os.Stdout
		}
	}

	return runBuild(ctx, p, buildConfig{
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
		OutputTee:       outTee,
	})
}

// Exec is not supported by the Hyper-V driver.
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

// Close releases resources held by the driver. Nothing to release yet.
func (d *Driver) Close() error { return nil }

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
	defer os.RemoveAll(workDir)

	seedISO, seedVHDX, err := buildSeed(workDir, buildID, isWindows, req)
	if err != nil {
		return nil, fmt.Errorf("hyperv build: %w", err)
	}

	p := newLocalProvisioner(d.Verbose)

	// Re-assert isolation every build rather than trusting persistence: a crash
	// or manual edit can silently weaken the build switch. VerifyNetwork is a
	// Windows-only Provisioner method, reached via interface assertion.
	if v, ok := p.(interface {
		VerifyNetwork(context.Context, string) error
	}); ok {
		if err := v.VerifyNetwork(ctx, sw); err != nil {
			return nil, fmt.Errorf("hyperv build: %w", err)
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
		BuildOverlay: filepath.Join(workDir, "build"+filepath.Ext(base)),
		SeedISO:      seedISO,
		SeedVHDX:     seedVHDX,
		IsWindows:    isWindows,
		Generation:   generation,
		BuildSwitch:  sw,
		NATSwitch:    sw + "-nat",
		Timeout:      req.Timeout,
	})
}

// Exec is not supported by the Hyper-V driver.
func (d *Driver) Exec(ctx context.Context, req *driver.BuildRequest) error {
	return driver.ErrExecNotSupported
}

// Close releases resources held by the driver. Nothing to release yet.
func (d *Driver) Close() error { return nil }

package hyperv

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/relaycfg"
)

// buildFake is a Provisioner that stands in for a real Hyper-V host AND
// simulates the relay VM's behaviour, so runBuild can be exercised end to end
// without any VM:
//
//   - When the relay VM "starts", it listens for the claim exactly as the real
//     relay does; on the claim it records the collector URL + token and POSTs
//     /v1/ready, which is what StartRelay blocks on.
//   - When the build VM "starts", it POSTs /v1/complete with a configurable exit
//     code, which is what runBuild blocks on.
//
// CreateDiffDisk creates a stub overlay file so overlay cleanup is observable.
type buildFake struct {
	claimAddr string
	buildExit int // exit code the simulated build reports on /v1/complete
	// powersOffSilently simulates a guest that powers off without ever
	// reporting completion (the build VM goes Off, nothing reaches /v1/complete).
	powersOffSilently bool

	claimed       atomic.Value // relaycfg.Config received by the simulated relay
	transport     atomic.Value // http.RoundTripper: the collector tunnel
	lastBuildSpec atomic.Value // VMSpec, captured from CreateVM (build VM)
	createRelay   atomic.Int32
	createBuild   atomic.Int32
	startRelay    atomic.Int32
	startBuild    atomic.Int32
	remove        atomic.Int32
	diffDisks     atomic.Int32
	relayReadyErr atomic.Value // error, if the simulated ready POST failed
}

func (f *buildFake) EnsureNetwork(context.Context, NetworkSpec) (*NetworkResources, error) {
	return &NetworkResources{}, nil
}
func (f *buildFake) TeardownNetwork(context.Context, string) error { return nil }

func (f *buildFake) CreateVM(_ context.Context, spec VMSpec) (*VMHandle, error) {
	f.createBuild.Add(1)
	f.lastBuildSpec.Store(spec)
	return &VMHandle{Name: spec.Name}, nil
}
func (f *buildFake) CreateRelayVM(context.Context, RelayVMSpec) (*VMHandle, error) {
	f.createRelay.Add(1)
	return &VMHandle{Name: "relay"}, nil
}

func (f *buildFake) StartVM(_ context.Context, name string) error {
	switch {
	case strings.HasPrefix(name, "warden-relay-"):
		f.startRelay.Add(1)
		f.simulateRelayReady()
	case strings.HasPrefix(name, "warden-build-"):
		f.startBuild.Add(1)
		if !f.powersOffSilently {
			f.simulateBuildComplete()
		}
	}
	return nil
}

func (f *buildFake) StopVM(context.Context, string) error { return nil }
func (f *buildFake) RemoveVM(context.Context, string) error {
	f.remove.Add(1)
	return nil
}

// CreateDiffDisk writes a stub file at the overlay path so the test can assert
// runBuild deletes it during teardown.
func (f *buildFake) CreateDiffDisk(_ context.Context, overlay, _ string) error {
	f.diffDisks.Add(1)
	return os.WriteFile(overlay, []byte("stub-overlay"), 0o644)
}
func (f *buildFake) VMState(context.Context, string) (string, error) {
	if f.powersOffSilently {
		return "Off", nil
	}
	return "Running", nil
}
func (f *buildFake) GuestKVP(context.Context, string) (map[string]string, error) {
	return nil, nil
}
func (f *buildFake) DisksInUse(context.Context) ([]string, error) { return nil, nil }
func (f *buildFake) ListVMs(context.Context, string) ([]string, error) {
	return nil, nil
}

// simulateRelayReady mirrors the relay: accept the tunnel and the claim, then
// POST /v1/ready to the collector over the tunnel.
func (f *buildFake) simulateRelayReady() {
	ln, err := net.Listen("tcp", f.claimAddr)
	if err != nil {
		f.relayReadyErr.Store(fmt.Errorf("claim listen: %w", err))
		return
	}
	go func() {
		_ = relaycfg.Serve(ln, "", func(
			c relaycfg.Config, rt http.RoundTripper,
		) (relaycfg.Identity, error) {
			f.claimed.Store(c)
			f.transport.Store(rt)
			if err := f.post("/v1/ready", nil); err != nil {
				f.relayReadyErr.Store(fmt.Errorf("ready POST: %w", err))
			}
			return relaycfg.Identity{
				CACert:          "-----BEGIN CERTIFICATE-----\n",
				LedgerPublicKey: "00",
			}, nil
		})
	}()
}

func (f *buildFake) post(path string, body io.Reader) error {
	c, _ := f.claimed.Load().(relaycfg.Config)
	rt, _ := f.transport.Load().(http.RoundTripper)
	req, _ := http.NewRequest(http.MethodPost, c.CollectorURL+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.CollectorToken)
	resp, err := (&http.Client{Transport: rt, Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

// simulateBuildComplete mirrors the relay reporting build completion.
func (f *buildFake) simulateBuildComplete() {
	if _, ok := f.claimed.Load().(relaycfg.Config); !ok {
		return
	}
	_ = f.post("/v1/complete",
		strings.NewReader(fmt.Sprintf(`{"exit_code":%d,"message":"done"}`, f.buildExit)))
}

// baseCfg returns a buildConfig wired to loopback + high test ports, with the
// relay overlay pre-created so teardown-removal is observable.
func baseCfg(t *testing.T, dir string) buildConfig {
	t.Helper()
	relayOverlay := filepath.Join(dir, "relay.vhdx")
	if err := os.WriteFile(relayOverlay, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	return buildConfig{
		BuildID:       "unit",
		OutputDir:     dir,
		RelayBootVHDX: "relay-boot.vhdx",
		RelayOverlay:  relayOverlay,
		BuildBaseVHDX: "build-base.vhdx",
		BuildOverlay:  filepath.Join(dir, "build.vhdx"),
		SeedISO:       filepath.Join(dir, "seed.iso"),
		BuildSwitch:   "warden-dev",
		NATSwitch:     "warden-dev-nat",
		relayIP:       "127.0.0.1",
		claimPort:     49301,
		readyTimeout:  5 * time.Second,
		Timeout:       5 * time.Second,
	}
}

func TestRunBuild_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	fp := &buildFake{claimAddr: "127.0.0.1:49301", buildExit: 0}

	res, err := runBuild(context.Background(), fp, cfg)
	if err != nil {
		if rerr, ok := fp.relayReadyErr.Load().(error); ok {
			t.Fatalf("runBuild failed: %v (relay-sim error: %v)", err, rerr)
		}
		t.Fatalf("runBuild failed: %v", err)
	}
	if res == nil || res.OutputDir != dir {
		t.Fatalf("BuildResult.OutputDir = %v, want %s", res, dir)
	}
	// Both VMs created + started, both overlays created.
	if got := fp.createRelay.Load(); got != 1 {
		t.Errorf("CreateRelayVM = %d, want 1", got)
	}
	if got := fp.createBuild.Load(); got != 1 {
		t.Errorf("CreateVM (build) = %d, want 1", got)
	}
	if spec, ok := fp.lastBuildSpec.Load().(VMSpec); !ok || spec.Generation != 2 {
		t.Errorf("build VM Generation = %v, want 2 (default for .vhdx base)",
			fp.lastBuildSpec.Load())
	}
	// COM1 must be wired to a host named pipe for guest-boot diagnostics.
	if spec, ok := fp.lastBuildSpec.Load().(VMSpec); ok {
		if !strings.HasPrefix(spec.COMPipePath, `\\.\pipe\warden-build-`) {
			t.Errorf("build VM COMPipePath = %q, want a "+
				"\\\\.\\pipe\\warden-build-* path", spec.COMPipePath)
		}
	}
	if got := fp.startBuild.Load(); got != 1 {
		t.Errorf("StartVM (build) = %d, want 1", got)
	}
	// Teardown: both VMs removed (build VM + relay VM), both overlays deleted.
	if got := fp.remove.Load(); got != 2 {
		t.Errorf("RemoveVM = %d, want 2 (build + relay)", got)
	}
	if _, err := os.Stat(cfg.BuildOverlay); !os.IsNotExist(err) {
		t.Error("build overlay should be removed on teardown")
	}
	if _, err := os.Stat(cfg.RelayOverlay); !os.IsNotExist(err) {
		t.Error("relay overlay should be removed on teardown")
	}
}

// TestRunBuild_Gen1 confirms a .vhd base drives a Generation-1 build VM (the
// Windows Server eval image path: BIOS/MBR, no Secure Boot, no conversion).
func TestRunBuild_Gen1(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	// Gen1 base + matching .vhd overlay.
	cfg.BuildBaseVHDX = "build-base.vhd"
	cfg.BuildOverlay = filepath.Join(dir, "build.vhd")
	cfg.Generation = 1
	cfg.IsWindows = true
	fp := &buildFake{claimAddr: "127.0.0.1:49301", buildExit: 0}

	if _, err := runBuild(context.Background(), fp, cfg); err != nil {
		if rerr, ok := fp.relayReadyErr.Load().(error); ok {
			t.Fatalf("runBuild failed: %v (relay-sim error: %v)", err, rerr)
		}
		t.Fatalf("runBuild failed: %v", err)
	}
	spec, ok := fp.lastBuildSpec.Load().(VMSpec)
	if !ok || spec.Generation != 1 {
		t.Errorf("build VM Generation = %v, want 1", fp.lastBuildSpec.Load())
	}
	if !spec.IsWindows {
		t.Error("build VM IsWindows = false, want true")
	}
}

// TestRunBuild_OverlayFormatMismatch rejects a build overlay whose extension
// does not match the base image (a differencing child must match its parent's
// on-disk format).
func TestRunBuild_OverlayFormatMismatch(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	cfg.BuildBaseVHDX = "build-base.vhd"                // Gen1 base
	cfg.BuildOverlay = filepath.Join(dir, "build.vhdx") // mismatched child
	if _, err := runBuild(context.Background(), &buildFake{}, cfg); err == nil {
		t.Fatal("expected a validation error for a base/overlay format mismatch")
	}
}

func TestRunBuild_NonzeroExit(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	fp := &buildFake{claimAddr: "127.0.0.1:49301", buildExit: 7}

	_, err := runBuild(context.Background(), fp, cfg)
	if err == nil {
		t.Fatal("expected an error for a nonzero build exit code")
	}
	if !strings.Contains(err.Error(), "code 7") {
		t.Errorf("error = %q, want it to mention exit code 7", err.Error())
	}
	if got := fp.remove.Load(); got != 2 {
		t.Errorf("RemoveVM = %d, want 2 even on failure (teardown)", got)
	}
}

func TestRunBuild_ValidatesConfig(t *testing.T) {
	cases := map[string]func(*buildConfig){
		"build ID":     func(c *buildConfig) { c.BuildID = "" },
		"output dir":   func(c *buildConfig) { c.OutputDir = "" },
		"relay boot":   func(c *buildConfig) { c.RelayBootVHDX = "" },
		"build base":   func(c *buildConfig) { c.BuildBaseVHDX = "" },
		"seed media":   func(c *buildConfig) { c.SeedISO = ""; c.SeedVHDX = "" },
		"build switch": func(c *buildConfig) { c.BuildSwitch = "" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := baseCfg(t, t.TempDir())
			mut(&cfg)
			if _, err := runBuild(
				context.Background(), &buildFake{}, cfg); err == nil {
				t.Errorf("expected a validation error when %s is empty", name)
			}
		})
	}
}

// TestRunBuild_VMOffWithoutCompletion: a guest that powers off without
// reporting an exit code fails the build promptly instead of hanging until the
// build timeout (heartbeat silence only counts once heartbeats have started).
func TestRunBuild_VMOffWithoutCompletion(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	cfg.Timeout = time.Minute
	cfg.vmPoll = 50 * time.Millisecond
	fp := &buildFake{claimAddr: "127.0.0.1:49301", powersOffSilently: true}

	start := time.Now()
	_, err := runBuild(context.Background(), fp, cfg)
	if err == nil ||
		!strings.Contains(err.Error(), "powered off without reporting an exit code") {
		t.Fatalf("err = %v, want the powered-off error", err)
	}
	if took := time.Since(start); took > vmOffGrace+10*time.Second {
		t.Errorf("took %s; the off VM should fail the build within the grace", took)
	}
	if got := fp.remove.Load(); got != 2 {
		t.Errorf("RemoveVM = %d, want 2 (teardown still runs)", got)
	}
}

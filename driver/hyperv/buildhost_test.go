package hyperv

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// buildFake is a Provisioner that stands in for a real Hyper-V host AND
// faithfully simulates the relay VM's boot-time behaviour, so runBuild can be
// exercised end to end without any VM:
//
//   - When the relay VM "starts", it fetches its config from the responder
//     (capturing the token exactly as the real init does) and POSTs /v1/ready,
//     which is what StartRelay blocks on.
//   - When the build VM "starts", it POSTs /v1/complete with a configurable exit
//     code, which is what runBuild blocks on.
//
// CreateDiffDisk creates a stub overlay file so overlay cleanup is observable.
type buildFake struct {
	host          string
	configPort    int
	collectorPort int
	buildExit     int // exit code the simulated build reports on /v1/complete

	token         atomic.Value // string, captured from the config fetch
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
		f.simulateBuildComplete()
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

func (f *buildFake) configURL() string {
	return fmt.Sprintf("http://%s:%d/config", f.host, f.configPort)
}
func (f *buildFake) collectorURL(path string) string {
	return fmt.Sprintf("http://%s:%d%s", f.host, f.collectorPort, path)
}

// simulateRelayReady mirrors the relay init: GET /config, parse the token, then
// POST /v1/ready with it.
func (f *buildFake) simulateRelayReady() {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(f.configURL())
	if err != nil {
		f.relayReadyErr.Store(fmt.Errorf("config fetch: %w", err))
		return
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "OUTPUT_SINK_TOKEN="); ok {
			f.token.Store(v)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, f.collectorURL("/v1/ready"), nil)
	if tok, ok := f.token.Load().(string); ok {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rr, err := client.Do(req)
	if err != nil {
		f.relayReadyErr.Store(fmt.Errorf("ready POST: %w", err))
		return
	}
	_, _ = io.Copy(io.Discard, rr.Body)
	_ = rr.Body.Close()
}

// simulateBuildComplete mirrors the relay reporting build completion.
func (f *buildFake) simulateBuildComplete() {
	client := &http.Client{Timeout: 3 * time.Second}
	body := strings.NewReader(fmt.Sprintf(`{"exit_code":%d,"message":"done"}`, f.buildExit))
	req, _ := http.NewRequest(http.MethodPost, f.collectorURL("/v1/complete"), body)
	req.Header.Set("Content-Type", "application/json")
	if tok, ok := f.token.Load().(string); ok {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rr, err := client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, rr.Body)
	_ = rr.Body.Close()
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
		natHostIP:     "127.0.0.1",
		configPort:    49299,
		collectorPort: 49390,
		readyTimeout:  5 * time.Second,
		Timeout:       5 * time.Second,
	}
}

func TestRunBuild_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	fp := &buildFake{host: cfg.natHostIP, configPort: cfg.configPort, collectorPort: cfg.collectorPort, buildExit: 0}

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
		t.Errorf("build VM Generation = %v, want 2 (default for .vhdx base)", fp.lastBuildSpec.Load())
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
	fp := &buildFake{host: cfg.natHostIP, configPort: cfg.configPort, collectorPort: cfg.collectorPort, buildExit: 0}

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
	cfg.BuildBaseVHDX = "build-base.vhd" // Gen1 base
	cfg.BuildOverlay = filepath.Join(dir, "build.vhdx") // mismatched child
	if _, err := runBuild(context.Background(), &buildFake{}, cfg); err == nil {
		t.Fatal("expected a validation error for a base/overlay format mismatch")
	}
}

func TestRunBuild_NonzeroExit(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	fp := &buildFake{host: cfg.natHostIP, configPort: cfg.configPort, collectorPort: cfg.collectorPort, buildExit: 7}

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
		"build ID":    func(c *buildConfig) { c.BuildID = "" },
		"output dir":  func(c *buildConfig) { c.OutputDir = "" },
		"relay boot":  func(c *buildConfig) { c.RelayBootVHDX = "" },
		"build base":  func(c *buildConfig) { c.BuildBaseVHDX = "" },
		"seed media":  func(c *buildConfig) { c.SeedISO = ""; c.SeedVHDX = "" },
		"build switch": func(c *buildConfig) { c.BuildSwitch = "" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := baseCfg(t, t.TempDir())
			mut(&cfg)
			if _, err := runBuild(context.Background(), &buildFake{}, cfg); err == nil {
				t.Errorf("expected a validation error when %s is empty", name)
			}
		})
	}
}

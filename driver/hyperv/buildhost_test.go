package hyperv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
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
//   - Like Hyper-V, it keeps each VM's Notes and the VMs attached to each
//     switch, which RelayNotes and SwitchAdapters read back; the relay VM's
//     seed "ISO" (copySeedISO) gives the simulated relay its claim token.
//
// CreateDiffDisk creates a stub overlay file so overlay cleanup is observable.
type buildFake struct {
	mu       sync.Mutex
	attached map[string][]string // switch -> attached VMs (preset, then created)
	notes    map[string]string   // relay VM -> Notes
	// intruder, when set, is attached to the relay's build switch as the relay
	// VM is created: a foreign VM on the build's segment.
	intruder     string
	// serveToken, when set, is the token the simulated relay requires in
	// place of its seed's.
	serveToken string
	adapterCalls atomic.Int32
	seed         atomic.Value // string: the relay seed the relay VM was created with
	lastRelay    atomic.Value // RelayVMSpec

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
	// buildVMsAtClaim is 1 + the build VMs created or started when the claim
	// arrived (0: no claim yet).
	buildVMsAtClaim atomic.Int32
}

func (f *buildFake) EnsureNetwork(context.Context, NetworkSpec) (*NetworkResources, error) {
	return &NetworkResources{}, nil
}
func (f *buildFake) TeardownNetwork(context.Context, string) error { return nil }

func (f *buildFake) CreateVM(_ context.Context, spec VMSpec) (*VMHandle, error) {
	f.createBuild.Add(1)
	f.lastBuildSpec.Store(spec)
	f.attach(spec.SwitchName, spec.Name)
	return &VMHandle{Name: spec.Name}, nil
}
func (f *buildFake) CreateRelayVM(_ context.Context, spec RelayVMSpec) (*VMHandle, error) {
	f.createRelay.Add(1)
	f.lastRelay.Store(spec)
	if b, err := os.ReadFile(spec.SeedISO); err == nil {
		f.seed.Store(string(b))
	}
	f.mu.Lock()
	if f.notes == nil {
		f.notes = map[string]string{}
	}
	f.notes[spec.Name] = spec.Notes
	f.mu.Unlock()
	f.attach(spec.BuildSwitch, spec.Name)
	if f.intruder != "" {
		f.attach(spec.BuildSwitch, f.intruder)
	}
	return &VMHandle{Name: spec.Name}, nil
}

// attach records vm's adapter on sw.
func (f *buildFake) attach(sw, vm string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attached == nil {
		f.attached = map[string][]string{}
	}
	f.attached[sw] = append(f.attached[sw], vm)
}

// seedKey is key's value in the relay seed the relay VM was created with.
func (f *buildFake) seedKey(key string) string {
	seed, _ := f.seed.Load().(string)
	for _, line := range strings.Split(seed, "\n") {
		if v, ok := strings.CutPrefix(line, "warden."+key+"="); ok {
			return v
		}
	}
	return ""
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
// RemoveVM forgets the VM's Notes and adapters, as Hyper-V frees its address
// and slot.
func (f *buildFake) RemoveVM(_ context.Context, name string) error {
	f.remove.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.notes, name)
	for sw, vms := range f.attached {
		var keep []string
		for _, vm := range vms {
			if vm != name {
				keep = append(keep, vm)
			}
		}
		f.attached[sw] = keep
	}
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
func (f *buildFake) RelayNotes(context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for vm, n := range f.notes {
		out[vm] = n
	}
	return out, nil
}
func (f *buildFake) SwitchAdapters(
	_ context.Context, switches []string,
) (map[string][]string, error) {
	f.adapterCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]string{}
	for _, sw := range switches {
		if vms := f.attached[sw]; len(vms) > 0 {
			out[sw] = append([]string(nil), vms...)
		}
	}
	return out, nil
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
		token := f.seedKey("claim_token")
		if f.serveToken != "" {
			token = f.serveToken
		}
		_ = relaycfg.Serve(ln, token, func(
			c relaycfg.Config, rt http.RoundTripper,
		) (relaycfg.Identity, error) {
			f.claimed.Store(c)
			f.transport.Store(rt)
			f.buildVMsAtClaim.Store(1 + f.createBuild.Load() + f.startBuild.Load())
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

// baseCfg returns a buildConfig wired to loopback and a free claim port, with
// the relay overlay pre-created so teardown-removal is observable. The port is
// picked at runtime: a fixed one can sit in a range Windows has excluded.
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
		NATPrefix:     testNATPrefix,
		HostIP:        testHostIP,
		RelaySeedDir:  filepath.Join(dir, "relay-seed"),
		makeSeedISO:   copySeedISO,
		claimPort:     freePort(t),
		readyTimeout:  5 * time.Second,
		Timeout:       5 * time.Second,
	}
}

// freePort returns a loopback TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port //nolint:forcetypeassert // a tcp listener
}

// The tests' NAT is 127.0.0.0/30 with the host at .2, so the allocator's
// lowest free address, where the fake relay listens, is 127.0.0.1: loopback
// on every platform.
var (
	testNATPrefix = netip.MustParsePrefix("127.0.0.0/30")
	testHostIP    = netip.MustParseAddr("127.0.0.2")
)

// copySeedISO stands in for IMAPI2: the "ISO" is a copy of the staged seed
// file, so a fake relay can read the token from it.
func copySeedISO(seedDir, iso string) error {
	b, err := os.ReadFile(filepath.Join(seedDir, RelaySeedFile))
	if err != nil {
		return err
	}
	return os.WriteFile(iso, b, 0o600)
}

// claimAddr is where the fake relay listens for cfg's claim.
func claimAddr(cfg buildConfig) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.claimPort))
}

func TestRunBuild_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	fp := &buildFake{claimAddr: claimAddr(cfg), buildExit: 0}

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
	fp := &buildFake{claimAddr: claimAddr(cfg), buildExit: 0}

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
	fp := &buildFake{claimAddr: claimAddr(cfg), buildExit: 7}

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
	fp := &buildFake{claimAddr: claimAddr(cfg), powersOffSilently: true}

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

// TestRunBuild_ClaimBeforeBuildVM pins BOOT-01's order on Hyper-V: no build VM
// is created or started until StartRelay's claim (which carries the request's
// build link) has returned.
func TestRunBuild_ClaimBeforeBuildVM(t *testing.T) {
	configured, _ := buildlink.Parse("100.126.255.252/30")
	for _, link := range []buildlink.Link{{}, configured} {
		dir := t.TempDir()
		cfg := baseCfg(t, dir)
		cfg.Request = &driver.BuildRequest{UpstreamSystemCA: true, BuildLink: link}
		fp := &buildFake{claimAddr: claimAddr(cfg)}
		if _, err := runBuild(context.Background(), fp, cfg); err != nil {
			t.Fatalf("runBuild: %v", err)
		}
		if got := fp.buildVMsAtClaim.Load(); got != 1 {
			t.Fatalf("%d build VM operations ran before the claim", got-1)
		}
		c, _ := fp.claimed.Load().(relaycfg.Config)
		want := link.OrDefault().Claim()
		if c.BuildLink == nil || *c.BuildLink != *want {
			t.Fatalf("claim build_link = %+v, want %+v", c.BuildLink, want)
		}
	}
}

// TestStartRelay_SeedAndClaim pins Decisions 2 to 4 end to end on the fake:
// the relay is claimed at the address allocated in the NAT prefix, with the
// token its seed carries as the claim's Bearer; the seed holds only the
// pre-claim values; req.DNS is exactly the claim's upstream_dns; the Notes
// record the allocation; and teardown frees the address and the switch.
func TestStartRelay_SeedAndClaim(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	dns := []netip.Addr{netip.MustParseAddr("10.20.0.53"), netip.MustParseAddr("10.20.0.54")}
	cfg.Request = &driver.BuildRequest{UpstreamSystemCA: true, DNS: dns}
	fp := &buildFake{claimAddr: claimAddr(cfg)}
	if _, err := runBuild(context.Background(), fp, cfg); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	if tok := fp.seedKey("claim_token"); !isClaimToken(tok) {
		t.Errorf("seed claim_token = %q, want 64 lowercase hex digits", tok)
	}
	for key, want := range map[string]string{"seed": "1", "net": "static",
		"nat_ip": "127.0.0.1", "nat_prefix": "30", "nat_gw": "127.0.0.2"} {
		if got := fp.seedKey(key); got != want {
			t.Errorf("seed %s = %q, want %q", key, got, want)
		}
	}
	if seed, _ := fp.seed.Load().(string); strings.Contains(seed, "dns") ||
		strings.Contains(seed, "10.20.0.53") {
		t.Errorf("the seed carries a resolver:\n%s", seed)
	}
	c, _ := fp.claimed.Load().(relaycfg.Config)
	if strings.Join(c.UpstreamDNS, " ") != "10.20.0.53 10.20.0.54" {
		t.Errorf("claim upstream_dns = %v, want [vm] dns in order", c.UpstreamDNS)
	}
	spec, _ := fp.lastRelay.Load().(RelayVMSpec)
	if spec.Notes != "warden-relay-ip=127.0.0.1 warden-relay-switch=warden-dev" ||
		spec.BuildSwitch != "warden-dev" || spec.SeedISO != cfg.RelaySeedDir+".iso" {
		t.Errorf("relay VM spec = %+v", spec)
	}
	notes, _ := fp.RelayNotes(context.Background())
	attached, _ := fp.SwitchAdapters(context.Background(), []string{"warden-dev"})
	if len(notes) != 0 || len(attached) != 0 {
		t.Errorf("after teardown: notes %v, adapters %v", notes, attached)
	}
}

// TestStartRelay_WrongTokenRefused: a relay that requires another token
// refuses the claim, and StartRelay fails and tears the relay down.
func TestStartRelay_WrongTokenRefused(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	cfg.readyTimeout = time.Second
	fp := &buildFake{claimAddr: claimAddr(cfg), serveToken: strings.Repeat("ee", 32)}
	if _, err := runBuild(context.Background(), fp, cfg); err == nil {
		t.Fatal("the claim with the seed's token was accepted by a relay wanting another")
	}
	if fp.createBuild.Load() != 0 || fp.remove.Load() != 1 {
		t.Errorf("build VMs created %d, VMs removed %d; want 0 and 1",
			fp.createBuild.Load(), fp.remove.Load())
	}
}

// TestRunBuild_DurableSlot: a durable build's relay takes the lowest free
// slot (one with a kept build VM is busy) and the build VM joins it.
func TestRunBuild_DurableSlot(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	cfg.BuildSwitch = ""
	cfg.Slots = []string{"warden-dev-0", "warden-dev-1"}
	fp := &buildFake{claimAddr: claimAddr(cfg),
		attached: map[string][]string{"warden-dev-0": {"warden-build-kept"}}}
	if _, err := runBuild(context.Background(), fp, cfg); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	relay, _ := fp.lastRelay.Load().(RelayVMSpec)
	build, _ := fp.lastBuildSpec.Load().(VMSpec)
	if relay.BuildSwitch != "warden-dev-1" || build.SwitchName != "warden-dev-1" {
		t.Errorf("relay on %q, build VM on %q; want both on warden-dev-1",
			relay.BuildSwitch, build.SwitchName)
	}
}

// TestRunBuild_SlotForeignAdapter: a VM other than this build's two on the
// slot refuses the build before the build VM starts (BOOT-01).
func TestRunBuild_SlotForeignAdapter(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	cfg.BuildSwitch = ""
	cfg.Slots = []string{"warden-dev-0"}
	fp := &buildFake{claimAddr: claimAddr(cfg), intruder: "warden-build-other"}
	_, err := runBuild(context.Background(), fp, cfg)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "also attached: warden-build-other") {
		t.Fatalf("err = %v", err)
	}
	if fp.startBuild.Load() != 0 {
		t.Error("the build VM started on a segment with a foreign VM")
	}
}

func TestCheckSlotAdapters(t *testing.T) {
	const r, b = "warden-relay-x", "warden-build-x"
	tests := []struct {
		attached []string
		ok       bool
	}{
		{[]string{r, b}, true},
		{[]string{b, r}, true},
		{[]string{r}, false},
		{[]string{r, b, b}, false},
		{[]string{r, b, "warden-relay-y"}, false},
	}
	for _, tt := range tests {
		if err := checkSlotAdapters("warden-dev-0", tt.attached, r, b); (err == nil) != tt.ok {
			t.Errorf("%v: err = %v", tt.attached, err)
		}
	}
}

// TestRunBuild_KeepVMsKeepsRelay: --keep-vms keeps both VMs, so the relay's
// Notes and adapter keep holding its address and slot.
func TestRunBuild_KeepVMsKeepsRelay(t *testing.T) {
	dir := t.TempDir()
	cfg := baseCfg(t, dir)
	cfg.KeepVMs = true
	fp := &buildFake{claimAddr: claimAddr(cfg)}
	if _, err := runBuild(context.Background(), fp, cfg); err != nil {
		t.Fatalf("runBuild: %v", err)
	}
	if fp.remove.Load() != 0 {
		t.Errorf("RemoveVM = %d, want 0 with --keep-vms", fp.remove.Load())
	}
	if notes, _ := fp.RelayNotes(context.Background()); len(notes) != 1 {
		t.Errorf("relay Notes after a kept build: %v", notes)
	}
}

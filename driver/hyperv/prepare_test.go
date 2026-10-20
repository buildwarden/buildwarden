package hyperv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
	"github.com/buildwarden/buildwarden/ledger"
)

// prepFake plays a prepare VM: each VMState call advances one step through
// steps; while a step is Running, GuestKVP returns its items.
type prepFake struct {
	fakeProv
	steps   []prepStep
	i       int
	created VMSpec
	removed bool
}

type prepStep struct {
	state string
	kv    map[string]string
}

func (f *prepFake) CreateVM(_ context.Context, s VMSpec) (*VMHandle, error) {
	f.created = s
	return &VMHandle{Name: s.Name}, nil
}
func (f *prepFake) CreateDiffDisk(_ context.Context, overlay, _ string) error {
	return os.WriteFile(overlay, []byte("prepared-layer"), 0o644)
}
func (f *prepFake) RemoveVM(context.Context, string) error { f.removed = true; return nil }
func (f *prepFake) VMState(context.Context, string) (string, error) {
	if f.i >= len(f.steps) {
		return "Off", nil
	}
	f.i++
	return f.steps[f.i-1].state, nil
}
func (f *prepFake) GuestKVP(context.Context, string) (map[string]string, error) {
	return f.steps[f.i-1].kv, nil
}

func fakeSeed(workDir string) (string, string, error) {
	dir := filepath.Join(workDir, "prepare-seed")
	if err := stagePrepareSeedDir(dir, "amd64"); err != nil {
		return "", "", err
	}
	return filepath.Join(workDir, "prepare-seed.iso"), dir, nil
}

// withBase sets up a hermetic cache with an active generalized base.
func withBase(t *testing.T) evalManifestEntry {
	t.Helper()
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir()) // prepare lock
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	base := filepath.Join(t.TempDir(), "eval.vhdx")
	if err := os.WriteFile(base, []byte("generalized"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := evalManifestEntry{
		Name: "windows-server-2025", Path: base, SHA256: sum256([]byte("generalized")),
		Generation: 2, OS: "windows",
	}
	m := &evalManifest{Active: e.Name, Images: map[string]evalManifestEntry{e.Name: e}}
	if err := saveEvalManifest(m); err != nil {
		t.Fatal(err)
	}
	return e
}

func doneSteps(reportedAt string) []prepStep {
	return []prepStep{
		{state: "Running", kv: map[string]string{"SessionMonitor": "1"}},
		{state: "Running", kv: map[string]string{winguest.KVPState: "running:services"}},
		{state: "Running", kv: map[string]string{winguest.KVPState: "running:reboot"}},
		{state: "Running", kv: map[string]string{
			winguest.KVPState:          winguest.StateDone,
			winguest.KVPGraceMinutes:   "14400",
			winguest.KVPLicenseStatus:  "2",
			winguest.KVPLicenseChannel: evalChannel,
			winguest.KVPReportedAt:     reportedAt,
		}},
		{state: "Off"},
	}
}

const evalChannel = "Windows(R) Operating System, TIMEBASED_EVAL channel"

var fastPrepare = PrepareOptions{Poll: time.Millisecond, Timeout: 5 * time.Second}

func TestPrepareBaseRecordsActivePreparedLayer(t *testing.T) {
	base := withBase(t)
	f := &prepFake{steps: doneSteps("2026-10-20T18:00:00.0000000Z")}

	pb, err := prepareBase(context.Background(), f, fakeSeed, fastPrepare)
	if err != nil {
		t.Fatalf("prepareBase: %v", err)
	}
	assertPrepareVM(t, f, pb)
	if pb.GraceEnd != "2026-10-30T18:00:00Z" {
		t.Errorf("grace end = %q, want 2026-10-30T18:00:00Z", pb.GraceEnd)
	}
	if pb.BaseSHA256 != base.SHA256 || pb.SHA256 != sum256([]byte("prepared-layer")) {
		t.Errorf("hashes not recorded: %+v", pb)
	}
	if fi, err := os.Stat(pb.Path); err != nil || fi.Mode().Perm()&0o200 != 0 {
		t.Errorf("prepared layer should exist read-only: %v %v", fi, err)
	}

	got, gotBase, ok := activePrepared()
	if !ok || got.Path != pb.Path || gotBase.Name != base.Name {
		t.Fatalf("activePrepared = %+v, %+v, %v", got, gotBase, ok)
	}
	assertPreparedSource(t, pb.Path)
}

// assertPrepareVM checks the prepare VM had no network, took its generation
// and disk format from the base, and was removed.
func assertPrepareVM(t *testing.T, f *prepFake, pb *PreparedBase) {
	t.Helper()
	if f.created.SwitchName != "" {
		t.Errorf("prepare VM got switch %q; prepare must have no network",
			f.created.SwitchName)
	}
	if f.created.Generation != 2 || !strings.HasSuffix(pb.Path, ".vhdx") {
		t.Errorf("generation/format not taken from the base: %+v %s", f.created, pb.Path)
	}
	if !f.removed {
		t.Error("prepare VM was not removed")
	}
}

// assertPreparedSource checks the layer cache records the layer as prepared
// by this warden, with every prepare-seed file as a hashed input.
func assertPreparedSource(t *testing.T, layer string) {
	t.Helper()
	src, ok := LayerCache().Source(layer)
	if !ok || src.Kind != ledger.SourcePrepared || src.PreparedBy != driver.PreparedBy() {
		t.Fatalf("layer cache source = %+v, %v", src, ok)
	}
	names := map[string]bool{}
	for _, in := range src.Inputs {
		names[in.Name] = in.SHA256 != ""
	}
	for _, want := range []string{
		"unattend.xml", "Autounattend.xml", winguest.PrepareScriptName,
	} {
		if !names[want] {
			t.Errorf("prepared inputs lack a hashed %s: %+v", want, src.Inputs)
		}
	}
}

func TestPrepareBaseFailsWithoutDone(t *testing.T) {
	cases := map[string]struct {
		steps []prepStep
		want  string
	}{
		"guest error": {
			steps: []prepStep{
				{state: "Running", kv: map[string]string{
					winguest.KVPState: "error:register warden-run: boom",
				}},
				{state: "Off"},
			},
			want: "register warden-run: boom",
		},
		"off before done": {
			steps: []prepStep{
				{state: "Running", kv: map[string]string{
					winguest.KVPState: "running:defender",
				}},
				{state: "Off"},
			},
			want: `last guest state "running:defender"`,
		},
		"vm vanished": {
			steps: []prepStep{{state: "Running"}, {state: ""}},
			want:  "disappeared",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			withBase(t)
			f := &prepFake{steps: c.steps}
			_, err := prepareBase(context.Background(), f, fakeSeed, fastPrepare)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
			if _, _, ok := activePrepared(); ok {
				t.Error("a failed prepare must not set an active prepared base")
			}
			matches, _ := filepath.Glob(
				filepath.Join(evalImagesDir(), "prepared", "*.vhdx"))
			if len(matches) != 0 {
				t.Errorf("failed prepare left layers behind: %v", matches)
			}
		})
	}
}

func TestPrepareBaseTimesOut(t *testing.T) {
	withBase(t)
	steps := make([]prepStep, 10000)
	for i := range steps {
		steps[i] = prepStep{
			state: "Running", kv: map[string]string{winguest.KVPState: "running:ngen"},
		}
	}
	opts := PrepareOptions{Poll: time.Millisecond, Timeout: 50 * time.Millisecond}
	_, err := prepareBase(context.Background(), &prepFake{steps: steps}, fakeSeed, opts)
	if err == nil || !strings.Contains(err.Error(), "did not finish within") {
		t.Fatalf("err = %v, want a timeout", err)
	}
}

func TestPrepareBaseExclusive(t *testing.T) {
	withBase(t)
	unlock, ok, err := driver.TryHostLock(prepareLockName)
	if err != nil || !ok {
		t.Fatalf("taking the prepare lock: %v %v", ok, err)
	}
	defer unlock()
	_, err = prepareBase(context.Background(), &prepFake{}, fakeSeed, fastPrepare)
	if !errors.Is(err, ErrPrepareRunning) {
		t.Fatalf("err = %v, want ErrPrepareRunning", err)
	}
}

func TestPrepareBaseNeedsBase(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	_, err := prepareBase(context.Background(), &prepFake{}, fakeSeed, fastPrepare)
	if err == nil || !strings.Contains(err.Error(), "no generalized Hyper-V base image") {
		t.Fatalf("err = %v", err)
	}
}

func TestGraceEndOnlyForEval(t *testing.T) {
	r := map[string]string{
		winguest.KVPGraceMinutes:   "60",
		winguest.KVPReportedAt:     "2026-10-20T00:00:00Z",
		winguest.KVPLicenseChannel: "Windows(R) Operating System, VOLUME_KMS channel",
	}
	if got := graceEnd(r); got != "" {
		t.Errorf("non-eval grace end = %q, want empty", got)
	}
	r[winguest.KVPLicenseChannel] = evalChannel
	if got := graceEnd(r); got != "2026-10-20T01:00:00Z" {
		t.Errorf("eval grace end = %q", got)
	}
}

func TestParseKVPLines(t *testing.T) {
	kv := parseKVPLines("a=1\r\nwarden-x=y=z\n\nbad\n")
	if kv["a"] != "1" || kv["warden-x"] != "y=z" || len(kv) != 2 {
		t.Errorf("parseKVPLines = %v", kv)
	}
}

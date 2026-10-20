package hyperv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

// fetchPinnedEval fetches a pinned fake eval image and returns its path and URL.
func fetchPinnedEval(t *testing.T, body []byte) (path, url string) {
	t.Helper()
	srv, _ := evalServer(t, body)
	url = srv.URL + "/server2025.vhd"
	path, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025", URL: url, SHA256: sum256(body),
	})
	if err != nil {
		t.Fatal(err)
	}
	return path, url
}

// stageTestSeed stages a real build-guest seed plus build script in workDir.
func stageTestSeed(t *testing.T, workDir string) (seed, script string) {
	t.Helper()
	seed, err := stageSeedDir(workDir, runtime.GOARCH, nil)
	if err != nil {
		t.Skipf("no warden-io guest binary here: %v", err)
	}
	if seed != buildSeedDir(workDir) {
		t.Fatalf("buildSeedDir = %q, want %q", buildSeedDir(workDir), seed)
	}
	script = filepath.Join(workDir, "build.ps1")
	if err := os.WriteFile(script, []byte("Write-Host hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	return seed, script
}

// TestDescribeBuildEvalImage fetches a pinned eval image and checks that a
// build's descriptor carries its pinned URL provenance, generation and seed.
func TestDescribeBuildEvalImage(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("fake-eval-vhd")
	base, url := fetchPinnedEval(t, body)
	seed, script := stageTestSeed(t, t.TempDir())

	env, err := describeBuild(LayerCache(), []string{base}, true, 1, seed, script)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ledger.DecodeVMDiskChain(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.Driver != "hyperv" || d.GuestOS != "windows" || d.Generation != 1 {
		t.Fatalf("descriptor = %+v", d)
	}
	l := d.Layers[0]
	want := ledger.LayerSource{Kind: ledger.SourceURL, URL: url, Pinned: true}
	if l.Role != ledger.LayerRoleDisk || l.Format != "vhd" || l.SHA256 != sum256(body) ||
		l.Source.Kind != want.Kind || l.Source.URL != want.URL || !l.Source.Pinned {
		t.Fatalf("base layer = %+v, want the pinned eval VHD", l)
	}
	assertBuildSeed(t, d.Seed)
}

// assertBuildSeed checks a build's recorded seed: the agent, the bootstrap
// and the build script, and no answer file (builds boot a prepared image;
// answer files belong to the prepare seed).
func assertBuildSeed(t *testing.T, seed []ledger.SeedFile) {
	t.Helper()
	names := map[string]bool{}
	for _, s := range seed {
		names[s.Name] = true
	}
	for _, want := range []string{"warden-io.exe", "warden-run.ps1", driver.SeedBuildScript} {
		if !names[want] {
			t.Errorf("seed lacks %s: %+v", want, seed)
		}
	}
	if names["unattend.xml"] || names["Autounattend.xml"] {
		t.Errorf("build seed carries an answer file: %+v", seed)
	}
}

// TestRecordEvalLayerDistrustsStaleSidecar checks that re-pinning a cached
// image whose bytes no longer match its .sha256 sidecar does not hand the
// sidecar hash, or the URL provenance, to builds.
func TestRecordEvalLayerDistrustsStaleSidecar(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	dest := filepath.Join(evalImagesDir(), "x.vhd")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordEvalLayer(dest, sum256([]byte("original")), "https://x/x.vhd", true, false, nil)

	info, err := LayerCache().Lookup(dest, "")
	if err != nil {
		t.Fatal(err)
	}
	if info.SHA256 != sum256([]byte("tampered")) || info.Source.Kind != ledger.SourceLocal {
		t.Fatalf("layer = %+v, want the real hash as a local file", info)
	}
}

func TestDescribeBuildLocalImage(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	img := filepath.Join(t.TempDir(), "guest.vhdx")
	if err := os.WriteFile(img, []byte("gen2"), 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := describeBuild(LayerCache(), []string{img}, false, 2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ledger.DecodeVMDiskChain(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.GuestOS != "linux" || d.Generation != 2 || d.Layers[0].Format != "vhdx" ||
		d.Layers[0].Source.Kind != ledger.SourceLocal || len(d.Seed) != 0 {
		t.Fatalf("descriptor = %+v", d)
	}
}

package qemu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/winguest"
)

// hermeticCache points driver.CacheDir (and the lock dir) at a temp dir.
func hermeticCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LocalAppData", dir)   // Windows
	t.Setenv("XDG_CACHE_HOME", dir) // Linux
	t.Setenv("HOME", dir)           // macOS
	if err := os.MkdirAll(filepath.Join(driver.CacheDir(), "images"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsPrepareArgsHaveNoNetwork(t *testing.T) {
	args := strings.Join(
		windowsPrepareArgs("aarch64", "hvf", "/l.qcow2", "/v.fd", "/s.img"), " ")
	for _, want := range []string{
		"-nic none",
		"file=/l.qcow2,format=qcow2",
		"if=pflash,format=raw,file=/v.fd",
		"if=none,id=seed,format=raw,file=/s.img",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("prepare args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, "readonly=on,file=/s.img") || strings.Contains(args, "netdev") {
		t.Errorf("seed must be writable and there must be no network:\n%s", args)
	}
}

func TestReadPrepareResult(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "seed.img")
	block := func(seq, state string) string {
		return "WARDEN-PREPARE-STATUS seq=" + seq + "\n" +
			winguest.KVPState + "=" + state +
			"\nEND-WARDEN-PREPARE-STATUS\n"
	}
	cases := map[string]struct {
		raw, wantErr string
	}{
		"done":        {raw: block("1", "running:ngen") + block("2", "done")},
		"guest error": {raw: block("5", "error:boom"), wantErr: "boom"},
		"stopped":     {raw: block("5", "running:defender"), wantErr: "running:defender"},
		"no status":   {raw: "", wantErr: "without reporting a status"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			writeFile(t, img, string(make([]byte, 512))+c.raw)
			_, err := readPrepareResult(img)
			if c.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErr != "" &&
				(err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
		})
	}
}

func TestResolveWindowsBaseImage(t *testing.T) {
	hermeticCache(t)
	req := &driver.BuildRequest{}
	ga := "arm64"

	if _, _, err := resolveWindowsBaseImage("aarch64", req); err == nil ||
		!strings.Contains(err.Error(), "no prepared Windows base image") {
		t.Fatalf("no base: err = %v", err)
	}

	// A base installed before the generalizing install boots on its own.
	base := WindowsImagePath(ga)
	writeFile(t, base, "base")
	chain, vars, err := resolveWindowsBaseImage("aarch64", req)
	if err != nil || len(chain) != 1 || chain[0] != base || vars != windowsVarsPath(ga) {
		t.Fatalf("legacy base: %q %q %v", chain, vars, err)
	}

	// A generalized base must be prepared first.
	writeFile(t, generalizedMarker(ga), "")
	if _, _, err := resolveWindowsBaseImage("aarch64", req); err == nil ||
		!strings.Contains(err.Error(), "has not been prepared") {
		t.Fatalf("unprepared generalized base: err = %v", err)
	}

}

func TestResolveWindowsBaseImagePrepared(t *testing.T) {
	hermeticCache(t)
	req := &driver.BuildRequest{}
	ga := "arm64"
	base := WindowsImagePath(ga)
	writeFile(t, base, "base")
	writeFile(t, generalizedMarker(ga), "")

	// With an active layer, the chain is base then layer, with its vars.
	layer := filepath.Join(driver.CacheDir(), "images", "prepared", "windows-arm64-x.qcow2")
	layerVars := strings.TrimSuffix(layer, ".qcow2") + "-vars.fd"
	writeFile(t, layer, "layer")
	writeFile(t, layerVars, "vars")
	now := time.Now().UTC()
	m := &preparedManifest{Active: layer, Prepared: []PreparedLayer{{
		Path: layer, VarsFD: layerVars, PreparedAt: now.Format(time.RFC3339),
		GraceEnd: now.Add(10 * 24 * time.Hour).Format(time.RFC3339),
	}}}
	if err := savePreparedManifest(ga, m); err != nil {
		t.Fatal(err)
	}
	chain, vars, err := resolveWindowsBaseImage("aarch64", req)
	if err != nil || len(chain) != 2 || chain[0] != base || chain[1] != layer ||
		vars != layerVars {
		t.Fatalf("prepared: %q %q %v", chain, vars, err)
	}

	// Under the minimum grace, the build is refused.
	m.Prepared[0].GraceEnd = now.Add(time.Hour).Format(time.RFC3339)
	if err := savePreparedManifest(ga, m); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveWindowsBaseImage("aarch64", req); err == nil ||
		!strings.Contains(err.Error(), "evaluation grace ends") {
		t.Fatalf("low grace: err = %v", err)
	}
}

func TestRetireAndDiscardWindowsLayers(t *testing.T) {
	hermeticCache(t)
	ga := "arm64"
	dir := filepath.Join(driver.CacheDir(), "images", "prepared")
	var m preparedManifest
	for _, n := range []string{"a", "b", "c", "d"} {
		p := filepath.Join(dir, n+".qcow2")
		writeFile(t, p, n)
		writeFile(t, p+".vars", n)
		m.Prepared = append(m.Prepared, PreparedLayer{Path: p, VarsFD: p + ".vars"})
	}
	m.Active = m.Prepared[3].Path
	if err := savePreparedManifest(ga, &m); err != nil {
		t.Fatal(err)
	}
	retireWindowsLayers(ga, os.Stderr)
	got, err := loadPreparedManifest(ga)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Prepared) != 2 || got.Prepared[0].Path != m.Prepared[2].Path {
		t.Fatalf("after retire: %+v, want c and d", got.Prepared)
	}
	for _, gone := range m.Prepared[:2] {
		if _, err := os.Stat(gone.Path); !os.IsNotExist(err) {
			t.Errorf("%s not deleted", gone.Path)
		}
	}

	discardWindowsLayers(ga)
	if _, err := os.Stat(preparedManifestPath(ga)); !os.IsNotExist(err) {
		t.Error("manifest not removed")
	}
	if _, err := os.Stat(m.Prepared[3].Path); !os.IsNotExist(err) {
		t.Error("active layer not removed with its base")
	}
}

func TestPrepareWindowsLayerNeedsGeneralizedBase(t *testing.T) {
	hermeticCache(t)
	writeFile(t, WindowsImagePath("arm64"), "pre-split base")
	_, err := PrepareWindowsLayer("arm64", false, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "predates the generalizing install") {
		t.Fatalf("err = %v", err)
	}
}

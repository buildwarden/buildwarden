package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/ledger"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestCache(t *testing.T, dir string) *LayerCache {
	t.Helper()
	return &LayerCache{Path: filepath.Join(dir, LayerCacheFile)}
}

var localSource = ledger.LayerSource{Kind: ledger.SourceLocal}

func TestLayerCacheRecordedProvenanceReused(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "base.vhd")
	writeFile(t, disk, "pristine")
	c := newTestCache(t, dir)

	// A deliberately wrong hash proves the cached value (not a re-hash) is used.
	fake := strings.Repeat("1", 64)
	src := ledger.LayerSource{Kind: ledger.SourceURL, URL: "https://x/base.vhd", Pinned: true}
	if err := c.Record(disk, fake, "", src); err != nil {
		t.Fatal(err)
	}
	info, err := c.Lookup(disk, "")
	if err != nil {
		t.Fatal(err)
	}
	if info.SHA256 != fake || info.Source.URL != src.URL || !info.Source.Pinned {
		t.Fatalf("lookup = %+v, want the recorded entry", info)
	}
	if info.Size != int64(len("pristine")) {
		t.Fatalf("size = %d", info.Size)
	}
}

func TestLayerCacheInvalidation(t *testing.T) {
	want := hashString(t, "changed!")
	rewrite := func(t *testing.T, disk string) { writeFile(t, disk, "changed!") }
	tests := []struct {
		name   string
		mutate func(t *testing.T, disk string)
		parent string
	}{
		{name: "content and size", mutate: rewrite},
		{name: "mtime only", mutate: func(t *testing.T, disk string) {
			rewrite(t, disk)
			// Same size as before is still caught by mtime.
			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(disk, later, later); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "parent changed", parent: strings.Repeat("2", 64), mutate: rewrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			disk := filepath.Join(dir, "base.qcow2")
			writeFile(t, disk, "original")
			c := newTestCache(t, dir)
			src := ledger.LayerSource{Kind: ledger.SourceURL, URL: "https://x"}
			if err := c.Record(disk, hashString(t, "original"), "", src); err != nil {
				t.Fatal(err)
			}
			tt.mutate(t, disk)
			info, err := c.Lookup(disk, tt.parent)
			if err != nil {
				t.Fatal(err)
			}
			if info.SHA256 != want {
				t.Fatalf("sha = %s, want re-hash %s", info.SHA256, want)
			}
			if info.Source.Kind != ledger.SourceLocal {
				t.Fatalf("source = %+v, want provenance dropped", info.Source)
			}
		})
	}
}

func TestLayerCacheVerify(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.img")
	drift := filepath.Join(dir, "drift.img")
	gone := filepath.Join(dir, "gone.img")
	c := newTestCache(t, dir)
	same := hashString(t, "same")
	for _, p := range []string{keep, drift, gone} {
		writeFile(t, p, "same")
		if err := c.Record(p, same, "", localSource); err != nil {
			t.Fatal(err)
		}
	}
	// Tamper without changing size or mtime: only a full re-hash notices.
	fi, _ := os.Stat(drift)
	writeFile(t, drift, "diff")
	_ = os.Chtimes(drift, fi.ModTime(), fi.ModTime())
	_ = os.Remove(gone)

	res, err := c.Verify()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range res {
		got[filepath.Base(r.Path)] = r.Status
	}
	want := map[string]string{"keep.img": "ok", "drift.img": "changed", "gone.img": "missing"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if info, _ := c.Lookup(drift, ""); info.SHA256 == same {
		t.Error("verify did not update the drifted entry")
	}
}

// diskChainFixture is a base disk with a child, a seed dir and a build script.
type diskChainFixture struct {
	base, child, seed, script string
}

func newDiskChainFixture(t *testing.T) diskChainFixture {
	t.Helper()
	dir := t.TempDir()
	f := diskChainFixture{
		base:   filepath.Join(dir, "base.vhd"),
		child:  filepath.Join(dir, "prepared.vhd"),
		seed:   filepath.Join(dir, "seed"),
		script: filepath.Join(dir, "build.ps1"),
	}
	writeFile(t, f.base, "base")
	writeFile(t, f.child, "child")
	if err := os.MkdirAll(f.seed, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.seed, "warden-run.ps1"), "run")
	writeFile(t, filepath.Join(f.seed, "unattend.xml"), "xml")
	writeFile(t, f.script, "echo hi")
	return f
}

func (f diskChainFixture) build(t *testing.T, c *LayerCache) *DiskChain {
	t.Helper()
	b := NewDiskChain(c, "hyperv", "windows", "amd64", 1)
	for _, p := range []string{f.base, f.child} {
		if err := b.AddLayer(ledger.LayerRoleDisk, p, "vhd"); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

func TestDiskChainEnvironment(t *testing.T) {
	f := newDiskChainFixture(t)
	b := f.build(t, newTestCache(t, t.TempDir()))
	if err := b.AddSeedDir(f.seed); err != nil {
		t.Fatal(err)
	}
	if err := b.AddSeedFile(SeedBuildScript, f.script); err != nil {
		t.Fatal(err)
	}
	env, err := b.Environment()
	if err != nil {
		t.Fatal(err)
	}
	if typ, err := ledger.EnvironmentType(env.Payload); err != nil || typ != env.Type {
		t.Fatalf("payload names %q (%v), want %q", typ, err, env.Type)
	}
	d, err := ledger.DecodeVMDiskChain(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.Generation != 1 || len(d.Layers) != 2 || d.Layers[0].Name != "base.vhd" {
		t.Fatalf("descriptor = %+v", d)
	}
	if d.Layers[0].SHA256 != hashString(t, "base") {
		t.Fatalf("base sha = %s", d.Layers[0].SHA256)
	}
	var names []string
	for _, s := range d.Seed {
		names = append(names, s.Name)
	}
	want := SeedBuildScript + ",unattend.xml,warden-run.ps1"
	if strings.Join(names, ",") != want {
		t.Fatalf("seed = %v, want sorted %s", names, want)
	}
}

// TestDiskChainChildBoundToParent checks that a child's cache entry is bound to
// its parent's hash: a changed base forces the child to be re-hashed.
func TestDiskChainChildBoundToParent(t *testing.T) {
	f := newDiskChainFixture(t)
	c := newTestCache(t, t.TempDir())
	_ = c.Record(f.child, strings.Repeat("3", 64), hashString(t, "base"), localSource)

	writeFile(t, f.base, "base, modified")
	got := f.build(t, c).Descriptor().Layers[1].SHA256
	if want := hashString(t, "child"); got != want {
		t.Fatalf("child sha = %s, want re-hash %s", got, want)
	}
}

func hashString(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "h")
	writeFile(t, p, s)
	sum, err := HashFileSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

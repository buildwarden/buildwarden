package vz

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// t0 is a fixed instant the fixtures age .prepared markers from (TEST-04).
var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// makeImage lays out a valid prepared image directory named name in cacheDir
// whose .prepared marker was written at prepared, and returns its path.
func makeImage(t *testing.T, cacheDir, name string, prepared time.Time) string {
	t.Helper()
	dir := filepath.Join(cacheDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		"disk.img", "aux-storage", "hardware-model", "machine-id", ".prepared",
	} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(filepath.Join(dir, ".prepared"), prepared, prepared); err != nil {
		t.Fatal(err)
	}
	return dir
}

// recordIPSW records dir's disk as prepared from the IPSW at url.
func recordIPSW(t *testing.T, c *ImageCache, dir, url string) {
	t.Helper()
	if err := c.recordPreparedImage(dir, ipswInput(url, "")); err != nil {
		t.Fatal(err)
	}
}

func TestImageDir(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, cache string)
		active string // written to <cache>/active when set
		want   string // base name, "" for none
	}{
		{name: "empty cache", setup: func(*testing.T, string) {}},
		{name: "pointer to a valid image wins over a newer one",
			setup: func(t *testing.T, cache string) {
				makeImage(t, cache, "macos-a", t0)
				makeImage(t, cache, "macos-b", t0.Add(time.Hour))
			},
			active: "macos-a\n", want: "macos-a"},
		{name: "no pointer: newest .prepared",
			setup: func(t *testing.T, cache string) {
				makeImage(t, cache, "macos-a", t0)
				makeImage(t, cache, "macos-b", t0.Add(time.Hour))
			},
			want: "macos-b"},
		{name: "no pointer: the name breaks an mtime tie",
			setup: func(t *testing.T, cache string) {
				makeImage(t, cache, "macos-b", t0)
				makeImage(t, cache, "macos-a", t0)
			},
			want: "macos-a"},
		{name: "pointer to a missing directory falls back",
			setup: func(t *testing.T, cache string) {
				makeImage(t, cache, "macos-a", t0)
				makeImage(t, cache, "macos-b", t0.Add(time.Hour))
			},
			active: "macos-gone\n", want: "macos-b"},
		{name: "pointer to an invalid directory falls back",
			setup: func(t *testing.T, cache string) {
				bad := makeImage(t, cache, "macos-bad", t0.Add(2*time.Hour))
				if err := os.Remove(filepath.Join(bad, ".prepared")); err != nil {
					t.Fatal(err)
				}
				makeImage(t, cache, "macos-a", t0)
			},
			active: "macos-bad\n", want: "macos-a"},
		{name: "a pointer that escapes the cache is ignored",
			setup: func(t *testing.T, cache string) {
				makeImage(t, cache, "macos-a", t0)
			},
			active: "../elsewhere\n", want: "macos-a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := t.TempDir()
			tc.setup(t, cache)
			if tc.active != "" {
				p := filepath.Join(cache, activeFile)
				if err := os.WriteFile(p, []byte(tc.active), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := (&ImageCache{CacheDir: cache}).ImageDir()
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if tc.want != "" {
				want = filepath.Join(cache, tc.want)
			}
			if got != want {
				t.Fatalf("ImageDir = %q, want %q", got, want)
			}
		})
	}
}

func TestSetActive(t *testing.T) {
	cache := t.TempDir()
	c := &ImageCache{CacheDir: cache}
	a := makeImage(t, cache, "macos-a", t0)
	makeImage(t, cache, "macos-b", t0.Add(time.Hour))
	if err := c.SetActive(a); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(cache, activeFile))
	if err != nil || string(b) != "macos-a\n" {
		t.Fatalf("active = %q, %v", b, err)
	}
	if got, _ := c.ImageDir(); got != a {
		t.Fatalf("ImageDir after SetActive = %q, want %q", got, a)
	}
	leftovers, _ := filepath.Glob(filepath.Join(cache, activeFile+".*"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left: %v", leftovers)
	}
	if err := c.SetActive(filepath.Join(t.TempDir(), "macos-x")); err == nil {
		t.Fatal("SetActive accepted a directory outside the cache")
	}
}

func TestImages(t *testing.T) {
	cache := t.TempDir()
	c := &ImageCache{CacheDir: cache}
	a := makeImage(t, cache, "macos-a", t0)
	b := makeImage(t, cache, "macos-b", t0.Add(time.Hour))
	if err := os.MkdirAll(filepath.Join(cache, "macos-partial"), 0o755); err != nil {
		t.Fatal(err)
	}
	recordIPSW(t, c, a, ipsw27_0)

	imgs := c.Images()
	if len(imgs) != 2 {
		t.Fatalf("Images = %+v, want the two valid images", imgs)
	}
	got := map[string]Image{}
	for _, img := range imgs {
		got[img.Dir] = img
	}
	if img := got[a]; img.Active || img.IPSW != "UniversalMac_27.0.1_26A100_Restore.ipsw" ||
		!img.Prepared.Equal(t0) {
		t.Errorf("older image = %+v", img)
	}
	if img := got[b]; !img.Active || img.IPSW != "" || !img.Prepared.Equal(t0.Add(time.Hour)) {
		t.Errorf("active image = %+v", img)
	}

	if err := c.SetActive(a); err != nil {
		t.Fatal(err)
	}
	for _, img := range c.Images() {
		if img.Active != (img.Dir == a) {
			t.Errorf("after SetActive(a): %+v", img)
		}
	}
}

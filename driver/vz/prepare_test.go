package vz

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

const (
	ipsw27_0 = "https://updates.cdn-apple.com/a/UniversalMac_27.0.1_26A100_Restore.ipsw"
	ipsw27_2 = "https://updates.cdn-apple.com/b/UniversalMac_27.2_26B123_Restore.ipsw"
)

// fakeHost replaces every host step of an ImageCache and records the calls.
type fakeHost struct {
	calls     []string
	latest    string
	latestErr error
	failWith  error // returned by restore and refresh when set
	stderr    bytes.Buffer
}

// cache returns an ImageCache in a fresh temp dir wired to h.
func (h *fakeHost) cache(t *testing.T) *ImageCache {
	t.Helper()
	return h.wire(&ImageCache{CacheDir: t.TempDir()})
}

func (h *fakeHost) wire(c *ImageCache) *ImageCache {
	c.latest = func() (string, error) {
		h.calls = append(h.calls, "latest")
		return h.latest, h.latestErr
	}
	c.download = func(url, dst string) error {
		h.calls = append(h.calls, "download "+url)
		return os.WriteFile(dst, []byte("ipsw:"+url), 0o644)
	}
	c.restoreDisk = func(ipsw, imgDir string) error {
		h.calls = append(h.calls, "restore "+ipsw+" -> "+filepath.Base(imgDir))
		if h.failWith != nil {
			return h.failWith
		}
		for _, f := range []string{"disk.img", "aux-storage", "hardware-model", "machine-id"} {
			if err := os.WriteFile(filepath.Join(imgDir, f), []byte(f), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	c.refresh = func(imgDir string) error {
		h.calls = append(h.calls, "refresh "+filepath.Base(imgDir))
		return h.failWith
	}
	c.requireRoot = func() error {
		h.calls = append(h.calls, "root")
		return nil
	}
	c.log = &h.stderr
	return c
}

// keyDir is the image directory name an IPSW source keys to.
func keyDir(source string) string { return "macos-" + shortHash(source) }

func activeName(t *testing.T, c *ImageCache) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(c.CacheDir, activeFile))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestClassify(t *testing.T) {
	cache := t.TempDir()
	img := makeImage(t, cache, "macos-abc", t0)
	outside := makeImage(t, t.TempDir(), "macos-out", t0)
	notImage := filepath.Join(cache, "macos-partial")
	if err := os.MkdirAll(notImage, 0o755); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	ipsw := filepath.Join(work, "UniversalMac_27.2_26B123_Restore.ipsw")
	if err := os.WriteFile(ipsw, []byte("ipsw"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)

	for _, tc := range []struct {
		name, source string
		want         prepSource
		refused      bool
	}{
		{name: "empty", source: "", want: prepSource{kind: srcEmpty}},
		{name: "https URL", source: ipsw27_2, want: prepSource{kind: srcURL, path: ipsw27_2}},
		{name: "file URL", source: "file:///x/y.ipsw",
			want: prepSource{kind: srcURL, path: "file:///x/y.ipsw"}},
		{name: "absolute IPSW file", source: ipsw, want: prepSource{kind: srcFile, path: ipsw}},
		{name: "relative IPSW file", source: filepath.Base(ipsw),
			want: prepSource{kind: srcFile, path: ipsw}},
		{name: "prepared directory", source: img, want: prepSource{kind: srcImage, path: img}},
		{name: "its disk.img", source: filepath.Join(img, "disk.img"),
			want: prepSource{kind: srcImage, path: img}},
		{name: "prepared directory outside the cache", source: outside,
			want: prepSource{kind: srcImage, path: outside, outside: true}},
		{name: "missing path", source: filepath.Join(work, "nope.ipsw"), refused: true},
		{name: "directory that is not an image", source: notImage, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classify(tc.source, cache)
			if tc.refused {
				var fe *driver.FixableError
				if !errors.As(err, &fe) ||
					fe.Msg != "no IPSW file or prepared image at "+tc.source ||
					fe.Fix != "give an IPSW URL, a local .ipsw file, or a prepared image directory" {
					t.Fatalf("classify(%q) = %+v, %v; want the refusal", tc.source, got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// t.TempDir may sit behind a symlink (macOS /var): compare resolved.
			if got.kind != tc.want.kind || got.outside != tc.want.outside ||
				!samePath(got.path, tc.want.path) {
				t.Fatalf("classify(%q) = %+v, want %+v", tc.source, got, tc.want)
			}
		})
	}
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

func TestPrepare(t *testing.T) {
	for _, tc := range []struct {
		name string
		// setup lays out the cache and returns the source to prepare.
		setup      func(t *testing.T, c *ImageCache) string
		latest     string
		failWith   error
		wantCalls  []string // with <cache> standing for the cache dir
		wantActive string
		wantErr    bool
	}{
		{name: "no source refreshes the active image, looking up only after",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-old", t0)
				makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
				return ""
			},
			latest:    ipsw27_2,
			wantCalls: []string{"refresh macos-new", "latest"}, wantActive: "macos-new"},
		{name: "no source and no image restores the latest",
			setup:  func(*testing.T, *ImageCache) string { return "" },
			latest: ipsw27_2,
			wantCalls: []string{"latest", "root", "download " + ipsw27_2,
				"restore <cache>/ipsw-" + shortHash(ipsw27_2) + ".ipsw -> " + keyDir(ipsw27_2)},
			wantActive: keyDir(ipsw27_2)},
		{name: "a URL not yet prepared is restored",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-old", t0)
				return ipsw27_2
			},
			wantCalls: []string{"root", "download " + ipsw27_2,
				"restore <cache>/ipsw-" + shortHash(ipsw27_2) + ".ipsw -> " + keyDir(ipsw27_2)},
			wantActive: keyDir(ipsw27_2)},
		{name: "a URL whose image sorts second is refreshed in place",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-0000000000000000", t0.Add(time.Hour))
				makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
				return ipsw27_2
			},
			wantCalls: []string{"refresh " + keyDir(ipsw27_2)}, wantActive: keyDir(ipsw27_2)},
		{name: "a prepared directory is refreshed and made active",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
				return makeImage(t, c.CacheDir, "macos-old", t0)
			},
			wantCalls: []string{"refresh macos-old"}, wantActive: "macos-old"},
		{name: "a prepared disk.img is refreshed and made active",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
				return filepath.Join(makeImage(t, c.CacheDir, "macos-old", t0), "disk.img")
			},
			wantCalls: []string{"refresh macos-old"}, wantActive: "macos-old"},
		{name: "a failed refresh leaves the active image",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
				if err := c.SetActive(filepath.Join(c.CacheDir, "macos-new")); err != nil {
					t.Fatal(err)
				}
				return makeImage(t, c.CacheDir, "macos-old", t0)
			},
			failWith:  errors.New("hdiutil failed"),
			wantCalls: []string{"refresh macos-old"}, wantActive: "macos-new", wantErr: true},
		{name: "a failed restore leaves the active image",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
				if err := c.SetActive(filepath.Join(c.CacheDir, "macos-new")); err != nil {
					t.Fatal(err)
				}
				return ipsw27_2
			},
			failWith: errors.New("restore failed"),
			wantCalls: []string{"root", "download " + ipsw27_2,
				"restore <cache>/ipsw-" + shortHash(ipsw27_2) + ".ipsw -> " + keyDir(ipsw27_2)},
			wantActive: "macos-new", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &fakeHost{latest: tc.latest, failWith: tc.failWith}
			c := h.cache(t)
			source := tc.setup(t, c)
			dir, err := c.Prepare(source, false)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Prepare(%q) = %q, %v", source, dir, err)
			}
			want := strings.Join(tc.wantCalls, "\n")
			want = strings.ReplaceAll(want, "<cache>", c.CacheDir)
			if got := strings.Join(h.calls, "\n"); got != want {
				t.Fatalf("calls:\n%s\nwant:\n%s", got, want)
			}
			if got := activeName(t, c); got != tc.wantActive {
				t.Fatalf("active = %q, want %q", got, tc.wantActive)
			}
			if !tc.wantErr && dir != filepath.Join(c.CacheDir, tc.wantActive) {
				t.Fatalf("Prepare returned %q", dir)
			}
			for _, call := range h.calls {
				if img, ok := strings.CutPrefix(call, "refresh "); ok &&
					!strings.Contains(h.stderr.String(), "Refreshing boot agent on "+
						filepath.Join(c.CacheDir, img)) {
					t.Fatalf("no refresh message: %s", h.stderr.String())
				}
			}
		})
	}
}

func TestPrepareOutsideTheCacheIsRefused(t *testing.T) {
	h := &fakeHost{}
	c := h.cache(t)
	outside := makeImage(t, t.TempDir(), "macos-out", t0)
	_, err := c.Prepare(outside, false)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || fe.Fix != customImageFix(c.CacheDir) {
		t.Fatalf("Prepare(outside) = %v", err)
	}
	if len(h.calls) != 0 || activeName(t, c) != "" {
		t.Fatalf("calls %v, active %q", h.calls, activeName(t, c))
	}
}

func TestPrepareRestoreRecordsAndStamps(t *testing.T) {
	h := &fakeHost{}
	c := h.cache(t)
	dir, err := c.Prepare(ipsw27_2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !isValidImageDir(dir) {
		t.Fatalf("%s is not a prepared image", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, linkStampName)); err != nil {
		t.Fatalf("no link stamp: %v", err)
	}
	if got := c.recordedIPSW(dir); got != ipsw27_2 {
		t.Fatalf("recorded IPSW %q", got)
	}
	if !strings.Contains(h.stderr.String(), fmt.Sprintf("macOS image ready at %s", dir)) {
		t.Fatalf("stderr: %s", h.stderr.String())
	}
}

func TestPrepareUpdate(t *testing.T) {
	t.Run("restores the latest when it is not prepared", func(t *testing.T) {
		h := &fakeHost{latest: ipsw27_2}
		c := h.cache(t)
		makeImage(t, c.CacheDir, "macos-old", t0)
		dir, err := c.Prepare("", true)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"latest", "root", "download " + ipsw27_2,
			"restore " + filepath.Join(c.CacheDir, "ipsw-"+shortHash(ipsw27_2)+".ipsw") +
				" -> " + keyDir(ipsw27_2)}
		if strings.Join(h.calls, "\n") != strings.Join(want, "\n") {
			t.Fatalf("calls %q", h.calls)
		}
		if filepath.Base(dir) != keyDir(ipsw27_2) || activeName(t, c) != keyDir(ipsw27_2) {
			t.Fatalf("dir %q, active %q", dir, activeName(t, c))
		}
	})
	t.Run("refreshes the latest in place when it is prepared", func(t *testing.T) {
		h := &fakeHost{latest: ipsw27_2}
		c := h.cache(t)
		old := makeImage(t, c.CacheDir, "macos-old", t0.Add(time.Hour))
		makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
		if err := c.SetActive(old); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Prepare("", true); err != nil {
			t.Fatal(err)
		}
		if strings.Join(h.calls, ",") != "latest,refresh "+keyDir(ipsw27_2) {
			t.Fatalf("calls %q", h.calls)
		}
		if activeName(t, c) != keyDir(ipsw27_2) {
			t.Fatalf("active %q", activeName(t, c))
		}
	})
	t.Run("with a source is refused before anything runs", func(t *testing.T) {
		h := &fakeHost{latest: ipsw27_2}
		c := h.cache(t)
		_, err := c.Prepare(ipsw27_0, true)
		var fe *driver.FixableError
		if !errors.As(err, &fe) || fe.Msg != "--update picks the newest IPSW itself" ||
			fe.Fix != "drop the source, or drop --update to prepare that source" {
			t.Fatalf("Prepare(source, update) = %v", err)
		}
		if len(h.calls) != 0 {
			t.Fatalf("calls %q", h.calls)
		}
	})
	t.Run("a failed lookup is an error", func(t *testing.T) {
		h := &fakeHost{latestErr: errors.New("offline")}
		c := h.cache(t)
		makeImage(t, c.CacheDir, "macos-old", t0)
		if _, err := c.Prepare("", true); err == nil || !strings.Contains(err.Error(), "offline") {
			t.Fatalf("Prepare(update) offline = %v", err)
		}
		if activeName(t, c) != "" {
			t.Fatalf("active %q", activeName(t, c))
		}
	})
}

// writeIPSW writes a cached IPSW file named name into the cache.
func writeIPSW(t *testing.T, c *ImageCache, name string) string {
	t.Helper()
	p := filepath.Join(c.CacheDir, name)
	if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestPrepareDeletesTheImagesIPSW(t *testing.T) {
	t.Run("after a URL restore, once recorded", func(t *testing.T) {
		h := &fakeHost{}
		c := h.cache(t)
		other := writeIPSW(t, c, "ipsw-"+shortHash(ipsw27_0)+".ipsw")
		dir, err := c.Prepare(ipsw27_2, false)
		if err != nil {
			t.Fatal(err)
		}
		if exists(filepath.Join(c.CacheDir, "ipsw-"+shortHash(ipsw27_2)+".ipsw")) {
			t.Fatal("the restored image's IPSW is still cached")
		}
		if !exists(other) {
			t.Fatal("another image's IPSW was deleted")
		}
		src, _ := c.LayerCache().Source(filepath.Join(dir, "disk.img"))
		if len(src.Inputs) != 1 || src.Inputs[0].URL != ipsw27_2 || src.Inputs[0].SHA256 == "" {
			t.Fatalf("recorded inputs %+v, want the hashed IPSW", src.Inputs)
		}
	})
	t.Run("after a refresh", func(t *testing.T) {
		h := &fakeHost{}
		c := h.cache(t)
		makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
		own := writeIPSW(t, c, "ipsw-"+shortHash(ipsw27_2)+".ipsw")
		other := writeIPSW(t, c, "ipsw-"+shortHash(ipsw27_0)+".ipsw")
		latest := writeIPSW(t, c, "ipsw-latest.ipsw")
		if _, err := c.Prepare("", false); err != nil {
			t.Fatal(err)
		}
		if exists(own) || !exists(other) || !exists(latest) {
			t.Fatalf("own %v other %v latest %v", exists(own), exists(other), exists(latest))
		}
	})
	t.Run("a failed prepare deletes nothing", func(t *testing.T) {
		h := &fakeHost{failWith: errors.New("hdiutil failed")}
		c := h.cache(t)
		makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
		own := writeIPSW(t, c, "ipsw-"+shortHash(ipsw27_2)+".ipsw")
		if _, err := c.Prepare("", false); err == nil {
			t.Fatal("Prepare succeeded")
		}
		if !exists(own) {
			t.Fatal("the IPSW was deleted after a failed prepare")
		}
	})
}

func TestOutOfDate(t *testing.T) {
	const (
		local   = "file:///Users/j/UniversalMac_27.2_26B123_Restore.ipsw"
		update  = "sudo warden image prepare --driver vz --update"
		avail   = " is available. To update (about 25 GB, 15 minutes): " + update
		already = " is already prepared. To use it (no download): " + update
	)
	for _, tc := range []struct {
		name, active, recorded, u string
		uPrepared                 bool
		want                      string // "" for no warning
	}{
		{name: "the active image is u's key", active: "/c/" + keyDir(ipsw27_2), u: ipsw27_2},
		{name: "the recorded IPSW file name is u's (a local file)",
			active: "/c/macos-1111", recorded: local, u: ipsw27_2},
		{name: "u not prepared", active: "/c/macos-90a1", recorded: ipsw27_0, u: ipsw27_2,
			want: "the active macOS image is macOS 27.0.1 (26A100); macOS 27.2 (26B123)" + avail},
		{name: "u already prepared", active: "/c/macos-90a1", recorded: ipsw27_0, u: ipsw27_2,
			uPrepared: true,
			want:      "the active macOS image is macOS 27.0.1 (26A100); macOS 27.2 (26B123)" + already},
		{name: "no recorded IPSW", active: "/c/macos-90a1", u: ipsw27_2, uPrepared: true,
			want: "the active macOS image is macos-90a1 (IPSW not recorded); " +
				"macOS 27.2 (26B123)" + already},
		{name: "names that are not Apple's", active: "/c/macos-90a1",
			recorded: "https://mirror/old.ipsw", u: "https://mirror/new.ipsw",
			want: "the active macOS image is old.ipsw; new.ipsw" + avail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := outOfDate(tc.active, tc.recorded, tc.u, tc.uPrepared)
			if ok != (tc.want != "") || got != tc.want {
				t.Fatalf("outOfDate = %q, %v\nwant %q", got, ok, tc.want)
			}
		})
	}
}

func TestOutOfDateReadsTheCache(t *testing.T) {
	c := &ImageCache{CacheDir: t.TempDir()}
	old := makeImage(t, c.CacheDir, "macos-90a1", t0)
	recordIPSW(t, c, old, ipsw27_0)
	if msg, ok := c.OutOfDate(old, ipsw27_2); !ok || !strings.Contains(msg, "is available") {
		t.Fatalf("u not prepared: %q %v", msg, ok)
	}
	makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
	if msg, ok := c.OutOfDate(old, ipsw27_2); !ok || !strings.Contains(msg, "already prepared") {
		t.Fatalf("u prepared: %q %v", msg, ok)
	}
}

func TestPrepareOutOfDateWarning(t *testing.T) {
	setupOld := func(t *testing.T, c *ImageCache) {
		old := makeImage(t, c.CacheDir, "macos-90a1", t0)
		recordIPSW(t, c, old, ipsw27_0)
	}
	t.Run("a no-source refresh warns after it succeeded", func(t *testing.T) {
		h := &fakeHost{latest: ipsw27_2}
		c := h.cache(t)
		setupOld(t, c)
		if _, err := c.Prepare("", false); err != nil {
			t.Fatal(err)
		}
		if strings.Join(h.calls, ",") != "refresh macos-90a1,latest" {
			t.Fatalf("calls %q", h.calls)
		}
		want := "warning: the active macOS image is macOS 27.0.1 (26A100); macOS 27.2 (26B123) is available."
		if !strings.Contains(h.stderr.String(), want) {
			t.Fatalf("stderr: %s", h.stderr.String())
		}
	})
	t.Run("a failed refresh does not look up", func(t *testing.T) {
		h := &fakeHost{latest: ipsw27_2, failWith: errors.New("hdiutil failed")}
		c := h.cache(t)
		setupOld(t, c)
		if _, err := c.Prepare("", false); err == nil {
			t.Fatal("Prepare succeeded")
		}
		if strings.Join(h.calls, ",") != "refresh macos-90a1" {
			t.Fatalf("calls %q", h.calls)
		}
	})
	t.Run("a failed lookup is silent", func(t *testing.T) {
		h := &fakeHost{latestErr: errors.New("offline")}
		c := h.cache(t)
		setupOld(t, c)
		if _, err := c.Prepare("", false); err != nil {
			t.Fatalf("Prepare offline = %v", err)
		}
		if strings.Contains(h.stderr.String(), "warning:") || strings.Contains(h.stderr.String(), "offline") {
			t.Fatalf("stderr: %s", h.stderr.String())
		}
	})
	for _, tc := range []struct {
		name   string
		source func(c *ImageCache) string
		update bool
		calls  string
	}{
		{name: "--update", update: true, calls: "latest,root,download " + ipsw27_2 + ",restore"},
		{name: "a URL", source: func(*ImageCache) string { return ipsw27_0 },
			calls: "root,download " + ipsw27_0 + ",restore"},
		{name: "a prepared directory",
			source: func(c *ImageCache) string { return filepath.Join(c.CacheDir, "macos-90a1") },
			calls:  "refresh macos-90a1"},
	} {
		t.Run("no lookup or warning after "+tc.name, func(t *testing.T) {
			h := &fakeHost{latest: ipsw27_2}
			c := h.cache(t)
			setupOld(t, c)
			src := ""
			if tc.source != nil {
				src = tc.source(c)
			}
			if _, err := c.Prepare(src, tc.update); err != nil {
				t.Fatal(err)
			}
			var short []string
			for _, call := range h.calls {
				if strings.HasPrefix(call, "restore ") {
					call = "restore"
				}
				short = append(short, call)
			}
			if got := strings.Join(short, ","); got != tc.calls {
				t.Fatalf("calls %q, want %q", got, tc.calls)
			}
			if strings.Contains(h.stderr.String(), "warning:") {
				t.Fatalf("stderr: %s", h.stderr.String())
			}
		})
	}
	t.Run("no second lookup or warning after a first-time restore", func(t *testing.T) {
		h := &fakeHost{latest: ipsw27_2}
		c := h.cache(t)
		if _, err := c.Prepare("", false); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(strings.Join(h.calls, ","), "latest"); n != 1 {
			t.Fatalf("calls %q", h.calls)
		}
		if strings.Contains(h.stderr.String(), "warning:") {
			t.Fatalf("stderr: %s", h.stderr.String())
		}
	})
}

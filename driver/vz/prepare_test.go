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
		for _, f := range vzImageFiles {
			p := filepath.Join(imgDir, f)
			if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
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

// trace is the recorded calls, comma-separated.
func (h *fakeHost) trace() string { return strings.Join(h.calls, ",") }

// warned reports whether anything was printed as a warning.
func (h *fakeHost) warned() bool { return strings.Contains(h.stderr.String(), "warning:") }

// restoreCall is the restore step's call for a URL source downloaded into c.
func restoreCall(c *ImageCache, url string) string {
	ipsw := filepath.Join(c.CacheDir, "ipsw-"+shortHash(url)+".ipsw")
	return "restore " + ipsw + " -> " + keyDir(url)
}

func mustPrepare(t *testing.T, c *ImageCache, source string, update bool) string {
	t.Helper()
	dir, err := c.Prepare(source, update)
	if err != nil {
		t.Fatalf("Prepare(%q, %v) = %v", source, update, err)
	}
	return dir
}

// vzImageFiles are the files the restore step writes into an image directory.
var vzImageFiles = []string{"disk.img", "aux-storage", "hardware-model", "machine-id"}

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

	const (
		fileURL   = "file:///x/y.ipsw"
		sourceFix = "give an IPSW URL, a local .ipsw file, or a prepared image directory"
	)
	for _, tc := range []struct {
		name, source string
		want         prepSource
		refused      bool
	}{
		{name: "empty", source: "", want: prepSource{kind: srcEmpty}},
		{name: "https URL", source: ipsw27_2,
			want: prepSource{kind: srcURL, path: ipsw27_2}},
		{name: "file URL", source: fileURL, want: prepSource{kind: srcURL, path: fileURL}},
		{name: "absolute IPSW file", source: ipsw,
			want: prepSource{kind: srcFile, path: ipsw}},
		{name: "relative IPSW file", source: filepath.Base(ipsw),
			want: prepSource{kind: srcFile, path: ipsw}},
		{name: "prepared directory", source: img,
			want: prepSource{kind: srcImage, path: img}},
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
				msg := "no IPSW file or prepared image at " + tc.source
				checkRefusal(t, err, msg, sourceFix)
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

// checkRefusal fails unless err is a FixableError with exactly msg and fix.
func checkRefusal(t *testing.T, err error, msg, fix string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) || fe.Msg != msg || fe.Fix != fix {
		t.Fatalf("got %#v, want the refusal %q / %q", err, msg, fix)
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

// activeNew lays out macos-new as the active image and macos-old beside it.
func activeNew(t *testing.T, c *ImageCache) (old string) {
	t.Helper()
	newer := makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
	if err := c.SetActive(newer); err != nil {
		t.Fatal(err)
	}
	return makeImage(t, c.CacheDir, "macos-old", t0)
}

func TestPrepare(t *testing.T) {
	for _, tc := range []struct {
		name string
		// setup lays out the cache and returns the source to prepare.
		setup      func(t *testing.T, c *ImageCache) string
		latest     string
		failWith   error
		wantCalls  func(c *ImageCache) []string
		wantActive string
		wantErr    bool
	}{
		{name: "no source refreshes the active image, looking up only after",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-old", t0)
				makeImage(t, c.CacheDir, "macos-new", t0.Add(time.Hour))
				return ""
			},
			latest: ipsw27_2,
			wantCalls: func(*ImageCache) []string {
				return []string{"refresh macos-new", "latest"}
			},
			wantActive: "macos-new"},
		{name: "no source and no image restores the latest",
			setup:  func(*testing.T, *ImageCache) string { return "" },
			latest: ipsw27_2,
			wantCalls: func(c *ImageCache) []string {
				return []string{"latest", "root", "download " + ipsw27_2,
					restoreCall(c, ipsw27_2)}
			},
			wantActive: keyDir(ipsw27_2)},
		{name: "a URL not yet prepared is restored",
			setup: func(t *testing.T, c *ImageCache) string {
				makeImage(t, c.CacheDir, "macos-old", t0)
				return ipsw27_2
			},
			wantCalls: func(c *ImageCache) []string {
				return []string{"root", "download " + ipsw27_2,
					restoreCall(c, ipsw27_2)}
			},
			wantActive: keyDir(ipsw27_2)},
		{name: "a URL whose image sorts second is refreshed in place",
			setup: func(t *testing.T, c *ImageCache) string {
				first := "macos-0000000000000000"
				makeImage(t, c.CacheDir, first, t0.Add(time.Hour))
				makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
				return ipsw27_2
			},
			wantCalls: func(*ImageCache) []string {
				return []string{"refresh " + keyDir(ipsw27_2)}
			},
			wantActive: keyDir(ipsw27_2)},
		{name: "a prepared directory is refreshed and made active",
			setup: activeNew,
			wantCalls: func(*ImageCache) []string {
				return []string{"refresh macos-old"}
			},
			wantActive: "macos-old"},
		{name: "a prepared disk.img is refreshed and made active",
			setup: func(t *testing.T, c *ImageCache) string {
				return filepath.Join(activeNew(t, c), "disk.img")
			},
			wantCalls: func(*ImageCache) []string {
				return []string{"refresh macos-old"}
			},
			wantActive: "macos-old"},
		{name: "a failed refresh leaves the active image",
			setup:    activeNew,
			failWith: errors.New("hdiutil failed"),
			wantCalls: func(*ImageCache) []string {
				return []string{"refresh macos-old"}
			},
			wantActive: "macos-new", wantErr: true},
		{name: "a failed restore leaves the active image",
			setup: func(t *testing.T, c *ImageCache) string {
				activeNew(t, c)
				return ipsw27_2
			},
			failWith: errors.New("restore failed"),
			wantCalls: func(c *ImageCache) []string {
				return []string{"root", "download " + ipsw27_2,
					restoreCall(c, ipsw27_2)}
			},
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
			if want := strings.Join(tc.wantCalls(c), ","); h.trace() != want {
				t.Fatalf("calls:\n%s\nwant:\n%s", h.trace(), want)
			}
			if got := activeName(t, c); got != tc.wantActive {
				t.Fatalf("active = %q, want %q", got, tc.wantActive)
			}
			if !tc.wantErr && dir != filepath.Join(c.CacheDir, tc.wantActive) {
				t.Fatalf("Prepare returned %q", dir)
			}
			checkRefreshMessages(t, h, c)
		})
	}
}

// checkRefreshMessages fails unless each refresh printed the message f6
// case 2 looks for.
func checkRefreshMessages(t *testing.T, h *fakeHost, c *ImageCache) {
	t.Helper()
	for _, call := range h.calls {
		img, ok := strings.CutPrefix(call, "refresh ")
		msg := "Refreshing boot agent on " + filepath.Join(c.CacheDir, img)
		if ok && !strings.Contains(h.stderr.String(), msg) {
			t.Fatalf("no %q: %s", msg, h.stderr.String())
		}
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
	dir := mustPrepare(t, c, ipsw27_2, false)
	if !isValidImageDir(dir) {
		t.Fatalf("%s is not a prepared image", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, linkStampName)); err != nil {
		t.Fatalf("no link stamp: %v", err)
	}
	if got := c.recordedIPSW(dir); got != ipsw27_2 {
		t.Fatalf("recorded IPSW %q", got)
	}
	if !strings.Contains(h.stderr.String(), "macOS image ready at "+dir) {
		t.Fatalf("stderr: %s", h.stderr.String())
	}
}

func TestPrepareUpdateRestoresTheLatest(t *testing.T) {
	h := &fakeHost{latest: ipsw27_2}
	c := h.cache(t)
	makeImage(t, c.CacheDir, "macos-old", t0)
	dir := mustPrepare(t, c, "", true)
	want := "latest,root,download " + ipsw27_2 + "," + restoreCall(c, ipsw27_2)
	if h.trace() != want {
		t.Fatalf("calls %q", h.calls)
	}
	if filepath.Base(dir) != keyDir(ipsw27_2) || activeName(t, c) != keyDir(ipsw27_2) {
		t.Fatalf("dir %q, active %q", dir, activeName(t, c))
	}
}

func TestPrepareUpdateRefreshesAPreparedLatest(t *testing.T) {
	h := &fakeHost{latest: ipsw27_2}
	c := h.cache(t)
	old := makeImage(t, c.CacheDir, "macos-old", t0.Add(time.Hour))
	makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
	if err := c.SetActive(old); err != nil {
		t.Fatal(err)
	}
	mustPrepare(t, c, "", true)
	if h.trace() != "latest,refresh "+keyDir(ipsw27_2) {
		t.Fatalf("calls %q", h.calls)
	}
	if activeName(t, c) != keyDir(ipsw27_2) {
		t.Fatalf("active %q", activeName(t, c))
	}
}

func TestPrepareUpdateWithASourceIsRefused(t *testing.T) {
	h := &fakeHost{latest: ipsw27_2}
	c := h.cache(t)
	_, err := c.Prepare(ipsw27_0, true)
	checkRefusal(t, err, "--update picks the newest IPSW itself",
		"drop the source, or drop --update to prepare that source")
	if len(h.calls) != 0 {
		t.Fatalf("calls %q", h.calls)
	}
}

func TestPrepareUpdateFailedLookupIsAnError(t *testing.T) {
	h := &fakeHost{latestErr: errors.New("offline")}
	c := h.cache(t)
	makeImage(t, c.CacheDir, "macos-old", t0)
	_, err := c.Prepare("", true)
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("Prepare(update) offline = %v", err)
	}
	if activeName(t, c) != "" {
		t.Fatalf("active %q", activeName(t, c))
	}
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

func cachedIPSW(c *ImageCache, url string) string {
	return filepath.Join(c.CacheDir, "ipsw-"+shortHash(url)+".ipsw")
}

func TestPrepareDeletesTheImagesIPSW(t *testing.T) {
	t.Run("after a URL restore, once recorded", func(t *testing.T) {
		h := &fakeHost{}
		c := h.cache(t)
		other := writeIPSW(t, c, filepath.Base(cachedIPSW(c, ipsw27_0)))
		dir := mustPrepare(t, c, ipsw27_2, false)
		if exists(cachedIPSW(c, ipsw27_2)) {
			t.Fatal("the restored image's IPSW is still cached")
		}
		if !exists(other) {
			t.Fatal("another image's IPSW was deleted")
		}
		src, _ := c.LayerCache().Source(filepath.Join(dir, "disk.img"))
		in := src.Inputs
		if len(in) != 1 || in[0].URL != ipsw27_2 || in[0].SHA256 == "" {
			t.Fatalf("recorded inputs %+v, want the hashed IPSW", in)
		}
	})
	t.Run("after a refresh", func(t *testing.T) {
		h := &fakeHost{}
		c := h.cache(t)
		makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
		own := writeIPSW(t, c, filepath.Base(cachedIPSW(c, ipsw27_2)))
		other := writeIPSW(t, c, filepath.Base(cachedIPSW(c, ipsw27_0)))
		latest := writeIPSW(t, c, "ipsw-latest.ipsw")
		mustPrepare(t, c, "", false)
		if exists(own) || !exists(other) || !exists(latest) {
			t.Fatalf("own %v other %v latest %v",
				exists(own), exists(other), exists(latest))
		}
	})
	t.Run("a failed prepare deletes nothing", func(t *testing.T) {
		h := &fakeHost{failWith: errors.New("hdiutil failed")}
		c := h.cache(t)
		makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
		own := writeIPSW(t, c, filepath.Base(cachedIPSW(c, ipsw27_2)))
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
		from27  = "the active macOS image is macOS 27.0.1 (26A100); macOS 27.2 (26B123)"
	)
	for _, tc := range []struct {
		name, active, recorded, u string
		uPrepared                 bool
		want                      string // "" for no warning
	}{
		{name: "the active image is u's key",
			active: "/c/" + keyDir(ipsw27_2), u: ipsw27_2},
		{name: "the recorded IPSW file name is u's (a local file)",
			active: "/c/macos-1111", recorded: local, u: ipsw27_2},
		{name: "u not prepared", active: "/c/macos-90a1", recorded: ipsw27_0, u: ipsw27_2,
			want: from27 + avail},
		{name: "u already prepared", active: "/c/macos-90a1", recorded: ipsw27_0,
			u: ipsw27_2, uPrepared: true, want: from27 + already},
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
	msg, ok := c.OutOfDate(old, ipsw27_2)
	if !ok || !strings.Contains(msg, "is available") {
		t.Fatalf("u not prepared: %q %v", msg, ok)
	}
	makeImage(t, c.CacheDir, keyDir(ipsw27_2), t0)
	msg, ok = c.OutOfDate(old, ipsw27_2)
	if !ok || !strings.Contains(msg, "already prepared") {
		t.Fatalf("u prepared: %q %v", msg, ok)
	}
}

// oldActive is a cache whose only image, macos-90a1, was restored from 27.0.1.
func oldActive(t *testing.T, h *fakeHost) *ImageCache {
	t.Helper()
	c := h.cache(t)
	recordIPSW(t, c, makeImage(t, c.CacheDir, "macos-90a1", t0), ipsw27_0)
	return c
}

func TestPrepareWarnsAfterARefresh(t *testing.T) {
	h := &fakeHost{latest: ipsw27_2}
	mustPrepare(t, oldActive(t, h), "", false)
	if h.trace() != "refresh macos-90a1,latest" {
		t.Fatalf("calls %q", h.calls)
	}
	want := "warning: the active macOS image is macOS 27.0.1 (26A100); " +
		"macOS 27.2 (26B123) is available."
	if !strings.Contains(h.stderr.String(), want) {
		t.Fatalf("stderr: %s", h.stderr.String())
	}
}

func TestPrepareFailedRefreshDoesNotLookUp(t *testing.T) {
	h := &fakeHost{latest: ipsw27_2, failWith: errors.New("hdiutil failed")}
	if _, err := oldActive(t, h).Prepare("", false); err == nil {
		t.Fatal("Prepare succeeded")
	}
	if h.trace() != "refresh macos-90a1" {
		t.Fatalf("calls %q", h.calls)
	}
}

func TestPrepareFailedLookupIsSilent(t *testing.T) {
	h := &fakeHost{latestErr: errors.New("offline")}
	mustPrepare(t, oldActive(t, h), "", false)
	if h.warned() || strings.Contains(h.stderr.String(), "offline") {
		t.Fatalf("stderr: %s", h.stderr.String())
	}
}

func TestPrepareNoWarningWithASource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source func(c *ImageCache) string
		update bool
		calls  string
	}{
		{name: "--update", update: true,
			calls: "latest,root,download " + ipsw27_2 + ",restore"},
		{name: "a URL", source: func(*ImageCache) string { return ipsw27_0 },
			calls: "root,download " + ipsw27_0 + ",restore"},
		{name: "a prepared directory",
			source: func(c *ImageCache) string {
				return filepath.Join(c.CacheDir, "macos-90a1")
			},
			calls: "refresh macos-90a1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &fakeHost{latest: ipsw27_2}
			c := oldActive(t, h)
			src := ""
			if tc.source != nil {
				src = tc.source(c)
			}
			mustPrepare(t, c, src, tc.update)
			short := make([]string, 0, len(h.calls))
			for _, call := range h.calls {
				if strings.HasPrefix(call, "restore ") {
					call = "restore"
				}
				short = append(short, call)
			}
			if got := strings.Join(short, ","); got != tc.calls || h.warned() {
				t.Fatalf("calls %q (want %q), stderr: %s",
					got, tc.calls, h.stderr.String())
			}
		})
	}
}

func TestPrepareFirstRestoreLooksUpOnce(t *testing.T) {
	h := &fakeHost{latest: ipsw27_2}
	mustPrepare(t, h.cache(t), "", false)
	if strings.Count(h.trace(), "latest") != 1 || h.warned() {
		t.Fatalf("calls %q, stderr: %s", h.calls, h.stderr.String())
	}
}

func TestPrepareLocalIPSW(t *testing.T) {
	for _, relative := range []bool{false, true} {
		t.Run(fmt.Sprintf("relative=%v", relative), func(t *testing.T) {
			checkLocalIPSW(t, relative)
		})
	}
}

// checkLocalIPSW prepares a local IPSW file given by its absolute or
// relative path, then prepares it again.
func checkLocalIPSW(t *testing.T, relative bool) {
	const (
		name    = "UniversalMac_27.2_26B123_Restore.ipsw"
		content = "a local ipsw"
	)
	h := &fakeHost{}
	c := h.cache(t)
	work := t.TempDir()
	file := filepath.Join(work, name)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	source := file
	if relative {
		t.Chdir(work)
		source = name
	}
	dir := mustPrepare(t, c, source, false)
	if want := "root,restore " + file + " -> " + keyDir(file); h.trace() != want {
		t.Fatalf("calls %q, want %q (no download, the file read in place)",
			h.trace(), want)
	}
	if filepath.Base(dir) != keyDir(file) || activeName(t, c) != keyDir(file) {
		t.Fatalf("dir %q active %q, want %s", dir, activeName(t, c), keyDir(file))
	}
	if m, _ := filepath.Glob(filepath.Join(c.CacheDir, "ipsw-*")); len(m) != 0 {
		t.Fatalf("IPSW copied into the cache: %v", m)
	}
	if !exists(file) {
		t.Fatal("the local IPSW was deleted")
	}
	src, _ := c.LayerCache().Source(filepath.Join(dir, "disk.img"))
	sum, _ := driver.HashFileSHA256(file)
	in := src.Inputs
	if len(in) != 1 || in[0].URL != "file://"+filepath.ToSlash(file) || in[0].SHA256 != sum ||
		in[0].Size != int64(len(content)) {
		t.Fatalf("recorded inputs %+v", in)
	}
	if imgs := c.Images(); len(imgs) != 1 || imgs[0].IPSW != name {
		t.Fatalf("Images = %+v", imgs)
	}

	h.calls = nil
	mustPrepare(t, c, source, false)
	if h.trace() != "refresh "+keyDir(file) {
		t.Fatalf("second prepare: calls %q, want a refresh in place", h.trace())
	}
}

func TestPrepareFileURLIsDownloaded(t *testing.T) {
	h := &fakeHost{}
	c := h.cache(t)
	const u = "file:///Volumes/x/UniversalMac_27.2_26B123_Restore.ipsw"
	dir := mustPrepare(t, c, u, false)
	if filepath.Base(dir) != keyDir(u) || len(h.calls) < 2 || h.calls[1] != "download "+u {
		t.Fatalf("dir %q calls %q", dir, h.calls)
	}
}

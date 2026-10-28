package hyperv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

// Generalized base images for the Hyper-V build guest (see
// docs/design/hyperv-windows-prepare.md). A base comes from, in order: an
// explicit --source (a URL, or a local file registered in place), the
// [vm] windows_image setting, or the registry's default URL. For Windows Server
// 2025 that default is Microsoft's evaluation fwlink, which Microsoft repoints
// at its current media, so "the latest image" means whatever it resolves to.
//
// Provenance instead of pinning: nothing is pinned in code. A download comes
// over HTTPS (the default URL must end on a Microsoft host), and the resolved
// URL, ETag, Last-Modified and the sha256 computed while downloading are
// recorded in the manifest and in every build's ledger. --sha256 is an
// optional extra check. A URL base is downloaded under the file's own name,
// so new media lands next to the base it replaces; the old base is deleted
// once no prepared layer or VM uses it.
//
// This lives outside the Windows build tag (pure I/O + HTTP, no Hyper-V calls)
// so it is testable on any platform.

// EvalImageSpec is a known build-guest base image.
type EvalImageSpec struct {
	Name    string // registry key, e.g. "windows-server-2025"
	Version string // human label
	OS      string // "windows"
	// URL is the default source; empty means the operator must supply one.
	URL string
	// Official marks a URL whose download must resolve to a Microsoft host.
	Official bool
	Note     string // licensing / acquisition guidance
}

// DefaultEvalImage is the registry image `image prepare` uses by default.
const DefaultEvalImage = "windows-server-2025"

// officialHostSuffix is where Microsoft's evaluation downloads are served from
// (the fwlink redirects to software-static.download.prss.microsoft.com).
const officialHostSuffix = ".microsoft.com"

// evalImageRegistry carries the known build-guest base images.
var evalImageRegistry = map[string]EvalImageSpec{
	"windows-server-2025": {
		Name:    "windows-server-2025",
		Version: "Windows Server 2025 Evaluation",
		OS:      "windows",
		URL: "https://go.microsoft.com/fwlink/?linkid=2293215" +
			"&clcid=0x409&culture=en-us&country=us",
		Official: true,
		Note: "180-day evaluation, never activated by warden (prepare has no " +
			"network), so each prepared layer runs on the 10-day initial grace " +
			"and is refreshed weekly. The default URL is Microsoft's Evaluation " +
			"Center link for the current Gen2 VHDX.",
	},
	"windows-server-2022": {
		Name:    "windows-server-2022",
		Version: "Windows Server 2022 Evaluation",
		OS:      "windows",
		Note: "No default URL: pass the image with --source or set " +
			"[vm] windows_image.",
	},
}

// LookupEvalImage returns the registry spec for a name, or false.
func LookupEvalImage(name string) (EvalImageSpec, bool) {
	s, ok := evalImageRegistry[name]
	return s, ok
}

// EvalImageNames returns the registry keys (for CLI help / listing).
func EvalImageNames() []string {
	names := make([]string, 0, len(evalImageRegistry))
	for k := range evalImageRegistry {
		names = append(names, k)
	}
	return names
}

// FetchOptions configures a fetch.
type FetchOptions struct {
	Name     string    // registry key (or a bare label when URL is supplied)
	URL      string    // a URL or local path; empty uses the registry URL
	SHA256   string    // optional expected hash (lowercase hex), verified
	Force    bool      // re-download even if the remote looks unchanged
	Progress io.Writer // status sink (nil = quiet)
}

// evalImagesDir is where fetched build-guest base images and their manifest live.
func evalImagesDir() string {
	return filepath.Join(hypervCacheDir(), "images")
}

// ownedBase reports whether p is a base image warden downloaded (into
// evalImagesDir), and so may delete once it is replaced and unused.
func ownedBase(p string) bool {
	return p != "" && diskKey(filepath.Dir(p)) == diskKey(evalImagesDir())
}

func evalManifestPath() string {
	return filepath.Join(evalImagesDir(), "index.json")
}

// evalManifest records fetched images and which one is the active build base.
type evalManifest struct {
	Active string                       `json:"active"`
	Images map[string]evalManifestEntry `json:"images"`
	// Prepared lists prepared layers (newest last); ActivePrepared is the path
	// of the one builds boot from.
	Prepared       []PreparedBase `json:"prepared,omitempty"`
	ActivePrepared string         `json:"active_prepared,omitempty"`
	// OldBases are replaced base files kept until no prepared layer or VM
	// uses them.
	OldBases []string `json:"old_bases,omitempty"`
}

type evalManifestEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Generation int    `json:"generation"`
	OS         string `json:"os"`
	Pinned     bool   `json:"pinned"` // verified against an expected hash
	Bytes      int64  `json:"bytes"`
	FetchedAt  string `json:"fetched_at"`
	// What the URL resolved to when fetched; a refresh compares these with a
	// HEAD request to tell whether the media changed.
	ResolvedURL  string `json:"resolved_url,omitempty"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

func loadEvalManifest() (*evalManifest, error) {
	m := &evalManifest{Images: map[string]evalManifestEntry{}}
	b, err := os.ReadFile(evalManifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, fmt.Errorf(
			"eval image manifest %s is corrupt: %w", evalManifestPath(), err)
	}
	if m.Images == nil {
		m.Images = map[string]evalManifestEntry{}
	}
	return m, nil
}

func saveEvalManifest(m *evalManifest) error {
	if err := os.MkdirAll(evalImagesDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := evalManifestPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, evalManifestPath())
}

// resolveActiveEvalImage returns the active build-guest base recorded by a prior
// fetch, or ("", ...) when none is set. It is the manifest fallback in
// resolveBuildBaseVHDX.
func resolveActiveEvalImage() (path string, isWindows bool, generation int, ok bool) {
	m, err := loadEvalManifest()
	if err != nil || m.Active == "" {
		return "", false, 0, false
	}
	e, exists := m.Images[m.Active]
	if !exists {
		return "", false, 0, false
	}
	if _, err := os.Stat(e.Path); err != nil {
		return "", false, 0, false
	}
	return e.Path, e.OS == "windows", e.Generation, true
}

// FetchEvalImage makes opts' image (or the registry default) the active
// generalized base and returns its path: a local file is registered in place;
// a URL is resolved with HEAD and downloaded unless the manifest already holds
// exactly that remote file.
func FetchEvalImage(ctx context.Context, opts FetchOptions) (string, error) {
	if opts.Name == "" {
		return "", fmt.Errorf("fetch: empty image name (try one of: %s)",
			strings.Join(EvalImageNames(), ", "))
	}
	spec, known := LookupEvalImage(opts.Name)
	if !known && opts.URL == "" {
		return "", fmt.Errorf("unknown image %q and no --source given (known images: %s)",
			opts.Name, strings.Join(EvalImageNames(), ", "))
	}
	// Before any download or hash: everything below lands in the cache.
	if err := checkDirVisible(hypervCacheDir(), "the Hyper-V cache",
		cacheVisibleFix); err != nil {
		return "", err
	}
	if isLocalImage(opts.URL) {
		return registerLocalImage(opts)
	}
	url, remote, err := resolveFetchURL(ctx, opts, spec)
	if err != nil {
		return "", err
	}
	gen, err := diskGeneration(remote.Name)
	if err != nil {
		return "", err
	}
	if path, ok, err := unchangedRemote(opts, url, remote); ok || err != nil {
		return path, err
	}
	dest := filepath.Join(evalImagesDir(), remote.Name)
	if err := os.MkdirAll(evalImagesDir(), 0o755); err != nil {
		return "", err
	}
	gotHash, n, err := downloadVerified(ctx, remote.URL, dest, opts.Progress)
	if err != nil {
		return "", err
	}
	logf(opts.Progress, "downloaded %s (%d bytes, sha256:%s)\n",
		remote.Name, n, short(gotHash))
	expected := strings.ToLower(strings.TrimSpace(opts.SHA256))
	if expected != "" && gotHash != expected {
		_ = os.Remove(dest)
		_ = os.Remove(dest + ".sha256")
		return "", fmt.Errorf("verification FAILED for %s:\n  expected sha256 %s\n"+
			"  got      sha256 %s\nthe download was removed",
			remote.Name, expected, gotHash)
	}
	return activateBase(opts, evalManifestEntry{
		Name: opts.Name, Path: dest, URL: url, SHA256: gotHash, Generation: gen,
		OS: firstNonEmpty(spec.OS, "windows"), Pinned: expected != "", Bytes: n,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		ResolvedURL: remote.URL, ETag: remote.ETag, LastModified: remote.LastModified,
	}, true)
}

// resolveFetchURL picks the URL to fetch (opts.URL, else the registry
// default) and resolves it. The registry default is only trusted when it
// lands on a Microsoft host; an explicit URL is the operator's choice.
func resolveFetchURL(
	ctx context.Context, opts FetchOptions, spec EvalImageSpec,
) (string, remoteImage, error) {
	url := firstNonEmpty(opts.URL, spec.URL)
	if url == "" {
		return "", remoteImage{}, driver.Fixable(
			"warden image prepare --driver hyperv --source <url-or-path>, "+
				"or set [vm] windows_image",
			"no default download for %q. %s", opts.Name, spec.Note)
	}
	remote, err := resolveRemote(ctx, url)
	if err != nil {
		return "", remoteImage{}, err
	}
	if spec.Official && opts.URL == "" &&
		!strings.HasSuffix(strings.ToLower(remote.Host), officialHostSuffix) {
		return "", remoteImage{}, fmt.Errorf("the default %s download resolved to %s, "+
			"not a Microsoft host; refusing it (pass --source to use another image)",
			opts.Name, remote.URL)
	}
	return url, remote, nil
}

// unchangedRemote reports whether the manifest's entry for opts.Name is
// already this exact remote file (same resolved URL, size and ETag, still on
// disk), in which case nothing is downloaded and it stays the active base.
func unchangedRemote(
	opts FetchOptions, url string, remote remoteImage,
) (string, bool, error) {
	if opts.Force {
		return "", false, nil
	}
	m, err := loadEvalManifest()
	if err != nil {
		return "", false, err
	}
	prev, ok := m.Images[opts.Name]
	if !ok || prev.ResolvedURL != remote.URL || prev.Bytes != remote.Size ||
		prev.ETag != remote.ETag {
		return "", false, nil
	}
	if _, err := os.Stat(prev.Path); err != nil {
		return "", false, nil
	}
	expected := strings.ToLower(strings.TrimSpace(opts.SHA256))
	if expected != "" && expected != prev.SHA256 {
		return "", false, fmt.Errorf("verification FAILED for %s: expected sha256 %s, "+
			"the cached download has %s", prev.Path, expected, prev.SHA256)
	}
	logf(opts.Progress, "base %s is current (%s)\n", filepath.Base(prev.Path), url)
	prev.URL = url
	prev.Pinned = prev.Pinned || expected != ""
	path, err := activateBase(opts, prev, false)
	return path, true, err
}

// activateBase records e as opts.Name's base and makes it active. A base file
// it replaces is kept in OldBases until no prepared layer or VM uses it, if
// warden downloaded it; a file the user registered in place is theirs.
// fresh reports that e.SHA256 was computed from bytes just downloaded.
func activateBase(opts FetchOptions, e evalManifestEntry, fresh bool) (string, error) {
	m, err := loadEvalManifest()
	if err != nil {
		return "", err
	}
	if prev, ok := m.Images[opts.Name]; ok && ownedBase(prev.Path) && prev.Path != e.Path {
		m.OldBases = append(m.OldBases, prev.Path)
	}
	m.Images[opts.Name] = e
	m.Active = opts.Name
	recordEvalLayer(e.Path, e.SHA256, firstNonEmpty(e.ResolvedURL, e.URL), e.Pinned,
		fresh, opts.Progress)
	if err := saveEvalManifest(m); err != nil {
		return "", err
	}
	logf(opts.Progress, "activated %q as the Hyper-V base: %s\n", opts.Name, e.Path)
	return e.Path, nil
}

// remoteImage is what a download URL resolves to.
type remoteImage struct {
	URL, Host, Name    string
	Size               int64
	ETag, LastModified string
}

// resolveRemote follows url's redirects with a HEAD request and reports the
// final location, the file name in its path, and its size and validators.
func resolveRemote(ctx context.Context, url string) (remoteImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return remoteImage{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return remoteImage{}, fmt.Errorf("resolving %s: %w", url, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return remoteImage{}, fmt.Errorf("resolving %s: HTTP %s", url, resp.Status)
	}
	final := resp.Request.URL
	name, err := downloadFileName(final.Path)
	if err != nil {
		return remoteImage{}, fmt.Errorf("resolving %s: %w (from %s)", url, err, final)
	}
	return remoteImage{
		URL: final.String(), Host: final.Hostname(), Name: name,
		Size: resp.ContentLength, ETag: resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// downloadFileName is the cache file name for a download, from the last
// element of its final (post-redirect) URL path. The server chooses that
// path, so the name must be one plain file name: a decoded `..\` or `\` would
// otherwise lead filepath.Join out of the image cache on Windows.
func downloadFileName(urlPath string) (string, error) {
	bad := fmt.Errorf("no safe file name in URL path %q", urlPath)
	name := pathpkg.Base(urlPath)
	if strings.Contains(name, "..") {
		return "", bad
	}
	if strings.HasSuffix(urlPath, "/") || name == "" || name == "." ||
		strings.ContainsAny(name, `/\:`) || !filepath.IsLocal(name) {
		return "", bad
	}
	return name, nil
}

// downloadVerified streams url to dest.part while hashing, then atomically
// renames to dest and writes a .sha256 integrity sidecar. Returns the hash and
// byte count.
func downloadVerified(
	ctx context.Context, url, dest string, progress io.Writer,
) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	logf(progress, "downloading %s\n", url)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}

	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	pw := &progressWriter{total: resp.ContentLength, out: progress}
	n, copyErr := io.Copy(io.MultiWriter(f, h, pw), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("download %s: %w", url, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", 0, closeErr
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if err := os.WriteFile(dest+".sha256", []byte(hash+"\n"), 0o644); err != nil {
		_ = os.Remove(tmp)
		return "", 0, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", 0, err
	}
	return hash, n, nil
}

// progressWriter logs download progress at ~5% (or ~256MiB) boundaries.
type progressWriter struct {
	total   int64
	written int64
	nextPct int
	nextAbs int64
	out     io.Writer
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	if w.out == nil {
		return len(p), nil
	}
	if w.total > 0 {
		pct := int(w.written * 100 / w.total)
		if pct >= w.nextPct {
			fmt.Fprintf(w.out, "  %d%% (%d/%d bytes)\n", pct, w.written, w.total)
			w.nextPct = pct - pct%5 + 5
		}
	} else if w.written >= w.nextAbs {
		fmt.Fprintf(w.out, "  %d bytes\n", w.written)
		w.nextAbs = w.written + 256<<20
	}
	return len(p), nil
}

func logf(w io.Writer, format string, args ...any) {
	if w != nil {
		fmt.Fprintf(w, format, args...)
	}
}

func short(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ActiveImage returns the build-guest base pinned as active by a prior
// `warden image prepare --driver hyperv`, if any.
func ActiveImage() (path string, ok bool) {
	p, _, _, ok := resolveActiveEvalImage()
	return p, ok
}

// isLocalImage reports whether source names an existing local file rather
// than a URL.
func isLocalImage(source string) bool {
	if source == "" || strings.Contains(source, "://") {
		return false
	}
	fi, err := os.Stat(source)
	return err == nil && fi.Mode().IsRegular()
}

// registerLocalImage records an image the operator supplied as a local file,
// in place (no copy: these are many GB), as the active base.
func registerLocalImage(opts FetchOptions) (string, error) {
	path, err := filepath.Abs(opts.URL)
	if err != nil {
		return "", err
	}
	gen, err := diskGeneration(path)
	if err != nil {
		return "", err
	}
	info, pinned, err := pinLocalImage(opts, path)
	if err != nil {
		return "", err
	}
	return activateBase(opts, evalManifestEntry{
		Name: opts.Name, Path: path, SHA256: info.SHA256, Generation: gen,
		OS: "windows", Pinned: pinned, Bytes: info.Size,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}, false)
}

// diskGeneration is the Hyper-V generation a disk's format boots as.
func diskGeneration(path string) (int, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".vhd":
		return 1, nil
	case ".vhdx":
		return 2, nil
	}
	return 0, driver.Fixable("", "%s is not a .vhd (Gen1) or .vhdx (Gen2) image", path)
}

// pinLocalImage hashes path (or reuses its cached hash) and checks it against
// the expected digest, if any. A match is recorded as pinned in the layer
// cache.
func pinLocalImage(opts FetchOptions, path string) (driver.LayerInfo, bool, error) {
	cache := LayerCache()
	cache.Progress = opts.Progress
	info, err := cache.Lookup(path, "")
	if err != nil {
		return info, false, err
	}
	expected := strings.ToLower(strings.TrimSpace(opts.SHA256))
	if expected == "" {
		return info, false, nil
	}
	if info.SHA256 != expected {
		return info, false, fmt.Errorf("pinned verification FAILED for %s:\n"+
			"  expected sha256 %s\n  got      sha256 %s", path, expected, info.SHA256)
	}
	src := ledger.LayerSource{Kind: ledger.SourceLocal, Pinned: true}
	return info, true, cache.Record(path, info.SHA256, "", src)
}

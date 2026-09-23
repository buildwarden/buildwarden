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
	"path/filepath"
	"strings"
	"time"
)

// Eval-image fetch-and-pin for the Hyper-V build guest.
//
// The primary Windows build path boots Microsoft's stock Windows Server
// Evaluation image (a Gen1 BIOS/MBR .vhd) as-is, with no image surgery. This
// file is the `warden image` side of that: it downloads a pinned image,
// verifies it against a pinned SHA256, and records it (with provenance) as the
// active build-guest base so the driver's resolveBuildBaseVHDX can find it.
//
// Security posture (this is a supply-chain tool): a fetch is only "pinned" when
// the downloaded bytes verify against an expected SHA256 (supplied on the CLI or
// carried in the registry). When no expected hash is known, the download is NOT
// silently trusted: the bytes are cached and their computed hash printed, but
// the image is refused as the active base unless the operator explicitly accepts
// it. Pinning is then a fast second call that verifies the already-cached bytes.
//
// This lives outside the Windows build tag (pure I/O + HTTP, no Hyper-V calls)
// so it is testable on any platform.

// EvalImageSpec is a known, pinnable build-guest base image.
type EvalImageSpec struct {
	Name       string // registry key, e.g. "windows-server-2025"
	Version    string // human label
	OS         string // "windows"
	Ext        string // on-disk extension, e.g. ".vhd"
	Generation int    // Hyper-V generation the image boots as (1 for the eval VHD)
	URL        string // pinned download URL; empty means the operator must supply --url
	SHA256     string // pinned expected hash (lowercase hex); empty means not yet pinned
	Note       string // licensing / acquisition guidance
}

// evalImageRegistry carries the known build-guest base images. The Windows
// Server evaluation VHD sits behind the Microsoft Evaluation Center
// (registration required) and Microsoft does not publish a stable, scrapable
// SHA256 for it, so the URL and SHA256 are intentionally left empty here: the
// operator supplies --url on first fetch, pins the printed --sha256, and a
// maintainer can then commit those pinned values into this table so a later
// `warden image fetch windows-server-2025` becomes a single command.
var evalImageRegistry = map[string]EvalImageSpec{
	"windows-server-2025": {
		Name:       "windows-server-2025",
		Version:    "Windows Server 2025 Evaluation",
		OS:         "windows",
		Ext:        ".vhd",
		Generation: 1,
		URL:        "",
		SHA256:     "",
		Note: "180-day evaluation (activate within 10 days or it auto-shuts-down). " +
			"Download the VHD from the Microsoft Evaluation Center " +
			"(https://www.microsoft.com/evalcenter/download-windows-server-2025; " +
			"free registration) and pass its URL with --url. It is a Gen1 BIOS/MBR " +
			"VHD and boots on a Generation-1 VM as-is (no conversion).",
	},
	"windows-server-2022": {
		Name:       "windows-server-2022",
		Version:    "Windows Server 2022 Evaluation",
		OS:         "windows",
		Ext:        ".vhd",
		Generation: 1,
		URL:        "",
		SHA256:     "",
		Note: "180-day evaluation. Download the VHD from the Microsoft Evaluation " +
			"Center (registration required) and pass its URL with --url. Gen1 " +
			"BIOS/MBR VHD, boots on a Generation-1 VM as-is.",
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

// FetchOptions configures a fetch-and-pin.
type FetchOptions struct {
	Name           string    // registry key (or a bare label when URL is supplied)
	URL            string    // overrides the registry URL
	SHA256         string    // overrides/supplies the expected hash (lowercase hex)
	AcceptUnpinned bool       // use the image even when no expected hash is known
	Force          bool       // re-download even if a valid cached copy exists
	Progress       io.Writer // status sink (nil = quiet)
}

// evalImagesDir is where fetched build-guest base images and their manifest live.
func evalImagesDir() string {
	return filepath.Join(hypervCacheDir(), "images")
}

func evalManifestPath() string {
	return filepath.Join(evalImagesDir(), "index.json")
}

// evalManifest records fetched images and which one is the active build base.
type evalManifest struct {
	Active string                       `json:"active"`
	Images map[string]evalManifestEntry `json:"images"`
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
		return nil, fmt.Errorf("eval image manifest %s is corrupt: %w", evalManifestPath(), err)
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

// FetchEvalImage downloads (if needed), verifies, caches, and pins a
// build-guest base image, marking it the active base on success. It returns the
// cached path.
func FetchEvalImage(ctx context.Context, opts FetchOptions) (string, error) {
	if opts.Name == "" {
		return "", fmt.Errorf("fetch: empty image name (try one of: %s)", strings.Join(EvalImageNames(), ", "))
	}
	spec, known := LookupEvalImage(opts.Name)
	if !known {
		// Allow a bare label as long as a URL is supplied.
		if opts.URL == "" {
			return "", fmt.Errorf("unknown image %q and no --url given (known images: %s)",
				opts.Name, strings.Join(EvalImageNames(), ", "))
		}
		spec = EvalImageSpec{Name: opts.Name, OS: "windows", Ext: ".vhd", Generation: 1}
	}

	url := opts.URL
	if url == "" {
		url = spec.URL
	}
	if url == "" {
		return "", fmt.Errorf(
			"no download URL for %q. %s\nThen: warden image fetch %s --url <that-url>",
			opts.Name, spec.Note, opts.Name)
	}

	expected := strings.ToLower(strings.TrimSpace(opts.SHA256))
	if expected == "" {
		expected = strings.ToLower(strings.TrimSpace(spec.SHA256))
	}

	ext := spec.Ext
	if ext == "" {
		ext = ".vhd"
	}
	if err := os.MkdirAll(evalImagesDir(), 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(evalImagesDir(), opts.Name+ext)

	// Reuse a cached copy when it still verifies (against the expected hash if
	// pinned, else its integrity sidecar), unless --force.
	if !opts.Force {
		if cachedHash, ok := cachedEvalHash(dest); ok {
			if expected != "" && cachedHash != expected {
				logf(opts.Progress, "cached %s failed pinned verification (have %s..., want %s...); re-downloading\n",
					filepath.Base(dest), short(cachedHash), short(expected))
			} else {
				logf(opts.Progress, "using cached %s (sha256:%s)\n", filepath.Base(dest), short(cachedHash))
				return finishFetch(opts, spec, dest, cachedHash, expected)
			}
		}
	}

	gotHash, n, err := downloadVerified(ctx, url, dest, opts.Progress)
	if err != nil {
		return "", err
	}
	logf(opts.Progress, "downloaded %s (%d bytes, sha256:%s)\n", filepath.Base(dest), n, short(gotHash))

	if expected != "" && gotHash != expected {
		_ = os.Remove(dest)
		_ = os.Remove(dest + ".sha256")
		return "", fmt.Errorf(
			"pinned verification FAILED for %q:\n  expected sha256 %s\n  got      sha256 %s\n"+
				"the download does not match the pinned hash; the cached file was removed",
			opts.Name, expected, gotHash)
	}
	return finishFetch(opts, spec, dest, gotHash, expected)
}

// finishFetch records the fetched image in the manifest and decides whether it
// may become the active base. A pinned (verified) image is activated; an
// unpinned one is recorded but only activated with AcceptUnpinned.
func finishFetch(opts FetchOptions, spec EvalImageSpec, dest, gotHash, expected string) (string, error) {
	pinned := expected != "" && gotHash == expected

	m, err := loadEvalManifest()
	if err != nil {
		return "", err
	}
	fi, _ := os.Stat(dest)
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	gen := spec.Generation
	if gen == 0 {
		gen = 1
	}
	osName := spec.OS
	if osName == "" {
		osName = "windows"
	}
	m.Images[opts.Name] = evalManifestEntry{
		Name:       opts.Name,
		Path:       dest,
		URL:        firstNonEmpty(opts.URL, spec.URL),
		SHA256:     gotHash,
		Generation: gen,
		OS:         osName,
		Pinned:     pinned,
		Bytes:      size,
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if !pinned && !opts.AcceptUnpinned {
		// Cache the bytes, but refuse to make unverified media the active base.
		if err := saveEvalManifest(m); err != nil {
			return "", err
		}
		return dest, fmt.Errorf(
			"UNPINNED: %q downloaded to %s but is not verified against a pinned hash.\n"+
				"  computed sha256: %s\n"+
				"To PIN it (verifies the already-cached bytes, no re-download):\n"+
				"  warden image fetch %s --sha256 %s\n"+
				"Or to use it as-is without a pinned hash:\n"+
				"  warden image fetch %s --accept-unpinned",
			opts.Name, dest, gotHash, opts.Name, gotHash, opts.Name)
	}

	m.Active = opts.Name
	if err := saveEvalManifest(m); err != nil {
		return "", err
	}
	if pinned {
		logf(opts.Progress, "pinned and activated %q as the Hyper-V build-guest base\n", opts.Name)
	} else {
		logf(opts.Progress, "activated %q as the Hyper-V build-guest base (UNPINNED - no authenticity check)\n", opts.Name)
	}
	return dest, nil
}

// downloadVerified streams url to dest.part while hashing, then atomically
// renames to dest and writes a .sha256 integrity sidecar. Returns the hash and
// byte count.
func downloadVerified(ctx context.Context, url, dest string, progress io.Writer) (string, int64, error) {
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

// cachedEvalHash returns the hash of a cached file: it prefers the .sha256
// sidecar, and (defensively) recomputes when the sidecar is missing.
func cachedEvalHash(dest string) (string, bool) {
	if _, err := os.Stat(dest); err != nil {
		return "", false
	}
	if b, err := os.ReadFile(dest + ".sha256"); err == nil {
		if s := strings.ToLower(strings.TrimSpace(string(b))); s != "" {
			return s, true
		}
	}
	f, err := os.Open(dest)
	if err != nil {
		return "", false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
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

package hyperv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

// mediaServer plays Microsoft's download endpoint: /fwlink redirects to
// /media/<name>, which serves the current bytes (GET or HEAD) with an ETag.
// setMedia swaps what the link points at; gets counts full downloads.
type mediaServer struct {
	*httptest.Server
	mu   sync.Mutex
	name string
	body []byte
	gets atomic.Int32
}

func newMediaServer(t *testing.T, name string, body []byte) *mediaServer {
	t.Helper()
	ms := &mediaServer{name: name, body: body}
	handler := func(w http.ResponseWriter, r *http.Request) {
		ms.mu.Lock()
		name, body := ms.name, ms.body
		ms.mu.Unlock()
		switch r.URL.Path {
		case "/fwlink":
			http.Redirect(w, r, "/media/"+name, http.StatusFound)
		case "/media/" + name:
			w.Header().Set("ETag", `"`+sum256(body)[:16]+`"`)
			w.Header().Set("Last-Modified", "Wed, 16 Oct 2024 15:40:30 GMT")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			if r.Method == http.MethodGet {
				ms.gets.Add(1)
				_, _ = w.Write(body)
			}
		default:
			http.NotFound(w, r)
		}
	}
	ms.Server = httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(ms.Close)
	return ms
}

func (ms *mediaServer) setMedia(name string, body []byte) {
	ms.mu.Lock()
	ms.name, ms.body = name, body
	ms.mu.Unlock()
}

// evalServer serves fixed bytes at any path (one hit per request).
func evalServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func sum256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestFetchEvalImage_ResolvesAndRecordsProvenance: a URL with no expected hash
// is downloaded under the resolved file name, activated, and recorded with
// its resolved URL and validators; nothing needs pinning.
func TestFetchEvalImage_ResolvesAndRecordsProvenance(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("server-2025-media-v1")
	ms := newMediaServer(t, "26100.1742_serverdatacentereval.vhdx", body)

	path, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025", URL: ms.URL + "/fwlink",
	})
	if err != nil {
		t.Fatalf("FetchEvalImage: %v", err)
	}
	if filepath.Base(path) != "26100.1742_serverdatacentereval.vhdx" {
		t.Errorf("downloaded to %s, want the resolved file name", path)
	}
	p, isWin, gen, ok := resolveActiveEvalImage()
	if !ok || p != path || !isWin || gen != 2 {
		t.Fatalf("active = %q win=%v gen=%d ok=%v", p, isWin, gen, ok)
	}
	m, _ := loadEvalManifest()
	e := m.Images["windows-server-2025"]
	if e.Pinned || e.SHA256 != sum256(body) || e.ETag == "" ||
		!strings.HasSuffix(e.ResolvedURL, "/media/26100.1742_serverdatacentereval.vhdx") ||
		e.URL != ms.URL+"/fwlink" {
		t.Errorf("manifest entry = %+v", e)
	}
	src, ok := LayerCache().Source(path)
	if !ok || src.Kind != ledger.SourceURL || src.URL != e.ResolvedURL {
		t.Errorf("layer source = %+v, want the resolved URL", src)
	}
}

func TestFetchEvalImage_UnchangedRemoteIsNotDownloadedAgain(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	ms := newMediaServer(t, "media-v1.vhdx", []byte("v1"))
	opts := FetchOptions{Name: "windows-server-2025", URL: ms.URL + "/fwlink"}
	if _, err := FetchEvalImage(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := FetchEvalImage(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := ms.gets.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1 (the second fetch only resolves)", got)
	}
	opts.Force = true
	if _, err := FetchEvalImage(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := ms.gets.Load(); got != 2 {
		t.Errorf("downloads = %d after --force, want 2", got)
	}
}

// TestFetchEvalImage_NewMediaLandsBesideOldBase: when the link moves to new
// media, it is downloaded next to the old base, which is kept in OldBases.
func TestFetchEvalImage_NewMediaLandsBesideOldBase(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	ms := newMediaServer(t, "media-v1.vhdx", []byte("v1"))
	opts := FetchOptions{Name: "windows-server-2025", URL: ms.URL + "/fwlink"}
	v1, err := FetchEvalImage(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	ms.setMedia("media-v2.vhdx", []byte("v2-newer"))
	v2, err := FetchEvalImage(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 || filepath.Base(v2) != "media-v2.vhdx" {
		t.Fatalf("v1=%s v2=%s, want a new file beside the old", v1, v2)
	}
	if _, err := os.Stat(v1); err != nil {
		t.Errorf("old base removed early: %v", err)
	}
	m, _ := loadEvalManifest()
	if len(m.OldBases) != 1 || m.OldBases[0] != v1 || m.Images[m.Active].Path != v2 {
		t.Errorf("manifest = active %s, old %v", m.Images[m.Active].Path, m.OldBases)
	}
}

func TestFetchEvalImage_ExpectedHash(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("v1")
	ms := newMediaServer(t, "media-v1.vhdx", body)
	_, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025", URL: ms.URL + "/fwlink", SHA256: sum256([]byte("x")),
	})
	if err == nil || !strings.Contains(err.Error(), "verification FAILED") {
		t.Fatalf("mismatch: err = %v", err)
	}
	if _, _, _, ok := resolveActiveEvalImage(); ok {
		t.Error("a mismatched download must not become the active base")
	}
	if _, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025", URL: ms.URL + "/fwlink", SHA256: sum256(body),
	}); err != nil {
		t.Fatal(err)
	}
	m, _ := loadEvalManifest()
	if !m.Images["windows-server-2025"].Pinned {
		t.Error("a matching --sha256 should record the base as pinned")
	}
}

// TestFetchEvalImage_DefaultURLMustBeMicrosoft: the registry default is only
// trusted when it resolves to a Microsoft host.
func TestFetchEvalImage_DefaultURLMustBeMicrosoft(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	ms := newMediaServer(t, "media.vhdx", []byte("v1"))
	orig := evalImageRegistry["windows-server-2025"]
	t.Cleanup(func() { evalImageRegistry["windows-server-2025"] = orig })
	spec := orig
	spec.URL = ms.URL + "/fwlink" // resolves to 127.0.0.1, not microsoft.com
	evalImageRegistry["windows-server-2025"] = spec

	_, err := FetchEvalImage(context.Background(), FetchOptions{Name: "windows-server-2025"})
	if err == nil || !strings.Contains(err.Error(), "not a Microsoft host") {
		t.Fatalf("err = %v, want the non-Microsoft refusal", err)
	}
	// The same URL passed explicitly is the operator's choice.
	if _, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025", URL: ms.URL + "/fwlink",
	}); err != nil {
		t.Fatalf("explicit URL: %v", err)
	}
}

func TestFetchEvalImage_DefaultRegistryURL(t *testing.T) {
	spec, ok := LookupEvalImage("windows-server-2025")
	if !ok || !spec.Official ||
		!strings.HasPrefix(spec.URL, "https://go.microsoft.com/fwlink/") {
		t.Fatalf("windows-server-2025 spec = %+v", spec)
	}
}

func TestFetchEvalImage_UnknownNameNeedsURL(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	if _, err := FetchEvalImage(
		context.Background(), FetchOptions{Name: "no-such-image"}); err == nil {
		t.Fatal("expected an error for an unknown image with no --source")
	}
	if _, err := FetchEvalImage(
		context.Background(), FetchOptions{Name: "windows-server-2022"}); err == nil ||
		!strings.Contains(err.Error(), "no default download") {
		t.Fatalf("2022 has no default URL: err = %v", err)
	}
}

func TestRegisterLocalImageActivatesInPlace(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	local := filepath.Join(t.TempDir(), "server2025.vhdx")
	if err := os.WriteFile(local, []byte("local-media"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025", URL: local,
	})
	if err != nil || path != local {
		t.Fatalf("register = %q, %v", path, err)
	}
	m, _ := loadEvalManifest()
	if e := m.Images["windows-server-2025"]; e.URL != "" || e.Pinned || m.Active == "" {
		t.Errorf("local entry = %+v active=%q", e, m.Active)
	}
}

func TestResolveBuildBaseVHDX_ManifestFallback(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	t.Setenv("WARDEN_HYPERV_BUILD_IMAGE", "")
	t.Setenv("WARDEN_HYPERV_BUILD_GEN", "")
	body := []byte("fake-vhd-fallback")
	srv, _ := evalServer(t, body)

	fetched, err := FetchEvalImage(context.Background(), FetchOptions{
		Name:   "windows-server-2025",
		URL:    srv.URL + "/server2025.vhd",
		SHA256: sum256(body),
	})
	if err != nil {
		t.Fatalf("FetchEvalImage: %v", err)
	}

	// No --image, no env, pinned but not prepared: builds boot only prepared
	// images, so resolution asks for a prepare.
	_, _, _, err = resolveBuildBaseVHDX(&driver.BuildRequest{})
	if err == nil || !strings.Contains(err.Error(), "not been prepared") {
		t.Fatalf("unprepared base: err = %v, want a prepare hint", err)
	}

	// Once prepared, the chain is the base then the prepared layer.
	layer := activateTestLayer(t)
	chain, isWin, gen, err := resolveBuildBaseVHDX(&driver.BuildRequest{})
	if err != nil {
		t.Fatalf("resolveBuildBaseVHDX prepared: %v", err)
	}
	if len(chain) != 2 || chain[0] != fetched || chain[1] != layer {
		t.Errorf("chain = %q, want [%q %q]", chain, fetched, layer)
	}
	if !isWin || gen != 1 {
		t.Errorf("resolved isWindows=%v gen=%d, want true,1", isWin, gen)
	}

	// A moved base breaks the chain: say so instead of booting a broken disk.
	if err := os.Rename(fetched, fetched+".moved"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveBuildBaseVHDX(&driver.BuildRequest{}); err == nil ||
		!strings.Contains(err.Error(), "chain is incomplete") {
		t.Fatalf("moved base: err = %v", err)
	}
}

// activateTestLayer records a stand-in prepared layer over the active base and
// makes it active.
func activateTestLayer(t *testing.T) string {
	t.Helper()
	layer := filepath.Join(t.TempDir(), "prepared.vhd")
	if err := os.WriteFile(layer, []byte("layer"), 0o444); err != nil {
		t.Fatal(err)
	}
	m, err := loadEvalManifest()
	if err != nil {
		t.Fatal(err)
	}
	m.Prepared = []PreparedBase{{Path: layer, Base: "windows-server-2025", Generation: 1}}
	m.ActivePrepared = layer
	if err := saveEvalManifest(m); err != nil {
		t.Fatal(err)
	}
	return layer
}

// The download's file name comes from the server's final URL; anything but a
// single plain name is refused, so a redirect cannot place it outside the cache.
func TestDownloadFileName(t *testing.T) {
	good := map[string]string{
		"/26100.1742.amd64fre_serverdatacentereval_en-us.vhdx": "26100.1742.amd64fre_" +
			"serverdatacentereval_en-us.vhdx",
		"/dir/image.vhd": "image.vhd",
	}
	for in, want := range good {
		if got, err := downloadFileName(in); err != nil || got != want {
			t.Errorf("downloadFileName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "/", "/dir/", "/..", `/..\..\evil.vhdx`, `/x\evil.vhdx`, "/c:evil.vhdx",
		"/a..b.vhdx",
	} {
		if got, err := downloadFileName(in); err == nil {
			t.Errorf("downloadFileName(%q) = %q, want an error", in, got)
		}
	}
}

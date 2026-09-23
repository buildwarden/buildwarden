package hyperv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// evalServer serves fixed bytes as a fake image and counts requests, so a test
// can assert whether a fetch re-downloaded or used the cache.
func evalServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", itoa(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func sum256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestFetchEvalImage_PinnedSuccess(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("fake-vhd-bytes-pinned")
	srv, _ := evalServer(t, body)

	path, err := FetchEvalImage(context.Background(), FetchOptions{
		Name:   "windows-server-2025",
		URL:    srv.URL + "/server2025.vhd",
		SHA256: sum256(body),
	})
	if err != nil {
		t.Fatalf("FetchEvalImage: %v", err)
	}
	if path == "" {
		t.Fatal("empty path")
	}
	// Active + pinned in the manifest, and resolvable.
	p, isWin, gen, ok := resolveActiveEvalImage()
	if !ok || p != path {
		t.Fatalf("resolveActiveEvalImage = %q,%v want %q,true", p, ok, path)
	}
	if !isWin || gen != 1 {
		t.Errorf("active image isWindows=%v gen=%d, want true,1", isWin, gen)
	}
	m, _ := loadEvalManifest()
	if !m.Images["windows-server-2025"].Pinned {
		t.Error("image should be marked Pinned after a matching hash")
	}
}

func TestFetchEvalImage_PinnedMismatch(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("fake-vhd-bytes")
	srv, _ := evalServer(t, body)

	_, err := FetchEvalImage(context.Background(), FetchOptions{
		Name:   "windows-server-2025",
		URL:    srv.URL + "/server2025.vhd",
		SHA256: sum256([]byte("something-else")),
	})
	if err == nil {
		t.Fatal("expected a pinned-verification failure")
	}
	// Must not have activated a mismatched image.
	if _, _, _, ok := resolveActiveEvalImage(); ok {
		t.Error("a hash-mismatched image must not become the active base")
	}
}

func TestFetchEvalImage_UnpinnedRefusedThenPinsFromCache(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("fake-vhd-bytes-unpinned")
	srv, hits := evalServer(t, body)

	// No expected hash and no --accept: cached but refused as active.
	_, err := FetchEvalImage(context.Background(), FetchOptions{
		Name: "windows-server-2025",
		URL:  srv.URL + "/server2025.vhd",
	})
	if err == nil {
		t.Fatal("expected an UNPINNED refusal without --accept-unpinned")
	}
	if _, _, _, ok := resolveActiveEvalImage(); ok {
		t.Error("unpinned image must not be active before it is pinned")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}

	// Pin it with the computed hash: should verify the CACHED bytes (no
	// re-download) and activate.
	if _, err := FetchEvalImage(context.Background(), FetchOptions{
		Name:   "windows-server-2025",
		URL:    srv.URL + "/server2025.vhd",
		SHA256: sum256(body),
	}); err != nil {
		t.Fatalf("pin from cache: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d after pin, want still 1 (should reuse cache)", got)
	}
	if _, _, _, ok := resolveActiveEvalImage(); !ok {
		t.Error("image should be active after pinning")
	}
}

func TestFetchEvalImage_AcceptUnpinned(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	body := []byte("fake-vhd-bytes-accepted")
	srv, _ := evalServer(t, body)

	if _, err := FetchEvalImage(context.Background(), FetchOptions{
		Name:           "windows-server-2025",
		URL:            srv.URL + "/server2025.vhd",
		AcceptUnpinned: true,
	}); err != nil {
		t.Fatalf("accept-unpinned fetch: %v", err)
	}
	m, _ := loadEvalManifest()
	e := m.Images["windows-server-2025"]
	if e.Pinned {
		t.Error("accept-unpinned image must be recorded as NOT pinned")
	}
	if m.Active != "windows-server-2025" {
		t.Error("accept-unpinned image should be active")
	}
}

func TestFetchEvalImage_UnknownNameNeedsURL(t *testing.T) {
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", t.TempDir())
	if _, err := FetchEvalImage(context.Background(), FetchOptions{Name: "no-such-image"}); err == nil {
		t.Fatal("expected an error for an unknown image with no --url")
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

	// No --image, no env: resolveBuildBaseVHDX should discover the pinned image.
	path, isWin, gen, err := resolveBuildBaseVHDX(&driver.BuildRequest{})
	if err != nil {
		t.Fatalf("resolveBuildBaseVHDX fallback: %v", err)
	}
	if path != fetched {
		t.Errorf("resolved %q, want %q", path, fetched)
	}
	if !isWin || gen != 1 {
		t.Errorf("resolved isWindows=%v gen=%d, want true,1", isWin, gen)
	}
}

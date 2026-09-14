package collector

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, cfg Config) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg.OutputDir = dir
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(c.Handler())
	t.Cleanup(srv.Close)
	return srv, dir
}

func put(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func TestArtifactRoundTrip(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "sekret"})
	want := bytes.Repeat([]byte("pytorch-wheel"), 100000) // ~1.3 MB
	resp := put(t, srv.URL+"/v1/artifacts/wheels/torch.whl", "sekret", want)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, err := os.ReadFile(filepath.Join(dir, "artifacts", "wheels", "torch.whl"))
	if err != nil {
		t.Fatalf("read landed artifact: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("landed artifact differs (%d vs %d bytes)", len(got), len(want))
	}
}

func TestLedgerRoundTrip(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "t"})
	resp := put(t, srv.URL+"/v1/ledger", "t", []byte("LEDGERDATA"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "ledger"))
	if string(got) != "LEDGERDATA" {
		t.Fatalf("ledger = %q", got)
	}
}

func TestAuthRequired(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "sekret"})
	// Wrong token.
	resp := put(t, srv.URL+"/v1/artifacts/x.bin", "wrong", []byte("data"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", resp.StatusCode)
	}
	// No token.
	resp = put(t, srv.URL+"/v1/artifacts/x.bin", "", []byte("data"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, "artifacts", "x.bin")); !os.IsNotExist(err) {
		t.Fatal("artifact was written despite failed auth")
	}
}

func TestPathSafety(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "t"})

	// Names my policy rejects outright (they reach the handler uncleaned).
	for _, name := range []string{".hidden", "a/.hidden/b", "bad$char"} {
		resp := put(t, srv.URL+"/v1/artifacts/"+name, "t", []byte("x"))
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("name %q: status = %d, want 400", name, resp.StatusCode)
		}
	}

	// Traversal attempts must never write outside the artifacts dir. The client
	// and ServeMux clean dot-segments (yielding 404) and isSafeRelPath is
	// defense in depth; either way nothing escapes.
	put(t, srv.URL+"/v1/artifacts/../evil", "t", []byte("x")).Body.Close()
	put(t, srv.URL+"/v1/artifacts/a/../../evil", "t", []byte("x")).Body.Close()
	parent := filepath.Dir(dir)
	for _, p := range []string{
		filepath.Join(dir, "evil"),
		filepath.Join(parent, "evil"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("traversal wrote a file at %s", p)
		}
	}
}

func TestSizeCap(t *testing.T) {
	srv, _ := newTestServer(t, Config{Token: "t", MaxArtifactBytes: 1024})
	resp := put(t, srv.URL+"/v1/artifacts/big.bin", "t", bytes.Repeat([]byte("x"), 4096))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

func TestNoAuthWhenTokenEmpty(t *testing.T) {
	srv, dir := newTestServer(t, Config{}) // no token => auth disabled
	resp := put(t, srv.URL+"/v1/artifacts/ok.bin", "", []byte("hello"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "artifacts", "ok.bin"))
	if string(got) != "hello" {
		t.Fatalf("artifact = %q", got)
	}
}

func TestHealthz(t *testing.T) {
	srv, _ := newTestServer(t, Config{Token: "t"})
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("get healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "ok") {
		t.Fatalf("healthz body = %q", b)
	}
}

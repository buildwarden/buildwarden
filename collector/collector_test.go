package collector

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	t.Cleanup(func() { _ = c.Close() }) // after srv.Close; frees the dir on Windows
	srv := httptest.NewServer(c.Handler())
	t.Cleanup(srv.Close)
	return srv, dir
}

// newTestCollector is newTestServer but returns the Collector itself.
func newTestCollector(t *testing.T, cfg Config) (*httptest.Server, *Collector) {
	t.Helper()
	cfg.OutputDir = t.TempDir()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() }) // after srv.Close; frees the dir on Windows
	srv := httptest.NewServer(c.Handler())
	t.Cleanup(srv.Close)
	return srv, c
}

// TestOutputTeeForwardsAndPersists verifies that a configured OutputTee receives
// a copy of POST /v1/output while build-output.log is still written.
func TestOutputTeeForwardsAndPersists(t *testing.T) {
	var tee bytes.Buffer
	srv, dir := newTestServer(t, Config{Token: "t", OutputTee: &tee})

	payload := []byte("hello from build.ps1\nnumpy 2.5.3 OK\n")
	req, err := http.NewRequest(
		http.MethodPost, srv.URL+"/v1/output", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Tee got a copy.
	if got := tee.String(); got != string(payload) {
		t.Errorf("tee = %q, want %q", got, payload)
	}
	// File persisted too.
	onDisk, err := os.ReadFile(filepath.Join(dir, "build-output.log"))
	if err != nil {
		t.Fatalf("read build-output.log: %v", err)
	}
	if string(onDisk) != string(payload) {
		t.Errorf("build-output.log = %q, want %q", onDisk, payload)
	}
}

// TestOutputNoTeePersists verifies output is still captured when no tee is set
// (the --quiet path).
func TestOutputNoTeePersists(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "t"}) // OutputTee nil

	payload := []byte("quiet-mode build output\n")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/output", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "build-output.log"))
	if err != nil {
		t.Fatalf("read build-output.log: %v", err)
	}
	if string(onDisk) != string(payload) {
		t.Errorf("build-output.log = %q, want %q", onDisk, payload)
	}
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

func TestEnvironmentRoundTrip(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "t"})
	want := []byte(`{"type":"vm-disk-chain/v1"}`)
	resp := put(t, srv.URL+"/v1/environment", "t", want)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, _ := os.ReadFile(filepath.Join(dir, EnvironmentFile))
	if !bytes.Equal(got, want) {
		t.Fatalf("environment = %q, want %q", got, want)
	}

	resp = put(t, srv.URL+"/v1/environment", "t",
		bytes.Repeat([]byte("x"), MaxEnvironmentBytes+1))
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: status = %d, want 413", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, EnvironmentFile)); !os.IsNotExist(err) {
		t.Fatalf("oversize payload left %s behind (err=%v)", EnvironmentFile, err)
	}

	resp = put(t, srv.URL+"/v1/environment", "", want)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", resp.StatusCode)
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

func getReq(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
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

func TestBuildScriptServed(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "build.ps1")
	want := []byte("Write-Host 'building'\r\nexit 0\r\n")
	if err := os.WriteFile(scriptPath, want, 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	srv, _ := newTestServer(t, Config{Token: "sekret", BuildScriptPath: scriptPath})

	// Wrong/absent token is rejected.
	resp := getReq(t, srv.URL+"/v1/build-script", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}

	// Correct token returns the exact staged bytes.
	resp = getReq(t, srv.URL+"/v1/build-script", "sekret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("served script = %q, want %q", got, want)
	}
}

func TestBuildScriptUnsetIs404(t *testing.T) {
	srv, _ := newTestServer(t, Config{Token: "sekret"}) // no BuildScriptPath
	resp := getReq(t, srv.URL+"/v1/build-script", "sekret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when no script staged", resp.StatusCode)
	}
}

func TestBootstrapServed(t *testing.T) {
	want := []byte(`{"version":1,"trust":{"mode":"exclusive"}}`)
	srv, _ := newTestServer(t, Config{Token: "sekret", Bootstrap: want})
	resp := getReq(t, srv.URL+"/v1/bootstrap", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}
	resp = getReq(t, srv.URL+"/v1/bootstrap", "sekret")
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, want) {
		t.Fatalf("got %d %q, want 200 %q", resp.StatusCode, got, want)
	}

	srv, _ = newTestServer(t, Config{Token: "sekret"})
	resp = getReq(t, srv.URL+"/v1/bootstrap", "sekret")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unset status = %d, want 404", resp.StatusCode)
	}
}

// putWithTrailer streams body as a chunked PUT carrying the hash trailer, the
// way the relay uploads captures.
func putWithTrailer(t *testing.T, url, token string, body []byte, hash string) *http.Response {
	t.Helper()
	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodPut, url, pr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Trailer = http.Header{HashTrailer: nil}
	go func() {
		_, _ = pw.Write(body)
		req.Trailer.Set(HashTrailer, hash)
		_ = pw.Close()
	}()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestCaptureContentAddressed(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "t"})
	hash := strings.Repeat("ab", 32)
	body := []byte("captured body")
	for _, name := range []string{"001-GET-example.com-a", "002-GET-example.com-b?x=1"} {
		resp := putWithTrailer(
			t, srv.URL+"/v1/captures/"+url.PathEscape(name), "t", body, hash)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d", name, resp.StatusCode)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "payloads", hash))
	if err != nil || string(got) != string(body) {
		t.Fatalf("payload = %q, %v", got, err)
	}
	for _, name := range []string{"001-GET-example.com-a", "002-GET-example.com-b_x_1"} {
		got, err := os.ReadFile(filepath.Join(dir, "captures", name))
		if err != nil || string(got) != string(body) {
			t.Fatalf("capture %s = %q, %v", name, got, err)
		}
	}
}

func TestCaptureRequiresHashTrailer(t *testing.T) {
	srv, dir := newTestServer(t, Config{Token: "t"})
	resp := putWithTrailer(t, srv.URL+"/v1/captures/x", "t", []byte("b"), "nothex")
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("capture without a valid hash trailer was accepted")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "payloads")); len(entries) != 0 {
		t.Fatalf("rejected capture left %d payload files", len(entries))
	}
}

func TestContextFileAndListing(t *testing.T) {
	ctxDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(ctxDir, "src", "pkg"), 0o755)
	_ = os.WriteFile(filepath.Join(ctxDir, "src", "a.txt"), []byte("A"), 0o644)
	_ = os.WriteFile(filepath.Join(ctxDir, "src", "pkg", "b.txt"), []byte("B"), 0o644)
	srv, _ := newTestServer(t, Config{Token: "t", ContextDir: ctxDir})

	get := func(p string) (*http.Response, string) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/context/"+p, nil)
		req.Header.Set("Authorization", "Bearer t")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	resp, body := get("src/a.txt")
	if resp.StatusCode != 200 || body != "A" || resp.Header.Get(ContextKindHeader) != "file" {
		t.Fatalf("file: %d %q %q", resp.StatusCode, body,
			resp.Header.Get(ContextKindHeader))
	}
	resp, body = get("src/")
	if resp.Header.Get(ContextKindHeader) != "list" || body != "src/a.txt\nsrc/pkg/b.txt\n" {
		t.Fatalf("list: %q", body)
	}
	resp, body = get("src")
	if resp.Header.Get(ContextKindHeader) != "list" || body != "src/a.txt\nsrc/pkg/b.txt\n" {
		t.Fatalf("dir without slash: %q", body)
	}
	if resp, _ = get("src/missing"); resp.StatusCode != 404 {
		t.Fatalf("missing: %d", resp.StatusCode)
	}
	if resp, _ = get("src/../../etc/passwd"); resp.StatusCode == 200 {
		t.Fatal("traversal served")
	}
}

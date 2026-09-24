package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newScriptRelay builds a minimal Relay with a working ledger for exercising
// handleBuildScript without standing up the full listener stack.
func newScriptRelay(t *testing.T, cfg Config) *Relay {
	t.Helper()
	l, err := NewLedger(LedgerConfig{
		Writer:      io.Discard,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	return &Relay{cfg: cfg, contextDir: cfg.ContextDir, ledger: l}
}

// The share-backed path (qemu/vz/container): the script is read from ContextDir
// at BuildScriptPath and served verbatim.
func TestHandleBuildScript_FromContextDir(t *testing.T) {
	dir := t.TempDir()
	want := []byte("#!/bin/sh\necho building\n")
	if err := os.WriteFile(filepath.Join(dir, "build.sh"), want, 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	r := newScriptRelay(t, Config{ContextDir: dir, BuildScriptPath: "build.sh"})

	req := httptest.NewRequest(http.MethodGet, "http://artifacts/build-script", nil)
	_, resp := r.handleBuildScript(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, want) {
		t.Fatalf("served %q, want %q", got, want)
	}
}

// An empty BuildScriptPath defaults to build.sh; a missing file 404s rather
// than panicking.
func TestHandleBuildScript_MissingIs404(t *testing.T) {
	r := newScriptRelay(t, Config{ContextDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodGet, "http://artifacts/build-script", nil)
	_, resp := r.handleBuildScript(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for absent script", resp.StatusCode)
	}
}

// The diskless path (Hyper-V): when OutputSinkURL is set the script is pulled
// from the collector on demand with the bearer token, and re-served.
func TestHandleBuildScript_FromCollector(t *testing.T) {
	want := []byte("Write-Host building\r\nexit 0\r\n")
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get("Authorization")
		gotPath = req.URL.Path
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	r := newScriptRelay(t, Config{OutputSinkURL: srv.URL, OutputSinkToken: "tok-xyz"})
	req := httptest.NewRequest(http.MethodGet, "http://artifacts/build-script", nil)
	_, resp := r.handleBuildScript(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, want) {
		t.Fatalf("served %q, want %q", got, want)
	}
	if gotPath != "/v1/build-script" {
		t.Errorf("collector path = %q, want /v1/build-script", gotPath)
	}
	if gotAuth != "Bearer tok-xyz" {
		t.Errorf("collector auth = %q, want Bearer tok-xyz", gotAuth)
	}
}

package relaycfg

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponder_ServesConfigOnceAsEnvLines(t *testing.T) {
	r, err := New("http://192.168.240.1:8300", "tok-abc123")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	// First GET returns the config.
	resp, err := http.Get(srv.URL + "/config")
	if err != nil {
		t.Fatalf("GET /config: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Parse it exactly the way the relay init does: KEY=VALUE lines, known keys.
	got := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if ok {
			got[k] = v
		}
	}
	if got["OUTPUT_SINK_URL"] != "http://192.168.240.1:8300" {
		t.Errorf("OUTPUT_SINK_URL = %q", got["OUTPUT_SINK_URL"])
	}
	if got["OUTPUT_SINK_TOKEN"] != "tok-abc123" {
		t.Errorf("OUTPUT_SINK_TOKEN = %q", got["OUTPUT_SINK_TOKEN"])
	}
	if !r.Served() {
		t.Error("Served() = false after a successful GET")
	}

	// Second GET is refused (serve-once).
	resp2, err := http.Get(srv.URL + "/config")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusGone {
		t.Errorf("second GET status = %d, want 410", resp2.StatusCode)
	}
}

func TestResponder_MethodNotAllowed(t *testing.T) {
	r, err := New("http://c/", "t")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/config", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("POST /config: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", resp.StatusCode)
	}
	if r.Served() {
		t.Error("a rejected POST must not mark the config served")
	}
}

func TestNew_RejectsInjectionAndEmptyURL(t *testing.T) {
	if _, err := New("http://c/\nEVIL=1", "t"); err == nil {
		t.Error("expected error for newline in sink URL")
	}
	if _, err := New("http://c/", "tok\r\nEVIL=1"); err == nil {
		t.Error("expected error for CRLF in token")
	}
	if _, err := New("", "t"); err == nil {
		t.Error("expected error for empty sink URL")
	}
}

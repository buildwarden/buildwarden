package relay

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/ledger"
)

// startTestRelay wires a relay to a real in-process collector, with the
// guest-facing listeners on loopback, and returns a client that reaches the
// relay's HTTP listener for any host name (as the build guest would).
func startTestRelay(
	t *testing.T, cfg collector.Config, capture string,
) (*Relay, *collector.Collector, *http.Client, string) {
	t.Helper()
	out := t.TempDir()
	cfg.OutputDir = out
	cfg.Token = "tok"
	col, err := collector.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(col.Handler())
	t.Cleanup(srv.Close)

	httpLn, _ := net.Listen("tcp", "127.0.0.1:0")
	httpsLn, _ := net.Listen("tcp", "127.0.0.1:0")
	ctrlLn, _ := net.Listen("tcp", "127.0.0.1:0")
	dnsConn, _ := net.ListenPacket("udp", "127.0.0.1:0")

	r, err := Start(Config{
		CollectorURL:    srv.URL,
		CollectorToken:  "tok",
		CaptureMode:     capture,
		SelfIP:          net.ParseIP("127.0.0.1"),
		UpstreamDNS:     "127.0.0.1:1",
		DNSPacketConn:   dnsConn,
		HTTPListener:    httpLn,
		HTTPSListener:   httpsLn,
		ControlListener: ctrlLn,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(r.Stop) // ends the ledger stream before the collector closes
	select {
	case <-col.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("relay never posted ready")
	}
	addr := httpLn.Addr().String()
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		},
	}}
	return r, col, client, out
}

func get(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// getExpect does a GET and fails unless the status and body both match.
func getExpect(t *testing.T, c *http.Client, url string, wantCode int, wantBody string) {
	t.Helper()
	if code, body := get(t, c, url); code != wantCode || body != wantBody {
		t.Fatalf("GET %s = %d %q, want %d %q", url, code, body, wantCode, wantBody)
	}
}

// getExpectCode does a GET and fails unless the status matches.
func getExpectCode(t *testing.T, c *http.Client, url string, wantCode int) {
	t.Helper()
	if code, _ := get(t, c, url); code != wantCode {
		t.Fatalf("GET %s = %d, want %d", url, code, wantCode)
	}
}

// postArtifact POSTs an artifact and fails unless the relay accepts it.
func postArtifact(t *testing.T, c *http.Client, url, body string) {
	t.Helper()
	resp, err := c.Post(url, "application/octet-stream", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("artifact POST %s: %d", url, resp.StatusCode)
	}
}

// waitHeartbeat blocks until the collector has seen a forwarded heartbeat.
func waitHeartbeat(t *testing.T, col *collector.Collector) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for col.LastHeartbeat().IsZero() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if col.LastHeartbeat().IsZero() {
		t.Fatal("heartbeat never forwarded to the collector")
	}
}

// verifyLedgerLanded checks the artifact and the flushed, verifiable ledger.
func verifyLedgerLanded(t *testing.T, out string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, "artifacts", "dist", "app.bin"))
	if err != nil || string(b) != "ARTIFACT" {
		t.Fatalf("artifact landed = %q, %v", b, err)
	}
	data, err := os.ReadFile(filepath.Join(out, "ledger"))
	if err != nil {
		t.Fatalf("ledger not flushed before completion: %v", err)
	}
	if _, err := ledger.Verify(data); err != nil {
		t.Fatalf("ledger does not verify: %v", err)
	}
}

func TestRelayCollectorEndToEnd(t *testing.T) {
	ctxDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(ctxDir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(ctxDir, "src", "main.c"), []byte("int main(){}"), 0o644)
	script := filepath.Join(t.TempDir(), "build.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nmake\n"), 0o755)

	r, col, client, out := startTestRelay(t, collector.Config{
		ContextDir: ctxDir, BuildScriptPath: script,
	}, "")
	if r.LedgerPublicKeyHex() == "" || len(r.CACert()) == 0 {
		t.Fatal("relay identity incomplete")
	}

	getExpect(t, client, "http://artifacts/build-script", 200, "#!/bin/sh\nmake\n")
	getExpect(t, client, "http://cwd/src/main.c", 200, "int main(){}")
	getExpect(t, client, "http://cwd/src/", 200, "src/main.c\n")
	getExpectCode(t, client, "http://cwd/src/missing", 404)

	postArtifact(t, client, "http://artifacts/dist/app.bin", "ARTIFACT")

	getExpectCode(t, client, "http://artifacts/heartbeat", 200)
	waitHeartbeat(t, col)

	getExpectCode(t, client, "http://artifacts/exit?code=0", 200)
	sig, err := col.Wait(context.Background(), 0)
	if err != nil || sig.ExitCode != 0 {
		t.Fatalf("completion = %+v, %v", sig, err)
	}

	verifyLedgerLanded(t, out)
}

func TestRelayCapturesToCollector(t *testing.T) {
	r, _, _, out := startTestRelay(t, collector.Config{}, "headers")
	r.savePayloadBytes(
		[]byte("HTTP/1.1 200 OK\r\n\r\n"), "001-GET-example.com", "response-headers")
	entries, err := os.ReadDir(filepath.Join(out, "captures"))
	if err != nil || len(entries) != 1 ||
		entries[0].Name() != "001-GET-example.com.response-headers" {
		t.Fatalf("captures = %v, %v", entries, err)
	}
	if p, _ := os.ReadDir(filepath.Join(out, "payloads")); len(p) != 1 {
		t.Fatalf("payloads = %d, want 1", len(p))
	}

	// A streamed body capture lands under the same content address.
	body := io.NopCloser(strings.NewReader("streamed body"))
	rc := r.newCapturingReadCloser(body, "002-GET-example.com", "")
	if b, _ := io.ReadAll(rc); string(b) != "streamed body" {
		t.Fatalf("client read %q", b)
	}
	_ = rc.Close()
	got, err := os.ReadFile(filepath.Join(out, "captures", "002-GET-example.com"))
	if err != nil || string(got) != "streamed body" {
		t.Fatalf("streamed capture = %q, %v", got, err)
	}
}

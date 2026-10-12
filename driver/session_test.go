package driver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/relay"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// fakeRelay runs the real relay behind the real claim protocol, the way
// cmd/relay does, with its guest listeners on loopback.
func fakeRelay(t *testing.T, claimToken string) (claimAddr, guestHTTP string) {
	t.Helper()
	claimLn, _ := net.Listen("tcp", "127.0.0.1:0")
	httpLn, _ := net.Listen("tcp", "127.0.0.1:0")
	httpsLn, _ := net.Listen("tcp", "127.0.0.1:0")
	ctrlLn, _ := net.Listen("tcp", "127.0.0.1:0")
	dns, _ := net.ListenPacket("udp", "127.0.0.1:0")
	go func() {
		_ = relaycfg.Serve(claimLn, claimToken, func(c relaycfg.Config, rt http.RoundTripper) (relaycfg.Identity, error) {
			r, err := relay.Start(relay.Config{
				CollectorURL: c.CollectorURL, CollectorToken: c.CollectorToken, CollectorTransport: rt,
				CaptureMode: c.CaptureMode, SelfIP: net.ParseIP("127.0.0.1"),
				UpstreamDNS: "127.0.0.1:1", DNSPacketConn: dns,
				HTTPListener: httpLn, HTTPSListener: httpsLn, ControlListener: ctrlLn,
			})
			if err != nil {
				return relaycfg.Identity{}, err
			}
			return relaycfg.Identity{CACert: string(r.CACert()), LedgerPublicKey: r.LedgerPublicKeyHex()}, nil
		})
	}()
	return claimLn.Addr().String(), httpLn.Addr().String()
}

func guestClient(addr string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		},
	}}
}

func TestSessionClaimAndWait(t *testing.T) {
	out := t.TempDir()
	s, err := StartSession(SessionConfig{OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	claimAddr, guest := fakeRelay(t, "claim-secret")
	id, err := s.ClaimRelay(context.Background(), claimAddr, "claim-secret",
		&BuildRequest{UpstreamSystemCA: true}, nil)
	if err != nil {
		t.Fatalf("ClaimRelay: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "ca.cert.pem")); string(b) != id.CACert || !strings.Contains(id.CACert, "CERTIFICATE") {
		t.Fatal("ca.cert.pem not written from the claim identity")
	}

	c := guestClient(guest)
	resp, err := c.Get("http://artifacts/exit?code=7")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = s.Wait(ctx)
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 7 {
		t.Fatalf("Wait = %v, want *ExitError with code 7", err)
	}
	if _, err := os.Stat(filepath.Join(out, "ledger")); err != nil {
		t.Fatalf("ledger missing after completion: %v", err)
	}

	FinalizeOutput(out, true)
	for _, n := range []string{"ledger.zst", "ca.cert.pem.zst"} {
		if _, err := os.Stat(filepath.Join(out, n)); err != nil {
			t.Fatalf("%s missing after FinalizeOutput: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "ledger")); err == nil {
		t.Fatal("uncompressed ledger left behind")
	}
}

func TestStageBuildScriptPrecedence(t *testing.T) {
	work := t.TempDir()
	ctx := t.TempDir()
	_ = os.WriteFile(filepath.Join(ctx, "build.sh"), []byte("from-context"), 0o755)
	_ = os.WriteFile(filepath.Join(ctx, "build.ps1"), []byte("from-context-ps"), 0o644)
	explicit := filepath.Join(t.TempDir(), "x.sh")
	_ = os.WriteFile(explicit, []byte("explicit"), 0o755)
	df := filepath.Join(t.TempDir(), "Dockerfile")
	_ = os.WriteFile(df, []byte("FROM alpine\nRUN echo translated\n"), 0o644)

	read := func(req *BuildRequest) string {
		p, err := StageBuildScript(req, work)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		return string(b)
	}
	if got := read(&BuildRequest{Script: explicit, ContextDir: ctx, Containerfile: df}); got != "explicit" {
		t.Fatalf("explicit script: %q", got)
	}
	if got := read(&BuildRequest{ContextDir: ctx, Containerfile: df}); !strings.Contains(got, "translated") {
		t.Fatalf("containerfile translation: %q", got)
	}
	if got := read(&BuildRequest{ContextDir: ctx}); got != "from-context" {
		t.Fatalf("context build.sh: %q", got)
	}
	if got := read(&BuildRequest{ContextDir: ctx, GuestOS: "windows", Containerfile: df}); got != "from-context-ps" {
		t.Fatalf("windows ignores containerfile, uses build.ps1: %q", got)
	}
	if got := read(&BuildRequest{GuestOS: "windows"}); got != "exit 0\r\n" {
		t.Fatalf("windows no-op: %q", got)
	}
}

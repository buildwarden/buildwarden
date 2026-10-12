package relaycfg

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestTunnelCarriesCollectorTraffic opens a tunnel, claims over it, and has the
// "relay" issue requests through the tunnel transport to a handler served on
// the orchestrator side, as the real relay does to its collector.
func TestTunnelCarriesCollectorTraffic(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	rtCh := make(chan http.RoundTripper, 1)
	go func() {
		_ = Serve(ln, "tok", func(c Config, rt http.RoundTripper) (Identity, error) {
			if c.CollectorURL != TunnelURL {
				return Identity{}, errors.New("expected the tunnel URL")
			}
			rtCh <- rt
			return Identity{CACert: "PEM"}, nil
		})
	}()

	var gotBody string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = r.Method + " " + r.URL.Path + " " + string(b)
		_, _ = io.WriteString(w, "pong")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, done, err := OpenTunnel(ctx, addr, "tok", handler)
	if err != nil {
		t.Fatalf("OpenTunnel: %v", err)
	}
	if _, err := Claim(ctx, addr, "tok", Config{CollectorURL: TunnelURL}); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	rt := <-rtCh
	if rt == nil {
		t.Fatal("relay got no tunnel transport")
	}
	client := &http.Client{Transport: rt}
	resp, err := client.Post(TunnelURL+"/v1/output", "text/plain", strings.NewReader("ping"))
	if err != nil {
		t.Fatalf("request over tunnel: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "pong" || gotBody != "POST /v1/output ping" {
		t.Fatalf("tunnel round trip: resp %q, handler saw %q", b, gotBody)
	}

	conn.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("done not closed after the tunnel conn closed")
	}
}

func TestTunnelURLWithoutTunnelRejected(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	go func() {
		_ = Serve(ln, "", func(Config, http.RoundTripper) (Identity, error) {
			return Identity{CACert: "PEM"}, nil
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Claim(
		ctx, addr, "", Config{CollectorURL: TunnelURL},
	); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
}

func TestTunnelWrongTokenRejected(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	go func() {
		_ = Serve(ln, "right", func(Config, http.RoundTripper) (Identity, error) {
			return Identity{CACert: "PEM"}, nil
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := OpenTunnel(
		ctx, addr, "wrong", http.NotFoundHandler(),
	); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
}

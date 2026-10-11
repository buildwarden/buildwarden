package relaycfg

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestClaimRoundTrip(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	f := false
	want := Config{
		CollectorURL:     "http://10.0.2.2:9000",
		CollectorToken:   "ctok",
		CaptureMode:      "bodies",
		UpstreamCACerts:  []string{"-----BEGIN CERTIFICATE-----\n"},
		UpstreamSystemCA: &f,
		Environment:      &Environment{Payload: []byte("{}"), Metadata: []byte{0xa0}},
	}
	got := make(chan Config, 1)
	served := make(chan error, 1)
	go func() {
		served <- Serve(ln, "claim-tok", func(c Config, _ http.RoundTripper) (Identity, error) {
			got <- c
			return Identity{CACert: "PEM", LedgerPublicKey: "abcd"}, nil
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := Claim(ctx, addr, "claim-tok", want)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if id.CACert != "PEM" || id.LedgerPublicKey != "abcd" {
		t.Fatalf("identity = %+v", id)
	}
	c := <-got
	if c.CollectorURL != want.CollectorURL || c.CaptureMode != "bodies" ||
		c.UpstreamSystemCA == nil || *c.UpstreamSystemCA ||
		string(c.Environment.Payload) != "{}" {
		t.Fatalf("relay received %+v", c)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve: %v", err)
	}
	// The listener is closed after one claim.
	if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("claim listener still accepting after a successful claim")
	}
}

func TestClaimWrongTokenRejectedWithoutConsuming(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	go func() {
		_ = Serve(ln, "right", func(Config, http.RoundTripper) (Identity, error) {
			return Identity{CACert: "PEM"}, nil
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Claim(ctx, addr, "wrong", Config{CollectorURL: "http://x"}); !errors.Is(err, ErrRejected) {
		t.Fatalf("wrong token: err = %v, want ErrRejected", err)
	}
	if _, err := Claim(ctx, addr, "right", Config{CollectorURL: "http://x"}); err != nil {
		t.Fatalf("right token after a refused claim: %v", err)
	}
}

func TestClaimRetriesUntilRelayListens(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	ln.Close() // nothing listening yet
	go func() {
		time.Sleep(600 * time.Millisecond)
		ln2, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		_ = Serve(ln2, "", func(Config, http.RoundTripper) (Identity, error) { return Identity{CACert: "PEM"}, nil })
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Claim(ctx, addr, "", Config{CollectorURL: "http://x"}); err != nil {
		t.Fatalf("Claim did not retry until the relay came up: %v", err)
	}
}

func TestClaimRequiresCollectorURL(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	go func() {
		_ = Serve(ln, "", func(Config, http.RoundTripper) (Identity, error) { return Identity{CACert: "PEM"}, nil })
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Claim(ctx, addr, "", Config{}); !errors.Is(err, ErrRejected) {
		t.Fatalf("empty collector URL: err = %v, want ErrRejected", err)
	}
	if _, err := Claim(ctx, addr, "", Config{CollectorURL: "http://x"}); err != nil {
		t.Fatalf("valid claim after an invalid one: %v", err)
	}
}

func TestStartFailureReported(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	served := make(chan error, 1)
	go func() {
		served <- Serve(ln, "", func(Config, http.RoundTripper) (Identity, error) { return Identity{}, errors.New("boom") })
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Claim(ctx, addr, "", Config{CollectorURL: "http://x"}); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
	if err := <-served; err == nil || err.Error() != "boom" {
		t.Fatalf("Serve err = %v, want boom", err)
	}
}

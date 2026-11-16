package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

func TestParseBuildLink(t *testing.T) {
	tests := []struct {
		in   string
		want *relaycfg.BuildLink
		err  string
	}{
		{"", nil, ""},
		{"100.127.255.252/30", &relaycfg.BuildLink{Prefix: "100.127.255.252/30",
			Relay: "100.127.255.253", Guest: "100.127.255.254"}, ""},
		{"192.0.2.0/24", &relaycfg.BuildLink{Prefix: "192.0.2.0/24",
			Relay: "192.0.2.1", Guest: "192.0.2.2"}, ""},
		{"192.0.2.1/24", nil, "did you mean 192.0.2.0/24"},
		{"192.0.2.0/31", nil, "length"},
		{"127.0.0.0/30", nil, "loopback"},
		{"nonsense", nil, "netip.ParsePrefix"},
	}
	for _, tt := range tests {
		got, err := parseBuildLink(tt.in)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%q: err = %v, want %q", tt.in, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tt.in, err)
			continue
		}
		if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
			t.Errorf("%q = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

// --upstream-ca is read by warden's rule: only certificates are returned,
// the skips and each path's line are printed with the source --upstream-ca,
// and a path that gives nothing is the error.
func TestReadUpstreamCAs(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1),
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	dir := t.TempDir()
	good := filepath.Join(dir, "corp.pem")
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("SECRET")})
	data := append(append([]byte{}, cert...), keyBlock...)
	if err := os.WriteFile(good, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	got, err := readUpstreamCAs(&out, good)
	if err != nil || !bytes.Equal(got, cert) {
		t.Fatalf("got %q, %v", got, err)
	}
	want := "collector: warning: upstream CA " + good + ": block 2 is a PRIVATE KEY and " +
		"is not sent to the relay; a private key does not belong in a file of trust " +
		"roots\n" +
		"collector: upstream CA " + good + " (--upstream-ca): 1 certificate to send; 1 " +
		"block skipped\n"
	if out.String() != want {
		t.Errorf("printed:\n%s\nwant:\n%s", out.String(), want)
	}

	missing := filepath.Join(dir, "gone.pem")
	out.Reset()
	_, err = readUpstreamCAs(&out, good+","+missing)
	var fe *driver.FixableError
	if !errors.As(err, &fe) ||
		fe.Msg != "upstream CA path "+missing+" (from --upstream-ca) does not exist" {
		t.Errorf("error %v", err)
	}
	if got, err := readUpstreamCAs(&out, ""); got != nil || err != nil {
		t.Errorf("an empty list read something: %q, %v", got, err)
	}
}

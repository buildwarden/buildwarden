//go:build linux

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// TestSubjectHashMatchesOpenSSL pins subjectHash to `openssl x509 -hash` for
// subjects created with `openssl req -subj` (OpenSSL 3.5).
func TestSubjectHashMatchesOpenSSL(t *testing.T) {
	for _, tc := range []struct {
		name pkix.Name
		want string
	}{
		{pkix.Name{Organization: []string{"W"}, CommonName: "warden test CA"}, "01c21f41"},
		{pkix.Name{CommonName: "  BuildWarden   Relay CA "}, "281e2dfe"},
	} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: tc.name,
			NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		got, err := subjectHash(cert)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("subjectHash(%q) = %s, want %s", tc.name.String(), got, tc.want)
		}
	}
}

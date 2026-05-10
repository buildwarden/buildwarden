package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestCanonicalString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BuildWarden", "buildwarden"},
		{"  Hello   World  ", " hello world "},
		{"NoChange", "nochange"},
		{"Mixed\tWhitespace\n Here", "mixed whitespace here"},
		{"Ünïcödé", "Ünïcödé"},
		{"", ""},
	}
	for _, tt := range tests {
		got := canonicalString(tt.input)
		if got != tt.want {
			t.Errorf("canonicalString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCertSubjectHash(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Test CA",
			Organization: []string{"BuildWarden"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	caCertPEM = pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: certDER,
	})

	hash, err := certSubjectHash()
	if err != nil {
		t.Fatalf("certSubjectHash: %v", err)
	}
	if len(hash) != 8 {
		t.Errorf("hash length = %d, want 8", len(hash))
	}
	for _, c := range hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("hash contains non-hex char %q", string(c))
		}
	}
}

func TestCertSubjectHash_InvalidPEM(t *testing.T) {
	caCertPEM = []byte("not a pem")
	_, err := certSubjectHash()
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

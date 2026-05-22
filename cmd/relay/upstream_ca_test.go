package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testCertPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl,
		&key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestBuildUpstreamCertPool_NoBundleFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELAY_SYSTEM_CA", "true")

	pool, err := buildUpstreamCertPool(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pool != nil {
		t.Fatal("expected nil pool when no bundle and system CAs enabled")
	}
}

func TestBuildUpstreamCertPool_ValidBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELAY_SYSTEM_CA", "true")

	certPEM := testCertPEM(t)
	if err := os.WriteFile(
		filepath.Join(dir, "upstream-ca-bundle.pem"), certPEM, 0644,
	); err != nil {
		t.Fatal(err)
	}

	pool, err := buildUpstreamCertPool(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
}

func TestBuildUpstreamCertPool_InvalidBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELAY_SYSTEM_CA", "true")

	if err := os.WriteFile(
		filepath.Join(dir, "upstream-ca-bundle.pem"),
		[]byte("not a cert"), 0644,
	); err != nil {
		t.Fatal(err)
	}

	_, err := buildUpstreamCertPool(dir)
	if err == nil {
		t.Fatal("expected error for invalid bundle")
	}
}

func TestBuildUpstreamCertPool_SystemDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELAY_SYSTEM_CA", "false")

	certPEM := testCertPEM(t)
	if err := os.WriteFile(
		filepath.Join(dir, "upstream-ca-bundle.pem"), certPEM, 0644,
	); err != nil {
		t.Fatal(err)
	}

	pool, err := buildUpstreamCertPool(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
}

func TestBuildUpstreamCertPool_SystemDisabledNoBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELAY_SYSTEM_CA", "false")

	pool, err := buildUpstreamCertPool(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		t.Fatal("expected non-nil (empty) pool, not nil")
	}
}

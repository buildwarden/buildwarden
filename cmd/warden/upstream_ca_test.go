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

func generateTestCert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template,
		&key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestPrepareUpstreamCACerts_NoOp(t *testing.T) {
	dir := t.TempDir()
	err := prepareUpstreamCACerts(dir, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(dir, "upstream-ca-bundle.pem"))
	if !os.IsNotExist(err) {
		t.Fatal("expected no bundle file when no paths and system CA enabled")
	}
}

func TestPrepareUpstreamCACerts_SingleFile(t *testing.T) {
	ledgerDir := t.TempDir()
	certDir := t.TempDir()
	certPEM := generateTestCert(t)
	certPath := filepath.Join(certDir, "ca.pem")
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatal(err)
	}

	err := prepareUpstreamCACerts(ledgerDir, []string{certPath}, true)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := os.ReadFile(filepath.Join(ledgerDir, "upstream-ca-bundle.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if countPEMCerts(bundle) != 1 {
		t.Fatalf("expected 1 cert in bundle, got %d", countPEMCerts(bundle))
	}
}

func TestPrepareUpstreamCACerts_MultipleCertsInOneFile(t *testing.T) {
	ledgerDir := t.TempDir()
	certDir := t.TempDir()
	cert1 := generateTestCert(t)
	cert2 := generateTestCert(t)
	combined := append(cert1, cert2...)
	certPath := filepath.Join(certDir, "bundle.pem")
	if err := os.WriteFile(certPath, combined, 0644); err != nil {
		t.Fatal(err)
	}

	err := prepareUpstreamCACerts(ledgerDir, []string{certPath}, true)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := os.ReadFile(filepath.Join(ledgerDir, "upstream-ca-bundle.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if countPEMCerts(bundle) != 2 {
		t.Fatalf("expected 2 certs in bundle, got %d", countPEMCerts(bundle))
	}
}

func TestPrepareUpstreamCACerts_Directory(t *testing.T) {
	ledgerDir := t.TempDir()
	certDir := t.TempDir()

	for _, f := range []struct {
		name string
		data []byte
	}{
		{"a.pem", generateTestCert(t)},
		{"b.crt", generateTestCert(t)},
		{"c.txt", []byte("not a cert")},
	} {
		if err := os.WriteFile(
			filepath.Join(certDir, f.name), f.data, 0644,
		); err != nil {
			t.Fatal(err)
		}
	}

	err := prepareUpstreamCACerts(ledgerDir, []string{certDir}, true)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := os.ReadFile(filepath.Join(ledgerDir, "upstream-ca-bundle.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if countPEMCerts(bundle) != 2 {
		t.Fatalf("expected 2 certs (skipping .txt), got %d", countPEMCerts(bundle))
	}
}

func TestPrepareUpstreamCACerts_InvalidPEM(t *testing.T) {
	ledgerDir := t.TempDir()
	certDir := t.TempDir()
	certPath := filepath.Join(certDir, "bad.pem")
	if err := os.WriteFile(certPath, []byte("not valid PEM"), 0644); err != nil {
		t.Fatal(err)
	}

	err := prepareUpstreamCACerts(ledgerDir, []string{certPath}, true)
	if err == nil {
		t.Fatal("expected error for invalid PEM file")
	}
}

func TestPrepareUpstreamCACerts_MissingPath(t *testing.T) {
	ledgerDir := t.TempDir()
	err := prepareUpstreamCACerts(ledgerDir, []string{"/nonexistent/path"}, true)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestPrepareUpstreamCACerts_MultipleFiles(t *testing.T) {
	ledgerDir := t.TempDir()
	certDir := t.TempDir()
	path1 := filepath.Join(certDir, "one.pem")
	path2 := filepath.Join(certDir, "two.pem")
	if err := os.WriteFile(path1, generateTestCert(t), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, generateTestCert(t), 0644); err != nil {
		t.Fatal(err)
	}

	err := prepareUpstreamCACerts(ledgerDir, []string{path1, path2}, true)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := os.ReadFile(filepath.Join(ledgerDir, "upstream-ca-bundle.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if countPEMCerts(bundle) != 2 {
		t.Fatalf("expected 2 certs, got %d", countPEMCerts(bundle))
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got := expandHome("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Fatalf("expandHome(~/foo/bar) = %q, want %q", got, want)
	}

	got = expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Fatalf("expandHome should not modify absolute paths, got %q", got)
	}
}

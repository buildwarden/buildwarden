package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pavlo-v-chernykh/keystore-go/v4"
)

func testCA(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "warden test CA", Organization: []string{"W"},
		},
		NotBefore:             time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		NotAfter:              time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// TestWriteJKSReadable checks the keystore against an independent reader, which
// also verifies the integrity digest under the password.
func TestWriteJKSReadable(t *testing.T) {
	cert := testCA(t)
	data, err := writeJKS("warden", cert, jksPassword)
	if err != nil {
		t.Fatal(err)
	}
	ks := keystore.New()
	if err := ks.Load(bytes.NewReader(data), []byte(jksPassword)); err != nil {
		t.Fatalf("load: %v", err)
	}
	e, err := ks.GetTrustedCertificateEntry("warden")
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if !bytes.Equal(e.Certificate.Content, cert.Raw) || e.Certificate.Type != "X.509" {
		t.Errorf("entry = type %q, %d bytes",
			e.Certificate.Type, len(e.Certificate.Content))
	}
	if !e.CreationTime.Equal(cert.NotBefore) {
		t.Errorf("creation time %v, want NotBefore %v", e.CreationTime, cert.NotBefore)
	}
	if err := keystore.New().Load(bytes.NewReader(data), []byte("wrong")); err == nil {
		t.Error("loaded with the wrong password: digest not checked")
	}

	again, _ := writeJKS("warden", cert, jksPassword)
	if !bytes.Equal(data, again) {
		t.Error("output is not deterministic")
	}
}

// TestWriteJKSMatchesKeytool compares the bytes with what the JDK's keytool
// writes for the same certificate. It runs wherever keytool is on PATH (CI
// installs a JDK). keytool stamps the entry with the current time, so ours is
// written with keytool's timestamp; every other byte must match.
func TestWriteJKSMatchesKeytool(t *testing.T) {
	keytool, err := exec.LookPath("keytool")
	if err != nil {
		t.Skip("keytool not on PATH")
	}
	cert := testCA(t)
	dir := t.TempDir()
	pemPath := filepath.Join(dir, "ca.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(pemPath, pemData, 0o644); err != nil {
		t.Fatal(err)
	}
	ref := filepath.Join(dir, "ref.jks")
	out, err := exec.Command(keytool, "-importcert", "-noprompt", "-storetype", "JKS",
		"-alias", "warden", "-file", pemPath, "-keystore", ref,
		"-storepass", jksPassword).CombinedOutput()
	if err != nil {
		t.Fatalf("keytool -importcert: %v\n%s", err, out)
	}
	want, err := os.ReadFile(ref)
	if err != nil {
		t.Fatal(err)
	}

	// The timestamp follows magic, version, count, tag and the alias.
	off := 16 + 2 + len("warden")
	stamped := *cert
	stamped.NotBefore = time.UnixMilli(int64(binary.BigEndian.Uint64(want[off : off+8])))
	got, err := writeJKS("warden", &stamped, jksPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bytes differ from keytool's:\n got %x\nwant %x", got, want)
	}

	ours := filepath.Join(dir, "ours.jks")
	if err := os.WriteFile(ours, got, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(keytool, "-list", "-storetype", "JKS",
		"-keystore", ours, "-storepass", jksPassword).CombinedOutput(); err != nil {
		t.Fatalf("keytool -list rejects ours: %v\n%s", err, out)
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/relay"
)

func TestLoadUpstreamCA_NoBundle(t *testing.T) {
	dir := t.TempDir()
	cfg := relay.Config{}
	if err := loadUpstreamCA(dir, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamCACerts) != 0 {
		t.Errorf("expected no certs, got %d", len(cfg.UpstreamCACerts))
	}
}

func TestLoadUpstreamCA_WithBundle(t *testing.T) {
	dir := t.TempDir()
	pem := []byte(`-----BEGIN CERTIFICATE-----
MIIBdTCCARugAwIBAgIRANBsmgQg/V11RYExT5zR5CAwCgYIKoZIzj0EAwIwEjEQ
MA4GA1UEChMHQWNtZSBDbzAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBa
MBIxEDAOBgNVBAoTB0FjbWUgQ28wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
o0IwQDAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQU
AAAAAAAAAAAAAAAAAAAAAAAAAAAAMAoGCCqGSM49BAMCA0gAMEUCIQDAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAIgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
-----END CERTIFICATE-----
`)
	if err := os.WriteFile(
		filepath.Join(dir, "upstream-ca-bundle.pem"), pem, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := relay.Config{}
	if err := loadUpstreamCA(dir, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamCACerts) != 1 {
		t.Fatalf("expected 1 cert bundle, got %d", len(cfg.UpstreamCACerts))
	}
	if len(cfg.UpstreamCACerts[0]) == 0 {
		t.Error("cert bundle is empty")
	}
}

func TestLoadUpstreamCA_SystemCADisabled(t *testing.T) {
	dir := t.TempDir()
	pem := []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(
		filepath.Join(dir, "upstream-ca-bundle.pem"), pem, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RELAY_SYSTEM_CA", "false")
	cfg := relay.Config{}
	if err := loadUpstreamCA(dir, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.UpstreamSystemCA == nil {
		t.Fatal("UpstreamSystemCA should be set")
	}
	if *cfg.UpstreamSystemCA {
		t.Error("UpstreamSystemCA should be false")
	}
}

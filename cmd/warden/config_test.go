package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath_Directory(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM alpine"), 0644); err != nil {
		t.Fatal(err)
	}

	dockerfile, contextDir, err := ResolvePath(dir)
	if err != nil {
		t.Fatalf("ResolvePath(%q): %v", dir, err)
	}
	if dockerfile != df {
		t.Errorf("dockerfile = %q, want %q", dockerfile, df)
	}
	if contextDir != dir {
		t.Errorf("contextDir = %q, want %q", contextDir, dir)
	}
}

func TestResolvePath_File(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile.prod")
	if err := os.WriteFile(df, []byte("FROM alpine"), 0644); err != nil {
		t.Fatal(err)
	}

	dockerfile, contextDir, err := ResolvePath(df)
	if err != nil {
		t.Fatalf("ResolvePath(%q): %v", df, err)
	}
	if dockerfile != df {
		t.Errorf("dockerfile = %q, want %q", dockerfile, df)
	}
	if contextDir != dir {
		t.Errorf("contextDir = %q, want %q", contextDir, dir)
	}
}

func TestResolvePath_Empty(t *testing.T) {
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	df := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(df, []byte("FROM alpine"), 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	dockerfile, contextDir, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath(\"\"): %v", err)
	}
	if dockerfile != df {
		t.Errorf("dockerfile = %q, want %q", dockerfile, df)
	}
	if contextDir != dir {
		t.Errorf("contextDir = %q, want %q", contextDir, dir)
	}
}

func TestResolvePath_Containerfile(t *testing.T) {
	dir := t.TempDir()
	cf := filepath.Join(dir, "Containerfile")
	if err := os.WriteFile(cf, []byte("FROM alpine"), 0644); err != nil {
		t.Fatal(err)
	}

	dockerfile, _, err := ResolvePath(dir)
	if err != nil {
		t.Fatalf("ResolvePath(%q): %v", dir, err)
	}
	if dockerfile != cf {
		t.Errorf("dockerfile = %q, want %q (Containerfile)", dockerfile, cf)
	}
}

func TestResolvePath_NoDockerfile(t *testing.T) {
	dir := t.TempDir()

	_, _, err := ResolvePath(dir)
	if err == nil {
		t.Fatal("expected error for dir with no Dockerfile")
	}
}

func TestResolvePath_NonExistent(t *testing.T) {
	_, _, err := ResolvePath("/nonexistent/path/xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Output.Color != "auto" {
		t.Errorf("color = %q, want auto", cfg.Output.Color)
	}
	if cfg.Runtime.CLI != "" {
		t.Errorf("runtime = %q, want empty", cfg.Runtime.CLI)
	}
}

func TestLoadConfig_ProjectFile(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `[runtime]
cli = "podman"

[output]
verbose = true
`
	err := os.WriteFile(
		filepath.Join(dir, "warden.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Runtime.CLI != "podman" {
		t.Errorf("runtime = %q, want podman", cfg.Runtime.CLI)
	}
	if !cfg.Output.Verbose {
		t.Error("verbose = false, want true")
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	t.Setenv("WARDEN_CTR_CLI", "docker")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("WARDEN_VERBOSE", "1")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Runtime.CLI != "docker" {
		t.Errorf("runtime = %q, want docker", cfg.Runtime.CLI)
	}
	if cfg.Output.Color != "never" {
		t.Errorf("color = %q, want never", cfg.Output.Color)
	}
	if !cfg.Output.Verbose {
		t.Error("verbose = false, want true")
	}
}

func TestLoadConfig_UpstreamCACertsEnv(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	t.Setenv("WARDEN_UPSTREAM_CA_CERTS", "/path/to/ca.pem:/other/ca.crt")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Relay.UpstreamCACerts) != 2 {
		t.Fatalf("got %d certs, want 2", len(cfg.Relay.UpstreamCACerts))
	}
	if cfg.Relay.UpstreamCACerts[0] != "/path/to/ca.pem" {
		t.Errorf("cert[0] = %q, want /path/to/ca.pem",
			cfg.Relay.UpstreamCACerts[0])
	}
	if cfg.Relay.UpstreamCACerts[1] != "/other/ca.crt" {
		t.Errorf("cert[1] = %q, want /other/ca.crt",
			cfg.Relay.UpstreamCACerts[1])
	}
}

func TestLoadConfig_UpstreamCACertsToml(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `[relay]
upstream_ca_certs = ["/my/corp-ca.pem"]
system_ca_bundle = false
`
	err := os.WriteFile(
		filepath.Join(dir, "warden.toml"), []byte(tomlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Relay.UpstreamCACerts) != 1 {
		t.Fatalf("got %d certs, want 1", len(cfg.Relay.UpstreamCACerts))
	}
	if cfg.Relay.UpstreamCACerts[0] != "/my/corp-ca.pem" {
		t.Errorf("cert[0] = %q, want /my/corp-ca.pem",
			cfg.Relay.UpstreamCACerts[0])
	}
	if cfg.Relay.SystemCABundle == nil || *cfg.Relay.SystemCABundle {
		t.Error("system_ca_bundle should be false")
	}
}

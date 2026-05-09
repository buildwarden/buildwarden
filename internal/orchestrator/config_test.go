package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath_Directory(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	os.WriteFile(df, []byte("FROM alpine"), 0644)

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
	os.WriteFile(df, []byte("FROM alpine"), 0644)

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
	// Resolve symlinks (macOS /var -> /private/var)
	dir, _ = filepath.EvalSymlinks(dir)
	df := filepath.Join(dir, "Dockerfile")
	os.WriteFile(df, []byte("FROM alpine"), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

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
	os.WriteFile(cf, []byte("FROM alpine"), 0644)

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
	// In a temp dir with no config files, should get defaults
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

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
	os.WriteFile(filepath.Join(dir, "warden.toml"), []byte(tomlContent), 0644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

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
	os.Chdir(dir)
	defer os.Chdir(orig)

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

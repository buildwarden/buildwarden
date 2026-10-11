package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Runtime RuntimeConfig `toml:"runtime"`
	Relay   RelayConfig   `toml:"relay"`
	Build   BuildCfg      `toml:"build"`
	VM      VMConfig      `toml:"vm"`
	Output  OutputConfig  `toml:"output"`
}

// VMConfig holds defaults for the VM-driver flags (--cpus, --memory,
// --network); a flag always wins.
type VMConfig struct {
	CPUs     int    `toml:"cpus"`
	MemoryMB int    `toml:"memory_mb"`
	Network  string `toml:"network"`
}

type RelayConfig struct {
	UpstreamCACerts []string `toml:"upstream_ca_certs"`
	SystemCABundle  *bool    `toml:"system_ca_bundle"`
}

type RuntimeConfig struct {
	CLI        string `toml:"cli"`
	Driver     string `toml:"driver"`
	RelayImage string `toml:"relay_image"`
}

type BuildCfg struct {
	Dockerfile string `toml:"dockerfile"`
	Context    string `toml:"context"`
	Capture    string `toml:"capture"`
	OutputDir  string `toml:"output_dir"`
	Compress   *bool  `toml:"compress"`
}

type OutputConfig struct {
	Color   string `toml:"color"`
	Verbose bool   `toml:"verbose"`
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Output: OutputConfig{Color: "auto"},
	}

	// User-level config
	if home, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(home, ".config", "warden", "config.toml")
		if err := loadTOML(userPath, cfg); err != nil {
			return nil, err
		}
	}

	// Project-level config (overrides user)
	if err := loadTOML("warden.toml", cfg); err != nil {
		return nil, err
	}

	// Environment variable overrides
	if cli := os.Getenv("WARDEN_CTR_CLI"); cli != "" {
		cfg.Runtime.CLI = cli
	}
	if d := os.Getenv("WARDEN_DRIVER"); d != "" {
		cfg.Runtime.Driver = d
	}
	if img := os.Getenv("WARDEN_IMAGE"); img != "" {
		cfg.Runtime.RelayImage = img
	}
	if caCerts := os.Getenv("WARDEN_UPSTREAM_CA_CERTS"); caCerts != "" {
		paths := strings.Split(caCerts, ":")
		cfg.Relay.UpstreamCACerts = append(cfg.Relay.UpstreamCACerts, paths...)
	}
	if os.Getenv("NO_COLOR") != "" {
		cfg.Output.Color = "never"
	}
	if os.Getenv("WARDEN_VERBOSE") != "" {
		cfg.Output.Verbose = true
	}

	return cfg, nil
}

func loadTOML(path string, cfg *Config) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

// DetectRuntime probes for a working container runtime.
// Preference order: finch, docker, podman.
func DetectRuntime() (string, error) {
	for _, name := range []string{"finch", "docker", "podman"} {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := exec.CommandContext(ctx, name, "info").Run()
		cancel()
		if err == nil {
			return name, nil
		}
	}
	return "", errors.New("no container runtime found (tried finch, docker, podman)")
}

// ResolvePath handles the single-argument path logic:
//   - If path is a file: use it as Dockerfile, parent dir as context
//   - If path is a directory: use it as context, discover Dockerfile inside
//   - If path is empty: use CWD as context, discover Dockerfile
func ResolvePath(path string) (dockerfile, contextDir string, err error) {
	if path == "" {
		path = "."
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		return "", "", fmt.Errorf("cannot access %q: %w", path, statErr)
	}

	if !info.IsDir() {
		// Path is a file — treat as Dockerfile, parent is context
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", "", err
		}
		return abs, filepath.Dir(abs), nil
	}

	// Path is a directory — discover Dockerfile inside it
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}

	df, err := discoverDockerfile(abs)
	if err != nil {
		return "", "", err
	}
	return df, abs, nil
}

func discoverDockerfile(dir string) (string, error) {
	for _, name := range []string{"Dockerfile", "Containerfile"} {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no Dockerfile or Containerfile found in %s", dir)
}

// resolveBuildInput resolves the build context and (optional) Dockerfile for
// any driver. The container driver always builds a Dockerfile. VM drivers
// build whichever the context provides, with the precedence the drivers apply
// (driver.StageBuildScript): --script, then a Dockerfile (Linux guests), then
// build.sh / build.ps1 in the context. A path may name the context directory
// or a file in it (a Dockerfile, or a build script).
func resolveBuildInput(path, driverName, guestOS, script string) (dockerfile, contextDir, buildScript string, err error) {
	if driverName == "" || driverName == "container" {
		dockerfile, contextDir, err = ResolvePath(path)
		return dockerfile, contextDir, "", err
	}
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", "", fmt.Errorf("cannot access %q: %w", path, err)
	}
	contextDir = abs
	if !info.IsDir() {
		contextDir = filepath.Dir(abs)
		if isScriptName(abs) && script == "" {
			script = abs
		}
	}

	windows := guestOS == "windows"
	if !windows {
		if !info.IsDir() && !isScriptName(abs) {
			dockerfile = abs
		} else if df, err := discoverDockerfile(contextDir); err == nil {
			dockerfile = df
		}
	}
	if script != "" || dockerfile != "" {
		return dockerfile, contextDir, script, nil
	}
	want := "build.sh"
	if windows {
		want = "build.ps1"
	}
	if _, err := os.Stat(filepath.Join(contextDir, want)); err != nil {
		hint := " or a Dockerfile"
		if windows {
			hint = ""
		}
		return "", "", "", fmt.Errorf(
			"nothing to build in %s: pass --script, or add %s%s", contextDir, want, hint)
	}
	return "", contextDir, "", nil
}

func isScriptName(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".sh", ".ps1":
		return true
	}
	return false
}

func validateDriver(d string) error {
	switch d {
	case "", "container", "qemu", "vz", "hyperv":
		return nil
	default:
		return fmt.Errorf(
			"unknown driver %q (valid: container, qemu, vz, hyperv)", d)
	}
}

func validateCapture(c string) error {
	switch c {
	case "", "none", "headers", "bodies", "all":
		return nil
	default:
		return fmt.Errorf(
			"invalid capture mode %q (valid: none, headers, bodies, all)", c)
	}
}

func validateFlagsForDriver(d string) error {
	isVM := d == "qemu" || d == "vz" || d == "hyperv"
	for _, f := range []struct {
		set  bool
		name string
	}{
		{flagScript != "", "--script"},
		{flagImage != "", "--image"},
		{flagCPUs != 0, "--cpus"},
		{flagMemoryMB != 0, "--memory"},
		{flagKeepVMs, "--keep-vms"},
	} {
		if f.set && !isVM {
			return fmt.Errorf("%s requires a VM driver (--driver qemu, vz, or hyperv)", f.name)
		}
	}
	if flagNetwork != "" && d != "hyperv" {
		return fmt.Errorf("--network is only used by --driver hyperv")
	}
	return nil
}

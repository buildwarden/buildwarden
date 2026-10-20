package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/buildwarden/buildwarden/driver"
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
	// ImageRefreshAfter / ImageMinGrace: ages such as "7d" or "36h".
	ImageRefreshAfter string `toml:"image_refresh_after"`
	ImageMinGrace     string `toml:"image_min_grace"`
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
	// TempDir is where each build's private work dir is created; --tmpdir
	// wins, and empty means the OS temp dir.
	TempDir string `toml:"tmpdir"`
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
		return driver.Fixable("fix the TOML syntax in "+path, "parsing %s: %v", path, err)
	}
	return nil
}

// DetectRuntime probes for a working container runtime.
// Preference order: finch, docker, podman.
func DetectRuntime() (string, error) {
	var installed []string
	for _, name := range []string{"finch", "docker", "podman"} {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		installed = append(installed, name)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := exec.CommandContext(ctx, name, "info").Run()
		cancel()
		if err == nil {
			return name, nil
		}
	}
	if len(installed) == 0 {
		return "", driver.Fixable(
			"install Finch (brew install --cask finch && finch vm init), "+
				"Docker or Podman, or use a VM driver (--driver qemu or vz)",
			"no container runtime found (tried finch, docker, podman)")
	}
	// Installed but not answering: almost always its VM or daemon is down.
	return "", driver.Fixable(runtimeStartHint(installed[0]),
		"container runtime %s is installed but not running (`%s info` failed)",
		strings.Join(installed, ", "), installed[0])
}

// runtimeStartHint is the command that starts a stopped runtime.
func runtimeStartHint(name string) string {
	switch name {
	case "finch":
		return "start its VM: finch vm start (or finch vm init the first time)"
	case "podman":
		return "start its machine: podman machine start " +
			"(or podman machine init the first time)"
	default:
		return "start Docker (Docker Desktop, or the docker service)"
	}
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
		return "", "", driver.Fixable("", "cannot access %q: %v", path, statErr)
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
	return "", driver.Fixable("pass the Dockerfile's path, or the directory that holds it",
		"no Dockerfile or Containerfile found in %s", dir)
}

// resolveBuildInput resolves the build context and (optional) Dockerfile for
// any driver. The container driver always builds a Dockerfile. VM drivers
// build whichever the context provides, with the precedence the drivers apply
// (driver.StageBuildScript): --script, then a Dockerfile (Linux guests), then
// build.sh / build.ps1 in the context. A path may name the context directory
// or a file in it (a Dockerfile, or a build script).
func resolveBuildInput(
	path, driverName, guestOS, script string,
) (dockerfile, contextDir, buildScript string, err error) {
	if driverName == "" || driverName == "container" {
		dockerfile, contextDir, err = ResolvePath(path)
		return dockerfile, contextDir, "", err
	}
	return resolveVMBuildInput(path, guestOS, script)
}

// resolveVMBuildInput resolves the context dir and either a Dockerfile or a
// build script for a VM driver, applying driver.StageBuildScript's precedence:
// --script, then a Dockerfile (Linux guests), then build.sh / build.ps1.
func resolveVMBuildInput(
	path, guestOS, script string,
) (dockerfile, contextDir, buildScript string, err error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", "", driver.Fixable("", "cannot access %q: %v", path, err)
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
		dockerfile = vmDockerfile(abs, contextDir, info.IsDir())
	}
	if script != "" || dockerfile != "" {
		return dockerfile, contextDir, script, nil
	}
	if err := requireBuildFile(contextDir, windows); err != nil {
		return "", "", "", err
	}
	return "", contextDir, "", nil
}

// vmDockerfile returns the Dockerfile to build for a Linux VM guest: the path
// itself when it names one, else one discovered in the context dir, or "" when
// there is none.
func vmDockerfile(abs, contextDir string, isDir bool) string {
	if !isDir && !isScriptName(abs) {
		return abs
	}
	if df, err := discoverDockerfile(contextDir); err == nil {
		return df
	}
	return ""
}

// requireBuildFile confirms the context holds the build file a VM guest needs
// when neither a --script nor a Dockerfile was resolved.
func requireBuildFile(contextDir string, windows bool) error {
	want := "build.sh"
	if windows {
		want = "build.ps1"
	}
	if _, err := os.Stat(filepath.Join(contextDir, want)); err != nil {
		hint := " or a Dockerfile"
		if windows {
			hint = ""
		}
		return driver.Fixable("",
			"nothing to build in %s: pass --script, or add %s%s",
			contextDir, want, hint)
	}
	return nil
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
		return driver.Fixable("",
			"unknown driver %q (valid: container, qemu, vz, hyperv)", d)
	}
}

func validateCapture(c string) error {
	switch c {
	case "", "none", "headers", "bodies", "all":
		return nil
	default:
		return driver.Fixable("",
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
			return driver.Fixable("",
				"%s requires a VM driver (--driver qemu, vz, or hyperv)", f.name)
		}
	}
	if flagNetwork != "" && d != "hyperv" {
		return driver.Fixable("", "--network is only used by --driver hyperv")
	}
	return nil
}

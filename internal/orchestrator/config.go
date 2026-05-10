package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Runtime RuntimeConfig `toml:"runtime"`
	Build   BuildCfg     `toml:"build"`
	Output  OutputConfig `toml:"output"`
}

type RuntimeConfig struct {
	CLI string `toml:"cli"`
}

type BuildCfg struct {
	Dockerfile string `toml:"dockerfile"`
	Context    string `toml:"context"`
	Capture    string `toml:"capture"`
	OutputDir  string `toml:"output_dir"`
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
		loadTOML(userPath, cfg)
	}

	// Project-level config (overrides user)
	loadTOML("warden.toml", cfg)

	// Environment variable overrides
	if cli := os.Getenv("WARDEN_CTR_CLI"); cli != "" {
		cfg.Runtime.CLI = cli
	}
	if os.Getenv("NO_COLOR") != "" {
		cfg.Output.Color = "never"
	}
	if os.Getenv("WARDEN_VERBOSE") != "" {
		cfg.Output.Verbose = true
	}

	return cfg, nil
}

func loadTOML(path string, cfg *Config) {
	if _, err := os.Stat(path); err == nil {
		toml.DecodeFile(path, cfg) //nolint:errcheck
	}
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

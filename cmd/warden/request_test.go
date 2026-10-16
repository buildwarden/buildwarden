package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveBuildInputVMDrivers(t *testing.T) {
	dir := t.TempDir()
	df := write(t, dir, "Dockerfile", "FROM alpine")
	sh := write(t, dir, "build.sh", "true")

	// A directory with a Dockerfile: the Dockerfile is used (Linux guests).
	got, ctx, script, err := resolveBuildInput(dir, "qemu", "", "")
	if err != nil || got != df || ctx != dir || script != "" {
		t.Fatalf("dir: %q %q %q %v", got, ctx, script, err)
	}
	// A script path as the positional argument becomes the build script.
	got, ctx, script, err = resolveBuildInput(sh, "vz", "", "")
	if err != nil || script != sh || ctx != dir {
		t.Fatalf("script path: %q %q %q %v", got, ctx, script, err)
	}
	// Windows guests ignore Dockerfiles and need build.ps1 or --script.
	if _, _, _, err := resolveBuildInput(dir, "hyperv", "windows", ""); err == nil ||
		!strings.Contains(err.Error(), "build.ps1") {
		t.Fatalf("windows without build.ps1: err = %v", err)
	}
	write(t, dir, "build.ps1", "exit 0")
	if got, _, _, err := resolveBuildInput(dir, "qemu", "windows", ""); err != nil ||
		got != "" {
		t.Fatalf("windows with build.ps1: %q %v", got, err)
	}
	// An explicit --script satisfies any context.
	empty := t.TempDir()
	if _, _, s, err := resolveBuildInput(empty, "qemu", "", sh); err != nil || s != sh {
		t.Fatalf("--script in empty context: %q %v", s, err)
	}
	if _, _, _, err := resolveBuildInput(empty, "qemu", "", ""); err == nil {
		t.Fatal("empty context without --script accepted")
	}
}

func TestResolveBuildInputContainerNeedsDockerfile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "build.sh", "true")
	if _, _, _, err := resolveBuildInput(dir, "", "", ""); err == nil {
		t.Fatal("container driver accepted a context without a Dockerfile")
	}
}

func TestValidateFlagsForDriver(t *testing.T) {
	defer func() { flagCPUs, flagNetwork, flagKeepVMs, flagTimeout = 0, "", false, "" }()

	flagTimeout = "10m"
	if err := validateFlagsForDriver(""); err != nil {
		t.Fatalf("--timeout must be accepted by every driver: %v", err)
	}
	flagCPUs = 8
	if err := validateFlagsForDriver("container"); err == nil {
		t.Fatal("--cpus accepted by the container driver")
	}
	for _, d := range []string{"qemu", "vz", "hyperv"} {
		if err := validateFlagsForDriver(d); err != nil {
			t.Fatalf("--cpus refused by %s: %v", d, err)
		}
	}
	flagCPUs = 0
	flagNetwork = "warden-dev"
	if err := validateFlagsForDriver("qemu"); err == nil {
		t.Fatal("--network accepted by qemu")
	}
	if err := validateFlagsForDriver("hyperv"); err != nil {
		t.Fatalf("--network refused by hyperv: %v", err)
	}
}

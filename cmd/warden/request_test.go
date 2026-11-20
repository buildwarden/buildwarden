package main

import (
	"fmt"
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
	got, ctx, script, err := resolveBuildInput(dir, isVMDriver("qemu"), "", "")
	if err != nil || got != df || ctx != dir || script != "" {
		t.Fatalf("dir: %q %q %q %v", got, ctx, script, err)
	}
	// A script path as the positional argument becomes the build script.
	got, ctx, script, err = resolveBuildInput(sh, isVMDriver("vz"), "", "")
	if err != nil || script != sh || ctx != dir {
		t.Fatalf("script path: %q %q %q %v", got, ctx, script, err)
	}
	// Windows guests ignore Dockerfiles and need build.ps1 or --script.
	_, _, _, err = resolveBuildInput(dir, isVMDriver("hyperv"), "windows", "")
	if err == nil ||
		!strings.Contains(err.Error(), "build.ps1") {
		t.Fatalf("windows without build.ps1: err = %v", err)
	}
	write(t, dir, "build.ps1", "exit 0")
	got, _, _, err = resolveBuildInput(dir, isVMDriver("qemu"), "windows", "")
	if err != nil || got != "" {
		t.Fatalf("windows with build.ps1: %q %v", got, err)
	}
	// An explicit --script satisfies any context.
	empty := t.TempDir()
	_, _, s, err := resolveBuildInput(empty, isVMDriver("qemu"), "", sh)
	if err != nil || s != sh {
		t.Fatalf("--script in empty context: %q %v", s, err)
	}
	if _, _, _, err := resolveBuildInput(empty, isVMDriver("qemu"), "", ""); err == nil {
		t.Fatal("empty context without --script accepted")
	}
}

func TestResolveBuildInputContainerNeedsDockerfile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "build.sh", "true")
	if _, _, _, err := resolveBuildInput(dir, isVMDriver(""), "", ""); err == nil {
		t.Fatal("container driver accepted a context without a Dockerfile")
	}
}

func TestValidateFlagsForDriver(t *testing.T) {
	defer func() { flagCPUs, flagKeepVMs, flagTimeout = 0, false, "" }()

	flagTimeout = "10m"
	if err := validateFlagsForDriver(isVMDriver("")); err != nil {
		t.Fatalf("--timeout must be accepted by every driver: %v", err)
	}
	flagCPUs = 8
	if err := validateFlagsForDriver(isVMDriver("container")); err == nil {
		t.Fatal("--cpus accepted by the container driver")
	}
	for _, d := range []string{"qemu", "vz", "hyperv"} {
		if err := validateFlagsForDriver(isVMDriver(d)); err != nil {
			t.Fatalf("--cpus refused by %s: %v", d, err)
		}
	}
}

// Behaviour 2 and 3: --network is hyperv's flag; another driver refuses it as
// a flag it does not take, naming itself.
func TestNetworkIsHypervs(t *testing.T) {
	for _, d := range []string{"qemu", "firecracker", "vz", "container"} {
		inConfigFiles(t, "")
		_, err := runWarden(t, "build", "--driver", d, "--network", "warden-dev")
		want := `unknown flag --network for driver "` + d + `"`
		if err == nil || err.Error() != want {
			t.Errorf("--network on %s: %v, want %q", d, err, want)
		}
	}
	inConfigFiles(t, "")
	if _, err := runWarden(t, "build", "--driver", "hyperv", "--network", "warden-ci",
		"--help"); err != nil {
		t.Fatal(err)
	}
	if got := driverFlags.argsFor("run"); len(got) != 1 || got[0] != "--network=warden-ci" {
		t.Errorf("hyperv's args = %q, want [--network=warden-ci]", got)
	}
}

func TestFirecrackerIsAVMDriver(t *testing.T) {
	defer func() { flagCPUs, flagKeepVMs, flagImage = 0, false, "" }()
	if err := checkDriverName("firecracker"); err != nil {
		t.Fatalf("checkDriverName(firecracker) = %v", err)
	}
	flagCPUs, flagKeepVMs, flagImage = 2, true, "disk.qcow2"
	if err := validateFlagsForDriver(isVMDriver("firecracker")); err != nil {
		t.Fatalf("VM flags refused by firecracker: %v", err)
	}
	if err := validateFlagsForDriver(isVMDriver("container")); err == nil ||
		!strings.Contains(err.Error(), "firecracker") {
		t.Fatalf("the VM-driver hint does not name firecracker: %v", err)
	}
	flagCPUs, flagKeepVMs, flagImage = 0, false, ""
	fc := linkedDrivers["firecracker"].Capabilities
	if err := validateGuestOS("windows", "firecracker", fc); err == nil {
		t.Fatal("--guest-os windows accepted by firecracker")
	}
	caches, err := layerCaches("firecracker")
	if err != nil || caches["firecracker"] == nil {
		t.Fatalf("layerCaches(firecracker) = %v, %v", caches, err)
	}
}

// linkedSel is the selection of the driver cfg names, as prepare finds it
// when no driver binary of that name is on PATH or beside warden: a linked
// driver, or the container driver's binary, which the tests build beside the
// test binary (TestMain).
func linkedSel(cfg *Config) *selectedDriver {
	n := driverName(cfg)
	if l := linkedDrivers[n]; l != nil {
		return &selectedDriver{found: &foundDriver{Name: n, Found: foundLinked,
			Linked: true}, linked: l}
	}
	sd, err := findSelected(cfg)
	if err != nil {
		panic(fmt.Sprintf("the driver %s: %v", n, err))
	}
	return sd
}

// testCaps is the capabilities of the driver named d, as linkedSel finds it.
func testCaps(d string) capabilities {
	return linkedSel(&Config{Runtime: RuntimeConfig{Driver: d}}).caps()
}

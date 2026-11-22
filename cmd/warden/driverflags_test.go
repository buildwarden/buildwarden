package main

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver/container"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// The scan finds the driver before the parse, with three lines in
// particular: the ext form, a --driver read as another flag's value, and
// one after "--" (the design's Configuration, steps 1 and 4).
func TestScanAndParseAgree(t *testing.T) {
	t.Run("the ext form", func(t *testing.T) {
		name, rest, ok := extCommand([]string{"nomad", "job", "run", "--driver",
			"docker"})
		if ok && (name != "nomad" || !slices.Equal(rest, []string{"job", "run",
			"--driver", "docker"})) {
			t.Errorf("ext form = %q %q", name, rest)
		}
		if v, found := scanValue([]string{"build", "./ctx", "--", "--driver", "x"},
			"driver"); found {
			t.Errorf("the scan read --driver %q after --", v)
		}
	})
	t.Run("--driver read as the value of --network", func(t *testing.T) {
		dir := inConfigFiles(t, "")
		write(t, dir, "Dockerfile", "FROM alpine\n")
		_, err := runWarden(t, "build", "--network", "--driver", "hyperv")
		wantFixable(t, err, "--driver was read as the value of --network",
			"give --network its value, or write --network=<value>")
	})
	t.Run("--config read as the value of --network", func(t *testing.T) {
		dir := inConfigFiles(t, "")
		write(t, dir, "Dockerfile", "FROM alpine\n")
		cfg := write(t, t.TempDir(), "x.toml", "[runtime]\ndriver = \"hyperv\"\n")
		_, err := runWarden(t, "build", "--network", "--config", cfg)
		wantFixable(t, err, "--config was read as the value of --network",
			"give --network its value, or write --network=<value>")
	})
	t.Run("the driver selected from the file --config names", func(t *testing.T) {
		inConfigFiles(t, "")
		cfg := write(t, t.TempDir(), "x.toml", "[runtime]\ndriver = \"hyperv\"\n")
		if _, err := runWarden(t, "build", "--config", cfg, "--network=a",
			"--help"); err != nil {
			t.Fatal(err)
		}
		if got := driverFlags.argsFor("run"); !slices.Equal(got,
			[]string{"--network=a"}) {
			t.Errorf("args = %q", got)
		}
	})
}

// DRV-62, DRV-63: a flag with a value is sent once per occurrence as
// --name=value, in the order typed and in either spelling, its bytes
// unchanged; a switch is settled by warden, the last value winning, and sent
// as --name only when it is on. A flag declared for every command the driver
// lists is accepted before the subcommand's name.
func TestDriverArgs(t *testing.T) {
	for _, tc := range []struct {
		line []string
		cmd  string
		want []string
	}{
		{[]string{"build", "--runtime", "docker", "--runtime=", "--help"}, "run",
			[]string{"--runtime=docker", "--runtime="}},
		{[]string{"--runtime", "podman", "build", "--help"}, "run",
			[]string{"--runtime=podman"}},
		{[]string{"image", "prepare", "--driver", "qemu", "--force", "--edition",
			"Windows Server 2025 Datacenter", "--help"}, "image-prepare",
			[]string{"--edition=Windows Server 2025 Datacenter", "--force"}},
		{[]string{"image", "prepare", "--driver", "qemu", "--force",
			"--force=false", "--help"}, "image-prepare", []string{}},
	} {
		inConfigFiles(t, "")
		if _, err := runWarden(t, tc.line...); err != nil {
			t.Fatalf("%q: %v", tc.line, err)
		}
		if got := driverFlags.argsFor(tc.cmd); !slices.Equal(got, tc.want) {
			t.Errorf("%q: args %q, want %q", tc.line, got, tc.want)
		}
	}
}

// Behaviour 1: help shows the selected driver's flags in a section of its
// own, and never fails for the driver's sake.
func TestHelpShowsTheDriversFlags(t *testing.T) {
	inConfigFiles(t, "")
	out, err := runWarden(t, "build", "--driver", "hyperv", "--help")
	if err != nil || !strings.Contains(out, "Flags of driver \"hyperv\":") ||
		!strings.Contains(out, "--network") {
		t.Errorf("help with hyperv: %v\n%s", err, out)
	}
	inConfigFiles(t, "")
	t.Setenv("PATH", t.TempDir())
	out, err = runWarden(t, "build", "--driver", "nomad", "--help")
	if err != nil || !strings.Contains(out, "Flags of the driver: not shown: ") {
		t.Errorf("help with a driver that is not found: %v\n%s", err, out)
	}
}

// a27 (6): warden's own keys are still read in any letter case, as on main,
// while the same file's [Runtime] CLI is the container driver's to refuse.
func TestWardensOwnKeysInAnyCase(t *testing.T) {
	inConfigFiles(t, "[Runtime]\ndriver = \"container\"\nCLI = \"podman\"\n"+
		"[BUILD]\nteardown_timeout = \"90s\"\n")
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.Driver != "container" || cfg.Build.teardown() != 90*time.Second {
		t.Errorf("driver %q, teardown %v; want container and 90s", cfg.Runtime.Driver,
			cfg.Build.teardown())
	}
	_, err = drivercmd.Resolve("container", container.Settings, drivercmd.CmdRun, nil,
		absConfigFile(cfg), os.Environ())
	wantFixable(t, err, configPath(t)+": [Runtime] CLI: keys are case-sensitive, and "+
		"the container driver reads [driver.container] cli (old name [runtime] cli)",
		"write it as [driver.container] cli")
}

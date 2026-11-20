package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// tomlString writes v as a TOML basic string.
func tomlString(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch {
		case r == '"' || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func cliFile(v string) string { return "[runtime]\ncli = " + tomlString(v) + "\n" }

// runtimeFlag is --runtime as a test gives it; warden hands it to the
// container driver in args.
var runtimeFlag string

// writeScript writes an executable shell script.
func writeScript(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// A run that does not use the container runtime does not validate the
// setting (UX-08): one warden.toml may hold the settings of several drivers,
// for runs that choose among them on the command line.
func TestUnusedRuntimeIsNotValidated(t *testing.T) {
	const unused = "nonsense"
	vmDrivers := []string{"qemu", "vz", "hyperv", "firecracker"}

	for _, drv := range append([]string{"", "container"}, vmDrivers...) {
		t.Run("resolveConfig with --driver "+drv, func(t *testing.T) {
			inConfigFiles(t, cliFile(unused))
			flagDriver = drv
			if _, err := resolveConfig(); err != nil {
				t.Fatalf("resolveConfig: %v", err)
			}
		})
	}
	// These two return before the runtime is resolved, after their driver's
	// own clean. That clean removes what it takes for the leftovers of builds
	// that are not running, so it is given nothing to find: the user cache
	// directory, where the build locks are, and --tmpdir are empty
	// directories. On Windows hyperv's clean lists the host's real VMs, which
	// a test cannot redirect, so that row is not run there.
	for _, drv := range []string{"hyperv", "firecracker"} {
		t.Run("clean with --driver "+drv, func(t *testing.T) {
			if drv == "hyperv" && runtime.GOOS == "windows" {
				t.Skip("hyperv's clean would act on this host's VMs")
			}
			inConfigFiles(t, cliFile(unused))
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("LocalAppData", t.TempDir())
			flagDriver, flagTempDir = drv, t.TempDir()
			if err := runClean(cleanCmd, nil); err != nil {
				t.Fatalf("clean: %v", err)
			}
		})
	}
}

// doctorChildEnv names the driver for TestDoctorChild, in the child process
// only; doctorChildRan is the line that child prints before it runs doctor.
const (
	doctorChildEnv = "WARDEN_TEST_DOCTOR_DRIVER"
	doctorChildRan = "doctor child: running"
)

// TestDoctorChild is the child process of TestDoctorIgnoresAnUnusedRuntime
// and does nothing on its own. doctor ends the process when the checks of
// hyperv or firecracker fail, as they do on every host but their own, so it
// cannot be called in the test's process.
func TestDoctorChild(t *testing.T) {
	drv := os.Getenv(doctorChildEnv)
	if drv == "" {
		return
	}
	flagDriver = drv
	fmt.Println(doctorChildRan)
	if err := doctorCmd.RunE(doctorCmd, nil); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		var fe *driver.FixableError
		if errors.As(err, &fe) {
			fmt.Printf("To fix: %s\n", fe.Fix)
		}
	}
}

// doctor under a VM driver does not use the container runtime either, so it
// says nothing about an unsupported one.
func TestDoctorIgnoresAnUnusedRuntime(t *testing.T) {
	const unused = "nonsense"
	for _, drv := range []string{"qemu", "vz", "hyperv", "firecracker"} {
		t.Run("--driver "+drv, func(t *testing.T) {
			inConfigFiles(t, cliFile(unused))
			// doctor is not read-only: under firecracker it probes reflink
			// copies in the user cache directory and leaves the driver's
			// work root there. The child gets an empty cache directory, set
			// before its environment is copied from this process's.
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("LocalAppData", t.TempDir())
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			child := exec.Command(self, "-test.run=^TestDoctorChild$")
			child.Env = append(os.Environ(), doctorChildEnv+"="+drv)
			// doctor's own verdict on this host is not the subject: only
			// that it says nothing about the runtime.
			out, _ := child.CombinedOutput()
			if !strings.Contains(string(out), doctorChildRan) {
				t.Fatalf("the child did not run doctor:\n%s", out)
			}
			for _, about := range []string{unused, "[runtime] cli"} {
				if strings.Contains(string(out), about) {
					t.Errorf("doctor mentions %q:\n%s", about, out)
				}
			}
		})
	}
}

// runLine runs warden command with the levels the test set on its command
// line: --driver from flagDriver, --runtime from runtimeFlag.
func runLine(t *testing.T, command string) error {
	t.Helper()
	line := []string{command}
	if flagDriver != "" {
		line = append(line, "--driver", flagDriver)
	}
	if runtimeFlag != "" {
		line = append(line, "--runtime="+runtimeFlag)
	}
	_, err := runWarden(t, line...)
	return err
}

// shipPrograms writes executables of the given names into dir; each creates
// marker when it is run. It returns the first one's path.
func shipPrograms(t *testing.T, dir, marker string, names ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	script := fmt.Sprintf("#!/bin/sh\necho ran >> '%s'\n", marker)
	for _, name := range names {
		writeScript(t, filepath.Join(dir, name), script)
	}
	return filepath.Join(dir, names[0])
}

// wantNotRun checks that no shipped program ran, and that the command made
// no output directory in the project.
func wantNotRun(t *testing.T, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("a program the configuration named was run (marker: %v)", err)
	}
	if _, err := os.Stat("warden-output"); !os.IsNotExist(err) {
		t.Errorf("an output directory was made (%v)", err)
	}
}

// clean under qemu or vz no longer runs the container runtime's cleanup, so
// a program a project's warden.toml names as the runtime is neither read nor
// run (Behaviour 5); the container driver's refusal of it is
// driver/container's TestUnsupportedRuntimeIsNeverRun.
func TestCleanUnderAVMDriverRunsNoRuntime(t *testing.T) {
	for _, tc := range []struct{ name, project, driver string }{
		{"beside driver = \"qemu\"", "[runtime]\ncli = \"./rt\"\ndriver = \"qemu\"\n", ""},
		{"with --driver vz", cliFile("./rt"), "vz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			project := inConfigFiles(t, tc.project)
			shipPrograms(t, project, marker, "rt", "pull", "container", "network",
				"image", "info")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("LocalAppData", t.TempDir())
			flagDriver = tc.driver
			if err := runLine(t, "clean"); err != nil {
				t.Errorf("err = %v, want none: no runtime is read", err)
			}
			wantNotRun(t, marker)
		})
	}
}

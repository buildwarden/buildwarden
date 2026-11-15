package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/buildwarden/buildwarden/driver"
)

// The four levels that can name the container runtime, lowest first.
var runtimeLevels = []string{"user", "project", "env", "flag"}

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

// userConfigFile is the path of the user's config file under the test's
// home, as the error names it.
func userConfigFile(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".config", "warden", "config.toml")
}

// nameRuntime runs the test from fresh config directories with the runtime
// named v at one level and nowhere else. It returns what the error calls
// that level. The flag's value is a copy, so that it shares no memory with a
// string constant of the test binary.
func nameRuntime(t *testing.T, level, v string) string {
	t.Helper()
	switch level {
	case "user":
		inConfigFiles(t, cliFile(v), "")
		return "[runtime] cli in " + userConfigFile(t)
	case "project":
		inConfigFiles(t, "", cliFile(v))
		return "[runtime] cli in warden.toml"
	case "env":
		inConfigFiles(t, "", "")
		t.Setenv("WARDEN_CTR_CLI", v)
		return "WARDEN_CTR_CLI"
	case "flag":
		inConfigFiles(t, "", "")
		flagRuntime = strings.Clone(v)
		return "--runtime"
	}
	t.Fatalf("unknown level %q", level)
	return ""
}

// resolvedRuntime is what a command that uses the container runtime gets:
// the configuration as resolveConfig leaves it, through resolveRuntime.
func resolvedRuntime(t *testing.T) (string, error) {
	t.Helper()
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	return resolveRuntime(cfg)
}

// wantRuntimeRefused checks that err is the refusal of v as named at setting:
// a FixableError, which main exits 1 on (UX-09), whose message starts with
// the setting and the quoted value.
func wantRuntimeRefused(t *testing.T, err error, setting, v string) *driver.FixableError {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError refusing %q", err, v)
	}
	var es exitStatusError
	if errors.As(err, &es) {
		t.Errorf("the refusal carries exit status %d, want main's exit 1", int(es))
	}
	if want := setting + " is " + strconv.Quote(v) + "; "; !strings.HasPrefix(fe.Msg, want) {
		t.Errorf("message %q does not start with %q", fe.Msg, want)
	}
	if fe.Fix == "" {
		t.Error("the refusal has no fix")
	}
	return fe
}

func TestSupportedRuntimeFromEverySource(t *testing.T) {
	for i, name := range []string{"finch", "docker", "podman"} {
		for _, level := range runtimeLevels {
			t.Run(name+" from "+level, func(t *testing.T) {
				nameRuntime(t, level, name)
				got, err := resolvedRuntime(t)
				if err != nil || got != name {
					t.Fatalf("resolveRuntime = %q, %v; want %q", got, err, name)
				}
				// The string that is run is the list's own, not an equal one
				// that a flag, a variable or a file supplied.
				if unsafe.StringData(got) != unsafe.StringData(containerRuntimes[i]) {
					t.Errorf("resolveRuntime returned a string equal to %q "+
						"that is not containerRuntimes' member", name)
				}
			})
		}
	}
}

func TestUnsupportedRuntimeRefusedFromEverySource(t *testing.T) {
	for _, v := range []string{
		"./docker", "tools/docker", "/usr/bin/docker", `..\x.exe`,
		`C:\x\docker.exe`, "C:docker", "docker.exe", "Docker", " docker",
		"docker ", "nerdctl", "sh", "sudo docker", " ",
	} {
		for _, level := range runtimeLevels {
			t.Run(strconv.Quote(v)+" from "+level, func(t *testing.T) {
				setting := nameRuntime(t, level, v)
				got, err := resolvedRuntime(t)
				if got != "" {
					t.Errorf("resolveRuntime returned %q with its refusal", got)
				}
				wantRuntimeRefused(t, err, setting, v)
			})
		}
	}
}

// Detection and the check share one list, in the order detection has always
// probed.
func TestDetectRuntimeProbesTheSupportedList(t *testing.T) {
	want := []string{"finch", "docker", "podman"}
	if !slices.Equal(containerRuntimes, want) {
		t.Fatalf("containerRuntimes = %v, want %v", containerRuntimes, want)
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	bin, probed := t.TempDir(), filepath.Join(t.TempDir(), "probed")
	for _, name := range want {
		script := fmt.Sprintf("#!/bin/sh\necho %s \"$1\" >> '%s'\nexit 1\n", name, probed)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	if rt, err := DetectRuntime(); err == nil {
		t.Fatalf("DetectRuntime = %q with no runtime answering", rt)
	}
	got, err := os.ReadFile(probed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "finch info\ndocker info\npodman info\n" {
		t.Errorf("detection ran %q, want info of finch, docker, podman in order", got)
	}
}

// inRuntimeLevels runs the test with the runtime named at each level whose
// value is not empty; user and project are file bodies.
func inRuntimeLevels(t *testing.T, user, project, env, flag string) {
	t.Helper()
	inConfigFiles(t, user, project)
	t.Setenv("WARDEN_CTR_CLI", env)
	flagRuntime = flag
}

// The value in effect is the highest level that names a runtime. A level
// below it is not read, so an unsupported value there is not an error; an
// unsupported value in effect is one, and no lower level is used in its place.
func TestRuntimeOverridesComeFirst(t *testing.T) {
	const bad = "./rt"
	inProject, inUser := "[runtime] cli in warden.toml", "the user's file"
	for _, tc := range []struct {
		name                     string
		user, project, env, flag string
		want                     string // the runtime, or
		refused                  string // the setting the refusal names
	}{
		{name: "flag over an unsupported project value",
			project: bad, flag: "docker", want: "docker"},
		{name: "variable over an unsupported project value",
			project: bad, env: "podman", want: "podman"},
		{name: "project over an unsupported user value",
			user: bad, project: "docker", want: "docker"},
		{name: "flag over an unsupported variable",
			env: bad, flag: "finch", want: "finch"},
		{name: "unsupported flag over a supported project value",
			project: "docker", flag: bad, refused: "--runtime"},
		{name: "unsupported variable over supported files",
			user: "finch", project: "docker", env: bad, refused: "WARDEN_CTR_CLI"},
		{name: "unsupported project value over a supported user value",
			user: "podman", project: bad, refused: inProject},
		{name: "both files hold the same unsupported string",
			user: bad, project: bad, refused: inProject},
		{name: "only the user's file holds it",
			user: bad, refused: inUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user, project := "", ""
			if tc.user != "" {
				user = cliFile(tc.user)
			}
			if tc.project != "" {
				project = cliFile(tc.project)
			}
			inRuntimeLevels(t, user, project, tc.env, tc.flag)
			got, err := resolvedRuntime(t)
			if tc.refused == "" {
				if err != nil || got != tc.want {
					t.Fatalf("resolveRuntime = %q, %v; want %q", got, err, tc.want)
				}
				return
			}
			if tc.refused == inUser {
				tc.refused = "[runtime] cli in " + userConfigFile(t)
			}
			wantRuntimeRefused(t, err, tc.refused, bad)
		})
	}
}

// An empty flag and an empty variable are not set. cli = "" in a file is
// that file's value: it hides a lower file's, and the runtime is detected.
func TestEmptyRuntime(t *testing.T) {
	t.Run("an empty flag and variable leave the project's value", func(t *testing.T) {
		inRuntimeLevels(t, "", cliFile("podman"), "", "")
		if got, err := resolvedRuntime(t); err != nil || got != "podman" {
			t.Fatalf("resolveRuntime = %q, %v; want podman", got, err)
		}
	})
	t.Run("an empty flag and variable leave the user's value", func(t *testing.T) {
		inRuntimeLevels(t, cliFile("finch"), "", "", "")
		if got, err := resolvedRuntime(t); err != nil || got != "finch" {
			t.Fatalf("resolveRuntime = %q, %v; want finch", got, err)
		}
	})
	t.Run("an empty project value hides the user's and is detected", func(t *testing.T) {
		inRuntimeLevels(t, cliFile("podman"), cliFile(""), "", "")
		t.Setenv("PATH", t.TempDir()) // detection finds nothing, on any host
		got, err := resolvedRuntime(t)
		if got != "" || err == nil ||
			!strings.Contains(err.Error(), "no container runtime found") {
			t.Fatalf("resolveRuntime = %q, %v; want detection's error", got, err)
		}
	})
}

// loadedConfig is LoadConfig's result, which never holds the marker.
func loadedConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Runtime.CLI == unsetCLI {
		t.Fatal("LoadConfig returned the marker as the runtime")
	}
	return cfg
}

// While the project's file is decoded, [runtime] cli holds a marker; a file
// that leaves it did not set the key, and the user's value is put back. A
// file that could produce the marker would have the user's value run in
// place of its own.
func TestRuntimeUnsetMarker(t *testing.T) {
	user := cliFile("podman")
	for name, project := range map[string]string{
		"no project file":                  "",
		"a project file with other keys":   "[output]\nverbose = true\n",
		"a [runtime] table without cli":    "[runtime]\ndriver = \"container\"\n",
		"a project file with only comment": "# nothing set\n",
	} {
		t.Run(name+" restores the user's value and source", func(t *testing.T) {
			inConfigFiles(t, user, project)
			cfg := loadedConfig(t)
			if cfg.Runtime.CLI != "podman" || cfg.Runtime.CLISource != userConfigFile(t) {
				t.Errorf("runtime = %q from %q, want podman from the user's file",
					cfg.Runtime.CLI, cfg.Runtime.CLISource)
			}
		})
	}
	t.Run("no file names a runtime", func(t *testing.T) {
		inConfigFiles(t, "", "[runtime]\ndriver = \"container\"\n")
		if cfg := loadedConfig(t); cfg.Runtime.CLI != "" {
			t.Errorf("runtime = %q, want empty", cfg.Runtime.CLI)
		}
	})
	for name, value := range map[string]string{
		"basic string":              "\"\xff\"",
		"literal string":            "'\xff'",
		"multi-line basic string":   "\"\"\"\xff\"\"\"",
		"multi-line literal string": "'''\xff'''",
	} {
		t.Run("the marker's byte in a "+name+" fails the load", func(t *testing.T) {
			inConfigFiles(t, user, "[runtime]\ncli = "+value+"\n")
			cfg, err := LoadConfig()
			var fe *driver.FixableError
			if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "parsing warden.toml") {
				t.Fatalf("LoadConfig = %+v, %v; want the file refused as not TOML", cfg, err)
			}
		})
	}
	for _, value := range []string{`"\xff"`, `"\u00ff"`, `"ÿ"`} {
		t.Run("cli = "+value+" is an unsupported value of the project's", func(t *testing.T) {
			inConfigFiles(t, user, "[runtime]\ncli = "+value+"\n")
			cfg := loadedConfig(t)
			got, err := resolveRuntime(cfg)
			if got != "" {
				t.Errorf("resolveRuntime returned %q, want only the refusal", got)
			}
			wantRuntimeRefused(t, err, "[runtime] cli in warden.toml", "\u00ff")
		})
	}
}

// The decoder fills [runtime] cli from a key in any letter case, in a table,
// a dotted key or an inline table. The check is on the field, so it does not
// depend on how the file spelled the key; the error names the key as
// documented.
func TestRuntimeKeyInOtherSpellings(t *testing.T) {
	spellings := map[string]string{
		"[runtime] CLI":   "[runtime]\nCLI = %s\n",
		"[runtime] Cli":   "[runtime]\nCli = %s\n",
		"[Runtime] cli":   "[Runtime]\ncli = %s\n",
		"[RUNTIME] CLI":   "[RUNTIME]\nCLI = %s\n",
		"dotted key":      "runtime.CLI = %s\n",
		"an inline table": "Runtime = { CLI = %s }\n",
	}
	for name, format := range spellings {
		for _, v := range []string{"./rt", "sh"} {
			t.Run(name+" "+v, func(t *testing.T) {
				// podman in the user's file is what a file that passed for
				// one without the key would be replaced by.
				inConfigFiles(t, cliFile("podman"), fmt.Sprintf(format, tomlString(v)))
				got, err := resolvedRuntime(t)
				if got != "" {
					t.Errorf("resolveRuntime returned %q, want only the refusal", got)
				}
				wantRuntimeRefused(t, err, "[runtime] cli in warden.toml", v)
			})
		}
	}
	t.Run("[Runtime] CLI selects a supported runtime", func(t *testing.T) {
		inConfigFiles(t, cliFile("docker"), "[Runtime]\nCLI = \"podman\"\n")
		if got, err := resolvedRuntime(t); err != nil || got != "podman" {
			t.Fatalf("resolveRuntime = %q, %v; want podman", got, err)
		}
	})
	// Which of two spellings in one table the decoder takes varies from run
	// to run. Whichever it is has been checked.
	t.Run("both spellings in one table", func(t *testing.T) {
		inConfigFiles(t, "", "[runtime]\ncli = \"docker\"\nCLI = \"./rt\"\n")
		for range 200 {
			got, err := resolvedRuntime(t)
			if err == nil && got == "docker" {
				continue
			}
			if got != "" {
				t.Fatalf("resolveRuntime returned %q, want docker or the refusal", got)
			}
			wantRuntimeRefused(t, err, "[runtime] cli in warden.toml", "./rt")
		}
	})
}

// doctorChildEnv names the driver for TestDoctorChild, in the child process
// only; doctorChildRan is the line that child prints before it runs doctor.
const (
	doctorChildEnv = "WARDEN_TEST_DOCTOR_DRIVER"
	doctorChildRan = "doctor child: running"
)

// TestDoctorChild is the child process of the doctor rows of
// TestUnusedRuntimeIsNotValidated and does nothing on its own. doctor ends
// the process when the checks of hyperv or firecracker fail, as they do on
// every host but their own, so those rows cannot call it in the test's
// process.
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

// A run that does not use the container runtime does not validate the
// setting (UX-08): one warden.toml may hold the settings of several drivers,
// for runs that choose among them on the command line.
func TestUnusedRuntimeIsNotValidated(t *testing.T) {
	const unused = "nonsense"
	vmDrivers := []string{"qemu", "vz", "hyperv", "firecracker"}

	for _, drv := range append([]string{"", "container"}, vmDrivers...) {
		t.Run("resolveConfig with --driver "+drv, func(t *testing.T) {
			inConfigFiles(t, "", cliFile(unused))
			flagDriver = drv
			if _, err := resolveConfig(); err != nil {
				t.Fatalf("resolveConfig: %v", err)
			}
		})
	}
	for _, drv := range vmDrivers {
		t.Run("newDriver for "+drv, func(t *testing.T) {
			inConfigFiles(t, "", cliFile(unused))
			flagDriver = drv
			cfg, err := resolveConfig()
			if err != nil {
				t.Fatalf("resolveConfig: %v", err)
			}
			d, err := newDriver(cfg)
			if err != nil || d.Name() != drv {
				t.Fatalf("newDriver = %v, %v; want the %s driver", d, err, drv)
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
			inConfigFiles(t, "", cliFile(unused))
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("LocalAppData", t.TempDir())
			flagDriver, flagTempDir = drv, t.TempDir()
			if err := runClean(cleanCmd, nil); err != nil {
				t.Fatalf("clean: %v", err)
			}
		})
	}
	for _, drv := range vmDrivers {
		t.Run("doctor with --driver "+drv, func(t *testing.T) {
			inConfigFiles(t, "", cliFile(unused))
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
					t.Errorf("doctor --driver %s mentions %q:\n%s", drv, about, out)
				}
			}
		})
	}
}

// runtimeCommands are the calls that resolve the container runtime: the one
// build and shell make, clean and doctor.
var runtimeCommands = []struct {
	name string
	run  func(t *testing.T) error
}{
	{"build and shell (newDriver)", func(t *testing.T) error {
		cfg, err := resolveConfig()
		if err != nil {
			t.Fatalf("resolveConfig: %v", err)
		}
		_, err = newDriver(cfg)
		return err
	}},
	{"clean", func(*testing.T) error { return runClean(cleanCmd, nil) }},
	{"doctor", func(t *testing.T) error {
		doctorCmd.SetOut(io.Discard)
		t.Cleanup(func() { doctorCmd.SetOut(nil) })
		return doctorCmd.RunE(doctorCmd, nil)
	}},
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
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, names[0])
}

// wantNotRun checks that no shipped program ran.
func wantNotRun(t *testing.T, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("a program the configuration named was run (marker: %v)", err)
	}
}

// The security fix. A build context ships a program and a warden.toml that
// names it as the container runtime, by path or as an interpreter that runs
// the file named like warden's first argument. Before the fix clean ran the
// program, and newDriver and doctor accepted it for a later run; now each
// refuses it and nothing is run. The same holds for a program the flag, the
// variable or the user's file names.
func TestUnsupportedRuntimeIsNeverRun(t *testing.T) {
	const viaShell = "sh" // runs ./pull, ./container, ... from the project
	inProject := "[runtime] cli in warden.toml"
	for _, tc := range []struct {
		name      string
		project   string // the project's file
		elsewhere string // or the level that names the shipped path
		driver    string // --driver
		value     string // the value refused, when not the path
		onlyClean bool
	}{
		{name: `cli = "./rt"`, project: cliFile("./rt"), value: "./rt"},
		{name: `CLI = "./rt"`, project: "[runtime]\nCLI = \"./rt\"\n", value: "./rt"},
		{name: `cli = "sh"`, project: cliFile(viaShell), value: viaShell},
		{name: `cli = "./rt" beside driver = "qemu"`, value: "./rt", onlyClean: true,
			project: "[runtime]\ncli = \"./rt\"\ndriver = \"qemu\"\n"},
		{name: `cli = "./rt" with --driver vz`, value: "./rt", onlyClean: true,
			project: cliFile("./rt"), driver: "vz"},
		{name: "the program's path in --runtime", elsewhere: "flag"},
		{name: "the program's path in WARDEN_CTR_CLI", elsewhere: "env"},
		{name: "the program's path in the user's file", elsewhere: "user"},
	} {
		for _, command := range runtimeCommands {
			if tc.onlyClean && command.name != "clean" {
				continue
			}
			t.Run(tc.name+", "+command.name, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "ran")
				program := shipPrograms(t, t.TempDir(), marker, "rt")
				setting, value := inProject, tc.value
				if tc.elsewhere != "" {
					setting, value = nameRuntime(t, tc.elsewhere, program), program
				} else {
					project := inConfigFiles(t, "", tc.project)
					shipPrograms(t, project, marker,
						"rt", "pull", "container", "network", "image", "info")
				}
				flagDriver = tc.driver
				err := command.run(t)
				wantNotRun(t, marker)
				wantRuntimeRefused(t, err, setting, value)
			})
		}
	}
}

// An unsupported value in effect is the error. Neither the level below it
// nor detection is tried in its place (GLOBAL-10).
func TestUnsupportedRuntimeHasNoFallback(t *testing.T) {
	for _, command := range runtimeCommands {
		t.Run(command.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			project := inConfigFiles(t, cliFile("podman"), cliFile("./rt"))
			shipPrograms(t, project, marker, "rt")
			// A finch that answers detection, on a PATH with nothing else.
			bin := t.TempDir()
			shipPrograms(t, bin, marker, "finch")
			t.Setenv("PATH", bin)
			err := command.run(t)
			wantNotRun(t, marker)
			wantRuntimeRefused(t, err, "[runtime] cli in warden.toml", "./rt")
		})
	}
}

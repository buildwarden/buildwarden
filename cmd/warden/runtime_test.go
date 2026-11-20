package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/buildwarden/buildwarden/driver/container"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// The three levels that can name the container runtime, lowest first.
var runtimeLevels = []string{"file", "env", "flag"}

// inProjectFile is what the error calls [runtime] cli of the working
// directory's warden.toml: the key, in the file by its path.
func inProjectFile(t *testing.T) string {
	t.Helper()
	return "[runtime] cli in " + configPath(t)
}

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

// nameRuntime runs the test from a fresh working directory with the runtime
// named v at one level and nowhere else. It returns what the error calls
// that level. The flag's value is a copy, so that it shares no memory with a
// string constant of the test binary.
func nameRuntime(t *testing.T, level, v string) string {
	t.Helper()
	switch level {
	case "file":
		inConfigFiles(t, cliFile(v))
		return inProjectFile(t)
	case "env":
		inConfigFiles(t, "")
		t.Setenv("WARDEN_CTR_CLI", v)
		return "WARDEN_CTR_CLI"
	case "flag":
		inConfigFiles(t, "")
		runtimeFlag = strings.Clone(v)
		return "--runtime"
	}
	t.Fatalf("unknown level %q", level)
	return ""
}

// inRuntimeLevels runs the test with the runtime named at each level whose
// value is not empty; file is the config file's body.
func inRuntimeLevels(t *testing.T, file, env, flag string) {
	t.Helper()
	inConfigFiles(t, file)
	t.Setenv("WARDEN_CTR_CLI", env)
	runtimeFlag = flag
}

// runtimeFlag is --runtime as a test gives it; warden hands it to the
// container driver in args.
var runtimeFlag string

// containerSettings are the container driver's settings for run as the kit
// resolves them: from the config file resolveConfig chose, runtimeFlag and
// the environment.
func containerSettings(t *testing.T) (driver.Settings, error) {
	t.Helper()
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	var args []string
	if runtimeFlag != "" {
		args = []string{"--runtime=" + runtimeFlag}
	}
	return drivercmd.Resolve("container", container.Settings, drivercmd.CmdRun, args,
		absConfigFile(cfg), os.Environ())
}

// resolvedRuntime is what a command that uses the container runtime gets:
// the container driver's settings, through resolveRuntime.
func resolvedRuntime(t *testing.T) (string, error) {
	t.Helper()
	set, err := containerSettings(t)
	if err != nil {
		return "", err
	}
	return resolveRuntime(set)
}

// wantRuntime checks that the runtime in effect resolves to want, and
// returns what resolveRuntime returned.
func wantRuntime(t *testing.T, want string) string {
	t.Helper()
	got, err := resolvedRuntime(t)
	if err != nil || got != want {
		t.Fatalf("resolveRuntime = %q, %v; want %q", got, err, want)
	}
	return got
}

// wantRefusal checks that err is the refusal of v as named at setting: a
// FixableError, which main exits 1 on (UX-09), whose message starts with the
// setting and the quoted value. It returns the message and the fix.
func wantRefusal(t *testing.T, err error, setting, v string) (msg, fix string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError refusing %q", err, v)
	}
	var es exitStatusError
	if errors.As(err, &es) {
		t.Errorf("the refusal carries exit status %d, want main's exit 1", int(es))
	}
	want := setting + " is " + strconv.Quote(v) + "; "
	if !strings.HasPrefix(fe.Msg, want) {
		t.Errorf("message %q does not start with %q", fe.Msg, want)
	}
	if fe.Fix == "" {
		t.Error("the refusal has no fix")
	}
	return fe.Msg, fe.Fix
}

// wantRuntimeRefused checks that the runtime in effect is refused, as
// wantRefusal does, and that resolveRuntime returns no runtime with it.
func wantRuntimeRefused(t *testing.T, setting, v string) (msg, fix string) {
	t.Helper()
	got, err := resolvedRuntime(t)
	if got != "" {
		t.Errorf("resolveRuntime returned %q with its refusal", got)
	}
	return wantRefusal(t, err, setting, v)
}

func TestSupportedRuntimeFromEverySource(t *testing.T) {
	for i, name := range []string{"finch", "docker", "podman"} {
		for _, level := range runtimeLevels {
			t.Run(name+" from "+level, func(t *testing.T) {
				nameRuntime(t, level, name)
				got := wantRuntime(t, name)
				// The string that is run is the list's own, not an equal one
				// that a flag, a variable or a file supplied.
				member := containerRuntimes[i]
				if unsafe.StringData(got) != unsafe.StringData(member) {
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
				wantRuntimeRefused(t, setting, v)
			})
		}
	}
}

// writeScript writes an executable shell script.
func writeScript(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
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
		writeScript(t, filepath.Join(bin, name), script)
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

// The value in effect is the highest level that names a runtime. A level
// below it is not read, so an unsupported value there is not an error; an
// unsupported value in effect is one, and no lower level is used in its place.
func TestRuntimeOverridesComeFirst(t *testing.T) {
	const bad, inFile = "./rt", "the config file"
	for _, tc := range []struct {
		name            string
		file, env, flag string
		want            string // the runtime, or
		refused         string // the setting the refusal names
	}{
		{name: "flag over an unsupported file value",
			file: bad, flag: "docker", want: "docker"},
		{name: "variable over an unsupported file value",
			file: bad, env: "podman", want: "podman"},
		{name: "flag over an unsupported variable",
			env: bad, flag: "finch", want: "finch"},
		{name: "unsupported flag over a supported file value",
			file: "docker", flag: bad, refused: "--runtime"},
		{name: "unsupported variable over a supported file value",
			file: "docker", env: bad, refused: "WARDEN_CTR_CLI"},
		{name: "unsupported file value with no level over it",
			file: bad, refused: inFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := ""
			if tc.file != "" {
				file = cliFile(tc.file)
			}
			inRuntimeLevels(t, file, tc.env, tc.flag)
			switch tc.refused {
			case "":
				wantRuntime(t, tc.want)
			case inFile:
				wantRuntimeRefused(t, inProjectFile(t), bad)
			default:
				wantRuntimeRefused(t, tc.refused, bad)
			}
		})
	}
}

// An empty flag and an empty variable are not set. cli = "" in the file sets
// nothing either: the runtime is detected.
func TestEmptyRuntime(t *testing.T) {
	t.Run("an empty flag and variable leave the file's value", func(t *testing.T) {
		inRuntimeLevels(t, cliFile("podman"), "", "")
		wantRuntime(t, "podman")
	})
	t.Run(`cli = "" in the file is detected`, func(t *testing.T) {
		inRuntimeLevels(t, cliFile(""), "", "")
		t.Setenv("PATH", t.TempDir()) // detection finds nothing, on any host
		got, err := resolvedRuntime(t)
		if got != "" || err == nil ||
			!strings.Contains(err.Error(), "no container runtime found") {
			t.Fatalf("resolveRuntime = %q, %v; want detection's error", got, err)
		}
	})
}

// The permanent error names the level that set the value and gives the fix
// for that level.
func TestUnsupportedRuntimeMessage(t *testing.T) {
	const (
		supports = `is "dokcer"; the container driver supports finch, docker and podman`
		fileFix  = "set it to finch, docker or podman, or remove it; " +
			"--runtime with one of those replaces it for one command"
		file = "/src/repo/warden.toml"
	)
	for _, tc := range []struct{ source, msg, fix string }{
		{cliFromFlag, "--runtime " + supports,
			"pass --runtime finch, docker or podman, or leave it out"},
		{cliFromEnv, "WARDEN_CTR_CLI " + supports,
			"set WARDEN_CTR_CLI to finch, docker or podman, or unset it"},
		{file + " [runtime] cli", "[runtime] cli in " + file + " " + supports, fileFix},
	} {
		err := unsupportedRuntime("dokcer", tc.source)
		var fe *driver.FixableError
		if !errors.As(err, &fe) {
			t.Fatalf("%s: %v is not a FixableError", tc.source, err)
		}
		if fe.Msg != tc.msg || fe.Fix != tc.fix {
			t.Errorf("%s:\ngot  %q\n     %q\nwant %q\n     %q",
				tc.source, fe.Msg, fe.Fix, tc.msg, tc.fix)
		}
	}
}

// The value may come from a file someone else wrote. It is in the message
// once, quoted, which keeps its control characters out of the terminal, and
// nothing else in the message or the fix is taken from it: not the directory
// of a path, which a fix must not recommend for PATH.
func TestRuntimeErrorPrintsTheValueOnlyQuoted(t *testing.T) {
	const dir = "/zq-top/zq-dir\x1b[31m\nzq-line"
	values := []string{dir + "/docker", dir + "/rt", "zq-name\x1b[31m\nzq-line"}
	for i, v := range values {
		for _, level := range runtimeLevels {
			t.Run(fmt.Sprintf("value %d from %s", i, level), func(t *testing.T) {
				setting := nameRuntime(t, level, v)
				msg, fix := wantRuntimeRefused(t, setting, v)
				wantOnlyQuoted(t, msg, fix, v)
			})
		}
	}
}

// wantOnlyQuoted checks that msg holds v once, quoted, and that the rest of
// it and fix hold neither a control character nor any other piece of v.
func wantOnlyQuoted(t *testing.T, msg, fix, v string) {
	t.Helper()
	quoted := strconv.Quote(v)
	if n := strings.Count(msg, quoted); n != 1 {
		t.Fatalf("the message holds the quoted value %d times: %q", n, msg)
	}
	rest := strings.Replace(msg, quoted, "", 1) + " " + fix
	for _, part := range []string{"\x1b", "\n", "zq-"} {
		if strings.Contains(rest, part) {
			t.Errorf("outside the quoted value the error holds %q: %q", part, rest)
		}
	}
}

// wantCaseRefusal checks that err is Decision 24's refusal of the container
// driver's runtime key spelt as spelling: a FixableError that names the
// spelling and the key to write (DRV-C12, Behaviour 19).
func wantCaseRefusal(t *testing.T, err error, spelling string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError refusing %s", err, spelling)
	}
	want := ": " + spelling + ": keys are case-sensitive, and the container driver " +
		"reads [driver.container] cli (old name [runtime] cli)"
	if !strings.Contains(fe.Msg, want) || !strings.HasPrefix(fe.Fix,
		"write it as [driver.container] cli") && !strings.HasPrefix(fe.Fix, "keep one") {
		t.Errorf("refusal %q, fix %q; want %q and the key to write", fe.Msg, fe.Fix,
			want)
	}
}

// main's decoder filled [runtime] cli from a key in any letter case, in a
// table, a dotted key or an inline table. The container driver reads it
// exactly, and refuses another spelling with the key to write (Decision 24,
// DRV-C12): before the runtime's check, so nothing it names is run, and a
// supported runtime in another case is refused too. Two spellings in one
// table are refused every time, where main took one of the two at random.
func TestRuntimeKeyInOtherSpellings(t *testing.T) {
	spellings := map[string]struct{ format, spelling string }{
		"[runtime] CLI":   {"[runtime]\nCLI = %s\n", "[runtime] CLI"},
		"[runtime] Cli":   {"[runtime]\nCli = %s\n", "[runtime] Cli"},
		"[Runtime] cli":   {"[Runtime]\ncli = %s\n", "[Runtime] cli"},
		"[RUNTIME] CLI":   {"[RUNTIME]\nCLI = %s\n", "[RUNTIME] CLI"},
		"dotted key":      {"runtime.CLI = %s\n", "[runtime] CLI"},
		"an inline table": {"Runtime = { CLI = %s }\n", "[Runtime] CLI"},
	}
	for name, sp := range spellings {
		for _, v := range []string{"./rt", "sh"} {
			t.Run(name+" "+v, func(t *testing.T) {
				inConfigFiles(t, fmt.Sprintf(sp.format, tomlString(v)))
				got, err := resolvedRuntime(t)
				if got != "" {
					t.Errorf("resolveRuntime returned %q with the refusal",
						got)
				}
				wantCaseRefusal(t, err, sp.spelling)
			})
		}
	}
	t.Run("[Runtime] CLI naming a supported runtime is refused", func(t *testing.T) {
		inConfigFiles(t, "[Runtime]\nCLI = \"podman\"\n")
		_, err := resolvedRuntime(t)
		wantCaseRefusal(t, err, "[Runtime] CLI")
	})
	t.Run("both spellings in one table", func(t *testing.T) {
		inConfigFiles(t, "[runtime]\ncli = \"docker\"\nCLI = \"./rt\"\n")
		for range 200 {
			got, err := resolvedRuntime(t)
			if got != "" {
				t.Fatalf("resolveRuntime = %q, want the refusal", got)
			}
			wantCaseRefusal(t, err, "[runtime] holds cli and CLI")
		}
	})
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

// runtimeCommands are the commands that resolve the container runtime, and
// the container driver's probe, the call through which build and shell do.
var runtimeCommands = []struct {
	name string
	run  func(t *testing.T) error
}{
	{"the container driver's probe", func(t *testing.T) error {
		cfg, err := resolveConfig()
		if err != nil {
			t.Fatalf("resolveConfig: %v", err)
		}
		probe := &driver.Start{Contract: 1, Probe: true, Mode: driver.ModeBuild,
			GuestOS: "linux", ConfigFile: absConfigFile(cfg), Args: runtimeArgs()}
		_, err = callLinkedRun(context.Background(), linkedDrivers["container"],
			"container", cfg, &driver.BuildRequest{}, probe, nil)
		return err
	}},
	{"build", func(t *testing.T) error { return runLine(t, "build") }},
	{"shell", func(t *testing.T) error { return runLine(t, "shell") }},
	{"clean", func(t *testing.T) error { return runLine(t, "clean") }},
	{"doctor", func(t *testing.T) error { return runLine(t, "doctor") }},
}

// runtimeArgs are the container driver's args for runtimeFlag.
func runtimeArgs() []string {
	if runtimeFlag == "" {
		return []string{}
	}
	return []string{"--runtime=" + runtimeFlag}
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

// addDockerfile makes the working directory a build context, so that build
// and shell get past their arguments to the runtime.
func addDockerfile(t *testing.T) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "Dockerfile", "FROM alpine\n")
}

// The security fix. A build context ships a program and a warden.toml that
// names it as the container runtime, by path or as an interpreter that runs
// the file named like warden's first argument. Before the fix clean ran the
// program, doctor accepted it, and warden handed it to the driver, which
// build and shell then ran; now each refuses it and nothing is run. The same
// holds for a program the flag or the variable names. Since the driver
// contract, [runtime] CLI is refused for its letter case before its value is
// looked at (Behaviour 19), and clean under qemu or vz no longer runs the
// container runtime's cleanup, so it reads no runtime (Behaviour 5).
func TestUnsupportedRuntimeIsNeverRun(t *testing.T) {
	const viaShell = "sh" // runs ./pull, ./container, ... from the project
	shipped := []string{"rt", "pull", "container", "network", "image", "info"}
	for _, tc := range []struct {
		name      string
		project   string // the project's file
		elsewhere string // or the level that names the shipped path
		driver    string // --driver
		value     string // the value refused, when not the path
		onlyClean bool
		// caseRefused: refused for the key's letter case; unread: not read.
		caseRefused, unread bool
	}{
		{name: `cli = "./rt"`, project: cliFile("./rt"), value: "./rt"},
		{name: `CLI = "./rt"`, project: "[runtime]\nCLI = \"./rt\"\n", value: "./rt",
			caseRefused: true},
		{name: `cli = "sh"`, project: cliFile(viaShell), value: viaShell},
		{name: `cli = "./rt" beside driver = "qemu"`, value: "./rt", onlyClean: true,
			project: "[runtime]\ncli = \"./rt\"\ndriver = \"qemu\"\n", unread: true},
		{name: `cli = "./rt" with --driver vz`, value: "./rt", onlyClean: true,
			project: cliFile("./rt"), driver: "vz", unread: true},
		{name: "the program's path in --runtime", elsewhere: "flag"},
		{name: "the program's path in WARDEN_CTR_CLI", elsewhere: "env"},
	} {
		for _, command := range runtimeCommands {
			if tc.onlyClean && command.name != "clean" {
				continue
			}
			t.Run(tc.name+", "+command.name, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "ran")
				program := shipPrograms(t, t.TempDir(), marker, "rt")
				var setting string
				value := tc.value
				if tc.elsewhere != "" {
					setting = nameRuntime(t, tc.elsewhere, program)
					value = program
				} else {
					project := inConfigFiles(t, tc.project)
					shipPrograms(t, project, marker, shipped...)
					setting = inProjectFile(t)
				}
				addDockerfile(t)
				flagDriver = tc.driver
				err := command.run(t)
				wantNotRun(t, marker)
				switch {
				case tc.unread && err != nil:
					t.Errorf("err = %v, want none: no runtime is read", err)
				case tc.unread:
				case tc.caseRefused:
					wantCaseRefusal(t, err, "[runtime] CLI")
				default:
					wantRefusal(t, err, setting, value)
				}
			})
		}
	}
}

// An unsupported value in effect is the error. Detection is not tried in its
// place (GLOBAL-10).
func TestUnsupportedRuntimeHasNoFallback(t *testing.T) {
	for _, command := range runtimeCommands {
		t.Run(command.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			project := inConfigFiles(t, cliFile("./rt"))
			shipPrograms(t, project, marker, "rt")
			// A finch that answers detection, on a PATH with nothing else.
			bin := t.TempDir()
			shipPrograms(t, bin, marker, "finch")
			t.Setenv("PATH", bin)
			addDockerfile(t)
			err := command.run(t)
			wantNotRun(t, marker)
			wantRefusal(t, err, inProjectFile(t), "./rt")
		})
	}
}

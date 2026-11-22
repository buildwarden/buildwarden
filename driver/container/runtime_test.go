package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// These are runtime-cli-project-path's tests, moved from cmd/warden with the
// container driver's runtime (K1 task 22): the runtime is read through the
// kit, from --runtime in args, WARDEN_CTR_CLI and the config file, each of
// [driver.container] cli and its old name [runtime] cli.

// The three levels that can name the container runtime, lowest first.
var runtimeLevels = []string{"file", "env", "flag"}

// The two keys a file names the runtime by: the key, and its old name, read
// until 1.0.
var runtimeKeys = []struct{ table, label string }{
	{"runtime", "[runtime] cli"},
	{"driver.container", "[driver.container] cli"},
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

func cliFile(table, v string) string { return "[" + table + "]\ncli = " + tomlString(v) + "\n" }

// runtimeTest is one test's levels: a config file in a project directory of
// its own, which is the working directory, the variable and the flag.
type runtimeTest struct {
	t       *testing.T
	project string
	file    string // the config file's path, "" for none
	flag    string
	environ []string
}

func newRuntimeTest(t *testing.T) *runtimeTest {
	t.Helper()
	r := &runtimeTest{t: t, project: t.TempDir()}
	t.Chdir(r.project)
	t.Setenv("WARDEN_CTR_CLI", "")
	t.Setenv("WARDEN_IMAGE", "")
	return r
}

// configure writes body as the project's warden.toml; empty is no file.
func (r *runtimeTest) configure(body string) {
	r.t.Helper()
	if body == "" {
		r.file = ""
		return
	}
	r.file = filepath.Join(r.project, "warden.toml")
	if err := os.WriteFile(r.file, []byte(body), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

// inFile is what the error calls key of the project's warden.toml.
func (r *runtimeTest) inFile(label string) string { return label + " in " + r.file }

// name names the runtime v at one level, in the table given for the file,
// and nowhere else, and returns what the error calls that level. The flag's
// value is a copy, so that it shares no memory with a string constant of
// the test binary.
func (r *runtimeTest) name(level, table, label, v string) string {
	r.t.Helper()
	switch level {
	case "file":
		r.configure(cliFile(table, v))
		return r.inFile(label)
	case "env":
		r.t.Setenv("WARDEN_CTR_CLI", v)
		return "WARDEN_CTR_CLI"
	case "flag":
		r.flag = strings.Clone(v)
		return "--runtime"
	}
	r.t.Fatalf("unknown level %q", level)
	return ""
}

func (r *runtimeTest) args() []string {
	if r.flag == "" {
		return []string{}
	}
	return []string{"--runtime=" + r.flag}
}

// settings are the driver's settings for command, as the kit resolves them.
func (r *runtimeTest) settings(command string) (driver.Settings, error) {
	return drivercmd.Resolve("container", Settings, command, r.args(), r.file, os.Environ())
}

// resolved is what a command that uses the runtime gets.
func (r *runtimeTest) resolved() (string, error) {
	set, err := r.settings(drivercmd.CmdRun)
	if err != nil {
		return "", err
	}
	return resolveRuntime(set)
}

// wantRuntime checks that the runtime in effect resolves to want, and
// returns what resolveRuntime returned.
func (r *runtimeTest) wantRuntime(want string) string {
	r.t.Helper()
	got, err := r.resolved()
	if err != nil || got != want {
		r.t.Fatalf("resolveRuntime = %q, %v; want %q", got, err, want)
	}
	return got
}

// wantRefusal checks that err is the refusal of v as named at setting: a
// FixableError whose message starts with the setting and the quoted value.
// It returns the message and the fix.
func wantRefusal(t *testing.T, err error, setting, v string) (msg, fix string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError refusing %q", err, v)
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

// wantRefused checks that the runtime in effect is refused, as wantRefusal
// does, and that resolveRuntime returns no runtime with it.
func (r *runtimeTest) wantRefused(setting, v string) (msg, fix string) {
	r.t.Helper()
	got, err := r.resolved()
	if got != "" {
		r.t.Errorf("resolveRuntime returned %q with its refusal", got)
	}
	return wantRefusal(r.t, err, setting, v)
}

// forEachLevel runs fn for each level and, at the file's, for each key.
func forEachLevel(t *testing.T, name string, fn func(t *testing.T, r *runtimeTest,
	level, table, label string)) {
	for _, level := range runtimeLevels {
		keys := runtimeKeys
		if level != "file" {
			keys = keys[:1]
		}
		for _, k := range keys {
			sub := name + " from " + level
			if level == "file" {
				sub += " " + k.label
			}
			t.Run(sub, func(t *testing.T) {
				fn(t, newRuntimeTest(t), level, k.table, k.label)
			})
		}
	}
}

func TestSupportedRuntimeFromEverySource(t *testing.T) {
	for i, name := range []string{"finch", "docker", "podman"} {
		forEachLevel(t, name, func(t *testing.T, r *runtimeTest, level, table,
			label string) {
			r.name(level, table, label, name)
			got := r.wantRuntime(name)
			// The string that is run is the list's own, not an equal one
			// that a flag, a variable or a file supplied.
			if unsafe.StringData(got) != unsafe.StringData(runtimes[i]) {
				t.Errorf("resolveRuntime returned a string equal to %q that is "+
					"not runtimes' member", name)
			}
		})
	}
}

func TestUnsupportedRuntimeRefusedFromEverySource(t *testing.T) {
	for _, v := range []string{
		"./docker", "tools/docker", "/usr/bin/docker", `..\x.exe`,
		`C:\x\docker.exe`, "C:docker", "docker.exe", "Docker", " docker",
		"docker ", "nerdctl", "sh", "sudo docker", " ",
	} {
		forEachLevel(t, strconv.Quote(v), func(t *testing.T, r *runtimeTest, level, table,
			label string) {
			setting := r.name(level, table, label, v)
			r.wantRefused(setting, v)
		})
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
	if !slices.Equal(runtimes, want) {
		t.Fatalf("runtimes = %v, want %v", runtimes, want)
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	bin, probed := t.TempDir(), filepath.Join(t.TempDir(), "probed")
	for _, name := range want {
		script := fmt.Sprintf("#!/bin/sh\necho %s \"$*\" >> '%s'\nexit 1\n", name, probed)
		writeScript(t, filepath.Join(bin, name), script)
	}
	t.Setenv("PATH", bin)
	if rt, err := detectRuntime(); err == nil {
		t.Fatalf("detectRuntime = %q with no runtime answering", rt)
	}
	got, err := os.ReadFile(probed)
	if err != nil {
		t.Fatal(err)
	}
	wantRan := "finch version\ndocker version --format {{.Server.Version}}\n" +
		"podman version --format {{.Server.Version}}\n"
	if string(got) != wantRan {
		t.Errorf("detection ran %q, want the version query of finch, docker, "+
			"podman in order", got)
	}
}

// installRuntime puts a fake runtime named name on a PATH of its own; it
// gives r to its version query.
func installRuntime(t *testing.T, name string, r reply) {
	t.Helper()
	dir := t.TempDir()
	writeRuntime(t, dir, name, r)
	onlyOnPath(t, dir)
}

func TestDetectRuntimeExplainsTheFix(t *testing.T) {
	t.Run("none installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := detectRuntime()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "install") {
			t.Fatalf("detectRuntime = %v, want a FixableError with an install hint",
				err)
		}
	})
	t.Run("installed but stopped", func(t *testing.T) {
		installRuntime(t, "finch", fails)
		_, err := detectRuntime()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "finch vm start") {
			t.Fatalf("detectRuntime = %v, want a FixableError saying to start "+
				"finch's VM", err)
		}
	})
	t.Run("running", func(t *testing.T) {
		installRuntime(t, "podman", answers)
		if rt, err := detectRuntime(); err != nil || rt != "podman" {
			t.Fatalf("detectRuntime = %q, %v; want podman", rt, err)
		}
	})
}

// The value in effect is the highest level that names a runtime. A level
// below it is not read, so an unsupported value there is not an error; an
// unsupported value in effect is one, and no lower level is used in its place.
func TestRuntimeOverridesComeFirst(t *testing.T) {
	const bad, inFile = "./rt", "the config file"
	for _, k := range runtimeKeys {
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
			t.Run(k.label+" "+tc.name, func(t *testing.T) {
				r := newRuntimeTest(t)
				if tc.file != "" {
					r.configure(cliFile(k.table, tc.file))
				}
				t.Setenv("WARDEN_CTR_CLI", tc.env)
				r.flag = tc.flag
				switch tc.refused {
				case "":
					r.wantRuntime(tc.want)
				case inFile:
					r.wantRefused(r.inFile(k.label), bad)
				default:
					r.wantRefused(tc.refused, bad)
				}
			})
		}
	}
}

// An empty flag and an empty variable are not set. cli = "" in the file sets
// nothing either: the runtime is detected.
func TestEmptyRuntime(t *testing.T) {
	for _, k := range runtimeKeys {
		t.Run(k.label+": an empty flag and variable leave the file's value",
			func(t *testing.T) {
				r := newRuntimeTest(t)
				r.configure(cliFile(k.table, "podman"))
				r.wantRuntime("podman")
			})
		t.Run(k.label+` = "" is detected`, func(t *testing.T) {
			r := newRuntimeTest(t)
			r.configure(cliFile(k.table, ""))
			t.Setenv("PATH", t.TempDir()) // detection finds nothing, on any host
			got, err := r.resolved()
			if got != "" || err == nil ||
				!strings.Contains(err.Error(), "no container runtime found") {
				t.Fatalf("resolveRuntime = %q, %v; want detection's error", got,
					err)
			}
		})
	}
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
		{file + " [driver.container] cli", "[driver.container] cli in " + file + " " +
			supports, fileFix},
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
		forEachLevel(t, fmt.Sprintf("value %d", i), func(t *testing.T, r *runtimeTest,
			level, table, label string) {
			setting := r.name(level, table, label, v)
			msg, fix := r.wantRefused(setting, v)
			wantOnlyQuoted(t, msg, fix, v)
		})
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

// wantCaseRefusal checks that err is Decision 24's refusal of the runtime
// key spelt as spelling: a FixableError that names the spelling and the key
// to write (DRV-C12, Behaviour 19).
func wantCaseRefusal(t *testing.T, err error, spelling string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError refusing %s", err, spelling)
	}
	// The old name is named beside the key when the spelling is the old
	// name's, folded.
	want := ": " + spelling + ": keys are case-sensitive, and the container driver " +
		"reads [driver.container] cli"
	if strings.EqualFold(strings.Fields(spelling)[0], "[runtime]") {
		want += " (old name [runtime] cli)"
	}
	if !strings.Contains(fe.Msg, want) || !strings.HasPrefix(fe.Fix,
		"write it as [driver.container] cli") && !strings.HasPrefix(fe.Fix, "keep one") {
		t.Errorf("refusal %q, fix %q; want %q and the key to write", fe.Msg, fe.Fix,
			want)
	}
}

// warden's decoder once filled [runtime] cli from a key in any letter case,
// in a table, a dotted key or an inline table. The container driver reads it
// exactly, and refuses another spelling with the key to write (Decision 24,
// DRV-C12): before the runtime's check, so nothing it names is run, and a
// supported runtime in another case is refused too. Two spellings in one
// table are refused every time. The same holds for [driver.container] cli.
func TestRuntimeKeyInOtherSpellings(t *testing.T) {
	spellings := map[string]struct{ format, spelling string }{
		"[runtime] CLI":   {"[runtime]\nCLI = %s\n", "[runtime] CLI"},
		"[runtime] Cli":   {"[runtime]\nCli = %s\n", "[runtime] Cli"},
		"[Runtime] cli":   {"[Runtime]\ncli = %s\n", "[Runtime] cli"},
		"[RUNTIME] CLI":   {"[RUNTIME]\nCLI = %s\n", "[RUNTIME] CLI"},
		"dotted key":      {"runtime.CLI = %s\n", "[runtime] CLI"},
		"an inline table": {"Runtime = { CLI = %s }\n", "[Runtime] CLI"},
		"[driver.container] CLI": {"[driver.container]\nCLI = %s\n",
			"[driver.container] CLI"},
		"[Driver.Container] cli": {"[Driver.Container]\ncli = %s\n",
			"[Driver.Container] cli"},
		"[driver.Container] Cli": {"[driver.Container]\nCli = %s\n",
			"[driver.Container] Cli"},
		"dotted driver key": {"driver.container.CLI = %s\n", "[driver.container] CLI"},
		"a driver inline table": {"Driver = { container = { cli = %s } }\n",
			"[Driver.container] cli"},
	}
	for name, sp := range spellings {
		for _, v := range []string{"./rt", "sh"} {
			t.Run(name+" "+v, func(t *testing.T) {
				r := newRuntimeTest(t)
				r.configure(fmt.Sprintf(sp.format, tomlString(v)))
				got, err := r.resolved()
				if got != "" {
					t.Errorf("resolveRuntime returned %q with the refusal",
						got)
				}
				wantCaseRefusal(t, err, sp.spelling)
			})
		}
	}
	for _, table := range []string{"[Runtime]", "[Driver.Container]"} {
		t.Run(table+" CLI naming a supported runtime is refused", func(t *testing.T) {
			r := newRuntimeTest(t)
			r.configure(table + "\nCLI = \"podman\"\n")
			_, err := r.resolved()
			wantCaseRefusal(t, err, table+" CLI")
		})
	}
	for _, k := range runtimeKeys {
		t.Run("both spellings in "+k.table, func(t *testing.T) {
			r := newRuntimeTest(t)
			r.configure("[" + k.table + "]\ncli = \"docker\"\nCLI = \"./rt\"\n")
			for range 200 {
				got, err := r.resolved()
				if got != "" {
					t.Fatalf("resolveRuntime = %q, want the refusal", got)
				}
				wantCaseRefusal(t, err, "["+k.table+"] holds cli and CLI")
			}
		})
	}
}

// runtimeCommands are the container driver's commands that resolve the
// runtime, called through the kit as warden calls them: the probe of run
// (build and shell), doctor, clean, and reap and attach of a build with no
// state file, which resolve it from the build's settings.
var runtimeCommands = []struct {
	name string
	call func(r *runtimeTest) error
}{
	{"the probe of run", func(r *runtimeTest) error {
		return r.call(drivercmd.CmdRun, &drivercmd.Request{Start: &driver.Start{
			Contract: 1, Probe: true, Mode: driver.ModeBuild, GuestOS: "linux",
			ConfigFile: r.file, Args: r.args()}})
	}},
	{"doctor", func(r *runtimeTest) error {
		return r.call(drivercmd.CmdDoctor, &drivercmd.Request{Contract: 1,
			ConfigFile: r.file, Args: r.args()})
	}},
	{"clean", func(r *runtimeTest) error {
		return r.call(drivercmd.CmdClean, &drivercmd.Request{Contract: 1,
			ConfigFile: r.file, Args: r.args(), KeepIDs: []string{}})
	}},
	{"reap", func(r *runtimeTest) error {
		return r.call(drivercmd.CmdReap, &drivercmd.Request{Contract: 1,
			BuildID: "k3v9x2ab", ConfigFile: r.file, Args: r.args(),
			StateFile: filepath.Join(r.t.TempDir(), "none.state")})
	}},
	{"attach", func(r *runtimeTest) error {
		return r.call(drivercmd.CmdAttach, &drivercmd.Request{Contract: 1,
			BuildID: "k3v9x2ab", ConfigFile: r.file, Args: r.args(),
			StateFile: filepath.Join(r.t.TempDir(), "none.state")})
	}},
}

// call runs command of the container driver with req, through the kit.
func (r *runtimeTest) call(command string, req *drivercmd.Request) error {
	var out bytes.Buffer
	req.Streams = drivercmd.Streams{Stdin: strings.NewReader(""), Stdout: &out,
		Stderr: &out}
	_, err := drivercmd.Call(context.Background(), Driver(), command, req, nil)
	return err
}

// shipPrograms writes executables of the given names into dir; each creates
// marker when it is run.
func shipPrograms(t *testing.T, dir, marker string, names ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	script := fmt.Sprintf("#!/bin/sh\necho ran >> '%s'\n", marker)
	for _, name := range names {
		writeScript(t, filepath.Join(dir, name), script)
	}
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
// the file named like the runtime's first argument. Before the fix clean ran
// the program, doctor accepted it, and the driver ran it in build and shell;
// now each command refuses it and nothing is run. The same holds for a
// program the flag or the variable names, and for [driver.container] cli.
// [runtime] CLI is refused for its letter case before its value is looked at
// (Behaviour 19).
func TestUnsupportedRuntimeIsNeverRun(t *testing.T) {
	const viaShell = "sh" // runs ./pull, ./container, ... from the project
	shipped := []string{"rt", "pull", "container", "network", "image", "info",
		"version"}
	type project struct {
		name, body, value, label string
		caseRefused              bool
	}
	var projects []project
	for _, k := range runtimeKeys {
		projects = append(projects,
			project{name: k.label + ` = "./rt"`, body: cliFile(k.table, "./rt"),
				value: "./rt", label: k.label},
			project{name: k.label + ` = "sh"`, body: cliFile(k.table, viaShell),
				value: viaShell, label: k.label})
	}
	projects = append(projects,
		project{name: `[runtime] CLI = "./rt"`, body: "[runtime]\nCLI = \"./rt\"\n",
			caseRefused: true},
		project{name: `[driver.container] CLI = "./rt"`,
			body: "[driver.container]\nCLI = \"./rt\"\n", caseRefused: true})
	for _, command := range runtimeCommands {
		for _, p := range projects {
			t.Run(p.name+", "+command.name, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "ran")
				r := newRuntimeTest(t)
				r.configure(p.body)
				shipPrograms(t, r.project, marker, shipped...)
				err := command.call(r)
				wantNotRun(t, marker)
				if p.caseRefused {
					label := strings.TrimSuffix(p.name, ` = "./rt"`)
					wantCaseRefusal(t, err, label)
					return
				}
				wantRefusal(t, err, r.inFile(p.label), p.value)
			})
		}
		for _, level := range []string{"flag", "env"} {
			name := "the program's path from " + level + ", " + command.name
			t.Run(name, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "ran")
				r := newRuntimeTest(t)
				dir := t.TempDir()
				shipPrograms(t, dir, marker, "rt")
				program := filepath.Join(dir, "rt")
				setting := r.name(level, "", "", program)
				err := command.call(r)
				wantNotRun(t, marker)
				wantRefusal(t, err, setting, program)
			})
		}
	}
}

// An unsupported value in effect is the error. Detection is not tried in its
// place (GLOBAL-10).
func TestUnsupportedRuntimeHasNoFallback(t *testing.T) {
	for _, command := range runtimeCommands {
		for _, k := range runtimeKeys {
			t.Run(k.label+", "+command.name, func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "ran")
				r := newRuntimeTest(t)
				r.configure(cliFile(k.table, "./rt"))
				shipPrograms(t, r.project, marker, "rt")
				// A finch that answers detection, on a PATH with nothing else.
				bin := t.TempDir()
				shipPrograms(t, bin, marker, "finch")
				t.Setenv("PATH", bin)
				err := command.call(r)
				wantNotRun(t, marker)
				wantRefusal(t, err, r.inFile(k.label), "./rt")
			})
		}
	}
}

// A value in one of the forms that ran before is refused with the message
// that says what to change, under either key (TRANSITIONAL(runtime-former-
// values)).
func TestFormerValueInEitherKey(t *testing.T) {
	for _, k := range runtimeKeys {
		t.Run(k.label, func(t *testing.T) {
			r := newRuntimeTest(t)
			r.configure(cliFile(k.table, "/usr/local/bin/docker"))
			msg, fix := r.wantRefused(r.inFile(k.label), "/usr/local/bin/docker")
			const asPath = "the container runtime is named, not given as a path"
			if !strings.Contains(msg, asPath) ||
				!strings.HasPrefix(fix, "set it to docker;") {
				t.Errorf("message %q, fix %q: want the former value's", msg, fix)
			}
		})
	}
}

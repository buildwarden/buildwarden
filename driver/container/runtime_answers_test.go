package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// reply is how a fake runtime answers its version query.
type reply int

const (
	// answers prints a server version, as an engine that is up does.
	answers reply = iota
	// fails prints the client's part and exits 1, as docker does when its
	// daemon is down and podman when its machine is stopped.
	fails
	// hangs does not return, as an engine that is starting may not.
	hangs
	// emptyServer exits 0 and prints an empty line: a docker whose Server
	// section is empty.
	emptyServer
	// slow answers after two seconds, as an engine under load may.
	slow
)

// writeRuntime writes a fake runtime name into dir that gives r to its
// version query and exits 2 to any other call, so a test that passes knows
// the query was the one versionQuery names. On Windows it is name.cmd,
// which PATHEXT finds as name.
func writeRuntime(t *testing.T, dir, name string, r reply) {
	t.Helper()
	query := strings.Join(versionQuery(name), " ")
	if runtime.GOOS == "windows" {
		const ping = `"%SystemRoot%\System32\ping.exe" -n `
		body := map[reply]string{
			answers:     "echo 27.3.1\r\nexit 0",
			fails:       "echo Client: Docker Engine\r\nexit 1",
			hangs:       ping + "30 127.0.0.1\r\nexit 0",
			emptyServer: "echo.\r\nexit 0",
			slow:        ping + "3 127.0.0.1 >nul\r\necho 27.3.1\r\nexit 0",
		}[r]
		script := "@echo off\r\nif not \"%*\"==\"" + query + "\" exit 2\r\n" +
			body + "\r\n"
		if err := os.WriteFile(filepath.Join(dir, name+".cmd"), []byte(script),
			0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	body := map[reply]string{
		answers: "echo 27.3.1",
		fails:   "echo 'Client: Docker Engine'\nexit 1",
		// Not exec: the shell is what is killed, and sleep, its child,
		// still holds stdout.
		hangs:       "/bin/sleep 30",
		emptyServer: "echo",
		slow:        "/bin/sleep 2\necho 27.3.1",
	}[r]
	writeScript(t, filepath.Join(dir, name),
		"#!/bin/sh\n[ \"$*\" = '"+query+"' ] || exit 2\n"+body+"\n")
}

// onlyOnPath makes dir the whole PATH, but for the directory Windows runs a
// batch file's interpreter from, and the only directory the driver looks a
// runtime up in. System32 can hold the host's own docker.exe, as the GitHub
// Windows runner's does, which PATH would find for a runtime dir lacks; the
// lookup in dir alone never finds it, and a runtime it finds is the first
// on PATH, so the one run (TEST-04).
func onlyOnPath(t *testing.T, dir string) {
	t.Helper()
	path := dir
	if runtime.GOOS == "windows" {
		path += string(os.PathListSeparator) +
			filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	t.Setenv("PATH", path)
	old := lookPath
	// A name with a separator is looked up as that file alone, with
	// PATHEXT's extensions on Windows.
	lookPath = func(name string) (string, error) {
		return exec.LookPath(filepath.Join(dir, name))
	}
	t.Cleanup(func() { lookPath = old })
}

// The probe asks each runtime for its engine's version, not for `info`,
// and counts it up only when the query succeeds within the timeout and
// prints a version (Decision 26, r4).
func TestRuntimeAnswersItsVersionQuery(t *testing.T) {
	for _, name := range runtimes {
		for _, tc := range []struct {
			what string
			r    reply
			want bool
		}{
			{"answers", answers, true},
			{"fails", fails, false},
			{"hangs", hangs, false},
			{"prints an empty server version", emptyServer, false},
		} {
			t.Run(name+" "+tc.what, func(t *testing.T) {
				wantAnswers(t, name, tc.r, tc.want)
			})
		}
	}
}

// wantAnswers checks runtimeAnswers on a fake runtime name that gives r.
func wantAnswers(t *testing.T, name string, r reply, want bool) {
	t.Helper()
	dir := t.TempDir()
	writeRuntime(t, dir, name, r)
	onlyOnPath(t, dir)
	timeout := 3 * time.Second
	if r == hangs {
		timeout = 200 * time.Millisecond
	}
	start := time.Now()
	if got := runtimeAnswers(name, timeout); got != want {
		t.Errorf("runtimeAnswers(%s) = %v, want %v", name, got, want)
	}
	// The timeout, and the second a child holding stdout is given, with
	// room for a slow runner.
	if d := time.Since(start); r == hangs && d > 10*time.Second {
		t.Errorf("runtimeAnswers took %v on a runtime that hangs", d)
	}
}

// The query per runtime: docker and podman print the server's version
// (podman on Linux, which has no server, falls back to its client's), and
// finch runs its own version, which asks nerdctl in its VM.
func TestVersionQueryPerRuntime(t *testing.T) {
	for name, want := range map[string]string{
		"docker": "docker version --format '{{.Server.Version}}'",
		"podman": "podman version --format '{{.Server.Version}}'",
		"finch":  "finch version",
	} {
		if got := versionCommand(name); got != want {
			t.Errorf("versionCommand(%s) = %q, want %q", name, got, want)
		}
	}
}

// A runtime whose query fails is named with the command that was run.
func TestNotAnsweringNamesTheQuery(t *testing.T) {
	err := notAnswering("finch", "finch, docker")
	want := "container runtime finch, docker is installed but not running " +
		"(`finch version` failed)"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("notAnswering = %v, want %q", err, want)
	}
}

// shortTimeouts sets the probe's and doctor's timeouts for one test.
func shortTimeouts(t *testing.T, answer, doctor time.Duration) {
	t.Helper()
	oldAnswer, oldDoctor := answerTimeout, doctorTimeout
	answerTimeout, doctorTimeout = answer, doctor
	t.Cleanup(func() { answerTimeout, doctorTimeout = oldAnswer, oldDoctor })
}

// Detection passes over an installed runtime that is not up, and takes the
// next one that is.
func TestDetectRuntimeSkipsOneThatIsNotUp(t *testing.T) {
	dir := t.TempDir()
	writeRuntime(t, dir, "finch", fails)
	writeRuntime(t, dir, "podman", fails)
	writeRuntime(t, dir, "docker", answers)
	onlyOnPath(t, dir)
	if rt, err := detectRuntime(); err != nil || rt != "docker" {
		t.Fatalf("detectRuntime = %q, %v; want docker", rt, err)
	}
}

// With docker and podman both installed and answering, and no runtime
// named, detection chooses podman.
func TestDetectRuntimePrefersPodmanOverDocker(t *testing.T) {
	dir := t.TempDir()
	writeRuntime(t, dir, "docker", answers)
	writeRuntime(t, dir, "podman", answers)
	onlyOnPath(t, dir)
	if rt, err := detectRuntime(); err != nil || rt != "podman" {
		t.Fatalf("detectRuntime = %q, %v; want podman", rt, err)
	}
}

// slowTimeouts are timeouts the slow reply falls between: past
// answerTimeout, within doctorTimeout. answerTimeout leaves a reply that
// answers at once a second, for a loaded runner.
func slowTimeouts(t *testing.T) {
	t.Helper()
	shortTimeouts(t, time.Second, 5*time.Second)
}

// fakeRuntimes puts each runtime in replies on a PATH of their own, with
// no runtime named.
func fakeRuntimes(t *testing.T, replies map[string]reply) {
	t.Helper()
	dir := t.TempDir()
	for name, r := range replies {
		writeRuntime(t, dir, name, r)
	}
	onlyOnPath(t, dir)
	t.Setenv("WARDEN_CTR_CLI", "")
	t.Setenv("WARDEN_IMAGE", "")
}

// runRuntime is the runtime run's probe chooses, or its error.
func runRuntime(t *testing.T) (string, error) {
	t.Helper()
	set, err := drivercmd.Resolve("container", Settings, drivercmd.CmdRun, nil, "",
		os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	d, err := checkedRunner(set)
	if err != nil {
		return "", err
	}
	return d.runtime, nil
}

// doctorSays is what doctor prints, or its error.
func doctorSays(t *testing.T) (string, error) {
	t.Helper()
	_, out, err := callContainer(t, drivercmd.CmdDoctor,
		&drivercmd.Request{Contract: 1, Args: []string{}})
	return out, err
}

// Doctor detects as the build does, with answerTimeout per candidate: with
// podman slower than that but within doctorTimeout, and docker fast, both
// choose docker, and doctor does not report a runtime the build skips
// (Decision 26, r4).
func TestDoctorChoosesTheRuntimeTheBuildDoes(t *testing.T) {
	fakeRuntimes(t, map[string]reply{"podman": slow, "docker": answers})
	slowTimeouts(t)
	if rt, err := runRuntime(t); err != nil || rt != "docker" {
		t.Fatalf("run's probe = %q, %v; want docker", rt, err)
	}
	if out, err := doctorSays(t); err != nil || out != "container runtime: docker\n" {
		t.Fatalf("doctor = %v:\n%q; want docker, unblocked", err, out)
	}
}

// With only a slow podman, run passes over it and finds none answering,
// and doctor, re-asking it with doctorTimeout, is blocked with the slow
// hint: podman is up, and a build runs on it when it is named.
func TestDoctorGivesTheSlowHint(t *testing.T) {
	fakeRuntimes(t, map[string]reply{"podman": slow})
	slowTimeouts(t)
	_, err := runRuntime(t)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "installed but not running") {
		t.Fatalf("run's probe = %v; want the slow podman passed over", err)
	}
	_, err = doctorSays(t)
	wantMsg := "container runtime podman is running but answered after more " +
		"than 1 s, so builds skip it"
	wantFix := "name it: --runtime podman, WARDEN_CTR_CLI=podman or " +
		"[driver.container] cli = \"podman\""
	if !errors.As(err, &fe) || fe.Msg != wantMsg || fe.Fix != wantFix {
		t.Fatalf("doctor = %v; want %q, fix %q", err, wantMsg, wantFix)
	}
}

// With two slow runtimes, finch and podman, and docker absent or down,
// doctor names the first in detection's order that answers the re-ask,
// finch, not the last that answers, podman (Decision 26, r4).
func TestDoctorNamesTheFirstSlowRuntime(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replies map[string]reply
	}{
		{"docker absent", map[string]reply{"finch": slow, "podman": slow}},
		{"docker down", map[string]reply{"finch": slow, "podman": slow,
			"docker": fails}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeRuntimes(t, tc.replies)
			slowTimeouts(t)
			_, err := doctorSays(t)
			want := "container runtime finch is running but answered after " +
				"more than 1 s, so builds skip it"
			var fe *driver.FixableError
			if !errors.As(err, &fe) || fe.Msg != want ||
				!strings.Contains(fe.Fix, "--runtime finch") {
				t.Fatalf("doctor = %v; want %q naming finch", err, want)
			}
		})
	}
}

// With every installed runtime down, doctor's re-asks find none, and it
// keeps detection's error and the start hint.
func TestDoctorGivesTheStartHintWhenNoneIsUp(t *testing.T) {
	fakeRuntimes(t, map[string]reply{"podman": fails, "docker": fails})
	slowTimeouts(t)
	_, err := doctorSays(t)
	wantStartHint(t, err, "podman, docker is installed but not running")
}

// wantStartHint checks that err is a fixable error whose message holds msg
// and whose fix is podman's start hint.
func wantStartHint(t *testing.T, err error, msg string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "podman machine start") ||
		!strings.Contains(fe.Msg, msg) {
		t.Fatalf("doctor = %v; want %q with the start hint", err, msg)
	}
}

// Doctor asks the runtime detection chooses once: detection's answer,
// within answerTimeout, is its health check. With none up, it asks each
// installed runtime twice, once in detection and once in the re-ask, and
// no more: its worst case is three queries of answerTimeout and three of
// doctorTimeout.
func TestDoctorAsksEachRuntimeAtMostTwice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	for _, tc := range []struct {
		name string
		exit map[string]int
		want string
	}{
		{"podman up", map[string]int{"finch": 1, "podman": 0, "docker": 0},
			"finch\npodman\n"},
		{"none up", map[string]int{"finch": 1, "podman": 1, "docker": 1},
			"finch\npodman\ndocker\nfinch\npodman\ndocker\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, probed := t.TempDir(), filepath.Join(t.TempDir(), "probed")
			for name, exit := range tc.exit {
				script := fmt.Sprintf(
					"#!/bin/sh\necho %s >> '%s'\necho 27.3.1\nexit %d\n",
					name, probed, exit)
				writeScript(t, filepath.Join(bin, name), script)
			}
			onlyOnPath(t, bin)
			t.Setenv("WARDEN_CTR_CLI", "")
			t.Setenv("WARDEN_IMAGE", "")
			_, _ = doctorSays(t)
			if got, _ := os.ReadFile(probed); string(got) != tc.want {
				t.Errorf("doctor asked %q, want %q", got, tc.want)
			}
		})
	}
}

// Doctor's health check of a named runtime waits doctorTimeout, not the
// probe's answerTimeout, and is blocked with the start hint when the
// runtime does not answer in it.
func TestDoctorWaitsItsOwnTimeoutForANamedRuntime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		r      reply
		doctor time.Duration
		refuse bool
	}{
		{"a slow runtime within doctor's timeout", slow, 5 * time.Second, false},
		{"a runtime that hangs past it", hangs, 200 * time.Millisecond, true},
		{"a runtime whose engine is down", fails, 5 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeRuntimes(t, map[string]reply{"podman": tc.r})
			shortTimeouts(t, 50*time.Millisecond, tc.doctor)
			t.Setenv("WARDEN_CTR_CLI", "podman")
			out, err := doctorSays(t)
			if tc.refuse {
				wantStartHint(t, err, "installed but not running")
			} else if err != nil || out != "container runtime: podman\n" {
				t.Fatalf("doctor = %v:\n%q; want podman, unblocked", err, out)
			}
		})
	}
}

// A runtime whose CLI prints its version and exits 0 while a child it left
// behind holds stdout open is up: Output's WaitDelay ends the wait with
// exec.ErrWaitDelay, which does not make the engine down.
func TestRuntimeAnswersWhileAChildHoldsStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	dir := t.TempDir()
	writeScript(t, filepath.Join(dir, "podman"),
		"#!/bin/sh\necho 27.3.1\n/bin/sleep 5 &\nexit 0\n")
	onlyOnPath(t, dir)
	if !runtimeAnswers("podman", 4*time.Second) {
		t.Fatal("runtimeAnswers = false for a runtime that printed its version " +
			"and exited 0 while its child held stdout")
	}
}

// The build's first engine call on a named runtime (checkEngine) waits
// doctorTimeout, not answerTimeout: a runtime slower than the probe's
// timeout but within doctor's passes, and one slower than doctor's fails.
func TestBuildEngineCheckWaitsDoctorTimeout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		doctor time.Duration
		pass   bool
	}{
		{"within doctor's timeout", 5 * time.Second, true},
		{"past doctor's timeout", 200 * time.Millisecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRuntime(t, dir, "podman", slow)
			onlyOnPath(t, dir)
			shortTimeouts(t, 50*time.Millisecond, tc.doctor)
			t.Setenv("WARDEN_CTR_CLI", "podman")
			t.Setenv("WARDEN_IMAGE", "")
			set, err := drivercmd.Resolve("container", Settings, drivercmd.CmdRun, nil,
				"", os.Environ())
			if err != nil {
				t.Fatal(err)
			}
			err = checkEngine("podman", set)
			if tc.pass && err != nil {
				t.Fatalf("checkEngine = %v; want the slow podman to pass", err)
			}
			var fe *driver.FixableError
			if !tc.pass && (!errors.As(err, &fe) ||
				!strings.Contains(fe.Msg, "installed but not running")) {
				t.Fatalf("checkEngine = %v; want installed but not running", err)
			}
		})
	}
}

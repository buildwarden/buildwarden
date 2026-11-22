package container

import (
	"errors"
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
	// slow answers after a second, as an engine under load may.
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
			slow:        ping + "2 127.0.0.1 >nul\r\necho 27.3.1\r\nexit 0",
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
		slow:        "/bin/sleep 1\necho 27.3.1",
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
	if rt, err := detectRuntime(answerTimeout); err != nil || rt != "docker" {
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
	if rt, err := detectRuntime(answerTimeout); err != nil || rt != "podman" {
		t.Fatalf("detectRuntime = %q, %v; want podman", rt, err)
	}
}

// Doctor is the health check: it waits doctorTimeout, not the probe's
// answerTimeout, for the runtime it resolved, named or detected, and is
// blocked with the start hint when the runtime does not answer in it. Its
// detection gives each candidate doctorTimeout too: every case's runtime
// is slower than answerTimeout.
func TestDoctorWaitsItsOwnTimeout(t *testing.T) {
	for _, tc := range []struct {
		name          string
		r             reply
		doctor        time.Duration
		named, refuse bool
	}{
		{"a slow named runtime within doctor's timeout", slow, 5 * time.Second,
			true, false},
		{"a slow detected runtime within doctor's timeout", slow, 5 * time.Second,
			false, false},
		{"a named runtime that hangs past it", hangs, 200 * time.Millisecond,
			true, true},
		{"a named runtime whose engine is down", fails, 5 * time.Second, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRuntime(t, dir, "podman", tc.r)
			onlyOnPath(t, dir)
			named := ""
			if tc.named {
				named = "podman"
			}
			shortTimeouts(t, 50*time.Millisecond, tc.doctor)
			t.Setenv("WARDEN_CTR_CLI", named)
			t.Setenv("WARDEN_IMAGE", "")
			wantDoctor(t, tc.refuse)
		})
	}
}

// wantDoctor checks that doctor is blocked by a podman that is not running
// when refuse is set, and names podman unblocked when it is not.
func wantDoctor(t *testing.T, refuse bool) {
	t.Helper()
	_, out, err := callContainer(t, drivercmd.CmdDoctor,
		&drivercmd.Request{Contract: 1, Args: []string{}})
	if !refuse {
		if err != nil || out != "container runtime: podman\n" {
			t.Fatalf("doctor = %v:\n%q; want podman, unblocked", err, out)
		}
		return
	}
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "podman machine start") ||
		!strings.Contains(fe.Msg, "installed but not running") {
		t.Fatalf("doctor = %v; want a fixable error with the start hint", err)
	}
}

// A build's detection gives each candidate answerTimeout, not doctor's: the
// slow runtime doctor finds healthy is passed over, and the probe of run
// finds none answering.
func TestRunDetectionSkipsARuntimeSlowerThanItsTimeout(t *testing.T) {
	dir := t.TempDir()
	writeRuntime(t, dir, "podman", slow)
	onlyOnPath(t, dir)
	shortTimeouts(t, 50*time.Millisecond, 5*time.Second)
	t.Setenv("WARDEN_CTR_CLI", "")
	t.Setenv("WARDEN_IMAGE", "")
	set, err := drivercmd.Resolve("container", Settings, drivercmd.CmdRun, nil, "",
		os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	_, err = checkedRunner(set)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "installed but not running") {
		t.Fatalf("run's probe = %v; want the slow podman passed over, none answering",
			err)
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

//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The end-to-end cases that signal warden, the driver or both: a terminal's
// signals and kill(1) have no counterpart for one process on Windows, where
// container-any's native step runs them.

// lockedBuffer is a bytes.Buffer that a test reads while a process writes it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// running is a warden started in the background.
type running struct {
	t      *testing.T
	cmd    *exec.Cmd
	output *lockedBuffer
	done   chan struct{}
}

func (e *e2e) start(extraEnv []string, args ...string) *running {
	e.t.Helper()
	cmd := e.command(extraEnv, args...)
	r := &running{t: e.t, cmd: cmd, output: &lockedBuffer{}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = r.output, r.output
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	go func() {
		_ = cmd.Wait()
		close(r.done)
	}()
	e.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-r.done
	})
	return r
}

// waitFor waits until warden has printed s.
func (r *running) waitFor(s string) {
	r.t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !strings.Contains(r.output.String(), s) {
		if time.Now().After(deadline) {
			r.t.Fatalf("warden did not print %q:\n%s", s, r.output.String())
		}
		select {
		case <-r.done:
			if !strings.Contains(r.output.String(), s) {
				r.t.Fatalf("warden exited without printing %q:\n%s", s,
					r.output.String())
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (r *running) signal(sig os.Signal) {
	r.t.Helper()
	if err := r.cmd.Process.Signal(sig); err != nil {
		r.t.Fatal(err)
	}
}

// wait waits for warden's exit and returns its output and exit code.
func (r *running) wait() (string, int) {
	r.t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Minute):
		r.t.Fatalf("warden did not exit:\n%s", r.output.String())
	}
	return r.output.String(), r.cmd.ProcessState.ExitCode()
}

// children are the PIDs of pid's child processes.
func children(t *testing.T, pid int) []int {
	t.Helper()
	pgrep, err := exec.LookPath("pgrep")
	if err != nil {
		t.Skip("pgrep is not on PATH")
	}
	out, err := exec.Command(pgrep, "-P", strconv.Itoa(pid)).Output()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatal(err)
	}
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(f); err == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

// buildProcesses are the build's driver, a child of warden, and its stub
// relay, a child of the driver; each is killed when the test ends.
func (r *running) buildProcesses() (driverPID, relayPID int) {
	r.t.Helper()
	drivers := children(r.t, r.cmd.Process.Pid)
	if len(drivers) != 1 {
		r.t.Fatalf("warden has %d children, want the driver", len(drivers))
	}
	relays := children(r.t, drivers[0])
	if len(relays) != 1 {
		r.t.Fatalf("the driver has %d children, want the stub relay", len(relays))
	}
	r.t.Cleanup(func() {
		_ = syscall.Kill(drivers[0], syscall.SIGKILL)
		_ = syscall.Kill(relays[0], syscall.SIGKILL)
	})
	return drivers[0], relays[0]
}

// gone waits until pid has exited.
func gone(t *testing.T, pid int, what string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("%s (PID %d) is still running", what, pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A build ended by a signal: Ctrl-C exits 130 and SIGTERM 1, each after the
// driver's teardown; a second Ctrl-C during the teardown kills the driver
// and reaps (DRV-52).
func TestE2ESignals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		signals []os.Signal
		env     []string
		code    int
		says    string
	}{
		{"ctrl-c", []os.Signal{os.Interrupt}, nil, 130, "build interrupted"},
		{"sigterm", []os.Signal{syscall.SIGTERM}, nil, 1, "build terminated (SIGTERM)"},
		{"second ctrl-c", []os.Signal{os.Interrupt, os.Interrupt},
			[]string{"WARDEN_FAKE_TEARDOWN=1m"}, 130, "build interrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newE2E(t)
			e.script("output started", "sleep 1m")
			r := e.start(tc.env, e.buildArgs()...)
			r.waitFor("started")
			_, relay := r.buildProcesses()
			r.signal(tc.signals[0])
			if len(tc.signals) > 1 {
				r.waitFor("press ctrl+c again to force")
				gone(t, relay, "the stub relay")
				r.signal(tc.signals[1])
			}
			out, code := r.wait()
			wantExit(t, code, tc.code, out)
			wantIn(t, "output", out, tc.says)
			e.noLeftovers(0)
		})
	}
}

// A driver killed mid-build is a driver defect: the build fails, and warden
// reaps it (DRV-54).
func TestE2EKilledDriver(t *testing.T) {
	e := newE2E(t)
	e.script("output started", "sleep 1m")
	r := e.start(nil, e.buildArgs()...)
	r.waitFor("started")
	driverPID, _ := r.buildProcesses()
	if err := syscall.Kill(driverPID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	out, code := r.wait()
	wantExit(t, code, 1, out)
	wantIn(t, "output", out, "driver defect", "ended with neither stopped nor failed",
		"build FAILED")
	e.noLeftovers(0)
}

// A warden killed mid-build closes the driver's input; the driver tears
// down by itself and leaves no relay and no work directory (DRV-50, DRV-51).
// warden's own lock, record and stage directory stay for warden clean.
func TestE2EKilledWarden(t *testing.T) {
	e := newE2E(t)
	e.script("output started", "sleep 1m")
	r := e.start(nil, e.buildArgs()...)
	r.waitFor("started")
	driverPID, relayPID := r.buildProcesses()
	r.signal(syscall.SIGKILL)
	r.wait()
	gone(t, driverPID, "the driver")
	gone(t, relayPID, "the stub relay")
	for _, left := range e.leftovers() {
		if strings.HasPrefix(filepath.Base(left), "warden-fake-") {
			t.Errorf("the driver left its work directory %s", left)
		}
	}
}

// warden clean reaps a build whose warden and driver were both killed, from
// its record, and runs the driver's clean with the live builds as keep_ids,
// so a running build's work stays (DRV-27, DRV-58, DRV-60).
func TestE2EClean(t *testing.T) {
	e := newE2E(t)
	e.script("output started", "sleep 1m")
	// The fake ignores the end of its input, so it outlives its warden
	// until it is killed too.
	dead := e.start([]string{"WARDEN_FAKE_FAULT=ignore-end-of-input"}, e.buildArgs()...)
	dead.waitFor("started")
	driverPID, _ := dead.buildProcesses()
	dead.signal(syscall.SIGKILL)
	dead.wait()
	if err := syscall.Kill(driverPID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	gone(t, driverPID, "the driver")

	live := e.start(nil, append([]string{"build", "--driver", "fake", "--tmpdir", e.tmp,
		"--output", filepath.Join(e.root, "live")}, e.ctx)...)
	live.waitFor("started")
	stray := filepath.Join(e.tmp, "warden-fake-abcd1234")
	if err := os.Mkdir(stray, 0o700); err != nil {
		t.Fatal(err)
	}
	before := e.leftovers()

	_, stderr, code := e.run("", nil, "clean", "--driver", "fake")
	wantExit(t, code, 0, stderr)
	wantIn(t, "clean", stderr, "Removing "+stray)
	var kept []string
	for _, left := range e.leftovers() {
		kept = append(kept, filepath.Base(left))
	}
	liveID := buildIDOf(t, live.output.String())
	for _, f := range kept {
		if !strings.Contains(f, liveID) {
			t.Errorf("clean left %s; before it: %v", f, before)
		}
	}
	if !strings.Contains(strings.Join(kept, " "), "warden-fake-"+liveID) {
		t.Errorf("clean removed the running build's work: %v", kept)
	}
	live.signal(os.Interrupt)
	out, code := live.wait()
	wantExit(t, code, 130, out)
	e.noLeftovers(0)
}

// buildIDOf is the build ID the fake printed in output.
func buildIDOf(t *testing.T, output string) string {
	t.Helper()
	_, after, ok := strings.Cut(output, ", build ")
	if !ok || len(after) < 8 {
		t.Fatalf("no build ID in:\n%s", output)
	}
	return after[:8]
}

// warden shell holds the build, attaches to it on warden's terminal and
// exits with the shell's code, then tears the build down (DRV-28, DRV-53).
func TestE2EShell(t *testing.T) {
	e := newE2E(t)
	_, stderr, code := e.run("exit 5\n", nil, "shell", "--driver", "fake", "--tmpdir", e.tmp,
		e.ctx)
	wantExit(t, code, 5, stderr)
	e.noLeftovers(0)
}

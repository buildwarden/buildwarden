package container

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lesiw/ctrctl"

	"github.com/buildwarden/buildwarden/driver"
)

// exitHost is a driver.Host that records the exits reported to it.
type exitHost struct {
	driver.Host
	exits chan string
}

func (h *exitHost) Exited(which driver.ExitedPart, detail string) {
	h.exits <- string(which) + ": " + detail
}

// fakeCLI writes a container runtime CLI, a POSIX shell script whose body is
// cases, a case statement over "$1 $2"; every invocation's arguments are
// appended to the returned log. It is ctrctl's CLI until the test ends.
func fakeCLI(t *testing.T, cases string) (path, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake runtime is a POSIX shell script")
	}
	dir := t.TempDir()
	path, log = filepath.Join(dir, "rt"), filepath.Join(dir, "calls")
	body := "#!/bin/sh\necho \"$*\" >> '" + log + "'\ncase \"$1 $2\" in\n" + cases +
		"\nesac\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := ctrctl.Cli
	ctrctl.Cli = []string{path}
	t.Cleanup(func() { ctrctl.Cli = old })
	return path, log
}

// fakeRuntime installs a container runtime CLI that answers "container wait"
// with wait's output and "logs" with a console line.
func fakeRuntime(t *testing.T, wait string) {
	t.Helper()
	fakeCLI(t, "\"container wait\") "+wait+" ;;\n"+
		"\"logs \"*) echo 'initialize: relay unreachable' ;;")
}

// The build container's exit is reported as exited, with its exit code and
// the last lines of its console (ORCH-01, BOOT-01).
func TestWatchExitReportsTheBuildContainer(t *testing.T) {
	fakeRuntime(t, "echo 3")
	h := &exitHost{exits: make(chan string, 1)}
	b := &build{buildID: "k3v9x2ab", buildContainer: "warden-build-k3v9x2ab"}
	stop := b.watchExit(h)
	select {
	case got := <-h.exits:
		want := "build: warden-io exited with code 3:\ninitialize: relay unreachable"
		if got != want {
			t.Errorf("exited %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no exit was reported")
	}
	stop()
}

// Stopping the watch returns while the container still runs, and nothing is
// reported: the one goroutine that reports has ended, so no exit reaches
// warden after Run returned.
func TestWatchExitStoppedReportsNothing(t *testing.T) {
	fakeRuntime(t, "sleep 1; echo 0")
	h := &exitHost{exits: make(chan string, 1)}
	b := &build{buildID: "k3v9x2ab", buildContainer: "warden-build-k3v9x2ab"}
	b.watchExit(h)()
	select {
	case got := <-h.exits:
		t.Errorf("exited %q after the watch was stopped", got)
	default:
	}
}

// The held start of mode shell: the build container is created with
// initialize --hold, and the wait for its setup ends once initialize has
// recorded the shell environment (BOOT-03: the probes only look).
func TestHeldStart(t *testing.T) {
	_, log := fakeCLI(t, `"container create") echo warden-build-k3v9x2ab ;;
"container exec") test -e "$(dirname "$0")/setup-done" || exit 1 ;;
"container inspect") echo true ;;`)
	b := &build{buildID: "k3v9x2ab", baseImage: "alpine", holderContainer: "holder",
		subnet: driver.Subnet{RelayIP: "10.89.0.2"}}
	if err := b.createBuildContainer(true); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "initialize --gateway=10.89.0.2 --workdir=/work "+
		"--hold") {
		t.Errorf("the build container was not created held:\n%s", calls)
	}
	done := make(chan error, 1)
	go func() { done <- b.waitHeld(t.Context()) }()
	select {
	case err := <-done:
		t.Fatalf("waitHeld = %v before setup was done", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "setup-done"), nil,
		0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("waitHeld = %v once setup was done", err)
	}
	calls, _ = os.ReadFile(log)
	if !strings.Contains(string(calls), "container exec warden-build-k3v9x2ab test -f "+
		shellEnvFile) {
		t.Errorf("waitHeld did not look for %s:\n%s", shellEnvFile, calls)
	}
}

// heldHost is a driver.Host whose Started is recorded and whose Wait returns
// once stop is closed.
type heldHost struct {
	driver.Host
	started chan struct{}
	stop    chan struct{}
}

func (h *heldHost) Started() error { close(h.started); return nil }

func (h *heldHost) Wait(ctx context.Context) string {
	select {
	case <-h.stop:
	case <-ctx.Done():
	}
	return "shell-exited"
}

// A held run starts the waiter on the build container by name, as mode build
// does, so that clean's process scan keeps a running shell session of
// another user's (DRV-27).
func TestHoldStartsTheWaiter(t *testing.T) {
	_, log := fakeCLI(t, `"container exec") exit 0 ;;
"container wait") sleep 1; echo 0 ;;`)
	b := &build{buildID: "k3v9x2ab", buildContainer: "warden-build-k3v9x2ab",
		req: &driver.BuildRequest{Stderr: io.Discard}}
	h := &heldHost{started: make(chan struct{}), stop: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- b.hold(t.Context(), h) }()
	select {
	case <-h.started:
	case err := <-done:
		t.Fatalf("hold = %v before the environment was started", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the held environment was not reported started")
	}
	want := "container wait warden-build-k3v9x2ab"
	deadline := time.Now().Add(5 * time.Second)
	for {
		calls, _ := os.ReadFile(log)
		if strings.Contains(string(calls), want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no waiter %q was started:\n%s", want, calls)
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(h.stop)
	if err := <-done; err != nil {
		t.Fatalf("hold = %v once warden stopped it", err)
	}
}

// A held container that stops during its setup fails the shell with its
// console.
func TestHeldStartSetupFails(t *testing.T) {
	fakeCLI(t, `"container exec") exit 1 ;;
"container inspect") echo false ;;
"logs "*) echo 'initialize: no such image' ;;`)
	b := &build{buildContainer: "warden-build-k3v9x2ab"}
	err := b.waitHeld(t.Context())
	const want = "setup failed:\ninitialize: no such image"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("waitHeld = %v, want the setup failure with the console", err)
	}
}

// Attach enters the held build container of the build ID and returns the
// shell's exit code as its wrapper recorded it, also when the runtime CLI
// exits 1 whatever the shell did, as finch does with a TTY (UX-09).
func TestAttach(t *testing.T) {
	rt, log := fakeCLI(t, `"container inspect") echo true ;;
"container exec") case "$*" in
	*" cat /.warden/shell-exit") echo 7 ;;
	*) exit 1 ;;
	esac ;;`)
	code, err := (&runner{runtime: rt}).Attach("k3v9x2ab", strings.NewReader(""),
		&strings.Builder{}, &strings.Builder{})
	if err != nil || code != 7 {
		t.Fatalf("Attach = %d, %v; want the shell's 7", code, err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "warden-build-k3v9x2ab sh -c . "+shellEnvFile) {
		t.Errorf("the shell was not run in the build's container:\n%s", calls)
	}
}

// An attach to a build with no held environment fails as an attach, with an
// error and no exit code of a shell, which warden reports and exits 1 for
// (UX-09).
func TestAttachFails(t *testing.T) {
	rt, log := fakeCLI(t, `"container inspect") echo 'no such container' >&2; exit 1 ;;`)
	code, err := (&runner{runtime: rt}).Attach("k3v9x2ab", strings.NewReader(""),
		&strings.Builder{}, &strings.Builder{})
	var fe *driver.FixableError
	if !errors.As(err, &fe) || code != 0 ||
		!strings.Contains(fe.Msg, "no held shell environment of build k3v9x2ab") {
		t.Fatalf("Attach = %d, %v; want the failed attach", code, err)
	}
	if calls, _ := os.ReadFile(log); strings.Contains(string(calls), "exec") {
		t.Errorf("a shell was started for a build with no held environment:\n%s", calls)
	}
}

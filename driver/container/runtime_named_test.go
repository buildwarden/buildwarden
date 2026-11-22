package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// unusableRuntime is a state of a named podman that a command refuses: the
// end of the message, and a part of the fix.
type unusableRuntime struct {
	name, msg, fix string
	installed      bool
}

// The query the stub podman is asked, as its log records it.
const podmanQueried = "version --format {{.Server.Version}}\n"

// A runtime named by --runtime, WARDEN_CTR_CLI or either key that is not on
// PATH is refused by the probe of run and by doctor with a fixable error
// that names the source and the runtime, and nothing is run; detection is
// not tried in its place (Behaviour 20, Decision 26, a28).
func TestNamedRuntimeNotInstalled(t *testing.T) {
	state := unusableRuntime{"not installed",
		"which is not installed (not found on PATH)", "install Podman", false}
	for _, command := range runtimeCommands[:2] {
		forEachLevel(t, state.name+", "+command.name, func(t *testing.T,
			r *runtimeTest, level, table, label string) {
			calls := stubRuntimes(t, false)
			setting := r.name(level, table, label, "podman")
			wantUnusable(t, command.call(r), setting, state)
			wantRan(t, calls, "")
		})
	}
}

// A named runtime on PATH whose engine is down passes the probe of run,
// which makes no engine call; the build's first engine call, its version
// query, fails it with the fixable error, the start hint and a pointer to
// warden doctor; and doctor, which asks the engine, is blocked with an
// error that names the source (r4, CI repair).
func TestNamedRuntimeWhoseEngineIsDown(t *testing.T) {
	forEachLevel(t, "the probe of run", func(t *testing.T, r *runtimeTest, level,
		table, label string) {
		calls := stubRuntimes(t, true)
		r.name(level, table, label, "podman")
		if err := runtimeCommands[0].call(r); err != nil {
			t.Errorf("the probe = %v; want it to pass without asking the engine", err)
		}
		wantRan(t, calls, "")
	})
	forEachLevel(t, "the build", func(t *testing.T, r *runtimeTest, level, table,
		label string) {
		calls := stubRuntimes(t, true)
		r.name(level, table, label, "podman")
		var out bytes.Buffer
		_, err := drivercmd.Call(context.Background(), Driver(), drivercmd.CmdRun,
			&drivercmd.Request{Start: &driver.Start{Contract: 1, BuildID: "k3v9x2ab",
				Mode: driver.ModeBuild, GuestOS: "linux", TempDir: t.TempDir(),
				ConfigFile: r.file, Args: r.args()},
				Streams: drivercmd.Streams{Stderr: &out}},
			&exitHost{exits: make(chan string, 1)})
		var fe *driver.FixableError
		want := "container runtime podman is installed but not running " +
			"(`podman version --format '{{.Server.Version}}'` failed)"
		if !errors.As(err, &fe) || fe.Msg != want ||
			!strings.Contains(fe.Fix, "podman machine start") ||
			!strings.Contains(fe.Fix, "warden doctor") {
			t.Fatalf("the build = %v; want a fixable %q, with the start hint "+
				"and warden doctor", err, want)
		}
		wantRan(t, calls, podmanQueried)
	})
	state := unusableRuntime{"not answering", "which is installed but not running " +
		"(`podman version --format '{{.Server.Version}}'` failed)",
		"podman machine start", true}
	forEachLevel(t, "doctor", func(t *testing.T, r *runtimeTest, level, table,
		label string) {
		calls := stubRuntimes(t, true)
		setting := r.name(level, table, label, "podman")
		wantUnusable(t, runtimeCommands[1].call(r), setting, state)
		wantRan(t, calls, podmanQueried)
	})
}

// wantRan checks the stub runtimes' log.
func wantRan(t *testing.T, calls, want string) {
	t.Helper()
	if ran, _ := os.ReadFile(calls); string(ran) != want {
		t.Errorf("the runtimes ran %q, want %q", ran, want)
	}
}

// stubRuntimes makes PATH a directory of its own holding a docker that
// answers, which detection would choose, and, when installed, a podman whose
// every call fails; both log their calls to the returned file.
func stubRuntimes(t *testing.T, installed bool) (calls string) {
	t.Helper()
	if installed && runtime.GOOS == "windows" {
		t.Skip("the stub runtime is a POSIX shell script")
	}
	bin, calls := t.TempDir(), filepath.Join(t.TempDir(), "calls")
	if installed {
		writeScript(t, filepath.Join(bin, "podman"),
			fmt.Sprintf("#!/bin/sh\necho \"$*\" >> '%s'\nexit 1\n", calls))
	}
	if runtime.GOOS != "windows" {
		writeScript(t, filepath.Join(bin, "docker"),
			fmt.Sprintf("#!/bin/sh\necho docker \"$*\" >> '%s'\n", calls))
	}
	t.Setenv("PATH", bin)
	return calls
}

// wantUnusable checks that err refuses podman, named at setting, in state.
func wantUnusable(t *testing.T, err error, setting string, state unusableRuntime) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError", err)
	}
	if want := setting + " names the container runtime podman, " + state.msg; fe.Msg != want {
		t.Errorf("message %q, want %q", fe.Msg, want)
	}
	if !strings.Contains(fe.Fix, state.fix) || !strings.Contains(fe.Fix, "another runtime") {
		t.Errorf("fix %q: want %q and naming another runtime", fe.Fix, state.fix)
	}
}

// A named runtime that is installed and answers passes the probe, and
// doctor.
func TestNamedRuntimeThatAnswers(t *testing.T) {
	for _, command := range runtimeCommands[:2] {
		forEachLevel(t, command.name, func(t *testing.T, r *runtimeTest, level, table,
			label string) {
			installRuntime(t, "podman", answers)
			r.name(level, table, label, "podman")
			if err := command.call(r); err != nil {
				t.Fatalf("got %v for a podman that answers", err)
			}
		})
	}
}

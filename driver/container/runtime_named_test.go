package container

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// unusableRuntime is a state of a named podman that the probe refuses: the
// end of the message, and a part of the fix.
type unusableRuntime struct {
	name, msg, fix string
	installed      bool
}

// A runtime named by --runtime, WARDEN_CTR_CLI or either key, that is not on
// PATH or whose info fails, is refused by the probe of run and by doctor with
// a fixable error that names the source and the runtime; nothing is run past
// info, and detection is not tried in its place (Behaviour 20, Decision 26,
// a28).
func TestNamedRuntimeThatCannotBeUsed(t *testing.T) {
	probeAndDoctor := runtimeCommands[:2]
	for _, state := range []unusableRuntime{
		{"not installed", "which is not installed (not found on PATH)", "install Podman",
			false},
		{"not answering", "which is installed but not running (`podman info` failed)",
			"podman machine start", true},
	} {
		for _, command := range probeAndDoctor {
			forEachLevel(t, state.name+", "+command.name, func(t *testing.T,
				r *runtimeTest, level, table, label string) {
				calls := stubRuntimes(t, state.installed)
				setting := r.name(level, table, label, "podman")
				wantUnusable(t, command.call(r), setting, state)
				ran, _ := os.ReadFile(calls)
				want := ""
				if state.installed {
					want = "info\n"
				}
				if string(ran) != want {
					t.Errorf("the runtimes ran %q, want %q", ran, want)
				}
			})
		}
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
	if runtime.GOOS == "windows" {
		t.Skip("the stub runtime is a POSIX shell script")
	}
	for _, command := range runtimeCommands[:2] {
		forEachLevel(t, command.name, func(t *testing.T, r *runtimeTest, level, table,
			label string) {
			installRuntime(t, "podman", 0)
			r.name(level, table, label, "podman")
			if err := command.call(r); err != nil {
				t.Fatalf("got %v for a podman that answers", err)
			}
		})
	}
}

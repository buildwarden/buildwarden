package container

import (
	"os"
	"path/filepath"
	"runtime"
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

// fakeRuntime installs a container runtime CLI that answers "container wait"
// with script's output and "logs" with a console line.
func fakeRuntime(t *testing.T, wait string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake runtime is a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "rt")
	body := "#!/bin/sh\ncase \"$1 $2\" in\n" +
		"\"container wait\") " + wait + " ;;\n" +
		"\"logs \"*) echo 'initialize: relay unreachable' ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := ctrctl.Cli
	ctrctl.Cli = []string{path}
	t.Cleanup(func() { ctrctl.Cli = old })
}

// The build container's exit is reported as exited, with its exit code and
// the last lines of its console (ORCH-01, BOOT-01).
func TestWatchExitReportsTheBuildContainer(t *testing.T) {
	fakeRuntime(t, "echo 3")
	h := &exitHost{exits: make(chan string, 1)}
	b := &build{buildContainer: "warden-build-k3v9x2ab"}
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
	b := &build{buildContainer: "warden-build-k3v9x2ab"}
	b.watchExit(h)()
	select {
	case got := <-h.exits:
		t.Errorf("exited %q after the watch was stopped", got)
	default:
	}
}

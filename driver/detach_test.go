package driver

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The background image prepare is given the config file its build read, and
// nothing when the build read none.
func TestDetachedArgs(t *testing.T) {
	orig := ConfigFile
	t.Cleanup(func() { ConfigFile = orig })
	args := []string{"image", "prepare", "--driver", "qemu"}

	ConfigFile = "/src/ci/warden-ci.toml"
	want := append([]string{"--config", "/src/ci/warden-ci.toml"}, args...)
	if got := detachedArgs(args); !slices.Equal(got, want) {
		t.Errorf("with a file: %q, want %q", got, want)
	}
	ConfigFile = ""
	if got := detachedArgs(args); !slices.Equal(got, args) {
		t.Errorf("with none: %q, want %q", got, args)
	}
}

// The detached start runs the extension-less file it is given, not a
// warden.exe beside it. On Windows, os/exec would have run warden.exe; on
// other systems the .exe copy is inert and this guards the plain start.
func TestStartDetachedRunsNamedFile(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	named := filepath.Join(dir, "warden")
	copyFile(t, self, named)
	copyFile(t, self, filepath.Join(dir, "warden.exe"))
	t.Setenv(detachedHelperEnv, "1")

	logPath := filepath.Join(dir, "prepare.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	if err := startDetached(named, log, nil); err != nil {
		t.Fatalf("startDetached: %v", err)
	}
	got := waitForLine(t, logPath)
	if g, w := realPath(t, got), realPath(t, named); g != w {
		t.Errorf("the started process is %s, want %s", g, w)
	}
}

// A start that fails returns its error and leaves the reason in the log.
func TestStartDetachedFailedStart(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-warden")
	orig := executable
	t.Cleanup(func() { executable = orig })
	executable = func() (string, error) { return missing, nil }

	logPath := filepath.Join(dir, "prepare.log")
	if err := StartDetached(logPath, "image", "prepare"); err == nil {
		t.Fatal("StartDetached of a missing file returned nil")
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "could not start " + missing; !strings.Contains(string(b), want) {
		t.Errorf("log = %q, want it to contain %q", b, want)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// waitForLine polls path until it holds a complete first line and returns it;
// it fails the test at the deadline.
func waitForLine(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if line, _, ok := strings.Cut(string(b), "\n"); ok {
			return strings.TrimSpace(line)
		}
		if time.Now().After(deadline) {
			t.Fatalf("no line in %s after 30s (have %q)", path, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// realPath resolves links, so a temp directory behind one compares equal.
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

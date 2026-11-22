package specs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pinDirs are the six specifications tools/conformance/ pins.
var pinDirs = []string{"bootstrap-config", "collector-inputs", "collector-outputs",
	"executor-initialize", "relay-claim", "relay-guest-api"}

// pinTree is a checkout holding the six go.mod files, each pinning its
// specification and the harness at version, the seams page, and a make that
// does nothing, so check-pins.sh reads only the pins.
func pinTree(t *testing.T, version string) (dir, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the pin scripts are POSIX sh")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("check-pins.sh needs jq")
	}
	dir = t.TempDir()
	const repo = "github.com/buildwarden/buildwarden-specs"
	for _, d := range pinDirs {
		mod := "module github.com/buildwarden/buildwarden/tools/conformance/" + d +
			"\n\ngo 1.26\n\nrequire (\n\t" + repo + "/conformance " + version +
			"\n\t" + repo + "/" + d + " " + version + "\n)\n"
		write(t, filepath.Join(dir, "tools", "conformance", d, "go.mod"), mod)
		write(t, filepath.Join(dir, "tools", "conformance", d, "go.sum"), "")
	}
	write(t, filepath.Join(dir, "docs", "book", "src", "concepts", "seams.md"), "# Seams\n")
	bin := filepath.Join(dir, "bin")
	write(t, filepath.Join(bin, "make"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(bin, "make"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

func write(t *testing.T, p, text string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// script runs a tools/conformance script in dir with the environment.
func script(t *testing.T, dir, name string, env ...string) (string, error) {
	t.Helper()
	self, err := filepath.Abs(filepath.Join("..", "tools", "conformance", name))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", self)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestCheckPinsPreRelease: a pull request to a program trunk may pin a
// pre-release of the specifications and the harness; one to main may not
// (FLOW-20, Decision 15).
func TestCheckPinsPreRelease(t *testing.T) {
	dir, path := pinTree(t, "v1.2.0-dev.3")
	if out, err := script(t, dir, "check-pins.sh", "PATH="+path,
		"GITHUB_BASE_REF=program/componentization"); err != nil {
		t.Fatalf("to the program trunk: %v\n%s", err, out)
	}
	out, err := script(t, dir, "check-pins.sh", "PATH="+path, "GITHUB_BASE_REF=main")
	if err == nil || !strings.Contains(out, "not a release tag") {
		t.Fatalf("to main: %v\n%s", err, out)
	}
	dir, path = pinTree(t, "v1.2.0")
	if out, err := script(t, dir, "check-pins.sh", "PATH="+path,
		"GITHUB_BASE_REF=main"); err != nil {
		t.Fatalf("a release to main: %v\n%s", err, out)
	}
}

// TestPinSpecsNewestNotOnMain: SPECS=newest takes pre-releases, so it is
// refused on main (FLOW-20).
func TestPinSpecsNewestNotOnMain(t *testing.T) {
	dir, path := pinTree(t, "v1.2.0")
	for _, args := range [][]string{{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty",
			"-m", "x"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).
			CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	out, err := script(t, dir, "pin-specs.sh", "PATH="+path, "SPECS=newest")
	if err == nil || !strings.Contains(out, "main never pins") {
		t.Fatalf("SPECS=newest on main: %v\n%s", err, out)
	}
}

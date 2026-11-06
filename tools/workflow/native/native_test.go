// Package native holds only tests: the files it checks are the scheduled
// native session's permissions and its native-run.sh launcher.
package native

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeSettingsRules guards settings.native.json against the rule shapes
// docs/design/native-allowlist-scratch.md and native-allowlist-scratch-tools.md
// fixed or rejected.
func TestNativeSettingsRules(t *testing.T) {
	raw, err := os.ReadFile("settings.native.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("settings.native.json: %v", err)
	}
	has := func(list []string, rule string) bool {
		for _, r := range list {
			if r == rule {
				return true
			}
		}
		return false
	}
	// The launcher's only route.
	if !has(s.Permissions.Allow, "Bash(tools/workflow/*)") {
		t.Error("allow lacks Bash(tools/workflow/*), which native-run.sh needs")
	}
	for _, r := range s.Permissions.Allow {
		// A "prefix/:*" rule never matches a path below prefix.
		if strings.HasSuffix(r, "/:*)") {
			t.Errorf("allow rule %q ends in /:*", r)
		}
		// "*" spans any text, so this would admit any .scratch/ command.
		if strings.HasPrefix(r, "Bash(.scratch/*/tools") {
			t.Errorf("allow rule %q admits a branch's scripts unchecked", r)
		}
	}
	for _, rule := range []string{
		"Bash(git -C * push origin main*)",
		"Bash(git -C * push origin HEAD:main*)",
		"Bash(git -C * push --force*)",
		"Bash(git -C * push -f*)",
		"Bash(git -C * push * --force*)",
		"Bash(git -C * push origin --delete*)",
		"Bash(git -C * push --mirror*)",
	} {
		if !has(s.Permissions.Deny, rule) {
			t.Errorf("deny lacks %s", rule)
		}
	}
}

// TestNativeRun runs native-run.sh in the repository newLauncherRepo builds.
func TestNativeRun(t *testing.T) {
	for _, tool := range []string{"sh", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	launcher, err := filepath.Abs("native-run.sh")
	if err != nil {
		t.Fatal(err)
	}
	repo := newLauncherRepo(t)

	const (
		x = ".scratch/t/tools/x.sh"
		w = ".scratch/t/dist/warden"
	)
	args := func(a ...string) []string { return a }
	tests := []struct {
		name string
		args []string
		want string // stdout; empty means refused with exit 2
		code int
	}{
		{"script", args("--", x), "x WARDEN_A=\n", 0},
		{"script with variable", args("WARDEN_A=a b", "--", x), "x WARDEN_A=a b\n", 0},
		{"warden with variable", args("WARDEN_A=1", "--", w), "warden WARDEN_A=1\n", 0},
		{"exit status", args("WARDEN_EXIT=7", "--", w), "warden WARDEN_A=\n", 7},
		{"non-WARDEN name", args("PATH=/x", "--", w), "", 2},
		{"lower-case name", args("WARDEN_a=1", "--", w), "", 2},
		{"secret name", args("WARDEN_X_PASSWORD=1", "--", w), "", 2},
		{"token name", args("WARDEN_X_TOKEN=1", "--", x), "", 2},
		{"word before --", args("WARDEN_A=1", w), "", 2},
		{"assignment only", args("WARDEN_A=1"), "", 2},
		{"no command", args("WARDEN_A=1", "--"), "", 2},
		{"other command", args("WARDEN_A=1", "--", "sh", "-c", "echo bad"), "", 2},
		{"go run", args("WARDEN_A=1", "--", "go", "run", "."), "", 2},
		{"go -C not .scratch", args("WARDEN_A=1", "--", "go", "-C", "..", "test"), "", 2},
		{"dotdot component", args("--", ".scratch/t/tools/../tools/x.sh"), "", 2},
		{"dot component", args("--", ".scratch/t/tools/./x.sh"), "", 2},
		{"empty component", args("--", ".scratch/t/tools//x.sh"), "", 2},
		{"dotdot name", args("--", ".scratch/../tools/x.sh"), "", 2},
		{"dotdot in warden path", args("WARDEN_A=1", "--", w+"/../../tools/x.sh"), "", 2},
		{"dotdot after warden", args("WARDEN_A=1", "--", w+"/../x"), "", 2},
		{"untracked script", args("--", ".scratch/t/tools/y.sh"), "", 2},
		{"branch tools/workflow", args("--", ".scratch/t/tools/workflow/z.sh"), "", 2},
		{"workflow, empty", args("--", ".scratch/t/tools//workflow/z.sh"), "", 2},
		{"plain directory", args("--", ".scratch/p/tools/x.sh"), "", 2},
		{"warden without variable", args("--", w), "", 2},
		{"script name with space", args("--", ".scratch/t/x.sh foo/tools/bar"), "", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runLauncher(t, launcher, repo, tc.args)
			if code != tc.code || stdout != tc.want {
				t.Fatalf("exit %d, stdout %q; want exit %d, stdout %q (stderr %q)",
					code, stdout, tc.code, tc.want, stderr)
			}
			if tc.want == "" {
				t.Log(stderr)
				oneLine := strings.Count(stderr, "\n") == 1
				if !strings.HasPrefix(stderr, "native-run.sh: ") || !oneLine {
					t.Errorf("not one native-run.sh line: %q", stderr)
				}
			}
		})
	}

	// The relative .scratch/ paths it checks are the session's only from the
	// checkout root.
	stdout, _, code := runLauncher(t, launcher, filepath.Join(repo, "tools"), args("--", x))
	if code != 2 || stdout != "" {
		t.Errorf("ran from a subdirectory: exit %d, stdout %q", code, stdout)
	}
}

// newLauncherRepo returns a temporary repository whose worktree .scratch/t
// holds a tracked tools/x.sh, a tracked tools/workflow/z.sh, an untracked
// tools/y.sh and an untracked stub dist/warden; .scratch/p is a plain
// directory with a tools/x.sh. Every stub prints its name and WARDEN_A, and
// exits with WARDEN_EXIT.
func newLauncherRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cfg := []string{"-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false"}
		cmd := exec.Command("git", append(cfg, args...)...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	stub := func(name string, path ...string) {
		t.Helper()
		file := filepath.Join(append([]string{repo}, path...)...)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "#!/bin/sh\necho \"" + name + " WARDEN_A=$WARDEN_A\"\n" +
			"exit ${WARDEN_EXIT:-0}\n"
		if err := os.WriteFile(file, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	stub("x", "tools", "x.sh")
	stub("z", "tools", "workflow", "z.sh")
	git("add", "tools")
	git("commit", "-q", "-m", "stubs")
	git("worktree", "add", "-q", "--detach", filepath.Join(".scratch", "t"))
	stub("y", ".scratch", "t", "tools", "y.sh")
	stub("warden", ".scratch", "t", "dist", "warden")
	stub("p", ".scratch", "p", "tools", "x.sh")
	return repo
}

// runLauncher runs the launcher with sh in dir and returns its stdout, stderr
// and exit status.
func runLauncher(t *testing.T, launcher, dir string, args []string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{launcher}, args...)...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return stdout.String(), stderr.String(), ee.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), 0
}

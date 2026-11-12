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
	"runtime"
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

// launcherCase is one run of native-run.sh: its arguments, and the stdout and
// exit status expected. An empty want means refused with exit 2.
type launcherCase struct {
	name string
	args []string
	want string
	code int
}

// TestNativeRun runs native-run.sh in the repository newLauncherRepo builds.
func TestNativeRun(t *testing.T) {
	launcher, repo := launcherSetup(t)
	const (
		x = ".scratch/t/tools/x.sh"
		w = ".scratch/t/dist/warden"
		a = "WARDEN_A=1"
	)
	args := func(a ...string) []string { return a }
	tests := []launcherCase{
		{"script", args("--", x), "x WARDEN_A=\n", 0},
		{"script with variable", args("WARDEN_A=a b", "--", x), "x WARDEN_A=a b\n",
			0},
		{"warden with variable", args(a, "--", w), "warden WARDEN_A=1\n", 0},
		{"exit status", args("WARDEN_EXIT=7", "--", w), "warden WARDEN_A=\n", 7},
		{"root dist/warden", args(a, "--", "dist/warden-x"), "root-dist WARDEN_A=1\n", 0},
		{"root ./warden", args(a, "--", "./warden"), "root WARDEN_A=1\n", 0},
		{"non-WARDEN name", args("PATH=/x", "--", w), "", 2},
		{"lower-case name", args("WARDEN_a=1", "--", w), "", 2},
		{"secret name", args("WARDEN_X_PASSWORD=1", "--", w), "", 2},
		{"token name", args("WARDEN_X_TOKEN=1", "--", x), "", 2},
		{"word before --", args(a, w), "", 2},
		{"assignment only", args(a), "", 2},
		{"no command", args(a, "--"), "", 2},
		{"other command", args(a, "--", "sh", "-c", "echo bad"), "", 2},
		{"go run", args(a, "--", "go", "run", "."), "", 2},
		{"go -C not .scratch", args(a, "--", "go", "-C", "..", "test"), "", 2},
		{"go -C nested", args(a, "--", "go", "-C", ".scratch/a/b", "test"), "", 2},
		{"dotdot component", args("--", ".scratch/t/tools/../tools/x.sh"), "", 2},
		{"dot component", args("--", ".scratch/t/tools/./x.sh"), "", 2},
		{"empty component", args("--", ".scratch/t/tools//x.sh"), "", 2},
		{"dotdot name", args("--", ".scratch/../tools/x.sh"), "", 2},
		{"dotdot in warden path", args(a, "--", w+"/../../tools/x.sh"), "", 2},
		{"dotdot after warden", args(a, "--", w+"/../x"), "", 2},
		{"untracked script", args("--", ".scratch/t/tools/y.sh"), "", 2},
		{"branch tools/workflow", args("--", ".scratch/t/tools/workflow/z.sh"), "", 2},
		{"workflow, empty", args("--", ".scratch/t/tools//workflow/z.sh"), "", 2},
		// Each of the next two passes every check but one: tools/x.sh is
		// tracked in each, by the main repository for .scratch/p.
		{"plain directory", args("--", ".scratch/p/tools/x.sh"), "", 2},
		{"other repository", args("--", ".scratch/o/tools/x.sh"), "", 2},
		{"warden without variable", args("--", w), "", 2},
		{"script name with space", args("--", ".scratch/t/x.sh foo/tools/bar"), "", 2},
	}
	// An untracked file named "x*" matches the tracked x.sh as a glob.
	// Windows file names cannot hold "*".
	if runtime.GOOS != "windows" {
		glob := launcherCase{"glob name", args("--", ".scratch/t/tools/x*"), "", 2}
		tests = append(tests, glob)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runLauncher(t, launcher, repo, tc.args)
			if code != tc.code || stdout != tc.want {
				t.Fatalf("exit %d, stdout %q; want exit %d, stdout %q (stderr %q)",
					code, stdout, tc.code, tc.want, stderr)
			}
			if tc.want == "" {
				checkRefusal(t, stderr)
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

// TestNativeRunGo runs the go forms on the package gopkg, whose test fails
// unless WARDEN_A reaches it. go test's output carries a timing, so only its
// start is compared.
func TestNativeRunGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found")
	}
	launcher, repo := launcherSetup(t)
	args := func(a ...string) []string { return a }
	const a, p = "WARDEN_A=1", "./gopkg"
	tests := []launcherCase{
		{"go test", args(a, "--", "go", "test", p), "ok", 0},
		{"go -C test", args(a, "--", "go", "-C", ".scratch/t", "test", p), "ok", 0},
		{"variable empty", args("WARDEN_A=", "--", "go", "test", p), "--- FAIL", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runLauncher(t, launcher, repo, tc.args)
			if code != tc.code || !strings.HasPrefix(stdout, tc.want) {
				t.Fatalf("exit %d, stdout %q; want exit %d, stdout from %q (%q)",
					code, stdout, tc.code, tc.want, stderr)
			}
		})
	}
}

// launcherSetup skips without sh or git, and returns the launcher's absolute
// path and a repository from newLauncherRepo.
func launcherSetup(t *testing.T) (string, string) {
	t.Helper()
	for _, tool := range []string{"sh", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	launcher, err := filepath.Abs("native-run.sh")
	if err != nil {
		t.Fatal(err)
	}
	return launcher, newLauncherRepo(t)
}

// checkRefusal fails unless stderr is one native-run.sh line.
func checkRefusal(t *testing.T, stderr string) {
	t.Helper()
	t.Log(stderr)
	oneLine := strings.Count(stderr, "\n") == 1
	if !strings.HasPrefix(stderr, "native-run.sh: ") || !oneLine {
		t.Errorf("not one native-run.sh line: %q", stderr)
	}
}

// newLauncherRepo returns a temporary repository whose worktree .scratch/t
// holds a tracked tools/x.sh, a tracked tools/workflow/z.sh, a tracked Go
// package gopkg whose test needs WARDEN_A, an untracked tools/y.sh, an
// untracked tools/x* (not on Windows) and an untracked stub dist/warden. .scratch/p is a plain
// directory whose tools/x.sh the main repository tracks; .scratch/o is a
// separate repository that tracks its tools/x.sh. The root holds untracked
// stubs dist/warden-x and warden. Every stub prints its name and
// WARDEN_A, and exits with WARDEN_EXIT.
func newLauncherRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cfg := []string{"-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}
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
	write := func(body string, path ...string) {
		t.Helper()
		file := filepath.Join(append([]string{repo}, path...)...)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	stub("x", "tools", "x.sh")
	stub("z", "tools", "workflow", "z.sh")
	stub("p", ".scratch", "p", "tools", "x.sh")
	write("module example.test/launcher\n\ngo 1.21\n", "go.mod")
	write("package gopkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n"+
		"func TestVar(t *testing.T) {\n\tif os.Getenv(\"WARDEN_A\") == \"\" {\n"+
		"\t\tt.Fatal(\"WARDEN_A unset\")\n\t}\n}\n", "gopkg", "v_test.go")
	git("add", "tools", "go.mod", "gopkg", filepath.Join(".scratch", "p"))
	git("commit", "-q", "-m", "stubs")
	git("worktree", "add", "-q", "--detach", filepath.Join(".scratch", "t"))
	stub("y", ".scratch", "t", "tools", "y.sh")
	if runtime.GOOS != "windows" {
		stub("glob", ".scratch", "t", "tools", "x*")
	}
	stub("warden", ".scratch", "t", "dist", "warden")
	stub("root-dist", "dist", "warden-x")
	stub("root", "warden")
	stub("o", ".scratch", "o", "tools", "x.sh")
	git("-C", filepath.Join(".scratch", "o"), "init", "-q")
	git("-C", filepath.Join(".scratch", "o"), "add", "tools")
	git("-C", filepath.Join(".scratch", "o"), "commit", "-q", "-m", "stub")
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

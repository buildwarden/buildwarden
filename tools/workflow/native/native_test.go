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
	_, goErr := exec.LookPath("go")
	tests := []struct {
		name   string
		args   []string
		want   string // stdout; empty means refused with exit 2
		code   int
		prefix bool // want is a prefix of stdout, which carries a timing
		needGo bool
		noWin  bool // Windows file names cannot hold "*"
	}{
		{name: "script", args: args("--", x), want: "x WARDEN_A=\n"},
		{name: "script with variable", args: args("WARDEN_A=a b", "--", x), want: "x WARDEN_A=a b\n"},
		{name: "warden with variable", args: args("WARDEN_A=1", "--", w), want: "warden WARDEN_A=1\n"},
		{name: "exit status", args: args("WARDEN_EXIT=7", "--", w), want: "warden WARDEN_A=\n", code: 7},
		{name: "root dist/warden", args: args("WARDEN_A=1", "--", "dist/warden-linux"), want: "root-dist WARDEN_A=1\n"},
		{name: "root ./warden", args: args("WARDEN_A=1", "--", "./warden"), want: "root WARDEN_A=1\n"},
		// The package's test fails unless WARDEN_A reaches it.
		{name: "go test", args: args("WARDEN_A=1", "--", "go", "test", "./gopkg"), want: "ok", prefix: true, needGo: true},
		{name: "go -C test", args: args("WARDEN_A=1", "--", "go", "-C", ".scratch/t", "test", "./gopkg"), want: "ok", prefix: true, needGo: true},
		{name: "go test without variable", args: args("WARDEN_A=", "--", "go", "test", "./gopkg"), want: "--- FAIL", prefix: true, code: 1, needGo: true},
		{name: "non-WARDEN name", args: args("PATH=/x", "--", w), code: 2},
		{name: "lower-case name", args: args("WARDEN_a=1", "--", w), code: 2},
		{name: "secret name", args: args("WARDEN_X_PASSWORD=1", "--", w), code: 2},
		{name: "token name", args: args("WARDEN_X_TOKEN=1", "--", x), code: 2},
		{name: "word before --", args: args("WARDEN_A=1", w), code: 2},
		{name: "assignment only", args: args("WARDEN_A=1"), code: 2},
		{name: "no command", args: args("WARDEN_A=1", "--"), code: 2},
		{name: "other command", args: args("WARDEN_A=1", "--", "sh", "-c", "echo bad"), code: 2},
		{name: "go run", args: args("WARDEN_A=1", "--", "go", "run", "."), code: 2},
		{name: "go -C not .scratch", args: args("WARDEN_A=1", "--", "go", "-C", "..", "test"), code: 2},
		{name: "dotdot component", args: args("--", ".scratch/t/tools/../tools/x.sh"), code: 2},
		{name: "dot component", args: args("--", ".scratch/t/tools/./x.sh"), code: 2},
		{name: "empty component", args: args("--", ".scratch/t/tools//x.sh"), code: 2},
		{name: "dotdot name", args: args("--", ".scratch/../tools/x.sh"), code: 2},
		{name: "dotdot in warden path", args: args("WARDEN_A=1", "--", w+"/../../tools/x.sh"), code: 2},
		{name: "dotdot after warden", args: args("WARDEN_A=1", "--", w+"/../x"), code: 2},
		{name: "untracked script", args: args("--", ".scratch/t/tools/y.sh"), code: 2},
		{name: "branch tools/workflow", args: args("--", ".scratch/t/tools/workflow/z.sh"), code: 2},
		{name: "workflow, empty", args: args("--", ".scratch/t/tools//workflow/z.sh"), code: 2},
		// Each of the next three passes every check but one: tools/x.sh is
		// tracked in each, by the main repository for .scratch/p.
		{name: "plain directory", args: args("--", ".scratch/p/tools/x.sh"), code: 2},
		{name: "other repository", args: args("--", ".scratch/o/tools/x.sh"), code: 2},
		// An untracked file named "x*" matches the tracked x.sh as a glob.
		{name: "glob name", args: args("--", ".scratch/t/tools/x*"), code: 2, noWin: true},
		{name: "go -C nested", args: args("WARDEN_A=1", "--", "go", "-C", ".scratch/a/b", "test"), code: 2},
		{name: "warden without variable", args: args("--", w), code: 2},
		{name: "script name with space", args: args("--", ".scratch/t/x.sh foo/tools/bar"), code: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needGo && goErr != nil {
				t.Skip("go not found")
			}
			if tc.noWin && runtime.GOOS == "windows" {
				t.Skip("no such file name on Windows")
			}
			stdout, stderr, code := runLauncher(t, launcher, repo, tc.args)
			got := stdout
			if tc.prefix && strings.HasPrefix(got, tc.want) {
				got = tc.want
			}
			if code != tc.code || got != tc.want {
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
// holds a tracked tools/x.sh, a tracked tools/workflow/z.sh, a tracked Go
// package gopkg whose test needs WARDEN_A, an untracked tools/y.sh, an
// untracked tools/x* (not on Windows) and an untracked stub dist/warden. .scratch/p is a plain
// directory whose tools/x.sh the main repository tracks; .scratch/o is a
// separate repository that tracks its tools/x.sh. The root holds untracked
// stubs dist/warden-linux and warden. Every stub prints its name and
// WARDEN_A, and exits with WARDEN_EXIT.
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
	stub("root-dist", "dist", "warden-linux")
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

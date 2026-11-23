// Package native holds only tests: the files it checks are the scheduled
// native session's permissions, its native-run.sh launcher and its
// native-push.sh push script.
package native

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	for _, r := range s.Permissions.Allow {
		// Every push goes through wf commit or native-push.sh (FLOW-17).
		if strings.HasPrefix(r, "Bash(git push") || strings.Contains(r, " push ") ||
			strings.HasSuffix(r, " push)") {
			t.Errorf("allow rule %q allows a push", r)
		}
	}
	for _, rule := range []string{
		"Bash(git push*)",
		"Bash(git -C * push*)",
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

// matches is the reading of the Claude Code matcher these tests hold the
// settings to: a trailing ":*" matches the prefix alone or followed by a space
// and anything, and every other "*" matches any text, spaces, slashes and
// colons included (docs/design/native-allowlist-scratch.md). Nothing recorded
// of the matcher says a "*" stops at a colon.
func matches(rule, command string) bool {
	p := strings.TrimSuffix(strings.TrimPrefix(rule, "Bash("), ")")
	if prefix, ok := strings.CutSuffix(p, ":*"); ok {
		return command == prefix || strings.HasPrefix(command, prefix+" ")
	}
	parts := strings.Split(p, "*")
	re := "^"
	for i, part := range parts {
		if i > 0 {
			re += ".*"
		}
		re += regexp.QuoteMeta(part)
	}
	return regexp.MustCompile(re + "$").MatchString(command)
}

// TestNativePushRules records, under matches, why the push allows went: the
// old allow admitted a push to another branch and a remote delete, which the
// new denies refuse; and the cost of the second deny, a commit message that
// contains " push", which the -F form avoids (FLOW-17).
func TestNativePushRules(t *testing.T) {
	const oldAllow = "Bash(git push origin feat/*)"
	for _, c := range []struct {
		rule, command string
		want          bool
	}{
		{oldAllow, "git push origin feat/x:workflow", true},
		{oldAllow, "git push origin feat/x :feat/other", true},
		{oldAllow, "git push origin feat/x --force", true},
		{"Bash(git push*)", "git push origin feat/x:workflow", true},
		{"Bash(git push*)", "git push origin feat/x :feat/other", true},
		{"Bash(git -C * push*)", "git -C .scratch/x push origin feat/x:workflow", true},
		{"Bash(git -C * push*)", "git -C .scratch/x push origin feat/x :feat/other", true},
		{"Bash(git -C * push*)", "git -C .dev/workflow push origin workflow", true},
		// The false positive, and the form that avoids it.
		{"Bash(git -C * push*)", `git -C .scratch/x commit -m "a push fix"`, true},
		{"Bash(git -C * push*)", "git -C .scratch/x commit -F ../x.msg", false},
		{"Bash(git push*)", "tools/workflow/native/native-push.sh x", false},
		{"Bash(git -C * push*)", "tools/workflow/native/native-push.sh x", false},
		{"Bash(git status:*)", "git status --short", true},
		{"Bash(git status:*)", "git statusx", false},
	} {
		if got := matches(c.rule, c.command); got != c.want {
			t.Errorf("%s on %q: %v, want %v", c.rule, c.command, got, c.want)
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
		x  = ".scratch/t/tools/x.sh"
		w  = ".scratch/t/dist/warden"
		a  = "WARDEN_A=1"
		s2 = ".scratch/w/r/i"
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
		// A second repository's worktree, .scratch/<owner>/<repo>/<item>.
		{"second script", args("--", s2+"/tools/x.sh"), "w-x WARDEN_A=\n", 0},
		{"second warden", args(a, "--", s2+"/dist/warden"), "w-warden WARDEN_A=1\n", 0},
		{"second, no clone", args("--", ".scratch/n/r/i/tools/x.sh"), "", 2},
		{"second warden, no clone", args(a, "--", ".scratch/n/r/i/dist/warden"), "", 2},
		{"second, other repository", args("--", ".scratch/w/r/j/tools/x.sh"), "", 2},
		{"second warden, other repository", args(a, "--", ".scratch/w/r/j/dist/warden"),
			"", 2},
		{"second untracked", args("--", s2+"/tools/y.sh"), "", 2},
		{"second tools/workflow", args("--", s2+"/tools/workflow/z.sh"), "", 2},
		{"second dotdot path", args("--", s2+"/tools/../tools/x.sh"), "", 2},
		{"second dotdot name", args("--", ".scratch/w/../r/tools/x.sh"), "", 2},
		{"two components", args("--", ".scratch/w/r/tools/x.sh"), "", 2},
		{"four components", args("--", s2+"/k/tools/x.sh"), "", 2},
		{"go -C second, other repository", args(a, "--", "go", "-C", ".scratch/w/r/j",
			"test"), "", 2},
		{"go -C second, no clone", args(a, "--", "go", "-C", ".scratch/n/r/i", "test"),
			"", 2},
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
		{"go -C second", args(a, "--", "go", "-C", ".scratch/w/r/i", "test", p), "ok", 0},
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
// stubs dist/warden-x and warden. secondRepos adds the second
// repositories' worktrees. Every stub prints its name and WARDEN_A, and
// exits with WARDEN_EXIT.
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
	secondRepos(t, repo, git, stub, write)
	return repo
}

// secondRepos adds to newLauncherRepo's repository a clone of a second
// repository at .dev/checkouts/w/r, tracking tools/x.sh, tools/workflow/z.sh
// and the package gopkg, with its worktree .scratch/w/r/i holding an
// untracked tools/y.sh and dist/warden; .scratch/w/r/j, a worktree of the
// main repository, which tracks tools/x.sh too, with an untracked
// dist/warden; and .scratch/n/r/i, a worktree of a repository with no clone
// under .dev/checkouts, tracking tools/x.sh, with an untracked dist/warden.
func secondRepos(t *testing.T, repo string, git func(...string),
	stub func(string, ...string), write func(string, ...string)) {
	t.Helper()
	clone := filepath.Join(".dev", "checkouts", "w", "r")
	stub("w-x", ".dev", "checkouts", "w", "r", "tools", "x.sh")
	stub("w-z", ".dev", "checkouts", "w", "r", "tools", "workflow", "z.sh")
	write("module example.test/second\n\ngo 1.21\n", ".dev", "checkouts", "w", "r", "go.mod")
	write("package gopkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n"+
		"func TestVar(t *testing.T) {\n\tif os.Getenv(\"WARDEN_A\") == \"\" {\n"+
		"\t\tt.Fatal(\"WARDEN_A unset\")\n\t}\n}\n",
		".dev", "checkouts", "w", "r", "gopkg", "v_test.go")
	git("-C", clone, "init", "-q")
	git("-C", clone, "add", "tools", "go.mod", "gopkg")
	git("-C", clone, "commit", "-q", "-m", "second")
	git("-C", clone, "worktree", "add", "-q", "--detach",
		filepath.Join("..", "..", "..", "..", ".scratch", "w", "r", "i"))
	stub("w-y", ".scratch", "w", "r", "i", "tools", "y.sh")
	stub("w-warden", ".scratch", "w", "r", "i", "dist", "warden")
	git("worktree", "add", "-q", "--detach", filepath.Join(".scratch", "w", "r", "j"))
	stub("j-warden", ".scratch", "w", "r", "j", "dist", "warden")

	// A repository outside the checkout, whose worktree sits where a clone's
	// would, but with no clone at .dev/checkouts/n/r.
	far := t.TempDir()
	cmd := exec.Command("git", "init", "-q", far)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(far, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\necho n-x\n"
	if err := os.WriteFile(filepath.Join(far, "tools", "x.sh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	git("-C", far, "add", "tools")
	git("-C", far, "commit", "-q", "-m", "far")
	git("-C", far, "worktree", "add", "-q", "--detach",
		filepath.Join(repo, ".scratch", "n", "r", "i"))
	stub("n-warden", ".scratch", "n", "r", "i", "dist", "warden")
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

// TestNativeClonesRules: a session makes a worktree of a second
// repository's clone with the two narrow allows, and runs no other git
// command in the clone; every push from a clone stays denied, so
// native-push.sh is the only route (docs/design/loop-repository.md).
func TestNativeClonesRules(t *testing.T) {
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
		t.Fatal(err)
	}
	any := func(rules []string, command string) bool {
		for _, r := range rules {
			if matches(r, command) {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		command       string
		allow, denied bool
	}{
		{"git -C .dev/checkouts/o/r fetch origin", true, false},
		{"git -C .dev/checkouts/o/r worktree add ../../../../.scratch/o/r/x feat/x", true,
			false},
		{"git -C .dev/checkouts/o/r commit -m x", false, false},
		{"git -C .dev/checkouts/o/r checkout main", false, false},
		{"git -C .dev/checkouts/o/r reset --hard", false, false},
		{"git -C .dev/checkouts/o/r config user.name x", false, false},
		{"git -C .dev/checkouts/o/r push origin feat/x", false, true},
		{"git -C .dev/checkouts/o/r push --force origin feat/x", false, true},
		{"git -C .dev/checkouts/o/r push origin main", false, true},
	} {
		if got := any(s.Permissions.Allow, c.command); got != c.allow {
			t.Errorf("%q allowed %v, want %v", c.command, got, c.allow)
		}
		if got := any(s.Permissions.Deny, c.command); got != c.denied {
			t.Errorf("%q denied %v, want %v", c.command, got, c.denied)
		}
	}
}

// TestNoPipeIntoGrepQ: no native script pipes a command into grep -q. Under
// pipefail grep -q exits at its first match, the writer takes SIGPIPE on its
// next write, and the pipeline exits 141, which read as "no match"
// (docs/design/native-loop-fixes.md, fix 10). The scripts capture the output
// first and test it with case or a match.
func TestNoPipeIntoGrepQ(t *testing.T) {
	scripts, err := filepath.Glob("*.sh")
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no native scripts: %v", err)
	}
	shape := regexp.MustCompile(`\| *grep +-[a-zA-Z]*q`)
	for _, f := range scripts {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if shape.MatchString(line) {
				t.Errorf("%s:%d pipes into grep -q: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
	// The shape is what the test looks for.
	for _, c := range []struct {
		line string
		want bool
	}{
		{`pmset -g batt | grep -q "AC Power"`, true},
		{`printf '%s' "$auth" | grep -Eq x`, true},
		{`pmset -g sched |grep -qi wake`, true},
		{`grep -qx 'status: done' "$q"`, false},
		{`ls | grep x`, false},
	} {
		if got := shape.MatchString(c.line); got != c.want {
			t.Errorf("%q: matched %v, want %v", c.line, got, c.want)
		}
	}
}

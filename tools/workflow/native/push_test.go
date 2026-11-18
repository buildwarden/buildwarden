package native

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pushRepo is a checkout of a bare origin whose worktrees under .scratch/
// are: w on feat/item, whose item the queue state at .dev/workflow says is in
// progress; q on feat/waiting, whose item is queued; n on feat/gone, whose
// item has no queue file; m on main-x; d detached; u on feat/Upper. .scratch/p
// is a plain directory and .scratch/o another repository on feat/item.
type pushRepo struct {
	t            *testing.T
	dir, origin  string
	script       string
	gitConfigArg []string
}

func (r pushRepo) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append(r.gitConfigArg, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r pushRepo) write(body string, path ...string) {
	r.t.Helper()
	file := filepath.Join(append([]string{r.dir}, path...)...)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func newPushRepo(t *testing.T) pushRepo {
	t.Helper()
	launcher, _ := launcherSetup(t)
	root := t.TempDir()
	r := pushRepo{t: t, dir: filepath.Join(root, "repo"), origin: filepath.Join(root,
		"origin.git"), script: filepath.Join(filepath.Dir(launcher), "native-push.sh"),
		gitConfigArg: []string{"-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}}
	r.git(root, "init", "-q", "--bare", r.origin)
	r.git(root, "init", "-q", r.dir)
	r.write("one\n", "f.txt")
	r.git(r.dir, "add", "f.txt")
	r.git(r.dir, "commit", "-q", "-m", "one")
	r.git(r.dir, "remote", "add", "origin", r.origin)
	for name, branch := range map[string]string{"w": "feat/item", "q": "feat/waiting",
		"n": "feat/gone", "m": "main-x", "u": "feat/Upper"} {
		wt := filepath.Join(".scratch", name)
		r.git(r.dir, "worktree", "add", "-q", "-b", branch, wt)
	}
	r.git(r.dir, "worktree", "add", "-q", "--detach", filepath.Join(".scratch", "d"))
	r.write("x\n", ".scratch", "p", "f.txt")
	other := filepath.Join(r.dir, ".scratch", "o")
	r.git(r.dir, "init", "-q", "-b", "feat/item", other)
	r.git(other, "commit", "-q", "--allow-empty", "-m", "o")
	r.write("feature: item\nstatus: in_progress\n", ".dev", "workflow", "queue", "item.yaml")
	r.write("feature: waiting\nstatus: queued\n", ".dev", "workflow", "queue",
		"waiting.yaml")
	return r
}

// TestNativePush runs native-push.sh: each refusal is one line and exit 2
// before anything runs; the in-progress item's branch is pushed to the bare
// origin under its own name; a non-fast-forward is refused by git (FLOW-17).
func TestNativePush(t *testing.T) {
	r := newPushRepo(t)
	for _, c := range []struct {
		name string
		args []string
		dir  string
	}{
		{"no name", nil, ""},
		{"two arguments", []string{"w", "--force"}, ""},
		{"empty name", []string{""}, ""},
		{"dotdot", []string{".."}, ""},
		{"slash", []string{"w/x"}, ""},
		{"no worktree", []string{"missing"}, ""},
		{"plain directory", []string{"p"}, ""},
		{"other repository", []string{"o"}, ""},
		{"detached", []string{"d"}, ""},
		{"not feat/", []string{"m"}, ""},
		{"upper case", []string{"u"}, ""},
		{"not queued", []string{"n"}, ""},
		{"not in progress", []string{"q"}, ""},
		{"from a subdirectory", []string{"w"}, ".dev"},
	} {
		t.Run(c.name, func(t *testing.T) { r.refused(t, c.dir, c.args...) })
	}
	if refs := r.git(r.dir, "ls-remote", "origin"); refs != "" {
		t.Fatalf("a refusal pushed: %s", refs)
	}

	wt := filepath.Join(r.dir, ".scratch", "w")
	if _, stderr, code := r.push("", "w"); code != 0 {
		t.Fatalf("push: exit %d: %s", code, stderr)
	}
	head := r.git(wt, "rev-parse", "HEAD")
	if got := r.git(r.dir, "ls-remote", "origin"); got != head+"\trefs/heads/feat/item" {
		t.Fatalf("origin has %q, want feat/item at %s", got, head)
	}
	if up := r.git(wt, "rev-parse", "--abbrev-ref", "@{upstream}"); up != "origin/feat/item" {
		t.Fatalf("upstream %q", up)
	}
	r.git(wt, "commit", "-q", "--amend", "--allow-empty", "-m", "rewritten")
	if _, _, code := r.push("", "w"); code == 0 || code == 2 {
		t.Fatalf("a non-fast-forward push: exit %d, want git's refusal", code)
	}
	if got := r.git(r.dir, "ls-remote", "origin"); got != head+"\trefs/heads/feat/item" {
		t.Fatalf("the non-fast-forward moved origin: %q", got)
	}
}

// refused runs native-push.sh and fails unless it refused with one line and
// exit 2.
func (r pushRepo) refused(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, stderr, code := runLauncher(t, r.script, filepath.Join(r.dir, dir), args)
	if code != 2 || out != "" || strings.Count(stderr, "\n") != 1 ||
		!strings.HasPrefix(stderr, "native-push.sh: ") {
		t.Fatalf("exit %d, stdout %q, stderr %q; want one refusal line", code, out,
			stderr)
	}
}

// push runs native-push.sh with sh in the subdirectory dir of the checkout.
func (r pushRepo) push(dir string, args ...string) (string, string, int) {
	r.t.Helper()
	return runLauncher(r.t, r.script, filepath.Join(r.dir, dir), args)
}

// TestCommitFromFile runs the commit form a machine's session uses, since the
// deny Bash(git -C * push*) refuses a -m message that contains " push": the
// message in .scratch/<name>.msg, beside the worktree, and git -C
// .scratch/<name> commit -F ../<name>.msg. It commits, and the worktree is
// left clean for the wrapper's cleanup (FLOW-16, FLOW-17).
func TestCommitFromFile(t *testing.T) {
	r := newPushRepo(t)
	r.write("two\n", ".scratch", "w", "f.txt")
	const msg = "a push fix\n\nThe body.\n"
	r.write(msg, ".scratch", "w.msg")
	r.git(r.dir, "-C", filepath.Join(".scratch", "w"), "add", "f.txt")
	r.git(r.dir, "-C", filepath.Join(".scratch", "w"), "commit", "-q", "-F", "../w.msg")
	wt := filepath.Join(r.dir, ".scratch", "w")
	if got := r.git(wt, "log", "-1", "--format=%B"); got != strings.TrimSpace(msg) {
		t.Fatalf("committed message %q, want %q", got, msg)
	}
	if st := r.git(wt, "status", "--porcelain"); st != "" {
		t.Fatalf("the worktree is not clean: %q", st)
	}
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// nativeDirs is a state whose machine m serves container-any and linux-kvm
// and lists o/here and o/specs, and a dedicated checkout of o/here whose
// profile file prepares container-any with a command that fails and
// linux-kvm with one that passes.
func nativeDirs(t *testing.T) (dir, repo string) {
	t.Helper()
	dir = stateDir(t, here)
	write(t, dir, "hosts.yaml", "machines:\n  - name: m\n    hosts: [container-any, "+
		"linux-kvm, qemu-any]\n    repositories: [o/here, o/specs]\n")
	repo = codeRepo(t)
	_ = os.MkdirAll(filepath.Join(repo, "docs", "development"), 0o755)
	write(t, repo, "docs/development/loop-profile.yaml", "version: 1\ntrunk: main\n"+
		"toolchain_check: [git, --version]\nnative:\n  prepare:\n"+
		"    container-any: [[git, no-such-command]]\n    linux-kvm: [[git, --version]]\n"+
		"  check:\n    linux-kvm: [[git, no-such-command]]\n")
	return dir, repo
}

// TestNativeCommand: wf native lists the clones to reset, runs each
// profile's native.prepare for the machine's hosts with a failure staling
// only its host, and checks the clones at install without making one
// (Validation plan, Native; a14).
func TestNativeCommand(t *testing.T) {
	dir, repo := nativeDirs(t)
	args := []string{"-repo", repo, "-machine", "m"}
	if got := must(t, dir, "native", append(args, "-clones")...); got != "o/specs\tmain\n" {
		t.Errorf("wf native -clones = %q", got)
	}
	out := must(t, dir, "native", append(args, "-prepare")...)
	if !strings.Contains(out, "stale\tcontainer-any\tgit no-such-command in o/here failed") ||
		!strings.Contains(out, "prepared\tlinux-kvm\tgit --version in o/here") ||
		strings.Contains(out, "qemu-any") {
		t.Errorf("wf native -prepare printed:\n%s", out)
	}

	out, err := wf(t, dir, "native", append(args, "-check")...)
	if err == nil || !strings.Contains(out, "FAIL  no clone of o/specs: run git clone "+
		"https://github.com/o/specs.git") {
		t.Fatalf("no clone: err %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".dev", "checkouts")); err == nil {
		t.Fatal("wf native -check made a clone directory")
	}
	other := codeRepo(t)
	named := filepath.Join(t.TempDir(), "o", "specs.git")
	gitT(t, filepath.Dir(other), "clone", "-q", "--bare", "origin.git", named)
	clone := filepath.Join(repo, ".dev", "checkouts", "o", "specs")
	gitT(t, repo, "clone", "-q", named, filepath.Join(".dev", "checkouts", "o", "specs"))
	entireHooksIn(t, repo)
	entireHooksIn(t, clone)
	out = must(t, dir, "native", append(args, "-check")...)
	for _, w := range []string{"ok    the clone", "ok    the clone of o/specs is clean",
		"ok    Entire git hooks in " + filepath.ToSlash(repo),
		"ok    Entire git hooks in " + filepath.ToSlash(clone),
		"ok    o/here toolchain: git --version",
		"warn  o/here linux-kvm: git no-such-command"} {
		if !strings.Contains(out, w) {
			t.Errorf("wf native -check lacks %q:\n%s", w, out)
		}
	}
}

// TestNativeCheckEntireHooks: wf native -check fails a checkout whose hooks
// directory lacks one of Entire's five git hooks, or holds one that is not
// Entire's, naming the hooks and entire enable (a6, FLOW-18).
func TestNativeCheckEntireHooks(t *testing.T) {
	dir, repo := nativeDirs(t)
	write(t, dir, "hosts.yaml", "machines:\n  - name: m\n    hosts: [linux-kvm]\n")
	args := []string{"native", "-repo", repo, "-machine", "m", "-check"}
	out, err := wf(t, dir, args[0], args[1:]...)
	want := "FAIL  Entire git hooks in " + filepath.ToSlash(repo) + ": prepare-commit-msg, " +
		"commit-msg, post-commit, post-rewrite, pre-push missing or not Entire's; run " +
		"entire enable in " + filepath.ToSlash(repo)
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("no hooks: err %v, want %q in\n%s", err, want, out)
	}
	entireHooksIn(t, repo)
	hooks := gitHooksDir(t, repo)
	write(t, hooks, "pre-push", "#!/bin/sh\nexit 0\n")
	out, err = wf(t, dir, args[0], args[1:]...)
	if err == nil || !strings.Contains(out, ": pre-push missing or not Entire's") {
		t.Fatalf("a foreign pre-push: err %v\n%s", err, out)
	}
	entireHooksIn(t, repo)
	out = must(t, dir, args[0], args[1:]...)
	if !strings.Contains(out, "ok    Entire git hooks in "+filepath.ToSlash(repo)) {
		t.Errorf("all five hooks:\n%s", out)
	}
}

// gitHooksDir returns the hooks directory git uses in checkout dir.
func gitHooksDir(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--git-path", "hooks")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	hooks := strings.TrimSpace(string(out))
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(dir, hooks)
	}
	return hooks
}

// entireHooksIn writes stand-ins for Entire's five git hooks into checkout
// dir, each naming entire as Entire's own do.
func entireHooksIn(t *testing.T, dir string) {
	t.Helper()
	hooks := gitHooksDir(t, dir)
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"prepare-commit-msg", "commit-msg", "post-commit",
		"post-rewrite", "pre-push"} {
		write(t, hooks, h, "#!/bin/sh\n# Entire CLI hooks\nentire hooks git "+h+"\n")
	}
}

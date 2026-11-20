package main

import (
	"os"
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
	gitT(t, repo, "clone", "-q", named, filepath.Join(".dev", "checkouts", "o", "specs"))
	out = must(t, dir, "native", append(args, "-check")...)
	for _, w := range []string{"ok    the clone", "ok    the clone of o/specs is clean",
		"ok    o/here toolchain: git --version",
		"warn  o/here linux-kvm: git no-such-command"} {
		if !strings.Contains(out, w) {
			t.Errorf("wf native -check lacks %q:\n%s", w, out)
		}
	}
}

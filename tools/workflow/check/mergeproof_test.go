package check

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// numbered is f.txt as main holds it: the lines 1 to 40.
func numbered() string {
	var b strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "%d\n", i)
	}
	return b.String()
}

// addAfter returns an edit that puts the line ADDED after line n.
func addAfter(n int) func(string) string {
	return func(s string) string {
		at := fmt.Sprintf("\n%d\n", n)
		return strings.Replace("\n"+s, at, at+"ADDED\n", 1)[1:]
	}
}

// proofRepo is a checkout whose main holds f.txt and whose feat/demo, which
// has merged that main, adds ADDED after line 5 and is pushed. It leaves
// feat/demo checked out.
func proofRepo(t *testing.T) string {
	t.Helper()
	work := repo(t, design)
	git(t, work, "checkout", "-q", "main")
	commitEdit(t, work, "f.txt", "numbers", func(string) string { return numbered() })
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "checkout", "-q", "feat/demo")
	git(t, work, "merge", "-q", "--no-edit", "main")
	commitEdit(t, work, "f.txt", "add a line", addAfter(5))
	git(t, work, "push", "-q", "origin", "feat/demo")
	git(t, work, "fetch", "-q", "origin")
	return work
}

// squashed squash-merges feat/demo as PR #42, has GitHub delete the remote
// branch, checks feat/demo out again and returns the squash.
func squashed(t *testing.T, work string) string {
	t.Helper()
	squash(t, work, "#42")
	sha := fullHead(t, work, "main")
	git(t, work, "push", "-q", "origin", ":feat/demo")
	git(t, work, "fetch", "-q", "--prune", "origin")
	git(t, work, "checkout", "-q", "feat/demo")
	return sha
}

// TestMergeTreeProof: the third proof of refMerged holds only when merging
// the ref onto the PR commit's parent, from their single merge base, writes
// exactly the PR commit's tree (a14 (d) and Decision 23 of the loop-parallel
// design). Each case is the review's scenario of the same name; a ref the
// proof does not confirm leaves the item at its merge step (FLOW-08, FLOW-16).
func TestMergeTreeProof(t *testing.T) {
	const local = "refs/heads/feat/demo"
	check := func(t *testing.T, work, sha string, want bool) {
		t.Helper()
		info, ok := proof(work, local, sha)
		if ok != want {
			t.Errorf("proof of %s as %s: %v (%s), want %v", local, sha, ok, info, want)
		}
		if !want && !strings.Contains(info, sha) {
			t.Errorf("info %q does not name the PR commit %s", info, sha)
		}
		if rs := Phase(Env{Repo: work}, x3(42)); OK(rs) != want {
			t.Errorf("x3: %v (%s), want %v", OK(rs), failures(rs)+rs[0].Info, want)
		}
	}
	t.Run("ok_squash", func(t *testing.T) {
		work := proofRepo(t)
		check(t, work, squashed(t, work), true)
	})
	t.Run("adjacent_old", func(t *testing.T) {
		work := proofRepo(t)
		pre := fullHead(t, work, "feat/demo")
		git(t, work, "checkout", "-q", "main")
		commitEdit(t, work, "f.txt", "edit line 7", func(s string) string {
			return strings.Replace(s, "\n7\n", "\nSEVEN\n", 1)
		})
		git(t, work, "push", "-q", "origin", "main")
		git(t, work, "checkout", "-q", "feat/demo")
		git(t, work, "merge", "-q", "--no-edit", "main")
		git(t, work, "push", "-q", "origin", "feat/demo")
		sha := squashed(t, work)
		git(t, work, "reset", "-q", "--hard", pre)
		check(t, work, sha, true)
	})
	after := map[string]func(t *testing.T, work string){
		"move_after": func(t *testing.T, work string) {
			commitEdit(t, work, "f.txt", "move the line", func(s string) string {
				return addAfter(30)(strings.Replace(s, "ADDED\n", "", 1))
			})
		},
		"chmod_after": func(t *testing.T, work string) {
			git(t, work, "update-index", "--chmod=+x", "f.txt")
			git(t, work, "commit", "-q", "-m", "chmod")
		},
		"bin_after": func(t *testing.T, work string) {
			commitEdit(t, work, "bin.dat", "binary", func(string) string {
				return "\x00\x01\x02binary"
			})
		},
		"empty_after": func(t *testing.T, work string) {
			commitEdit(t, work, "empty", "an empty file",
				func(string) string { return "" })
		},
	}
	for name, fn := range after {
		t.Run(name, func(t *testing.T) {
			work := proofRepo(t)
			sha := squashed(t, work)
			fn(t, work)
			check(t, work, sha, false)
		})
	}
	t.Run("other_pr", func(t *testing.T) {
		work := proofRepo(t)
		git(t, work, "checkout", "-q", "main")
		commitEdit(t, work, "f.txt", "a fix naming (#42)", addAfter(35))
		git(t, work, "push", "-q", "origin", "main")
		git(t, work, "fetch", "-q", "origin")
		git(t, work, "checkout", "-q", "feat/demo")
		check(t, work, fullHead(t, work, "main"), false)
		// wf stale asks Merged before it lands an item at its merge step.
		f := x3(42)
		if Merged(Env{Repo: work}, f) {
			t.Error("Merged reports a branch that never merged as merged")
		}
	})
	t.Run("no merge base", func(t *testing.T) {
		work := proofRepo(t)
		sha := squashed(t, work)
		git(t, work, "checkout", "-q", "--orphan", "unrelated")
		commitEdit(t, work, "f.txt", "unrelated history", addAfter(5))
		git(t, work, "branch", "-q", "-f", "feat/demo", "unrelated")
		check(t, work, sha, false)
		if info, _ := proof(work, local, sha); !strings.Contains(info, "no merge base") {
			t.Errorf("info %q, want it to say there is no merge base", info)
		}
	})
	// Several merge bases: TestCrissCrossMergeBasesFail.
}

// TestMergeTreeProofNeedsGit240: a git without merge-tree's --merge-base,
// which git 2.40 added, does not prove the ref; the line says why, with
// git's error and version, instead of reading a failed run as a result.
func TestMergeTreeProofNeedsGit240(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in git is a shell script")
	}
	const ref = "refs/heads/feat/demo"
	work := mainMoved(t)
	squash(t, work, "#9")
	sha := fullHead(t, work, "main")
	if info, ok := proof(work, ref, sha); !ok {
		t.Fatalf("the case needs the proof to hold with this machine's git: %s", info)
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --merge-base=*)\n" +
		"  echo \"error: unknown option \\`merge-base'\" >&2; exit 129;; esac; done\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	info, ok := proof(work, ref, sha)
	if ok {
		t.Fatal("proven with a git that refuses --merge-base")
	}
	for _, want := range []string{"not compared", "unknown option", "git 2.40",
		"git version"} {
		if !strings.Contains(info, want) {
			t.Errorf("info %q, want %q", info, want)
		}
	}
	if rs := Phase(Env{Repo: work}, x3(9)); OK(rs) {
		t.Errorf("x3 with a git that refuses --merge-base: %s", rs[0].Info)
	}
}

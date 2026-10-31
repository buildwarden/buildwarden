// Package gittest builds scratch git repositories for tier 1's tests, with
// fixed identities and dates so every run makes the same commits (TEST-04).
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Repo is a scratch checkout whose origin is a bare repository beside it.
type Repo struct {
	T   testing.TB
	Dir string
}

var env = []string{
	"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
	"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
}

// New returns a checkout on main with one commit holding files, pushed to
// origin, and then switched to a new branch feat/x.
func New(t testing.TB, files map[string]string) *Repo {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	r := &Repo{T: t, Dir: filepath.Join(root, "work")}
	r.Git("init", "-q", "--bare", "-b", "main", origin)
	r.Git("init", "-q", "-b", "main", r.Dir)
	r.Git("-C", r.Dir, "remote", "add", "origin", origin)
	r.Commit("base", files)
	r.Git("-C", r.Dir, "push", "-q", "origin", "main")
	r.Git("-C", r.Dir, "checkout", "-q", "-b", "feat/x")
	return r
}

// Git runs git in the checkout (or, for init, wherever its arguments say).
func (r *Repo) Git(args ...string) string {
	r.T.Helper()
	cmd := exec.Command("git", args...)
	if _, err := os.Stat(r.Dir); err == nil {
		cmd.Dir = r.Dir
	}
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Commit writes files (an empty content deletes the file) and commits them
// with msg.
func (r *Repo) Commit(msg string, files map[string]string) {
	r.T.Helper()
	for name, body := range files {
		p := filepath.Join(r.Dir, filepath.FromSlash(name))
		if body == "" {
			if err := os.Remove(p); err != nil {
				r.T.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.T.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			r.T.Fatal(err)
		}
	}
	r.Git("add", "-A")
	r.Git("commit", "-q", "--allow-empty", "-m", msg)
}

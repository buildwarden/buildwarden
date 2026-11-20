package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// specsClone makes a code repository with the branches, as codeRepo does,
// and clones it as o/specs under a new -checkouts directory, which it
// returns with the clone.
func specsClone(t *testing.T, names ...string) (checkouts, clone string) {
	t.Helper()
	other := codeRepo(t, names...)
	checkouts = t.TempDir()
	clone = filepath.Join(checkouts, "o", "specs")
	gitT(t, checkouts, "clone", "-q", filepath.Join(filepath.Dir(other), "origin.git"), clone)
	for _, n := range names {
		gitT(t, clone, "branch", "-q", "feat/"+n, "origin/feat/"+n)
	}
	return checkouts, clone
}

// TestMainAndAreasPerRepository: wf main -repository records a second
// repository's trunk in repos/<owner>/<name>/main.yaml and leaves the root's
// alone; wf areas -check -repository compares that repository's map with its
// clone's trunk and the edges given (Validation plan test 13, Decision 8).
func TestMainAndAreasPerRepository(t *testing.T) {
	dir := stateDir(t, here)
	sha := strings.Repeat("ab", 20)
	must(t, dir, "main", "-repository", "o/specs", "-sha", sha, "-ci", "red")
	specsMain := filepath.Join(dir, "repos", "o", "specs", "main.yaml")
	if _, err := os.Stat(specsMain); err != nil {
		t.Fatalf("no %s: %v", specsMain, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.yaml")); err == nil {
		t.Fatal("wf main -repository wrote the root main.yaml")
	}
	s := load(t, dir)
	if m := s.MainOf("o/specs"); m == nil || m.CI != "red" || m.SHA != sha || s.Main != nil {
		t.Fatalf("o/specs main %+v, root %+v", m, s.Main)
	}
	must(t, dir, "main", "-sha", sha, "-ci", "green")
	if m := load(t, dir).Main; m == nil || m.CI != "green" {
		t.Fatalf("root main %+v", m)
	}
	refused(t, dir, "config.yaml's repositories has no entry for o/new", "main",
		"-repository", "o/new", "-sha", sha, "-ci", "green")

	checkouts, clone := specsClone(t, "x")
	write(t, filepath.Dir(specsMain), "areas.yaml", strings.Replace(
		smallMap("ordered", "base.txt, shared.txt", "y.txt"), "o/here", "o/specs", 1))
	edges := filepath.Join(t.TempDir(), "edges")
	if err := os.WriteFile(edges, []byte("x.txt y.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := must(t, dir, "areas", "-check", "-repository", "o/specs", "-checkouts", checkouts,
		"-edges", edges)
	if !strings.Contains(out, "pattern: x.txt of area x matches no file") ||
		!strings.Contains(out, "from origin/main at "+
			gitT(t, clone, "rev-parse", "--short", "origin/main")) {
		t.Errorf("wf areas -check -repository printed:\n%s", out)
	}
	if m := load(t, dir).MainOf("o/specs"); m == nil || m.Areas == nil {
		t.Fatalf("o/specs main.yaml records no area check: %+v", m)
	}
	if m := load(t, dir).Main; m.Areas != nil {
		t.Errorf("the root main.yaml records o/specs's area check: %+v", m.Areas)
	}
}

// TestStaleSecondBranch: wf stale seeds a refresh when only the item's
// branch in its second repository is behind that repository's trunk, and
// the note names the repository (Validation plan test 6, FLOW-10).
func TestStaleSecondBranch(t *testing.T) {
	work, dir := codeRepo(t, "a"), stateDir(t, here)
	checkouts, clone := specsClone(t, "a")
	reach(t, dir, work, "a", "1", 7, 7)
	edit(t, dir, "a", func(f *queue.Feature) {
		f.Repos = []string{"o/here", "o/specs"}
		f.Work = []queue.Work{{Repo: "o/specs", Branch: "feat/a", PR: &queue.PR{Number: 3,
			CI: "green", Head: gitT(t, clone, "rev-parse", "origin/feat/a")}}}
	})
	stale := func() []string {
		out := must(t, dir, "stale", "-repo", work, "-checkouts", checkouts, "-worker",
			"cloud/run")
		if strings.TrimSpace(out) == "" {
			return nil
		}
		return strings.Split(strings.TrimSpace(out), "\n")
	}
	if got := stale(); got != nil {
		t.Fatalf("both branches current: wf stale printed %q", got)
	}
	moveMain(t, clone, "specs moved", "shared.txt")
	main := gitT(t, clone, "rev-parse", "--short", "origin/main")
	note := "o/specs: main at " + main + ": 1 commit since the branch merged it; both " +
		"change shared.txt"
	if got := stale(); !slices.Equal(got, []string{"refresh\ta\tf6-refresh\t" + note}) {
		t.Fatalf("wf stale = %q, want the refresh for o/specs", got)
	}
	must(t, dir, "validate")
	// A red trunk of the second repository holds the refresh (FLOW-10).
	must(t, dir, "main", "-repository", "o/specs", "-sha", gitT(t, clone, "rev-parse",
		"origin/main"), "-ci", "red")
	if got := must(t, dir, "next", "-worker", "cloud/run"); !strings.Contains(got,
		"the trunk of o/specs is red at") {
		t.Errorf("wf next with o/specs red = %q", got)
	}
}

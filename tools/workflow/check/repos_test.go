package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// specsProfile is a second repository's profile file: a fast gate of its
// own and a guide directory.
const specsProfile = "version: 1\ntrunk: main\nfast_gate:\n  - [git, --version]\n" +
	"reference_docs:\n  guides: [guide/]\n"

// second makes a clone of a second repository o/specs under checkouts, with
// main pushed, and returns the clone. With profile, main carries it as the
// repository's profile file.
func second(t *testing.T, checkouts, profile string) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	clone := filepath.Join(checkouts, "o", "specs")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, root, "clone", "-q", origin, clone)
	git(t, clone, "checkout", "-q", "-b", "main")
	files := map[string]string{"spec.md": "v1\n"}
	if profile != "" {
		files["loop-profile.yaml"] = profile
	}
	for p, text := range files {
		if err := os.WriteFile(filepath.Join(clone, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, clone, "add", ".")
	git(t, clone, "commit", "-q", "-m", "base")
	git(t, clone, "push", "-q", "origin", "main")
	return clone
}

// branchIn commits a change to file on feat/demo in the clone and pushes it.
func branchIn(t *testing.T, clone, file string) {
	t.Helper()
	git(t, clone, "checkout", "-q", "-B", "feat/demo")
	full := filepath.Join(clone, filepath.FromSlash(file))
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", ".")
	git(t, clone, "commit", "-q", "-m", "change "+file)
	git(t, clone, "push", "-q", "origin", "feat/demo")
	git(t, clone, "fetch", "-q", "origin")
}

// twoRepos is an item homed here with a work entry in o/specs, and the
// environment that reads both.
func twoRepos(t *testing.T, phase int, profile string) (Env, *queue.Feature, string) {
	t.Helper()
	checkouts := t.TempDir()
	clone := second(t, checkouts, profile)
	env := here(repo(t, design))
	env.Checkouts = checkouts
	entry := queue.RepositoryEntry{Name: "o/specs", Trunk: "main"}
	if profile != "" {
		entry.Profile = "loop-profile.yaml"
	}
	env.State.Config.Repositories = append(env.State.Config.Repositories, entry)
	f := demo(phase)
	f.Repos = []string{hereRepo, "o/specs"}
	f.Work = []queue.Work{{Repo: "o/specs", Branch: "feat/demo"}}
	return env, f, clone
}

// named returns the results whose name ends with " in o/specs".
func named(rs []Result) []Result {
	var out []Result
	for _, r := range rs {
		if strings.HasSuffix(r.Name, " in o/specs") {
			out = append(out, r)
		}
	}
	return out
}

// TestGatesPerRepository: an item with a work entry is checked in each
// repository it has a branch in, through that repository's profile and
// against its trunk; a repository with no profile gets no fast gate and no
// reference-document check (Validation plan test 4, Decision 5).
func TestGatesPerRepository(t *testing.T) {
	env, f, clone := twoRepos(t, 0, specsProfile)
	if got := failures(named(Phase(env, f))); !strings.Contains(got,
		"branch on remote in o/specs") {
		t.Fatalf("phase 0 with no branch in o/specs: failures %q", got)
	}
	branchIn(t, clone, "spec.md")
	if rs := Phase(env, f); !OK(rs) || len(named(rs)) != 1 {
		t.Fatalf("phase 0 with both branches: %+v", rs)
	}

	f.Phase = 3
	gate := result(t, Phase(env, f), "git --version in o/specs")
	if gate.OK || gate.Info != "skipped (run with -run)" {
		t.Errorf("phase 3 fast gate in o/specs: %+v", gate)
	}
	env.Run = true
	if r := result(t, Phase(env, f), "git --version in o/specs"); !r.OK {
		t.Errorf("phase 3 fast gate in o/specs, run: %+v", r)
	}
	env.Run = false

	// The design declares surfaces, so each adopted repository's guides
	// must change.
	f.Phase = 5
	if r := result(t, Phase(env, f), "guides or examples updated in o/specs"); r.OK {
		t.Errorf("phase 5 with no guide changed in o/specs: %+v", r)
	}
	branchIn(t, clone, "guide/spec.md")
	if r := result(t, Phase(env, f), "guides or examples updated in o/specs"); !r.OK {
		t.Errorf("phase 5 with a guide changed in o/specs: %+v", r)
	}

	f.Phase = 6
	if r := result(t, Phase(env, f), "PR open with CI green (FLOW-11) in o/specs"); r.OK {
		t.Errorf("phase 6 with no PR in o/specs: %+v", r)
	}
	f.Work[0].PR = greenPR(t, clone, 7)
	if r := result(t, Phase(env, f), "PR open with CI green (FLOW-11) in o/specs"); !r.OK {
		t.Errorf("phase 6 with a green PR in o/specs: %+v", r)
	}
	if r := result(t, Phase(env, f), "current main merged in o/specs"); !r.OK {
		t.Errorf("phase 6, trunk merged in o/specs: %+v", r)
	}

	f.Phase = 8
	if got := failures(named(Phase(env, f))); !strings.Contains(got,
		"remote branch deleted (FLOW-16) in o/specs") {
		t.Errorf("phase 8 with the branch left in o/specs: failures %q", got)
	}
}

// TestNoProfileNoGate: a repository whose entry names no profile has no fast
// gate and no reference documents, and its branch checks still run.
func TestNoProfileNoGate(t *testing.T) {
	env, f, clone := twoRepos(t, 3, "")
	branchIn(t, clone, "spec.md")
	gates := func(rs []Result) []Result {
		var out []Result
		for _, r := range named(rs) {
			if !r.OK || strings.Contains(r.Name, "fast gate") ||
				strings.Contains(r.Name, "docs") ||
				strings.Contains(r.Name, "guides") {
				out = append(out, r)
			}
		}
		return out
	}
	if rs := gates(Phase(env, f)); len(rs) != 0 {
		t.Errorf("phase 3 in a repository with no profile: %+v", rs)
	}
	f.Phase = 5
	if rs := gates(Phase(env, f)); len(rs) != 0 {
		t.Errorf("phase 5 in a repository with no profile: %+v", rs)
	}
	f.Phase = 0
	if rs := named(Phase(env, f)); len(rs) != 1 || !rs[0].OK {
		t.Errorf("phase 0 in a repository with no profile: %+v", rs)
	}
}

// TestCloneNeeded: a check that needs a second repository's clone fails,
// naming -checkouts and the expected path, both with no flag and with no
// clone under it; an item homed in a repository with no entry fails the
// same way (Validation plan test 15, GLOBAL-11).
func TestCloneNeeded(t *testing.T) {
	env, f, _ := twoRepos(t, 0, specsProfile)
	dir := env.Checkouts
	env.Checkouts = ""
	if got := failures(Phase(env, f)); !strings.Contains(got, "o/specs is not the queue's "+
		"own repository, and no -checkouts is given: wf reads it in DIR/o/specs") {
		t.Errorf("no -checkouts: failures %q", got)
	}
	env.Checkouts = t.TempDir()
	want := "no clone of o/specs at " + filepath.Join(env.Checkouts, "o", "specs")
	if got := failures(Phase(env, f)); !strings.Contains(got, want) {
		t.Errorf("no clone: failures %q, want %q", got, want)
	}
	env.Checkouts = dir
	env.State.Config.Repositories = env.State.Config.Repositories[:1]
	if got := failures(Phase(env, f)); !strings.Contains(got,
		"config.yaml's repositories has no entry for o/specs") {
		t.Errorf("no entry: failures %q", got)
	}
	f.Repos, f.Work = []string{"o/specs"}, nil
	if got := failures(Phase(env, f)); !strings.HasPrefix(got,
		"repository (LOOP-01): config.yaml's repositories has no entry for o/specs") {
		t.Errorf("homed with no entry: failures %q", got)
	}
}

// TestProfileTrunkDiffers: a profile file whose trunk is not the entry's
// fails the checks that read it, naming both (LOOP-01).
func TestProfileTrunkDiffers(t *testing.T) {
	env, f, clone := twoRepos(t, 3, strings.Replace(specsProfile, "trunk: main",
		"trunk: feat/demo", 1))
	branchIn(t, clone, "spec.md")
	r := result(t, Phase(env, f), "fast gate (LOOP-01) in o/specs")
	if r.OK || !strings.Contains(r.Info, `gives trunk "feat/demo", but config.yaml's `+
		`entry gives "main"`) {
		t.Errorf("trunk differs: %+v", r)
	}
}

// mergeIn merges feat/demo into main in the checkout as a pull request's
// merge commit and pushes main.
func mergeIn(t *testing.T, work string, pr string) {
	t.Helper()
	git(t, work, "checkout", "-q", "main")
	git(t, work, "merge", "-q", "--no-ff", "-m", "Merge pull request #"+pr+
		" from o/feat/demo", "feat/demo")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "fetch", "-q", "origin")
	git(t, work, "checkout", "-q", "feat/demo")
}

// TestMergedPerRepository: the merge step passes only once every pull
// request is merged into its own trunk, and f6-merged-<n> only on the merge
// of its entry (Validation plan tests 4 and 5, FLOW-08).
func TestMergedPerRepository(t *testing.T) {
	env, f, clone := twoRepos(t, 7, specsProfile)
	branchIn(t, clone, "spec.md")
	f.PR, f.Work[0].PR = &queue.PR{Number: 5}, &queue.PR{Number: 7}
	got := failures(Phase(env, f))
	if !strings.Contains(got, "merged by a human (FLOW-08): ") ||
		!strings.Contains(got, "merged by a human (FLOW-08) in o/specs") {
		t.Fatalf("nothing merged: failures %q", got)
	}
	if OK(MergedFirst(env, f, 1)) {
		t.Fatal("f6-merged-1 passed before the merge")
	}
	if got := WorkMerges(env, f); got[0] != "" {
		t.Fatalf("WorkMerges before the merge: %q", got)
	}
	mergeIn(t, clone, "7")
	if rs := MergedFirst(env, f, 1); !OK(rs) {
		t.Fatalf("f6-merged-1 after the merge: %+v", rs)
	}
	if got, want := WorkMerges(env, f), strings.TrimSpace(gitOut(t, clone, "rev-parse",
		"origin/main")); got[0] != want {
		t.Fatalf("WorkMerges = %q, want the PR's merge %s", got, want)
	}
	if OK(Phase(env, f)) {
		t.Fatal("phase 7 passed with the home's PR not merged")
	}
	mergeIn(t, env.Repo, "5")
	if rs := Phase(env, f); !OK(rs) {
		t.Fatalf("phase 7 with both merged: %+v", rs)
	}
	if rs := MergedFirst(env, f, 2); OK(rs) {
		t.Fatalf("f6-merged-2 with one work entry: %+v", rs)
	}
}

// TestReportHeads: Review's report check passes only when the report's
// heads are the item's branch heads in every repository (a12, LINT-04).
func TestReportHeads(t *testing.T) {
	env, f, clone := twoRepos(t, 4, specsProfile)
	branchIn(t, clone, "spec.md")
	env.Workflow = t.TempDir()
	home := strings.TrimSpace(gitOut(t, env.Repo, "rev-parse", "--short=12", "feat/demo"))
	specs := strings.TrimSpace(gitOut(t, clone, "rev-parse", "--short=12", "feat/demo"))
	dir := filepath.Join(env.Workflow, "reports", "demo")
	_ = os.MkdirAll(dir, 0o755)
	write := func(json string) {
		for _, ext := range []string{".md", ".json"} {
			err := os.WriteFile(filepath.Join(dir, home+ext), []byte(json), 0o644)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	const name = "tier-2 report heads (LINT-04)"
	for _, tc := range []struct {
		why, json string
		ok        bool
	}{
		{"no heads", `{}`, false},
		{"the second head missing", `{"heads": {"o/here": "` + home + `"}}`, false},
		{"a stale head", `{"heads": {"o/here": "` + home +
			`", "o/specs": "0123456789ab"}}`, false},
		{"every head", `{"heads": {"o/here": "` + home + `", "o/specs": "` + specs + `"}}`,
			true},
	} {
		write(tc.json)
		if r := result(t, Phase(env, f), name); r.OK != tc.ok {
			t.Errorf("%s: %+v", tc.why, r)
		}
	}
	// An item with no work entry may have a report without heads.
	f.Repos, f.Work = nil, nil
	write(`{}`)
	for _, r := range Phase(env, f) {
		if r.Name == name {
			t.Errorf("no work, no heads: %+v", r)
		}
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// specsMap is o/specs's area map: its specification files and its guides.
const specsMap = "version: 1\nrepository: o/specs\ncatch_all: core\nareas:\n" +
	"  schema:\n    mode: exclusive\n    paths: [spec.md]\n" +
	"  guide:\n    mode: exclusive\n    paths: [guide/]\n  core:\n    mode: exclusive\n"

// TestInsideAreasPerRepository: a path the branch changes in a second
// repository lies in an area the design declares there, read against that
// repository's map under repos/ (Validation plan test 4, FLOW-03).
func TestInsideAreasPerRepository(t *testing.T) {
	env, f, clone := twoRepos(t, 3, "")
	branchIn(t, clone, "guide/spec.md")
	env.State.Dir = t.TempDir()
	dir := filepath.Join(env.State.Dir, "repos", "o", "specs")
	_ = os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "areas.yaml"), []byte(specsMap),
		0o644); err != nil {
		t.Fatal(err)
	}
	const name = "diff inside the declared areas (FLOW-03) in o/specs"
	f.Areas = &[]string{"o/specs:schema"}
	if r := result(t, Phase(env, f), name); r.OK ||
		!strings.Contains(r.Info, "guide/spec.md (area guide)") {
		t.Errorf("a path outside the areas declared there: %+v", r)
	}
	f.Areas = &[]string{"o/specs:schema", "o/specs:guide"}
	if r := result(t, Phase(env, f), name); !r.OK {
		t.Errorf("every path inside: %+v", r)
	}
}

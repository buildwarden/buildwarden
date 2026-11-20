package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// kindBefore is kindOf as wf hard-coded it before it read the profile file.
func kindBefore(p string) pathKind {
	switch {
	case strings.HasPrefix(p, "docs/development/"), strings.HasPrefix(p, "tools/tier1/"),
		strings.HasPrefix(p, "tools/workflow/check/"), p == "docs/design/TEMPLATE.md":
		return rulePath
	case strings.HasPrefix(p, ".claude/"), strings.HasPrefix(p, "tools/workflow/"):
		return machinery
	}
	return ordinary
}

// TestKindOfHere: this repository's profile file classifies every path of
// its tree as wf did before it read the file (Validation plan test 3).
func TestKindOfHere(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Skipf("no checkout of the repository to list: %v", err)
	}
	p := hereProfile(t)
	k, err := newClassifier(profilePath, profilePath, p.Rules)
	if err != nil {
		t.Fatal(err)
	}
	paths := strings.Fields(string(out))
	if len(paths) < 100 {
		t.Fatalf("only %d paths listed", len(paths))
	}
	for _, path := range append(paths, ".claude/x", "tools/workflow/x/y.go") {
		if got, want := k.kindOf(path), kindBefore(path); got != want {
			t.Errorf("kindOf(%s) = %d, was %d", path, got, want)
		}
	}
}

// onMain commits files to main and pushes it, then merges main into
// feat/demo and pushes that, unless merge is false.
func onMain(t *testing.T, work string, merge bool, files map[string]string) {
	t.Helper()
	git(t, work, "checkout", "-q", "main")
	for p, text := range files {
		if text == "" {
			git(t, work, "rm", "-q", p)
			delete(files, p)
		}
	}
	if len(files) > 0 {
		put(t, work, files)
	}
	git(t, work, "commit", "-q", "--allow-empty", "-m", "main")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "checkout", "-q", "feat/demo")
	if merge {
		git(t, work, "merge", "-q", "--no-edit", "main")
		git(t, work, "push", "-q", "origin", "feat/demo")
	}
	git(t, work, "fetch", "-q", "origin")
}

// TestClearanceReadsTheMergeBase: the clearance classifies paths with the
// profile file as it is at the branch's merge base, so a branch cannot free
// its own edits by editing the file (Decision 2, Validation plan test 3).
func TestClearanceReadsTheMergeBase(t *testing.T) {
	profile := readProfileText(t)
	for name, edit := range map[string]string{
		"paths narrowed": strings.Replace(profile, "paths: [docs/development/, ",
			"paths: [", 1),
		"rules removed": profile[:strings.Index(profile, "rules:")] +
			profile[strings.Index(profile, "areas:"):],
	} {
		work := repo(t, design)
		env := here(work)
		put(t, work, map[string]string{profilePath: edit,
			"docs/development/process.md": "### FLOW-01\n"})
		got, _ := clearance(t, env, "feature")
		for _, p := range []string{profilePath, "docs/development/process.md"} {
			if !strings.Contains(got, p+" is in no approved rule change") {
				t.Errorf("%s: %s not refused: %q", name, p, got)
			}
		}
		if got, _ := clearance(t, env, "process"); got != "" {
			t.Errorf("%s: a process item's branch: %q", name, got)
		}
	}
}

// TestClearanceRuleJobsAtTheMergeBase: a branch that edits rule_jobs so a CI
// job stops counting, and changes that job, still has the job's change
// guarded.
func TestClearanceRuleJobsAtTheMergeBase(t *testing.T) {
	work := repo(t, design)
	env := here(work)
	const ci = ".github/workflows/ci.yml"
	onMain(t, work, true, map[string]string{ci: "jobs: {lint: {steps: [{run: make lint}]}}\n"})
	put(t, work, map[string]string{
		profilePath: strings.Replace(readProfileText(t), `\bmake\s+(lint|enforcement)\b`,
			"nothing-matches", 1),
		ci: "jobs: {lint: {steps: [{run: make lint || true}]}}\n"})
	got, _ := clearance(t, env, "feature")
	if !strings.Contains(got, ci+" is in no approved rule change") {
		t.Errorf("the rule job's change not guarded: %q", got)
	}
}

// TestClearanceNotAdopted: a repository whose entry names no profile has no
// rule path, even on a branch that adds a profile file and rule files
// (Decision 3).
func TestClearanceNotAdopted(t *testing.T) {
	work := repo(t, design)
	env := here(work)
	env.State.Config.Repositories[0].Profile = ""
	put(t, work, map[string]string{profilePath: readProfileText(t),
		"docs/development/process.md": "### FLOW-01\n",
		"tools/workflow/x.go":         "package x\n"})
	if got, ran := clearance(t, env, "feature"); !ran || got != "" {
		t.Errorf("not adopted: ran %v, %q", ran, got)
	}
}

// TestClearanceAfterAdoption: once the entry names the profile, a branch that
// forked before the adoption fails until it merges its trunk; one that adds
// the file itself has every path under the new file's rule and machinery
// paths treated as a rule path; a file the merge base cannot parse fails,
// naming it (Decision 3, Defaults and overrides).
func TestClearanceAfterAdoption(t *testing.T) {
	work := repo(t, design)
	env := here(work)
	// The repository before its adoption: neither main nor the branch has
	// the file.
	onMain(t, work, true, map[string]string{profilePath: ""})
	onMain(t, work, false, map[string]string{profilePath: readProfileText(t)})
	put(t, work, map[string]string{"relay/x.go": "package relay\n"})
	if got, _ := clearance(t, env, "feature"); !strings.Contains(got,
		"which feat/demo has at neither its merge base") ||
		!strings.Contains(got, profilePath) ||
		!strings.Contains(got, "merge origin/main") {
		t.Errorf("forked before the adoption: %q", got)
	}
	git(t, work, "merge", "-q", "--no-edit", "main")
	git(t, work, "push", "-q", "origin", "feat/demo")
	git(t, work, "fetch", "-q", "origin")
	if got, _ := clearance(t, env, "feature"); got != "" {
		t.Errorf("after merging the trunk: %q", got)
	}

	adds := repo(t, design)
	env = here(adds)
	onMain(t, adds, true, map[string]string{profilePath: ""})
	put(t, adds, map[string]string{profilePath: readProfileText(t),
		"tools/workflow/x.go": "package x\n", "relay/x.go": "package relay\n"})
	got, _ := clearance(t, env, "feature")
	for _, p := range []string{profilePath, "tools/workflow/x.go"} {
		if !strings.Contains(got, p+" is in no approved rule change") {
			t.Errorf("a branch adding the file: %s not a rule path: %q", p, got)
		}
	}
	if strings.Contains(got, "relay/x.go") {
		t.Errorf("an ordinary path classified: %q", got)
	}

	bad := repo(t, design)
	env = here(bad)
	onMain(t, bad, true, map[string]string{profilePath: "version: [\n"})
	put(t, bad, map[string]string{"relay/x.go": "package relay\n"})
	if got, _ := clearance(t, env, "feature"); !strings.Contains(got,
		"profile file o/here:"+profilePath+" at the merge base") {
		t.Errorf("unparseable at the merge base: %q", got)
	}
}

// readProfileText is this repository's profile file's text.
func readProfileText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(profilePath)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

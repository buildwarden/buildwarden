package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const tmpl = `---
feature: <feature-slug>
---

# <Feature title>

## Problem

What is wrong or missing, for whom, and why now.

## Design

The behavior and structure of the change.

## Defaults and overrides

Each default this design adds or changes, why it is the obvious choice
(GLOBAL-09), and what overrides it, used exclusively (GLOBAL-10). Each merge or
fallback, named, with what triggers it and how the user sees it (GLOBAL-10,
GLOBAL-11). "None" when the design adds or changes no default, override, merge
or fallback.

## Implementation plan

Ordered tasks, each citing the rules it must satisfy.
`

const design = `---
feature: demo
status: draft
design_review: skip
drivers: [container]
seams: []
surfaces: [output]
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [ORCH-02]
concepts: [inputs]
---

# Demo

## Problem

Inputs are not recorded.

## Prior art

None.

## Decisions

Record them.

## Questions

None.

## Design

Write inputs/.

## Defaults and overrides

None.

## Cross-driver impact

All drivers share it.

## Validation plan

Unit tests.

## Implementation plan

1. Write inputs/ (ORCH-02).
2. Update the output guide
   (UX-01).
`

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// commit writes the design on feat/demo, commits and pushes it, and returns
// the commit.
func commit(t *testing.T, work, designText string) string {
	t.Helper()
	_ = os.WriteFile(filepath.Join(work, "docs", "design", "demo.md"), []byte(designText),
		0o644)
	git(t, work, "commit", "-q", "-am", "design")
	git(t, work, "push", "-q", "origin", "feat/demo")
	git(t, work, "fetch", "-q", "origin")
	out, err := exec.Command("git", "-C", work, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// archived is design as Publish leaves it: consolidated and archived.
var archived = strings.Replace(design, "status: draft", "status: archived", 1) + `
## Consolidation

Nothing landed that overlaps.

## Archive summary

Built inputs/.
`

// repo returns a checkout whose branch feat/demo carries the design and is
// pushed to a bare origin.
func repo(t *testing.T, designText string) string {
	t.Helper()
	return repoWith(t, tmpl, designText)
}

// repoWith is repo with the given TEMPLATE.md in place of tmpl.
func repoWith(t *testing.T, template, designText string) string {
	t.Helper()
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, root, "clone", "-q", origin, work)
	git(t, work, "checkout", "-q", "-b", "main")
	_ = os.MkdirAll(filepath.Join(work, "docs", "design"), 0o755)
	_ = os.WriteFile(filepath.Join(work, "docs", "design", "TEMPLATE.md"), []byte(template),
		0o644)
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "template")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "checkout", "-q", "-b", "feat/demo")
	_ = os.WriteFile(filepath.Join(work, "docs", "design", "demo.md"), []byte(designText),
		0o644)
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "design")
	git(t, work, "push", "-q", "origin", "feat/demo")
	git(t, work, "fetch", "-q", "origin")
	return work
}

func demo(phase int) *queue.Feature {
	no := false
	return &queue.Feature{Feature: "demo", Round: "feature", Phase: phase,
		Branch: "feat/demo", DesignReview: "skip",
		Design: "docs/design/demo.md", AltersRelationships: &no}
}

func failures(rs []Result) string {
	var out []string
	for _, r := range rs {
		if !r.OK {
			out = append(out, r.Name+": "+r.Info)
		}
	}
	return strings.Join(out, "; ")
}

func TestDesignAndPlanPass(t *testing.T) {
	env := Env{Repo: repo(t, design)}
	for _, phase := range []int{0, 1, 2} {
		if rs := Phase(env, demo(phase)); !OK(rs) {
			t.Errorf("phase %d: %s", phase, failures(rs))
		}
	}
}

func TestDesignFailures(t *testing.T) {
	bad := strings.Replace(design, "Write inputs/.",
		"The behavior and structure of the change.", 1)
	bad = strings.Replace(bad, "## Defaults and overrides\n\nNone.",
		"## Defaults and overrides\n\n"+tmplDefaults, 1)
	bad = strings.Replace(bad, "security_boundary: none", "security_boundary: env-beyond", 1)
	bad = strings.Replace(bad, "concepts: [inputs]\n", "", 1)
	bad = strings.Replace(bad, "   (UX-01).", "   and the getting-started page.", 1)
	env := Env{Repo: repo(t, bad)}
	f := demo(1)
	f.Questions = []queue.Question{{ID: "q1", Kind: "blocking", Status: "open"}}
	got := failures(Phase(env, f))
	for _, want := range []string{"section filled: Design", "section filled: Security note",
		"section filled: Defaults and overrides", "missing concepts", "1 open"} {
		if !strings.Contains(got, want) {
			t.Errorf("phase 1 failures %q lack %q", got, want)
		}
	}
	if got := failures(Phase(env, demo(2))); !strings.Contains(got, "task 2") {
		t.Errorf("phase 2 failures %q, want task 2 citing no rule", got)
	}
}

// tmplDefaults is tmpl's Defaults and overrides text.
var tmplDefaults = strings.TrimSpace(strings.SplitN(strings.SplitN(tmpl,
	"## Defaults and overrides\n", 2)[1], "\n## ", 2)[0])

// withoutDefaults is text with its Defaults and overrides section removed.
func withoutDefaults(t *testing.T, text string) string {
	t.Helper()
	start := strings.Index(text, "## Defaults and overrides\n")
	if start < 0 {
		t.Fatal("no Defaults and overrides section")
	}
	end := strings.Index(text[start+3:], "\n## ") + start + 4
	return text[:start] + text[end:]
}

// TestDefaultsSectionOnOlderTemplate: a feature branch cut before the
// template had Defaults and overrides still needs the section at phase 1.
func TestDefaultsSectionOnOlderTemplate(t *testing.T) {
	env := Env{Repo: repoWith(t, withoutDefaults(t, tmpl), withoutDefaults(t, design))}
	got := failures(Phase(env, demo(1)))
	if !strings.Contains(got, "section filled: Defaults and overrides") {
		t.Errorf("phase 1 failures %q lack the Defaults and overrides section", got)
	}
}

func TestMakeSkippedWithoutRun(t *testing.T) {
	env := Env{Repo: repo(t, design)}
	if rs := Phase(env, demo(3)); OK(rs) || !strings.Contains(failures(rs), "skipped") {
		t.Fatalf("phase 3 without -run: %s", failures(rs))
	}
	if rs := Phase(env, demo(5)); OK(rs) {
		t.Fatalf("phase 5 (surfaces [output], no guide change): want fail")
	}
}

func TestPublishArchivesOnTheBranch(t *testing.T) {
	work := repo(t, design)
	env := Env{Repo: work}
	f := demo(6)
	f.PR = &queue.PR{Number: 5, CI: "green"}
	got := failures(Phase(env, f))
	for _, want := range []string{"Consolidation", "design archived", "Archive summary"} {
		if !strings.Contains(got, want) {
			t.Errorf("phase 6 failures %q lack %q", got, want)
		}
	}
	commit(t, work, archived)
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("phase 6: %s", failures(rs))
	}
}

func TestMergedByPRAfterBranchDeleted(t *testing.T) {
	work := repo(t, design)
	env := Env{Repo: work}
	f := demo(7)
	f.Branch, f.PR = "feat/gone", &queue.PR{Number: 5, CI: "green"}
	if rs := Phase(env, f); OK(rs) {
		t.Fatal("merge passed with no merge on main")
	}
	git(t, work, "checkout", "-q", "main")
	git(t, work, "commit", "-q", "--allow-empty", "-m",
		"Merge pull request #5 from o/feat/gone")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "fetch", "-q", "origin")
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("phase 7: %s", failures(rs))
	}
	f.PR.Number = 50
	if rs := Phase(env, f); OK(rs) {
		t.Error("PR #50 matched the merge of #5")
	}
}

func TestDesignReviewRequired(t *testing.T) {
	req := strings.Replace(design, "design_review: skip", "design_review: required", 1)
	work := repo(t, req)
	env := Env{Repo: work}
	f := demo(2)
	f.DesignReview = "required"
	f.Steps = []queue.Step{{ID: "f2-design-review", Phase: 2}}
	if got := failures(Phase(env, f)); !strings.Contains(got, "0 approvals") {
		t.Fatalf("phase 2 without an approval: %q", got)
	}
	if got := failures(Review(env, f)); !strings.Contains(got, "0 approvals") {
		t.Fatalf("the review's own check without an approval: %q", got)
	}
	approved := commit(t, work, req+"\n<!-- as approved -->\n")
	commit(t, work, req+"\n## Design review\n\n- r1: approved at `"+approved+"`.\n")
	if rs := Review(env, f); !OK(rs) {
		t.Errorf("the review's own check, approved: %s", failures(rs))
	}
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("phase 2 approved: %s", failures(rs))
	}
	f.Steps = append(f.Steps, queue.Step{ID: "f2-design-review-r2", Phase: 2})
	if rs := Phase(env, f); OK(rs) {
		t.Error("a second review passed on the first approval")
	}
}

// TestPlanGateGuardsTheApprovedDesign: Plan may not change the design's
// impact fields or rule_changes past its approval; a rule change approved on
// its own line, at a commit that has it, passes (FLOW-19).
func TestPlanGateGuardsTheApprovedDesign(t *testing.T) {
	work := repo(t, ruled)
	env := Env{Repo: work}
	f := demo(2)
	f.DesignReview = "required"
	f.Steps = []queue.Step{{ID: "f2-design-review", Phase: 2, Status: "done"},
		{ID: "f2-plan", Phase: 2, Needs: []string{"f2-design-review"}}}
	approved := commit(t, work, ruled+"\n<!-- as approved -->\n")
	review := "\n## Design review\n\n- r1: approved at `" + approved + "`.\n"
	commit(t, work, ruled+review)
	if rs := Phase(env, f); !OK(rs) {
		t.Fatalf("plan as approved: %s", failures(rs))
	}
	for _, c := range []struct{ from, to, want string }{
		{"seams: []", "seams: [relay-claim]", "seams [] -> [relay-claim]"},
		{"alters_relationships: false", "alters_relationships: true",
			"alters_relationships false -> true"},
		{"security_boundary: none", "security_boundary: env-relay",
			"security_boundary none -> env-relay"},
		{"native_validation: []",
			"native_validation:\n  - {host: macos-vz, blocking: true, reason: r}",
			"blocking hosts [] -> [macos-vz]"},
		{"  - {rule: TEST-09, change: add, paths: [docs/development/testing.md]}",
			"  - {rule: TEST-09, change: add, paths: [docs/development/global.md]}",
			"rule_changes TEST-09 added or changed"},
		{"rule_changes:\n  - {rule: TEST-09, change: add, paths: " +
			"[docs/development/testing.md]}\n", "", "rule_changes TEST-09 dropped"},
	} {
		commit(t, work, strings.Replace(ruled, c.from, c.to, 1)+review)
		if got := failures(Phase(env, f)); !strings.Contains(got, c.want) {
			t.Errorf("%s: failures %q lack %q", c.to, got, c.want)
		}
	}
	late := strings.Replace(ruled, "rule_changes:\n", "rule_changes:\n"+
		"  - {rule: TEST-10, change: add, paths: [docs/development/global.md]}\n", 1)
	commit(t, work, late+review)
	if got := failures(Phase(env, f)); !strings.Contains(got,
		"rule_changes TEST-10 added or changed") {
		t.Errorf("a rule change added after the approval: %q", got)
	}
	commit(t, work, late+review+"- r1 addendum: rule change TEST-10 approved at `"+
		approved+"`.\n")
	if got := failures(Phase(env, f)); !strings.Contains(got, "TEST-10") {
		t.Errorf("an own approval at a commit without the entry passed: %q", got)
	}
	added := commit(t, work, late+review)
	commit(t, work, late+review+"- r1 addendum: rule change TEST-10 approved at `"+
		added+"`.\n")
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("a rule change with its own approval: %s", failures(rs))
	}
	// A re-review approved a changed impact field: the guard compares
	// against the last approval, not the first.
	seams := strings.Replace(ruled, "seams: []", "seams: [relay-claim]", 1)
	second := commit(t, work, seams+review)
	commit(t, work, seams+review+"- r2: approved at `"+second+"`.\n")
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("a design as its last approval left it: %s", failures(rs))
	}
}

func TestSkipCriteria(t *testing.T) {
	seams := strings.Replace(design, "seams: []", "seams: [relay-claim]", 1)
	env := Env{Repo: repo(t, seams)}
	if got := failures(Phase(env, demo(1))); !strings.Contains(got, "seams") {
		t.Errorf("skip with seams: %q", got)
	}
	env = Env{Repo: repo(t, design)}
	f := demo(1)
	f.Questions = []queue.Question{{ID: "a1", Kind: "assumed", Status: "open"}}
	if got := failures(Phase(env, f)); !strings.Contains(got, "a1") {
		t.Errorf("skip with an assumption: %q", got)
	}
	f.DesignReview = "required"
	if got := failures(Phase(env, f)); !strings.Contains(got, "recorded in queue") {
		t.Errorf("queue and design disagree: %q", got)
	}
}

// TestReReviewJudgesTheDeltaFromTheApproval: a design that needed review,
// back in Design, may skip the re-review when it barely changed since its
// approval, and may not when its impact changed.
func TestReReviewJudgesTheDeltaFromTheApproval(t *testing.T) {
	req := strings.Replace(design, "seams: []", "seams: [relay-claim]", 1)
	req = strings.Replace(req, "design_review: skip", "design_review: required", 1)
	work := repo(t, req)
	env := Env{Repo: work}
	approved := commit(t, work, req+"\n<!-- reviewed -->\n")
	review := "\n## Design review\n\n- r1: approved at `" + approved + "`.\n"
	skip := strings.Replace(req, "design_review: required", "design_review: skip", 1)
	commit(t, work, strings.Replace(skip, "Write inputs/.", "Write inputs/ first.", 1)+review)
	f := demo(1)
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("minimal change after approval: %s", failures(rs))
	}
	commit(t, work, strings.Replace(skip, "security_boundary: none",
		"security_boundary: env-relay", 1)+"\n## Security note\n\nNew.\n"+review)
	got := failures(Phase(env, f))
	if !strings.Contains(got, "security_boundary none -> env-relay") {
		t.Errorf("impact change after approval: %q", got)
	}
}

func TestProcessChecks(t *testing.T) {
	work := repo(t, design+"\n## Design review\n\n- r1: approved at `abc1234`.\n")
	env := Env{Repo: work}
	f := demo(1)
	f.Kind, f.Round = "process", "process"
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("x1: %s", failures(rs))
	}
	f.Phase, f.PR = 2, &queue.PR{Number: 5, CI: "green"}
	if got := failures(Phase(env, f)); !strings.Contains(got, "design archived") {
		t.Errorf("x2 before the archive: %q", got)
	}
	commit(t, work, archived)
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("x2: %s", failures(rs))
	}
	f.Phase = 3
	if rs := Phase(env, f); OK(rs) || !strings.Contains(failures(rs), "merged") {
		t.Errorf("x3 before merge: %s, want the merge check failing", failures(rs))
	}
	commit(t, work, withoutDefaults(t, design)+"\n## Design review\n\n- r1: approved at `abc1234`.\n")
	f.Phase, f.PR = 1, nil
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("x1 without Defaults and overrides: %s", failures(rs))
	}
	f.Design = "docs/design/missing.md"
	if rs := Phase(env, f); OK(rs) {
		t.Error("x1 passed without a design file")
	}
}

// TestArchivedDesignNeedsNoDesignReview: design_review decides a gate before
// implementation, so a design archived without it, as every design archived
// before FLOW-19 is, still passes; a draft without it does not.
func TestArchivedDesignNeedsNoDesignReview(t *testing.T) {
	for status, wantMissing := range map[string]bool{"archived": false, "draft": true} {
		text := strings.Replace(design, "design_review: skip\n", "", 1)
		text = strings.Replace(text, "status: draft", "status: "+status, 1)
		d, err := ParseDesign(text, tmpl)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(d.Front.Problems("demo"), "; ")
		if strings.Contains(got, "missing design_review") != wantMissing {
			t.Errorf("status %s: problems %q", status, got)
		}
	}
}

// TestApprovalMayWrap: a Design review entry's "approved at <sha>" may break
// across lines, as Markdown reflow leaves it (FLOW-19).
func TestApprovalMayWrap(t *testing.T) {
	entry := "\n## Design review\n\n- r1: approved\n  at `abc1234`.\n"
	d, err := ParseDesign(design+entry, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if got := approvals(d); len(got) != 1 || got[0] != "abc1234" {
		t.Fatalf("approvals = %v", got)
	}
}

// TestPublishPassesOnAnEarlyMerge: the human merged the PR before Publish's
// step was recorded, and GitHub deleted the branch. Publish then passes on the
// merge plus the design archived on main; before the merge it still needs the
// push, main and a green PR (FLOW-08, FLOW-12).
func TestPublishPassesOnAnEarlyMerge(t *testing.T) {
	work := repo(t, design)
	commit(t, work, archived)
	env := Env{Repo: work}
	f := demo(6)
	f.PR = &queue.PR{Number: 7, CI: "pending"}
	if got := failures(Phase(env, f)); !strings.Contains(got, "PR open with CI green") {
		t.Fatalf("phase 6 before the merge: %q, want the PR check", got)
	}
	git(t, work, "checkout", "-q", "main")
	git(t, work, "merge", "-q", "--no-ff", "-m",
		"Merge pull request #7 from o/feat/demo", "feat/demo")
	git(t, work, "push", "-q", "origin", "main", ":feat/demo")
	git(t, work, "branch", "-q", "-D", "feat/demo")
	git(t, work, "fetch", "-q", "--prune", "origin")
	if rs := Phase(env, f); !OK(rs) {
		t.Errorf("phase 6 after an early merge: %s", failures(rs))
	}
	p := demo(2)
	p.Kind, p.Round, p.PR = "process", "process", &queue.PR{Number: 7}
	if rs := Phase(env, p); !OK(rs) {
		t.Errorf("x2 after an early merge: %s", failures(rs))
	}
}

// squash squash-merges feat/demo into main as GitHub does, with the PR number
// in the subject, and pushes main.
func squash(t *testing.T, work string, pr string) {
	t.Helper()
	git(t, work, "checkout", "-q", "main")
	git(t, work, "merge", "-q", "--squash", "feat/demo")
	git(t, work, "commit", "-q", "-m", "Demo ("+pr+")")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "fetch", "-q", "origin")
}

// TestMergedBySquash: a squash merge never makes the branch head an ancestor
// of main, so it counts as merged only when main carries a commit naming the
// PR whose tree is the head's tree (FLOW-08, FLOW-16).
func TestMergedBySquash(t *testing.T) {
	work := repo(t, design)
	env := Env{Repo: work}
	f := demo(7)
	f.PR = &queue.PR{Number: 9, CI: "green"}
	squash(t, work, "#9")
	if rs := Phase(env, f); !OK(rs) || !strings.Contains(rs[0].Info, "squash-merged") {
		t.Fatalf("squash with the local and remote refs present: %s", failures(rs))
	}
	f.PR.Number = 90
	if rs := Phase(env, f); OK(rs) {
		t.Error("PR #90 matched the squash of #9")
	}
	f.PR = nil
	if rs := Phase(env, f); OK(rs) {
		t.Error("a squashed branch passed with no PR recorded")
	}
	f.PR = &queue.PR{Number: 9, CI: "green"}
	git(t, work, "checkout", "-q", "feat/demo")
	_ = os.WriteFile(filepath.Join(work, "late.txt"), []byte("late\n"), 0o644)
	git(t, work, "add", "late.txt")
	git(t, work, "commit", "-q", "-m", "pushed after the squash")
	if got := failures(Phase(env, f)); !strings.Contains(got, "refs/heads/feat/demo") ||
		!strings.Contains(got, "no PR commit has its tree") {
		t.Errorf("local head past the squash: %q, want it named as unmerged", got)
	}
	git(t, work, "push", "-q", "origin", "feat/demo")
	git(t, work, "fetch", "-q", "origin")
	git(t, work, "checkout", "-q", "main")
	git(t, work, "branch", "-q", "-D", "feat/demo")
	got := failures(Phase(env, f))
	if !strings.Contains(got, "refs/remotes/origin/feat/demo") {
		t.Errorf("remote head past the squash: %q, want it named as unmerged", got)
	}
}

// TestSquashOfAnOlderHeadFails: main moved under the branch between its last
// main-merge and the squash, so the squash's tree is not the branch's and the
// check names both.
func TestSquashOfAnOlderHeadFails(t *testing.T) {
	work := repo(t, design)
	env := Env{Repo: work}
	git(t, work, "checkout", "-q", "main")
	_ = os.WriteFile(filepath.Join(work, "other.txt"), []byte("other\n"), 0o644)
	git(t, work, "add", "other.txt")
	git(t, work, "commit", "-q", "-m", "another feature")
	squash(t, work, "#9")
	p := demo(3)
	p.Kind, p.Round, p.PR = "process", "process", &queue.PR{Number: 9}
	got := failures(Phase(env, p))
	if !strings.Contains(got, "is not contained in origin/main") ||
		strings.Count(got, "tree ") < 2 {
		t.Fatalf("x3 on a squash with a different tree: %q, want both trees named", got)
	}
}

// TestCloseAfterASquash: once the squashed local branch is deleted (with -D,
// FLOW-16) and GitHub has deleted the remote one, Close passes on the PR
// commit alone.
func TestCloseAfterASquash(t *testing.T) {
	work := repo(t, design)
	commit(t, work, archived)
	env := Env{Repo: work}
	p := demo(4)
	p.Kind, p.Round, p.PR = "process", "process", &queue.PR{Number: 9}
	squash(t, work, "#9")
	if got := failures(Phase(env, p)); !strings.Contains(got, "branch deleted") ||
		strings.Contains(got, "merged by a human") {
		t.Fatalf("x4 before the branches go: %q, want only the deletions failing", got)
	}
	git(t, work, "push", "-q", "origin", ":feat/demo")
	git(t, work, "branch", "-q", "-D", "feat/demo")
	git(t, work, "fetch", "-q", "--prune", "origin")
	if rs := Phase(env, p); !OK(rs) {
		t.Errorf("x4 after a squash: %s", failures(rs))
	}
}

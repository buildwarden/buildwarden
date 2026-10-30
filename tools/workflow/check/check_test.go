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

## Implementation plan

Ordered tasks, each citing the rules it must satisfy.
`

const design = `---
feature: demo
status: draft
round: parallel
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

// repo returns a checkout whose branch feat/demo carries the design and is
// pushed to a bare origin.
func repo(t *testing.T, designText string) string {
	t.Helper()
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, root, "clone", "-q", origin, work)
	git(t, work, "checkout", "-q", "-b", "main")
	_ = os.MkdirAll(filepath.Join(work, "docs", "design"), 0o755)
	_ = os.WriteFile(filepath.Join(work, "docs", "design", "TEMPLATE.md"), []byte(tmpl), 0o644)
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
	return &queue.Feature{Feature: "demo", Round: "parallel", Phase: phase,
		Branch: "feat/demo",
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
	for _, phase := range []int{0, 1, 2, 6} {
		if rs := Phase(env, demo(phase)); !OK(rs) {
			t.Errorf("phase %d: %s", phase, failures(rs))
		}
	}
}

func TestDesignFailures(t *testing.T) {
	bad := strings.Replace(design, "Write inputs/.",
		"The behavior and structure of the change.", 1)
	bad = strings.Replace(bad, "security_boundary: none", "security_boundary: env-beyond", 1)
	bad = strings.Replace(bad, "concepts: [inputs]\n", "", 1)
	bad = strings.Replace(bad, "   (UX-01).", "   and the getting-started page.", 1)
	env := Env{Repo: repo(t, bad)}
	f := demo(1)
	f.Questions = []queue.Question{{ID: "q1", Kind: "blocking", Status: "open"}}
	got := failures(Phase(env, f))
	for _, want := range []string{"section filled: Design", "section filled: Security note",
		"missing concepts", "1 open"} {
		if !strings.Contains(got, want) {
			t.Errorf("phase 1 failures %q lack %q", got, want)
		}
	}
	if got := failures(Phase(env, demo(2))); !strings.Contains(got, "task 2") {
		t.Errorf("phase 2 failures %q, want task 2 citing no rule", got)
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

func TestProcessChecks(t *testing.T) {
	env := Env{Repo: repo(t, design)}
	f := demo(1)
	f.Kind, f.Round = "process", "process"
	for _, phase := range []int{1, 2} {
		f.Phase = phase
		if rs := Phase(env, f); !OK(rs) {
			t.Errorf("x%d: %s", phase, failures(rs))
		}
	}
	f.Phase = 3
	if rs := Phase(env, f); OK(rs) || !strings.Contains(failures(rs), "merged") {
		t.Errorf("x3 before merge: %s, want the merge check failing", failures(rs))
	}
	f.Phase, f.Design = 1, "docs/design/missing.md"
	if rs := Phase(env, f); OK(rs) {
		t.Error("x1 passed without a design file")
	}
}

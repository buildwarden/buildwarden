package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// TestTestLinesAreApprovals: an umbrella test's line is an approval for any
// item, and a process test's only for a process item (FLOW-19, FLOW-20).
func TestTestLinesAreApprovals(t *testing.T) {
	review := "- r1, 2026-11-22: umbrella test passed at `aaa1111` (FLOW-20): U1 ...\n" +
		"- r2, 2026-11-23: process test passed at `bbb2222` (FLOW-20): P1 ...\n"
	d, err := ParseDesign(design+"\n## Design review\n\n"+review, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if got := approvals(d, false); !slices.Equal(got, []string{"aaa1111"}) {
		t.Errorf("a feature's approvals = %v", got)
	}
	if got := approvals(d, true); !slices.Equal(got, []string{"aaa1111", "bbb2222"}) {
		t.Errorf("a process item's approvals = %v", got)
	}
}

// TestUmbrellaPlanGate: the Plan gate takes an umbrella test's pass as the
// approval it compares with, and fails on an impact field changed after it
// (FLOW-19, FLOW-20).
func TestUmbrellaPlanGate(t *testing.T) {
	work := repo(t, ruled)
	env := here(work)
	env.State.Config.Repositories[0].ProgramTrunk = "program/componentization"
	f := demo(2)
	f.Program, f.DesignReview, f.RulesChanged = true, "umbrella", []string{"TEST-09"}
	f.Steps = []queue.Step{{ID: "f2-umbrella", Phase: 2, Status: "done"},
		{ID: "f2-plan", Phase: 2, Needs: []string{"f2-umbrella"}}}
	umbrella := strings.Replace(ruled, "design_review: required",
		"design_review: umbrella", 1)
	umbrella = strings.Replace(umbrella, "concepts: [inputs]\n",
		"concepts: [inputs]\nrelated: [componentization.md]\n", 1)
	passed := commit(t, work, umbrella)
	review := "\n## Design review\n\n- r1, 2026-11-22: umbrella test passed at `" + passed +
		"` (FLOW-20).\n"
	commit(t, work, umbrella+review)
	if rs := Phase(env, f); !OK(rs) {
		t.Fatalf("plan as passed: %s", failures(rs))
	}
	commit(t, work, strings.Replace(umbrella, "seams: []", "seams: [relay-claim]", 1)+review)
	if got := failures(Phase(env, f)); !strings.Contains(got, "seams [] -> [relay-claim]") {
		t.Fatalf("a seam added after the pass: %q", got)
	}
}

// TestProcessTestDelta: x2-implement's done-when compares the design with
// its process test's pass and fails on rule_changes changed after it; Design
// is done once it records process-test or required (FLOW-03, FLOW-20).
func TestProcessTestDelta(t *testing.T) {
	work := repo(t, ruled)
	env := here(work)
	env.State.Config.Repositories[0].ProgramTrunk = "program/componentization"
	p := demo(1)
	p.Kind, p.Round = "process", "process"
	tested := strings.Replace(ruled, "design_review: required",
		"design_review: process-test", 1)
	passed := commit(t, work, tested)
	got := failures(Phase(env, p))
	if !strings.Contains(got, "design_review recorded in queue") {
		t.Fatalf("Design with the queue unrecorded: %q", got)
	}
	p.DesignReview = "process-test"
	if rs := Phase(env, p); !OK(rs) {
		t.Fatalf("Design with process-test recorded: %s", failures(rs))
	}
	review := "\n## Design review\n\n- r1, 2026-11-22: process test passed at `" + passed +
		"` (FLOW-20).\n"
	commit(t, work, strings.Replace(tested, "paths: [docs/development/testing.md]",
		"paths: [docs/development/global.md]", 1)+review)
	p.Phase = 2
	if got := failures(Phase(env, p)); !strings.Contains(got,
		"rule_changes TEST-09 added or changed") {
		t.Fatalf("x2-implement after rule_changes changed: %q", got)
	}
}

// TestProcessTestAfterReturn: after a return to Design, a process item's
// new process test needs a pass of its own: the first one's line does not
// finish x2-process-test-r2 (FLOW-19, FLOW-20).
func TestProcessTestAfterReturn(t *testing.T) {
	work := repo(t, ruled)
	env := here(work)
	env.State.Config.Repositories[0].ProgramTrunk = "program/componentization"
	p := demo(2)
	p.Kind, p.Round, p.DesignReview = "process", "process", "process-test"
	p.Steps = []queue.Step{{ID: "x2-process-test", Phase: 2, Status: "done"},
		{ID: "x2-process-test-r2", Phase: 2, Status: "pending"}}
	tested := strings.Replace(ruled, "design_review: required",
		"design_review: process-test", 1)
	first := commit(t, work, tested)
	review := "\n## Design review\n\n- r1, 2026-11-22: process test passed at `" + first +
		"` (FLOW-20).\n"
	second := commit(t, work, tested+review)
	if rs := Review(env, p); OK(rs) {
		t.Fatal("x2-process-test-r2 finished on the first pass alone")
	}
	review += "- r2, 2026-11-23: process test passed at `" + second + "` (FLOW-20).\n"
	commit(t, work, tested+review)
	if rs := Review(env, p); !OK(rs) {
		t.Fatalf("x2-process-test-r2 with its own pass: %s", failures(rs))
	}
}

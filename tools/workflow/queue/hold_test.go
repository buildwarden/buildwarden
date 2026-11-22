package queue

import (
	"slices"
	"strings"
	"testing"
)

// TestHoldInProgress: an item in progress that already has a question is put
// on hold with the next free q<N>; every pending gate of its phase needs the
// answer step, and the cloud is offered none of its steps (FLOW-20).
func TestHoldInProgress(t *testing.T) {
	f := toPhase("f", 1, 2)
	f.Questions = []Question{{ID: "q1", Kind: "assumed", Status: "confirmed",
		Question: "?", Options: []string{"a"}, Recommended: "a"}}
	if err := f.PutOnHold("payoff not shown", "program-end", now, "cloud/run"); err != nil {
		t.Fatal(err)
	}
	q := f.Hold()
	if q == nil || q.ID != "q2" || q.Resumes != "program-end" {
		t.Fatalf("hold = %+v, want q2 resuming at program-end", q)
	}
	plan, err := f.Step("f2-plan")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Needs, "f2-answer-q2") {
		t.Fatalf("f2-plan needs %v, want the answer step", plan.Needs)
	}
	s := state(f)
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("cloud next = %s on a held item", got)
	}
	if got := next(t, s, "human/jeff"); got != "f f2-answer-q2" {
		t.Fatalf("human next = %s, want the answer step", got)
	}
}

// TestHeldCountsTowardNoLimit: a held item in progress is no holder of the
// in-progress limit and does not wait on the human; QUEUE.md lists it once,
// under Held, and not with the questions (FLOW-05, FLOW-20).
func TestHeldCountsTowardNoLimit(t *testing.T) {
	h := toPhase("h", 1, 1)
	h.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open",
		Question: "held for the program", Options: []string{"resume it"},
		Recommended: "resume it", Resumes: "program-end"}}
	q := feature("q", 2, "queued")
	s := state(h, q)
	inProgress(s, 1)
	if got := next(t, s, "cloud/run"); got != "q f0-branch-setup" {
		t.Fatalf("cloud next = %s; the held item must not fill the in-progress limit", got)
	}
	if w := s.WaitsOnHuman(); len(w) != 0 {
		t.Fatalf("waits on the human: %v; a held item does not", w[0].Feature)
	}
	md := Summary(s)
	held := md[strings.Index(md, "### Held"):]
	if !strings.Contains(held, "**h** (in progress, q1): held for the program; "+
		"resumes: program-end") {
		t.Fatalf("Held does not list h:\n%s", md)
	}
	if strings.Count(md, "held for the program") != 1 {
		t.Fatalf("the hold is listed more than once:\n%s", md)
	}

	// Answered, the item counts again.
	h.Questions[0].Status = "answered"
	if got := next(t, s, "cloud/run"); got != "h f1-design" {
		t.Fatalf("cloud next = %s once the hold is answered", got)
	}
}

// TestProcessHoldsOnlyFromImplement: while a program trunk is configured, a
// process item in Design, at its process test, or on hold holds no worker
// its holds name; with none configured, only one on hold is spared (FLOW-20).
func TestProcessHoldsOnlyFromImplement(t *testing.T) {
	a := feature("a", 9, "in_progress")
	design := process("design", 1, "in_progress", "cloud")
	tested := implementing("tested", 2, "cloud")
	tested.Steps = append(tested.Steps, Step{ID: "x2-process-test", Phase: 2,
		Affinity: "cloud", Status: "pending"})
	onHold := implementing("onhold", 3, "cloud")
	onHold.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open",
		Question: "payoff", Options: []string{"resume it"}, Recommended: "resume it",
		Resumes: "program-end"}}
	for _, p := range []*Feature{design, tested, onHold} {
		if h := programState(t, a, p).Hold(a, "cloud/run"); h != "" {
			t.Errorf("%s holds the cloud", h)
		}
	}
	// No program trunk: the loop is as before the program, and a process
	// item holds from its Design on.
	for _, p := range []*Feature{design, tested} {
		if h := mapped(t, a, p).Hold(a, "cloud/run"); h != p.Feature {
			t.Errorf("no program trunk: %s does not hold the cloud (%q)", p.Feature, h)
		}
	}
	if h := mapped(t, a, onHold).Hold(a, "cloud/run"); h != "" {
		t.Errorf("no program trunk: %s on hold holds the cloud", h)
	}
	if h := state(a, implementing("impl", 1, "cloud")).Hold(a, "cloud/run"); h != "impl" {
		t.Errorf("an implementing process item does not hold: %q", h)
	}
}

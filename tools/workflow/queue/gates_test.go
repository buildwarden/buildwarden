package queue

import (
	"slices"
	"strings"
	"testing"
)

// processState is a state with a program trunk configured, and a process
// item queued in it through Add.
func processState(t *testing.T) (*State, *Feature) {
	t.Helper()
	s := programState(t)
	f := &Feature{Feature: "rules", Priority: 1, Kind: "process",
		Budget: Budget{Estimate: Count{Runs: 1, Steps: 4}}}
	if err := s.Add(f, now, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	return s, f
}

// TestProcessTestSeeding: while a program trunk is configured a process
// item's x1-design is cloud, and a human cannot take it; phase 2 seeds
// x2-process-test or x2-design-review by what Design recorded, and
// x2-implement waits for it. With no program trunk, the template seeds as
// before (FLOW-03, FLOW-20).
func TestProcessTestSeeding(t *testing.T) {
	s, f := processState(t)
	if f.Steps[0].ID != "x1-design" || f.Steps[0].Affinity != "cloud" {
		t.Fatalf("first step %+v, want a cloud x1-design", f.Steps[0])
	}
	if got := next(t, s, "human/jeff"); got != "none" {
		t.Fatalf("human next = %s", got)
	}
	if got := next(t, s, "cloud/run"); got != "rules x1-design" {
		t.Fatalf("cloud next = %s", got)
	}
	for review, want := range map[string]string{
		"process-test": "x2-process-test cloud",
		"required":     "x2-design-review human",
	} {
		_, f := processState(t)
		f.Status, f.DesignReview = "in_progress", review
		Complete(f, &f.Steps[0], "cloud/run", now)
		st := f.Steps[1]
		if got := st.ID + " " + st.Affinity; got != want {
			t.Errorf("%s: phase 2 opens with %s, want %s", review, got, want)
		}
		impl, err := f.Step("x2-implement")
		if err != nil || !slices.Equal(impl.Needs, []string{st.ID}) {
			t.Errorf("%s: x2-implement %+v, %v; want it to need %s", review, impl, err,
				st.ID)
		}
		if errs := state(f).Validate(); review == "process-test" && len(errs) == 0 {
			t.Errorf("process-test validated with no program trunk")
		}
	}
	plain := process("plain", 1, "in_progress")
	if plain.Steps[0].Affinity != "human" {
		t.Fatalf("with no program trunk x1-design is %s", plain.Steps[0].Affinity)
	}
	Complete(plain, &plain.Steps[0], "human/jeff", now)
	if got := stepIDs(plain, "pending"); got != "x2-implement" {
		t.Fatalf("phase 2 with no FLOW-20 review = %q", got)
	}
}

// TestUmbrellaSeeding: a program feature whose Design recorded umbrella gets
// the cloud step f2-umbrella in place of the human's design review, and Plan
// waits for it; ToHumanReview turns a pending one into the human's review
// (FLOW-19, FLOW-20).
func TestUmbrellaSeeding(t *testing.T) {
	f := programItem("p", 1, "relay")
	f.DesignReview = "umbrella"
	Complete(f, &f.Steps[len(f.Steps)-1], "cloud/run", now)
	if got := stepIDs(f, "pending"); got != "f2-umbrella f2-plan" {
		t.Fatalf("phase 2 = %q", got)
	}
	um, _ := f.Step("f2-umbrella")
	if um.Affinity != "cloud" {
		t.Fatalf("f2-umbrella is %s", um.Affinity)
	}
	s := programState(t, f)
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	f.Program = false
	if errs := s.Validate(); len(errs) == 0 {
		t.Fatal("umbrella validated on a feature that is not a program item")
	}
	f.Program = true
	if err := ToHumanReview(f, "U4: a posture change", "cloud/run", now); err != nil {
		t.Fatal(err)
	}
	plan, _ := f.Step("f2-plan")
	if f.DesignReview != "required" ||
		!slices.Equal(plan.Needs, []string{"f2-design-review"}) ||
		!f.hasStep("f2-design-review") || f.hasStep("f2-umbrella") {
		t.Fatalf("after ToHumanReview: %s, %+v", f.DesignReview, f.Steps)
	}
}

// TestProgramReturns: a program item's third and fourth returns to Design
// loop again; its fifth escalates; a feature's third still does (FLOW-06,
// FLOW-20).
func TestProgramReturns(t *testing.T) {
	escalated := func(f *Feature) bool {
		return slices.ContainsFunc(f.Steps, func(st Step) bool {
			return strings.Contains(st.ID, "-escalate-") && st.Status == "pending"
		})
	}
	p := programItem("p", 1, "relay")
	for n := 1; n <= 5; n++ {
		Back(p, "cloud/run", now, "finding", 1)
		if escalated(p) != (n == 5) {
			t.Fatalf("program item, return %d: escalated %v", n, escalated(p))
		}
	}
	f := designing("f", 1, "relay")
	for n := 1; n <= 3; n++ {
		Back(f, "cloud/run", now, "finding", 1)
	}
	if !escalated(f) {
		t.Fatal("a feature's third return did not escalate")
	}
}

// TestProgramRunaway: a program item's first runaway waits on no one and
// is answered by the cloud's reestimate, recorded on the item; its second
// is a runaway that waits on the human, and the cloud may not re-estimate it
// again (FLOW-13, FLOW-20).
func TestProgramRunaway(t *testing.T) {
	p := toPhase("p", 1, 3)
	p.Program = true
	p.Budget.Spent.Steps = 14
	s := programState(t, p)
	if s.Runaway(p) || !s.NeedsReestimate(p) || len(s.WaitsOnHuman()) != 0 {
		t.Fatalf("first runaway: runaway %v, needs %v", s.Runaway(p), s.NeedsReestimate(p))
	}
	if got := next(t, s, "cloud/run"); got != "p f3-implement" {
		t.Fatalf("cloud next = %s: the first runaway holds no claim", got)
	}
	if err := s.Reestimate(p, "cloud/run", now); err != nil {
		t.Fatal(err)
	}
	if p.Budget.Estimate.Steps != 14 || p.Reestimates() != 1 {
		t.Fatalf("estimate %d, re-estimates %d", p.Budget.Estimate.Steps, p.Reestimates())
	}
	p.Budget.Spent.Steps = 28
	if !s.Runaway(p) || s.NeedsReestimate(p) || len(s.WaitsOnHuman()) != 1 {
		t.Fatal("the second runaway does not escalate")
	}
	if err := s.Reestimate(p, "cloud/run", now); err == nil {
		t.Fatal("a second re-estimate by the cloud")
	}
	if err := s.Reestimate(p, "human/jeff", now); err != nil {
		t.Fatalf("the human's re-estimate: %v", err)
	}
	f := toPhase("f", 2, 3)
	f.Budget.Spent.Steps = 14
	s = programState(t, f)
	if !s.Runaway(f) || s.Reestimate(f, "cloud/run", now) == nil {
		t.Fatal("a feature's runaway is not the human's")
	}
}

// TestFailedP1Holds: a process item at its process test that already has a
// question is held with the next free q<N>, resuming at the program's end,
// and x2-implement needs the answer (FLOW-20).
func TestFailedP1Holds(t *testing.T) {
	_, f := processState(t)
	f.Status, f.DesignReview = "in_progress", "process-test"
	f.Questions = []Question{{ID: "q1", Kind: "assumed", Status: "confirmed",
		Question: "?", Options: []string{"a"}, Recommended: "a"}}
	Complete(f, &f.Steps[0], "cloud/run", now)
	if err := f.PutOnHold("P1: no payoff shown", "program-end", now,
		"cloud/run"); err != nil {
		t.Fatal(err)
	}
	impl, _ := f.Step("x2-implement")
	q := f.Hold()
	if q == nil || q.ID != "q2" || !slices.Contains(impl.Needs, "x2-answer-q2") {
		t.Fatalf("hold %+v, x2-implement needs %v", q, impl.Needs)
	}
}

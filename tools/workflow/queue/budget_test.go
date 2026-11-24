package queue

import "testing"

// TestBudgeted: only a step before the item's Merge phase counts, other
// than an f6-merged-<n> and a refresh's first claim; a feature's merge and
// Close, and a process item's, do not (FLOW-13).
func TestBudgeted(t *testing.T) {
	f := feature("f", 1, "in_progress")
	p := process("p", 2, "in_progress")
	for _, c := range []struct {
		f    *Feature
		st   Step
		want bool
	}{
		{f, Step{ID: "f3-implement", Phase: 3}, true},
		{f, Step{ID: "f6-open-pr", Phase: 6}, true},
		{f, Step{ID: "f6-merged-1", Phase: 6}, false},
		{f, Step{ID: "f6-refresh", Phase: 6}, false},
		{f, Step{ID: "f7-merge", Phase: 7}, false},
		{f, Step{ID: "f8-close", Phase: 8}, false},
		{f, Step{ID: "f8-escalate-branch", Phase: 8}, false},
		{p, Step{ID: "x2-implement", Phase: 2}, true},
		{p, Step{ID: "x3-merge", Phase: 3}, false},
		{p, Step{ID: "x4-state", Phase: 4}, false},
		{p, Step{ID: "x4-close", Phase: 4}, false},
	} {
		if got := Budgeted(c.f, &c.st); got != c.want {
			t.Errorf("Budgeted(%s, %s) = %v, want %v", c.f.Feature, c.st.ID, got, c.want)
		}
	}
	f.Log(now, "cloud/run", "claimed f6-refresh until %s", now.Format("2006"))
	if !Budgeted(f, &Step{ID: "f6-refresh", Phase: 6}) {
		t.Error("a refresh claimed again does not count")
	}
}

// TestRunawayHoldsNothingFromMerge: an item past its runaway budget whose
// only pending step is Close is no runaway: Close is offered, its claim
// counts nothing, and the item does not wait on the human; before Merge the
// same spend holds it (FLOW-13).
func TestRunawayHoldsNothingFromMerge(t *testing.T) {
	f := toPhase("f", 1, 8)
	f.Budget.Spent.Steps = 16
	s := state(f)
	if s.Runaway(f) || len(s.WaitsOnHuman()) != 0 {
		t.Fatalf("runaway %v, waiting %d: Close is held", s.Runaway(f),
			len(s.WaitsOnHuman()))
	}
	if got := next(t, s, "cloud/run"); got != "f f8-close" {
		t.Fatalf("next = %s, want the runaway item's Close", got)
	}
	st, _ := f.Step("f8-close")
	s.Claim(f, st, "cloud/run", now)
	if f.Budget.Spent.Steps != 16 {
		t.Fatalf("Close counted: %d steps spent", f.Budget.Spent.Steps)
	}

	g := toPhase("g", 2, 3)
	g.Budget.Spent.Steps = 16
	s = state(g)
	if !s.Runaway(g) || next(t, s, "cloud/run") != "none" {
		t.Fatal("a runaway before Merge is not held")
	}
	st, _ = g.Step("f3-implement")
	s.Claim(g, st, "human/jeff", now)
	if g.Budget.Spent.Steps != 17 {
		t.Fatalf("Implement not counted: %d steps spent", g.Budget.Spent.Steps)
	}
}

// TestProgramRunawayAtMerge: a program item past its budget with only its
// merge pending needs no re-estimate (FLOW-13, FLOW-20).
func TestProgramRunawayAtMerge(t *testing.T) {
	p := toPhase("p", 1, 7)
	p.Program = true
	p.Budget.Spent.Steps = 16
	s := programState(t, p)
	if s.NeedsReestimate(p) || s.Runaway(p) {
		t.Fatalf("needs %v, runaway %v at the merge step", s.NeedsReestimate(p),
			s.Runaway(p))
	}
}

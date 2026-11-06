package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)

func feature(name string, prio int, status string) *Feature {
	f := &Feature{Feature: name, Priority: prio, Status: status, Round: "feature",
		Budget: Budget{Estimate: Count{Runs: 1, Steps: 7}}}
	Seed(f)
	return f
}

func state(fs ...*Feature) *State {
	s := &State{Features: fs}
	s.Config.RunBudget.Steps = 12
	s.Config.Limits.InProgress = 5
	s.Config.Limits.AltersRelationships = 1
	return s
}

func next(t *testing.T, s *State, worker string) string {
	t.Helper()
	p, _ := s.Next(worker, now)
	if p == nil {
		return "none"
	}
	return p.Feature.Feature + " " + p.Step.ID
}

func TestNextIsDepthFirst(t *testing.T) {
	a, b := feature("a", 1, "in_progress"), feature("b", 2, "in_progress")
	s := state(a, b)
	if got := next(t, s, "cloud"); got != "a f0-branch-setup" {
		t.Fatalf("next = %q, want the top-priority feature", got)
	}
	// a blocks on a human step: b advances, and a human worker sees a's step.
	a.Steps[0].Affinity = "human"
	if got := next(t, s, "cloud"); got != "b f0-branch-setup" {
		t.Fatalf("next = %q, want b while a waits on a human", got)
	}
	if got := next(t, s, "human"); got != "a f0-branch-setup" {
		t.Fatalf("human next = %q", got)
	}
	if got := next(t, s, "host:macos-vz"); got != "none" {
		t.Fatalf("host next = %q", got)
	}
	q := feature("q", 3, "queued")
	q.Steps[0].Affinity = "host:macos-vz"
	s.Features = append(s.Features, q)
	if got := next(t, s, "host:macos-vz"); got != "q f0-branch-setup" {
		t.Fatalf("host next = %q, want the queued feature's host step", got)
	}
	s.Claim(q, &q.Steps[0], "host:macos-vz", now, true)
	if q.Status != "queued" {
		t.Fatal("a host claim started a queued feature")
	}
}

func TestNextStartsFeatureOnlyWhenAllBlocked(t *testing.T) {
	a, q := feature("a", 1, "in_progress"), feature("q", 2, "queued")
	s := state(a, q)
	if got := next(t, s, "cloud"); got != "a f0-branch-setup" {
		t.Fatalf("next = %q", got)
	}
	a.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open"}}
	p, blocked := s.Next("cloud", now)
	if p == nil || p.Feature != q || !p.Starts {
		t.Fatalf("pick = %+v, want q started", p)
	}
	if len(blocked) != 1 || !strings.Contains(blocked[0].Reason, "q1") {
		t.Fatalf("blocked = %+v", blocked)
	}
	s.Config.Limits.InProgress = 1
	if got := next(t, s, "cloud"); got != "none" {
		t.Fatalf("next = %q, want none at the in-progress limit", got)
	}
}

func TestNextForOneFeature(t *testing.T) {
	a, q := feature("a", 1, "in_progress"), feature("q", 2, "queued")
	s := state(a, q)
	if p, _ := s.NextFor("q", "cloud", now); p == nil || p.Feature != q || !p.Starts {
		t.Fatalf("NextFor = %+v, want q started", p)
	}
	s.Config.Limits.InProgress = 1
	if p, _ := s.NextFor("q", "cloud", now); p != nil {
		t.Fatal("NextFor ignored the in-progress limit")
	}
}

func TestLeasesAndRunaway(t *testing.T) {
	a := feature("a", 1, "in_progress")
	s := state(a)
	s.Claim(a, &a.Steps[0], "cloud/run-1", now, true)
	if a.Budget.Spent.Steps != 1 || a.Budget.Spent.Runs != 1 {
		t.Fatalf("spent = %+v", a.Budget.Spent)
	}
	if got := next(t, s, "cloud/run-2"); got != "none" {
		t.Fatalf("next = %q, want none while leased", got)
	}
	if got := next(t, s, "cloud/run-1"); got != "a f0-branch-setup" {
		t.Fatalf("lease holder next = %q", got)
	}
	later := now.Add(3 * time.Hour)
	if p, _ := s.Next("cloud/run-2", later); p == nil {
		t.Fatal("expired lease not reclaimable")
	}
	a.Budget.Spent.Steps = 14
	if got := next(t, s, "cloud/run-1"); got != "none" {
		t.Fatalf("next = %q, want none at twice the estimate", got)
	}
}

func TestAltersSlot(t *testing.T) {
	yes := true
	a, b := feature("a", 1, "in_progress"), feature("b", 2, "in_progress")
	a.AltersRelationships, b.AltersRelationships = &yes, &yes
	a.Phase, a.Steps = 3, []Step{{ID: "f3-implement", Phase: 3, Affinity: "human",
		Status: "pending"}}
	b.Phase, b.Steps = 2, []Step{{ID: "f2-plan", Phase: 2, Affinity: "cloud",
		Status: "pending"}}
	s := state(a, b)
	if got := next(t, s, "cloud"); got != "none" {
		t.Fatalf("next = %q, want b held at the end of Design", got)
	}
	if errs := s.Validate(); len(errs) == 0 {
		t.Fatal("Validate accepted two alters_relationships features past Design")
	}
}

// advance completes the feature's first pending step until it reaches phase.
func advance(f *Feature, phase int) {
	for f.Phase < phase && f.Status != "done" {
		for i := range f.Steps {
			if f.Steps[i].Status == "pending" {
				Complete(f, &f.Steps[i], "test", now)
				break
			}
		}
	}
}

func walk(t *testing.T, f *Feature) []string {
	t.Helper()
	for f.Status != "done" {
		var st *Step
		for i := range f.Steps {
			if f.Steps[i].Status == "pending" {
				st = &f.Steps[i]
				break
			}
		}
		if st == nil {
			t.Fatalf("no pending step at %s/%d", f.Round, f.Phase)
		}
		Complete(f, st, "test", now)
	}
	var ids []string
	for _, st := range f.Steps {
		ids = append(ids, st.ID)
	}
	return ids
}

func TestCompleteWalksOneRound(t *testing.T) {
	f := feature("a", 1, "in_progress")
	want := "f0-branch-setup f1-design f2-design-review f2-plan f3-implement f4-review " +
		"f5-guide f6-merge-main f6-consolidate f6-open-pr f7-merge f8-close"
	if got := strings.Join(walk(t, f), " "); got != want {
		t.Fatalf("steps:\n got %s\nwant %s", got, want)
	}
	gates := map[string]bool{"f1-design": true, "f2-plan": true,
		"f6-open-pr": true, "f7-merge": true, "f8-close": true}
	for i := range f.Steps {
		st := &f.Steps[i]
		switch st.ID {
		case "f2-design-review", "f6-merge-main", "f6-consolidate":
			if IsGate(f, st) {
				t.Fatalf("%s is not its phase's gate", st.ID)
			}
		}
		if gates[st.ID] && !IsGate(f, st) {
			t.Fatalf("%s is its phase's gate", st.ID)
		}
	}
	if f.Steps[2].Affinity != "human" || len(f.Steps[2].Needs) != 0 ||
		f.Steps[3].Needs[0] != "f2-design-review" {
		t.Fatalf("design review %+v, plan %+v: a human step before Plan", f.Steps[2],
			f.Steps[3])
	}
}

func TestIsDesignReview(t *testing.T) {
	for id, want := range map[string]bool{"f2-design-review": true,
		"f2-design-review-r3": true, "f2-plan": false, "f3-design-review": false,
		"x1-design": false} {
		if got := IsDesignReview(&Step{ID: id}); got != want {
			t.Errorf("IsDesignReview(%s) = %v", id, got)
		}
	}
}

// TestOldOrderPhaseTwoKeepsTheReviewAsGate: a phase 2 seeded before the
// review moved ahead of Plan finishes under the order it was seeded with.
func TestOldOrderPhaseTwoKeepsTheReviewAsGate(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.Phase, f.DesignReview = 2, "required"
	f.Steps = []Step{{ID: "f2-plan", Phase: 2, Affinity: "cloud", Status: "pending"},
		{ID: "f2-design-review", Phase: 2, Affinity: "human", Status: "pending",
			Needs: []string{"f2-plan"}}}
	if IsGate(f, &f.Steps[0]) || !IsGate(f, &f.Steps[1]) {
		t.Fatalf("old order: plan gate %v, review gate %v, want the review",
			IsGate(f, &f.Steps[0]), IsGate(f, &f.Steps[1]))
	}
}

// TestReentryAfterADroppedReviewGetsFreshIds: an earlier phase 2 that left
// f2-plan but no f2-design-review (skipped, or dropped by Back) still makes
// the re-entry use -r2 for both.
func TestReentryAfterADroppedReviewGetsFreshIds(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.Phase, f.DesignReview = 2, "required"
	f.Steps = []Step{{ID: "f2-plan", Phase: 2, Affinity: "cloud", Status: "done"}}
	Seed(f)
	var ids []string
	for _, st := range f.Steps {
		ids = append(ids, st.ID+"<"+strings.Join(st.Needs, ","))
	}
	if got := strings.Join(ids, " "); got !=
		"f2-plan< f2-design-review-r2< f2-plan-r2<f2-design-review-r2" {
		t.Fatalf("steps %s", got)
	}
	if errs := state(f).Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	plan, _ := f.Step("f2-plan-r2")
	if !IsGate(f, plan) {
		t.Fatal("f2-plan-r2 is the re-entry's gate")
	}
}

func TestSkippedDesignReviewIsNotSeeded(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.DesignReview = "skip"
	ids := strings.Join(walk(t, f), " ")
	if strings.Contains(ids, "design-review") {
		t.Fatalf("steps %s: a skipped review is not seeded", ids)
	}
	plan, _ := f.Step("f2-plan")
	if !IsGate(f, plan) || len(plan.Needs) != 0 {
		t.Fatalf("with the review skipped, Plan %+v is the phase's gate, needing nothing",
			plan)
	}
}

func TestPRSlotIsSerial(t *testing.T) {
	a, b := feature("a", 1, "in_progress"), feature("b", 2, "in_progress")
	a.Phase, a.Steps = 7, []Step{{ID: "f7-merge", Phase: 7, Affinity: "cloud",
		Status: "pending"}}
	b.Phase, b.Steps = 6, nil
	Seed(b)
	s := state(a, b)
	if got := next(t, s, "cloud"); got != "a f7-merge" {
		t.Fatalf("next = %q, want a's merge check first", got)
	}
	a.Steps[0].Status = "done"
	a.Phase, a.Status = 8, "done"
	if got := next(t, s, "cloud"); got != "b f6-merge-main" {
		t.Fatalf("next = %q, want b once the slot is free", got)
	}
	a.Status, a.Phase = "in_progress", 7
	a.Steps[0].Status = "pending"
	s.Features = []*Feature{b, a}
	_, blocked := s.Next("cloud", now)
	if got := next(t, s, "cloud"); got != "a f7-merge" || !strings.Contains(blocked[0].Reason,
		"PR slot") {
		t.Fatalf("next = %q, blocked %+v", got, blocked)
	}
}

func TestBackToImplementKeepsDesign(t *testing.T) {
	f := feature("a", 1, "in_progress")
	advance(f, 6)
	Back(f, "test", now, "consolidation found a duplicate translator", 3)
	last := f.Steps[len(f.Steps)-1]
	if f.Phase != 3 || last.ID != "f3-implement-r2" {
		t.Fatalf("phase %d, last step %s", f.Phase, last.ID)
	}
	for _, st := range f.Steps {
		if strings.Contains(st.ID, "design-review-r2") {
			t.Fatal("a return to Implement needs no new design review")
		}
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "queue"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("run_budget: {steps: 8}\n"+
			"limits: {in_progress: 5, alters_relationships: 1}\n"), 0o644)
	s := &State{Dir: dir}
	f := feature("x", 1, "queued")
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Features) != 1 || got.Features[0].Steps[0].ID != "f0-branch-setup" ||
		got.Config.RunBudget.Steps != 8 {
		t.Fatalf("loaded %+v", got)
	}
	if errs := got.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

func TestBackToDesignEscalatesOnTheThirdReturn(t *testing.T) {
	f := feature("a", 1, "in_progress")
	advance(f, 3)
	for i := 1; i <= 3; i++ {
		Back(f, "test", now, "plan not viable", 1)
		gate := f.Steps[len(f.Steps)-1]
		if i == 3 {
			gate = f.Steps[len(f.Steps)-2]
		}
		if !IsGate(f, &gate) || gate.Status != "pending" || f.Phase != 1 {
			t.Fatalf("return %d: gate %+v, phase %d", i, gate, f.Phase)
		}
		esc := strings.Contains(strings.Join(gate.Needs, ","), "escalate")
		if esc != (i == 3) {
			t.Fatalf("return %d: needs %v", i, gate.Needs)
		}
		if i < 3 {
			advance(f, 3)
			id := fmt.Sprintf("f2-design-review-r%d", i+1)
			if _, err := f.Step(id); err != nil {
				t.Fatalf("return %d: a return to Design reseeds the review: %v",
					i, err)
			}
			plan, err := f.Step(fmt.Sprintf("f2-plan-r%d", i+1))
			if err != nil || len(plan.Needs) != 1 || plan.Needs[0] != id {
				t.Fatalf("return %d: plan %+v, want it after %s (%v)", i, plan, id, err)
			}
		}
	}
	if errs := state(f).Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

func TestNativeBudget(t *testing.T) {
	dir := t.TempDir()
	write := func(cfg string) *State {
		t.Helper()
		_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o644)
		s, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := write("run_budget: {steps: 12}\n")
	if got := s.Config.Budget(true); got != 12 {
		t.Fatalf("native budget without native_budget = %d, want run_budget's 12", got)
	}
	s = write("run_budget: {steps: 12}\nnative_budget: {steps: 8}\n")
	if s.Config.Budget(true) != 8 || s.Config.Budget(false) != 12 {
		t.Fatalf("budgets = native %d, run %d; want 8 and 12", s.Config.Budget(true),
			s.Config.Budget(false))
	}
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	s = write("run_budget: {steps: 12}\nnative_budget: {steps: 0}\n")
	errs := s.Validate()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "native_budget") {
		t.Fatalf("Validate = %v, want the native_budget problem", errs)
	}
}

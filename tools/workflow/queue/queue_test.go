package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)

func feature(name string, prio int, status string) *Feature {
	f := &Feature{Feature: name, Priority: prio, Status: status, Round: "parallel",
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
	if got := next(t, s, "cloud"); got != "a p0-branch-setup" {
		t.Fatalf("next = %q, want the top-priority feature", got)
	}
	// a blocks on a human step: b advances, and a human worker sees a's step.
	a.Steps[0].Affinity = "human"
	if got := next(t, s, "cloud"); got != "b p0-branch-setup" {
		t.Fatalf("next = %q, want b while a waits on a human", got)
	}
	if got := next(t, s, "human"); got != "a p0-branch-setup" {
		t.Fatalf("human next = %q", got)
	}
	if got := next(t, s, "host:macos-vz"); got != "none" {
		t.Fatalf("host next = %q", got)
	}
	q := feature("q", 3, "queued")
	q.Steps[0].Affinity = "host:macos-vz"
	s.Features = append(s.Features, q)
	if got := next(t, s, "host:macos-vz"); got != "q p0-branch-setup" {
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
	if got := next(t, s, "cloud"); got != "a p0-branch-setup" {
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
	if got := next(t, s, "cloud/run-1"); got != "a p0-branch-setup" {
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
	a.Phase, a.Steps = 3, []Step{{ID: "p3-implement", Phase: 3, Affinity: "human",
		Status: "pending"}}
	b.Phase, b.Steps = 2, []Step{{ID: "p2-plan", Phase: 2, Affinity: "cloud",
		Status: "pending"}}
	s := state(a, b)
	if got := next(t, s, "cloud"); got != "none" {
		t.Fatalf("next = %q, want b held at the end of Design", got)
	}
	if errs := s.Validate(); len(errs) == 0 {
		t.Fatal("Validate accepted two alters_relationships features past Design")
	}
}

func TestCompleteWalksBothRounds(t *testing.T) {
	f := feature("a", 1, "in_progress")
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
	want := "p0-branch-setup p1-design p2-plan p3-implement p4-review p5-guide p6-publish " +
		"p6-merge-round m0-merge-main m1-consolidate m2-plan m3-implement m4-review " +
		"m5-guide m6-publish m7-sign-off m8-archive"
	if got := strings.Join(ids, " "); got != want {
		t.Fatalf("steps:\n got %s\nwant %s", got, want)
	}
	if !IsGate(&f.Steps[1]) || IsGate(&f.Steps[7]) || !IsGate(&f.Steps[14]) ||
		!IsGate(&f.Steps[15]) || !IsGate(&f.Steps[16]) {
		t.Fatal("gate steps are each phase's last template step")
	}
}

func TestMergeQueueIsSerial(t *testing.T) {
	a, b := feature("a", 1, "in_progress"), feature("b", 2, "in_progress")
	a.Round, a.Phase, a.Steps = "merge", 7, []Step{{ID: "m7-sign-off", Phase: 7,
		Affinity: "human", Status: "pending"}}
	b.Phase, b.Steps = 6, []Step{{ID: "p6-merge-round", Phase: 6, Affinity: "cloud",
		Status: "pending"}}
	s := state(a, b)
	_, blocked := s.Next("cloud", now)
	if got := next(t, s, "cloud"); got != "none" || !strings.Contains(blocked[1].Reason,
		"merge queue") {
		t.Fatalf("next = %q, blocked %+v", got, blocked)
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
	if len(got.Features) != 1 || got.Features[0].Steps[0].ID != "p0-branch-setup" ||
		got.Config.RunBudget.Steps != 8 {
		t.Fatalf("loaded %+v", got)
	}
	if errs := got.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

func TestBackToDesignEscalatesOnTheThirdReturn(t *testing.T) {
	f := feature("a", 1, "in_progress")
	for f.Phase < 3 {
		Complete(f, &f.Steps[len(f.Steps)-1], "test", now)
	}
	for i := 1; i <= 3; i++ {
		Back(f, "test", now, "plan not viable")
		gate := f.Steps[len(f.Steps)-1]
		if i == 3 {
			gate = f.Steps[len(f.Steps)-2]
		}
		if !IsGate(&gate) || gate.Status != "pending" || f.Phase != 1 {
			t.Fatalf("return %d: gate %+v, phase %d", i, gate, f.Phase)
		}
		esc := strings.Contains(strings.Join(gate.Needs, ","), "escalate")
		if esc != (i == 3) {
			t.Fatalf("return %d: needs %v", i, gate.Needs)
		}
		if i < 3 {
			Complete(f, &f.Steps[len(f.Steps)-1], "test", now)
			Complete(f, &f.Steps[len(f.Steps)-1], "test", now)
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

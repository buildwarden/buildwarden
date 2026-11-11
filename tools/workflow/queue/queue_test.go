package queue

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	s.Features[0] = process("a", 1, "in_progress")
	if p, _ := s.NextFor("q", "cloud", now); p == nil || p.Feature != q {
		t.Fatalf("NextFor = %+v, want q not refused beside a process item", p)
	}
}

// TestLimitSkipsProcessItems: process items do not count toward the
// in-progress limit (FLOW-05), in Validate, in Next and in NextFor; the
// alters_relationships limit still counts them.
func TestLimitSkipsProcessItems(t *testing.T) {
	a, x := feature("a", 1, "in_progress"), process("x", 2, "in_progress")
	s := state(a, x)
	s.Config.Limits.InProgress = 1
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate = %v, want a process item not counted", errs)
	}
	b := feature("b", 3, "in_progress")
	s.Features = append(s.Features, b)
	if got := fmt.Sprint(s.Validate()); !strings.Contains(got,
		"features in progress, limit 1 (FLOW-05)") {
		t.Fatalf("Validate = %s, want two features over the limit", got)
	}

	yes := true
	f, p := toPhase("f", 1, 2), process("p", 2, "in_progress")
	f.AltersRelationships, p.AltersRelationships = &yes, &yes
	p.Phase = 2
	s = state(f, p)
	if got := fmt.Sprint(s.Validate()); !strings.Contains(got,
		"past Design alter relationships, limit 1") {
		t.Fatalf("Validate = %s, want the process item counted toward alters", got)
	}
}

// TestNextStartsFeatureBesideProcessItem: at limit 1, an in-progress process
// item takes no cloud slot from a queued feature; an in-progress feature does.
func TestNextStartsFeatureBesideProcessItem(t *testing.T) {
	x, q := process("x", 1, "in_progress"), feature("q", 2, "queued")
	x.Steps[0].Affinity = "human"
	s := state(x, q)
	s.Config.Limits.InProgress = 1
	if p, _ := s.Next("cloud", now); p == nil || p.Feature != q || !p.Starts {
		t.Fatalf("pick = %+v, want q started beside the process item", p)
	}
	if p, _ := s.NextFor("q", "cloud", now); p == nil || p.Feature != q {
		t.Fatalf("NextFor = %+v, want q not refused at the limit", p)
	}
	a := feature("a", 1, "in_progress")
	a.Steps[0].Affinity = "human"
	s = state(a, q)
	s.Config.Limits.InProgress = 1
	if got := next(t, s, "cloud"); got != "none" {
		t.Fatalf("next = %q, want none with a feature at the limit", got)
	}
}

// TestHumanStartsProcessItemAtLimit: with the features at the limit, a human
// claim starts a queued process item and the state still validates.
func TestHumanStartsProcessItemAtLimit(t *testing.T) {
	a, x := feature("a", 1, "in_progress"), process("x", 2, "queued")
	a.Steps[0].Affinity = "human"
	s := state(a, x)
	s.Config.Limits.InProgress = 1
	p, _ := s.Next("human/jeff", now)
	if p == nil || p.Feature != x || p.Step.ID != "x1-design" {
		t.Fatalf("human pick = %+v, want x x1-design", p)
	}
	s.Claim(p.Feature, p.Step, "human/jeff", now, true)
	if x.Status != "in_progress" {
		t.Fatalf("status = %s, want the claim to start x", x.Status)
	}
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate = %v, want a started process item not counted", errs)
	}
}

// toPhase is a feature in progress at phase, its earlier steps done.
func toPhase(name string, prio, phase int) *Feature {
	f := feature(name, prio, "in_progress")
	advance(f, phase)
	return f
}

// TestNextTakesClassOrder: a queued fix (class 3) starts ahead of an
// in-progress class-4 feature, the security-suite case, but not at the
// in-progress limit.
func TestNextTakesClassOrder(t *testing.T) {
	parent := feature("parent", 1, "done")
	sec := feature("sec", 9, "in_progress")
	fix := feature("fix", 5, "queued")
	fix.Follows = "parent"
	s := state(parent, sec, fix)
	if p, _ := s.Next("cloud", now); p == nil || p.Feature != fix || !p.Starts {
		t.Fatalf("pick = %+v, want the queued fix started ahead of class-4 sec", p)
	}
	s.Config.Limits.InProgress = 1
	if got := next(t, s, "cloud"); got != "sec f0-branch-setup" {
		t.Fatalf("at the in-progress limit next = %q, want sec's step", got)
	}

	// A phase-7 merge check (class 4) comes after the fix's start.
	merging := toPhase("merging", 2, 7)
	s = state(parent, merging, fix)
	if p, _ := s.Next("cloud", now); p == nil || p.Feature != fix || !p.Starts {
		t.Fatalf("pick = %+v, want the fix before merging's f7-merge", p)
	}
}

// TestNextTakesImplementationFirst: a feature in Implement (class 1) goes
// ahead of a process item waiting on its merge (class 2), which is never
// offered, and nothing goes past a hold.
func TestNextTakesImplementationFirst(t *testing.T) {
	proc := process("proc", 1, "in_progress")
	advance(proc, 3)
	impl := toPhase("impl", 9, 3)
	s := state(proc, impl)
	if got := next(t, s, "cloud"); got != "impl f3-implement" {
		t.Fatalf("next = %q, want the class-1 feature before x3-merge", got)
	}
	impl.Steps[len(impl.Steps)-1].Affinity = "human"
	pick, blocked := s.Next("cloud", now)
	want := []Blocked{{"impl", "waiting on human step f3-implement"},
		{"proc", "waiting on the human's merge (no PR recorded, FLOW-08)"}}
	if pick != nil || !slices.Equal(blocked, want) {
		t.Fatalf("next = %+v, %+v; want no pick once impl blocks, proc on its merge",
			pick, blocked)
	}
	impl.Steps[len(impl.Steps)-1].Affinity = "cloud"

	// A hold still stops a class-1 feature's cloud step.
	rules := process("rules", 2, "in_progress", "cloud")
	s = state(rules, impl)
	pick, blocked = s.Next("cloud", now)
	if pick != nil || len(blocked) != 2 || blocked[0].Feature != "impl" ||
		blocked[0].Reason != "held by process item rules" {
		t.Fatalf("next = %+v, %+v; want impl held by rules", pick, blocked)
	}
}

// TestHumanTakesQueuedProcessFirst: a human gets a queued process item's
// design (class 2) before an in-progress class-4 feature's human step.
func TestHumanTakesQueuedProcessFirst(t *testing.T) {
	a := feature("a", 1, "in_progress")
	a.Steps[0].Affinity = "human"
	s := state(a, process("p", 7, "queued"))
	if p, _ := s.Next("human/jeff", now); p == nil || p.Step.ID != "x1-design" ||
		!p.Starts {
		t.Fatalf("human pick = %+v, want p x1-design", p)
	}
}

func TestFollowUpWaitsForParent(t *testing.T) {
	parent := toPhase("parent", 1, 3)
	parent.Steps[len(parent.Steps)-1].Affinity = "host:macos-vz"
	fix := feature("fix", 2, "queued")
	fix.Follows = "parent"
	proc := process("proc", 3, "queued")
	proc.Follows = "parent"
	s := state(parent, fix, proc)
	pick, blocked := s.Next("cloud", now)
	if pick != nil || len(blocked) != 2 || blocked[1].Feature != "fix" ||
		blocked[1].Reason != "follows parent, not yet done (FLOW-05)" {
		t.Fatalf("cloud next = %+v, %+v; want fix waiting on parent", pick, blocked)
	}
	if got := next(t, s, "human"); got != "proc x1-design" {
		t.Fatalf("human next = %q; follows on a process item is only a record", got)
	}
	if p, _ := s.NextFor("fix", "cloud", now); p == nil || p.Feature != fix || !p.Starts {
		t.Fatalf("NextFor = %+v; naming the fix skips the gate", p)
	}
	parent.Status = "done"
	if p, _ := s.Next("cloud", now); p == nil || p.Feature != fix || !p.Starts {
		t.Fatalf("pick = %+v, want fix started once parent is done", p)
	}
}

func TestOrderFollowsPhaseChanges(t *testing.T) {
	names := func(s *State) string {
		var out []string
		for _, f := range s.Ordered() {
			out = append(out, f.Feature)
		}
		return strings.Join(out, " ")
	}
	a := toPhase("a", 2, 2)
	z := feature("z", 9, "in_progress")
	q := feature("q", 3, "queued")
	s := state(process("p", 1, "in_progress"), a, z, q)
	if got := names(s); got != "p a z q" {
		t.Fatalf("Ordered = %q", got)
	}
	s.Claim(q, &q.Steps[0], "cloud/run", now, true)
	if got := names(s); got != "p a q z" {
		t.Fatalf("after q's claim Ordered = %q, want q among the in-progress items",
			got)
	}
	advance(a, 3)
	if got := names(s); got != "a p q z" {
		t.Fatalf("after a reaches Implement Ordered = %q, want a first", got)
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
	if !s.Runaway(a) {
		t.Fatal("in-progress item at twice the estimate is not a runaway")
	}
	if got := next(t, s, "cloud/run-1"); got != "none" {
		t.Fatalf("next = %q, want none at twice the estimate", got)
	}
	d := feature("d", 2, "done")
	d.Budget.Spent.Steps = 30
	if s.Runaway(d) {
		t.Fatal("done item past the factor is a runaway")
	}
}

func TestListed(t *testing.T) {
	p := feature("p", 9, "in_progress")
	p.Kind = "process"
	s := state(feature("q", 2, "queued"), feature("d", 1, "done"),
		feature("a", 1, "in_progress"), p)
	names := func(fs []*Feature) string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Feature)
		}
		return strings.Join(out, " ")
	}
	if got := names(s.Listed(false)); got != "p a q" {
		t.Fatalf("Listed(false) = %q, want open items in FLOW-05 order", got)
	}
	if got := names(s.Listed(true)); got != "p a q d" {
		t.Fatalf("Listed(true) = %q, want done items added after the open ones", got)
	}
}

func TestClass(t *testing.T) {
	for _, c := range []struct {
		kind, status, follows string
		phase, want           int
	}{
		{"process", "queued", "", 1, 2},
		{"process", "in_progress", "", 1, 2},
		{"process", "in_progress", "", 2, 1},
		{"process", "in_progress", "", 3, 2},
		{"process", "in_progress", "", 4, 1},
		{"process", "in_progress", "a", 3, 2},
		{"", "queued", "", 0, 4},
		{"", "queued", "a", 0, 3},
		{"", "in_progress", "", 2, 4},
		{"", "in_progress", "a", 2, 3},
		{"", "in_progress", "", 3, 1},
		{"", "in_progress", "a", 6, 1},
		{"", "in_progress", "", 7, 4},
		{"", "in_progress", "a", 7, 3},
		{"", "in_progress", "", 8, 1},
		{"", "done", "", 8, 0},
		{"process", "done", "", 4, 0},
	} {
		f := &Feature{Kind: c.kind, Status: c.status, Follows: c.follows, Phase: c.phase}
		if got := f.Class(); got != c.want {
			t.Errorf("Class(%+v) = %d, want %d", c, got, c.want)
		}
	}
}

func TestValidateFollows(t *testing.T) {
	parent := feature("parent", 1, "done")
	fix := feature("fix", 2, "queued")
	fix.Follows = "parent"
	if errs := state(parent, fix).Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v; a done parent is a valid one", errs)
	}
	for _, name := range []string{"nobody", "fix"} {
		fix.Follows = name
		errs := state(parent, fix).Validate()
		if len(errs) != 1 || !strings.Contains(errs[0].Error(),
			`fix: follows "`+name+`"`) {
			t.Fatalf("follows %s: Validate = %v", name, errs)
		}
	}
}

func TestAddLogsRenumbering(t *testing.T) {
	a, b, c := feature("a", 1, "in_progress"), feature("b", 2, "queued"),
		feature("c", 3, "queued")
	s := state(a, b, c)
	fix := &Feature{Feature: "fix", Priority: 2, Follows: "a"}
	changed, err := s.Add(fix, now, "cloud/run")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 3 || len(a.History) != 0 {
		t.Fatalf("changed %d, a's history %v; want fix, b and c, and a untouched",
			len(changed), a.History)
	}
	for _, f := range []*Feature{b, c} {
		want := fmt.Sprintf("priority %d -> %d (renumbered by wf new: fix queued at 2)",
			f.Priority-1, f.Priority)
		if len(f.History) != 1 || f.History[0].Event != want ||
			f.History[0].By != "cloud/run" {
			t.Errorf("%s history %+v, want %q", f.Feature, f.History, want)
		}
	}
	if fix.Class() != 3 || s.Features[0] != fix {
		t.Fatalf("fix is class %d, first is %s; want class 3, ahead of the class-4 a",
			fix.Class(), s.Features[0].Feature)
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

// advance completes the feature's first pending step of its current phase
// until it reaches phase.
func advance(f *Feature, phase int) {
	for f.Phase < phase && f.Status != "done" {
		for i := range f.Steps {
			if f.Steps[i].Status == "pending" && f.Steps[i].Phase == f.Phase {
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

func TestIsMerge(t *testing.T) {
	for id, want := range map[string]bool{"f7-merge": true, "f7-merge-r2": true,
		"x3-merge": true, "f6-merge-main": false, "x2-implement": false,
		"f7-other": false, "m6-merge-round": false} {
		if got := IsMerge(&Step{ID: id}); got != want {
			t.Errorf("IsMerge(%s) = %v", id, got)
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
	// a holds the slot at its merge step, which is never offered: no pick.
	slotHeld := func() {
		t.Helper()
		pick, blocked := s.Next("cloud", now)
		got := map[string]string{}
		for _, b := range blocked {
			got[b.Feature] = b.Reason
		}
		want := map[string]string{
			"a": "waiting on the human's merge (no PR recorded, FLOW-08)",
			"b": "waiting on the PR slot, held by a (FLOW-05)"}
		if pick != nil || len(blocked) != 2 || !maps.Equal(got, want) {
			t.Fatalf("next = %+v, blocked %+v; want b on the slot, a on its merge",
				pick, blocked)
		}
	}
	slotHeld()
	a.Steps[0].Status = "done"
	a.Phase, a.Status = 8, "done"
	if got := next(t, s, "cloud"); got != "b f6-merge-main" {
		t.Fatalf("next = %q, want b once the slot is free", got)
	}
	a.Status, a.Phase = "in_progress", 7
	a.Steps[0].Status = "pending"
	s.Features = []*Feature{b, a}
	slotHeld()
}

// mergeWaiting is an in-progress item at its round's merge step, with PR
// number pr (0 for none recorded).
func mergeWaiting(f *Feature, pr int) *Feature {
	advance(f, map[string]int{"feature": 7, "process": 3}[f.Round])
	if pr > 0 {
		f.PR = &PR{Number: pr}
	}
	return f
}

// TestNextSkipsMergeWait: no worker is offered a merge step; the item is
// reported as waiting on the human's merge and the walk goes on (FLOW-05).
func TestNextSkipsMergeWait(t *testing.T) {
	parent := feature("parent", 1, "done")
	merging := mergeWaiting(process("merging", 53, "in_progress"), 104)
	fix := feature("fix", 60, "in_progress")
	fix.Follows, fix.Phase = "parent", 1
	fix.Steps = []Step{{ID: "f1-design-r3", Phase: 1, Affinity: "cloud",
		Status: "pending"}}
	s := state(parent, merging, fix)
	pick, blocked := s.Next("cloud", now)
	want := []Blocked{{"merging", "waiting on the human's merge of PR #104 (FLOW-08)"}}
	if pick == nil || pick.Feature != fix || pick.Step.ID != "f1-design-r3" ||
		!slices.Equal(blocked, want) {
		t.Fatalf("next = %+v, %+v; want the fix past merging's x3-merge", pick, blocked)
	}

	// Every worker gets the merge reason, not the step's affinity.
	for _, w := range []string{"host:macos-vz", "human/jeff"} {
		s := state(merging)
		if p, blocked := s.Next(w, now); p != nil || !slices.Equal(blocked, want) {
			t.Errorf("%s: next = %+v, %+v; want merging on its merge", w, p, blocked)
		}
	}

	// A feature at f7-merge, its -r2 form too, yields within class 4: to an
	// in-progress feature behind it, or to a queued one, which starts.
	for _, id := range []string{"f7-merge", "f7-merge-r2"} {
		a := mergeWaiting(feature("a", 1, "in_progress"), 7)
		a.Steps[len(a.Steps)-1].ID = id
		b := feature("b", 2, "in_progress")
		if got := next(t, state(a, b), "cloud"); got != "b f0-branch-setup" {
			t.Errorf("%s: next = %q, want b past a's merge", id, got)
		}
		q := feature("q", 2, "queued")
		s := state(a, q)
		s.Config.Limits.InProgress = 2
		if p, _ := s.Next("cloud", now); p == nil || p.Feature != q || !p.Starts {
			t.Errorf("%s: pick = %+v, want q started past a's merge", id, p)
		}
	}

	// With no PR recorded the reason is exact, for a nil PR and a PR 0.
	for _, pr := range []*PR{nil, {}} {
		f := mergeWaiting(process("x", 1, "in_progress"), 0)
		f.PR = pr
		const why = "the human's merge (no PR recorded, FLOW-08)"
		if got := mergeWait(f); got != why {
			t.Errorf("PR %+v: mergeWait = %q", pr, got)
		}
		_, blocked := state(f).Next("cloud", now)
		if !slices.Equal(blocked, []Blocked{{"x", "waiting on " + why}}) {
			t.Errorf("PR %+v: blocked %+v", pr, blocked)
		}
	}

	// An attended run on the item gets no pick and the reason.
	if p, blocked := state(merging).NextFor("merging", "cloud", now); p != nil ||
		!slices.Equal(blocked, want) {
		t.Fatalf("NextFor = %+v, %+v; want merging on its merge", p, blocked)
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
	f, p := feature("x", 1, "queued"), feature("p", 2, "done")
	f.Follows = "p"
	for _, it := range []*Feature{f, p} {
		if err := s.Save(it); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Features) != 2 || got.Features[0].Steps[0].ID != "f0-branch-setup" ||
		got.Features[0].Follows != "p" || got.Config.RunBudget.Steps != 8 {
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
				t.Fatalf("return %d: plan %+v, want it after %s (%v)", i, plan, id,
					err)
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

// laterSteps are steps Design or a native session added for Publish: a host
// validation and a human provisioning step.
func laterSteps() []Step {
	return []Step{
		{ID: "f6-validate-macos-vz", Phase: 6, Affinity: "host:macos-vz",
			Status: "pending"},
		{ID: "f6-provision-macos-vz", Phase: 6, Affinity: "human", Status: "pending"},
	}
}

func TestLaterPhaseStepWaitsForItsPhase(t *testing.T) {
	for phase := 2; phase <= 6; phase++ {
		f := feature("a", 1, "in_progress")
		f.Phase = phase
		f.Steps = append([]Step{{ID: StepID("feature", phase, "x"), Phase: phase,
			Affinity: "cloud", Status: "pending"}}, laterSteps()...)
		s := nativeState(f)
		got := next(t, s, "host:macos-vz")
		if want := map[bool]string{true: "a f6-validate-macos-vz",
			false: "none"}[phase == 6]; got != want {
			t.Fatalf("phase %d: host next = %q, want %q", phase, got, want)
		}
		if got := next(t, s, "human"); got != "a f6-provision-macos-vz" {
			t.Fatalf("phase %d: human next = %q, want the provisioning step",
				phase, got)
		}
		_, blocked := s.Next("host:macos-vz", now)
		if phase < 6 && strings.Contains(blocked[0].Reason, "f6-validate") {
			t.Fatalf("phase %d: a later step explains the wait: %v", phase, blocked)
		}
	}
}

func TestEarlyRefusesOnlyLaterHostAndCloudSteps(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.Phase = 3
	for _, c := range []struct {
		st    Step
		early bool
	}{
		{Step{ID: "f2-spike-macos-vz", Phase: 2, Affinity: "host:macos-vz"}, false},
		{Step{ID: "f3-implement", Phase: 3, Affinity: "cloud"}, false},
		{Step{ID: "f6-provision-macos-vz", Phase: 6, Affinity: "human"}, false},
		{Step{ID: "f6-validate-macos-vz", Phase: 6, Affinity: "host:macos-vz"}, true},
		{Step{ID: "f4-extra", Phase: 4, Affinity: "cloud"}, true},
	} {
		err := Early(f, &c.st)
		if (err != nil) != c.early {
			t.Fatalf("Early(%s) = %v, want refused %v", c.st.ID, err, c.early)
		}
		if err != nil && (!strings.Contains(err.Error(), "FLOW-04") ||
			!strings.Contains(err.Error(), "release it")) {
			t.Fatalf("Early(%s) = %v, want the FLOW-04 refusal and its way out",
				c.st.ID, err)
		}
	}
}

func TestHeldWorkerKeepsOnlyAnOfferedLease(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.Phase = 3
	f.Steps = append([]Step{{ID: "f3-implement", Phase: 3, Affinity: "cloud",
		Status: "pending"}}, laterSteps()...)
	s := nativeState(f, process("rules", 0, "in_progress", "host:*"))
	s.sortFeatures()
	worker := "host:macos-vz/s"
	s.Claim(f, &f.Steps[1], worker, now, true)
	if got := next(t, s, worker); got != "none" {
		t.Fatalf("next = %q, want no lease on a later-phase step", got)
	}
	f.Phase = 6
	if got := next(t, s, worker); got != "a f6-validate-macos-vz" {
		t.Fatalf("next = %q, want the leased step once its phase is reached", got)
	}
}

// publishWithValidation is a feature in Publish with merge-main done, four
// validations added in Implement (one leased, each with a note and the hand
// need of the old workaround), and a human repair step added in Guide.
func publishWithValidation(t *testing.T) (*Feature, *State) {
	t.Helper()
	f := feature("a", 1, "in_progress")
	advance(f, 3)
	for _, h := range []string{"macos-vz", "macos-qemu", "windows-hyperv", "qemu-any"} {
		f.Steps = append(f.Steps, Step{ID: "f6-validate-" + h, Phase: 6,
			Affinity: "host:" + h, Status: "pending", Note: "cases for " + h,
			Needs: []string{"f3-implement"}})
	}
	advance(f, 5)
	f.Steps = append(f.Steps, Step{ID: "f6-provision-macos-vz-dns", Phase: 6,
		Affinity: "human", Status: "pending", Note: "dns"})
	advance(f, 6)
	s := nativeState(f)
	merge, _ := f.Step("f6-merge-main")
	Complete(f, merge, "test", now)
	v, _ := f.Step("f6-validate-macos-vz")
	s.Claim(f, v, "host:macos-vz/s", now, true)
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	return f, s
}

func TestBackToImplementKeepsAddedLaterSteps(t *testing.T) {
	f, s := publishWithValidation(t)
	open, _ := f.Step("f6-open-pr")
	if got := strings.Join(open.Needs, ","); got != "f6-consolidate,f6-validate-macos-vz,"+
		"f6-validate-macos-qemu,f6-validate-windows-hyperv,f6-validate-qemu-any,"+
		"f6-provision-macos-vz-dns" {
		t.Fatalf("f6-open-pr needs %s, want consolidate and every added step", got)
	}
	Back(f, "test", now, "validation failed on macos-vz", 3)
	if f.hasStep("f6-open-pr") || f.hasStep("f6-consolidate") {
		t.Fatal("Back kept a pending template step of a later phase")
	}
	for _, st := range f.Steps {
		if st.Phase != 6 || st.Status != "pending" {
			continue
		}
		if !strings.Contains(st.ID, "validate") && !strings.Contains(st.ID, "provision") {
			t.Fatalf("kept %s", st.ID)
		}
		if st.Note == "" || st.Lease.ClaimedBy != "" ||
			slices.Contains(st.Needs, "f6-consolidate") {
			t.Fatalf("kept %+v: want its note, no lease, no dropped need", st)
		}
	}
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate after Back: %v", errs)
	}
}

func TestReenteredPublishWaitsForKeptSteps(t *testing.T) {
	f, s := publishWithValidation(t)
	Back(f, "test", now, "validation failed on macos-vz", 3)
	advance(f, 6)
	open, _ := f.Step("f6-open-pr-r2")
	if got := strings.Join(open.Needs, ","); got != "f6-consolidate-r2,"+
		"f6-validate-macos-vz,f6-validate-macos-qemu,f6-validate-windows-hyperv,"+
		"f6-validate-qemu-any,f6-provision-macos-vz-dns" {
		t.Fatalf("f6-open-pr-r2 needs %s", got)
	}
	repair, _ := f.Step("f6-provision-macos-vz-dns")
	if len(repair.Needs) != 0 {
		t.Fatalf("repair step needs %v, want none so it can be done early", repair.Needs)
	}
	// Validation waits for consolidation, then runs before the PR opens.
	if got := next(t, s, "host:windows-hyperv"); got != "none" {
		t.Fatalf("host next = %q before consolidation", got)
	}
	for _, id := range []string{"f6-merge-main-r2", "f6-consolidate-r2"} {
		st, _ := f.Step(id)
		Complete(f, st, "test", now)
	}
	if got := next(t, s, "host:windows-hyperv"); got != "a f6-validate-windows-hyperv" {
		t.Fatalf("host next = %q after consolidation", got)
	}
	if got := next(t, s, "cloud"); got != "none" {
		t.Fatalf("cloud next = %q, want open-pr held by validation", got)
	}
	// A step added once Publish is seeded is not linked to the gate.
	f.Steps = append(f.Steps, Step{ID: "f6-repair-late", Phase: 6, Affinity: "cloud",
		Status: "pending"})
	if open, _ := f.Step("f6-open-pr-r2"); slices.Contains(open.Needs, "f6-repair-late") {
		t.Fatal("a step added after seeding was linked to the gate")
	}
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

func TestBackToDesignKeepsAddedLaterSteps(t *testing.T) {
	f := feature("a", 1, "in_progress")
	advance(f, 3)
	f.Steps = append(f.Steps,
		Step{ID: "f2-spike-macos-vz", Phase: 2, Affinity: "host:macos-vz",
			Status: "pending", Needs: []string{"f3-implement"}},
		laterSteps()[0])
	Back(f, "test", now, "plan not viable", 1)
	for _, id := range []string{"f2-spike-macos-vz", "f6-validate-macos-vz"} {
		st, err := f.Step(id)
		if err != nil || st.Status != "pending" || len(st.Needs) != 0 {
			t.Fatalf("%s = %+v, %v: want it kept, pending, without the dropped need",
				id, st, err)
		}
	}
	if f.hasStep("f3-implement") || !f.hasStep("f1-design-r2") {
		t.Fatal("Back did not drop and reseed the template steps")
	}
	if errs := nativeState(f).Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	advance(f, 2)
	review, _ := f.Step("f2-design-review-r2")
	if slices.Contains(review.Needs, "f2-spike-macos-vz") {
		t.Fatal("only Publish's gate waits for added steps")
	}
}

func TestBackQueuesDoneValidationAgain(t *testing.T) {
	f := feature("a", 1, "in_progress")
	advance(f, 6)
	f.Steps = append(f.Steps,
		Step{ID: "f2-spike-macos-vz", Phase: 2, Affinity: "host:macos-vz", Status: "done"},
		Step{ID: "f6-validate-macos-vz", Phase: 6, Affinity: "host:macos-vz",
			Status: "done", Note: "cases"})
	ids := func() string {
		var out []string
		for _, st := range f.Steps {
			if strings.Contains(st.ID, "macos-vz") {
				out = append(out, st.ID+"="+st.Status)
			}
		}
		return strings.Join(out, " ")
	}
	Back(f, "test", now, "the other host failed", 3)
	want := "f2-spike-macos-vz=done f6-validate-macos-vz=done f6-validate-macos-vz-r2=pending"
	if got := ids(); got != want {
		t.Fatalf("after one return: %s, want %s", got, want)
	}
	if st, _ := f.Step("f6-validate-macos-vz-r2"); st.Note != "cases" {
		t.Fatalf("copy %+v: want the note", st)
	}
	Back(f, "test", now, "again before it ran", 3)
	if got := ids(); got != want {
		t.Fatalf("after a second return: %s, want %s", got, want)
	}
	st, _ := f.Step("f6-validate-macos-vz-r2")
	st.Status = "done"
	Back(f, "test", now, "a third return", 3)
	want = "f2-spike-macos-vz=done f6-validate-macos-vz=done " +
		"f6-validate-macos-vz-r2=done f6-validate-macos-vz-r3=pending"
	if got := ids(); got != want {
		t.Fatalf("after a third return: %s, want %s", got, want)
	}
	if errs := nativeState(f).Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

func TestValidateRejectsAReaddedStep(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.Steps = append(f.Steps, laterSteps()[0], laterSteps()[0])
	errs := nativeState(f).Validate()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "repeats") {
		t.Fatalf("Validate = %v, want the repeated id", errs)
	}
}

// The gate is the template step no other template step needs; an added
// step's needs do not move it, so Publish's host steps needing consolidate
// leave open-pr the gate, and a phase-2 step needing the review leaves Plan.
func TestAddedStepsDoNotMoveTheGate(t *testing.T) {
	f, _ := publishWithValidation(t)
	for id, want := range map[string]bool{"f6-open-pr": true, "f6-consolidate": false,
		"f6-validate-macos-vz": false} {
		if st, _ := f.Step(id); IsGate(f, st) != want {
			t.Errorf("IsGate(%s) = %v, want %v", id, !want, want)
		}
	}
	g := feature("b", 1, "in_progress")
	advance(g, 2)
	g.Steps = append(g.Steps, Step{ID: "f2-answer-q1", Phase: 2, Affinity: "human",
		Status: "pending", Needs: []string{"f2-design-review"}})
	for id, want := range map[string]bool{"f2-plan": true, "f2-design-review": false,
		"f2-answer-q1": false} {
		if st, _ := g.Step(id); IsGate(g, st) != want {
			t.Errorf("IsGate(%s) = %v, want %v", id, !want, want)
		}
	}
	if IsDesignReview(&Step{ID: "f6-design-review"}) ||
		!IsDesignReview(&Step{ID: "f2-design-review-r3"}) {
		t.Error("IsDesignReview: only phase 2's review, in any -r<n> form")
	}
}

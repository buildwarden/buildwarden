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
	inProgress(s, 5)
	return limited(s, 1)
}

// inProgress sets limits.in_progress as config.yaml would write it.
func inProgress(s *State, n int) { s.Config.Limits.InProgress = &n }

// limited sets limits.alters_relationships as config.yaml would write it.
func limited(s *State, n int) *State {
	s.Config.Limits.AltersRelationships = &n
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
	inProgress(s, 1)
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
	inProgress(s, 1)
	if p, _ := s.NextFor("q", "cloud", now); p != nil {
		t.Fatal("NextFor ignored the in-progress limit")
	}
	s.Features[0] = process("a", 1, "in_progress")
	if p, _ := s.NextFor("q", "cloud", now); p == nil || p.Feature != q {
		t.Fatalf("NextFor = %+v, want q not refused beside a process item", p)
	}
}

// TestLimitSkipsProcessItems: process items do not count toward the
// in-progress limit (FLOW-05), in Validate, in Next and in NextFor.
func TestLimitSkipsProcessItems(t *testing.T) {
	a, x := feature("a", 1, "in_progress"), process("x", 2, "in_progress")
	s := state(a, x)
	inProgress(s, 1)
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate = %v, want a process item not counted", errs)
	}
	b := feature("b", 3, "in_progress")
	s.Features = append(s.Features, b)
	if got := fmt.Sprint(s.Validate()); !strings.Contains(got,
		"features in progress, limit 1 (FLOW-05)") {
		t.Fatalf("Validate = %s, want two features over the limit", got)
	}
}

// TestNextStartsFeatureBesideProcessItem: at limit 1, an in-progress process
// item takes no cloud slot from a queued feature; an in-progress feature does.
func TestNextStartsFeatureBesideProcessItem(t *testing.T) {
	x, q := process("x", 1, "in_progress"), feature("q", 2, "queued")
	x.Steps[0].Affinity = "human"
	s := state(x, q)
	inProgress(s, 1)
	if p, _ := s.Next("cloud", now); p == nil || p.Feature != q || !p.Starts {
		t.Fatalf("pick = %+v, want q started beside the process item", p)
	}
	if p, _ := s.NextFor("q", "cloud", now); p == nil || p.Feature != q {
		t.Fatalf("NextFor = %+v, want q not refused at the limit", p)
	}
	a := feature("a", 1, "in_progress")
	a.Steps[0].Affinity = "human"
	s = state(a, q)
	inProgress(s, 1)
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
	inProgress(s, 1)
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
	inProgress(s, 1)
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

// held queues an item of the given kind through Add and puts it on hold, as
// wf new -hold does, in a state whose directory Save can write to.
func held(t *testing.T, kind, why string) (*State, *Feature) {
	t.Helper()
	s := state()
	s.Dir = t.TempDir()
	_ = os.MkdirAll(filepath.Join(s.Dir, "queue"), 0o755)
	f := &Feature{Feature: "held", Priority: 1, Kind: kind,
		Budget: Budget{Estimate: Count{Runs: 1, Steps: 7}}}
	if _, err := s.Add(f, now, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	if err := f.PutOnHold(why, now, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	return s, f
}

// TestPutOnHoldWrites: wf new -hold writes what the hand-edit wrote for a
// queued item (FLOW-20): q1 with the reason as given, the human answer step,
// and the item's first step needing it. The file it writes validates.
func TestPutOnHoldWrites(t *testing.T) {
	const why = "frozen while the componentization program runs"
	for kind, ids := range map[string][2]string{
		"":        {"f0-answer-q1", "f0-branch-setup"},
		"process": {"x1-answer-q1", "x1-design"},
	} {
		s, f := held(t, kind, why)
		q := f.OpenBlocking()
		if len(q) != 1 || q[0].ID != "q1" || q[0].Question != why {
			t.Errorf("%q: open blocking %+v, want q1 with the reason", kind, q)
		}
		if len(f.Steps) != 2 || f.Steps[0].ID != ids[0] ||
			f.Steps[0].Affinity != "human" || f.Steps[0].Phase != f.Phase ||
			f.Steps[1].ID != ids[1] ||
			!slices.Equal(f.Steps[1].Needs, ids[:1]) {
			t.Errorf("%q: steps %+v, want %v", kind, f.Steps, ids)
		}
		last := f.History[len(f.History)-1].Event
		if last != "on hold (q1): "+why {
			t.Errorf("%q: last history line %q", kind, last)
		}
		if md := Summary(s); !strings.Contains(md, "blocking question q1: "+why) {
			t.Errorf("%q: QUEUE.md does not list the hold:\n%s", kind, md)
		}
	}
}

// TestHeldItemWaitsForItsAnswer: the cloud gets no step of a held item and,
// while it is queued, no blocked line; the human is offered only the answer
// step. Answering q1 leaves the item queued with its first step ready
// (FLOW-07, FLOW-20).
func TestHeldItemWaitsForItsAnswer(t *testing.T) {
	for kind, want := range map[string][2]string{
		"":        {"cloud/run", "held f0-branch-setup"},
		"process": {"human/jeff", "held x1-design"},
	} {
		s, f := held(t, kind, "why")
		if p, blocked := s.Next("cloud/run", now); p != nil || len(blocked) != 0 {
			t.Errorf("%q: cloud pick %+v, blocked %v; want neither", kind, p, blocked)
		}
		if got := next(t, s, "human/jeff"); got != "held "+f.Steps[0].ID {
			t.Errorf("%q: human pick = %s, want only the answer step", kind, got)
		}
		// With the answer step leased to someone else, only the needs link
		// keeps a human from the process item's own first step.
		exp := now.Add(time.Hour)
		f.Steps[0].Lease = Lease{ClaimedBy: "human/other", Expires: &exp}
		if got := next(t, s, "human/jeff"); got != "none" {
			t.Errorf("%q: human pick = %s past a leased answer step", kind, got)
		}
		f.Steps[0].Lease = Lease{}

		// What wf answer does: the question answered, its step complete.
		f.Questions[0].Status = "answered"
		Complete(f, &f.Steps[0], "human/jeff", now)
		if f.Status != "queued" || len(f.OpenBlocking()) != 0 {
			t.Errorf("%q: status %s, open %v; want queued, none open", kind,
				f.Status, f.OpenBlocking())
		}
		if got := next(t, s, want[0]); got != want[1] {
			t.Errorf("%q: %s pick = %s, want %s", kind, want[0], got, want[1])
		}
		if errs := s.Validate(); len(errs) != 0 {
			t.Errorf("%q: Validate after the answer: %v", kind, errs)
		}
	}
}

// TestPutOnHoldRefusals: a hold is explicit and never merged into an item
// that is not newly queued (GLOBAL-10).
func TestPutOnHoldRefusals(t *testing.T) {
	asked := feature("asked", 3, "queued")
	asked.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open",
		Question: "?", Options: []string{"a"}, Recommended: "a"}}
	unseeded := &Feature{Feature: "c", Status: "queued", Round: "feature"}
	for name, tc := range map[string]struct {
		f   *Feature
		why string
	}{
		"no reason":        {feature("a", 1, "queued"), " "},
		"in progress":      {feature("b", 2, "in_progress"), "why"},
		"has a question":   {asked, "why"},
		"no step to gate":  {unseeded, "why"},
		"already finished": {feature("d", 4, "done"), "why"},
	} {
		before := len(tc.f.Steps)
		if err := tc.f.PutOnHold(tc.why, now, "human/jeff"); err == nil {
			t.Errorf("%s: PutOnHold succeeded", name)
		}
		if len(tc.f.Steps) != before || len(tc.f.History) != 0 {
			t.Errorf("%s: a refused hold changed the item: %+v", name, tc.f)
		}
	}
}

// altering is an in-progress feature that declares alters_relationships: true
// and has finished Design: its review and its Plan step are pending.
func altering(name string, prio int) *Feature {
	f := toPhase(name, prio, 2)
	declare(f, true)
	return f
}

// approved is altering with its design review done, so that only a slot can
// keep its Plan step from the cloud.
func approved(name string, prio int) *Feature {
	f := altering(name, prio)
	approve(f)
	return f
}

// declare writes alters_relationships into the queue file, as Design does.
func declare(f *Feature, alters bool) { f.AltersRelationships = &alters }

// approve completes the feature's pending design review.
func approve(f *Feature) {
	for i := range f.Steps {
		if st := &f.Steps[i]; IsDesignReview(st) && st.Status == "pending" {
			Complete(f, st, "human/jeff", now)
			return
		}
	}
}

// plan is the feature's pending Plan step.
func plan(t *testing.T, f *Feature) *Step {
	t.Helper()
	for i := range f.Steps {
		if st := &f.Steps[i]; IsPlan(st) && st.Status == "pending" {
			return st
		}
	}
	t.Fatalf("%s has no pending Plan step", f.Feature)
	return nil
}

// claimPlan claims the feature's Plan step for a cloud run of its own,
// cloud/<feature>, so that another run's Next passes over the lease.
func claimPlan(t *testing.T, s *State, f *Feature) {
	t.Helper()
	s.Claim(f, plan(t, f), "cloud/"+f.Feature, now, true)
}

// finishPlan claims and completes the feature's Plan step, which puts it in
// Implement.
func finishPlan(t *testing.T, s *State, f *Feature) {
	t.Helper()
	claimPlan(t, s, f)
	Complete(f, plan(t, f), "cloud/"+f.Feature, now)
}

// offerAt is NextFor's answer for the feature: the step's id, or why the
// worker gets none.
func offerAt(s *State, name, worker string, at time.Time) string {
	p, blocked := s.NextFor(name, worker, at)
	if p != nil {
		return p.Step.ID
	}
	return blocked[0].Reason
}

// wantOfferTo asserts what the worker is offered of the feature.
func wantOfferTo(t *testing.T, s *State, name, worker, want string) {
	t.Helper()
	if got := offerAt(s, name, worker, now); got != want {
		t.Fatalf("%s: %s is offered %q, want %q", name, worker, got, want)
	}
}

// wantOffer asserts what a cloud run that holds no lease is offered of the
// feature.
func wantOffer(t *testing.T, s *State, name, want string) {
	t.Helper()
	wantOfferTo(t, s, name, "cloud/run", want)
}

func wantNext(t *testing.T, s *State, worker, want string) {
	t.Helper()
	if got := next(t, s, worker); got != want {
		t.Fatalf("%s next = %q, want %q", worker, got, want)
	}
}

// wantValidate asserts Validate's findings: none for a valid state.
func wantValidate(t *testing.T, s *State, want ...string) {
	t.Helper()
	var got []string
	for _, err := range s.Validate() {
		got = append(got, err.Error())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Validate = %q, want %q", got, want)
	}
}

// slotWait is why a Plan step waits for a slot the holders have.
func slotWait(limit int, holders ...string) string {
	return fmt.Sprintf("waiting on an alters_relationships slot (limit %d), held by %s "+
		"(FLOW-05)", limit, strings.Join(holders, ", "))
}

// overLimit is Validate's finding for more holders than the limit.
func overLimit(limit int, holders ...string) string {
	return fmt.Sprintf("limit: %d features hold an alters_relationships slot, limit %d "+
		"(FLOW-05): %s", len(holders), limit, strings.Join(holders, ", "))
}

// TestAltersSlotHeldFromPlanClaim: a feature holds an alters_relationships
// slot from the claim of its Plan step until it is done, and at no other time
// (FLOW-05; cases 2 and 7 of the alters-slot design).
func TestAltersSlotHeldFromPlanClaim(t *testing.T) {
	a, b, c := approved("a", 2), approved("b", 3), approved("c", 4)
	d, e := altering("d", 1), feature("e", 5, "in_progress")
	declare(e, true)
	s := limited(state(a, b, c, d, e), 3)

	// Finished Design, reviewed or not, holds nothing: four such features at
	// limit 3 are valid, and each Plan is offered beside e, not yet designed.
	wantValidate(t, s)
	for _, f := range []*Feature{a, b, c} {
		wantNext(t, s, "cloud/run", f.Feature+" f2-plan")
		claimPlan(t, s, f)
	}

	// The claim holds: d, approved now, waits for the three.
	approve(d)
	wantOffer(t, s, "d", slotWait(3, "a", "b", "c"))
	wantValidate(t, s)

	// Only a Plan step waits: e's branch setup and its Design are offered
	// while every slot is held.
	wantOffer(t, s, "e", "f0-branch-setup")
	advance(e, 1)
	wantOffer(t, s, "e", "f1-design")

	// Release gives the slot back, and d, ranked ahead of c, is offered it.
	s.Release(c, plan(t, c), "cloud/c", now, "done-when failed")
	wantNext(t, s, "cloud/run", "d f2-plan")
	claimPlan(t, s, d)
	wantOffer(t, s, "c", slotWait(3, "d", "a", "b"))

	// Plan done holds.
	Complete(d, plan(t, d), "cloud/d", now)
	wantNext(t, s, "cloud/run", "d f3-implement")
	wantOffer(t, s, "c", slotWait(3, "d", "a", "b"))
	wantValidate(t, s)

	// The item done frees it: three held, the fourth waited.
	advance(d, 9)
	wantNext(t, s, "cloud/run", "c f2-plan")
	wantValidate(t, s)
}

// TestAltersSlotNoMutualHold: features that finish Design together do not
// hold each other, and a holder is never refused its own steps (cases 4, 5
// and 6).
func TestAltersSlotNoMutualHold(t *testing.T) {
	t.Run("both approved, neither planned", noMutualHoldBothApproved)
	t.Run("one in Implement, one finishes Design", noMutualHoldInImplement)
	t.Run("four finish Design", noMutualHoldFour)
}

// noMutualHoldBothApproved is case 4: the higher-ranked feature plans, and
// the other plans beside it at limit 3 or waits for it at limit 1.
func noMutualHoldBothApproved(t *testing.T) {
	for limit, second := range map[int]string{3: "f2-plan", 1: slotWait(1, "seam-specs")} {
		ss, dc := approved("seam-specs", 62), approved("driver-contract", 63)
		s := limited(state(ss, dc), limit)
		wantValidate(t, s)
		wantNext(t, s, "cloud/run", "seam-specs f2-plan")
		claimPlan(t, s, ss)
		wantOffer(t, s, "driver-contract", second)
		wantOfferTo(t, s, "seam-specs", "cloud/seam-specs", "f2-plan")
		wantValidate(t, s)
	}
}

// noMutualHoldInImplement is case 5: a feature in Implement goes on when
// another finishes Design, and that one plans once approved.
func noMutualHoldInImplement(t *testing.T) {
	ss, dc := approved("seam-specs", 62), toPhase("driver-contract", 63, 1)
	declare(dc, true)
	s := limited(state(ss, dc), 3)
	finishPlan(t, s, ss)
	advance(dc, 2)
	wantNext(t, s, "cloud/run", "seam-specs f3-implement")
	wantValidate(t, s)
	approve(dc)
	wantNext(t, s, "cloud/run", "seam-specs f3-implement")
	wantOffer(t, s, "driver-contract", "f2-plan")
	claimPlan(t, s, dc)
	wantOffer(t, s, "seam-specs", "f3-implement")
	wantValidate(t, s)
}

// noMutualHoldFour is case 6: Plan goes to the first three in FLOW-05 order
// and the fourth waits, naming them.
func noMutualHoldFour(t *testing.T) {
	fs := []*Feature{approved("a", 1), approved("b", 2), approved("c", 3),
		approved("d", 4)}
	s := limited(state(fs...), 3)
	wantValidate(t, s)
	for _, f := range fs[:3] {
		wantNext(t, s, "cloud/run", f.Feature+" f2-plan")
		claimPlan(t, s, f)
	}
	wantNext(t, s, "cloud/run", "none")
	wantOffer(t, s, "d", slotWait(3, "a", "b", "c"))
	wantValidate(t, s)
	for _, f := range fs[:3] {
		wantOfferTo(t, s, f.Feature, "cloud/"+f.Feature, "f2-plan")
		Complete(f, plan(t, f), "cloud/"+f.Feature, now)
		wantOffer(t, s, f.Feature, "f3-implement")
	}
	wantOffer(t, s, "d", slotWait(3, "a", "b", "c"))
	wantValidate(t, s)
}

// TestAltersSlotLimit: the limit is limits.alters_relationships, 1 when
// config.yaml does not set it, and a written value below 1 is an error
// (FLOW-05, FLOW-20, GLOBAL-10; cases 8 and 14, and "More holders than the
// limit").
func TestAltersSlotLimit(t *testing.T) {
	one, three := 1, 3
	for _, c := range []struct {
		name    string
		written *int
		limit   int
	}{{"absent", nil, 1}, {"1", &one, 1}, {"3", &three, 3}} {
		t.Run(c.name, func(t *testing.T) { altersLimitHolders(t, c.written, c.limit) })
		t.Run(c.name+", one in Implement", func(t *testing.T) {
			altersLimitInImplement(t, c.written, c.limit)
		})
	}
	t.Run("below 1", altersLimitBelowOne)
	t.Run("config.yaml", altersLimitLoaded)
}

// altersLimitHolders takes the holders from none to one over the limit. A
// Plan step is offered below the limit and waits from it on; only the claim
// past the limit, which wf next never offers, fails Validate, and it stops
// no holder.
func altersLimitHolders(t *testing.T, written *int, limit int) {
	var hs []*Feature
	for i := 0; i <= limit; i++ {
		hs = append(hs, approved(fmt.Sprintf("h%d", i), i+1))
	}
	s := state(append(slices.Clone(hs), approved("w", 9))...)
	s.Config.Limits.AltersRelationships = written
	var held []string
	for _, h := range hs {
		wantValidate(t, s)
		want := "f2-plan"
		if len(held) >= limit {
			want = slotWait(limit, held...)
		}
		wantOffer(t, s, "w", want)
		wantOffer(t, s, h.Feature, want)
		claimPlan(t, s, h)
		held = append(held, h.Feature)
	}
	wantValidate(t, s, overLimit(limit, held...))
	wantOffer(t, s, "w", slotWait(limit, held...))
	for _, h := range hs {
		wantOfferTo(t, s, h.Feature, "cloud/"+h.Feature, "f2-plan")
	}
}

// altersLimitInImplement is case 8 at limit 1, written or not: A in Implement
// goes on and B, approved, waits for A. At limit 3 B plans beside it.
func altersLimitInImplement(t *testing.T, written *int, limit int) {
	a, b := approved("a", 1), approved("b", 2)
	s := state(a, b)
	s.Config.Limits.AltersRelationships = written
	finishPlan(t, s, a)
	wantNext(t, s, "cloud/run", "a f3-implement")
	want := "f2-plan"
	if limit == 1 {
		want = slotWait(1, "a")
	}
	wantOffer(t, s, "b", want)
	wantValidate(t, s)
}

// altersLimitBelowOne is case 14: a written 0 or -2 is the one finding, with
// no count of holders beside it, and the reason a Plan step waits names the
// value. A holder still goes on.
func altersLimitBelowOne(t *testing.T) {
	for _, n := range []int{0, -2} {
		a, b, w := approved("a", 1), approved("b", 2), approved("w", 3)
		s := limited(state(a, b, w), n)
		finding := fmt.Sprintf("config: limits.alters_relationships %d is below 1", n)
		wait := fmt.Sprintf("waiting on limits.alters_relationships %d is below 1 "+
			"(FLOW-05)", n)
		wantValidate(t, s, finding)
		wantOffer(t, s, "w", wait)
		finishPlan(t, s, a)
		claimPlan(t, s, b)
		wantValidate(t, s, finding)
		wantOffer(t, s, "w", wait)
		wantOffer(t, s, "a", "f3-implement")
	}
}

// altersLimitLoaded: config.yaml's absent key reads as 1, and differs from a
// written 0.
func altersLimitLoaded(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		limits, offered string
		findings        []string
	}{
		{"", slotWait(1, "a"), nil},
		{"limits: {in_progress: 5}\n", slotWait(1, "a"), nil},
		{"limits: {alters_relationships: 3}\n", "f2-plan", nil},
		{"limits: {alters_relationships: 0}\n",
			"waiting on limits.alters_relationships 0 is below 1 (FLOW-05)",
			[]string{"config: limits.alters_relationships 0 is below 1"}},
	} {
		_ = os.WriteFile(filepath.Join(dir, "config.yaml"),
			[]byte("run_budget: {steps: 8}\n"+c.limits), 0o644)
		s, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		a, b := approved("a", 1), approved("b", 2)
		s.Features = []*Feature{a, b}
		finishPlan(t, s, a)
		wantOffer(t, s, "b", c.offered)
		wantValidate(t, s, c.findings...)
	}
}

// TestAltersSlotAcrossReturns: a holder keeps its slot through a return to
// Design while it has a finished plan, and the queue's value decides the rest
// (FLOW-06; cases 9, 10 and 12, and the flip of "More holders than the
// limit").
func TestAltersSlotAcrossReturns(t *testing.T) {
	t.Run("from Implement or later", returnKeepsSlot)
	t.Run("from Plan", returnFromPlanFreesSlot)
	t.Run("true to false", returnToFalseFreesSlot)
	t.Run("false to true", returnToTrueOverLimit)
}

// returnKeepsSlot is case 9: the done Plan step survives Back, and the
// holder's second plan needs no second slot.
func returnKeepsSlot(t *testing.T) {
	for _, phase := range []int{3, 4, 6} {
		a, b := approved("a", 1), approved("b", 2)
		s := state(a, b)
		finishPlan(t, s, a)
		advance(a, phase)
		Back(a, "cloud/a", now, "plan not viable", 1)
		wantOffer(t, s, "a", "f1-design-r2")
		wantOffer(t, s, "b", slotWait(1, "a"))
		wantValidate(t, s)
		advance(a, 2)
		approve(a)
		wantOffer(t, s, "a", "f2-plan-r2")
		wantOffer(t, s, "b", slotWait(1, "a"))
		wantValidate(t, s)
	}
}

// returnFromPlanFreesSlot is case 10: Back drops the unfinished Plan step and
// the slot with it, and the returned feature waits in its turn: at its next
// Plan step, not in the Design it returned to.
func returnFromPlanFreesSlot(t *testing.T) {
	a, b := approved("a", 1), approved("b", 2)
	s := state(a, b)
	claimPlan(t, s, a)
	wantOffer(t, s, "b", slotWait(1, "a"))
	Back(a, "cloud/a", now, "the design cannot be planned", 1)
	wantOffer(t, s, "b", "f2-plan")
	wantValidate(t, s)
	claimPlan(t, s, b)
	wantOffer(t, s, "a", "f1-design-r2")
	advance(a, 2)
	approve(a)
	wantOffer(t, s, "a", slotWait(1, "b"))
	wantValidate(t, s)
}

// returnToFalseFreesSlot is case 12: a redesign that changes true to false
// frees the slot, and the feature then plans beside the next holder.
func returnToFalseFreesSlot(t *testing.T) {
	a, b := approved("a", 1), approved("b", 2)
	s := state(a, b)
	finishPlan(t, s, a)
	Back(a, "cloud/a", now, "the seam stays as it is", 1)
	wantOffer(t, s, "b", slotWait(1, "a"))
	declare(a, false)
	wantOffer(t, s, "b", "f2-plan")
	claimPlan(t, s, b)
	advance(a, 2)
	approve(a)
	wantOffer(t, s, "a", "f2-plan-r2")
	wantValidate(t, s)
}

// returnToTrueOverLimit is the flip: a feature with a done Plan step whose
// value becomes true while every slot is held is a holder over the limit.
// Validate names every holder, each holder's next step is still offered, and
// no other altering feature's Plan step is. Putting the value back, as
// wf-design's step 5 says, makes the state valid again.
func returnToTrueOverLimit(t *testing.T) {
	a, f, c := approved("a", 1), toPhase("f", 2, 3), approved("c", 3)
	declare(f, false)
	s := state(a, f, c)
	finishPlan(t, s, a)
	Back(f, "cloud/f", now, "the change reaches the relay's role", 1)
	wantValidate(t, s)
	declare(f, true)
	wantValidate(t, s, overLimit(1, "a", "f"))
	wantOffer(t, s, "a", "f3-implement")
	wantOffer(t, s, "f", "f1-design-r2")
	wantOffer(t, s, "c", slotWait(1, "a", "f"))
	declare(f, false)
	wantValidate(t, s)
	wantOffer(t, s, "c", slotWait(1, "a"))
}

// TestAltersSlotExpiredClaim is case 11: an abandoned claim keeps the slot,
// with no clock read, until its Plan step is taken up again or released.
func TestAltersSlotExpiredClaim(t *testing.T) {
	a, b := approved("a", 1), approved("b", 2)
	s := state(a, b)
	claimPlan(t, s, a)
	later := now.Add(3 * time.Hour)
	if got := offerAt(s, "b", "cloud/run", later); got != slotWait(1, "a") {
		t.Fatalf("b past a's expiry: offered %q, want the wait for a's slot", got)
	}
	if got := offerAt(s, "a", "cloud/run", later); got != "f2-plan" {
		t.Fatalf("a past its expiry: offered %q, want any run to take up f2-plan", got)
	}
	wantValidate(t, s)
	s.Release(a, plan(t, a), "cloud/run", later, "abandoned")
	if got := offerAt(s, "b", "cloud/run", later); got != "f2-plan" {
		t.Fatalf("b after the release: offered %q, want f2-plan", got)
	}
}

// TestAltersSlotSkipsProcessItems is case 13: a process item that declares
// alters_relationships: true neither holds a slot nor waits for one (FLOW-05
// counts features; holds is how a process item stops other work).
func TestAltersSlotSkipsProcessItems(t *testing.T) {
	p, f, g := process("p", 1, "in_progress"), approved("f", 2), approved("g", 3)
	declare(p, true)
	advance(p, 2)
	s := state(p, f, g)
	claimPlan(t, s, f)
	wantNext(t, s, "cloud/run", "p x2-implement")
	s.Claim(p, &p.Steps[len(p.Steps)-1], "cloud/p", now, true)
	wantOffer(t, s, "g", slotWait(1, "f"))
	wantValidate(t, s)
	s.Release(f, plan(t, f), "cloud/f", now, "done-when failed")
	wantOffer(t, s, "g", "f2-plan")
	// Nor does a done Plan step left on it, as when a planned feature's kind
	// is changed by hand, make it a holder.
	p.Steps = append(p.Steps, Step{ID: "f2-plan", Phase: 2, Affinity: "cloud",
		Status: "done"})
	wantOffer(t, s, "g", "f2-plan")
	wantValidate(t, s)
}

func TestIsPlan(t *testing.T) {
	for id, want := range map[string]bool{"f2-plan": true, "f2-plan-r3": true,
		"f2-design-review": false, "f2-plan-macos-vz": false, "f3-plan": false,
		"x2-implement": false, "p2-plan": false} {
		if got := IsPlan(&Step{ID: id}); got != want {
			t.Errorf("IsPlan(%s) = %v", id, got)
		}
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
	a.PR = &PR{Number: 7}
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
			"a": "waiting on the human's merge of PR #7 (FLOW-08)",
			"b": "waiting on the PR slot, held by a (colliding: the queue's " +
				"repository has no area map) (FLOW-05)"}
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
	t.Run("process", mergeWaitProcess)
	t.Run("feature", mergeWaitFeature)
	t.Run("no PR", mergeWaitNoPR)
}

// mergeWaitProcess is the 2026-11-10 incident's shape: a process item at
// x3-merge ahead of an in-progress fix with a ready cloud step.
func mergeWaitProcess(t *testing.T) {
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

	// An attended run on the item gets no pick and the reason.
	if p, blocked := state(merging).NextFor("merging", "cloud", now); p != nil ||
		!slices.Equal(blocked, want) {
		t.Fatalf("NextFor = %+v, %+v; want merging on its merge", p, blocked)
	}
}

// mergeWaitFeature: a feature at f7-merge, its -r2 form too, yields within
// class 4: to an in-progress feature behind it, or to a queued one, which
// starts.
func mergeWaitFeature(t *testing.T) {
	for _, id := range []string{"f7-merge", "f7-merge-r2"} {
		a := mergeWaiting(feature("a", 1, "in_progress"), 7)
		a.Steps[len(a.Steps)-1].ID = id
		b := feature("b", 2, "in_progress")
		if got := next(t, state(a, b), "cloud"); got != "b f0-branch-setup" {
			t.Errorf("%s: next = %q, want b past a's merge", id, got)
		}
		q := feature("q", 2, "queued")
		s := state(a, q)
		inProgress(s, 2)
		if p, _ := s.Next("cloud", now); p == nil || p.Feature != q || !p.Starts {
			t.Errorf("%s: pick = %+v, want q started past a's merge", id, p)
		}
	}
}

// mergeWaitNoPR: with no PR recorded the reason is exact, for a nil PR and a
// PR 0.
func mergeWaitNoPR(t *testing.T) {
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

// limitTexts are config.yaml's limits lines for a limit that is absent,
// written at its default, written higher, and written below 1 twice.
func limitTexts(key string, def int) []string {
	return []string{"", fmt.Sprintf("limits: {%s: %d}\n", key, def),
		fmt.Sprintf("limits: {%s: %d}\n", key, def+2),
		fmt.Sprintf("limits: {%s: 0}\n", key), fmt.Sprintf("limits: {%s: -1}\n", key)}
}

// loadConfig loads a state whose config.yaml is the run budget and text.
func loadConfig(t *testing.T, text string) *State {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("run_budget: {steps: 8}\n"+text), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestInProgressLimitDefault: limits.in_progress is 5 when config.yaml does
// not write it, a written value is used as written, and one below 1 is an
// error under which nothing starts. Absent or 0 is no longer "no limit"
// (FLOW-05; case 16 of the loop-parallel design).
func TestInProgressLimitDefault(t *testing.T) {
	texts := limitTexts("in_progress", 5)
	for i, c := range []struct {
		limit    int
		started  string
		findings []string
	}{
		{5, "none", nil},
		{5, "none", nil},
		{7, "q f0-branch-setup", nil},
		{0, "none", []string{"config: limits.in_progress 0 is below 1"}},
		{-1, "none", []string{"config: limits.in_progress -1 is below 1"}},
	} {
		s := loadConfig(t, texts[i])
		if got := s.inProgressLimit(); got != c.limit {
			t.Fatalf("%q: the limit is %d, want %d", texts[i], got, c.limit)
		}
		for n := 1; n <= 5; n++ {
			f := feature(fmt.Sprintf("f%d", n), n, "in_progress")
			f.Steps[0].Affinity = "human"
			s.Features = append(s.Features, f)
		}
		s.Features = append(s.Features, feature("q", 6, "queued"))
		wantNext(t, s, "cloud", c.started)
		wantValidate(t, s, c.findings...)
	}
	// One more than the limit is the finding it was.
	s := loadConfig(t, "")
	for n := 1; n <= 6; n++ {
		s.Features = append(s.Features, feature(fmt.Sprintf("f%d", n), n, "in_progress"))
	}
	wantValidate(t, s,
		"limit: 6 features in progress, limit 5 (FLOW-05): f1, f2, f3, f4, f5, f6")
}

// TestWaitingLimitDefault: limits.waiting_on_human is 8 when config.yaml does
// not write it, a written value is used as written, and one below 1 is an
// error; no value means "no cap" (FLOW-05; case 31).
func TestWaitingLimitDefault(t *testing.T) {
	texts := limitTexts("waiting_on_human", 8)
	for i, c := range []struct {
		limit    int
		findings []string
	}{
		{8, nil},
		{8, nil},
		{10, nil},
		{0, []string{"config: limits.waiting_on_human 0 is below 1"}},
		{-1, []string{"config: limits.waiting_on_human -1 is below 1"}},
	} {
		s := loadConfig(t, texts[i])
		if got := s.waitingLimit(); got != c.limit {
			t.Fatalf("%q: the limit is %d, want %d", texts[i], got, c.limit)
		}
		wantValidate(t, s, c.findings...)
	}
}

// declaredState is a state whose config names the queue's own repository.
func declaredState(fs ...*Feature) *State {
	s := state(fs...)
	s.Config.Repository = "o/here"
	return s
}

// TestValidateDeclaredFields: Validate's checks of what an item declares
// (FLOW-02, FLOW-03): repos, areas, declared and collides_with.
func TestValidateDeclaredFields(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(f, p *Feature)
		want []string
	}{
		{"nothing declared", func(f, p *Feature) {}, nil},
		{"every field", func(f, p *Feature) {
			f.Repos = []string{"o/here", "o/specs"}
			f.Areas = &[]string{"relay", "book"}
			f.Seams, f.RulesChanged = []string{"relay-claim"}, []string{"GLOBAL-08"}
			f.Declared, f.CollidesWith = "4c1d9e2", []string{"other"}
			p.Repos, p.Areas = []string{"o/here"}, &[]string{}
			p.Declared = strings.Repeat("a", 40)
		}, nil},
		{"repos that are not owner/name", func(f, p *Feature) {
			f.Repos = []string{"o/here", "specs", "o/a/b", "/x", "o/"}
		}, []string{`a: repos entry "specs" is not owner/name`,
			`a: repos entry "o/a/b" is not owner/name`,
			`a: repos entry "/x" is not owner/name`,
			`a: repos entry "o/" is not owner/name`}},
		{"a repository twice", func(f, p *Feature) {
			f.Repos = []string{"o/here", "o/specs", "o/here"}
		}, []string{"a: repos names o/here twice"}},
		{"a process item homed elsewhere", func(f, p *Feature) {
			p.Repos = []string{"o/specs", "o/here"}
		}, []string{"p: a process item is homed in the queue's repository, not o/specs " +
			"(an item homed in another repository is tracked)"}},
		{"an area twice", func(f, p *Feature) {
			f.Areas = &[]string{"relay", "book", "relay"}
		}, []string{"a: areas names relay twice"}},
		{"declared is not a commit", func(f, p *Feature) {
			f.Declared, p.Declared = "HEAD", "4C1D9E2"
		}, []string{`a: declared "HEAD" is not a commit id`,
			`p: declared "4C1D9E2" is not a commit id`}},
		{"pr.head is not a commit", func(f, p *Feature) {
			f.PR = &PR{Number: 7, CI: "green", Head: "HEAD"}
			p.PR = &PR{Number: 8, CI: "green", Head: "0a1b2c3"}
		}, []string{`a: pr.head "HEAD" is not a commit id`}},
		{"seams with no declared", func(f, p *Feature) {
			f.Seams = []string{"relay-claim"}
		}, []string{"a: seams and rules_changed are what wf declare copies from the " +
			"design, and declared is not set"}},
		{"rules_changed with no declared", func(f, p *Feature) {
			f.RulesChanged = []string{"GLOBAL-08"}
		}, []string{"a: seams and rules_changed are what wf declare copies from the " +
			"design, and declared is not set"}},
		{"collides_with a name not in the queue", func(f, p *Feature) {
			f.CollidesWith = []string{"other", "gone", "a"}
		}, []string{`a: collides_with "gone", which is not another item in the queue`,
			`a: collides_with "a", which is not another item in the queue`}},
	} {
		f, o := feature("a", 2, "queued"), feature("other", 3, "queued")
		p := process("p", 1, "queued")
		c.set(f, p)
		s := declaredState(f, o, p)
		var got []string
		for _, err := range s.Validate() {
			got = append(got, err.Error())
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}

	// config.yaml's repository has no default: an item that names repos
	// needs it, and an item that names none does not.
	f, o := feature("a", 1, "queued"), feature("other", 2, "queued")
	f.Repos = []string{"o/here"}
	s := state(f, o)
	wantValidate(t, s, "config: repository is not set, and a names repos")
}

// TestValidateTracked: an item whose first repository is not the queue's own
// is tracked; it is not a process item, holds no worker, and its round is
// tracked. An item homed here that also works elsewhere is not tracked.
func TestValidateTracked(t *testing.T) {
	away := func() *Feature {
		f := &Feature{Feature: "m1", Priority: 1, Status: "queued", Round: "tracked",
			Phase: 1, Repos: []string{"o/relay"}}
		return f
	}
	f := away()
	s := declaredState(f)
	if !s.Tracked(f) {
		t.Fatal("an item homed in another repository is not tracked")
	}
	wantValidate(t, s)

	f.Seams = []string{"relay-claim"}
	wantValidate(t, s) // wf new -seams records them: wf declare cannot read its design.

	f.Round = "feature"
	wantValidate(t, s, `m1: a tracked item's round is tracked, not "feature"`)

	f = away()
	f.Holds = []string{"cloud"}
	wantValidate(t, declaredState(f), "m1: only a process item holds workers")

	both := feature("s1", 1, "queued")
	both.Repos = []string{"o/here", "o/specs"}
	s = declaredState(both)
	if s.Tracked(both) || s.Tracked(feature("x", 2, "queued")) {
		t.Fatal("an item homed here is tracked")
	}
	wantValidate(t, s)
	both.Round = "tracked"
	wantValidate(t, s, "s1: round tracked is for an item homed in another repository, "+
		"and this one is homed in the queue's")
}

// TestAdoptsValidate: adopts marks a reconciliation item (LOOP-07): the
// consumable and the version are both set, and the item is a feature.
func TestAdoptsValidate(t *testing.T) {
	f, p := feature("a", 2, "queued"), process("p", 1, "queued")
	s := declaredState(f, p)
	f.Adopts = &Adopts{Consumable: "buildwarden-relay", Version: "v0.3.0"}
	wantValidate(t, s)
	f.Adopts = &Adopts{Consumable: "buildwarden-relay"}
	wantValidate(t, s, "a: adopts needs both its consumable and its version")
	f.Adopts = &Adopts{Version: "v0.3.0"}
	wantValidate(t, s, "a: adopts needs both its consumable and its version")
	f.Adopts = nil
	p.Adopts = &Adopts{Consumable: "buildwarden-relay", Version: "v0.3.0"}
	wantValidate(t, s, "p: only a feature adopts a release (LOOP-07)")
}

// TestUrgentValidate: urgent is the human worker who marked the item, on a
// feature the waiting limit could refuse: not a process item, a tracked item
// or one that follows an item (FLOW-05). Nothing is checked about what the
// item fixes, and the mark is not an error on an item in progress or done.
func TestUrgentValidate(t *testing.T) {
	const notHuman = ": only a human marks an item urgent (FLOW-05)"
	f, parent := feature("a", 2, "queued"), feature("parent", 3, "done")
	p := process("p", 1, "queued")
	away := &Feature{Feature: "m1", Priority: 4, Status: "queued", Round: "tracked",
		Phase: 1, Repos: []string{"o/relay"}}
	s := declaredState(f, parent, p, away)
	for _, ok := range []string{"human/jeff", "human"} {
		f.Urgent = ok
		for _, status := range []string{"queued", "in_progress", "done"} {
			f.Status = status
			wantValidate(t, s)
		}
	}
	f.Status = "queued"
	for _, bad := range []string{"cloud/tick-1", "host:macos-vz/s", "jeff", "true"} {
		f.Urgent = bad
		wantValidate(t, s, "a: urgent names "+bad+notHuman)
	}
	f.Urgent, f.Follows = "human/jeff", "parent"
	wantValidate(t, s, "a: urgent, and it follows parent: "+
		"the waiting limit never refuses a fix (FLOW-05)")
	f.Urgent, f.Follows = "", ""
	p.Urgent = "human/jeff"
	wantValidate(t, s,
		"p: urgent on a process item: the waiting limit never refuses one (FLOW-05)")
	p.Urgent, away.Urgent = "", "human/jeff"
	wantValidate(t, s,
		"m1: urgent on a tracked item: the waiting limit never refuses one (FLOW-05)")
}

// queueFile is the text of the item's queue file under dir.
func queueFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "queue", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestNewFieldsRoundTrip: the fields an item declares, the urgent mark and
// pr.head survive a save and a load, and an item that has none of them is
// written as it was before they existed.
func TestNewFieldsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "queue"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("run_budget: {steps: 8}\nrepository: o/here\n"), 0o644)
	s := &State{Dir: dir}
	plain, full := feature("plain", 1, "queued"), feature("full", 2, "queued")
	full.Repos = []string{"o/here", "o/specs"}
	full.Seams, full.RulesChanged = []string{"relay-claim"}, []string{"GLOBAL-08", "TEST-05"}
	full.Declared, full.CollidesWith = "4c1d9e2", []string{"plain"}
	full.Adopts = &Adopts{Consumable: "buildwarden-relay", Version: "v0.3.0"}
	full.Urgent = "human/jeff"
	full.PR = &PR{Number: 12, CI: "green", Head: "0a1b2c3"}
	for _, f := range []*Feature{plain, full} {
		if err := s.Save(f); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) string { return queueFile(t, dir, name) }
	plainText, fullText := read("plain"), read("full")
	for _, key := range []string{"repos", "areas", "seams", "rules_changed", "declared",
		"adopts", "collides_with", "urgent", "head"} {
		if strings.Contains(plainText, key+":") {
			t.Errorf("an item with no %s is written with the key:\n%s", key, plainText)
		}
		if key != "areas" && !strings.Contains(fullText, key+":") {
			t.Errorf("an item with %s is written without the key:\n%s", key, fullText)
		}
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.Repository != "o/here" {
		t.Fatalf("config.yaml's repository loaded as %q", got.Config.Repository)
	}
	g, err := got.Find("full")
	if err != nil {
		t.Fatal(err)
	}
	if g.Urgent != "human/jeff" || g.PR == nil || g.PR.Head != "0a1b2c3" {
		t.Fatalf("loaded urgent %q, pr %+v", g.Urgent, g.PR)
	}
	// Saved again, the loaded item is the file it was loaded from.
	if err := got.Save(g); err != nil {
		t.Fatal(err)
	}
	if again := read("full"); again != fullText {
		t.Fatalf("a load and a save changed the file:\n%s\nwas:\n%s", again, fullText)
	}
	wantValidate(t, got)
}

// TestClassAdopts: a reconciliation item, which the queue marks with adopts,
// is in class 3 with the fixes (FLOW-05, LOOP-07).
func TestClassAdopts(t *testing.T) {
	for _, c := range []struct {
		kind, status, follows string
		phase, want           int
	}{
		{"", "queued", "", 0, 3},
		{"", "queued", "a", 0, 3},
		{"", "in_progress", "", 2, 3},
		{"", "in_progress", "", 3, 1},
		{"", "in_progress", "", 7, 3},
		{"", "done", "", 8, 0},
		{"process", "queued", "", 1, 2},
	} {
		f := adopting(&Feature{Kind: c.kind, Status: c.status, Follows: c.follows,
			Phase: c.phase})
		if got := f.Class(); got != c.want {
			t.Errorf("Class(%+v) with adopts = %d, want %d", c, got, c.want)
		}
	}
	// It is taken ahead of new functionality, whatever the priorities.
	newer, adoption := feature("newer", 1, "queued"), feature("adoption", 2, "queued")
	wantNext(t, state(newer, adopting(adoption)), "cloud", "adoption f0-branch-setup")
}

// TestTrackedRound: an item homed in another repository is tracked. wf seeds
// it two human steps, any claim starts it, it is outside every limit and is
// never in implementation (FLOW-03, FLOW-04, FLOW-05).
func TestTrackedRound(t *testing.T) {
	away := func(name string, prio int) *Feature {
		return &Feature{Feature: name, Priority: prio, Repos: []string{"o/relay"}}
	}
	f := away("m1", 9)
	s := declaredState()
	if _, err := s.Add(f, now, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	if adopting(away("m2", 2)).Class() != 3 || fixing(away("m3", 3), "m1").Class() != 3 {
		t.Fatal("a tracked item is not classed as the feature it is")
	}
	trackedSeeded(t, s, f)
	trackedOutsideLimits(t, s, f)
	trackedWalk(t, s, f)
}

func trackedSeeded(t *testing.T, s *State, f *Feature) {
	t.Helper()
	if f.Round != "tracked" || f.Phase != 1 || ids(f) != "t1-work:pending:human" ||
		PhaseName(f) != "work" {
		t.Fatalf("seeded %s/%d %s: %+v", f.Round, f.Phase, PhaseName(f), f.Steps)
	}
	wantValidate(t, s)
	if f.Class() != 4 {
		t.Fatalf("a queued tracked item is in class %d", f.Class())
	}
}

// trackedOutsideLimits: five undeclared features in progress and items
// waiting past the limit neither stop the tracked item's start nor count it.
func trackedOutsideLimits(t *testing.T, s *State, f *Feature) {
	t.Helper()
	var fs []*Feature
	for n := 1; n <= 5; n++ {
		fs = append(fs, working(fmt.Sprintf("f%d", n), n, 3))
	}
	s.Features = append(s.Features, parked(fs...)...)
	s.Features = append(s.Features, waiters(8)[1], waiters(8)[5])
	s.Features[len(s.Features)-2].Priority = 30
	s.Features[len(s.Features)-1].Priority = 31
	three := 3
	s.Config.Limits.WaitingOnHuman = &three
	wantNext(t, s, "cloud", "none")
	pick, _ := s.NextFor("m1", "human/jeff", now)
	if pick == nil || pick.Feature != f || pick.Step.ID != "t1-work" || !pick.Starts {
		t.Fatalf("the human is offered %+v, want m1's t1-work, starting it", pick)
	}
	if why := s.Guard(f, pick.Step, "human/jeff"); why != "" {
		t.Fatalf("Guard refuses a tracked item: %s", why)
	}
	s.Claim(f, pick.Step, "human/jeff", now, false)
	if f.Status != "in_progress" || f.Class() != 4 {
		t.Fatalf("after its first claim m1 is %s, class %d", f.Status, f.Class())
	}
	wantValidate(t, s)
	wantWaiting(t, s, 7)
	for _, l := range limits {
		if slices.Contains(s.holders(l), f) {
			t.Errorf("a tracked item holds %s", l.what)
		}
	}
}

// trackedWalk completes the tracked item's two steps, with no check.
func trackedWalk(t *testing.T, s *State, f *Feature) {
	t.Helper()
	Complete(f, &f.Steps[0], "human/jeff", now)
	if f.Phase != 2 || ids(f) != "t1-work:done:human t2-merge:pending:human" ||
		PhaseName(f) != "merge" || f.Class() != 4 {
		t.Fatalf("after its work m1 is %s/%d, class %d: %s", f.Round, f.Phase, f.Class(),
			ids(f))
	}
	wantOfferTo(t, s, "m1", "human/jeff", "t2-merge")
	Complete(f, &f.Steps[1], "human/jeff", now)
	if f.Status != "done" {
		t.Fatalf("after its merge m1 is %s", f.Status)
	}
	wantValidate(t, s)
}

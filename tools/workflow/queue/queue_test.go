package queue

import (
	"fmt"
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
	want := "f0-branch-setup f1-design f2-plan f2-design-review f3-implement f4-review " +
		"f5-guide f6-merge-main f6-consolidate f6-open-pr f7-merge f8-close"
	if got := strings.Join(walk(t, f), " "); got != want {
		t.Fatalf("steps:\n got %s\nwant %s", got, want)
	}
	gates := map[string]bool{"f1-design": true, "f2-design-review": true,
		"f6-open-pr": true, "f7-merge": true, "f8-close": true}
	for i := range f.Steps {
		st := &f.Steps[i]
		switch st.ID {
		case "f2-plan", "f6-merge-main", "f6-consolidate":
			if IsGate(f, st) {
				t.Fatalf("%s is not its phase's gate", st.ID)
			}
		}
		if gates[st.ID] && !IsGate(f, st) {
			t.Fatalf("%s is its phase's gate", st.ID)
		}
	}
	if f.Steps[3].Affinity != "human" || f.Steps[3].Needs[0] != "f2-plan" {
		t.Fatalf("design review %+v: a human step after Plan", f.Steps[3])
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
	if !IsGate(f, plan) {
		t.Fatal("with the review skipped, Plan is the phase's gate")
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
			if _, err := f.Step(fmt.Sprintf("f2-design-review-r%d", i+1)); err != nil {
				t.Fatalf("return %d: a return to Design reseeds the review: %v",
					i, err)
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
			t.Fatalf("phase %d: human next = %q, want the provisioning step", phase, got)
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
	advance(f, 6)
	open, _ = f.Step("f6-open-pr-r2")
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

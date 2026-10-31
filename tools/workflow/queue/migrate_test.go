package queue

import (
	"strings"
	"testing"
)

// legacy builds a parallel-round feature at the given phase, as the old
// tooling left it: earlier phases' steps done, the phase's step pending.
func legacy(name string, prio, phase int) *Feature {
	f := &Feature{Feature: name, Priority: prio, Status: "in_progress", Round: "parallel",
		Phase: phase, Budget: Budget{Estimate: Count{Runs: 1, Steps: 9}}}
	names := []string{"branch-setup", "design", "plan", "implement", "review", "guide",
		"publish"}
	for p := 0; p <= phase; p++ {
		st := Step{ID: StepID("parallel", p, names[p]), Phase: p, Affinity: "cloud",
			Status: "done"}
		if p == phase {
			st.Status = "pending"
		}
		f.Steps = append(f.Steps, st)
	}
	return f
}

func ids(f *Feature) string {
	var out []string
	for _, st := range f.Steps {
		out = append(out, st.ID+":"+st.Status+":"+st.Affinity)
	}
	return strings.Join(out, " ")
}

func TestMigrate(t *testing.T) {
	review := legacy("review", 2, 4)
	impl := legacy("impl", 3, 3)
	waiting := legacy("waiting", 4, 6)
	waiting.Steps[6].Status = "done"
	waiting.Steps = append(waiting.Steps, Step{ID: "p6-merge-round", Phase: 6,
		Affinity: "cloud", Status: "pending"})
	queued := legacy("queued", 5, 0)
	queued.Status = "queued"
	done := legacy("done", 6, 6)
	done.Round, done.Phase, done.Status = "merge", 8, "done"
	proc := process("rules", 1, "in_progress")
	Complete(proc, &proc.Steps[0], "human/jeff", now)
	proc.Steps[1].ID, proc.Steps[1].Affinity = "x2-implement", "human"
	proc.Steps = append(proc.Steps, Step{ID: "x3-sign-off", Phase: 3, Affinity: "human",
		Status: "pending"})

	s := state(review, impl, waiting, queued, done, proc)
	changed, err := s.Migrate("human/jeff", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 5 {
		t.Fatalf("changed %d items, want 5 (all but the done one)", len(changed))
	}
	for f, want := range map[*Feature]string{
		review: "p0-branch-setup:done:cloud p1-design:done:cloud p2-plan:done:cloud " +
			"p3-implement:done:cloud f4-review:pending:cloud",
		impl: "p0-branch-setup:done:cloud p1-design:done:cloud p2-plan:done:cloud " +
			"f3-implement:pending:cloud f3-design-review:pending:human",
		waiting: "p6-publish:done:cloud f6-merge-main:pending:cloud " +
			"f6-consolidate:pending:cloud f6-open-pr:pending:cloud",
		queued: "f0-branch-setup:pending:cloud",
		proc:   "x1-design:done:human x2-implement:pending:cloud x3-merge:pending:cloud",
	} {
		if got := ids(f); !strings.HasSuffix(got, want) {
			t.Errorf("%s steps:\n got %s\nwant suffix %s", f.Feature, got, want)
		}
		if f.Round != "feature" && f != proc {
			t.Errorf("%s round %q", f.Feature, f.Round)
		}
	}
	if impl.Steps[3].Needs[0] != "f3-design-review" {
		t.Errorf("impl implement needs %v, want the design review", impl.Steps[3].Needs)
	}
	if done.Round != "merge" {
		t.Error("a finished item keeps its round as history")
	}
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate after migrate: %v", errs)
	}

	inMerge := legacy("merging", 7, 6)
	inMerge.Round = "merge"
	if _, err := state(inMerge).Migrate("human/jeff", now); err == nil {
		t.Fatal("migrated an in-progress merge-round item")
	}
}

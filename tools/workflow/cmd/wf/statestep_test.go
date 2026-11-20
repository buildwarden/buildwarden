package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// TestStateStep: a human records a process item's state step with wf
// state-step; before Close it is the field alone, and in Close the step is
// added at once and x4-close waits for it; -clear takes it back (FLOW-03;
// merge-confirm-attended, Decision 4, a6).
func TestStateStep(t *testing.T) {
	work := codeRepo(t, "px")
	dir := stateDir(t, here)
	queued(t, dir, "fx", "2")
	atProcessMerge(t, dir, work, "px", 81)
	set := func(worker, name string, more ...string) []string {
		return append([]string{"-worker", worker, "-feature", name}, more...)
	}
	refused(t, dir, "only a human records a state step", "state-step",
		set("cloud/run", "px", "-note", "Update README.md.")...)
	refused(t, dir, "fx is not a process item", "state-step",
		set("human/jeff", "fx", "-note", "Update README.md.")...)
	refused(t, dir, "state-step needs -note", "state-step", set("human/jeff", "px")...)

	// Before Close: the field alone.
	must(t, dir, "state-step", set("human/jeff", "px", "-note", "Update README.md.")...)
	f := item(t, dir, "px")
	if f.StateStep != "Update README.md." || hasStep(f, "x4-state") {
		t.Fatalf("before Close: state_step %q, steps %v", f.StateStep, ids(f))
	}
	if got := lastEvent(t, dir, "px"); got != "human/jeff: state step recorded: "+
		"Update README.md." {
		t.Fatalf("history %q", got)
	}
	must(t, dir, "state-step", set("human/jeff", "px", "-clear")...)
	if f := item(t, dir, "px"); f.StateStep != "" {
		t.Fatalf("-clear left %q", f.StateStep)
	}
	must(t, dir, "state-step", set("human/jeff", "px", "-note", "Update README.md.")...)

	// The human merges and confirms; the seeded x4-state is offered to them.
	mergePR(t, work, "px", 81)
	must(t, dir, "done", "-repo", work, "-worker", "human/jeff", "-feature", "px",
		"-step", "x3-merge")
	if got := must(t, dir, "next", "-worker", "human/jeff"); !strings.HasPrefix(got,
		"next\tpx\tx4-state\thuman") {
		t.Fatalf("wf next after the confirmation = %q, want x4-state", got)
	}

	// In Close: a new note updates the step, and adds none.
	must(t, dir, "state-step", set("human/jeff", "px", "-note", "Update README.md:202.")...)
	f = item(t, dir, "px")
	if st, _ := f.Step("x4-state"); st.Note != "Update README.md:202." ||
		strings.Count(strings.Join(ids(f), " "), "x4-state") != 1 {
		t.Fatalf("in Close: %v, x4-state %+v", ids(f), st)
	}
	// -clear removes the pending step and x4-close's need on it.
	must(t, dir, "state-step", set("human/jeff", "px", "-clear")...)
	f = item(t, dir, "px")
	if c, _ := f.Step("x4-close"); hasStep(f, "x4-state") || len(c.Needs) != 0 ||
		f.StateStep != "" {
		t.Fatalf("-clear in Close: %v, x4-close needs %v", ids(f), c.Needs)
	}
	// Recorded in Close: added at once, and x4-close waits for it.
	must(t, dir, "state-step", set("human/jeff", "px", "-note", "Update README.md.")...)
	f = item(t, dir, "px")
	if c, _ := f.Step("x4-close"); !slices.Equal(c.Needs, []string{"x4-state"}) {
		t.Fatalf("recorded in Close: %v, x4-close needs %v", ids(f), c.Needs)
	}
	must(t, dir, "validate")

	// A done x4-state is not changed; a done item is refused.
	edit(t, dir, "px", func(f *queue.Feature) {
		st, _ := f.Step("x4-state")
		queue.Complete(f, st, "human/jeff", now)
	})
	refused(t, dir, "x4-state is done", "state-step",
		set("human/jeff", "px", "-note", "Again.")...)
	refused(t, dir, "x4-state is done", "state-step", set("human/jeff", "px", "-clear")...)
	edit(t, dir, "px", func(f *queue.Feature) {
		st, _ := f.Step("x4-close")
		queue.Complete(f, st, "cloud/run", now)
	})
	refused(t, dir, "px is done", "state-step", set("human/jeff", "px", "-note", "Again.")...)
}

func ids(f *queue.Feature) []string {
	var out []string
	for _, st := range f.Steps {
		out = append(out, st.ID)
	}
	return out
}

func hasStep(f *queue.Feature, id string) bool {
	_, err := f.Step(id)
	return err == nil
}

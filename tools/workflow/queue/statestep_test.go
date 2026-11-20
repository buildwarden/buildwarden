package queue

import (
	"slices"
	"strings"
	"testing"
)

// TestSeedAddsTheStateStep: a process item with a state step recorded gets
// x4-state when it enters Close, and x4-close waits for it; without one,
// neither; a hand-added x4-state is linked, not added again (FLOW-03;
// merge-confirm-attended, Decision 4, a6, a7).
func TestSeedAddsTheStateStep(t *testing.T) {
	const note = "Update README.md's process round."
	x := process("x", 1, "in_progress")
	x.StateStep = note
	advance(x, 4)
	st, err := x.Step("x4-state")
	if err != nil || st.Affinity != "human" || st.Phase != 4 || st.Status != "pending" ||
		st.Note != note {
		t.Fatalf("x4-state %+v, %v", st, err)
	}
	if c, _ := x.Step("x4-close"); !slices.Equal(c.Needs, []string{"x4-state"}) {
		t.Fatalf("x4-close needs %v, want x4-state", c.Needs)
	}
	wantValidate(t, state(x))

	plain := process("plain", 2, "in_progress")
	advance(plain, 4)
	if got := stepIDs(plain, "pending"); got != "x4-close" {
		t.Fatalf("without a state step: pending %q", got)
	}
	if c, _ := plain.Step("x4-close"); len(c.Needs) != 0 {
		t.Fatalf("without a state step x4-close needs %v", c.Needs)
	}

	hand := process("hand", 3, "in_progress")
	hand.StateStep = note
	hand.Steps = append(hand.Steps, Step{ID: "x4-state", Phase: 4, Affinity: "human",
		Status: "pending", Note: "added by hand"})
	advance(hand, 4)
	if got := stepIDs(hand, "pending"); got != "x4-state x4-close" {
		t.Fatalf("with a hand-added x4-state: pending %q", got)
	}
	if c, _ := hand.Step("x4-close"); !slices.Equal(c.Needs, []string{"x4-state"}) {
		t.Fatalf("with a hand-added x4-state x4-close needs %v", c.Needs)
	}
}

// TestLandedSeedsTheStateStep: an item that lands in Close gets its linked
// x4-state, and the escalation stays unlinked (FLOW-08).
func TestLandedSeedsTheStateStep(t *testing.T) {
	x := mergeWaiting(process("x", 1, "in_progress"), 9)
	x.StateStep = "Update README.md."
	Back(x, "cloud/run", now, "the design must change", 1)
	Landed(x, "human/jeff", now)
	if got := stepIDs(x, "pending"); got != "x4-state x4-close x4-escalate-returned" {
		t.Fatalf("pending %q", got)
	}
	if c, _ := x.Step("x4-close"); !slices.Equal(c.Needs, []string{"x4-state"}) {
		t.Fatalf("x4-close needs %v, want x4-state only", c.Needs)
	}
}

// TestBackRerunsNoProcessHostStep: a process item has no Publish, so a done
// host step added to its Close is not queued again on Back, though Close's
// gate now waits for added steps (merge-confirm-attended, the waiter
// coupling).
func TestBackRerunsNoProcessHostStep(t *testing.T) {
	x := process("x", 1, "in_progress")
	x.Steps = append(x.Steps, Step{ID: "x4-validate-macos-vz", Phase: 4,
		Affinity: "host:macos-vz", Status: "done"})
	advance(x, 2)
	Back(x, "cloud/run", now, "the design must change", 1)
	for _, st := range x.Steps {
		if strings.HasPrefix(st.ID, "x4-validate-macos-vz-r") {
			t.Fatalf("Back queued %s on a process item", st.ID)
		}
	}
}

// TestValidateStateStep: only a process item records a state step.
func TestValidateStateStep(t *testing.T) {
	f := feature("f", 1, "queued")
	f.StateStep = "Update README.md."
	x := process("x", 2, "queued")
	x.StateStep = "Update README.md."
	wantValidate(t, state(f, x), "f: state_step is for a process item (FLOW-03)")
}

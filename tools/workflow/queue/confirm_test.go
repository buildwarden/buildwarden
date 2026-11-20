package queue

import "testing"

// TestMayComplete: a human or host worker may complete a merge step or a
// merged-first confirmation as the cloud does, and no other step it does not
// satisfy (FLOW-04; merge-confirm-attended, Decision 2).
func TestMayComplete(t *testing.T) {
	merges := []string{"x3-merge", "f7-merge", "f7-merge-r2", "f6-merged-1"}
	for _, w := range []string{"cloud/tick", "human/jeff", "host:qemu-any/macbook.run"} {
		for _, id := range merges {
			if !MayComplete(w, &Step{ID: id, Affinity: "cloud"}) {
				t.Errorf("%s may not complete %s", w, id)
			}
		}
		ordinary := &Step{ID: "f3-implement", Affinity: "cloud"}
		if got := MayComplete(w, ordinary); got != (Kind(w) == "cloud") {
			t.Errorf("%s on %s: %v", w, ordinary.ID, got)
		}
		state := &Step{ID: "x4-state", Affinity: "human"}
		if got := MayComplete(w, state); got != (Kind(w) == "human") {
			t.Errorf("%s on %s: %v", w, state.ID, got)
		}
	}
}

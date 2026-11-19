package queue

import "testing"

// TestClaimCountsRuns: a claim counts a run when it is the first on the item
// by its worker's run, the kind and the text after the first "/" (a5).
func TestClaimCountsRuns(t *testing.T) {
	a := feature("a", 1, "in_progress")
	s := state(a)
	for _, c := range []struct {
		worker string
		runs   int
	}{
		{"cloud/A", 1},
		{"cloud/A", 1},    // a retry after a release
		{"cloud/B", 2},    // another run
		{"human/jeff", 2}, // a human's claim is never a run
		{"host:macos-vz/S", 3},
		{"host:macos-qemu/S", 3}, // one native session, two host steps
		{"cloud/S", 4},           // the same suffix in another kind
		{"host:macos-vz/mac.2", 5},
		{"host:macos-qemu/mac.2", 5}, // loop-native-lead's <machine>.<run>
	} {
		s.Claim(a, &a.Steps[0], c.worker, now)
		if got := a.Budget.Spent.Runs; got != c.runs {
			t.Fatalf("after %s's claim: %d runs spent, want %d", c.worker, got, c.runs)
		}
	}
}

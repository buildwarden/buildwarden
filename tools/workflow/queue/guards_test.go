package queue

import "testing"

// TestUnattendedHold: an open blocking question and a spent runaway budget
// stop every worker but a human, a machine's included; a human is offered
// the step (FLOW-07, FLOW-13).
func TestUnattendedHold(t *testing.T) {
	blocking := func(f *Feature) {
		f.Questions = append(f.Questions, Question{ID: "q1", Kind: "blocking",
			Status: "open", Question: "Which?", Options: []string{"a"},
			Recommended: "a"})
	}
	workers := map[string]string{"cloud": "cloud/run",
		"host:macos-vz": "host:macos-vz/macbook.r1", "human": "human/jeff"}
	runaway := func(f *Feature) { f.Budget.Spent.Steps = 14 }
	for _, c := range []struct {
		make func(*Feature)
		want string
	}{
		{blocking, "waiting on blocking question q1"},
		{runaway, "waiting on runaway budget (14 of 7 estimated steps spent; " +
			"escalate to human, FLOW-13)"},
	} {
		for _, aff := range []string{"cloud", "host:macos-vz", "human"} {
			f := working("a", 1, 3)
			st, _ := f.Step("f3-implement")
			st.Affinity = aff
			c.make(f)
			s := state(f)
			worker := workers[aff]
			want := c.want
			if aff == "human" {
				want = "f3-implement"
			}
			wantOfferTo(t, s, "a", worker, want)
		}
	}
}

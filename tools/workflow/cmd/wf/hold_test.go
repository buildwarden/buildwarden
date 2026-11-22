package main

import (
	"strings"
	"testing"
)

// TestHoldCommands: wf new -hold needs -resumes and records it; -resumes
// alone is refused; wf hold holds an item already in progress with the next
// free question (FLOW-20).
func TestHoldCommands(t *testing.T) {
	dir := stateDir(t, "")
	refused(t, dir, "a hold names when it resumes", "new", "-worker", "human/jeff",
		"-feature", "a", "-title", "A", "-priority", "1", "-estimate-steps", "4",
		"-hold", "not a program item")
	refused(t, dir, "-resumes needs -hold", "new", "-worker", "human/jeff",
		"-feature", "a", "-title", "A", "-priority", "1", "-estimate-steps", "4",
		"-resumes", "program-end")
	queued(t, dir, "a", "1", "-hold", "not a program item", "-resumes", "program-end")
	if q := item(t, dir, "a").Hold(); q == nil || q.ID != "q1" || q.Resumes != "program-end" {
		t.Fatalf("a's hold = %+v", q)
	}

	queued(t, dir, "b", "2")
	must(t, dir, "claim", "-worker", "cloud/run", "-feature", "b", "-step", "f0-branch-setup")
	out := must(t, dir, "hold", "-worker", "cloud/run", "-feature", "b",
		"-why", "the payoff is not shown", "-resumes", "program-end")
	if out != "held\tb\tq1\tresumes: program-end\n" {
		t.Fatalf("wf hold printed %q", out)
	}
	got := must(t, dir, "next", "-worker", "cloud/other")
	if strings.Contains(got, "next\tb\t") {
		t.Fatalf("wf next offers the held item: %q", got)
	}
	refused(t, dir, "a hold names when it resumes", "hold", "-worker", "cloud/run",
		"-feature", "b", "-why", "again")
}

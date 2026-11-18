package main

import (
	"strings"
	"testing"
)

const hyperv = "host:windows-hyperv"

// TestLeadCommands: wf new -lead and wf lead set a feature's lead for any
// worker kind, re-label its unleased steps, keep a leased one, and write one
// history line naming the worker, the old and new lead and the steps; a
// process item is refused (FLOW-04, FLOW-09).
func TestLeadCommands(t *testing.T) {
	dir := stateDir(t, "")
	for i, w := range []string{"human/jeff", "cloud/run-1", hyperv + "/windows-desktop.r1"} {
		name := []string{"a", "b", "c"}[i]
		must(t, dir, "new", "-worker", w, "-feature", name, "-title", "x", "-priority",
			[]string{"1", "2", "3"}[i], "-estimate-steps", "9", "-lead", hyperv)
		f := item(t, dir, name)
		if f.Lead != hyperv || f.Steps[0].Affinity != hyperv ||
			events(f)[0] != w+": queued at priority "+[]string{"1", "2", "3"}[i]+
				" (intake), led by "+hyperv {
			t.Fatalf("new -lead by %s: lead %q, steps %+v, history %q", w, f.Lead,
				f.Steps, events(f))
		}
	}
	refused(t, dir, "a process item is never led (FLOW-09)", "new", "-worker",
		"human/jeff", "-feature", "x", "-title", "x", "-priority", "9",
		"-estimate-steps", "9", "-kind", "process", "-lead", hyperv)

	queued(t, dir, "d", "4")
	must(t, dir, "claim", "-worker", "cloud/run-1", "-feature", "d", "-step",
		"f0-branch-setup")
	must(t, dir, "lead", "-worker", "host:macos-vz/macbook.r1", "-feature", "d",
		"-host", "windows-hyperv")
	f := item(t, dir, "d")
	if f.Lead != hyperv || f.Steps[0].Affinity != "cloud" {
		t.Fatalf("lead: %q, the leased step %s", f.Lead, f.Steps[0].Affinity)
	}
	const line = "host:macos-vz/macbook.r1: lead none -> " + hyperv +
		" (FLOW-04); left f0-branch-setup (leased to cloud/run-1)"
	if got := lastEvent(t, dir, "d"); got != line {
		t.Fatalf("history %q, want %q", got, line)
	}
	must(t, dir, "lead", "-worker", "cloud/run-1", "-feature", "a", "-clear")
	if got := lastEvent(t, dir, "a"); got != "cloud/run-1: lead "+hyperv+
		" -> none (FLOW-04); re-labelled f0-branch-setup" {
		t.Fatalf("history %q", got)
	}
	refused(t, dir, "lead needs -host NAME or -clear", "lead", "-worker", "cloud/r",
		"-feature", "a")
	refused(t, dir, "lead needs -worker", "lead", "-feature", "a", "-clear")
	must(t, dir, "validate")
	if out := must(t, dir, "status"); !strings.Contains(out, "\t  lead: "+hyperv+"\n") {
		t.Fatalf("wf status has no lead line:\n%s", out)
	}
}

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// atProcessMerge files a process item and takes it, in progress, to its
// merge step with PR pr recorded, its CI green.
func atProcessMerge(t *testing.T, dir, work, name string, pr int) {
	t.Helper()
	queued(t, dir, name, "1", "-kind", "process")
	edit(t, dir, name, func(f *queue.Feature) {
		f.Status = "in_progress"
		for f.Phase < 3 {
			for i := range f.Steps {
				if f.Steps[i].Status == "pending" && f.Steps[i].Phase == f.Phase {
					queue.Complete(f, &f.Steps[i], "test", now)
					break
				}
			}
		}
		f.PR = &queue.PR{Number: pr, CI: "green",
			Head: gitT(t, work, "rev-parse", "origin/feat/"+name)}
	})
}

// TestAttendedConfirmsAMerge: a human or host worker completes a merge step
// with wf done, which runs the merge's own check, once the human has merged;
// it may not claim one, and a step already done is not completed again
// (FLOW-04, FLOW-08; merge-confirm-attended, Decisions 2 and 3).
func TestAttendedConfirmsAMerge(t *testing.T) {
	work := codeRepo(t, "px", "ph", "fx")
	dir := stateDir(t, here)
	atProcessMerge(t, dir, work, "px", 61)
	atProcessMerge(t, dir, work, "ph", 62)
	reach(t, dir, work, "fx", "3", 7, 63)
	for _, c := range []struct{ name, step, worker string }{
		{"px", "x3-merge", "human/jeff"},
		{"ph", "x3-merge", "host:qemu-any/macbook.run"},
		{"fx", "f7-merge", "human/jeff"},
	} {
		step := []string{"-repo", work, "-worker", c.worker, "-feature", c.name, "-step",
			c.step}
		refused(t, dir, "may not take cloud step "+c.step, "claim", step...)
		out, err := wf(t, dir, "done", step...)
		if err == nil || !strings.Contains(err.Error(), c.step+" done-when not met") ||
			!strings.Contains(out, "FAIL merged by a human") {
			t.Fatalf("%s: wf done before the merge: %v\n%s", c.name, err, out)
		}
		if st, _ := item(t, dir, c.name).Step(c.step); st.Status != "pending" {
			t.Fatalf("%s: %s is %s before the merge", c.name, c.step, st.Status)
		}
	}
	mergePR(t, work, "px", 61)
	mergePR(t, work, "ph", 62)
	mergePR(t, work, "fx", 63)
	for _, c := range []struct {
		name, step, worker string
		phase              int
	}{
		{"px", "x3-merge", "human/jeff", 4},
		{"ph", "x3-merge", "host:qemu-any/macbook.run", 4},
		{"fx", "f7-merge", "human/jeff", 8},
	} {
		step := []string{"-repo", work, "-worker", c.worker, "-feature", c.name, "-step",
			c.step}
		if out, err := wf(t, dir, "done", step...); err != nil ||
			!strings.Contains(out, "ok   merged by a human") {
			t.Fatalf("%s: wf done after the merge: %v\n%s", c.name, err, out)
		}
		f := item(t, dir, c.name)
		if st, _ := f.Step(c.step); st.Status != "done" || f.Phase != c.phase {
			t.Fatalf("%s: %s %s, phase %d", c.name, c.step, st.Status, f.Phase)
		}
		before := len(events(f))
		refused(t, dir, c.step+" is already done; nothing to confirm", "done", step...)
		if got := len(events(item(t, dir, c.name))); got != before {
			t.Fatalf("%s: a second done wrote %d history lines", c.name, got-before)
		}
	}
	must(t, dir, "validate")
}

// TestStaleNamesTheConfirmation: wf stale's line for a merged item at its
// merge step gives the command that confirms it (merge-confirm-attended,
// Decision 7).
func TestStaleNamesTheConfirmation(t *testing.T) {
	work := codeRepo(t, "px", "fx")
	dir := stateDir(t, here)
	atProcessMerge(t, dir, work, "px", 71)
	reach(t, dir, work, "fx", "2", 7, 72)
	mergePR(t, work, "px", 71)
	mergePR(t, work, "fx", 72)
	want := []string{"merged: px (confirm: wf done -feature px -step x3-merge)",
		"merged: fx (confirm: wf done -feature fx -step f7-merge)"}
	if got := runStale(t, dir, work); !slices.Equal(got, want) {
		t.Fatalf("wf stale = %q, want %q", got, want)
	}
}

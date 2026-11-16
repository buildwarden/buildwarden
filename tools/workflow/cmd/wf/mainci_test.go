package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mainYAML is main.yaml as the state checkout has it.
func mainYAML(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "main.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func queueMD(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "QUEUE.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestMainCI: wf main records main's CI in main.yaml: red_since across red
// commits and cleared by a green one, a red the gates found kept for its
// commit, the follow-up once, and whether GitHub requires an up-to-date
// branch (FLOW-08, FLOW-09, FLOW-10).
func TestMainCI(t *testing.T) {
	dir := stateDir(t, here)
	queued(t, dir, "fix", "1")
	queued(t, dir, "other", "2")
	if md := queueMD(t, dir); !strings.Contains(md, "`main`: CI not recorded.") ||
		!strings.Contains(md, "up-to-date branch on `main`: not recorded") {
		t.Fatalf("with no main.yaml QUEUE.md says:\n%s", md)
	}
	has := func(wants ...string) {
		t.Helper()
		got := mainYAML(t, dir)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Fatalf("main.yaml lacks %q:\n%s", w, got)
			}
		}
	}
	lacks := func(w string) {
		t.Helper()
		if got := mainYAML(t, dir); strings.Contains(got, w) {
			t.Fatalf("main.yaml still has %q:\n%s", w, got)
		}
	}
	must(t, dir, "main", "-sha", "aaaaaaa", "-ci", "red", "-run", "https://ci/1")
	has("red_since: aaaaaaa", "run: https://ci/1")
	must(t, dir, "main", "-sha", "bbbbbbb", "-ci", "red")
	has("sha: bbbbbbb", "red_since: aaaaaaa")
	must(t, dir, "main", "-followup", "fix")
	has("followup: fix")
	refused(t, dir, "already recorded", "main", "-followup", "other")
	if md := queueMD(t, dir); !strings.Contains(md,
		"`main` is red at `bbbbbbb` since `aaaaaaa`: no run recorded. "+
			"Follow-up: **fix**.") {
		t.Fatalf("QUEUE.md does not open Needs Jeff with the red main:\n%s", md)
	}
	must(t, dir, "main", "-sha", "ccccccc", "-ci", "green")
	lacks("red_since")
	lacks("followup")
	refused(t, dir, "not recorded red", "main", "-followup", "fix")

	// The gates' red stands against a green reading of the same commit.
	must(t, dir, "main", "-sha", "ddddddd", "-ci", "red", "-gates")
	must(t, dir, "main", "-sha", "ddddddd", "-ci", "green")
	has("ci: red", "gates: true")
	must(t, dir, "main", "-sha", "eeeeeee", "-ci", "green")
	has("ci: green")
	lacks("gates")

	must(t, dir, "main", "-up-to-date", "required")
	has("up_to_date: required")
	if md := queueMD(t, dir); !strings.Contains(md,
		"GitHub requires an up-to-date branch on `main` (FLOW-08).") {
		t.Fatalf("QUEUE.md does not say the branch must be up to date:\n%s", md)
	}
	refused(t, dir, "required or not-required", "main", "-up-to-date", "yes")
	refused(t, dir, "not a commit id", "main", "-sha", "main", "-ci", "green")
	refused(t, dir, "green, red or pending", "main", "-sha", "fffffff", "-ci", "blue")
	must(t, dir, "validate")
}

// TestStatusLines: wf status prints an item's declarations.
func TestStatusLines(t *testing.T) {
	dir := stateDir(t, here)
	queued(t, dir, "x", "1", "-repos", "o/here,o/relay", "-areas", "relay,book",
		"-adopts", "relay@v1", "-urgent")
	out := must(t, dir, "status")
	for _, w := range []string{"repos: o/here, o/relay", "areas: relay, book",
		"adopts: relay v1", "urgent: human/jeff"} {
		if !strings.Contains(out, w) {
			t.Errorf("wf status lacks %q:\n%s", w, out)
		}
	}
}

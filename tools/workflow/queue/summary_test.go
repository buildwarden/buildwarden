package queue

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/queue.md")

// summaryState covers every section: a blocking question, a design review,
// a PR waiting, a runaway, a host step and a human step of a later phase, and
// the backlog with a fix. Its done items, one closed recently, one long ago and one spent
// past the runaway factor, must appear in none of them (FLOW-13).
func summaryState() *State {
	at := func(d int) time.Time { return now.Add(time.Duration(d) * 24 * time.Hour) }
	ev := func(d int) []Event { return []Event{{At: at(d), By: "test", Event: "e"}} }

	asked := feature("asked", 1, "in_progress")
	asked.Phase = 1
	asked.Steps = []Step{{ID: "f1-design", Phase: 1, Affinity: "cloud", Status: "pending",
		Lease: Lease{ClaimedBy: "cloud/run"}}}
	asked.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open",
		Question: "Which base?", Options: []string{"a", "b"}, Recommended: "a"}}
	asked.History = ev(0)

	review := feature("review", 2, "in_progress")
	review.Phase = 2
	review.Steps = []Step{{ID: "f2-design-review", Phase: 2, Affinity: "human",
		Status: "pending"},
		{ID: "f2-plan", Phase: 2, Affinity: "cloud", Status: "pending",
			Needs: []string{"f2-design-review"}},
		{ID: "f6-validate-windows-hyperv", Phase: 6, Affinity: "host:windows-hyperv",
			Status: "pending"},
		{ID: "f6-provision-windows-hyperv", Phase: 6, Affinity: "human",
			Status: "pending"}}
	review.History = ev(0)

	merging := feature("merging", 3, "in_progress")
	merging.Phase, merging.PR = 7, &PR{Number: 42, CI: "green"}
	merging.Steps = []Step{{ID: "f7-merge", Phase: 7, Affinity: "cloud", Status: "pending"}}
	merging.Budget = Budget{Estimate: Count{Steps: 5}, Spent: Count{Steps: 11}}
	merging.History = ev(0)

	queued := feature("queued", 4, "queued")
	queued.Follows = "recent"
	proc := process("rules", 5, "queued", "host:*")

	recent := feature("recent", 6, "done")
	recent.PR, recent.History = &PR{Number: 40}, ev(-2)
	old := feature("old", 7, "done")
	old.History = ev(-30)
	spent := feature("spent", 8, "done")
	spent.Budget.Spent.Steps, spent.History = 30, ev(-1)

	s := state(asked, review, merging, queued, proc, recent, old, spent)
	s.sortFeatures()
	return s
}

func TestSummaryGolden(t *testing.T) {
	got := Summary(summaryState())
	path := filepath.Join("testdata", "queue.md")
	if *update {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("Summary differs from %s (go test -run Golden -update):\n%s", path, got)
	}
	if strings.Contains(got, "Recently done") {
		t.Error("Summary has a Recently done section")
	}
	for _, done := range []string{"recent", "old", "spent"} {
		if strings.Contains(got, "**"+done+"**") || strings.Contains(got, "| "+done+" |") {
			t.Errorf("Summary lists the done item %q", done)
		}
	}
	if again := Summary(summaryState()); again != got {
		t.Fatal("Summary is not deterministic")
	}
}

func TestSaveWritesSummary(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "queue"), 0o755)
	s := &State{Dir: dir}
	f := feature("x", 1, "queued")
	s.Features = []*Feature{f}
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, SummaryFile))
	if err != nil || string(got) != Summary(s) {
		t.Fatalf("QUEUE.md = %q, %v", got, err)
	}
}

// TestSaveWritesClassOrder: a phase change moves an item between classes, and
// the next Save writes QUEUE.md in the new order without a reload.
func TestSaveWritesClassOrder(t *testing.T) {
	s := summaryState()
	s.Dir = t.TempDir()
	_ = os.MkdirAll(filepath.Join(s.Dir, "queue"), 0o755)
	review, _ := s.Find("review")
	review.Phase = 3
	if err := s.Save(review); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.Dir, SummaryFile))
	if err != nil {
		t.Fatal(err)
	}
	r, a := strings.Index(string(got), "| 2 | review |"),
		strings.Index(string(got), "| 1 | asked |")
	if r < 0 || a < 0 || r > a {
		t.Fatalf("QUEUE.md lists review at %d, asked at %d; want review (now in "+
			"Implement) first:\n%s", r, a, got)
	}
}

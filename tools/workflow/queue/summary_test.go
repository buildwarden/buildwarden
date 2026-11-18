package queue

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// TestSaveWritesClassOrderInEverySection: a change inside a command moves
// items between classes, and the next Save lists them in the new order in
// Needs Jeff, Waiting on a host and Backlog, not only in Active.
func TestSaveWritesClassOrderInEverySection(t *testing.T) {
	waiting := func(name string, prio int) *Feature {
		f := feature(name, prio, "in_progress")
		f.Phase = 2
		f.Steps = []Step{
			{ID: "f2-design-review", Phase: 2, Affinity: "human", Status: "pending"},
			{ID: "f6-validate-qemu-any", Phase: 6, Affinity: "host:qemu-any",
				Status: "pending"}}
		return f
	}
	first, second := waiting("first", 1), waiting("second", 2)
	plain, fix := feature("plain", 3, "queued"), feature("fix", 4, "queued")
	parent := feature("parent", 5, "done")
	s := state(first, second, plain, fix, parent)
	s.Dir = t.TempDir()
	_ = os.MkdirAll(filepath.Join(s.Dir, "queue"), 0o755)

	check := func(title, before, after string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(s.Dir, SummaryFile))
		if err != nil {
			t.Fatal(err)
		}
		sec := string(got)
		if i := strings.Index(sec, "\n## "+title+"\n"); i >= 0 {
			sec = sec[i+len(title)+5:]
		} else {
			t.Fatalf("QUEUE.md has no %s section:\n%s", title, got)
		}
		if j := strings.Index(sec, "\n## "); j >= 0 {
			sec = sec[:j]
		}
		b, a := strings.Index(sec, " "+before+" "), strings.Index(sec, " "+after+" ")
		if b < 0 || a < 0 || b > a {
			t.Errorf("%s lists %s at %d, %s at %d; want %s first:\n%s", title,
				before, b, after, a, before, sec)
		}
	}
	if err := s.Save(first); err != nil {
		t.Fatal(err)
	}
	check("Needs Jeff", "**first**:", "**second**:")
	check("Waiting on a host", "**first**:", "**second**:")
	check("Backlog", "plain", "fix")

	// second enters Implement (class 1) and fix gains follows (class 3).
	second.Phase, fix.Follows = 3, "parent"
	if err := s.Save(fix); err != nil {
		t.Fatal(err)
	}
	check("Needs Jeff", "**second**:", "**first**:")
	check("Waiting on a host", "**second**:", "**first**:")
	check("Backlog", "fix", "plain")
}

// at is a step of the phase with the affinity, pending, with needs.
func at(id string, phase int, affinity string, needs ...string) Step {
	return Step{ID: id, Phase: phase, Affinity: affinity, Status: "pending", Needs: needs}
}

// golden compares the text with testdata/<name>, which -update rewrites.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("Summary differs from %s (go test -run Summary -update):\n%s", path, got)
	}
}

// todayState is the queue at 0d5e755 on the workflow branch, as the
// loop-parallel design's What Jeff sees renders it: config.yaml's repository
// set, the map saved, main's CI and the map's comparison recorded, and no
// item declared.
func todayState(t *testing.T) *State {
	ev := []Event{{At: time.Date(2026, 11, 14, 21, 34, 0, 0, time.UTC), By: "t", Event: "e"}}
	contract := process("loop-contract", 60, "in_progress")
	contract.Phase, contract.PR = 3, &PR{Number: 119, CI: "green", Head: "1234567"}
	contract.Steps = []Step{at("x3-merge", 3, "cloud")}
	contract.Budget = Budget{Estimate: Count{Steps: 8}, Spent: Count{Steps: 2}}
	parallel := process("loop-parallel", 61, "in_progress")
	parallel.Steps[0].Lease.ClaimedBy = "human/jeff"
	parallel.Budget = Budget{Estimate: Count{Steps: 12}, Spent: Count{Steps: 1}}
	review := func(name string, prio, spent int) *Feature {
		f := feature(name, prio, "in_progress")
		f.Phase, f.Branch, f.Design = 2, "feat/"+name, "docs/design/"+name+".md"
		f.Steps = []Step{at("f2-design-review", 2, "human"),
			at("f2-plan", 2, "cloud", "f2-design-review")}
		f.Budget = Budget{Estimate: Count{Steps: 30}, Spent: Count{Steps: spent}}
		return f
	}
	specs, contractD := review("seam-specs", 64, 2), review("driver-contract", 65, 5)
	contractD.Steps[0].Lease.ClaimedBy = "human/jeff"
	contractD.Steps = append(contractD.Steps, at("f6-validate-macos-vz", 6, "host:macos-vz"))
	lead := process("loop-native-lead", 62, "queued")
	repo := process("loop-repository", 63, "queued")
	repo.Budget.Estimate.Steps = 10
	for _, f := range []*Feature{contract, parallel, specs, contractD, lead, repo} {
		f.History = ev
	}
	s := mapped(t, contract, parallel, specs, contractD, lead, repo)
	inProgress(s, 5)
	s.Main = &Main{SHA: "897a4a8", CI: "green", UpToDate: "not-required",
		Areas: &AreasResult{SHA: "897a4a8"}}
	return s
}

// laterState is the design's later state: case 7 after the Hyper-V pilot's
// PR has merged, with a relay task tracked and the waiting limit reached.
func laterState(t *testing.T) *State {
	ev := []Event{{At: time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC), By: "t", Event: "e"}}
	areas := func(f *Feature, names ...string) { f.Areas = &names }
	host := feature("relay-host-mode", 70, "in_progress")
	host.Phase, host.PR = 7, &PR{Number: 131, CI: "pending", Head: "abcdef0"}
	areas(host, "relay", "book")
	host.Steps = []Step{at("f7-merge", 7, "cloud"), at("f6-refresh", 7, "cloud")}
	host.Steps[1].Note = "main at 4f1c2aa: 1 commit since the branch merged it; both " +
		"change docs/book/src/guide/drivers.md"
	host.Budget = Budget{Estimate: Count{Steps: 18}, Spent: Count{Steps: 16}}
	claim := feature("driver-claim-conn", 71, "in_progress")
	claim.Phase = 6
	areas(claim, "driver-core", "rules")
	claim.Steps = []Step{at("f6-merge-main", 6, "cloud")}
	claim.Budget = Budget{Estimate: Count{Steps: 25}, Spent: Count{Steps: 20}}
	adopt := feature("adopt-relay-qemu", 72, "in_progress")
	adopt.Phase, adopt.Adopts = 2, &Adopts{Consumable: "buildwarden-relay", Version: "v0.3.0"}
	adopt.Branch, adopt.Design = "feat/adopt-relay-qemu", "docs/design/adopt-relay-qemu.md"
	areas(adopt, "driver/qemu", "book")
	adopt.Steps = []Step{at("f2-design-review", 2, "human")}
	adopt.Budget = Budget{Estimate: Count{Steps: 12}, Spent: Count{Steps: 3}}
	tracked := feature("relay-m1a", 74, "in_progress")
	tracked.Round, tracked.Phase = "tracked", 2
	tracked.Repos = []string{"buildwarden/buildwarden-relay"}
	tracked.PR = &PR{Number: 12, CI: "green"}
	tracked.Steps = []Step{at("t2-merge", 2, "human")}
	tracked.Budget = Budget{Estimate: Count{Steps: 2}, Spent: Count{Steps: 2}}
	plain := feature("relay-metrics", 75, "queued")
	marked := feature("relay-cve", 76, "queued")
	marked.Urgent = "human/jeff"
	fs := []*Feature{host, claim, adopt, tracked, plain, marked}
	for _, f := range fs {
		f.History = ev
	}
	s := mapped(t, fs...)
	limit := 2
	s.Config.Limits.WaitingOnHuman = &limit
	s.Main = &Main{SHA: "4f1c2aa", CI: "green", UpToDate: "required",
		Areas: &AreasResult{SHA: "4f1c2aa"}}
	return s
}

// TestSummary: QUEUE.md for the two states the loop-parallel design shows,
// with the count beside the limit, the two lists of open PRs, the items not
// counted, a marked feature's Kind, and the line above Backlog at the limit
// (FLOW-05, FLOW-08).
func TestSummary(t *testing.T) {
	today := Summary(todayState(t))
	golden(t, "queue-today.md", today)
	for _, want := range []string{
		"`main`: CI green at `897a4a8`. GitHub does not require an up-to-date branch on " +
			"`main` (FLOW-08). Area map: no difference from `main` at `897a4a8`.",
		"4 items in progress wait on you (limit 8): 1 merge, 3 design reviews. " +
			"No question or escalation.",
		"- **loop-contract**: [PR #119](https://github.com/buildwarden/buildwarden/" +
			"pull/119), CI green",
		"- **loop-native-lead**: design review (`x1-design`, FLOW-19)",
		"| 64 | seam-specs | feature | - | buildwarden/buildwarden | undeclared | " +
			"2 plan |",
		"| seam-specs, driver-contract | colliding |",
	} {
		if !strings.Contains(today, want) {
			t.Errorf("today's QUEUE.md lacks %q", want)
		}
	}
	s := laterState(t)
	later := Summary(s)
	golden(t, "queue-later.md", later)
	waiting, limit, _ := s.pastWaitingLimit()
	for _, want := range []string{
		fmt.Sprintf("%d items in progress wait on you (limit %d): 1 merge, 1 design "+
			"review.", len(waiting), limit),
		"### Ready to merge\n\nNone.",
		"- **relay-host-mode**: [PR #131](https://github.com/buildwarden/buildwarden/" +
			"pull/131): refresh pending (`f6-refresh`), then CI",
		"- **relay-m1a** (tracked): buildwarden/buildwarden-relay#12, your merge " +
			"(`t2-merge`)",
		"| a refresh, then your merge | [#131](https://github.com/buildwarden/" +
			"buildwarden/pull/131) behind `main` |",
		"| adoption (buildwarden-relay v0.3.0) |",
		"| 76 | relay-cve | feature, urgent |",
		"No queued feature starts: 2 items wait on you (limit 2). Fixes, process items " +
			"and security fixes you marked urgent still start.",
	} {
		if !strings.Contains(later, want) {
			t.Errorf("the later QUEUE.md lacks %q", want)
		}
	}
	if s.Main = nil; !strings.Contains(Summary(s), "`main`: CI not recorded. Whether "+
		"GitHub requires an up-to-date branch on `main`: not recorded (FLOW-08). Area "+
		"map: not checked against `main`.") {
		t.Error("with no main.yaml the header does not say what is not recorded")
	}
}

// TestDigestPage: the day's page is the run files in file-name order under
// the generated line; a page written by hand is moved into the directory as
// its first file, once; the same files give the same page (LOOP-06).
func TestDigestPage(t *testing.T) {
	s := &State{Dir: t.TempDir()}
	day := filepath.Join(s.Dir, "digest", "2026-11-15")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(name, text string) {
		err := os.WriteFile(filepath.Join(s.Dir, name), []byte(text), 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	put("digest/2026-11-15.md", "## Run by hand\n")
	put("digest/2026-11-14.md", "## An older day, with no directory\n")
	put("digest/2026-11-15/20261115T200000Z-human-jeff.md", "## Run human/jeff\n")
	put("digest/2026-11-15/20261115T030000Z-cloud.md", "## Run cloud\n")
	paths, err := s.Digest()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"digest/2026-11-15/" + earlierPage, "digest/2026-11-15.md"}
	if !slices.Equal(paths, want) {
		t.Fatalf("Digest wrote %v, want %v", paths, want)
	}
	page, _ := os.ReadFile(filepath.Join(s.Dir, "digest/2026-11-15.md"))
	const body = "\n# Digest 2026-11-15\n\n## Run by hand\n\n## Run cloud\n\n" +
		"## Run human/jeff\n"
	if !strings.HasPrefix(string(page), digestMark) || !strings.HasSuffix(string(page), body) {
		t.Fatalf("the page is:\n%s", page)
	}
	if again, err := s.Digest(); err != nil || !slices.Equal(again, want[1:]) {
		t.Fatalf("Digest again wrote %v, %v; want only the page", again, err)
	}
	if page2, _ := os.ReadFile(filepath.Join(s.Dir, "digest/2026-11-15.md")); string(page2) !=
		string(page) {
		t.Fatal("the same files gave another page")
	}
	if old, _ := os.ReadFile(filepath.Join(s.Dir, "digest/2026-11-14.md")); string(old) !=
		"## An older day, with no directory\n" {
		t.Fatal("a day with no directory was rewritten")
	}
}

package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const programTrunk = "program/componentization"

// fakeGitHub is what the gate reads from GitHub in the tests.
type fakeGitHub struct {
	view     PRView
	runs     []CheckRun
	required []string
}

func (g *fakeGitHub) PR(string, int) (PRView, error)               { return g.view, nil }
func (g *fakeGitHub) CheckRuns(string, string) ([]CheckRun, error) { return g.runs, nil }
func (g *fakeGitHub) Required(string, string) ([]string, error)    { return g.required, nil }

// gated is a program item at its merge step whose PR into the program trunk
// passes every point of the gate: the environment, the item and what GitHub
// says. edit runs on the branch before it is pushed.
type gated struct {
	env  Env
	f    *queue.Feature
	gh   *fakeGitHub
	work string
	head string
}

func gateFixture(t *testing.T, designText string, edit func(work string)) *gated {
	t.Helper()
	work := repo(t, designText)
	git(t, work, "push", "-q", "origin", "main:refs/heads/"+programTrunk)
	if edit != nil {
		edit(work)
		git(t, work, "push", "-q", "origin", "feat/demo")
	}
	git(t, work, "fetch", "-q", "origin")
	head := fullHead(t, work, "feat/demo")
	f := demo(7)
	f.Program, f.Trunk = true, programTrunk
	f.PR = &queue.PR{Number: 42, CI: "green", Head: head}
	s := hereState(f)
	s.Config.Repositories[0].ProgramTrunk = programTrunk
	wf := t.TempDir()
	report(t, wf, head, nil)
	gh := &fakeGitHub{view: PRView{Base: programTrunk, Head: head, Branch: "feat/demo"},
		runs: []CheckRun{{"ci-ok", "completed", "success"},
			{"sessions-private", "completed", "success"},
			{"push-only", "completed", "skipped"}},
		required: []string{"ci-ok", "sessions-private"}}
	return &gated{env: Env{Repo: work, Workflow: wf, State: s, GitHub: gh}, f: f, gh: gh,
		work: work, head: head}
}

// report writes the tier-2 report for the head with the findings.
func report(t *testing.T, wf, head string, findings []map[string]any) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"head": head[:12], "findings": findings})
	dir := filepath.Join(wf, "reports", "demo")
	_ = os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, head[:12]+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// openMust is a MUST finding still open on the rule.
func openMust(rule string) []map[string]any {
	return []map[string]any{{"id": 7, "rule": rule, "severity": "MUST", "status": "open"}}
}

// TestMergeReady: the merge gate passes with every point holding, and fails
// on each point alone, naming it (FLOW-08, Decision 9).
func TestMergeReady(t *testing.T) {
	g := gateFixture(t, design, nil)
	if rs := MergeReady(g.env, g.f, hereRepo); !OK(rs) {
		t.Fatalf("every point holds: %s", failures(rs))
	}
	cases := map[string]struct {
		point  string
		break_ func(t *testing.T, g *gated)
	}{
		"a base other than the program trunk": {"merge gate 1",
			func(_ *testing.T, g *gated) { g.gh.view.Base = "main" }},
		"a moved head": {"merge gate 2",
			func(_ *testing.T, g *gated) { g.f.PR.Head = strings.Repeat("a", 40) }},
		"a failed check": {"merge gate 3", func(_ *testing.T, g *gated) {
			g.gh.runs = append(g.gh.runs, CheckRun{"lint", "completed", "failure"})
		}},
		"a required check missing": {"merge gate 3",
			func(_ *testing.T, g *gated) { g.gh.runs = g.gh.runs[1:] }},
		"a ruleset that requires none": {"merge gate 3",
			func(_ *testing.T, g *gated) { g.gh.required = nil }},
		"a branch behind": {"merge gate 4", func(t *testing.T, g *gated) {
			git(t, g.work, "checkout", "-q", "-b", "moved", "origin/"+programTrunk)
			_ = os.WriteFile(filepath.Join(g.work, "moved.txt"), []byte("x\n"), 0o644)
			git(t, g.work, "add", "moved.txt")
			git(t, g.work, "commit", "-q", "-m", "moved")
			git(t, g.work, "push", "-q", "origin", "moved:"+programTrunk)
			git(t, g.work, "fetch", "-q", "origin")
			git(t, g.work, "checkout", "-q", "feat/demo")
		}},
		"an open MUST": {"merge gate 5", func(t *testing.T, g *gated) {
			report(t, g.env.Workflow, g.head, openMust("ORCH-02"))
		}},
		"a protected MUST, deferred": {"merge gate 5", func(t *testing.T, g *gated) {
			report(t, g.env.Workflow, g.head, openMust("GLOBAL-06"))
			deferIt(t, g)
		}},
		"a MUST under FLOW-20's never-break list, deferred": {"merge gate 5",
			func(t *testing.T, g *gated) {
				report(t, g.env.Workflow, g.head, openMust("FLOW-20"))
				deferIt(t, g)
			}},
		"a MUST Review recorded protected, deferred": {"merge gate 5",
			func(t *testing.T, g *gated) {
				report(t, g.env.Workflow, g.head, openMust("ORCH-02"))
				protect(t, g, "breaks a written schema $id")
				deferIt(t, g)
			}},
		"a MUST with no status": {"merge gate 5", func(t *testing.T, g *gated) {
			report(t, g.env.Workflow, g.head, []map[string]any{{"id": 7,
				"rule": "ORCH-02", "severity": "MUST"}})
		}},
		"a MUST-review": {"merge gate 5", func(t *testing.T, g *gated) {
			report(t, g.env.Workflow, g.head, []map[string]any{{"id": 7,
				"rule": "ORCH-02", "severity": "MUST-review",
				"status": "open (MUST-review, for the human at merge)"}})
		}},
		"a security-ack not the human's": {"merge gate 7", func(t *testing.T, g *gated) {
			*g = *gateFixture(t, strings.Replace(design, "security_boundary: none",
				"security_boundary: env-relay", 1), nil)
			rawRecords(t, g.env.Workflow, "- by: cloud/run\n  kind: security-ack\n"+
				"  feature: demo\n  head: "+g.head+"\n  note: acknowledged\n")
		}},
		"a pending human step": {"merge gate 6", func(_ *testing.T, g *gated) {
			g.f.Questions = []queue.Question{{ID: "q1", Kind: "blocking",
				Status: "open", Question: "?", Options: []string{"a"},
				Recommended: "a"}}
		}},
		"a posture change": {"merge gate 7", func(t *testing.T, g *gated) {
			*g = *gateFixture(t, strings.Replace(design, "security_boundary: none",
				"security_boundary: env-relay", 1), nil)
		}},
		"a merged-first PR open": {"merge gate 8", func(_ *testing.T, g *gated) {
			g.f.Work = []queue.Work{{Repo: "o/specs", Branch: "feat/demo",
				First: true}}
		}},
		"an uncleared rule change": {"rule changes cleared", func(t *testing.T, g *gated) {
			*g = *gateFixture(t, design, func(work string) {
				p := filepath.Join(work, "docs", "development", "process.md")
				_ = os.WriteFile(p, []byte("# a rule\n"), 0o644)
				git(t, work, "add", ".")
				git(t, work, "commit", "-q", "-m", "a rule")
			})
		}},
	}
	for name, c := range cases {
		g := gateFixture(t, design, nil)
		c.break_(t, g)
		got := failures(MergeReady(g.env, g.f, hereRepo))
		if !strings.Contains(got, c.point) {
			t.Errorf("%s: failures %q, want %s", name, got, c.point)
		}
		for _, other := range []string{"merge gate 1", "merge gate 2", "merge gate 3",
			"merge gate 4", "merge gate 5", "merge gate 6", "merge gate 7",
			"merge gate 8", "rule changes cleared"} {
			if other != c.point && strings.Contains(got, other) {
				t.Errorf("%s: %s fails too: %q", name, other, got)
			}
		}
	}

	// A MUST fixed, in any wording that starts so, does not stop the merge.
	g = gateFixture(t, design, nil)
	report(t, g.env.Workflow, g.head, []map[string]any{{"id": 7, "rule": "ORCH-02",
		"severity": "MUST", "status": "fixed in r2"}})
	if rs := MergeReady(g.env, g.f, hereRepo); !OK(rs) {
		t.Fatalf("a fixed MUST: %s", failures(rs))
	}
	// A MUST the records defer on the head no longer stops the merge.
	g = gateFixture(t, design, nil)
	report(t, g.env.Workflow, g.head, openMust("ORCH-02"))
	deferIt(t, g)
	if rs := MergeReady(g.env, g.f, hereRepo); !OK(rs) {
		t.Fatalf("a deferred MUST: %s", failures(rs))
	}
	// The human's acknowledgment of the security note lets a posture change
	// merge.
	g = gateFixture(t, strings.Replace(design, "security_boundary: none",
		"security_boundary: env-relay", 1), nil)
	if err := queue.AppendRecord(g.env.Workflow, queue.Record{By: "human/jeff",
		Kind: "security-ack", Feature: "demo", Head: g.head,
		Note: "security note read"}); err != nil {
		t.Fatal(err)
	}
	if rs := MergeReady(g.env, g.f, hereRepo); !OK(rs) {
		t.Fatalf("an acknowledged posture change: %s", failures(rs))
	}
}

// TestMergeReadyRelayClause: a security_boundary whose only trip is
// GLOBAL-06's relay clause merges on its review's security-note records, the
// design's (U4 or the design review) and Review's on the head, with no
// security-ack; without either, or with Review's on another head, it waits
// on the human's acknowledgment (FLOW-08 point 7, a14 (a)).
func TestMergeReadyRelayClause(t *testing.T) {
	relay := strings.Replace(design, "security_boundary: none",
		"security_boundary: env-relay", 1)
	note := func(t *testing.T, g *gated, ref, head string) {
		t.Helper()
		if err := queue.AppendRecord(g.env.Workflow, queue.Record{By: "cloud/run",
			Kind: "security-note", Feature: "demo", Ref: ref, Head: head,
			Note: "only the relay clause; no boundary posture changes"}); err != nil {
			t.Fatal(err)
		}
	}
	other := strings.Repeat("c", 40)
	both := []string{queue.SecurityNoteDesign, queue.SecurityNoteDiff}
	for name, c := range map[string]struct {
		refs  []string
		heads []string
		ok    bool
	}{
		"design and diff":      {both, nil, true},
		"design only":          {[]string{queue.SecurityNoteDesign}, nil, false},
		"diff only":            {[]string{queue.SecurityNoteDiff}, nil, false},
		"diff on another head": {both, []string{"", other}, false},
		"no record (posture)":  {nil, nil, false},
	} {
		g := gateFixture(t, relay, nil)
		for i, ref := range c.refs {
			head := g.head
			if i < len(c.heads) && c.heads[i] != "" {
				head = c.heads[i]
			}
			note(t, g, ref, head)
		}
		got := failures(MergeReady(g.env, g.f, hereRepo))
		if c.ok && got != "" {
			t.Errorf("%s: %s", name, got)
		}
		if !c.ok && !strings.Contains(got, "merge gate 7") {
			t.Errorf("%s: merge gate 7 passed: %q", name, got)
		}
	}
}

// rawRecords writes the program records file as given, as a hand edit
// would, past AppendRecord's checks.
func rawRecords(t *testing.T, wf, text string) {
	t.Helper()
	p := filepath.Join(wf, filepath.FromSlash(queue.RecordsFile))
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestMergeWaitsOnHuman: a loop merge waits on the human when the gate holds
// it at point 7 on a posture change he has not acknowledged, or at point 5 on
// a protected finding recorded on its head; not with security_boundary none,
// nor once he acknowledged it (FLOW-05, FLOW-20).
func TestMergeWaitsOnHuman(t *testing.T) {
	posture := strings.Replace(design, "security_boundary: none",
		"security_boundary: env-relay", 1)
	g := gateFixture(t, design, nil)
	if MergeWaitsOnHuman(g.env, g.f, hereRepo) {
		t.Error("security_boundary none waits on the human")
	}
	protect(t, g, "asked Jeff to dismiss it")
	if !MergeWaitsOnHuman(g.env, g.f, hereRepo) {
		t.Error("a protected finding on the head does not wait on the human")
	}
	g = gateFixture(t, posture, nil)
	if !MergeWaitsOnHuman(g.env, g.f, hereRepo) {
		t.Error("a posture change does not wait on the human")
	}
	if err := queue.AppendRecord(g.env.Workflow, queue.Record{By: "human/jeff",
		Kind: "security-ack", Feature: "demo", Head: g.head,
		Note: "security note read"}); err != nil {
		t.Fatal(err)
	}
	if MergeWaitsOnHuman(g.env, g.f, hereRepo) {
		t.Error("an acknowledged posture change still waits on the human")
	}
}

// protect records finding 7 of the head as protected.
func protect(t *testing.T, g *gated, note string) {
	t.Helper()
	if err := queue.AppendRecord(g.env.Workflow, queue.Record{By: "cloud/run",
		Kind: "protected", Feature: "demo", Ref: "7", Head: g.head,
		Note: note}); err != nil {
		t.Fatal(err)
	}
}

// deferIt records finding 7 of the head as deferred.
func deferIt(t *testing.T, g *gated) {
	t.Helper()
	if err := queue.AppendRecord(g.env.Workflow, queue.Record{By: "cloud/run",
		Kind: "deferred", Feature: "demo", Ref: "7", Head: g.head,
		Note: "not fixed in one round"}); err != nil {
		t.Fatal(err)
	}
}

// TestLoopMergeProof: after a merge commit on the program trunk, the merge
// step's proof holds once origin/<program trunk> is fetched and records the
// Merge pull request commit; on the unfetched ref it fails (FLOW-08).
func TestLoopMergeProof(t *testing.T) {
	g := gateFixture(t, design, nil)
	before := fullHead(t, g.work, "origin/"+programTrunk)
	git(t, g.work, "checkout", "-q", "-b", "pt", "origin/"+programTrunk)
	git(t, g.work, "merge", "-q", "--no-ff", "-m",
		"Merge pull request #42 from o/feat/demo", "feat/demo")
	git(t, g.work, "push", "-q", "origin", "pt:"+programTrunk)
	// The push moved the local tracking ref; a session that merged on GitHub
	// still has the one from before the merge.
	git(t, g.work, "update-ref", "refs/remotes/origin/"+programTrunk, before)
	git(t, g.work, "checkout", "-q", "feat/demo")
	git(t, g.work, "branch", "-q", "-D", "pt")
	if rs := Phase(g.env, g.f); OK(rs) {
		t.Fatal("the proof held on the unfetched program trunk")
	}
	git(t, g.work, "fetch", "-q", "origin")
	if rs := Phase(g.env, g.f); !OK(rs) {
		t.Fatalf("after the fetch: %s", failures(rs))
	}
	c := newChecker(g.env, g.f)
	c.merged()
	if want := fullHead(t, g.work, "origin/"+programTrunk); c.home.merge != want {
		t.Fatalf("recorded merge %s, want the Merge pull request commit %s",
			c.home.merge, want)
	}
}

// syncFixture is a program sync ready to merge: main moved past the program
// trunk, the program trunk moved too, and the sync branch merges main into
// it with the head recorded. conflict makes both sides change one file, and
// the sync's merge resolve it.
func syncFixture(t *testing.T, conflict bool) (Env, *fakeGitHub, string) {
	t.Helper()
	g := gateFixture(t, design, nil)
	w := g.work
	mainFile, progFile := "main.txt", "prog.txt"
	if conflict {
		mainFile, progFile = "both.txt", "both.txt"
	}
	git(t, w, "checkout", "-q", "main")
	_ = os.WriteFile(filepath.Join(w, mainFile), []byte("m\n"), 0o644)
	git(t, w, "add", mainFile)
	git(t, w, "commit", "-q", "-m", "process item")
	git(t, w, "push", "-q", "origin", "main")
	git(t, w, "checkout", "-q", "-b", "pt2", "origin/"+programTrunk)
	_ = os.WriteFile(filepath.Join(w, progFile), []byte("p\n"), 0o644)
	git(t, w, "add", progFile)
	git(t, w, "commit", "-q", "-m", "program item")
	git(t, w, "push", "-q", "origin", "pt2:"+programTrunk)
	git(t, w, "fetch", "-q", "origin")
	mainSHA := fullHead(t, w, "origin/main")
	branch := "sync/program-" + mainSHA[:12]
	git(t, w, "checkout", "-q", "-b", branch, "origin/"+programTrunk)
	if conflict {
		// The merge stops on the conflict; the commit below concludes it.
		_ = gitCmd(w, "merge", "-q", "--no-edit", "origin/main").Run()
		_ = os.WriteFile(filepath.Join(w, "both.txt"), []byte("p\nm\n"), 0o644)
		git(t, w, "add", "both.txt")
		git(t, w, "commit", "-q", "--no-edit")
	} else {
		git(t, w, "merge", "-q", "--no-edit", "origin/main")
	}
	git(t, w, "push", "-q", "origin", branch)
	git(t, w, "fetch", "-q", "origin")
	head := fullHead(t, w, branch)
	g.gh.view = PRView{Base: programTrunk, Head: head, Branch: branch}
	recordSync(t, g.env, branch, head)
	return g.env, g.gh, mainSHA
}

// recordSync records the sync head of the branch, as wf sync-program -head
// does.
func recordSync(t *testing.T, env Env, branch, head string) {
	t.Helper()
	if err := queue.AppendRecord(env.Workflow, queue.Record{By: "cloud/run",
		Kind: "sync", Repo: hereRepo, Ref: branch, Head: head,
		Note: "sync pushed"}); err != nil {
		t.Fatal(err)
	}
}

// TestSyncReady: the sync gate passes a recorded sync head that merges
// origin/main into the program trunk and nothing more, and fails one whose
// head is main's own commit, a moved head, a failed required check, an
// unrecorded head, a head with a commit beyond the merge, or a branch naming
// another main commit (FLOW-20, Decision 14).
func TestSyncReady(t *testing.T) {
	env, _, _ := syncFixture(t, false)
	if rs := SyncReady(env, hereRepo, 7); !OK(rs) {
		t.Fatalf("a sync head: %s", failures(rs))
	}
	for name, change := range map[string]func(t *testing.T, env Env, gh *fakeGitHub,
		mainSHA string){
		"main's own commit": func(_ *testing.T, _ Env, gh *fakeGitHub, mainSHA string) {
			gh.view.Head, gh.view.Branch = mainSHA, "main"
		},
		"a moved head": func(_ *testing.T, _ Env, gh *fakeGitHub, _ string) {
			gh.view.Head = strings.Repeat("b", 40)
		},
		"a failed required check": func(_ *testing.T, _ Env, gh *fakeGitHub, _ string) {
			gh.runs[0].Conclusion = "failure"
		},
		"an unrecorded head": func(t *testing.T, env Env, _ *fakeGitHub, _ string) {
			p := filepath.Join(env.Workflow, filepath.FromSlash(queue.RecordsFile))
			_ = os.Remove(p)
		},
		"a commit beyond the merge": func(t *testing.T, env Env, gh *fakeGitHub,
			_ string) {
			w := env.Repo
			_ = os.WriteFile(filepath.Join(w, "extra.txt"), []byte("x\n"), 0o644)
			git(t, w, "add", "extra.txt")
			git(t, w, "commit", "-q", "-m", "extra")
			git(t, w, "push", "-q", "origin", gh.view.Branch)
			git(t, w, "fetch", "-q", "origin")
			gh.view.Head = fullHead(t, w, gh.view.Branch)
			recordSync(t, env, gh.view.Branch, gh.view.Head)
		},
		"a branch naming another commit": func(t *testing.T, env Env, gh *fakeGitHub,
			_ string) {
			other := "sync/program-" + strings.Repeat("c", 12)
			git(t, env.Repo, "push", "-q", "origin", gh.view.Head+":refs/heads/"+other)
			gh.view.Branch = other
			recordSync(t, env, other, gh.view.Head)
		},
	} {
		env, gh, mainSHA := syncFixture(t, false)
		change(t, env, gh, mainSHA)
		if rs := SyncReady(env, hereRepo, 7); OK(rs) {
			t.Errorf("%s: the sync gate passed", name)
		}
	}
}

// TestSyncReadyResolution: a sync whose merge resolved a conflict passes
// only once the tier-2 report of its head has no open MUST finding (point 5,
// FLOW-08, FLOW-20).
func TestSyncReadyResolution(t *testing.T) {
	env, gh, _ := syncFixture(t, true)
	if got := failures(SyncReady(env, hereRepo, 7)); !strings.Contains(got, "sync gate 5") {
		t.Fatalf("a resolution with no report: %q", got)
	}
	write := func(findings []map[string]any) {
		data, _ := json.Marshal(map[string]any{"head": gh.view.Head[:12],
			"findings": findings})
		dir := filepath.Join(env.Workflow, filepath.FromSlash(SyncReportDir))
		_ = os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(filepath.Join(dir, gh.view.Head[:12]+".json"), data,
			0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(openMust("ORCH-02"))
	if got := failures(SyncReady(env, hereRepo, 7)); !strings.Contains(got, "sync gate 5") {
		t.Fatalf("a resolution with an open MUST: %q", got)
	}
	write(nil)
	if rs := SyncReady(env, hereRepo, 7); !OK(rs) {
		t.Fatalf("a reviewed resolution: %s", failures(rs))
	}
}

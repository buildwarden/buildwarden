package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// loopMerging is a program item at its merge step with its PR into the
// program trunk recorded, in a state with the program trunk configured and
// the gate answering pass.
func loopMerging(t *testing.T, pass bool) (*State, *Feature) {
	t.Helper()
	f := mergeWaiting(working("p", 1, 0, "relay"), 42)
	f.Program = true
	s := programState(t, f)
	s.MergeGate = func(*Feature, string) bool { return pass }
	return s, f
}

// TestLoopMergeOffered: a merge step on a program trunk is offered to a cloud
// worker only while the merge gate passes, and never otherwise; it does not
// wait on the human. Any other merge step is never offered and waits on the
// human (FLOW-04, FLOW-05, FLOW-08).
func TestLoopMergeOffered(t *testing.T) {
	s, f := loopMerging(t, true)
	if got := next(t, s, "cloud/run"); got != "p f7-merge" {
		t.Fatalf("cloud next = %s with the gate passing", got)
	}
	if got := next(t, s, "human/jeff"); got != "none" {
		t.Fatalf("human next = %s: the loop's merge is a cloud step", got)
	}
	if w := s.WaitsOnHuman(); len(w) != 0 {
		t.Fatalf("a loop merge waits on the human: %v", w[0].Feature)
	}
	st, _ := f.Step("f7-merge")
	if err := s.MayClaim(f, st, "cloud/run", now); err != nil {
		t.Fatalf("MayClaim: %v", err)
	}

	s, f = loopMerging(t, false)
	_, blocked := s.Next("cloud/run", now)
	if len(blocked) != 1 || !strings.Contains(blocked[0].Reason, "the merge gate in") {
		t.Fatalf("blocked = %v, want the merge gate", blocked)
	}
	st, _ = f.Step("f7-merge")
	if err := s.MayClaim(f, st, "cloud/run", now); err == nil {
		t.Fatal("MayClaim passed with the gate failing")
	}
	s.MergeGate = nil
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("cloud next = %s with no gate set", got)
	}

	// Not a program item: the human's merge, whatever the gate says.
	s, f = loopMerging(t, true)
	f.Program = false
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("cloud next = %s on main's merge", got)
	}
	if w := s.WaitsOnHuman(); len(w) != 1 {
		t.Fatal("main's merge does not wait on the human")
	}
	st, _ = f.Step("f7-merge")
	if err := s.MayClaim(f, st, "cloud/run", now); err == nil ||
		!strings.Contains(err.Error(), "never claimed") {
		t.Fatalf("MayClaim of the human's merge: %v", err)
	}
}

// TestLoopMergeTwoRepositories: a two-repository program item's f6-merged-1
// and f7-merge are both cloud merge steps, each under its repository's
// gate, and the item merges with no human step (FLOW-08, FLOW-20).
func TestLoopMergeTwoRepositories(t *testing.T) {
	f := working("p", 1, 0, "relay")
	f.Program = true
	f.Repos = []string{"buildwarden/buildwarden", "buildwarden/buildwarden-specs"}
	f.Work = []Work{{Repo: "buildwarden/buildwarden-specs", Branch: f.Branch, First: true,
		PR: &PR{Number: 5}}}
	advance(f, 6)
	s := programState(t, f)
	for i := range s.Config.Repositories {
		if s.Config.Repositories[i].Name == "buildwarden/buildwarden-specs" {
			s.Config.Repositories[i].ProgramTrunk = programTrunk
		}
	}
	gate := map[string]bool{}
	s.MergeGate = func(_ *Feature, repo string) bool { return gate[repo] }
	for i := range f.Steps {
		if st := &f.Steps[i]; st.Phase == 6 && !isMergedFirst(st) {
			st.Status = "done"
		}
	}
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("cloud next = %s before the specs gate passes", got)
	}
	gate["buildwarden/buildwarden-specs"] = true
	if got := next(t, s, "cloud/run"); got != "p f6-merged-1" {
		t.Fatalf("cloud next = %s, want the merged-first step", got)
	}
	st, _ := f.Step("f6-merged-1")
	f.Work[0].Merged = "0123456789abcdef0123456789abcdef01234567"
	Complete(f, st, "cloud/run", now)
	f.PR = &PR{Number: 42}
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("cloud next = %s before the home's gate passes", got)
	}
	gate["buildwarden/buildwarden"] = true
	if got := next(t, s, "cloud/run"); got != "p f7-merge" {
		t.Fatalf("cloud next = %s, want f7-merge", got)
	}
	for _, st := range f.Steps {
		if st.Status == "pending" && st.Affinity == "human" {
			t.Fatalf("a human step: %s", st.ID)
		}
	}
}

// TestLoopMergeWaitsOnHuman: a loop merge the gate holds on the human, at
// point 7 on a posture change or at point 5 on a protected finding, counts
// as waiting on him; one held on anything else waits on no one (FLOW-05,
// FLOW-20).
func TestLoopMergeWaitsOnHuman(t *testing.T) {
	for name, held := range map[string]bool{"held on the human": true,
		"held on the gate alone": false} {
		s, _ := loopMerging(t, false)
		s.MergeWaitsOnHuman = func(*Feature, string) bool { return held }
		if got := len(s.WaitsOnHuman()) == 1; got != held {
			t.Errorf("%s: waits on the human = %v", name, got)
		}
	}
}

// TestLoopMergeClaimedAgain: a loop merge's claim that lost a push race is
// asked again on the reloaded state with the merge gate wf commit wires on
// it, so it stands while the gate passes; with none wired it is refused
// (FLOW-04, FLOW-08).
func TestLoopMergeClaimedAgain(t *testing.T) {
	for name, wired := range map[string]bool{"wired": true, "not wired": false} {
		s, f := loopMerging(t, true)
		s.Dir = t.TempDir()
		_ = os.MkdirAll(filepath.Join(s.Dir, "queue"), 0o755)
		cfg, err := yaml.Marshal(s.Config)
		if err != nil {
			t.Fatal(err)
		}
		areas, err := os.ReadFile(filepath.Join("testdata", "areas.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(s.Dir, "config.yaml"), cfg, 0o644)
		_ = os.WriteFile(filepath.Join(s.Dir, "areas.yaml"), areas, 0o644)
		if err := s.Save(f); err != nil {
			t.Fatal(err)
		}
		r := CommitRequest{Worker: "cloud/run", Now: now}
		if wired {
			r.Wire = func(s *State) {
				s.MergeGate = func(*Feature, string) bool { return true }
			}
		}
		_, err = stateGit{s.Dir}.claimAgain(r, &claimRef{feature: f.Feature,
			step: "f7-merge"})
		if wired && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !wired && err == nil {
			t.Errorf("%s: claimed again with no merge gate", name)
		}
	}
}

// TestLoopMergeOneAtATime: while one item's loop merge into a program trunk
// is claimed under a live lease, no other loop merge in that repository is
// offered or claimed, and wf next says why; one in another repository is,
// and so is the same one once the lease expires (FLOW-08).
func TestLoopMergeOneAtATime(t *testing.T) {
	s, p, q := oneAtATime(t)
	pm, _ := p.Step("f7-merge")
	if err := s.MayClaim(p, pm, "cloud/a", now); err != nil {
		t.Fatal(err)
	}
	s.Claim(p, pm, "cloud/a", now)

	pick, _ := s.NextFor("r", "cloud/b", now)
	if pick == nil || pick.Step.ID != "f6-merged-1" {
		t.Fatalf("pick = %+v, want r's merge in another repository", pick)
	}
	pick, blocked := s.NextFor("q", "cloud/b", now)
	if pick != nil || len(blocked) != 1 ||
		!strings.Contains(blocked[0].Reason, "the loop merge p f7-merge in "+
			"buildwarden/buildwarden") ||
		!strings.Contains(blocked[0].Reason, "one merge into a program trunk at a time") {
		t.Fatalf("q: pick %+v, blocked %v", pick, blocked)
	}
	qm, _ := q.Step("f7-merge")
	if err := s.MayClaim(q, qm, "cloud/b", now); err == nil ||
		!strings.Contains(err.Error(), "at a time") {
		t.Fatalf("MayClaim of a second merge in the repository: %v", err)
	}
	if err := s.MayClaim(p, pm, "cloud/a", now); err != nil {
		t.Fatalf("the holder's own claim: %v", err)
	}
	later := pm.Lease.Expires.Add(1)
	if err := s.MayClaim(q, qm, "cloud/b", later); err != nil {
		t.Fatalf("after the lease expired: %v", err)
	}
}

// oneAtATime is a state with two program items at their merge step in the
// queue's own repository, p and q, and r at its merged-first step in the
// specifications repository, each on a program trunk whose gate passes.
func oneAtATime(t *testing.T) (*State, *Feature, *Feature) {
	t.Helper()
	p := mergeWaiting(working("p", 1, 0, "relay"), 42)
	q := mergeWaiting(working("q", 2, 0, "relay"), 43)
	r := working("r", 3, 0, "relay")
	r.Repos = []string{"buildwarden/buildwarden", "buildwarden/buildwarden-specs"}
	r.Work = []Work{{Repo: "buildwarden/buildwarden-specs", Branch: r.Branch, First: true,
		PR: &PR{Number: 5}}}
	advance(r, 6)
	for i := range r.Steps {
		if st := &r.Steps[i]; st.Phase == 6 && !isMergedFirst(st) {
			st.Status = "done"
		}
	}
	p.Program, q.Program, r.Program = true, true, true
	s := programState(t, p, q, r)
	for i := range s.Config.Repositories {
		if s.Config.Repositories[i].Name == "buildwarden/buildwarden-specs" {
			s.Config.Repositories[i].ProgramTrunk = programTrunk
		}
	}
	s.MergeGate = func(*Feature, string) bool { return true }
	return s, p, q
}

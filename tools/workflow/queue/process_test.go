package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func process(name string, prio int, status string, holds ...string) *Feature {
	f := &Feature{Feature: name, Priority: prio, Status: status, Kind: "process",
		Round: "process", Phase: 1, Holds: holds,
		Budget: Budget{Estimate: Count{Runs: 1, Steps: 4}}}
	Seed(f)
	return f
}

func TestProcessTemplateHumanDesignThenCloud(t *testing.T) {
	f := process("x", 1, "in_progress")
	var ids []string
	for f.Status != "done" {
		st := &f.Steps[len(f.Steps)-1]
		want := "cloud"
		if st.Phase == 1 {
			want = "human"
		}
		if st.Affinity != want || !IsGate(f, st) {
			t.Fatalf("step %+v: want a %s gate", st, want)
		}
		ids = append(ids, st.ID)
		Complete(f, st, "test", now)
	}
	if got := strings.Join(ids, " "); got != "x1-design x2-implement x3-merge x4-close" {
		t.Fatalf("steps = %q", got)
	}
	if errs := state(f).Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

func TestProcessSortsFirstAndStartsOnAnyClaim(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "queue"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("run_budget: {steps: 8}\n"),
		0o644)
	s := &State{Dir: dir}
	for _, f := range []*Feature{feature("a", 1, "in_progress"),
		process("p", 9, "queued")} {
		if err := s.Save(f); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Features[0].Feature != "p" {
		t.Fatalf("order = %s, %s; want the process item first", s.Features[0].Feature,
			s.Features[1].Feature)
	}
	if got := next(t, s, "human/jeff"); got != "p x1-design" {
		t.Fatalf("human next = %q, want the process item's step", got)
	}
	p := s.Features[0]
	s.Claim(p, &p.Steps[0], "human/jeff", now, true)
	if p.Status != "in_progress" {
		t.Fatalf("status = %s; a human claim starts a process item", p.Status)
	}
}

func TestHoldsBlockMatchingWorkersUntilDone(t *testing.T) {
	a := feature("a", 1, "in_progress")
	a.Steps = append(a.Steps, Step{ID: "f0-on-mac", Affinity: "host:macos-vz",
		Status: "pending"})
	p := process("rules", 2, "in_progress", "cloud", "host:*")
	s := state(a, p)
	s.sortFeatures()

	pick, blocked := s.Next("cloud/run", now)
	if pick != nil || len(blocked) != 2 ||
		blocked[1].Reason != "held by process item rules" {
		t.Fatalf("cloud next = %+v, %+v; want a held by rules", pick, blocked)
	}
	if got := next(t, s, "host:macos-vz/run"); got != "none" {
		t.Fatalf("host next = %q, want none under host:*", got)
	}
	if got := next(t, s, "human/jeff"); got != "rules x1-design" {
		t.Fatalf("human next = %q; human is not held", got)
	}
	if s.Hold(p, "cloud") != "" {
		t.Fatal("a process item held its own steps")
	}

	// A queued feature does not start under the hold either.
	s.Features = append(s.Features, feature("q", 3, "queued"))
	a.Status = "done"
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("cloud next = %q, want no new feature under the hold", got)
	}
	a.Status = "in_progress"

	// Finishing the process item releases the hold.
	for p.Status != "done" {
		Complete(p, &p.Steps[len(p.Steps)-1], "human/jeff", now)
	}
	if got := next(t, s, "cloud/run"); got != "a f0-branch-setup" {
		t.Fatalf("cloud next after release = %q", got)
	}
}

func TestHoldLeavesAClaimedStepClaimed(t *testing.T) {
	a := feature("a", 1, "in_progress")
	s := state(a)
	s.Claim(a, &a.Steps[0], "cloud/run", now, true)
	p := process("rules", 2, "in_progress", "cloud")
	s.Features = append(s.Features, p)
	s.sortFeatures()

	if got := next(t, s, "cloud/run"); got != "a f0-branch-setup" {
		t.Fatalf("next = %q, want the step cloud/run already holds", got)
	}
	if got := next(t, s, "cloud/other"); got != "none" {
		t.Fatalf("another cloud worker got %q under the hold", got)
	}
	if !a.Steps[0].Lease.Held(now) || a.Steps[0].Lease.ClaimedBy != "cloud/run" {
		t.Fatalf("lease = %+v; the hold must not drop it", a.Steps[0].Lease)
	}
	pick, _ := s.NextFor("a", "cloud/run", now)
	if pick == nil || pick.Step.ID != "f0-branch-setup" {
		t.Fatalf("NextFor = %+v, want the leased step", pick)
	}
	if _, blocked := s.NextFor("a", "cloud/other", now); len(blocked) != 1 ||
		!strings.Contains(blocked[0].Reason, "held by process item rules") {
		t.Fatalf("NextFor blocked = %+v", blocked)
	}
}

func TestGateHonorsHolds(t *testing.T) {
	repo, _ := gitRepo(t)
	a := feature("a", 1, "in_progress")
	a.Branch = "feat/a"
	a.Steps[0].Affinity = "host:macos-qemu"
	s := nativeState(a, process("rules", 2, "in_progress", "host:macos-*"))
	s.sortFeatures()
	s.Gaps = []Gap{{ID: "g1", Host: "macos-vz", Status: "open"}}
	mac, _ := s.Machine("mac")
	g, err := s.Gate(mac, repo, nil, 7, now)
	if err != nil || g.Run {
		t.Fatalf("gate = %+v, %v; want idle while the mac hosts are held", g, err)
	}
	if !strings.Contains(strings.Join(g.Reasons, "\n"), "by process item rules") {
		t.Fatalf("reasons = %q, want the hold", g.Reasons)
	}
}

func TestValidateKindAndHolds(t *testing.T) {
	f := feature("f", 1, "queued")
	f.Holds = []string{"cloud"}
	p := process("p", 2, "queued", "cloud", "host:[", "nobody")
	p2 := process("p2", 3, "queued")
	p2.Round = "parallel"
	k := feature("k", 4, "queued")
	k.Kind = "chore"
	got := ""
	for _, e := range state(f, p, p2, k).Validate() {
		got += e.Error() + "\n"
	}
	for _, want := range []string{"f: only a process item holds workers",
		`p: hold "host:["`, `p: hold "nobody"`, "p2: a process item's round is process",
		`k: kind "chore"`} {
		if !strings.Contains(got, want) {
			t.Errorf("Validate = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, `"cloud"`) {
		t.Errorf("Validate = %q; cloud is a valid hold", got)
	}
}

func TestAddProcessItem(t *testing.T) {
	s := state(feature("a", 1, "in_progress"))
	p := &Feature{Feature: "rules", Priority: 2, Kind: "process", Holds: []string{"cloud"}}
	if _, err := s.Add(p, now, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	if p.Round != "process" || p.Phase != 1 || p.Steps[0].ID != "x1-design" ||
		s.Features[0] != p {
		t.Fatalf("added %+v; want round process, x1-design, sorted first", p)
	}
}

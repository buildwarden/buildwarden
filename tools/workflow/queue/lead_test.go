package queue

import (
	"strings"
	"testing"
	"time"
)

const lead = "host:windows-hyperv"

// ledFeature is a queued feature led by lead, seeded as Add seeds it.
func ledFeature(name string, prio int) *Feature {
	f := &Feature{Feature: name, Priority: prio, Status: "queued", Round: "feature",
		Lead: lead, Budget: Budget{Estimate: Count{Runs: 1, Steps: 7}}}
	Seed(f)
	return f
}

// affinities lists each step's id and affinity.
func affinities(f *Feature) string {
	var out []string
	for _, st := range f.Steps {
		out = append(out, st.ID+":"+st.Affinity)
	}
	return strings.Join(out, " ")
}

// TestSeedLed: a led feature's cloud template steps of phases 0 to 5 carry
// its lead, its design review stays human, and Publish, Merge and Close stay
// cloud; a return to Design seeds the lead again (FLOW-04, FLOW-09).
func TestSeedLed(t *testing.T) {
	f := ledFeature("k3", 1)
	f.Status = "in_progress"
	advance(f, 8)
	const want = "f0-branch-setup:" + lead + " f1-design:" + lead +
		" f2-design-review:human f2-plan:" + lead + " f3-implement:" + lead +
		" f4-review:" + lead + " f5-guide:" + lead + " f6-merge-main:cloud" +
		" f6-consolidate:cloud f6-open-pr:cloud f7-merge:cloud f8-close:cloud"
	if got := affinities(f); got != want {
		t.Fatalf("seeded\n%s\nwant\n%s", got, want)
	}

	g := ledFeature("k4", 2)
	g.Status = "in_progress"
	advance(g, 3)
	Back(g, "cloud/run", now, "the design missed a case", 1)
	if st, err := g.Step("f1-design-r2"); err != nil || st.Affinity != lead {
		t.Fatalf("Back re-seeded %+v (%v), want f1-design-r2 led", st, err)
	}
}

// TestLedStart: a worker that serves the lead starts a queued led feature,
// under the guard; the cloud does not, and reports the host step; a host
// that does not serve the lead is not offered it. A human still starts a
// queued process item and the cloud an unled feature (FLOW-04, FLOW-05).
func TestLedStart(t *testing.T) {
	k3 := ledFeature("k3", 1)
	s := state(k3)
	p, blocked := s.Next("cloud/run", now)
	if p != nil || len(blocked) != 1 ||
		blocked[0].Reason != "waiting on "+lead+" step f0-branch-setup" {
		t.Fatalf("cloud: pick %+v, blocked %+v", p, blocked)
	}
	wantNext(t, s, "host:macos-vz/macbook.r1", "none")
	p, _ = s.Next(lead+"/windows-desktop.r1", now)
	if p == nil || p.Step.ID != "f0-branch-setup" || !p.Starts {
		t.Fatalf("lead's machine: pick %+v, want a start on f0-branch-setup", p)
	}
	s.Claim(p.Feature, p.Step, lead+"/windows-desktop.r1", now, true)
	if k3.Status != "in_progress" {
		t.Fatalf("the lead's claim left k3 %s", k3.Status)
	}

	// The guard bounds the lead's start as it does the cloud's.
	k4 := ledFeature("k4", 2)
	s = state(k3, k4)
	inProgress(s, 1)
	wantNext(t, s, lead+"/windows-desktop.r2", "none")

	plain := feature("plain", 1, "queued")
	if s := state(plain); !s.Starts(plain, "cloud/run") || s.Starts(plain, lead+"/m.r") {
		t.Fatal("an unled feature is started by the cloud's claim alone")
	}
	x := process("x", 2, "queued")
	if !state(x).Starts(x, "human/jeff") {
		t.Fatal("a human claim no longer starts a queued process item")
	}
}

// TestValidateLead: a lead is host:<name>, only on a feature homed here, and
// some machine serves it when hosts.yaml lists any (FLOW-04, FLOW-09).
func TestValidateLead(t *testing.T) {
	x := process("x", 1, "queued")
	x.Lead = lead
	wantValidate(t, state(x),
		"x: lead on a process item: a process item is never led (FLOW-09)")

	bad := ledFeature("bad", 1)
	bad.Lead = "cloud"
	wantValidate(t, state(bad), `bad: lead "cloud" is not host:<name> (FLOW-04)`)

	s := declaredState()
	away := &Feature{Feature: "m1", Priority: 1, Repos: []string{"o/relay"}}
	if _, err := s.Add(away, now, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	away.Lead = lead
	wantValidate(t, s, "m1: lead on a tracked item: its steps are all human (FLOW-09)")
}

// TestSetLead: any worker sets or clears a lead; the pending template steps
// of phases 0 to 5 that no lease holds move between cloud and the lead, a
// held one keeps its affinity, and one history line names the old and new
// lead and the steps changed and left (FLOW-04, FLOW-09).
func TestSetLead(t *testing.T) {
	t.Run("any worker", setLeadAnyWorker)
	t.Run("re-labels", setLeadRelabels)
	t.Run("refusals", setLeadRefusals)
}

func setLeadAnyWorker(t *testing.T) {
	for _, w := range []string{"human/jeff", "cloud/run", "host:macos-vz/macbook.r1"} {
		g := feature("g", 2, "queued")
		if err := state(g).SetLead(g, w, lead, now); err != nil {
			t.Fatalf("%s: %v", w, err)
		}
		if g.Lead != lead || g.Steps[0].Affinity != lead {
			t.Fatalf("%s: lead %q, %s", w, g.Lead, affinities(g))
		}
	}
}

// lastLine is the feature's newest history line.
func lastLine(f *Feature) string { return f.History[len(f.History)-1].Event }

func setLeadRelabels(t *testing.T) {
	f := working("k3", 1, 2)
	s := state(f)
	plan, _ := f.Step("f2-plan")
	s.Claim(f, plan, "cloud/run", now, true)
	if err := s.SetLead(f, "cloud/run", lead, now); err != nil {
		t.Fatal(err)
	}
	const left = "lead none -> " + lead + " (FLOW-04); left f2-plan (leased to cloud/run)"
	if plan.Affinity != "cloud" || lastLine(f) != left {
		t.Fatalf("f2-plan %s, history %q, want it leased and left", plan.Affinity,
			lastLine(f))
	}

	s.Release(f, plan, "cloud/run", now, "test")
	// Cleared, the lead leaves f2-plan cloud; set again, it moves it.
	if err := s.SetLead(f, "human/jeff", "", now.Add(time.Minute)); err != nil ||
		f.Lead != "" || plan.Affinity != "cloud" {
		t.Fatalf("clear: %v, lead %q, f2-plan %s", err, f.Lead, plan.Affinity)
	}
	if err := s.SetLead(f, "human/jeff", lead, now); err != nil {
		t.Fatal(err)
	}
	if plan.Affinity != lead || !strings.HasSuffix(lastLine(f), "; re-labelled f2-plan") {
		t.Fatalf("f2-plan %s, history %q", plan.Affinity, lastLine(f))
	}
}

func setLeadRefusals(t *testing.T) {
	f := ledFeature("k3", 1)
	s := state(f)
	for _, c := range []struct{ lead, want string }{
		{lead, "k3's lead is already " + lead},
		{"windows", `lead "windows" is not host:<name> (FLOW-04)`},
	} {
		if err := s.SetLead(f, "cloud/run", c.lead, now); err == nil ||
			err.Error() != c.want {
			t.Fatalf("SetLead(%q) = %v, want %q", c.lead, err, c.want)
		}
	}
	s.Machines = []Machine{{Name: "macbook", Hosts: []string{"macos-vz"}}}
	if err := s.SetLead(f, "cloud/run", "", now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLead(f, "cloud/run", lead, now); err == nil {
		t.Fatal("a lead no machine serves was set")
	}
	x := process("x", 3, "queued")
	if err := s.SetLead(x, "human/jeff", lead, now); err == nil {
		t.Fatal("a process item was led")
	}
}

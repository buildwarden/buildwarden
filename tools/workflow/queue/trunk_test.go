package queue

import (
	"strings"
	"testing"
)

// programTrunk is the program trunk the tests' entries record.
const programTrunk = "program/componentization"

// programState is the design map's state with a program trunk recorded on
// the queue's own repository's entry.
func programState(t *testing.T, fs ...*Feature) *State {
	t.Helper()
	s := mapped(t, fs...)
	s.Config.Repositories[0].ProgramTrunk = programTrunk
	return s
}

// programItem is a program feature in Design with the areas declared.
func programItem(name string, prio int, areas ...string) *Feature {
	f := designing(name, prio, areas...)
	f.Program = true
	return f
}

// TestTrunkFor: the program trunk only for a program item that is not a
// process item and reaches no machinery, .github/ or .goreleaser.yaml path,
// in a repository whose entry has one; main for every other (FLOW-20).
func TestTrunkFor(t *testing.T) {
	plain := designing("plain", 3, "relay")
	proc := process("proc", 4, "in_progress")
	proc.Program = true
	for name, tc := range map[string]struct {
		f    *Feature
		want string
	}{
		"a program item":            {programItem("p", 1, "relay"), programTrunk},
		"undeclared":                {programItem("u", 1), programTrunk},
		"one that reaches .github":  {programItem("g", 1, "relay", "build"), "main"},
		"one that reaches the loop": {programItem("l", 1, "loop"), "main"},
		"not a program item":        {plain, "main"},
		"a process item":            {proc, "main"},
	} {
		s := programState(t, tc.f)
		got, err := s.TrunkFor(s.Config.Repository, tc.f)
		if err != nil || got != tc.want {
			t.Errorf("%s: TrunkFor = %q, %v; want %q", name, got, err, tc.want)
		}
	}
	f := programItem("p", 1, "relay")
	s := mapped(t, f)
	if got, _ := s.TrunkFor(s.Config.Repository, f); got != "main" {
		t.Errorf("no program trunk recorded: TrunkFor = %q, want main", got)
	}
}

// TestTrunkPinned: the trunk is recorded at the first declare and returned
// from then on; a declaration that would flip it is refused, and a later
// change elsewhere that would choose another trunk leaves the recorded one in
// force and the state valid, so it locks no push (FLOW-06, FLOW-20).
func TestTrunkPinned(t *testing.T) {
	f := programItem("p", 1, "relay")
	s := programState(t, f)
	wantDeclared(t, s, f, declaration("relay"), "cloud/run")
	if f.Trunk != programTrunk {
		t.Fatalf("recorded trunk %q, want %s", f.Trunk, programTrunk)
	}
	wantUndeclared(t, s, f, declaration("relay", "build"), "cloud/run",
		"would change the trunk the item recorded")
	f.Program = false
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("Validate = %v: a flip found outside wf declare fails the state", errs)
	}
	if got, _ := s.TrunkFor(s.Config.Repository, f); got != programTrunk {
		t.Fatalf("TrunkFor = %q after the flip, want the recorded %s", got, programTrunk)
	}
	f.Program = true

	// Taken off config.yaml, the program trunk is no longer read: the
	// item is back on main (the design's rollback).
	s.Config.Repositories[0].ProgramTrunk = ""
	if got, _ := s.TrunkFor(s.Config.Repository, f); got != "main" {
		t.Fatalf("with no program trunk configured TrunkFor = %q", got)
	}
}

// TestProgramTrunkIsNotTheTrunk: Validate refuses a program trunk equal to
// the trunk, and a program mark on a process item (FLOW-20).
func TestProgramTrunkIsNotTheTrunk(t *testing.T) {
	s := programState(t)
	s.Config.Repositories[0].ProgramTrunk = "main"
	if errs := s.Validate(); len(errs) == 0 ||
		!strings.Contains(errs[0].Error(), "a program trunk is not a trunk") {
		t.Fatalf("Validate = %v", errs)
	}
	proc := process("proc", 1, "queued")
	proc.Program = true
	s = programState(t, proc)
	if errs := s.Validate(); len(errs) == 0 ||
		!strings.Contains(errs[0].Error(), "a process item keeps the trunk") {
		t.Fatalf("Validate = %v", errs)
	}
}

// TestRefreshWaitsForItsOwnTrunk: the red-trunk hold reads program.yaml for
// an item on the program trunk and main.yaml for one on main (FLOW-10).
func TestRefreshWaitsForItsOwnTrunk(t *testing.T) {
	p := openPR("p", 1, 7)
	p.Program = true
	refresh(t, p)
	s := programState(t, p)
	s.Main = &Main{SHA: "abc1234", CI: "red"}
	if got := next(t, s, "cloud/run"); got != "p f6-refresh" {
		t.Fatalf("next = %s: main's red holds a program item's refresh", got)
	}
	s.ProgramMain = &Main{SHA: "def5678", CI: "red"}
	if got := next(t, s, "cloud/run"); got != "none" {
		t.Fatalf("next = %s under a red program trunk", got)
	}
}

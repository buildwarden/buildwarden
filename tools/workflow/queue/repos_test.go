package queue

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// twoMachines is a state where machines a and b both serve container-any,
// and a also lists o/relay.
func twoMachines(fs ...*Feature) *State {
	s := own(state(fs...), "o/here")
	s.Config.PriorityConfirmed = true
	s.Machines = []Machine{
		{Name: "a", Hosts: []string{"container-any"},
			Repositories: []string{"o/here", "o/relay"}},
		{Name: "b", Hosts: []string{"container-any"}},
	}
	return s
}

// relayStep is an item homed here, working in o/relay too, in Implement
// with one pending host step in o/relay.
func relayStep() *Feature {
	f := feature("r", 1, "in_progress")
	f.Branch, f.Repos = "feat/r", []string{"o/here", "o/relay"}
	f.Work = []Work{{Repo: "o/relay", Branch: "feat/r"}}
	f.Steps = []Step{{ID: "f3-validate-container-any", Phase: 0,
		Affinity: "host:container-any", Status: "pending", Repo: "o/relay"}}
	return f
}

// TestHostStepPerRepository: a host step goes only to a machine that serves
// its host name and lists its repository, and wf validate fails one no such
// machine can take, naming every serving machine (Validation plan test 9,
// FLOW-04).
func TestHostStepPerRepository(t *testing.T) {
	f := relayStep()
	s := twoMachines(f)
	if got := next(t, s, "host:container-any/a.run1"); got != "r f3-validate-container-any" {
		t.Errorf("machine a: next %q", got)
	}
	if got := next(t, s, "host:container-any/b.run1"); got != "none" {
		t.Errorf("machine b, which does not list o/relay: next %q", got)
	}
	wantValidate(t, s)
	s.Machines[0].Repositories = nil
	wantValidate(t, s, "r: step f3-validate-container-any: no machine serving host "+
		"container-any lists o/relay in hosts.yaml (a lacks o/relay; b lacks o/relay): "+
		"Jeff adds them to one machine's repositories (FLOW-04)")
	if got := next(t, s, "host:container-any/a.run1"); got != "none" {
		t.Errorf("neither listing it: next on a %q", got)
	}
	// One machine serving the name: the same holds for it alone.
	s.Machines = s.Machines[:1]
	wantValidate(t, s, "r: step f3-validate-container-any: no machine serving host "+
		"container-any lists o/relay in hosts.yaml (a lacks o/relay): "+
		"Jeff adds them to one machine's repositories (FLOW-04)")
}

// TestLedItemPerRepository: a led item's steps run in every repository it
// works in, so they go only to a machine that lists all of them; wf lead and
// wf declare refuse a lead no serving machine can take.
func TestLedItemPerRepository(t *testing.T) {
	led := feature("led", 1, "in_progress")
	led.Repos = []string{"o/here", "o/relay"}
	led.Work = []Work{{Repo: "o/relay"}}
	led.Lead = "host:container-any"
	led.relabel(now)
	other := feature("plain", 2, "in_progress")
	other.Steps = []Step{{ID: "f0-validate-container-any", Phase: 0,
		Affinity: "host:container-any", Status: "pending"}}
	s := twoMachines(led, other)
	wantValidate(t, s)
	if got := next(t, s, "host:container-any/a.run1"); got != "led f0-branch-setup" {
		t.Errorf("machine a: next %q", got)
	}
	got := next(t, s, "host:container-any/b.run1")
	if got != "plain f0-validate-container-any" {
		t.Errorf("machine b: next %q, want the other item's host step", got)
	}

	s.Machines[0].Repositories = []string{"o/here"}
	wantValidate(t, s, "led: lead: no machine serving host container-any lists o/here, "+
		"o/relay in hosts.yaml (a lacks o/relay; b lacks o/relay): Jeff adds them to "+
		"one machine's repositories (FLOW-04)")
	fresh := feature("fresh", 3, "queued")
	fresh.Repos = []string{"o/here", "o/relay"}
	s.Features = append(s.Features, fresh)
	err := s.SetLead(fresh, "cloud/run", "host:container-any", now)
	if err == nil || !strings.Contains(err.Error(), "or a worker clears the lead") {
		t.Errorf("wf lead with no machine for o/relay: %v", err)
	}
	s.Machines[0].Repositories = []string{"o/here", "o/relay"}
	if err := s.SetLead(fresh, "cloud/run", "host:container-any", now); err != nil {
		t.Errorf("wf lead once a lists o/relay: %v", err)
	}

	// wf declare keeps the lead, and refuses a work it cannot serve.
	s.Machines[0].Repositories = []string{"o/here"}
	plainLed := feature("pl", 4, "in_progress")
	plainLed.Branch, plainLed.Lead, plainLed.Phase = "feat/pl", "host:container-any", 1
	s.Features = append(s.Features, plainLed)
	before := *plainLed
	err = s.Declare(plainLed, Declaration{Commit: "4c1d9e2",
		Repos: []string{"o/here", "o/relay"}, Areas: &[]string{}}, "cloud/run", now)
	if err == nil || !strings.Contains(err.Error(), "wf declare changed nothing") {
		t.Fatalf("declare: %v", err)
	}
	if plainLed.Lead != before.Lead || !slices.Equal(plainLed.Repos, before.Repos) ||
		len(plainLed.Work) != 0 {
		t.Errorf("a refused declare wrote %+v", plainLed)
	}
}

// TestFingerprintPerRepository: the gate's fingerprint covers the trunk of
// each repository the machine lists, in its clone.
func TestFingerprintPerRepository(t *testing.T) {
	repo, _ := gitRepo(t)
	clone := filepath.Join(repo, ".dev", "checkouts", "o", "relay")
	_ = os.MkdirAll(clone, 0o755)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", clone}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "one")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	s := twoMachines(relayStep())
	a, b := &s.Machines[0], &s.Machines[1]
	before, err := s.fingerprint(a, repo)
	if err != nil {
		t.Fatal(err)
	}
	onlyHere, _ := s.fingerprint(b, repo)
	git("commit", "-q", "--allow-empty", "-m", "two")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	if after, _ := s.fingerprint(a, repo); after == before {
		t.Error("a's fingerprint did not change when o/relay's trunk moved")
	}
	if after, _ := s.fingerprint(b, repo); after != onlyHere {
		t.Error("b's fingerprint changed, though b does not list o/relay")
	}
}

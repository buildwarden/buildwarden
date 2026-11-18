package main

import (
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const machineWorker = "host:macos-vz/macbook.r1"

// hostItems are checkouts a and b whose items x and y have a host first step,
// x's claimed by machineWorker in a and pushed, and b brought up to date.
func hostItems(t *testing.T) (a, b string) {
	t.Helper()
	a, b = stateRepos(t, "", "x", "y")
	for _, name := range []string{"x", "y"} {
		edit(t, a, name, func(f *queue.Feature) { f.Steps[0].Affinity = "host:macos-vz" })
	}
	pushed(t, a, "human/jeff", "host steps", "queue/x.yaml", "queue/y.yaml")
	claimStep(t, a, machineWorker, "x", "f0-branch-setup")
	pushed(t, a, machineWorker, "x: claim", "-claimed", "x/f0-branch-setup")
	gitT(t, b, "pull", "-q", "origin", "workflow")
	return a, b
}

// TestRenewCommands: wf renew renews a lease the worker holds, for the host
// lease, and refuses another's, an expired one and a done step; wf commit
// -renewed renews again on the state that won a rejected push, counts
// nothing and claims nothing, and is refused (exit 4) when the step was
// released in between (FLOW-04, FLOW-13).
func TestRenewCommands(t *testing.T) {
	t.Run("renew", renewRefusals)
	t.Run("renewed again", renewedAgain)
	t.Run("renewal refused", renewalRefused)
}

func renewRefusals(t *testing.T) {
	a, _ := hostItems(t)
	renew := func(worker string) []string {
		return []string{"-worker", worker, "-feature", "x", "-step", "f0-branch-setup"}
	}
	refused(t, a, "it has a lease of "+machineWorker, "renew",
		renew("host:macos-vz/macbook.r2")...)
	must(t, a, "renew", renew(machineWorker)...)
	st, _ := item(t, a, "x").Step("f0-branch-setup")
	if got := st.Lease.Expires.Sub(now); got != 20*time.Hour {
		t.Fatalf("renewed for %v, want 20h", got)
	}
	refused(t, a, "may not take", "renew", renew("cloud/run")...)
}

func renewedAgain(t *testing.T) {
	a, b := hostItems(t)
	edit(t, b, "y", func(f *queue.Feature) { f.Title = "B's title" })
	pushed(t, b, "human/jeff", "y: title", "queue/y.yaml")
	before := item(t, a, "x")
	must(t, a, "renew", "-worker", machineWorker, "-feature", "x", "-step",
		"f0-branch-setup")
	out := pushed(t, a, machineWorker, "x: renew", "-renewed", "x/f0-branch-setup")
	if !strings.Contains(out, "renewed again on the new state: x/f0-branch-setup") {
		t.Fatalf("wf commit did not renew again:\n%s", out)
	}
	gitT(t, a, "pull", "-q", "origin", "workflow")
	x := item(t, a, "x")
	if x.Budget.Spent != before.Budget.Spent {
		t.Fatalf("the renewal spent %+v (was %+v)", x.Budget.Spent, before.Budget.Spent)
	}
	ev := events(x)
	claims := 0
	for _, e := range ev {
		if strings.Contains(e, "claimed f0-branch-setup") {
			claims++
		}
	}
	if claims != 1 || !strings.HasPrefix(ev[len(ev)-1],
		machineWorker+": renewed f0-branch-setup until ") {
		t.Fatalf("history %q, want one claim and a renewal last", ev)
	}
	if st, _ := x.Step("f0-branch-setup"); st.Lease.Expires.Sub(now) != 20*time.Hour {
		t.Fatalf("the lease runs %v", st.Lease.Expires.Sub(now))
	}
	if item(t, a, "y").Title != "B's title" {
		t.Fatal("the change that won was lost")
	}
}

func renewalRefused(t *testing.T) {
	a, b := hostItems(t)
	must(t, b, "release", "-session", "macbook.r1", "-why", "session ended (exit 1)")
	pushed(t, b, "native/macbook.r1", "release", "queue/x.yaml")
	must(t, a, "renew", "-worker", machineWorker, "-feature", "x", "-step",
		"f0-branch-setup")
	out, code := commit(t, a, machineWorker, "x: renew", "-renewed", "x/f0-branch-setup")
	if code != 4 || !strings.Contains(out, "the renewal of x/f0-branch-setup is refused "+
		"on the new state") {
		t.Fatalf("exit %d, want 4 refusing the renewal:\n%s", code, out)
	}
	if out, code := commit(t, a, machineWorker, "x", "-renewed", "x/s", "-claimed",
		"x/s"); code != 1 || !strings.Contains(out, "not both") {
		t.Fatalf("-renewed with -claimed: exit %d: %s", code, out)
	}
}

// TestReleaseCommands: release -session frees one session's leases; release
// -machine -stale frees an earlier scheduled session's, sparing the current
// run and an attended session, and the gate then offers the step (FLOW-04).
func TestReleaseCommands(t *testing.T) {
	dir := stateDir(t, "priority_confirmed: true\n")
	write(t, dir, "hosts.yaml", "machines:\n  - name: macbook\n    hosts: [macos-vz]\n")
	for i, name := range []string{"old", "cur", "att"} {
		queued(t, dir, name, string(rune('1'+i)))
		edit(t, dir, name, func(f *queue.Feature) {
			f.Steps[0].Affinity = "host:macos-vz"
		})
	}
	for name, run := range map[string]string{"old": "macbook.r1", "cur": "macbook.r2",
		"att": "macbook.attended-20261115T090000Z"} {
		claimStep(t, dir, "host:macos-vz/"+run, name, "f0-branch-setup")
	}
	refused(t, dir, "release takes -session", "release", "-machine", "macbook", "-stale",
		"-why", "x")
	refused(t, dir, "release needs -why", "release", "-session", "macbook.r1")
	out := must(t, dir, "release", "-machine", "macbook", "-stale", "-current",
		"macbook.r2", "-why", "an earlier session on macbook left it")
	if out != "released\told\tf0-branch-setup\thost:macos-vz/macbook.r1\n" {
		t.Fatalf("release -stale printed %q", out)
	}
	if got := lastEvent(t, dir, "old"); got != "host:macos-vz/macbook.r2: released "+
		"f0-branch-setup: an earlier session on macbook left it" {
		t.Fatalf("history %q", got)
	}
	stamp := t.TempDir() + "/stamp.json"
	gated, err := wf(t, dir, "gate", "-repo", dir, "-machine", "macbook", "-stamp", stamp)
	if err != nil || !strings.Contains(gated, "step\thost:macos-vz\told\tf0-branch-setup") {
		t.Fatalf("gate after the release: %v\n%s", err, gated)
	}
	out = must(t, dir, "release", "-session", "macbook.attended-20261115T090000Z", "-why",
		"attended session closed")
	if !strings.HasPrefix(out, "released\tatt\t") {
		t.Fatalf("release -session printed %q", out)
	}
	if st, _ := item(t, dir, "cur").Step("f0-branch-setup"); st.Lease.ClaimedBy == "" {
		t.Fatal("the current run's lease was released")
	}
}

// openQuestion gives the item an open blocking question q1.
func openQuestion(f *queue.Feature) {
	f.Questions = append(f.Questions, queue.Question{ID: "q1", Kind: "blocking",
		Status: "open", Question: "Which?", Options: []string{"a"}, Recommended: "a"})
}

// TestHostClaimAskedAgain: a machine's claim whose push is rejected because
// another session pushed an open blocking question, or spent the item's
// runaway budget, is refused on the new state (exit 4) with the reason; a
// human's claim in the same situation is claimed again (FLOW-07, FLOW-13).
func TestHostClaimAskedAgain(t *testing.T) {
	spent := func(f *queue.Feature) { f.Budget.Spent.Steps = 18 }
	for _, c := range []hostClaim{
		{machineWorker, "host:macos-vz", "blocking question q1", openQuestion},
		{machineWorker, "host:macos-vz", "runaway budget", spent},
		{"human/jeff", "human", "", openQuestion},
	} {
		t.Run(c.worker+" "+c.want, c.run)
	}
}

// hostClaim is a claim by worker of a step of affinity, rejected because
// another session pushed win; want is the refusal, "" when it stands.
type hostClaim struct {
	worker, affinity, want string
	win                    func(*queue.Feature)
}

func (c hostClaim) run(t *testing.T) {
	a, b := stateRepos(t, "", "x")
	edit(t, a, "x", func(f *queue.Feature) { f.Steps[0].Affinity = c.affinity })
	pushed(t, a, "human/jeff", "x: affinity", "queue/x.yaml")
	gitT(t, b, "pull", "-q", "origin", "workflow")
	edit(t, b, "x", c.win)
	pushed(t, b, "human/jeff", "x: win", "queue/x.yaml")
	claimStep(t, a, c.worker, "x", "f0-branch-setup")
	out, code := commit(t, a, c.worker, "x: claim", "-claimed", "x/f0-branch-setup")
	switch {
	case c.want == "" && (code != 0 || !strings.Contains(out, "claimed again")):
		t.Fatalf("exit %d, want the claim asked again:\n%s", code, out)
	case c.want != "" && (code != 4 || !strings.Contains(out,
		"the claim of x/f0-branch-setup is refused on the new state: "+c.want)):
		t.Fatalf("exit %d, want 4 with %q:\n%s", code, c.want, out)
	}
}

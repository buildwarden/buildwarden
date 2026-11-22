package queue

import (
	"slices"
	"strings"
	"testing"
)

// openPR is an in-progress feature at its merge step with PR number pr, its
// CI green for the head recorded.
func openPR(name string, prio, pr int, areas ...string) *Feature {
	f := mergeWaiting(working(name, prio, 6, areas...), pr)
	f.PR.CI, f.PR.Head = "green", "0123456789abcdef0123456789abcdef01234567"
	return f
}

// refresh seeds the item's refresh step and fails the test when none is
// seeded.
func refresh(t *testing.T, f *Feature) *Step {
	t.Helper()
	st := SeedRefresh(f, "main at 0a1b2c3: 1 commit", "cloud/run", now)
	if st == nil {
		t.Fatalf("%s: no refresh step seeded", f.Feature)
	}
	return st
}

func stepIDs(f *Feature, status string) string {
	var ids []string
	for _, st := range f.Steps {
		if st.Status == status {
			ids = append(ids, st.ID)
		}
	}
	return strings.Join(ids, " ")
}

func TestIsRefresh(t *testing.T) {
	for id, want := range map[string]bool{"f6-refresh": true, "f6-refresh-r2": true,
		"x2-refresh": true, "x2-refresh-r3": true, "f7-refresh": false,
		"f6-merge-main": false, "x3-refresh": false, "t1-refresh": false,
		"f6-refresh-main": false} {
		if got := IsRefresh(&Step{ID: id}); got != want {
			t.Errorf("IsRefresh(%s) = %v, want %v", id, got, want)
		}
	}
}

// TestRefreshStep: the refresh is one step, seeded once while it is pending,
// that puts its item in class 1, leaves the phase alone, goes away when the
// PR merges or the item is sent back, waits while main is red or a process
// item holds the worker, and costs the item no budget on its first claim
// (FLOW-05, FLOW-06, FLOW-10; cases 35 and 41 of the loop-parallel design).
func TestRefreshStep(t *testing.T) {
	t.Run("seeded once", refreshSeededOnce)
	t.Run("in Publish", refreshInPublish)
	t.Run("class 1", refreshClassOne)
	t.Run("under a hold", refreshUnderHold)
	t.Run("dropped", refreshDropped)
	t.Run("main red", refreshMainRed)
	t.Run("budget", refreshBudget)
}

func refreshSeededOnce(t *testing.T) {
	f := openPR("a", 1, 7)
	st := refresh(t, f)
	if st.ID != "f6-refresh" || st.Phase != 6 || st.Affinity != "cloud" ||
		st.Status != "pending" || st.Note == "" {
		t.Fatalf("seeded %+v, want a pending cloud step f6-refresh of phase 6", st)
	}
	if again := SeedRefresh(f, "main moved again", "cloud/run", now); again != nil {
		t.Fatalf("a second refresh %s was seeded beside a pending one", again.ID)
	}
	wantValidate(t, state(f))
	wantNext(t, state(f), "cloud/run", "a f6-refresh")

	// Its completion leaves the phase alone, and the next one gets a fresh
	// id.
	Complete(f, st, "cloud/run", now)
	if f.Phase != 7 || f.Status != "in_progress" || f.PendingRefresh() != nil {
		t.Fatalf("after the refresh: phase %d, %s", f.Phase, f.Status)
	}
	if st := refresh(t, f); st.ID != "f6-refresh-r2" {
		t.Fatalf("the second refresh is %s, want f6-refresh-r2", st.ID)
	}

	x := mergeWaiting(process("x", 2, "in_progress"), 9)
	if st := refresh(t, x); st.ID != "x2-refresh" || st.Phase != 2 {
		t.Fatalf("a process item's refresh is %+v, want x2-refresh of phase 2", st)
	}
	wantNext(t, state(x), "cloud/run", "x x2-refresh")
}

// refreshInPublish is case 38: a feature that merged main in Publish and has
// not finished it is refreshed there, and f6-open-pr waits for the refresh.
func refreshInPublish(t *testing.T) {
	f := publishing("a", 1)
	Complete(f, f.mergeMain(), "cloud/run", now)
	st := refresh(t, f)
	open, err := f.Step("f6-open-pr")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(open.Needs, st.ID) {
		t.Fatalf("f6-open-pr needs %v, want %s among them", open.Needs, st.ID)
	}
	if !slices.Equal(st.Needs, []string{"f6-consolidate"}) {
		t.Fatalf("the refresh needs %v, want the pending f6-consolidate", st.Needs)
	}
	s := state(f)
	wantValidate(t, s)
	wantNext(t, s, "cloud/run", "a f6-consolidate")
	cons, _ := f.Step("f6-consolidate")
	Complete(f, cons, "cloud/run", now)
	wantNext(t, s, "cloud/run", "a f6-refresh")
	st, _ = f.Step("f6-refresh")
	Complete(f, st, "cloud/run", now)
	if f.Phase != 6 {
		t.Fatalf("phase %d after the refresh, want Publish still", f.Phase)
	}
	wantNext(t, s, "cloud/run", "a f6-open-pr")

	// With the consolidation done, the refresh needs nothing.
	g := publishing("b", 2)
	Complete(g, g.mergeMain(), "cloud/run", now)
	cons, _ = g.Step("f6-consolidate")
	Complete(g, cons, "cloud/run", now)
	if st := refresh(t, g); len(st.Needs) != 0 {
		t.Fatalf("the refresh needs %v after the consolidation is done", st.Needs)
	}
}

func refreshClassOne(t *testing.T) {
	a, x := openPR("a", 1, 7), mergeWaiting(process("x", 2, "in_progress"), 9)
	if a.Class() != 4 || x.Class() != 2 {
		t.Fatalf("classes at the merge step: %d, %d; want 4 and 2", a.Class(), x.Class())
	}
	refresh(t, a)
	refresh(t, x)
	if a.Class() != 1 || x.Class() != 1 {
		t.Fatalf("classes with a refresh pending: %d, %d; want 1 and 1", a.Class(),
			x.Class())
	}
	// A run refreshes open PRs before it goes on with a process item or
	// starts anything.
	design := process("design", 3, "in_progress")
	design.Steps[0].Affinity = "cloud"
	s := state(design, feature("q", 4, "queued"), a)
	wantNext(t, s, "cloud/run", "a f6-refresh")
}

// refreshUnderHold is case 41: while a process item in progress holds the
// worker, no other item's refresh is taken, by the tick or an attended run.
func refreshUnderHold(t *testing.T) {
	a := openPR("a", 1, 7)
	refresh(t, a)
	s := state(a, implementing("x", 2, "cloud"))
	const why = "held by process item x"
	pick, blocked := s.Next("cloud/run", now)
	if pick != nil || !slices.Contains(blocked, Blocked{"a", why}) {
		t.Fatalf("next = %+v, %+v; want a held", pick, blocked)
	}
	wantOffer(t, s, "a", why)
}

func refreshDropped(t *testing.T) {
	// The PR merged first: the merge step's completion drops the refresh.
	a := openPR("a", 1, 7)
	refresh(t, a)
	merge, _ := a.Step("f7-merge")
	Complete(a, merge, "cloud/run", now)
	if a.Phase != 8 || a.PendingRefresh() != nil || a.hasStep("f6-refresh") {
		t.Fatalf("after the merge: phase %d, pending %q", a.Phase, stepIDs(a, "pending"))
	}
	wantValidate(t, state(a))

	// A return drops it: Publish merges main again (FLOW-06).
	for _, to := range []int{1, 3} {
		b := openPR("b", 1, 7)
		refresh(t, b)
		Back(b, "cloud/run", now, "a merged check fails on the branch", to)
		if b.PendingRefresh() != nil || b.hasStep("f6-refresh") {
			t.Fatalf("back -to %d kept the refresh: %q", to, stepIDs(b, "pending"))
		}
		wantValidate(t, state(b))
	}
	x := mergeWaiting(process("x", 1, "in_progress"), 9)
	refresh(t, x)
	Back(x, "cloud/run", now, "the design must change", 1)
	if x.PendingRefresh() != nil {
		t.Fatalf("a process item's return kept the refresh: %q", stepIDs(x, "pending"))
	}

	// In Publish, the need on a dropped refresh goes with it.
	c := publishing("c", 1)
	Complete(c, c.mergeMain(), "cloud/run", now)
	refresh(t, c)
	Back(c, "cloud/run", now, "a duplicate translator", 3)
	wantValidate(t, state(c))
}

// refreshMainRed is case 35: while main.yaml records main red no refresh is
// offered but the fix's own, and the run's steps go on to the fix.
func refreshMainRed(t *testing.T) {
	a, b := openPR("a", 1, 7), openPR("b", 2, 8)
	refresh(t, a)
	refresh(t, b)
	fix := feature("fix", 3, "queued")
	s := state(a, b, fix, feature("done", 9, "done"))
	fix.Follows = "done"
	s.Main = &Main{SHA: "0a1b2c3", CI: "red", Followup: "fix"}
	const why = "main is red at 0a1b2c3: no refresh until it is green (FLOW-10)"
	pick, blocked := s.Next("cloud/run", now)
	if pick == nil || pick.Feature != fix || !slices.Equal(blocked,
		[]Blocked{{"a", why}, {"b", why}}) {
		t.Fatalf("next = %+v, %+v; want the fix past the two refreshes", pick, blocked)
	}
	wantOffer(t, s, "a", why)

	// The fix's own refresh is the one still taken.
	s.Main.Followup = "b"
	wantNext(t, s, "cloud/run", "b f6-refresh")
	wantOffer(t, s, "a", why)
	wantOffer(t, s, "b", "f6-refresh")

	// A green result, or another head whose result is not red, lifts it.
	for _, m := range []*Main{{SHA: "0a1b2c3", CI: "green"}, {SHA: "4d5e6f7", CI: "pending"},
		nil} {
		s.Main = m
		wantNext(t, s, "cloud/run", "a f6-refresh")
	}

	// Only a refresh waits for main: the item's other steps go on.
	c := publishing("c", 1)
	s = state(c)
	s.Main = &Main{SHA: "0a1b2c3", CI: "red"}
	wantNext(t, s, "cloud/c", "c f6-merge-main")
}

// refreshBudget is a10 (d): a refresh's first claim costs the item no step,
// and a released refresh claimed again counts like any retried step.
func refreshBudget(t *testing.T) {
	a := openPR("a", 1, 7)
	s := state(a)
	st := refresh(t, a)
	spent := a.Budget.Spent
	s.Claim(a, st, "cloud/run-1", now)
	if got := a.Budget.Spent; got.Steps != spent.Steps || got.Runs != spent.Runs+1 {
		t.Fatalf("the first claim: spent %+v from %+v, want the steps left alone", got,
			spent)
	}
	s.Release(a, st, "cloud/run-1", now, "main moved again")
	s.Claim(a, st, "cloud/run-2", now)
	if got := a.Budget.Spent.Steps; got != spent.Steps+1 {
		t.Fatalf("the second claim: %d steps spent, want %d", got, spent.Steps+1)
	}
	// The next refresh is another step, whose first claim is free again.
	Complete(a, st, "cloud/run-2", now)
	st = refresh(t, a)
	s.Claim(a, st, "cloud/run-3", now)
	if got := a.Budget.Spent.Steps; got != spent.Steps+1 {
		t.Fatalf("a second refresh's first claim: %d steps spent, want %d", got,
			spent.Steps+1)
	}
	// A refresh claimed and then dropped by a return leaves its id to no
	// later refresh, whose first claim is free too (a10 (d)).
	c := openPR("c", 3, 7)
	sc := state(c)
	st = refresh(t, c)
	sc.Claim(c, st, "cloud/run-4", now)
	cSpent := c.Budget.Spent.Steps
	Back(c, "cloud/run-4", now, "a merged check fails on the branch", 3)
	st = refresh(t, c)
	if st.ID == "f6-refresh" {
		t.Fatal("a later refresh reused the dropped refresh's id")
	}
	sc.Claim(c, st, "cloud/run-5", now)
	if got := c.Budget.Spent.Steps; got != cSpent {
		t.Fatalf("the later refresh's first claim: %d steps spent, want %d", got, cSpent)
	}
	// Any other step counts at once.
	b := feature("b", 2, "in_progress")
	s.Claim(b, &b.Steps[0], "cloud/run-3", now)
	if b.Budget.Spent.Steps != 1 {
		t.Fatalf("a first claim of %s spent %d steps, want 1", b.Steps[0].ID,
			b.Budget.Spent.Steps)
	}
}

// TestLanded: an item sent back with its PR open, whose PR the human then
// merged, goes to Close with the finding on a human step (FLOW-08).
func TestLanded(t *testing.T) {
	a := openPR("a", 1, 7)
	refresh(t, a)
	Back(a, "cloud/run", now, "a merged check fails on the branch", 3)
	Landed(a, "human/jeff", now)
	if a.Phase != 8 || a.Status != "in_progress" {
		t.Fatalf("phase %d, %s; want Close", a.Phase, a.Status)
	}
	if got := stepIDs(a, "pending"); got != "f8-close f8-escalate-returned" {
		t.Fatalf("pending steps %q", got)
	}
	esc, _ := a.Step("f8-escalate-returned")
	if esc.Affinity != "human" || esc.Phase != 8 ||
		!strings.Contains(esc.Note, "a merged check fails on the branch") {
		t.Fatalf("the escalation is %+v, want a human step with the finding", esc)
	}
	s := state(a)
	wantValidate(t, s)
	if got := s.WaitsOnHuman(); len(got) != 1 || got[0] != a {
		t.Fatalf("WaitsOnHuman = %v, want the landed item", got)
	}
	wantNext(t, s, "cloud/run", "a f8-close")

	x := mergeWaiting(process("x", 1, "in_progress"), 9)
	Back(x, "cloud/run", now, "the design must change", 1)
	Landed(x, "human/jeff", now)
	if got := stepIDs(x, "pending"); x.Phase != 4 || got != "x4-close x4-escalate-returned" {
		t.Fatalf("a process item: phase %d, pending %q", x.Phase, got)
	}
}

// TestReadyToMerge: a PR is ready for the human's merge only while its item
// is at its merge step with pr.ci green and no refresh pending (FLOW-08;
// cases 18, 33 and 36).
func TestReadyToMerge(t *testing.T) {
	a := openPR("a", 1, 7)
	if !a.ReadyToMerge() {
		t.Fatal("an item at its merge step with a green PR is not ready")
	}
	for name, change := range map[string]func(f *Feature){
		"CI pending":  func(f *Feature) { f.PR.CI = "pending" },
		"CI red":      func(f *Feature) { f.PR.CI = "red" },
		"no PR":       func(f *Feature) { f.PR = nil },
		"a refresh":   func(f *Feature) { SeedRefresh(f, "main moved", "cloud/run", now) },
		"in Publish":  func(f *Feature) { f.Phase = 6 },
		"sent back":   func(f *Feature) { Back(f, "cloud/run", now, "more work", 3) },
		"in Close":    func(f *Feature) { advance(f, 8) },
		"not started": func(f *Feature) { f.Status = "queued" },
	} {
		f := openPR("f", 1, 7)
		change(f)
		if f.ReadyToMerge() {
			t.Errorf("%s: the PR is listed as ready", name)
		}
	}
	// A passed refresh, with CI read for the new head, makes it ready again.
	st := refresh(t, a)
	a.PR.CI = "pending"
	Complete(a, st, "cloud/run", now)
	if a.ReadyToMerge() {
		t.Fatal("ready with CI pending after the refresh")
	}
	a.PR.CI = "green"
	if !a.ReadyToMerge() {
		t.Fatal("not ready after a passed refresh with CI green")
	}
	x := mergeWaiting(process("x", 2, "in_progress"), 9)
	x.PR.CI = "green"
	if !x.ReadyToMerge() {
		t.Fatal("a process item at its merge step with a green PR is not ready")
	}
}

// TestMergedFirstSeeded: Publish seeds f6-merged-<n> for each work entry
// marked first, n its position in work; no worker is offered it, a step
// that needs it waits, and Publish ends only once it is done (Validation
// plan test 5, a5).
func TestMergedFirstSeeded(t *testing.T) {
	f := feature("a", 1, "in_progress")
	f.Repos = []string{"o/here", "o/specs", "o/relay"}
	f.Work = []Work{{Repo: "o/specs", Branch: "feat/a"},
		{Repo: "o/relay", Branch: "feat/a", First: true, PR: &PR{Number: 9}}}
	advance(f, 6)
	var ids []string
	for _, st := range f.Steps {
		if st.Phase == 6 {
			ids = append(ids, st.ID)
		}
	}
	if !slices.Equal(ids, []string{"f6-merge-main", "f6-consolidate", "f6-open-pr",
		"f6-merged-2"}) {
		t.Fatalf("Publish steps %v", ids)
	}
	st := f.Steps[len(f.Steps)-1]
	if n, ok := MergedFirst(&st); !ok || n != 2 {
		t.Fatalf("MergedFirst(%s) = %d, %v", st.ID, n, ok)
	}
	f.Steps = append(f.Steps, Step{ID: "f6-release-specs", Phase: 6, Affinity: "human",
		Status: "pending", Needs: []string{"f6-merged-2"}})
	s := state(f)
	for i := range f.Steps[:len(f.Steps)-2] {
		if f.Steps[i].Phase == 6 {
			Complete(f, &f.Steps[i], "test", now)
		}
	}
	for _, worker := range []string{"cloud", "human/jeff"} {
		_, blocked := s.Next(worker, now)
		if !slices.Contains(blocked, Blocked{"a",
			"waiting on the human's merge of o/relay PR #9 (FLOW-08)"}) {
			t.Errorf("%s: blocked %+v", worker, blocked)
		}
	}
	if f.Phase != 6 {
		t.Fatalf("Publish ended with f6-merged-2 pending: phase %d", f.Phase)
	}
	Complete(f, &f.Steps[len(f.Steps)-2], "test", now)
	if got := next(t, s, "human/jeff"); got != "a f6-release-specs" {
		t.Errorf("after the merge: next %q", got)
	}
}

// TestMergedFirstIDs: MergedFirst reads n from f6-merged-<n> and its -r<n>
// forms, and nothing else is a confirmation.
func TestMergedFirstIDs(t *testing.T) {
	for _, id := range []string{"f6-merged-1", "f6-merged-12-r2"} {
		if n, ok := MergedFirst(&Step{ID: id}); !ok || n != map[string]int{
			"f6-merged-1": 1, "f6-merged-12-r2": 12}[id] {
			t.Errorf("MergedFirst(%s) = %d, %v", id, n, ok)
		}
	}
	for _, id := range []string{"f7-merged-1", "f6-merged-0", "x2-merged-1", "f6-merged"} {
		if _, ok := MergedFirst(&Step{ID: id}); ok {
			t.Errorf("MergedFirst(%s) is a confirmation", id)
		}
	}
}

package queue

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// working is an in-progress feature at phase that declares the areas, its
// design review skipped so that every step up to its merge is the cloud's.
func working(name string, prio, phase int, areas ...string) *Feature {
	f := feature(name, prio, "in_progress")
	f.DesignReview = "skip"
	advance(f, phase)
	if areas != nil {
		f.Areas = &areas
	}
	return f
}

// parked makes the in-progress features' first pending step the human's, so
// that the cloud's walk passes them and reaches the queued items.
func parked(fs ...*Feature) []*Feature {
	for _, f := range fs {
		for i := range f.Steps {
			if f.Steps[i].Status == "pending" {
				f.Steps[i].Affinity = "human"
				break
			}
		}
	}
	return fs
}

// guard is Guard's answer for a cloud run's claim of the item's first pending
// step that takes a limit: its first step for a queued item, its Plan step,
// or its f6-merge-main.
func guard(t *testing.T, s *State, f *Feature) string {
	t.Helper()
	return s.Guard(f, taking(t, f), "cloud/"+f.Feature)
}

// taking is the item's pending step whose claim takes a limit.
func taking(t *testing.T, f *Feature) *Step {
	t.Helper()
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status != "pending" {
			continue
		}
		if f.Status == "queued" || IsPlan(st) || IsMergeMain(st) {
			return st
		}
	}
	t.Fatalf("%s has no pending step that takes a limit", f.Feature)
	return nil
}

// take claims the item's limit-taking step for a cloud run of its own, after
// asserting that Guard and wf next -feature both allow it.
func take(t *testing.T, s *State, f *Feature) *Step {
	t.Helper()
	st := taking(t, f)
	if why := guard(t, s, f); why != "" {
		t.Fatalf("%s: Guard refuses %s: %s", f.Feature, st.ID, why)
	}
	wantOfferTo(t, s, f.Feature, "cloud/"+f.Feature, st.ID)
	s.Claim(f, st, "cloud/"+f.Feature, now, true)
	return st
}

// wantRefused asserts that Guard refuses the item's limit-taking step with
// the reason, that wf next -feature gives the same reason, and that the
// state is still valid.
func wantRefused(t *testing.T, s *State, f *Feature, why string) {
	t.Helper()
	if got := guard(t, s, f); got != why {
		t.Fatalf("%s: Guard = %q, want %q", f.Feature, got, why)
	}
	want := why
	if f.Status != "queued" {
		want = "waiting on " + why
	}
	wantOfferTo(t, s, f.Feature, "cloud/"+f.Feature, want)
	wantValidate(t, s)
}

// TestGuardInProgressPerSet: the in-progress limit counts an item's set, the
// in-progress features that collide with it, and not the queue (FLOW-05;
// cases 9, 10 and 16 of the loop-parallel design).
func TestGuardInProgressPerSet(t *testing.T) {
	five := func(areas ...string) []*Feature {
		var fs []*Feature
		for n := 1; n <= 5; n++ {
			fs = append(fs, working(fmt.Sprintf("f%d", n), n, 3, areas...))
		}
		return parked(fs...)
	}
	const full = "the in-progress limit (limit 5): it collides with f1, f2, f3, f4, f5 " +
		"(FLOW-05)"

	// Case 9: five undeclared features collide with a sixth, declared or not.
	q := feature("q", 6, "queued")
	s := mapped(t, append(five(), q)...)
	wantRefused(t, s, q, full)
	wantNext(t, s, "cloud", "none")
	withAreas(q, "ledger")
	wantRefused(t, s, q, full)

	// Case 10: five declared features, each disjoint from a sixth, leave it
	// room; a seventh that collides with the five has none.
	q, r := item("q", "ledger"), item("r", "relaycfg", "book")
	q.Priority, r.Priority = 6, 7
	s = mapped(t, append(five("relaycfg"), q, r)...)
	wantRefused(t, s, r, full)
	wantNext(t, s, "cloud", "q f0-branch-setup")
	take(t, s, q)
	wantValidate(t, s)

	// Case 16: the limit is 5 when config.yaml writes none.
	s.Config.Limits.InProgress = nil
	wantRefused(t, s, r, full)

	// A holder's set counts too: h collides with four features and with x,
	// and x with h alone.
	fs := five("relaycfg")
	h, x := fs[0], item("x", "ledger")
	withAreas(h, "relaycfg", "ledger")
	x.Priority = 6
	s = mapped(t, append(fs, x)...)
	wantRefused(t, s, x, "the in-progress limit (limit 5): f1, which collides with it, "+
		"is already with f2, f3, f4, f5 (FLOW-05)")
}

// alter is an in-progress feature that declares alters_relationships: true
// and the areas, at its Plan step with no review to wait for.
func alter(name string, prio int, areas ...string) *Feature {
	f := working(name, prio, 2, areas...)
	declare(f, true)
	return f
}

// TestGuardAltersSlot: an alters_relationships slot is counted per set
// (FLOW-05; cases 2, 6 and 19).
func TestGuardAltersSlot(t *testing.T) {
	// Case 2: two undeclared features plan under a limit of 3.
	a, b := alter("a", 1), alter("b", 2)
	s := limited(mapped(t, a, b), 3)
	take(t, s, a)
	take(t, s, b)
	wantValidate(t, s)

	// Case 6: three items in one exclusive area hold the three slots; an
	// item near each of them is in none of their sets and plans too; a
	// fifth in that area waits.
	s3, s4, k1 := alter("s3", 1, "driver-core"), alter("s4", 2, "driver-core"),
		alter("k1", 3, "driver-core")
	s2 := alter("s2", 4, "relay", "executor", "rules", "book")
	x := alter("x", 5, "driver-core")
	for _, f := range []*Feature{s3, s4, k1, s2, x} {
		withRules(f, "GLOBAL-08")
	}
	s = limited(mapped(t, s3, s4, k1, s2, x), 3)
	for _, f := range []*Feature{s3, s4, k1, s2} {
		take(t, s, f)
	}
	wantValidate(t, s)
	wantRefused(t, s, x,
		"an alters_relationships slot (limit 3), held by s3, s4, k1 (FLOW-05)")

	star(t)

	s = limited(mapped(t, alter("a", 1)), 0)
	if got := guard(t, s, s.Features[0]); got !=
		"limits.alters_relationships 0 is below 1 (FLOW-05)" {
		t.Fatalf("under a limit below 1, Guard = %q", got)
	}
}

// star is case 19: the relations are not transitive. H collides with P1, P2
// and X, which are near one another; with H, P1 and P2 holding the three
// slots, X's own set is 2 and H's would be 4.
func star(t *testing.T) (*State, *Feature) {
	t.Helper()
	h := alter("h", 1, "driver-core", "relay", "collector")
	p1, p2, x := alter("p1", 2, "relay"), alter("p2", 3, "collector"),
		alter("x", 4, "driver-core")
	s := limited(mapped(t, h, p1, p2, x), 3)
	for _, f := range []*Feature{h, p1, p2} {
		take(t, s, f)
	}
	wantRefused(t, s, x, "an alters_relationships slot (limit 3): h, which collides with "+
		"it, is already with p1, p2 (FLOW-05)")
	return s, x
}

// publishing is a feature in Publish that declares the areas, its
// f6-merge-main pending.
func publishing(name string, prio int, areas ...string) *Feature {
	return working(name, prio, 6, areas...)
}

// TestGuardPRSlot: the PR slot is held from the claim of f6-merge-main until
// the PR merges, by an item and the items that collide with it or are near
// it (FLOW-05; cases 3, 4, 7, 8 and 20).
func TestGuardPRSlot(t *testing.T) {
	t.Run("held from the claim", prSlotFromClaim)
	t.Run("by relation", prSlotByRelation)
	t.Run("across a return", prSlotAcrossReturn)
}

func prSlotFromClaim(t *testing.T) {
	const held = "the PR slot, held by a (colliding: a declares no areas) (FLOW-05)"
	a, b := publishing("a", 2), publishing("b", 1)
	s := mapped(t, a, b)
	// Case 3: a claimed f6-merge-main holds the slot, as a done one does;
	// released, it is the higher priority's again.
	s.Claim(a, taking(t, a), "cloud/a", now, false)
	st := taking(t, a)
	wantRefused(t, s, b, held)
	s.Release(a, st, "cloud/a", now, "the merge conflicts")
	wantOffer(t, s, "b", "f6-merge-main")
	s.Claim(a, st, "cloud/a", now, false)
	Complete(a, st, "cloud/a", now)
	wantRefused(t, s, b, held)
	// A holder is never refused its own step.
	if why := s.Guard(a, st, "cloud/a"); why != "" {
		t.Fatalf("the holder is refused: %s", why)
	}
	// It holds through its merge step, with its PR recorded.
	advance(a, 7)
	a.PR = &PR{Number: 7}
	wantRefused(t, s, b, held)
	// Case 4: an item in Close holds nothing.
	advance(a, 8)
	wantOffer(t, s, "b", "f6-merge-main")
	take(t, s, b)
	wantValidate(t, s)
}

func prSlotByRelation(t *testing.T) {
	// Case 7: a driver pilot and an item in the relay are disjoint; both
	// hold the slot.
	k3, r := publishing("k3", 1, "driver/hyperv", "build", "book"),
		publishing("r", 2, "relay", "book")
	// A near item takes turns with the one it is near.
	core := publishing("core", 3, "driver-core")
	s := mapped(t, k3, r, core)
	take(t, s, k3)
	take(t, s, r)
	wantValidate(t, s)
	wantRefused(t, s, core, "the PR slot, held by k3 (near: driver-core reaches "+
		"driver/hyperv), r (near: driver-core reaches relay) (FLOW-05)")

	// Case 8: five adoptions, one per driver, are pairwise disjoint.
	var fs []*Feature
	for i, d := range []string{"container", "qemu", "vz", "hyperv", "firecracker"} {
		fs = append(fs, adopting(publishing(d, i+1, "driver/"+d, "book")))
	}
	s = mapped(t, fs...)
	for _, f := range fs {
		take(t, s, f)
	}
	wantValidate(t, s)
}

func prSlotAcrossReturn(t *testing.T) {
	const held = "the PR slot, held by a (near: driver/vz reaches driver-core) (FLOW-05)"
	a, b := publishing("a", 2, "driver-core"), publishing("b", 1, "driver/vz")
	s := mapped(t, a)
	Complete(a, take(t, s, a), "cloud/a", now)
	s.Features = append(s.Features, b)
	wantRefused(t, s, b, held)

	// A return before the PR was opened gives the slot up, and Publish
	// entered again holds it only from the new claim.
	Back(a, "cloud/a", now, "consolidation found work", 3)
	wantOffer(t, s, "b", "f6-merge-main")
	advance(a, 6)
	wantOffer(t, s, "b", "f6-merge-main")
	st := taking(t, a)
	if st.ID != "f6-merge-main-r2" {
		t.Fatalf("a entered Publish again with %s pending", st.ID)
	}
	s.Claim(a, st, "cloud/a", now, false)
	Complete(a, st, "cloud/a", now)
	wantRefused(t, s, b, held)

	// Case 20: a return with the PR recorded keeps the slot, in Implement
	// and in Design, and the returned item's own next f6-merge-main is not
	// refused.
	advance(a, 7)
	a.PR = &PR{Number: 12}
	for _, to := range []int{3, 1} {
		Back(a, "cloud/a", now, "the refresh found work", to)
		if a.Phase != to {
			t.Fatalf("a is in phase %d after a return to %d", a.Phase, to)
		}
		wantRefused(t, s, b, held)
	}
	a.DesignReview = "skip"
	advance(a, 6)
	if why := guard(t, s, a); why != "" {
		t.Fatalf("the returned item's own f6-merge-main is refused: %s", why)
	}
	wantRefused(t, s, b, held)
}

// TestGuardPRSlotTieBreak: when related items wait for the PR slot the higher
// priority goes first, whether or not it could take the slot itself (FLOW-05;
// cases 13 and 21).
func TestGuardPRSlotTieBreak(t *testing.T) {
	// Case 13: two near items wait; the lower priority is refused, whoever
	// asks.
	hi, lo := publishing("hi", 1, "driver-core"), publishing("lo", 3, "driver/vz")
	s := mapped(t, hi, lo)
	const ahead = "the PR slot: hi (priority 1) is ahead (FLOW-05)"
	wantRefused(t, s, lo, ahead)
	wantOfferTo(t, s, "lo", "cloud/attended", "waiting on "+ahead)
	wantNext(t, s, "cloud", "hi f6-merge-main")

	// Case 21: hi is refused by a holder it is near; lo, disjoint from that
	// holder, still waits behind hi.
	hv := publishing("hv", 2, "driver/hyperv")
	s = mapped(t, hv)
	st := take(t, s, hv)
	s.Features = append(s.Features, hi, lo)
	wantRefused(t, s, hi,
		"the PR slot, held by hv (near: driver-core reaches driver/hyperv) (FLOW-05)")
	wantRefused(t, s, lo, ahead)
	// The human merges the holder's PR: the higher priority goes next.
	Complete(hv, st, "cloud/hv", now)
	hv.Status = "done"
	wantRefused(t, s, lo, ahead)
	take(t, s, hi)
	wantRefused(t, s, lo,
		"the PR slot, held by hi (near: driver/vz reaches driver-core) (FLOW-05)")

	// An item that cannot take the slot for a reason of its own (a blocking
	// question, a spent budget) does not wait for it, and holds nobody back.
	hi, lo = publishing("hi", 1, "driver-core"), publishing("lo", 3, "driver/vz")
	s = mapped(t, hi, lo)
	hi.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open"}}
	wantOffer(t, s, "lo", "f6-merge-main")
	hi.Questions = nil
	hi.Budget.Spent.Steps = 2 * hi.Budget.Estimate.Steps
	wantOffer(t, s, "lo", "f6-merge-main")

	tieBreakArrivals(t)
}

// simAreas are the area lists generated items declare: nil is undeclared.
var simAreas = [][]string{nil, {}, {"relay"}, {"driver/hyperv", "book"}, {"driver/vz"},
	{"driver-core", "cli"}, {"collector"}, {"relaycfg", "book"}, {"ledger"},
	{"driver-core", "relay", "collector"}, {"book", "rules"}}

// tieBreakArrivals: items keep arriving in Publish while the human merges
// every open PR each round. No waiting item is passed over by a related item
// of lower priority, and once arrivals stop every item gets the slot.
func tieBreakArrivals(t *testing.T) {
	rng := rand.New(rand.NewPCG(21, 12))
	s := mapped(t)
	var waiting []*Feature
	round := func() {
		for _, f := range s.Features {
			if f.Status == "in_progress" && f.holdsPRSlot() {
				f.Status = "done"
			}
		}
		rng.Shuffle(len(waiting), func(i, j int) {
			waiting[i], waiting[j] = waiting[j], waiting[i]
		})
		var left []*Feature
		for _, f := range waiting {
			if s.Guard(f, taking(t, f), "cloud/sim") == "" {
				s.Claim(f, taking(t, f), "cloud/sim", now, false)
				continue
			}
			left = append(left, f)
		}
		for _, w := range left {
			notPassedOver(t, s, w)
		}
		waiting = left
		if errs := s.validateLimits(); len(errs) > 0 {
			t.Fatalf("after a round: %q", errs)
		}
	}
	prios := rng.Perm(60)
	for n := range 60 {
		areas := simAreas[rng.IntN(len(simAreas))]
		f := publishing(fmt.Sprintf("i%02d", n), prios[n]+1, areas...)
		s.Features = append(s.Features, f)
		waiting = append(waiting, f)
		round()
	}
	for rounds := 0; len(waiting) > 0; rounds++ {
		if rounds > 60 {
			t.Fatalf("%d items wait, 60 rounds after the last arrival", len(waiting))
		}
		round()
	}
}

// notPassedOver asserts that no item related to the waiting one, and of lower
// priority, holds the PR slot.
func notPassedOver(t *testing.T, s *State, w *Feature) {
	t.Helper()
	for _, h := range s.Features {
		if h.Status != "in_progress" || !h.holdsPRSlot() || h.Priority < w.Priority {
			continue
		}
		if k := s.Relation(w, h).Kind; k != Disjoint {
			t.Fatalf("%s (priority %d) took the slot past %s (priority %d), %s",
				h.Feature, h.Priority, w.Feature, w.Priority, k)
		}
	}
}

// TestGuardEveryWorker: the limits bind every worker's claim. A Plan step
// whose affinity was set to a host is refused past the limit, where wf claim
// let it stand and wf validate then failed (FLOW-04, FLOW-05; case 5).
func TestGuardEveryWorker(t *testing.T) {
	a, b, c := approved("a", 1), altering("b", 2), approved("c", 3)
	s := nativeState(a, b, c)
	finishPlan(t, s, a)
	plan(t, c).Affinity = "host:macos-vz"
	// A human claim takes no limit: the review is offered while every slot
	// is held.
	wantNext(t, s, "human/jeff", "b f2-design-review")
	if why := s.Guard(b, &b.Steps[len(b.Steps)-2], "human/jeff"); why != "" {
		t.Fatalf("the design review is refused: %s", why)
	}
	const wait = "an alters_relationships slot (limit 1), held by a (FLOW-05)"
	if got := s.Guard(c, plan(t, c), "host:macos-vz/s"); got != wait {
		t.Fatalf("the host's Plan claim: Guard = %q, want %q", got, wait)
	}
	wantOfferTo(t, s, "c", "host:macos-vz/s", "waiting on "+wait)
	wantOffer(t, s, "c", "waiting on host:macos-vz step f2-plan")
	wantValidate(t, s)

	// A host that takes a queued feature's step is held to the in-progress
	// limit and the waiting limit as the cloud is.
	q := feature("q", 9, "queued")
	q.Steps[0].Affinity = "host:macos-vz"
	var fs []*Feature
	for n := 1; n <= 5; n++ {
		fs = append(fs, working(fmt.Sprintf("f%d", n), n, 3))
	}
	s = nativeState(append(parked(fs...), q)...)
	if got := s.Guard(q, &q.Steps[0], "host:macos-vz/s"); !strings.HasPrefix(got,
		"the in-progress limit (limit 5): it collides with f1, f2, f3, f4, f5") {
		t.Fatalf("the host's claim on a queued feature: Guard = %q", got)
	}
	wantNext(t, s, "host:macos-vz/s", "none")
	if got := s.Guard(q, &q.Steps[0], "human/jeff"); got != "" {
		t.Fatalf("a human claim on a queued feature: Guard = %q", got)
	}
}

// simulation is a generated queue that a cloud worker walks with claims Guard
// allows, as wf next -feature offers them.
type simulation struct {
	t   *testing.T
	s   *State
	rng *rand.Rand
	// refused counts Guard's refusals by the limit they name.
	refused map[string]int
	prs     int
}

func simulate(t *testing.T, s *State, seed uint64, items int) *simulation {
	t.Helper()
	sim := &simulation{t: t, s: s, rng: rand.New(rand.NewPCG(seed, 5)),
		refused: map[string]int{}}
	for n := range items {
		f := feature(fmt.Sprintf("i%02d", n), n+1, "queued")
		f.DesignReview = "skip"
		if a := simAreas[sim.rng.IntN(len(simAreas))]; a != nil {
			f.Areas = &a
		}
		declare(f, sim.rng.IntN(2) == 0)
		s.Features = append(s.Features, f)
	}
	return sim
}

// move takes one random action on a random open item: its next step as wf
// next -feature offers it, claimed and usually completed; the human's merge
// of an item at its merge step; or, now and then, a return.
func (sim *simulation) move() {
	s, rng := sim.s, sim.rng
	f := s.Features[rng.IntN(len(s.Features))]
	if f.Status == "done" {
		return
	}
	worker := "cloud/" + f.Feature
	// A return from Close is left out: the PR has merged by then.
	if f.Status == "in_progress" && f.Phase > 1 && f.Phase < 8 && rng.IntN(12) == 0 {
		Back(f, worker, now, "generated", []int{1, 3}[rng.IntN(2)])
		return
	}
	if f.Phase == 7 {
		sim.merge(f)
		return
	}
	pick, blocked := s.NextFor(f.Feature, worker, now)
	if pick == nil {
		for _, l := range limits {
			if strings.Contains(blocked[0].Reason, l.what) {
				sim.refused[l.what]++
			}
		}
		return
	}
	if why := s.Guard(f, pick.Step, worker); why != "" {
		sim.t.Fatalf("%s: offered %s, which Guard refuses: %s", f.Feature, pick.Step.ID,
			why)
	}
	s.Claim(f, pick.Step, worker, now, false)
	switch rng.IntN(6) {
	case 0: // The claim is left to expire.
	case 1:
		s.Release(f, pick.Step, worker, now, "generated")
	default:
		if strings.HasPrefix(pick.Step.ID, "f6-open-pr") {
			sim.prs++
			f.PR = &PR{Number: sim.prs}
		}
		Complete(f, pick.Step, worker, now)
	}
}

// merge completes the item's merge step, as the tick does once the human has
// merged its PR.
func (sim *simulation) merge(f *Feature) {
	for i := range f.Steps {
		if st := &f.Steps[i]; IsMerge(st) && st.Status == "pending" {
			Complete(f, st, "cloud/"+f.Feature, now)
		}
	}
}

// simMaps are the maps a generated queue is walked under: the design's, a
// small one that knows few of the areas, and none.
func simMaps(t *testing.T) map[string]*AreaMap {
	t.Helper()
	small, err := ParseAreas([]byte(strings.Replace(smallMap, "o/r",
		"buildwarden/buildwarden", 1)))
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*AreaMap{"design map": designMap(t), "small map": small, "no map": nil}
}

// TestGuardThenValidate: starting from a valid state, Validate reports no
// limit finding after any claim Guard allows, for each of the three limits
// (FLOW-05), and by name for the star of case 19.
func TestGuardThenValidate(t *testing.T) {
	for name, m := range simMaps(t) {
		refused := map[string]int{}
		for seed := range uint64(4) {
			s := limited(mapped(t), 2)
			s.Areas = m
			inProgress(s, 4)
			sim := simulate(t, s, seed, 24)
			for n := range 700 {
				sim.move()
				if errs := s.validateLimits(); len(errs) > 0 {
					t.Fatalf("%s, seed %d, move %d: %q", name, seed, n, errs)
				}
			}
			for what, n := range sim.refused {
				refused[what] += n
			}
		}
		for _, l := range limits {
			if refused[l.what] == 0 {
				t.Errorf("%s: no claim was refused by %s", name, l.what)
			}
		}
	}
	s, x := star(t)
	wantValidate(t, s)
	// Had the claim been made round Guard, H's set would be the finding.
	s.Claim(x, taking(t, x), "cloud/x", now, false)
	wantValidate(t, s, overLimit(3, "h", "p1", "p2", "x"))
}

// TestGuardNoMutualHold: over generated queues a holder is never refused the
// step that took its limit, and the waits form no cycle: every refusal names
// items that hold or are ahead, and following the names always ends (FLOW-05).
func TestGuardNoMutualHold(t *testing.T) {
	for name, m := range simMaps(t) {
		for seed := range uint64(4) {
			s := limited(mapped(t), 2)
			s.Areas = m
			inProgress(s, 4)
			sim := simulate(t, s, seed+40, 20)
			for n := range 400 {
				sim.move()
				if n%20 == 0 {
					at := fmt.Sprintf("%s, seed %d, move %d", name, seed, n)
					acyclic(t, at, waitsOf(t, s, at))
				}
			}
		}
	}
}

// waitsOf returns, for each open item Guard refuses a step, the items its
// reasons name, and fails when a refusal is for a limit the item holds.
func waitsOf(t *testing.T, s *State, at string) map[string][]string {
	t.Helper()
	waits := map[string][]string{}
	for _, f := range s.Features {
		for i := range f.Steps {
			st := &f.Steps[i]
			if f.Status == "done" || st.Status != "pending" || !Offered(f, st) {
				continue
			}
			why := s.Guard(f, st, "cloud/sim")
			if why == "" {
				continue
			}
			for _, l := range limits {
				if l.holds(s, f) && strings.Contains(why, l.what) {
					t.Fatalf("%s: %s holds %s and is refused %s: %s", at,
						f.Feature, l.what, st.ID, why)
				}
			}
			for _, o := range s.Features {
				if o != f && strings.Contains(why, o.Feature) {
					waits[f.Feature] = append(waits[f.Feature], o.Feature)
				}
			}
		}
	}
	return waits
}

// acyclic fails when following the waits from any item comes back to an item
// on the way.
func acyclic(t *testing.T, at string, waits map[string][]string) {
	t.Helper()
	var path []string
	var visit func(name string)
	visit = func(name string) {
		if slices.Contains(path, name) {
			t.Fatalf("%s: the waits form a cycle: %v, %s", at, path, name)
		}
		path = append(path, name)
		for _, o := range waits[name] {
			visit(o)
		}
		path = path[:len(path)-1]
	}
	for name := range waits {
		visit(name)
	}
}

// TestValidateLimitsPerSet: Validate evaluates Guard's predicate for every
// holder. Each limit passes at its number and fails over it, with the limit:
// prefix, the item and the others in its set (FLOW-05).
func TestValidateLimitsPerSet(t *testing.T) {
	var fs []*Feature
	for n := 1; n <= 5; n++ {
		fs = append(fs, working(fmt.Sprintf("f%d", n), n, 3))
	}
	s := mapped(t, fs...)
	wantValidate(t, s)
	// A sixth that is disjoint from the five is in no set of theirs.
	s.Features = append(s.Features, working("d", 6, 3, "ledger"))
	for _, f := range fs {
		withAreas(f, "relaycfg")
	}
	wantValidate(t, s)
	s.Features = append(s.Features, working("g", 7, 3, "relaycfg"))
	wantValidate(t, s,
		"limit: 6 features in progress, limit 5 (FLOW-05): f1, f2, f3, f4, f5, g")

	a, b, c := alter("a", 1), alter("b", 2), alter("c", 3)
	s = limited(mapped(t, a, b, c), 2)
	for _, f := range []*Feature{a, b} {
		take(t, s, f)
	}
	wantValidate(t, s)
	s.Claim(c, taking(t, c), "cloud/c", now, false)
	wantValidate(t, s, overLimit(2, "a", "b", "c"))

	// The PR slot: a disjoint pair holds it; a colliding and a near pair
	// are each over.
	p, q, r := publishing("p", 1, "driver-core"), publishing("q", 2, "specs"),
		publishing("r", 3, "driver/vz")
	u := publishing("u", 4, "driver-core", "book")
	s = mapped(t, p, q, r, u)
	take(t, s, p)
	take(t, s, q)
	wantValidate(t, s)
	s.Claim(r, taking(t, r), "cloud/r", now, false)
	wantValidate(t, s, "limit: p holds the PR slot with r (near: driver/vz reaches "+
		"driver-core), limit 1 (FLOW-05)")
	s.Release(r, taking(t, r), "cloud/r", now, "generated")
	s.Claim(u, taking(t, u), "cloud/u", now, false)
	wantValidate(t, s, "limit: p holds the PR slot with u (colliding: area driver-core), "+
		"limit 1 (FLOW-05)")

	// A limit below 1 is the finding; no count is compared with it.
	s = limited(mapped(t, alter("a", 1)), 0)
	s.Claim(s.Features[0], taking(t, s.Features[0]), "cloud/a", now, false)
	wantValidate(t, s, "config: limits.alters_relationships 0 is below 1")
}

// waiters returns n in-progress items that wait on the human, of the kinds
// case 25 names in turn: a feature whose PR waits for the merge, a process
// item at its design review, a feature at its design review, and a process
// item with an escalation. The features declare areas that relate to nothing,
// so only the waiting limit can refuse a start beside them.
func waiters(n int) []*Feature {
	var out []*Feature
	for i := range n {
		name, prio := fmt.Sprintf("w%d", i+1), i+1
		var f *Feature
		switch i % 4 {
		case 0:
			f = withAreas(feature(name, prio, "in_progress"), "book")
			mergeWaiting(f, 100+i)
		case 1:
			f = process(name, prio, "in_progress")
		case 2:
			f = withAreas(toPhase(name, prio, 2), "book")
		case 3:
			f = process(name, prio, "in_progress")
			advance(f, 2)
			f.Steps[len(f.Steps)-1].Needs = []string{"x2-escalate-ci"}
			f.Steps = append(f.Steps, Step{ID: "x2-escalate-ci", Phase: 2,
				Affinity: "human", Status: "pending"})
		}
		out = append(out, f)
	}
	return out
}

// waitingState is n items that wait on the human and the others.
func waitingState(t *testing.T, n int, others ...*Feature) *State {
	t.Helper()
	return mapped(t, append(waiters(n), others...)...)
}

// queuedItem is a queued feature that declares only an ordered area, so it
// is disjoint from every other item.
func queuedItem(name string, prio int) *Feature {
	f := item(name, "designs")
	f.Priority = prio
	return f
}

// wantWaiting asserts the count of items that wait on the human.
func wantWaiting(t *testing.T, s *State, n int) {
	t.Helper()
	if got := s.WaitsOnHuman(); len(got) != n {
		var names []string
		for _, f := range got {
			names = append(names, f.Feature)
		}
		t.Fatalf("%d items wait on the human, want %d: %v", len(got), n, names)
	}
}

// waitingLimitReason is the waiting limit's refusal with the count beside
// the limit; the names follow.
func waitingLimitReason(count, limit int) string {
	return fmt.Sprintf("the waiting limit: %d items wait on the human (limit %d): ", count,
		limit)
}

// wantGuardHolds asserts that the waiting limit refuses the queued feature's
// start, for a cloud and a host worker.
func wantGuardHolds(t *testing.T, s *State, f *Feature, count int) {
	t.Helper()
	why := waitingLimitReason(count, s.waitingLimit())
	for _, w := range []string{"cloud/run", "host:macos-vz/s"} {
		if got := s.Guard(f, &f.Steps[0], w); !strings.HasPrefix(got, why) ||
			!strings.HasSuffix(got, " (FLOW-05)") {
			t.Fatalf("%s: Guard for %s = %q, want %q...", f.Feature, w, got, why)
		}
	}
}

// wantHeld is wantGuardHolds, and that wf next passes the feature over with
// a blocked line that gives the reason.
func wantHeld(t *testing.T, s *State, f *Feature, count int) {
	t.Helper()
	wantGuardHolds(t, s, f, count)
	why := waitingLimitReason(count, s.waitingLimit())
	_, blocked := s.Next("cloud/run", now)
	if !slices.ContainsFunc(blocked, func(b Blocked) bool {
		return b.Feature == f.Feature && strings.HasPrefix(b.Reason, why)
	}) {
		t.Fatalf("wf next has no blocked line for %s on the waiting limit: %+v", f.Feature,
			blocked)
	}
}

// wantStarts asserts that neither limit refuses the queued item's start, for
// a cloud and a host worker.
func wantStarts(t *testing.T, s *State, f *Feature) {
	t.Helper()
	for _, w := range []string{"cloud/run", "host:macos-vz/s"} {
		if why := s.Guard(f, &f.Steps[0], w); why != "" {
			t.Fatalf("%s: Guard for %s refuses the start: %s", f.Feature, w, why)
		}
	}
}

// TestWaitingLimit: no queued feature starts while the items that wait on the
// human number limits.waiting_on_human, 8 when unset, or more (FLOW-05; cases
// 25 to 31, 39, 40, 42 and 43).
func TestWaitingLimit(t *testing.T) {
	t.Run("what is refused", waitingRefused)
	t.Run("what counts", waitingCounts)
	t.Run("the fix for a red main", waitingRedMain)
	t.Run("a marked feature", waitingMarked)
}

func waitingRefused(t *testing.T) {
	// Case 25: eight wait, four features and four process items; the next
	// queued feature, disjoint from all, is refused, and the walk goes on.
	q := queuedItem("q", 20)
	s := waitingState(t, 8, q)
	wantWaiting(t, s, 8)
	wantHeld(t, s, q, 8)
	want := waitingLimitReason(8, 8) + "w4, w8, w2, w6, w1, w3, w5, w7 (FLOW-05)"
	if got := s.Guard(q, &q.Steps[0], "cloud/run"); got != want {
		t.Fatalf("Guard = %q, want %q", got, want)
	}
	wantOffer(t, s, "q", want)
	wantValidate(t, s)

	// Case 26: a fix that follows an item starts all the same.
	parent := feature("parent", 30, "done")
	fix := fixing(queuedItem("fix", 21), "parent")
	s.Features = append(s.Features, parent, fix)
	wantStarts(t, s, fix)
	wantNext(t, s, "cloud/run", "fix f0-branch-setup")

	// Case 27: a queued process item's design is offered to the human.
	p := process("p", 22, "queued")
	s.Features = append(s.Features, p)
	wantOfferTo(t, s, "p", "human/jeff", "x1-design")
	wantStarts(t, s, p)

	// Case 28: work in progress is never refused, so the count passes the
	// limit. A human step takes no limit either.
	busy := withAreas(toPhase("busy", 23, 3), "book")
	s.Features = append(s.Features, busy)
	wantOffer(t, s, "busy", "f3-implement")
	if why := s.Guard(q, &q.Steps[0], "human/jeff"); why != "" {
		t.Fatalf("a human claim is refused: %s", why)
	}
	s = waitingState(t, 11, q)
	wantHeld(t, s, q, 11)
	wantValidate(t, s)

	// Case 31: a limit below 1 is an error, and nothing starts under it.
	zero := 0
	s = waitingState(t, 0, q)
	s.Config.Limits.WaitingOnHuman = &zero
	wantValidate(t, s, "config: limits.waiting_on_human 0 is below 1")
	if got := s.Guard(q, &q.Steps[0], "cloud/run"); got !=
		"limits.waiting_on_human 0 is below 1 (FLOW-05)" {
		t.Fatalf("under a limit of 0, Guard = %q", got)
	}
	// A written limit is used as written.
	three := 3
	s = waitingState(t, 3, q)
	s.Config.Limits.WaitingOnHuman = &three
	wantHeld(t, s, q, 3)
	s = waitingState(t, 2, q)
	s.Config.Limits.WaitingOnHuman = &three
	wantStarts(t, s, q)
}

func waitingCounts(t *testing.T) {
	// Case 29: an item counts once, whatever it waits on him for.
	q := queuedItem("q", 20)
	s := waitingState(t, 7, q)
	w := s.Features[2]
	w.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open"},
		{ID: "q2", Kind: "blocking", Status: "open"}}
	w.Steps = append(w.Steps, Step{ID: "f2-escalate-3", Phase: 2, Affinity: "human",
		Status: "pending"})
	w.Budget.Spent.Steps = 2 * w.Budget.Estimate.Steps
	wantWaiting(t, s, 7)
	wantStarts(t, s, q)
	wantNext(t, s, "cloud/run", "q f0-branch-setup")

	// Each of the four alone makes an item wait.
	for name, set := range map[string]func(f *Feature){
		"a due human step": func(f *Feature) {
			f.Steps[len(f.Steps)-1].Affinity = "human"
		},
		"an open blocking question": func(f *Feature) {
			f.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open"}}
		},
		"a PR at its merge step": func(f *Feature) { mergeWaiting(f, 9) },
		"a spent budget":         func(f *Feature) { f.Budget.Spent.Steps = 14 },
	} {
		f := withAreas(toPhase("f", 1, 3), "book")
		s := mapped(t, f)
		wantWaiting(t, s, 0)
		set(f)
		if got := s.WaitsOnHuman(); len(got) != 1 {
			t.Errorf("%s: %d items wait", name, len(got))
		}
	}
	// An assumed question, an answered one, a human step that still needs
	// another, and a merge step with no PR recorded are not waiting.
	f := withAreas(toPhase("f", 1, 3), "book")
	f.Questions = []Question{{ID: "a1", Kind: "assumed", Status: "open"},
		{ID: "q1", Kind: "blocking", Status: "answered"}}
	f.Steps = append(f.Steps, Step{ID: "f3-escalate", Phase: 3, Affinity: "human",
		Status: "pending", Needs: []string{"f3-implement"}})
	wantWaiting(t, mapped(t, f, mergeWaiting(feature("g", 2, "in_progress"), 0)), 0)

	// Case 30: queued items on hold and a tracked item are not counted.
	s = waitingState(t, 5, q)
	for i, kind := range []string{"", "process"} {
		_, h := held(t, kind, "FLOW-20 freeze")
		h.Feature, h.Priority = fmt.Sprintf("h%d", i), 40+i
		s.Features = append(s.Features, h)
	}
	away := &Feature{Feature: "m1", Priority: 50, Status: "in_progress", Round: "tracked",
		Phase: 1, Repos: []string{"buildwarden/buildwarden-relay"},
		Steps: []Step{{ID: "t1-work", Phase: 1, Affinity: "human", Status: "pending"}}}
	s.Features = append(s.Features, away)
	wantWaiting(t, s, 5)
	wantStarts(t, s, q)

	// Case 39: a human step of a phase the item has not reached is not
	// counted, and is once the item reaches it.
	s = waitingState(t, 7, q)
	later := withAreas(toPhase("later", 30, 3), "book")
	later.Steps = append(later.Steps, Step{ID: "f6-release-specs", Phase: 6,
		Affinity: "human", Status: "pending"})
	s.Features = append(s.Features, later)
	wantWaiting(t, s, 7)
	wantStarts(t, s, q)
	advance(later, 6)
	s.Claim(later, taking(t, later), "cloud/later", now, false)
	wantWaiting(t, s, 8)
	wantHeld(t, s, q, 8)
}

func waitingRedMain(t *testing.T) {
	// Case 40: the item main.yaml names as the fix for a red main starts,
	// though it follows no item, and only while main is recorded red.
	q, fix := queuedItem("q", 20), queuedItem("fix", 21)
	s := waitingState(t, 8, q, fix)
	wantHeld(t, s, fix, 8)
	s.Main = &Main{SHA: "0a1b2c3", CI: "red", Followup: "fix"}
	wantStarts(t, s, fix)
	wantNext(t, s, "cloud/run", "fix f0-branch-setup")
	if _, blocked := s.Next("cloud/run", now); !slices.ContainsFunc(blocked,
		func(b Blocked) bool { return b.Feature == "q" }) {
		t.Fatalf("wf next passed q over without a blocked line: %+v", blocked)
	}
	for _, ci := range []string{"green", "pending"} {
		s.Main.CI = ci
		wantHeld(t, s, fix, 8)
	}
}

func waitingMarked(t *testing.T) {
	// Case 42: a feature a human marked urgent starts at 8 waiting and at
	// 11, and an unmarked one behind it is refused.
	for _, n := range []int{8, 11} {
		m, q := queuedItem("m", 20), queuedItem("q", 21)
		m.Urgent = "human/jeff"
		s := waitingState(t, n, m, q)
		wantStarts(t, s, m)
		wantGuardHolds(t, s, q, n)
		wantNext(t, s, "cloud/run", "m f0-branch-setup")
		_, blocked := s.Next("cloud/run", now)
		if slices.ContainsFunc(blocked, func(b Blocked) bool { return b.Feature == "m" }) {
			t.Fatalf("wf next has a blocked line for the marked feature: %+v", blocked)
		}
		wantValidate(t, s)
		// A mark that names no human is honoured by nothing.
		m.Urgent = "cloud/tick-1"
		wantHeld(t, s, m, n)
		// Cleared, the feature is refused again.
		m.Urgent = ""
		wantHeld(t, s, m, n)
	}

	// Case 43: the mark lifts the waiting limit and nothing else. An
	// undeclared marked feature is refused by the in-progress limit.
	m := feature("m", 20, "queued")
	m.Urgent = "human/jeff"
	var fs []*Feature
	for n := 1; n <= 5; n++ {
		fs = append(fs, working(fmt.Sprintf("f%d", n), n, 3))
	}
	s := mapped(t, append(parked(fs...), m)...)
	wantRefused(t, s, m,
		"the in-progress limit (limit 5): it collides with f1, f2, f3, f4, f5 (FLOW-05)")

	// It waits under a process item's hold, and for its own blocking
	// question.
	m = queuedItem("m", 20)
	m.Urgent = "human/jeff"
	holder := process("holder", 1, "in_progress", "cloud")
	s = waitingState(t, 8, m, holder)
	wantOffer(t, s, "m", "held by process item holder")
	wantNext(t, s, "cloud/run", "none")
	s, m = held(t, "", "waits for the release")
	m.Urgent = "human/jeff"
	wantOffer(t, s, m.Feature, "waiting on human step f0-answer-q1")

	// Once it is in progress and waits on the human it counts like any
	// other.
	m = queuedItem("m", 20)
	m.Urgent = "human/jeff"
	s = waitingState(t, 8, m)
	s.Claim(m, &m.Steps[0], "cloud/run", now, true)
	m.Questions = []Question{{ID: "q1", Kind: "blocking", Status: "open"}}
	wantWaiting(t, s, 9)
}

// nextLines is what wf next prints for the worker: a blocked line for each
// item passed over, then the pick or none.
func nextLines(s *State, worker string) []string {
	pick, blocked := s.Next(worker, now)
	var out []string
	for _, b := range blocked {
		out = append(out, "blocked "+b.Feature+" "+b.Reason)
	}
	if pick == nil {
		return append(out, "none")
	}
	return append(out, "next "+pick.Feature.Feature+" "+pick.Step.ID)
}

// TestNextUnchangedOnUndeclaredQueue: on a queue where no item declares
// areas and no map exists, wf next and wf validate print what they printed
// before the limits counted by relation (FLOW-05). testdata/undeclared is the
// open items of the live queue of 2026-11-15, cut down to what wf next reads,
// with five features in progress and one queued behind the in-progress limit;
// the lines are those main's wf (ad4ea50) prints for it.
func TestNextUnchangedOnUndeclaredQueue(t *testing.T) {
	s, err := Load(filepath.Join("testdata", "undeclared"))
	if err != nil {
		t.Fatal(err)
	}
	const (
		merge = "blocked runtime-cli-project-path waiting on the human's merge of PR " +
			"#120 (FLOW-08)"
		trust   = "blocked project-config-trust waiting on human step f2-design-review"
		specs   = "blocked seam-specs waiting on human step f2-design-review"
		k1      = "blocked driver-contract waiting on human step f2-answer-q1"
		session = "cloud/20261112T230554Z"
	)
	host := []string{
		"blocked loop-parallel waiting on human step x4-state, cloud step x2-implement",
		merge, trust, "blocked prebuilt-artifacts waiting on cloud step f1-design", specs,
		k1, "none"}
	for worker, want := range map[string][]string{
		"cloud/check": {"blocked loop-parallel waiting on human step x4-state, " +
			"step x2-implement leased to " + session,
			merge, trust,
			"blocked prebuilt-artifacts waiting on step f1-design leased to " +
				session,
			specs, k1, "none"},
		"host:macos-vz/check":       host,
		"host:windows-hyperv/check": host,
		"human/jeff":                {"next loop-parallel x4-state"},
	} {
		if got := nextLines(s, worker); !slices.Equal(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", worker, got, want)
		}
	}
	wantValidate(t, s)
	// Four of its items wait on the human, half the limit, so the waiting
	// limit changes nothing for it either.
	wantWaiting(t, s, 4)
}

// TestUrgent: the urgent mark is a human's, on a queued feature the waiting
// limit could refuse; it is refused on a process item, a tracked item and an
// item that follows one, under a human's name too; clearing it is a human's
// and is allowed on any item (FLOW-05; case 44). Nothing is asked about what
// the item fixes: the rule's words and intake hold the mark to a security
// fix.
func TestUrgent(t *testing.T) {
	t.Run("marked", urgentMarked)
	t.Run("cleared", urgentCleared)
}

// unnamed is the refusal of a mark by a human worker with no name.
const unnamed = "the mark names the human who sets it, as human/<name> (FLOW-05)"

func urgentMarked(t *testing.T) {
	const never = ": the waiting limit never refuses "
	f, parent := feature("a", 2, "queued"), feature("parent", 3, "done")
	fix, p := fixing(feature("fix", 4, "queued"), "parent"), process("p", 1, "queued")
	away := &Feature{Feature: "m1", Priority: 5, Status: "queued", Round: "tracked",
		Phase: 1, Repos: []string{"o/relay"}}
	s := declaredState(f, parent, fix, p, away)

	events := len(f.History)
	if err := s.MarkUrgent(f, "human/jeff", now); err != nil {
		t.Fatal(err)
	}
	if f.Urgent != "human/jeff" || len(f.History) != events+1 ||
		f.History[events] != (Event{At: now, By: "human/jeff",
			Event: "marked urgent (FLOW-05)"}) {
		t.Fatalf("marked: urgent %q, history %+v", f.Urgent, f.History[events:])
	}
	wantValidate(t, s)

	for _, c := range []struct {
		f      *Feature
		worker string
		want   string
	}{
		{feature("b", 6, "queued"), "cloud/tick-1",
			"only a human marks an item urgent (FLOW-05)"},
		{feature("b", 6, "queued"), "host:macos-vz/s",
			"only a human marks an item urgent (FLOW-05)"},
		{feature("b", 6, "queued"), "", "only a human marks an item urgent (FLOW-05)"},
		{feature("b", 6, "queued"), "human", unnamed},
		{feature("b", 6, "queued"), "human/", unnamed},
		{p, "human/jeff", "p is a process item" + never + "one (FLOW-05)"},
		{away, "human/jeff", "m1 is tracked" + never + "one (FLOW-05)"},
		{fix, "human/jeff", "fix follows parent" + never + "a fix (FLOW-05)"},
		{fix, "cloud/tick-1", "fix follows parent" + never + "a fix (FLOW-05)"},
		{feature("b", 6, "in_progress"), "human/jeff",
			"b is in progress: the waiting limit refuses only a start (FLOW-05)"},
		{feature("b", 6, "done"), "human/jeff",
			"b is done: the waiting limit refuses only a start (FLOW-05)"},
	} {
		events := len(c.f.History)
		err := s.MarkUrgent(c.f, c.worker, now)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s by %q: %v, want %q", c.f.Feature, c.worker, err, c.want)
		}
		if c.f.Urgent != "" || len(c.f.History) != events {
			t.Errorf("%s by %q: a refused mark changed it", c.f.Feature, c.worker)
		}
	}
}

func urgentCleared(t *testing.T) {
	f := feature("a", 2, "queued")
	f.Urgent = "human/jeff"
	s := declaredState(f)
	for _, w := range []string{"cloud/tick-1", "host:macos-vz/s", ""} {
		if err := s.ClearUrgent(f, w, now); err == nil ||
			err.Error() != "only a human clears the mark" || f.Urgent == "" {
			t.Errorf("clear by %q: %v, urgent %q", w, err, f.Urgent)
		}
	}
	urgentUnnamedRefused(t, s, f)
	// Clearing is allowed on any item: one in progress or done keeps the
	// mark as the record of its start until a human takes it away.
	for _, status := range []string{"queued", "in_progress", "done"} {
		f.Status, f.Urgent = status, "human/jeff"
		if err := s.ClearUrgent(f, "human/sam", now); err != nil || f.Urgent != "" {
			t.Fatalf("clear on a %s item: %v, urgent %q", status, err, f.Urgent)
		}
		if last := f.History[len(f.History)-1]; last.Event != "urgent cleared" ||
			last.By != "human/sam" {
			t.Fatalf("clear logged %+v", last)
		}
	}
	if err := s.ClearUrgent(f, "human/jeff", now); err == nil ||
		err.Error() != "a is not marked urgent" {
		t.Fatalf("clear on an unmarked item: %v", err)
	}
}

// urgentUnnamedRefused: a human worker with no name neither clears a mark
// nor makes one that anything honours; Validate fails it.
func urgentUnnamedRefused(t *testing.T, s *State, f *Feature) {
	t.Helper()
	for _, w := range []string{"human", "human/"} {
		if err := s.ClearUrgent(f, w, now); err == nil || err.Error() != unnamed ||
			f.Urgent == "" {
			t.Errorf("clear by %q: %v, urgent %q", w, err, f.Urgent)
		}
		g := feature("u", 3, "queued")
		g.Urgent = w
		if g.marked() {
			t.Errorf("urgent %q is honoured", w)
		}
		wantValidate(t, declaredState(g), "u: urgent names \""+w+"\": "+unnamed)
	}
}

// TestUrgentStartLine: the claim that starts a marked feature while the count
// is at the waiting limit or over says so in the item's history, where an
// unmarked start, or a marked one under the limit, logs started (FLOW-05;
// case 42).
func TestUrgentStartLine(t *testing.T) {
	started := func(f *Feature) string {
		for _, e := range f.History {
			if strings.HasPrefix(e.Event, "started") {
				return e.Event
			}
		}
		return ""
	}
	for _, c := range []struct {
		waiting int
		urgent  string
		line    string
	}{
		{8, "human/jeff", "started past the waiting limit (8 wait, limit 8): marked " +
			"urgent by human/jeff (FLOW-05)"},
		{11, "human/sam", "started past the waiting limit (11 wait, limit 8): marked " +
			"urgent by human/sam (FLOW-05)"},
		{7, "human/jeff", ""},
		{7, "", ""},
	} {
		m := queuedItem("m", 20)
		m.Urgent = c.urgent
		s := waitingState(t, c.waiting, m)
		if why := s.Guard(m, &m.Steps[0], "cloud/run"); why != "" {
			t.Fatalf("%d waiting, urgent %q: %s", c.waiting, c.urgent, why)
		}
		line := s.Claim(m, &m.Steps[0], "cloud/run", now, true)
		want := c.line
		if want == "" {
			want = "started (FLOW-05)"
		}
		if line != c.line || started(m) != want || m.Status != "in_progress" {
			t.Errorf("%d waiting, urgent %q: Claim returned %q and logged %q",
				c.waiting, c.urgent, line, started(m))
		}
		// The mark stays, as the record of why the item started.
		if m.Urgent != c.urgent {
			t.Errorf("the claim changed the mark to %q", m.Urgent)
		}
	}
	// The fix for a red main starts for that reason, marked or not.
	m := queuedItem("m", 20)
	m.Urgent = "human/jeff"
	s := waitingState(t, 8, m)
	s.Main = &Main{SHA: "0a1b2c3", CI: "red", Followup: "m"}
	if line := s.Claim(m, &m.Steps[0], "cloud/run", now, true); line != "" {
		t.Fatalf("the fix for a red main: Claim returned %q", line)
	}
}

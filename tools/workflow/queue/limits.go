package queue

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// limit is one row of FLOW-05's limits table: the claim that takes the limit,
// the state that holds it, which relations put a holder in another's set, and
// the most a set may number. limits is the only place a limit is written;
// Guard and Validate both evaluate it.
type limit struct {
	// what names the limit in a Guard reason.
	what string
	// key is the limit's key under config.yaml's limits, "" for a limit that
	// is not configurable.
	key string
	// near puts the holders near an item in its set, beside those that
	// collide with it.
	near bool
	most func(s *State) int
	// takes reports whether the worker's claim of the step would make the
	// item a holder.
	takes func(s *State, f *Feature, st *Step, worker string) bool
	holds func(s *State, f *Feature) bool
	// full is Guard's reason when the item's own set would be over.
	full func(head string, with []member) string
	// over is Validate's finding, after its prefix, for a holder whose set
	// is over.
	over func(most int, h *Feature, with []member) string
	// ahead is why the item waits its turn though no set would be over, or
	// "": the PR slot's tie-break.
	ahead func(s *State, f *Feature) string
}

// member is a holder in another item's set, with how the two relate.
type member struct {
	f   *Feature
	rel Relation
}

// limits is FLOW-05's table of the three limits that count by relation. The
// waiting limit is not in it: it counts by no relation and nothing holds it
// (waitingRefusal).
var limits = []*limit{
	{
		what: "the in-progress limit", key: "in_progress",
		most: (*State).inProgressLimit,
		takes: func(s *State, f *Feature, _ *Step, worker string) bool {
			return s.takesStart(f, worker)
		},
		holds: func(_ *State, f *Feature) bool { return f.Status == "in_progress" },
		full: func(head string, with []member) string {
			return fmt.Sprintf("%s: it collides with %s (FLOW-05)", head, names(with))
		},
		over: func(most int, h *Feature, with []member) string {
			return fmt.Sprintf("%d features in progress, limit %d (FLOW-05): %s",
				len(with)+1, most, h.Feature+", "+names(with))
		},
	},
	{
		what: "an alters_relationships slot", key: "alters_relationships",
		most: (*State).altersLimit,
		takes: func(_ *State, f *Feature, st *Step, _ string) bool {
			return IsPlan(st) && f.alters()
		},
		holds: func(_ *State, f *Feature) bool { return f.holdsAltersSlot() },
		full: func(head string, with []member) string {
			return fmt.Sprintf("%s, held by %s (FLOW-05)", head, names(with))
		},
		over: func(most int, h *Feature, with []member) string {
			return fmt.Sprintf("%d features hold an alters_relationships slot, "+
				"limit %d (FLOW-05): %s", len(with)+1, most,
				h.Feature+", "+names(with))
		},
	},
	{
		what: "the PR slot", near: true,
		most: func(*State) int { return 1 },
		takes: func(_ *State, _ *Feature, st *Step, _ string) bool {
			return IsMergeMain(st)
		},
		holds: func(_ *State, f *Feature) bool { return f.holdsPRSlot() },
		full: func(head string, with []member) string {
			return fmt.Sprintf("%s, held by %s (FLOW-05)", head, related(with))
		},
		over: func(most int, h *Feature, with []member) string {
			return fmt.Sprintf("%s holds the PR slot with %s, limit %d (FLOW-05)",
				h.Feature, related(with), most)
		},
		ahead: (*State).prSlotAhead,
	},
}

// names lists the members by name.
func names(ms []member) string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.f.Feature
	}
	return strings.Join(out, ", ")
}

// related lists the members, each with how it relates to the item whose set
// they are in: a (colliding: area driver-core), b (near: rule GLOBAL-08).
func related(ms []member) string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = fmt.Sprintf("%s (%s: %s)", m.f.Feature, m.rel.Kind, m.rel.Reason)
	}
	return strings.Join(out, ", ")
}

// head is how a Guard reason opens: the limit, and its number where the
// human sets it.
func (l *limit) head(most int) string {
	if l.key == "" {
		return l.what
	}
	return fmt.Sprintf("%s (limit %d)", l.what, most)
}

// counted reports whether the limits count the item: a feature that is not
// tracked (FLOW-05). A process item stops other work only through its holds,
// and a tracked item rests on state entered by hand.
func (s *State) counted(f *Feature) bool { return !f.IsProcess() && !s.Tracked(f) }

// takesStart reports whether the worker's claim of a step of the item takes
// the in-progress limit: the item is a queued counted feature and the worker
// is not a human, whose steps start no feature. Only the cloud's claim sets
// the feature in progress today (Starts); a host's is bounded as if it did,
// so a machine that takes an item's first step is held to the limits the
// cloud is (FLOW-04).
func (s *State) takesStart(f *Feature, worker string) bool {
	return f.Status == "queued" && s.counted(f) && Kind(worker) != "human"
}

// holders returns the items that hold the limit, in FLOW-05 order.
func (s *State) holders(l *limit) []*Feature {
	var out []*Feature
	for _, f := range s.Ordered() {
		if s.counted(f) && l.holds(s, f) {
			out = append(out, f)
		}
	}
	return out
}

// set returns the item's set for the limit without the item itself: the
// holders that collide with it and, for a limit that counts them, are near
// it.
func (s *State) set(l *limit, f *Feature, holders []*Feature) []member {
	var out []member
	for _, h := range holders {
		if h == f {
			continue
		}
		if r := s.Relation(f, h); r.Kind == Colliding || (l.near && r.Kind == Near) {
			out = append(out, member{h, r})
		}
	}
	return out
}

// Guard returns why the worker may not claim the step now, or "" (FLOW-05).
// It is the one test of the limits for every worker, behind wf next and wf
// claim alike. It applies the waiting limit, then each limit of the table the
// claim would take and the item does not hold: with the item added to the
// holders, no set may number more than the limit, the item's own or that of
// a holder related to it. The relations are not transitive, so a claim whose
// own set is small can still fill a holder's. Only an item that does not hold
// is asked and only holders are counted, so no two items wait for each other,
// and a state Guard allows has no limit finding in Validate.
func (s *State) Guard(f *Feature, st *Step, worker string) string {
	return s.guard(f, st, worker).why
}

// refusal is Guard's answer: the reason, and whether it is the waiting
// limit's, which wf next reports for a queued item where it passes over the
// in-progress limit's in silence.
type refusal struct {
	why     string
	waiting bool
}

func (s *State) guard(f *Feature, st *Step, worker string) refusal {
	if !s.counted(f) {
		return refusal{}
	}
	if why := s.waitingRefusal(f, worker); why != "" {
		return refusal{why: why, waiting: true}
	}
	for _, l := range limits {
		if !l.takes(s, f, st, worker) || l.holds(s, f) {
			continue
		}
		if why := s.refused(l, f); why != "" {
			return refusal{why: why}
		}
	}
	return refusal{}
}

// refused evaluates one limit's predicate on the state the item's claim would
// produce, and then its tie-break.
func (s *State) refused(l *limit, f *Feature) string {
	most := l.most(s)
	if most < 1 {
		// A written limit below 1 gives nothing to wait for: the reason is
		// the value, which Validate reports too (GLOBAL-10).
		return fmt.Sprintf("limits.%s %d is below 1 (FLOW-05)", l.key, most)
	}
	holders := s.holders(l)
	mine := s.set(l, f, holders)
	if len(mine)+1 > most {
		return l.full(l.head(most), mine)
	}
	for _, m := range mine {
		// The holder's set after the claim: itself, the item, and theirs.
		if theirs := s.set(l, m.f, holders); len(theirs)+2 > most {
			how := "collides with"
			if m.rel.Kind == Near {
				how = "is near"
			}
			return fmt.Sprintf("%s: %s, which %s it, is already with %s (FLOW-05)",
				l.head(most), m.f.Feature, how, names(theirs))
		}
	}
	if l.ahead != nil {
		return l.ahead(s, f)
	}
	return ""
}

// validateLimits evaluates each limit's predicate for every holder: Guard's
// test, on the state as it stands. A set that is over is reported once, for
// its first holder in FLOW-05 order.
func (s *State) validateLimits() []error {
	var errs []error
	for _, l := range limits {
		most := l.most(s)
		if most < 1 {
			// No count means anything against a limit below 1, which
			// validateConfig reports.
			continue
		}
		holders := s.holders(l)
		seen := map[string]bool{}
		for _, h := range holders {
			with := s.set(l, h, holders)
			if len(with)+1 <= most {
				continue
			}
			set := []string{h.Feature}
			for _, m := range with {
				set = append(set, m.f.Feature)
			}
			slices.Sort(set)
			if key := strings.Join(set, " "); !seen[key] {
				seen[key] = true
				errs = append(errs, fmt.Errorf("limit: %s", l.over(most, h, with)))
			}
		}
	}
	return errs
}

// altersLimit is how many features may hold an alters_relationships slot: the
// value config.yaml writes (FLOW-20), or FLOW-05's 1 when it writes none. A
// written value is returned as it is, one below 1 included, which Validate
// reports and under which Guard gives no slot: no default stands in for it
// (GLOBAL-10).
func (s *State) altersLimit() int {
	if n := s.Config.Limits.AltersRelationships; n != nil {
		return *n
	}
	return 1
}

// holdsAltersSlot reports whether the feature holds an alters_relationships
// slot (FLOW-05): it is in progress, is not a process item, declares
// alters_relationships: true, and one of its Plan steps is done or carries a
// claim. An expired claim still counts, so the answer reads no clock and wf
// next and wf validate cannot differ across an expiry; wf release gives the
// slot back. A done Plan step survives a return to Design (Back), so a
// feature with a finished plan keeps its slot through one (FLOW-06).
func (f *Feature) holdsAltersSlot() bool {
	if f.Status != "in_progress" || f.IsProcess() || !f.alters() {
		return false
	}
	for i := range f.Steps {
		st := &f.Steps[i]
		if IsPlan(st) && (st.Status == "done" || st.Lease.ClaimedBy != "") {
			return true
		}
	}
	return false
}

// hasPR reports whether the item has a pull request recorded.
func (f *Feature) hasPR() bool { return f.PR != nil && f.PR.Number > 0 }

// mergeMain returns the feature's newest step that merges main at the start
// of Publish, the last of its -r<n> forms, or nil.
func (f *Feature) mergeMain() *Step {
	var newest *Step
	for i := range f.Steps {
		if IsMergeMain(&f.Steps[i]) {
			newest = &f.Steps[i]
		}
	}
	return newest
}

// holdsPRSlot reports whether the feature holds the PR slot (FLOW-05): it is
// in Publish and its newest f6-merge-main is done or carries a claim, expired
// or not, as a Plan claim holds an alters_relationships slot; or it has a PR
// recorded and has not reached Close, whatever its phase, so a return to
// Implement or Design with the PR open keeps the slot. A return before the PR
// was opened gives it up, and Publish entered again holds it only from the
// claim of the new f6-merge-main. An item in Close holds nothing.
func (f *Feature) holdsPRSlot() bool {
	if f.Status != "in_progress" {
		return false
	}
	if f.hasPR() && f.Phase < 8 {
		return true
	}
	st := f.mergeMain()
	return f.Phase == 6 && st != nil && (st.Status == "done" || st.Lease.ClaimedBy != "")
}

// waitsForPRSlot reports whether the item waits for the PR slot: a counted
// feature in Publish whose newest f6-merge-main is pending with no claim,
// with no PR recorded, no open blocking question and no spent runaway budget.
func (s *State) waitsForPRSlot(f *Feature) bool {
	if !s.counted(f) || f.Status != "in_progress" || f.Phase != 6 || f.hasPR() {
		return false
	}
	st := f.mergeMain()
	return st != nil && st.Status == "pending" && st.Lease.ClaimedBy == "" &&
		len(f.OpenBlocking()) == 0 && !s.Runaway(f)
}

// prSlotAhead is the PR slot's tie-break: the item is refused while an item
// that collides with it or is near it, and has a higher priority, waits for
// the slot, whether or not that item could take the slot itself. Both are in
// Publish, so priority alone orders them, and it is strict: the waits form no
// cycle, and the highest-priority waiting item of a group is refused only by
// a holder. The reason names the highest priority ahead.
func (s *State) prSlotAhead(f *Feature) string {
	var first *Feature
	for _, o := range s.Features {
		if o == f || o.Priority >= f.Priority || !s.waitsForPRSlot(o) ||
			(first != nil && first.Priority < o.Priority) {
			continue
		}
		if k := s.Relation(f, o).Kind; k == Colliding || k == Near {
			first = o
		}
	}
	if first == nil {
		return ""
	}
	return fmt.Sprintf("the PR slot: %s (priority %d) is ahead (FLOW-05)", first.Feature,
		first.Priority)
}

// WaitsOnHuman returns the items that wait on the human, each once, in
// FLOW-05 order: the count the waiting limit compares, and the number
// QUEUE.md's Needs Jeff opens with (FLOW-05). An item waits when it is in
// progress, is not tracked, and has a human step that is due, an open
// blocking question, a pull request at its merge step, or a spent runaway
// budget (FLOW-13). A queued item is not counted, even one on hold: the loop
// has put no work behind it.
func (s *State) WaitsOnHuman() []*Feature {
	var out []*Feature
	for _, f := range s.Ordered() {
		if f.Status == "in_progress" && !s.Tracked(f) && s.waitsOnHuman(f) {
			out = append(out, f)
		}
	}
	return out
}

func (s *State) waitsOnHuman(f *Feature) bool {
	if len(f.OpenBlocking()) > 0 || s.Runaway(f) {
		return true
	}
	done := map[string]bool{}
	for _, st := range f.Steps {
		if st.Status == "done" {
			done[st.ID] = true
		}
	}
	for i := range f.Steps {
		st := &f.Steps[i]
		// A human step of a later phase is offered early so the human can
		// prepare (FLOW-04); nothing waits on it yet.
		if st.Status != "pending" || st.Phase > f.Phase || !allDone(st.Needs, done) {
			continue
		}
		if st.Affinity == "human" || (IsMerge(st) && f.hasPR()) {
			return true
		}
	}
	return false
}

// fixesMain reports whether the item is the fix main.yaml names for a main
// recorded red (FLOW-09).
func (s *State) fixesMain(f *Feature) bool {
	return s.Main != nil && s.Main.CI == "red" && s.Main.Followup == f.Feature
}

// marked reports whether a human marked the item urgent. A value that is not
// a human worker is honoured by nothing: Validate fails it (GLOBAL-10).
func (f *Feature) marked() bool { return f.Urgent != "" && Kind(f.Urgent) == "human" }

// pastWaitingLimit reports whether the items that wait on the human number
// the waiting limit or more, with the count and the limit.
func (s *State) pastWaitingLimit() (waiting []*Feature, limit int, past bool) {
	waiting, limit = s.WaitsOnHuman(), s.waitingLimit()
	return waiting, limit, len(waiting) >= limit
}

// waitingRefusal is the waiting limit: no queued feature starts while the
// items that wait on the human number the limit or more (FLOW-05). It refuses
// only the claim that takes the in-progress limit, so work in progress goes
// on and the count can pass the limit. A fix that follows an item, the fix
// for a red main and a feature a human marked urgent start whatever the
// count; a process item and a tracked item are not counted features, and
// Guard has passed them already.
func (s *State) waitingRefusal(f *Feature, worker string) string {
	if !s.takesStart(f, worker) || f.Follows != "" || s.fixesMain(f) || f.marked() {
		return ""
	}
	waiting, limit, past := s.pastWaitingLimit()
	if limit < 1 {
		return fmt.Sprintf("limits.waiting_on_human %d is below 1 (FLOW-05)", limit)
	}
	if !past {
		return ""
	}
	out := make([]string, len(waiting))
	for i, w := range waiting {
		out[i] = w.Feature
	}
	return fmt.Sprintf(
		"the waiting limit: %d items wait on the human (limit %d): %s (FLOW-05)",
		len(waiting), limit, strings.Join(out, ", "))
}

// startedPast returns the history line for the start of a queued feature
// that only its urgent mark lets start: the items that wait on the human
// number the waiting limit or more, and the item is no fix. It is "" for
// every other start.
func (s *State) startedPast(f *Feature) string {
	if !s.counted(f) || !f.marked() || f.Follows != "" || s.fixesMain(f) {
		return ""
	}
	waiting, limit, past := s.pastWaitingLimit()
	if !past {
		return ""
	}
	return fmt.Sprintf("started past the waiting limit (%d wait, limit %d): "+
		"marked urgent by %s (FLOW-05)", len(waiting), limit, f.Urgent)
}

// MarkUrgent marks a queued feature urgent for the human worker, so that the
// waiting limit does not refuse its start (FLOW-05). It is the rule of wf new
// -urgent and wf urgent. The mark is refused on an item the waiting limit
// never refuses, whoever asks: a process item, a tracked item, an item that
// follows one. It is refused for a worker that is not a human, and on an item
// that has started. What the item fixes is not asked: FLOW-05's words hold
// the mark to a security fix, and intake judges that.
func (s *State) MarkUrgent(f *Feature, worker string, now time.Time) error {
	const never = "the waiting limit never refuses"
	switch {
	case f.IsProcess():
		return fmt.Errorf("%s is a process item: %s one (FLOW-05)", f.Feature, never)
	case s.Tracked(f):
		return fmt.Errorf("%s is tracked: %s one (FLOW-05)", f.Feature, never)
	case f.Follows != "":
		return fmt.Errorf("%s follows %s: %s a fix (FLOW-05)", f.Feature, f.Follows, never)
	case Kind(worker) != "human":
		return errors.New("only a human marks an item urgent (FLOW-05)")
	case f.Status != "queued":
		return fmt.Errorf("%s is %s: the waiting limit refuses only a start (FLOW-05)",
			f.Feature, strings.ReplaceAll(f.Status, "_", " "))
	}
	f.Urgent = worker
	f.Log(now, worker, "marked urgent (FLOW-05)")
	return nil
}

// ClearUrgent removes the urgent mark from any item, for a human worker
// only: clearing puts the waiting limit back, and the mark is a human's.
func (s *State) ClearUrgent(f *Feature, worker string, now time.Time) error {
	if Kind(worker) != "human" {
		return errors.New("only a human clears the mark")
	}
	if f.Urgent == "" {
		return fmt.Errorf("%s is not marked urgent", f.Feature)
	}
	f.Urgent = ""
	f.Log(now, worker, "urgent cleared")
	return nil
}

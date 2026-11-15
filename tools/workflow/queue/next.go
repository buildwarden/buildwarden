package queue

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// Pick is the step a worker should take next.
type Pick struct {
	Feature *Feature
	Step    *Step
	// Starts is true when taking the step starts a queued feature.
	Starts bool
}

// Blocked says why an item has no step for the worker: an in-progress item,
// a fix waiting on the item it follows, or a queued feature the waiting limit
// refuses.
type Blocked struct {
	Feature string
	Reason  string
}

// Satisfies reports whether a worker may take a step of the given affinity
// (FLOW-04). A worker is its kind (cloud, human, host:<name>), optionally
// followed by /<session> to tell its leases apart.
func Satisfies(worker, affinity string) bool { return Kind(worker) == affinity }

// Kind strips the /<session> suffix from a worker.
func Kind(worker string) string { return strings.SplitN(worker, "/", 2)[0] }

// Hold returns the in-progress process item, other than f, whose holds match
// the worker, or "" when the worker may take new steps of f (FLOW-05).
func (s *State) Hold(f *Feature, worker string) string {
	for _, o := range s.Features {
		if o == f || !o.IsProcess() || o.Status != "in_progress" {
			continue
		}
		for _, h := range o.Holds {
			if ok, _ := path.Match(h, Kind(worker)); ok {
				return o.Feature
			}
		}
	}
	return ""
}

// Offered reports whether the feature has reached the step's phase, so a
// worker may be offered it (FLOW-04). A human step is offered early, so the
// human can prepare a later phase ahead of time; it never wakes a machine.
func Offered(f *Feature, st *Step) bool {
	return st.Phase <= f.Phase || st.Affinity == "human"
}

// Early returns the FLOW-04 refusal for claiming or completing a step whose
// phase the feature has not reached, or nil when the step is Offered.
func Early(f *Feature, st *Step) error {
	if Offered(f, st) {
		return nil
	}
	return fmt.Errorf("%s is a phase %d step; %s is in phase %d (FLOW-04); "+
		"release it with the evidence", st.ID, st.Phase, f.Feature, f.Phase)
}

// leasedTo returns the feature's offered pending step the worker already
// holds a live lease on: a hold never takes away a step already claimed.
func leasedTo(f *Feature, worker string, now time.Time) *Step {
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status == "pending" && Offered(f, st) && st.Lease.Held(now) &&
			st.Lease.ClaimedBy == worker {
			return st
		}
	}
	return nil
}

// Next picks the worker's next step in FLOW-05 order (Ordered): by class,
// in-progress before queued within a class, then priority. An in-progress
// item gives its first ready step. A queued item starts only when every item
// ahead of it is blocked: never under a hold, only when the limits allow the
// claim (Guard), and for a fix only once the item it follows is done. A human
// may take a queued item's step of its affinity without starting it, unless
// it is a process item. A worker an in-progress process item holds gets only
// the steps it already has leased. No worker is offered a merge step
// (IsMerge): an item waiting on the human's merge is blocked like one waiting
// on a human step, and the walk goes on (FLOW-05, FLOW-08). No worker is
// offered a step whose claim a limit refuses; only a claim that takes a limit
// the item does not hold is asked, so a holder is never stopped by one. It
// also returns why each item it passed over is blocked, for the digest: an
// in-progress item, a fix waiting on its parent, and a queued feature the
// waiting limit refuses. A queued feature the in-progress limit refuses is
// passed over without a line, as it always was.
func (s *State) Next(worker string, now time.Time) (*Pick, []Blocked) {
	var blocked []Blocked
	held := s.Hold(nil, worker) != ""
	for _, f := range s.Ordered() {
		var st *Step
		why := ""
		switch {
		case f.Status == "in_progress":
			st, why = s.inProgressStep(f, worker, now)
		case f.Status != "queued" || held:
			continue
		case s.waitsForParent(f):
			why = "follows " + f.Follows + ", not yet done (FLOW-05)"
		default:
			var reported bool
			if st, why, reported = s.offer(f, worker, now); st == nil && !reported {
				continue
			}
		}
		if st != nil {
			return &Pick{Feature: f, Step: st, Starts: s.Starts(f, worker)}, blocked
		}
		blocked = append(blocked, Blocked{f.Feature, why})
	}
	return nil, blocked
}

// Starts reports whether the worker's claim of a step of the item sets it in
// progress: a cloud claim on a queued feature, and any claim on a queued
// process item, whose steps are all human.
func (s *State) Starts(f *Feature, worker string) bool {
	return f.Status == "queued" && (Kind(worker) == "cloud" || f.IsProcess())
}

// inProgressStep is the in-progress item's step for the worker, or why there
// is none: under a hold, only a step the worker already has leased.
func (s *State) inProgressStep(f *Feature, worker string, now time.Time) (*Step, string) {
	if h := s.Hold(f, worker); h != "" {
		if st := leasedTo(f, worker, now); st != nil {
			return st, ""
		}
		return nil, "held by process item " + h
	}
	st, why, _ := s.offer(f, worker, now)
	return st, why
}

// waitsForParent reports whether a queued fix follows an item still open, so
// it is never built on a main that lacks that item. A process item's follows
// is only a record.
func (s *State) waitsForParent(f *Feature) bool {
	if f.Follows == "" || f.IsProcess() {
		return false
	}
	p, err := s.Find(f.Follows)
	return err == nil && p.Status != "done"
}

// offer returns the feature's first step the worker can take now, or why
// there is none. A step of a later phase is skipped without adding to the
// reasons: the current phase's steps already say why the feature waits. A
// merge step is skipped for every worker, whatever its affinity: only wf done
// in the tick's merge confirmation completes it (FLOW-08). A step whose claim
// a limit refuses is skipped for every worker (Guard). For a queued feature
// that refusal is the whole reason, since every step of it waits on the
// start; reported says it is the waiting limit's, the one such refusal wf
// next prints for a queued item.
func (s *State) offer(f *Feature, worker string, now time.Time) (
	st *Step, why string, reported bool,
) {
	done := map[string]bool{}
	for _, st := range f.Steps {
		if st.Status == "done" {
			done[st.ID] = true
		}
	}
	var waits []string
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status != "pending" || !Offered(f, st) || !allDone(st.Needs, done) {
			continue
		}
		wait, refused := s.wait(f, st, worker, now)
		if refused.why != "" && f.Status == "queued" {
			return nil, refused.why, refused.waiting
		}
		if wait == "" {
			return st, "", false
		}
		waits = append(waits, wait)
	}
	if len(waits) == 0 {
		return nil, "no pending step (phase done-when not yet advanced?)", false
	}
	return nil, "waiting on " + strings.Join(waits, ", "), false
}

// wait returns what the worker waits on before it can take the offered step,
// or "", with Guard's refusal when that is the reason.
func (s *State) wait(f *Feature, st *Step, worker string, now time.Time) (string, refusal) {
	switch {
	case IsMerge(st):
		return mergeWait(f), refusal{}
	case !Satisfies(worker, st.Affinity):
		return fmt.Sprintf("%s step %s", st.Affinity, st.ID), refusal{}
	case st.Lease.Held(now) && st.Lease.ClaimedBy != worker:
		return fmt.Sprintf("step %s leased to %s", st.ID, st.Lease.ClaimedBy), refusal{}
	}
	if Kind(worker) == "cloud" {
		if why := s.cloudHold(f); why != "" {
			return why, refusal{}
		}
	}
	r := s.guard(f, st, worker)
	return r.why, r
}

// mergeWait is the reason an item at its merge step waits.
func mergeWait(f *Feature) string {
	if f.PR != nil && f.PR.Number > 0 {
		return fmt.Sprintf("the human's merge of PR #%d (FLOW-08)", f.PR.Number)
	}
	return "the human's merge (no PR recorded, FLOW-08)"
}

// cloudHold returns why the unattended loop must not take a step of the item
// yet: an open blocking question (FLOW-07) or a spent runaway budget
// (FLOW-13). These two are the cloud's alone and are not limits; the limits
// are Guard's, for every worker.
func (s *State) cloudHold(f *Feature) string {
	if q := f.OpenBlocking(); len(q) > 0 {
		return fmt.Sprintf("blocking question %s", q[0].ID)
	}
	if s.Runaway(f) {
		return fmt.Sprintf("runaway budget (%d of %d estimated steps spent; "+
			"escalate to human, FLOW-13)",
			f.Budget.Spent.Steps, f.Budget.Estimate.Steps)
	}
	return ""
}

// NextFor is Next restricted to one feature, for an attended run on it. The
// limits apply as they do in Next (Guard), and the reason a start is refused
// is returned whichever limit it is. It does not wait for the item a fix
// follows: naming the item is the human's explicit word.
func (s *State) NextFor(name, worker string, now time.Time) (*Pick, []Blocked) {
	f, err := s.Find(name)
	if err != nil {
		return nil, []Blocked{{name, err.Error()}}
	}
	if h := s.Hold(f, worker); h != "" {
		if st := leasedTo(f, worker, now); st != nil {
			return &Pick{Feature: f, Step: st}, nil
		}
		return nil, []Blocked{{name, "held by process item " + h}}
	}
	st, why, _ := s.offer(f, worker, now)
	if st == nil {
		return nil, []Blocked{{name, why}}
	}
	return &Pick{Feature: f, Step: st, Starts: s.Starts(f, worker)}, nil
}

func allDone(ids []string, done map[string]bool) bool {
	for _, id := range ids {
		if !done[id] {
			return false
		}
	}
	return true
}

// Claim leases the step to worker for the configured lease time and counts it
// against the feature's budget. newRun counts a run too, the first time a run
// touches the feature. A cloud claim on a queued feature starts it, and so
// does any claim on a queued process item, whose steps are all human.
func (s *State) Claim(f *Feature, st *Step, worker string, now time.Time, newRun bool) {
	mins := s.Config.LeaseMinutes
	if mins == 0 {
		mins = 120
	}
	exp := now.Add(time.Duration(mins) * time.Minute).UTC()
	st.Lease = Lease{ClaimedBy: worker, Expires: &exp}
	if s.Starts(f, worker) {
		f.Status = "in_progress"
		f.Log(now, worker, "started (FLOW-05)")
	}
	f.Budget.Spent.Steps++
	if newRun {
		f.Budget.Spent.Runs++
	}
	f.Log(now, worker, "claimed %s until %s", st.ID, exp.Format(time.RFC3339))
}

// Release drops a lease without completing the step.
func (s *State) Release(f *Feature, st *Step, worker string, now time.Time, why string) {
	st.Lease = Lease{}
	f.Log(now, worker, "released %s: %s", st.ID, why)
}

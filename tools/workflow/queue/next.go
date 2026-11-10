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

// Blocked says why an in-progress item, or a fix waiting on the item it
// follows, has no step for the worker.
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
// ahead of it is blocked: never under a hold, for the cloud only within the
// in-progress limit (which counts no process items), and for a fix only once the item it follows is done. A
// human or host may take a queued item's step of its affinity without
// starting it, unless it is a process item. A worker an in-progress process
// item holds gets only the steps it already has leased. It also returns why
// each in-progress item, and each fix waiting on its parent, it passed over
// is blocked, for the digest.
func (s *State) Next(worker string, now time.Time) (*Pick, []Blocked) {
	var blocked []Blocked
	cloud := Kind(worker) == "cloud"
	mayStart := s.Hold(nil, worker) == "" && (!cloud || !s.atLimit())
	for _, f := range s.Ordered() {
		var st *Step
		why := ""
		switch {
		case f.Status == "in_progress":
			st, why = s.inProgressStep(f, worker, now)
		case f.Status != "queued" || !mayStart:
			continue
		case s.waitsForParent(f):
			why = "follows " + f.Follows + ", not yet done (FLOW-05)"
		default:
			if st, _ = s.ready(f, worker, now); st == nil {
				continue
			}
		}
		if st != nil {
			starts := f.Status == "queued" && (cloud || f.IsProcess())
			return &Pick{Feature: f, Step: st, Starts: starts}, blocked
		}
		blocked = append(blocked, Blocked{f.Feature, why})
	}
	return nil, blocked
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
	return s.ready(f, worker, now)
}

// atLimit reports whether the cloud may start no more items (FLOW-05).
// Process items do not count toward the limit.
func (s *State) atLimit() bool {
	inProgress := 0
	for _, f := range s.Features {
		if f.Status == "in_progress" && !f.IsProcess() {
			inProgress++
		}
	}
	n := s.Config.Limits.InProgress
	return n > 0 && inProgress >= n
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

// ready returns the feature's first step the worker can take now, or why
// there is none. A step of a later phase is skipped without adding to the
// reasons: the current phase's steps already say why the feature waits.
func (s *State) ready(f *Feature, worker string, now time.Time) (*Step, string) {
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
		if !Satisfies(worker, st.Affinity) {
			waits = append(waits, fmt.Sprintf("%s step %s", st.Affinity, st.ID))
			continue
		}
		if st.Lease.Held(now) && st.Lease.ClaimedBy != worker {
			waits = append(waits, fmt.Sprintf("step %s leased to %s", st.ID,
				st.Lease.ClaimedBy))
			continue
		}
		if Kind(worker) == "cloud" {
			if why := s.cloudHold(f, st); why != "" {
				waits = append(waits, why)
				continue
			}
		}
		return st, ""
	}
	if len(waits) == 0 {
		return nil, "no pending step (phase done-when not yet advanced?)"
	}
	return nil, "waiting on " + strings.Join(waits, ", ")
}

// cloudHold returns why the unattended loop must not take the step yet.
func (s *State) cloudHold(f *Feature, st *Step) string {
	if q := f.OpenBlocking(); len(q) > 0 {
		return fmt.Sprintf("blocking question %s", q[0].ID)
	}
	if s.Runaway(f) {
		return fmt.Sprintf("runaway budget (%d of %d estimated steps spent; "+
			"escalate to human, FLOW-13)",
			f.Budget.Spent.Steps, f.Budget.Estimate.Steps)
	}
	if f.alters() && st.Phase >= 2 && f.Phase < 6 {
		if holder := s.altersHolder(f); holder != "" {
			return fmt.Sprintf("the alters_relationships slot, held by %s (FLOW-05)",
				holder)
		}
	}
	if base := retryRe.ReplaceAllString(st.ID, ""); base == StepID(f.Round, 6, "merge-main") {
		if holder := s.prSlotHolder(f); holder != "" {
			return fmt.Sprintf("the PR slot, held by %s (FLOW-05)", holder)
		}
	}
	return ""
}

// prSlotHolder returns another in-progress feature past the start of
// Publish, which holds the one open feature PR FLOW-05 allows.
func (s *State) prSlotHolder(f *Feature) string {
	for _, o := range s.Features {
		if o == f || o.IsProcess() || o.Status != "in_progress" || o.Phase < 6 {
			continue
		}
		if o.Phase > 6 || o.stepDone(6, "merge-main") {
			return o.Feature
		}
	}
	return ""
}

// stepDone reports whether a step of the given phase and template name is
// done, in any of its -r<n> forms.
func (f *Feature) stepDone(phase int, name string) bool {
	for _, st := range f.Steps {
		if st.Phase == phase && st.Status == "done" &&
			retryRe.ReplaceAllString(st.ID, "") == StepID(f.Round, phase, name) {
			return true
		}
	}
	return false
}

// altersHolder returns another in-progress feature that alters relationships
// and is past Design, which holds the one slot FLOW-05 allows.
func (s *State) altersHolder(f *Feature) string {
	for _, o := range s.Features {
		if o != f && o.Status == "in_progress" && o.alters() && o.Phase >= 2 {
			return o.Feature
		}
	}
	return ""
}

// NextFor is Next restricted to one feature, for an attended run on it: the
// in-progress limit counts every feature but process items. It does not wait for the item
// a fix follows: naming the item is the human's explicit word.
func (s *State) NextFor(name, worker string, now time.Time) (*Pick, []Blocked) {
	f, err := s.Find(name)
	if err != nil {
		return nil, []Blocked{{name, err.Error()}}
	}
	if f.Status == "queued" && Kind(worker) == "cloud" && s.atLimit() {
		return nil, []Blocked{{name, "the in-progress limit (FLOW-05)"}}
	}
	if h := s.Hold(f, worker); h != "" {
		if st := leasedTo(f, worker, now); st != nil {
			return &Pick{Feature: f, Step: st}, nil
		}
		return nil, []Blocked{{name, "held by process item " + h}}
	}
	st, why := s.ready(f, worker, now)
	if st == nil {
		return nil, []Blocked{{name, why}}
	}
	return &Pick{Feature: f, Step: st, Starts: f.Status == "queued" &&
		(Kind(worker) == "cloud" || f.IsProcess())}, nil
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
	if f.Status == "queued" && (Kind(worker) == "cloud" || f.IsProcess()) {
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

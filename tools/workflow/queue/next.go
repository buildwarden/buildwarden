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

// Blocked says why an in-progress feature has no step for the worker.
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

// leasedTo returns the feature's pending step the worker already holds a
// live lease on: a hold never takes away a step already claimed.
func leasedTo(f *Feature, worker string, now time.Time) *Step {
	for i := range f.Steps {
		st := &f.Steps[i]
		if st.Status == "pending" && st.Lease.Held(now) && st.Lease.ClaimedBy == worker {
			return st
		}
	}
	return nil
}

// Next picks the worker's next step in FLOW-05 order: process items first,
// then the highest-priority in-progress feature with a ready step; else, for
// the cloud, a new feature when every in-progress one is blocked and the
// in-progress limit allows, and for a human or host, a queued feature's step
// of its affinity. A worker an in-progress process item holds gets only the
// steps it already has leased. It also returns why each in-progress feature
// it passed over is blocked, for the digest.
func (s *State) Next(worker string, now time.Time) (*Pick, []Blocked) {
	var blocked []Blocked
	inProgress := 0
	for _, f := range s.Features {
		if f.Status != "in_progress" {
			continue
		}
		inProgress++
		if h := s.Hold(f, worker); h != "" {
			if st := leasedTo(f, worker, now); st != nil {
				return &Pick{Feature: f, Step: st}, blocked
			}
			blocked = append(blocked, Blocked{f.Feature, "held by process item " + h})
			continue
		}
		st, why := s.ready(f, worker, now)
		if st != nil {
			return &Pick{Feature: f, Step: st}, blocked
		}
		blocked = append(blocked, Blocked{f.Feature, why})
	}
	return s.nextQueued(worker, inProgress, now), blocked
}

// nextQueued is Next's fallback once no in-progress feature has a step: a
// queued feature's step, which only a cloud claim (or any claim on a process
// item) starts.
func (s *State) nextQueued(worker string, inProgress int, now time.Time) *Pick {
	if s.Hold(nil, worker) != "" {
		return nil
	}
	cloud := Kind(worker) == "cloud"
	if n := s.Config.Limits.InProgress; cloud && n > 0 && inProgress >= n {
		return nil
	}
	// A human or host may also do a queued feature's preparatory step
	// (pushing a plan, say); that does not start a feature.
	for _, f := range s.Features {
		if f.Status != "queued" {
			continue
		}
		if st, _ := s.ready(f, worker, now); st != nil {
			return &Pick{Feature: f, Step: st, Starts: cloud || f.IsProcess()}
		}
	}
	return nil
}

// ready returns the feature's first step the worker can take now, or why
// there is none.
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
		if st.Status != "pending" || !allDone(st.Needs, done) {
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
	if f.alters() && st.Phase >= 2 && f.Round == "parallel" {
		if holder := s.altersHolder(f); holder != "" {
			return fmt.Sprintf("the alters_relationships slot, held by %s (FLOW-05)",
				holder)
		}
	}
	if strings.HasSuffix(st.ID, "merge-round") {
		for _, o := range s.Features {
			if o != f && o.Status == "in_progress" && o.Round == "merge" {
				return fmt.Sprintf("the merge queue, behind %s", o.Feature)
			}
		}
	}
	return ""
}

// altersHolder returns another in-progress feature that alters relationships
// and is past Design, which holds the one slot FLOW-05 allows.
func (s *State) altersHolder(f *Feature) string {
	for _, o := range s.Features {
		if o != f && o.Status == "in_progress" && o.alters() && (o.Phase >= 2 ||
			o.Round == "merge") {
			return o.Feature
		}
	}
	return ""
}

// NextFor is Next restricted to one feature, for an attended run on it: the
// in-progress limit still counts every feature.
func (s *State) NextFor(name, worker string, now time.Time) (*Pick, []Blocked) {
	f, err := s.Find(name)
	if err != nil {
		return nil, []Blocked{{name, err.Error()}}
	}
	if f.Status == "queued" && Kind(worker) == "cloud" {
		inProgress := 0
		for _, o := range s.Features {
			if o.Status == "in_progress" {
				inProgress++
			}
		}
		if n := s.Config.Limits.InProgress; n > 0 && inProgress >= n {
			return nil, []Blocked{{name, "the in-progress limit (FLOW-05)"}}
		}
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

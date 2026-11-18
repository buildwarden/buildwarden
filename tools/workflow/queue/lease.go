package queue

import (
	"fmt"
	"strings"
	"time"
)

// Renew sets the expiry of a lease the worker holds to now plus its lease
// length, and logs it (FLOW-04). It counts nothing against the item's budget.
// It refuses a step that is not pending, and one whose live lease is not the
// worker's: an expired lease is claimed again, not renewed.
func (s *State) Renew(f *Feature, st *Step, worker string, now time.Time) error {
	if st.Status != "pending" {
		return fmt.Errorf("%s is %s", st.ID, st.Status)
	}
	if !st.Lease.Held(now) || st.Lease.ClaimedBy != worker {
		held := "no live lease"
		if st.Lease.Held(now) {
			held = "a lease of " + st.Lease.ClaimedBy
		}
		return fmt.Errorf("%s is not leased to %s: it has %s", st.ID, worker, held)
	}
	exp := now.Add(s.leaseLength(worker)).UTC()
	st.Lease.Expires = &exp
	f.Log(now, worker, "renewed %s until %s", st.ID, exp.Format(time.RFC3339))
	return nil
}

// Session is the worker's /<session> suffix, or "".
func Session(worker string) string {
	_, session, _ := strings.Cut(worker, "/")
	return session
}

// Released is one lease releaseLeases dropped.
type Released struct {
	Feature   *Feature
	Step      string
	ClaimedBy string
}

// releaseLeases releases every pending step whose live lease is held by a
// worker that match accepts, with a history line each by the worker that by
// names for the claimant, and returns what it released.
func (s *State) releaseLeases(match func(claimant string) bool,
	by func(claimant string) string, why string, now time.Time) []Released {
	var out []Released
	for _, f := range s.Features {
		for i := range f.Steps {
			st := &f.Steps[i]
			claimant := st.Lease.ClaimedBy
			if st.Status != "pending" || !st.Lease.Held(now) || !match(claimant) {
				continue
			}
			s.Release(f, st, by(claimant), now, why)
			out = append(out, Released{f, st.ID, claimant})
		}
	}
	return out
}

// ReleaseSession releases every live lease of the session <machine>.<run>,
// as the machine's wrapper does when the session ends, and an attended
// session at its Close (FLOW-04).
func (s *State) ReleaseSession(session, why string, now time.Time) []Released {
	return s.releaseLeases(func(c string) bool { return Session(c) == session },
		func(c string) string { return c }, why, now)
}

// ReleaseStale releases every live lease of a host worker whose session names
// the machine, <machine>.<run>, other than current, the wrapper's own run:
// what an earlier scheduled session of the machine left (FLOW-04). A session
// whose run starts attended- is skipped, so a scheduled run never frees the
// step of a live attended session. The history lines are by the claimant's
// kind with the current session.
func (s *State) ReleaseStale(machine, current, why string, now time.Time) []Released {
	match := func(c string) bool {
		run, ok := strings.CutPrefix(Session(c), machine+".")
		return ok && run != "" && Session(c) != current &&
			strings.HasPrefix(Kind(c), "host:") && !strings.HasPrefix(run, "attended-")
	}
	return s.releaseLeases(match, func(c string) string { return Kind(c) + "/" + current },
		why, now)
}

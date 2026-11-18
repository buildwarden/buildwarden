package queue

import (
	"strings"
	"testing"
	"time"
)

// TestLeaseLength: a host worker's claim gets host_lease_minutes, 1200 when
// unset, and a cloud or human claim lease_minutes, 120 when unset (FLOW-04).
func TestLeaseLength(t *testing.T) {
	for _, c := range []struct {
		worker      string
		lease, host int
		want        time.Duration
	}{
		{"cloud/run", 0, 0, 120 * time.Minute},
		{"human/jeff", 90, 0, 90 * time.Minute},
		{"host:macos-vz/macbook.r1", 90, 0, 1200 * time.Minute},
		{"host:macos-vz/macbook.r1", 0, 300, 300 * time.Minute},
		{"cloud/run", 0, 300, 120 * time.Minute},
	} {
		f := feature("a", 1, "in_progress")
		f.Steps[0].Affinity = Kind(c.worker)
		s := state(f)
		s.Config.LeaseMinutes, s.Config.HostLeaseMinutes = c.lease, c.host
		s.Claim(f, &f.Steps[0], c.worker, now, false)
		if got := f.Steps[0].Lease.Expires.Sub(now); got != c.want {
			t.Errorf("%s with lease %d, host %d: %v, want %v", c.worker, c.lease,
				c.host, got, c.want)
		}
	}
	s := state()
	s.Config.HostLeaseMinutes = -1
	wantValidate(t, s, "config: host_lease_minutes -1 is below 0")
}

// TestRenew: a worker renews only a live lease of its own, for its lease
// length from now, and the renewal counts nothing (FLOW-04, FLOW-13).
func TestRenew(t *testing.T) {
	const w = "host:macos-vz/macbook.r1"
	f := feature("a", 1, "in_progress")
	f.Steps[0].Affinity = "host:macos-vz"
	s := state(f)
	st := &f.Steps[0]
	s.Claim(f, st, w, now, true)
	spent := f.Budget.Spent
	later := now.Add(10 * time.Hour)
	if err := s.Renew(f, st, w, later); err != nil {
		t.Fatal(err)
	}
	if got := st.Lease.Expires.Sub(later); got != 1200*time.Minute ||
		f.Budget.Spent != spent {
		t.Fatalf("renewed for %v, spent %+v (was %+v)", got, f.Budget.Spent, spent)
	}
	if e := lastLine(f); !strings.HasPrefix(e, "renewed f0-branch-setup until ") {
		t.Fatalf("history %q", e)
	}
	for _, c := range []struct {
		worker string
		at     time.Time
		want   string
	}{
		{"host:macos-vz/macbook.r2", later, "it has a lease of " + w},
		{w, later.Add(21 * time.Hour), "it has no live lease"},
	} {
		if err := s.Renew(f, st, c.worker, c.at); err == nil ||
			!strings.Contains(err.Error(), c.want) {
			t.Fatalf("Renew by %s: %v, want %q", c.worker, err, c.want)
		}
	}
	Complete(f, st, w, later)
	if err := s.Renew(f, st, w, later); err == nil {
		t.Fatal("a done step was renewed")
	}
}

// TestReleaseStale: the wrapper's release before the gate frees the live
// leases an earlier scheduled session of the machine left, spares the
// current run, an attended session and other machines, and the gate then
// offers the step; release -session frees one session's leases (FLOW-04).
func TestReleaseStale(t *testing.T) {
	repo, _ := gitRepo(t)
	mk := func(name string, prio int, by string) *Feature {
		f := feature(name, prio, "in_progress")
		f.Steps[0].Affinity = "host:macos-vz"
		exp := now.Add(time.Hour)
		f.Steps[0].Lease = Lease{ClaimedBy: by, Expires: &exp}
		return f
	}
	old := mk("old", 1, "host:macos-vz/mac.r1")
	cur := mk("cur", 2, "host:macos-vz/mac.r2")
	att := mk("att", 3, "host:macos-vz/mac.attended-20261118T100000Z")
	other := mk("other", 4, "host:macos-vz/win.r1")
	s := nativeState(old, cur, att, other)
	mac, _ := s.Machine("mac")
	g, _ := s.Gate(mac, repo, nil, 7, now)
	if g.Run {
		t.Fatalf("the gate ran on leased steps: %+v", g)
	}
	rs := s.ReleaseStale("mac", "mac.r2", "an earlier session on mac left it", now)
	if len(rs) != 1 || rs[0].Feature != old || rs[0].ClaimedBy != "host:macos-vz/mac.r1" {
		t.Fatalf("released %+v, want old's step alone", rs)
	}
	e := old.History[len(old.History)-1]
	if e.By != "host:macos-vz/mac.r2" ||
		e.Event != "released f0-branch-setup: an earlier session on mac left it" {
		t.Fatalf("history %+v", e)
	}
	g, _ = s.Gate(mac, repo, nil, 7, now)
	if !g.Run || !strings.Contains(g.Reasons[0], "step\thost:macos-vz\told\tf0-branch-setup") {
		t.Fatalf("after the release the gate = %+v, want old's step", g)
	}
	rs = s.ReleaseSession("mac.attended-20261118T100000Z", "attended session closed", now)
	if len(rs) != 1 || rs[0].Feature != att || att.Steps[0].Lease.ClaimedBy != "" {
		t.Fatalf("release -session released %+v", rs)
	}
	if cur.Steps[0].Lease.ClaimedBy == "" || other.Steps[0].Lease.ClaimedBy == "" {
		t.Fatal("a release freed the current run's or another machine's lease")
	}
}

package queue

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitRepo makes a scratch repository whose origin/main and origin/feat/a refs
// exist, and returns it with a function that moves origin/main.
func gitRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			// Fixtures hold the bytes written whatever the machine's core.autocrlf.
			"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.autocrlf",
			"GIT_CONFIG_VALUE_0=false")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "one")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("update-ref", "refs/remotes/origin/feat/a", "HEAD")
	return dir, func() {
		git("commit", "-q", "--allow-empty", "-m", "two")
		git("update-ref", "refs/remotes/origin/main", "HEAD")
	}
}

// nativeState is a state with two machines whose priority order is
// confirmed, so the gate decides on the work.
func nativeState(fs ...*Feature) *State {
	s := state(fs...)
	s.Config.PriorityConfirmed = true
	s.Machines = []Machine{
		{Name: "mac", Hosts: []string{"macos-vz", "macos-qemu"}},
		{Name: "win", Hosts: []string{"windows-hyperv", "qemu-any"}},
	}
	return s
}

func TestGateRunsForAReadyHostStep(t *testing.T) {
	repo, _ := gitRepo(t)
	a := feature("a", 1, "in_progress")
	a.Branch = "feat/a"
	a.Steps[0].Affinity = "host:macos-qemu"
	s := nativeState(a)
	mac, _ := s.Machine("mac")
	g, err := s.Gate(mac, repo, nil, 7, now)
	want := "host:macos-qemu\ta\tf0-branch-setup"
	if err != nil || !g.Run || !strings.Contains(g.Reasons[0], want) {
		t.Fatalf("mac gate = %+v, %v; want the macos-qemu step", g, err)
	}
	win, _ := s.Machine("win")
	if g, _ := s.Gate(win, repo, nil, 7, now); g.Run {
		t.Fatalf("win gate ran with no step or gap of its own: %+v", g)
	}
	// A step another worker holds is not ready.
	s.Claim(a, &a.Steps[0], "host:macos-qemu/attended", now)
	if g, _ := s.Gate(mac, repo, nil, 7, now); g.Run {
		t.Fatalf("gate ran on a leased step: %+v", g)
	}
}

// The 2026-11-01 case: a Publish validation written in Design must not wake
// its machine while the feature is still in Plan.
func TestGateWaitsForTheStepsPhase(t *testing.T) {
	repo, _ := gitRepo(t)
	a := feature("a", 1, "in_progress")
	a.Branch, a.Phase = "feat/a", 2
	a.Steps = []Step{{ID: "f2-plan", Phase: 2, Affinity: "cloud", Status: "pending"},
		{ID: "f6-validate-windows-hyperv", Phase: 6, Affinity: "host:windows-hyperv",
			Status: "pending"}}
	s := nativeState(a)
	win, _ := s.Machine("win")
	if g, _ := s.Gate(win, repo, nil, 7, now); g.Run {
		t.Fatalf("gate woke win for a later phase's step: %+v", g)
	}
	a.Phase, a.Steps[0].Status = 6, "done"
	g, err := s.Gate(win, repo, nil, 7, now)
	if err != nil || !g.Run || !strings.Contains(g.Reasons[0], "f6-validate-windows-hyperv") {
		t.Fatalf("win gate in Publish = %+v, %v; want the validation", g, err)
	}
}

func TestGateSweepsGapsOnlyWhenSomethingMoved(t *testing.T) {
	repo, moveMain := gitRepo(t)
	a := feature("a", 1, "in_progress")
	a.Branch = "feat/a"
	a.Steps[0].Affinity = "human"
	s := nativeState(a)
	s.Gaps = []Gap{
		{ID: "g1", Host: "qemu-any", Status: "open"},
		{ID: "g2", Host: "macos-vz", Status: "done"},
	}
	win, _ := s.Machine("win")
	g, err := s.Gate(win, repo, nil, 7, now)
	if err != nil || !g.Run || !strings.Contains(g.Reasons[0], "first sweep\tg1") {
		t.Fatalf("first gate = %+v, %v; want a first sweep of g1", g, err)
	}
	stamp := &GateStamp{Print: g.Print, At: now}
	if g, _ := s.Gate(win, repo, stamp, 7, now.Add(time.Hour)); g.Run {
		t.Fatalf("gate ran with nothing moved: %+v", g)
	}
	if g, _ := s.Gate(win, repo, stamp, 7, now.Add(8*24*time.Hour)); !g.Run {
		t.Fatalf("gate did not re-sweep after 7 days: %+v", g)
	}
	moveMain()
	if g, _ := s.Gate(win, repo, stamp, 7, now.Add(time.Hour)); !g.Run ||
		!strings.Contains(g.Reasons[0], "moved") {
		t.Fatalf("gate did not run after main moved: %+v", g)
	}
	// The mac's only gap is done: idle whatever moved.
	mac, _ := s.Machine("mac")
	if g, _ := s.Gate(mac, repo, nil, 7, now); g.Run {
		t.Fatalf("mac gate ran on a done gap: %+v", g)
	}
}

// TestFingerprintProgramTrunk: a program trunk's move changes the
// fingerprint of a machine that lists its repository, and the gate sweeps
// on it; one of a repository the machine does not list changes nothing
// (FLOW-09, FLOW-20).
func TestFingerprintProgramTrunk(t *testing.T) {
	repo, _ := gitRepo(t)
	clone := filepath.Join(repo, ".dev", "checkouts", "o", "relay")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatal(err)
	}
	git(clone, "init", "-q")
	git(clone, "commit", "-q", "--allow-empty", "-m", "one")
	git(clone, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(clone, "update-ref", "refs/remotes/origin/program/p", "HEAD")
	s := own(nativeState(), "o/here")
	for i := range s.Config.Repositories {
		if n := s.Config.Repositories[i].Name; n == "o/here" || n == "o/relay" {
			s.Config.Repositories[i].ProgramTrunk = "program/p"
		}
	}
	s.Gaps = []Gap{{ID: "g1", Host: "macos-vz", Status: "open"}}
	mac, _ := s.Machine("mac")
	before, err := s.fingerprint(mac, repo)
	if err != nil {
		t.Fatal(err)
	}
	stamp := &GateStamp{Print: before, At: now}
	// The program trunk absent, then present: it moved.
	git(repo, "update-ref", "refs/remotes/origin/program/p", "HEAD")
	after, _ := s.fingerprint(mac, repo)
	if after == before {
		t.Fatal("the fingerprint did not change when the program trunk moved")
	}
	g, err := s.Gate(mac, repo, stamp, 7, now.Add(time.Hour))
	if err != nil || !g.Run ||
		!strings.Contains(g.Reasons[0], "a trunk, a program trunk or a waiting branch moved") {
		t.Fatalf("gate after a program-trunk move = %+v, %v; want a sweep", g, err)
	}
	// o/relay is not listed: its program trunk's move changes nothing.
	git(clone, "commit", "-q", "--allow-empty", "-m", "two")
	git(clone, "update-ref", "refs/remotes/origin/program/p", "HEAD")
	if again, _ := s.fingerprint(mac, repo); again != after {
		t.Error("the fingerprint changed when an unlisted repository's program trunk moved")
	}
	// Once listed, it counts.
	mac.Repositories = []string{"o/here", "o/relay"}
	listed, _ := s.fingerprint(mac, repo)
	git(clone, "commit", "-q", "--allow-empty", "-m", "three")
	git(clone, "update-ref", "refs/remotes/origin/program/p", "HEAD")
	if moved, _ := s.fingerprint(mac, repo); moved == listed {
		t.Error("the fingerprint did not change when a listed repository's program trunk moved")
	}
}

func TestStampRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "stamp.json")
	if st, err := ReadStamp(path); st != nil || err != nil {
		t.Fatalf("missing stamp = %v, %v", st, err)
	}
	if err := WriteStamp(path, GateStamp{Print: "abc", At: now}); err != nil {
		t.Fatal(err)
	}
	st, err := ReadStamp(path)
	if err != nil || st.Print != "abc" || !st.At.Equal(now) {
		t.Fatalf("stamp = %+v, %v", st, err)
	}
}

// TestValidateNative: two machines may serve one host name; a pending step,
// an open gap or an open item's lead that no machine serves fails (FLOW-04).
func TestValidateNative(t *testing.T) {
	a := feature("a", 1, "in_progress")
	a.Steps[0].Affinity = "host:windows-container"
	led := feature("led", 2, "queued")
	led.Lead = "host:linux-any"
	s := nativeState(a, led)
	s.Machines = append(s.Machines, Machine{Name: "other", Hosts: []string{"qemu-any"}})
	s.Gaps = []Gap{{ID: "g1", Host: "macos-finch", Status: "open"},
		{ID: "g2", Host: "macos-finch", Status: "done"}}
	var got []string
	for _, e := range s.Validate() {
		got = append(got, e.Error())
	}
	want := []string{
		"step f0-branch-setup: no machine serves host windows-container",
		"led: lead: no machine serves host linux-any",
		"gap g1: no machine serves host macos-finch",
	}
	if len(got) != len(want) {
		t.Fatalf("validate = %q, want %d problems", got, len(want))
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			t.Fatalf("validate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Without hosts.yaml no host is checked, as before.
	s.Machines = nil
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("validate without hosts.yaml = %v", errs)
	}
}

// TestValidateGapStatus: a gap is open, deferred or done, and a deferred one
// says what it waits on (FLOW-09, TEST-03), with or without hosts.yaml.
func TestValidateGapStatus(t *testing.T) {
	for _, machines := range [][]Machine{nil, nativeState().Machines} {
		s := state()
		s.Machines = machines
		s.Gaps = []Gap{
			{ID: "g1", Host: "macos-vz", Status: "open"},
			{ID: "g2", Host: "macos-vz", Status: "deferred",
				Note: "2026-11-03: waits on a firewalld host from Jeff."},
			{ID: "g3", Host: "macos-vz", Status: "done"},
			{ID: "g4", Host: "macos-vz", Status: "skipped"},
			{ID: "g5", Host: "macos-vz", Status: "deferred", Note: " "},
		}
		var got []string
		for _, e := range s.Validate() {
			got = append(got, e.Error())
		}
		want := []string{`gap g4: status "skipped" is not open, deferred or done`,
			"gap g5: deferred needs a note"}
		if len(got) != len(want) {
			t.Fatalf("machines %v: validate = %q, want %d problems", machines, got,
				len(want))
		}
		for i := range want {
			if !strings.Contains(got[i], want[i]) {
				t.Errorf("validate[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	}
}

// TestGateSkipsDeferredGaps: a deferred gap wakes no native session.
func TestGateSkipsDeferredGaps(t *testing.T) {
	repo, _ := gitRepo(t)
	s := nativeState()
	s.Gaps = []Gap{{ID: "g1", Host: "qemu-any", Status: "deferred", Note: "waits"}}
	win, _ := s.Machine("win")
	if g, err := s.Gate(win, repo, nil, 7, now); err != nil || g.Run {
		t.Fatalf("gate on a deferred gap = %+v, %v; want idle", g, err)
	}
}

func TestLoadReadsHostsAndGaps(t *testing.T) {
	dir := t.TempDir()
	must := func(p, body string) {
		t.Helper()
		_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("config.yaml", "priority_confirmed: true\n")
	must("hosts.yaml", "machines:\n  - name: mac\n    hosts: [macos-vz]\n")
	must("backlog/native-gaps.yaml", "gaps:\n  - {id: g1, host: macos-vz, status: open}\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Machines) != 1 || s.Machines[0].Hosts[0] != "macos-vz" ||
		len(s.Gaps) != 1 || s.Gaps[0].ID != "g1" {
		t.Fatalf("loaded %+v %+v", s.Machines, s.Gaps)
	}
}

func TestValidateMachineStatus(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "native", "status"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("run_budget: {steps: 8}\n"),
		0o644)
	_ = os.WriteFile(filepath.Join(dir, "hosts.yaml"),
		[]byte("machines:\n  - {name: mac, hosts: [macos-vz]}\n"), 0o644)
	write := func(name, body string) {
		_ = os.WriteFile(filepath.Join(dir, "native", "status", name), []byte(body), 0o644)
	}
	write("mac.yaml", "machine: mac\nlast_run: 2026-10-29T20:13:35Z\noutcome: failed\n"+
		"reason: \"claude exited 1\"\nsince: 2026-10-29T04:40:00Z\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := s.Validate(); len(errs) != 0 || len(s.Statuses) != 1 {
		t.Fatalf("Validate = %v, statuses %d; want a clean single status", errs,
			len(s.Statuses))
	}
	write("pc.yaml", "machine: other\nlast_run: 2026-10-29T04:00:00Z\noutcome: broken\n"+
		"reason: \"\"\nsince: 2026-10-30T00:00:00Z\n")
	s, _ = Load(dir)
	got := ""
	for _, e := range s.Validate() {
		got += e.Error() + "\n"
	}
	for _, want := range []string{"does not match the file name", "not in hosts.yaml",
		`outcome "broken"`, "one line", "since not after last_run"} {
		if !strings.Contains(got, want) {
			t.Errorf("Validate = %q, want %q", got, want)
		}
	}
}

// TestGatePriorityUnconfirmed: while the human has not confirmed the
// priority order the gate is idle with that reason, whatever is ready
// (FLOW-13).
func TestGatePriorityUnconfirmed(t *testing.T) {
	repo, _ := gitRepo(t)
	a := feature("a", 1, "in_progress")
	a.Steps[0].Affinity = "host:macos-vz"
	s := nativeState(a)
	s.Config.PriorityConfirmed = false
	mac, _ := s.Machine("mac")
	g, err := s.Gate(mac, repo, nil, 7, now)
	if err != nil || g.Run || len(g.Reasons) != 1 ||
		g.Reasons[0] != "idle\tthe priority order is not confirmed (FLOW-13)" {
		t.Fatalf("gate = %+v, %v; want idle on the unconfirmed order", g, err)
	}
}

// TestGateSharedHost: two machines that serve one host name both see its
// ready step; the first claim pushed wins (FLOW-04).
func TestGateSharedHost(t *testing.T) {
	repo, _ := gitRepo(t)
	a := feature("a", 1, "in_progress")
	a.Branch = "feat/a"
	a.Steps[0].Affinity = "host:qemu-any"
	s := nativeState(a)
	s.Machines[0].Hosts = append(s.Machines[0].Hosts, "qemu-any")
	wantValidate(t, s)
	for _, name := range []string{"mac", "win"} {
		m, _ := s.Machine(name)
		g, err := s.Gate(m, repo, nil, 7, now)
		if err != nil || !g.Run || !strings.Contains(g.Reasons[0], "a\tf0-branch-setup") {
			t.Fatalf("%s gate = %+v, %v; want the qemu-any step", name, g, err)
		}
	}
}

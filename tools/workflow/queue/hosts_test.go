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
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
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

func nativeState(fs ...*Feature) *State {
	s := state(fs...)
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
	want := "host:macos-qemu\ta\tp0-branch-setup"
	if err != nil || !g.Run || !strings.Contains(g.Reasons[0], want) {
		t.Fatalf("mac gate = %+v, %v; want the macos-qemu step", g, err)
	}
	win, _ := s.Machine("win")
	if g, _ := s.Gate(win, repo, nil, 7, now); g.Run {
		t.Fatalf("win gate ran with no step or gap of its own: %+v", g)
	}
	// A step another worker holds is not ready.
	s.Claim(a, &a.Steps[0], "host:macos-qemu/attended", now, true)
	if g, _ := s.Gate(mac, repo, nil, 7, now); g.Run {
		t.Fatalf("gate ran on a leased step: %+v", g)
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

func TestValidateNative(t *testing.T) {
	a := feature("a", 1, "in_progress")
	a.Steps[0].Affinity = "host:windows-container"
	s := nativeState(a)
	s.Machines = append(s.Machines, Machine{Name: "other", Hosts: []string{"qemu-any"}})
	s.Gaps = []Gap{{ID: "g1", Host: "macos-finch", Status: "open"},
		{ID: "g2", Host: "macos-finch", Status: "done"}}
	var got []string
	for _, e := range s.Validate() {
		got = append(got, e.Error())
	}
	want := []string{
		"host qemu-any served by both win and other",
		"step p0-branch-setup: no machine serves host windows-container",
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
	// Without hosts.yaml nothing is checked, as before.
	s.Machines = nil
	if errs := s.Validate(); len(errs) != 0 {
		t.Fatalf("validate without hosts.yaml = %v", errs)
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

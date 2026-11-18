package queue

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// order is the state's open items in FLOW-05 order, each with its number.
func order(s *State) string {
	var out []string
	for _, f := range s.Listed(false) {
		out = append(out, f.Feature+":"+strconv.Itoa(f.Priority))
	}
	return strings.Join(out, " ")
}

// wantPositions renumbers the state and checks that the order did not change
// and that the open items are numbered 1..N in it.
func wantPositions(t *testing.T, s *State, when string) {
	t.Helper()
	var before []string
	for _, f := range s.Listed(false) {
		before = append(before, f.Feature)
	}
	s.Renumber()
	for i, f := range s.Listed(false) {
		if f.Feature != before[i] {
			t.Fatalf("%s: Renumber changed the order: %v, now %s", when, before, order(s))
		}
		if f.Priority != i+1 {
			t.Fatalf("%s: %s is numbered %d at position %d: %s", when, f.Feature,
				f.Priority, i+1, order(s))
		}
	}
}

// added queues the item through Add at the asked position.
func added(t *testing.T, s *State, f *Feature, asked int, at time.Time) *Feature {
	t.Helper()
	f.Priority = asked
	if f.Budget.Estimate.Steps == 0 {
		f.Budget.Estimate = Count{Runs: 1, Steps: 7}
	}
	if err := s.Add(f, at, "human/jeff"); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestRenumberFollowsOrder: after each event that moves an item in FLOW-05's
// order the open items are numbered by their positions, and renumbering never
// changes the order.
func TestRenumberFollowsOrder(t *testing.T) {
	s := state()
	a := added(t, s, &Feature{Feature: "a"}, 1, now)
	b := added(t, s, &Feature{Feature: "b"}, 2, now)
	p := added(t, s, &Feature{Feature: "p", Kind: "process"}, 3, now)
	added(t, s, &Feature{Feature: "parent"}, 4, now)
	wantPositions(t, s, "queued")
	if got := order(s); got != "p:1 a:2 b:3 parent:4" {
		t.Fatalf("after Add: %s, want the process item first", got)
	}
	s.Claim(b, &b.Steps[0], "cloud/run", now, true)
	wantPositions(t, s, "b started")
	if got := order(s); got != "p:1 b:2 a:3 parent:4" {
		t.Fatalf("after b's start: %s", got)
	}
	s.Claim(p, &p.Steps[0], "human/jeff", now, false)
	advance(p, 2)
	wantPositions(t, s, "p in Implement")
	s.Claim(a, &a.Steps[0], "cloud/run", now, false)
	advance(a, 7)
	advance(b, 7)
	wantPositions(t, s, "a and b waiting on the merge")
	for _, f := range []*Feature{a, b} {
		for i := range f.Steps {
			if st := &f.Steps[i]; st.Status == "pending" && st.Phase == 7 {
				Complete(f, st, "cloud/run", now)
			}
		}
	}
	wantPositions(t, s, "merges confirmed into Close")
	if a.Class() != 1 || order(s) != "p:1 b:2 a:3 parent:4" {
		t.Fatalf("in Close: %s, class %d", order(s), a.Class())
	}
	Back(b, "cloud/run", now, "a finding", 3)
	wantPositions(t, s, "b back to Implement")
	walk(t, a)
	wantPositions(t, s, "a closed")
	if got := order(s); got != "p:1 b:2 parent:3" {
		t.Fatalf("after a closed: %s", got)
	}
	fix := added(t, s, &Feature{Feature: "fix", Follows: "parent"}, 1, now)
	wantPositions(t, s, "a fix queued")
	if fix.Priority != 3 {
		t.Fatalf("the fix is at %d, want 3, after the class-1 items: %s", fix.Priority,
			order(s))
	}
}

// placement returns the item's placement history lines.
func placement(f *Feature) []string {
	var out []string
	for _, e := range f.History {
		if strings.Contains(e.Event, "position") || strings.Contains(e.Event, "priority") {
			out = append(out, e.Event)
		}
	}
	return out
}

// placeState is impl (class 1), p (a queued process item), z (an in-progress
// class-4 feature) and the queued a, b and c.
func placeState(t *testing.T) *State {
	t.Helper()
	s := state()
	impl := added(t, s, &Feature{Feature: "impl"}, 1, now)
	s.Claim(impl, &impl.Steps[0], "cloud/run", now, false)
	advance(impl, 3)
	added(t, s, &Feature{Feature: "p", Kind: "process"}, 2, now)
	z := added(t, s, &Feature{Feature: "z"}, 3, now)
	s.Claim(z, &z.Steps[0], "cloud/run", now, false)
	for _, n := range []string{"a", "b", "c"} {
		added(t, s, &Feature{Feature: n}, 9, now)
	}
	if got := order(s); got != "impl:1 p:2 z:3 a:4 b:5 c:6" {
		t.Fatalf("placeState: %s", got)
	}
	return s
}

// TestPlace: wf new places an item at the asked position when its class and
// status allow it, and otherwise as near as they do, saying what was asked; a
// queued item never goes ahead of an in-progress item of its class. Only the
// placed item gets a history line.
func TestPlace(t *testing.T) {
	for _, c := range []struct {
		asked int
		want  string
		line  string
	}{
		{5, "impl:1 p:2 z:3 a:4 n:5 b:6 c:7", "queued at position 5"},
		{4, "impl:1 p:2 z:3 n:4 a:5 b:6 c:7", "queued at position 4"},
		{1, "impl:1 p:2 z:3 n:4 a:5 b:6 c:7", "queued at position 4 (asked 1)"},
		{7, "impl:1 p:2 z:3 a:4 b:5 c:6 n:7", "queued at position 7"},
		{99, "impl:1 p:2 z:3 a:4 b:5 c:6 n:7", "queued at position 7 (asked 99)"},
	} {
		s := placeState(t)
		lines := map[string]int{}
		for _, f := range s.Features {
			lines[f.Feature] = len(f.History)
		}
		n := added(t, s, &Feature{Feature: "n"}, c.asked, now)
		if got := order(s); got != c.want {
			t.Errorf("asked %d: %s, want %s", c.asked, got, c.want)
		}
		if got := placement(n); len(got) != 1 || got[0] != c.line {
			t.Errorf("asked %d: n's lines %q, want %q", c.asked, got, c.line)
		}
		for _, f := range s.Features {
			if f != n && len(f.History) != lines[f.Feature] {
				t.Errorf("asked %d: %s got a history line: %v", c.asked, f.Feature,
					f.History[len(f.History)-1].Event)
			}
		}
	}
	// A process item goes ahead of every queued feature, wherever it is asked.
	s := placeState(t)
	q := added(t, s, &Feature{Feature: "q", Kind: "process"}, 9, now)
	if got := order(s); got != "impl:1 p:2 q:3 z:4 a:5 b:6 c:7" ||
		placement(q)[0] != "queued at position 3 (asked 9)" {
		t.Fatalf("a queued process item: %s, %v", got, placement(q))
	}
}

// TestMove: wf move is a human's, refused for a done item and a position
// below 1, places as wf new does and logs one line on the moved item.
func TestMove(t *testing.T) {
	s := placeState(t)
	c, _ := s.Find("c")
	for _, w := range []string{"cloud/run", "host:macbook/s", "human"} {
		if err := s.Move(c, 4, "", w, now); err == nil ||
			!strings.Contains(err.Error(), "LOOP-04") {
			t.Errorf("%s: Move = %v, want a refusal naming LOOP-04", w, err)
		}
	}
	if err := s.Move(c, 0, "", "human/jeff", now); err == nil {
		t.Error("Move to 0 was not refused")
	}
	if err := s.Move(c, 4, "his reason", "human/jeff", now); err != nil {
		t.Fatal(err)
	}
	if got := order(s); got != "impl:1 p:2 z:3 c:4 a:5 b:6" {
		t.Fatalf("after the move: %s", got)
	}
	if got := c.History[len(c.History)-1].Event; got !=
		"moved to position 4 from 6 (asked 4): his reason" {
		t.Fatalf("the line is %q", got)
	}
	if err := s.Move(c, 1, "", "human/jeff", now); err != nil {
		t.Fatal(err)
	}
	if got := c.History[len(c.History)-1].Event; got !=
		"moved to position 4 from 4 (asked 1)" {
		t.Fatalf("the clamped line is %q", got)
	}
	impl, _ := s.Find("impl")
	walk(t, impl)
	if err := s.Move(impl, 1, "", "human/jeff", now); err == nil ||
		!strings.Contains(err.Error(), "done") {
		t.Fatalf("moving a done item: %v", err)
	}
}

// TestTieBreak: equal numbers, which only a hand edit, an old wf or a replay
// leaves, order by the later placement, then by name.
func TestTieBreak(t *testing.T) {
	s := state()
	b := added(t, s, &Feature{Feature: "b"}, 1, now)
	a := added(t, s, &Feature{Feature: "a"}, 2, now.Add(time.Minute))
	c := added(t, s, &Feature{Feature: "c"}, 3, now)
	a.Priority, b.Priority, c.Priority = 1, 1, 1
	if got := order(s); got != "a:1 b:1 c:1" {
		t.Fatalf("ties: %s, want a (placed later), then b and c by name", got)
	}
	legacy := &Feature{Feature: "d", Priority: 1, Status: "queued", Round: "feature"}
	legacy.Log(now.Add(2*time.Minute), "human/jeff", "queued at priority 1 (intake)")
	s.Features = append(s.Features, legacy)
	if got := order(s); got != "d:1 a:1 b:1 c:1" {
		t.Fatalf("a legacy placement line: %s", got)
	}
}

// doneState is a state checkout in a temporary directory with the items
// queued through Add and saved.
func doneState(t *testing.T, names ...string) *State {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "queue"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("run_budget: {steps: 8}\nlimits: {in_progress: 5}\n"), 0o644)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range names {
		f := added(t, s, &Feature{Feature: n}, i+1, now)
		if err := s.Save(f); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// closeItem walks the item to done at the time and saves it.
func closeItem(t *testing.T, s *State, name string, at time.Time) *Feature {
	t.Helper()
	f, err := s.Find(name)
	if err != nil {
		t.Fatal(err)
	}
	for f.Status != "done" {
		for i := range f.Steps {
			if f.Steps[i].Status == "pending" {
				if f.Status == "queued" {
					s.Claim(f, &f.Steps[i], "cloud/run", at, false)
				}
				Complete(f, &f.Steps[i], "cloud/run", at)
				break
			}
		}
	}
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
	return f
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// TestDoneFolder: Load reads queue/ and queue/done/, refuses an item in
// both, Save moves a closing item and removes its old file, a done parent in
// queue/done/ validates and releases its fix, and Listed(true) lists done
// items in the order they closed.
func TestDoneFolder(t *testing.T) {
	s := doneState(t, "a", "b", "c", "fix")
	fix, _ := s.Find("fix")
	fix.Follows = "a"
	if err := s.Save(fix); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(s.Dir, "queue", "done"), 0o755)
	s, err := Load(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	closeItem(t, s, "b", now.Add(time.Hour))
	closeItem(t, s, "a", now.Add(2*time.Hour))
	if exists(s.Dir, "queue/a.yaml") || !exists(s.Dir, "queue/done/a.yaml") {
		t.Fatal("Save did not move the closed a to queue/done/")
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "queue", "done", "a.yaml"))
	if strings.Contains(string(data), "priority:") {
		t.Fatalf("a done item carries a priority:\n%s", data)
	}
	got, err := Load(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := got.Validate(); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
	if order(got) != "fix:1 c:2" {
		t.Fatalf("the open items: %s", order(got))
	}
	if p, _ := got.Next("cloud", now); p == nil || p.Feature.Feature != "fix" {
		t.Fatalf("next = %+v, want the fix of the done a started", p)
	}
	legacy := &Feature{Feature: "old", Status: "done", Round: "feature"}
	legacy.Log(now.Add(90*time.Minute), "human/jeff", "closed by hand")
	got.Features = append(got.Features, legacy)
	var names []string
	for _, f := range got.Listed(true) {
		names = append(names, f.Feature)
	}
	if strings.Join(names, " ") != "fix c b old a" {
		t.Fatalf("Listed(true) = %v, want the done items in the order they closed", names)
	}

	if err := os.WriteFile(filepath.Join(s.Dir, "queue", "a.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(s.Dir); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("Load with a in both places: %v", err)
	}
}

// TestDoneFolderValidate: an item in queue/done/ is done and has no number,
// and an open item is numbered 1 or more; numbers that are not positions, or
// repeat, are not an error.
func TestDoneFolderValidate(t *testing.T) {
	s := doneState(t, "a", "b")
	_ = os.MkdirAll(filepath.Join(s.Dir, "queue", "done"), 0o755)
	write := func(dir, name, text string) {
		if err := os.WriteFile(filepath.Join(s.Dir, "queue", dir, name+".yaml"),
			[]byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("done", "d1", "feature: d1\nstatus: done\nround: feature\npriority: 4\n")
	write("done", "d2", "feature: d2\nstatus: queued\nround: feature\n")
	write("", "q", "feature: q\nstatus: queued\nround: feature\npriority: 0\n")
	write("", "r", "feature: r\nstatus: queued\nround: feature\npriority: 1\n")
	got, err := Load(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, e := range got.Validate() {
		lines = append(lines, e.Error())
	}
	slices.Sort(lines)
	want := []string{
		"d1: a done item in queue/done/ has no priority, not 4",
		"d2: status queued in queue/done/, which holds done items only",
		"q: priority 0 is below 1",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Validate:\n%s\nwant:\n%s", strings.Join(lines, "\n"),
			strings.Join(want, "\n"))
	}
}

// TestOldLayoutNumbers: with no queue/done/, a closing item stays in queue/
// numbered past the highest number held, so numbers stay unique across every
// item, which an old wf's Validate requires.
func TestOldLayoutNumbers(t *testing.T) {
	s := doneState(t, "a", "b", "c")
	closeItem(t, s, "b", now)
	got, err := Load(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, f := range got.Features {
		if o, ok := seen[f.Priority]; ok {
			t.Fatalf("%s and %s both hold %d", f.Feature, o, f.Priority)
		}
		seen[f.Priority] = f.Feature
	}
	b, _ := got.Find("b")
	if order(got) != "a:1 c:2" || b.Priority != 3 || !exists(s.Dir, "queue/b.yaml") {
		t.Fatalf("open %s, b at %d", order(got), b.Priority)
	}
	closeItem(t, got, "a", now)
	again, _ := Load(s.Dir)
	a, _ := again.Find("a")
	if b, _ = again.Find("b"); a.Priority != 4 || b.Priority != 3 {
		t.Fatalf("a at %d and b at %d, want 4 and 3", a.Priority, b.Priority)
	}
}

// TestMigrateDone: wf migrate -done creates queue/done/, moves every done
// item into it without its number, renumbers the open items, and refuses a
// second run and a worker that is not a named human.
func TestMigrateDone(t *testing.T) {
	s := doneState(t, "a", "b", "c")
	closeItem(t, s, "a", now)
	s, _ = Load(s.Dir)
	c, _ := s.Find("c")
	c.Priority = 7
	if err := s.writeNumber(c); err != nil {
		t.Fatal(err)
	}
	s, _ = Load(s.Dir)
	if _, err := s.MigrateDone("cloud/run"); err == nil {
		t.Fatal("a cloud worker's migration was not refused")
	}
	moved, err := s.MigrateDone("human/jeff")
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0].Feature != "a" || exists(s.Dir, "queue/a.yaml") ||
		!exists(s.Dir, "queue/done/a.yaml") {
		t.Fatalf("moved %v", moved)
	}
	got, err := Load(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := got.Find("a")
	if a.Priority != 0 || order(got) != "b:1 c:2" || len(got.Validate()) != 0 {
		t.Fatalf("a at %d, open %s, Validate %v", a.Priority, order(got), got.Validate())
	}
	if _, err := got.MigrateDone("human/jeff"); err == nil ||
		!strings.Contains(err.Error(), "already") {
		t.Fatalf("a second run: %v", err)
	}
}

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

var now = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

// stateDir is a checkout of the workflow branch with the config and no item.
func stateDir(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("run_budget: {steps: 8}\n"+config), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// wf runs one wf command on the state checkout and returns what it printed
// and its error.
func wf(t *testing.T, dir, cmd string, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	runErr := run(cmd, append([]string{"-state", dir}, args...), now)
	os.Stdout = stdout
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

// must is wf for a command that has to succeed.
func must(t *testing.T, dir, cmd string, args ...string) string {
	t.Helper()
	out, err := wf(t, dir, cmd, args...)
	if err != nil {
		t.Fatalf("wf %s %v: %v", cmd, args, err)
	}
	return out
}

// refused asserts that the command fails with the message.
func refused(t *testing.T, dir, want, cmd string, args ...string) {
	t.Helper()
	_, err := wf(t, dir, cmd, args...)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("wf %s %v: %v, want %q", cmd, args, err, want)
	}
}

// queued files a feature with wf new and returns its name.
func queued(t *testing.T, dir, name string, priority string, more ...string) string {
	t.Helper()
	must(t, dir, "new", append([]string{"-worker", "human/jeff", "-feature", name,
		"-title", "The item " + name, "-priority", priority, "-estimate-steps", "9"},
		more...)...)
	return name
}

// load reads the state checkout.
func load(t *testing.T, dir string) *queue.State {
	t.Helper()
	s, err := queue.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// item is the named item as the state checkout has it.
func item(t *testing.T, dir, name string) *queue.Feature {
	t.Helper()
	f, err := load(t, dir).Find(name)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestClaimIsGuarded: wf claim applies FLOW-05's limits to every worker, as
// wf next does, and a step the worker already holds stays claimable (FLOW-04,
// FLOW-05; case 5 of the loop-parallel design).
func TestClaimIsGuarded(t *testing.T) {
	dir := stateDir(t, "limits: {in_progress: 1}\n")
	a, b := queued(t, dir, "a", "1"), queued(t, dir, "b", "2")
	claim := func(name, worker string) []string {
		return []string{"-worker", worker, "-feature", name, "-step", "f0-branch-setup"}
	}
	must(t, dir, "claim", claim(a, "cloud/run-1")...)
	const full = "f0-branch-setup may not be claimed: the in-progress limit (limit 1): " +
		"it collides with a (FLOW-05)"
	refused(t, dir, full, "claim", claim(b, "cloud/run-2")...)
	const line = "blocked\tb\tthe in-progress limit (limit 1): it collides with a " +
		"(FLOW-05)\nnone\n"
	if out := must(t, dir, "next", "-worker", "cloud/run-2", "-feature", b); out != line {
		t.Fatalf("wf next -feature b printed %q", out)
	}
	// The holder claims its own step again: a holder is never refused.
	must(t, dir, "claim", claim(a, "cloud/run-1")...)
	if got := item(t, dir, b).Status; got != "queued" {
		t.Fatalf("the refused claim left b %s", got)
	}
	must(t, dir, "validate")
}

const (
	here = "repository: o/here\n"
	away = "o/relay"
)

// events are the item's history lines, without their times.
func events(f *queue.Feature) []string {
	var out []string
	for _, e := range f.History {
		out = append(out, e.By+": "+e.Event)
	}
	return out
}

// lastEvent is the item's newest history line.
func lastEvent(t *testing.T, dir, name string) string {
	t.Helper()
	ev := events(item(t, dir, name))
	return ev[len(ev)-1]
}

// TestNewDeclares: wf new records the repositories, the areas and the release
// an item adopts; an item homed in another repository is tracked, and only
// for it are seams recorded at intake (FLOW-02, FLOW-03, LOOP-07).
func TestNewDeclares(t *testing.T) {
	t.Run("recorded", newRecords)
	t.Run("refused", newRefuses)
}

// declares is what the item's queue file says it declares, on one line.
func declares(t *testing.T, dir, name string) string {
	t.Helper()
	f := item(t, dir, name)
	areas, adopts := "undeclared", ""
	if f.Areas != nil {
		areas = fmt.Sprint(*f.Areas)
	}
	if f.Adopts != nil {
		adopts = f.Adopts.Consumable + "@" + f.Adopts.Version
	}
	var steps []string
	for _, st := range f.Steps {
		steps = append(steps, st.ID+":"+st.Affinity)
	}
	return fmt.Sprintf("%s/%d repos%v areas %s seams%v adopts[%s] urgent[%s] class %d %v",
		f.Round, f.Phase, f.Repos, areas, f.Seams, adopts, f.Urgent, f.Class(), steps)
}

func newRecords(t *testing.T) {
	dir := stateDir(t, here)
	queued(t, dir, "plain", "1")
	queued(t, dir, "full", "2", "-repos", "o/here, o/specs", "-areas", "relay,book",
		"-adopts", "buildwarden-relay@v0.3.0")
	// -areas given empty declares that the item changes no area.
	queued(t, dir, "empty", "3", "-areas", "")
	// An item homed elsewhere is tracked: two human steps, seams recorded.
	queued(t, dir, "m1", "4", "-repos", away, "-seams", "relay-claim, relay-image")
	for name, want := range map[string]string{
		"plain": "feature/0 repos[] areas undeclared seams[] adopts[] urgent[] class 4 " +
			"[f0-branch-setup:cloud]",
		"full": "feature/0 repos[o/here o/specs] areas [relay book] seams[] " +
			"adopts[buildwarden-relay@v0.3.0] urgent[] class 3 " +
			"[f0-branch-setup:cloud]",
		"empty": "feature/0 repos[] areas [] seams[] adopts[] urgent[] class 4 " +
			"[f0-branch-setup:cloud]",
		"m1": "tracked/1 repos[o/relay] areas undeclared seams[relay-claim relay-image] " +
			"adopts[] urgent[] class 4 [t1-work:human]",
	} {
		if got := declares(t, dir, name); got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
	must(t, dir, "validate")
}

func newRefuses(t *testing.T) {
	dir := stateDir(t, here)
	queued(t, dir, "plain", "1")
	for _, c := range []struct {
		want string
		args []string
	}{
		{"-seams is for a tracked item", []string{"-seams", "relay-claim"}},
		{"-seams is for a tracked item",
			[]string{"-repos", "o/here,o/specs", "-seams", "x"}},
		{`repos entry "specs" is not owner/name`, []string{"-repos", "o/here,specs"}},
		{"repos names o/here twice", []string{"-repos", "o/here,o/here"}},
		{"areas names relay twice", []string{"-areas", "relay,relay"}},
		{"-adopts is <consumable>@<version>", []string{"-adopts", "buildwarden-relay"}},
		{"-adopts is <consumable>@<version>", []string{"-adopts", "@v0.3.0"}},
		{"only a feature adopts a release (LOOP-07)",
			[]string{"-kind", "process", "-adopts", "buildwarden-relay@v0.3.0"}},
		{"a process item is homed in the queue's repository",
			[]string{"-kind", "process", "-repos", away}},
	} {
		args := append([]string{"-worker", "human/jeff", "-feature", "bad", "-title", "x",
			"-priority", "9", "-estimate-steps", "9"}, c.args...)
		refused(t, dir, c.want, "new", args...)
		if _, err := load(t, dir).Find("bad"); err == nil {
			t.Fatalf("new %v: the refused item was written", c.args)
		}
	}
	if got := item(t, dir, "plain").Priority; got != 1 {
		t.Fatalf("a refused wf new renumbered plain to %d", got)
	}

	// config.yaml's repository has no default: -repos needs it.
	bare := stateDir(t, "")
	refused(t, bare, "config: repository is not set, and x names repos", "new", "-worker",
		"human/jeff", "-feature", "x", "-title", "x", "-priority", "1", "-estimate-steps",
		"9", "-repos", "o/here")
}

// TestNewFollowsCopiesAreas: a fix filed with -follows and no -areas takes
// the repositories and areas of the item it follows, and its history says
// so; -repos and -areas override; an undeclared item gives an undeclared fix
// (FLOW-09; case 23).
func TestNewFollowsCopiesAreas(t *testing.T) {
	dir := stateDir(t, here)
	queued(t, dir, "x", "1", "-repos", "o/here,o/specs", "-areas", "driver/hyperv,book")
	queued(t, dir, "plain", "2")
	declared := func(name string) string {
		f := item(t, dir, name)
		areas := "undeclared"
		if f.Areas != nil {
			areas = "[" + strings.Join(*f.Areas, " ") + "]"
		}
		return strings.Join(f.Repos, " ") + " " + areas
	}
	for _, c := range []struct {
		name, follows string
		args          []string
		want, event   string
	}{
		{"fix", "x", nil, "o/here o/specs [driver/hyperv book]",
			"cloud/tick-1: repos and areas taken from x"},
		{"narrow", "x", []string{"-areas", "book"}, " [book]", ""},
		{"moved", "x", []string{"-repos", "o/here"}, "o/here [driver/hyperv book]",
			"cloud/tick-1: areas taken from x"},
		{"both", "x", []string{"-repos", "o/here", "-areas", ""}, "o/here []", ""},
		{"bare", "plain", nil, " undeclared",
			"cloud/tick-1: repos and areas taken from plain: it has none"},
	} {
		args := append([]string{"-worker", "cloud/tick-1", "-feature", c.name, "-title",
			"x", "-priority", "9", "-estimate-steps", "9", "-follows", c.follows},
			c.args...)
		must(t, dir, "new", args...)
		if got := declared(c.name); got != c.want {
			t.Errorf("%s: declares %q, want %q", c.name, got, c.want)
		}
		got := lastEvent(t, dir, c.name)
		if want := c.event; (want != "" && got != want) ||
			(want == "" && strings.Contains(got, "taken from")) {
			t.Errorf("%s: logged %q, want %q", c.name, got, want)
		}
	}
	must(t, dir, "validate")
}

// TestUrgentCommands: wf new -urgent and wf urgent write the mark and its
// history line for a human worker and for no one else, never on a process
// item, a tracked item or an item that follows one; wf status shows the mark;
// and wf claim prints the line of a start past the waiting limit (FLOW-05;
// cases 42 and 44).
func TestUrgentCommands(t *testing.T) {
	const onlyHuman = "only a human marks an item urgent (FLOW-05)"
	dir := stateDir(t, here+"limits: {waiting_on_human: 1}\n")
	queued(t, dir, "sec", "1", "-urgent")
	if f := item(t, dir, "sec"); f.Urgent != "human/jeff" ||
		lastEvent(t, dir, "sec") != "human/jeff: marked urgent (FLOW-05)" {
		t.Fatalf("wf new -urgent: urgent %q, history %q", f.Urgent, events(f))
	}
	base := func(name string, more ...string) []string {
		return append([]string{"-feature", name, "-title", "x", "-priority", "9",
			"-estimate-steps", "9"}, more...)
	}
	queued(t, dir, "parent", "8")
	for _, c := range []struct {
		want string
		args []string
	}{
		{onlyHuman, base("bad", "-worker", "cloud/tick-1", "-urgent")},
		{onlyHuman, base("bad", "-urgent")},
		{"bad is a process item: the waiting limit never refuses one (FLOW-05)",
			base("bad", "-worker", "human/jeff", "-urgent", "-kind", "process")},
		{"bad is tracked: the waiting limit never refuses one (FLOW-05)",
			base("bad", "-worker", "human/jeff", "-urgent", "-repos", away)},
		{"bad follows parent: the waiting limit never refuses a fix (FLOW-05)",
			base("bad", "-worker", "cloud/tick-1", "-urgent", "-follows", "parent")},
		{"bad follows parent: the waiting limit never refuses a fix (FLOW-05)",
			base("bad", "-worker", "human/jeff", "-urgent", "-follows", "parent")},
	} {
		refused(t, dir, c.want, "new", c.args...)
		if _, err := load(t, dir).Find("bad"); err == nil {
			t.Fatalf("new %v: the refused item was written", c.args)
		}
	}

	// wf urgent marks an item already queued, and clears the mark.
	queued(t, dir, "late", "2")
	mark := func(worker, name string, more ...string) []string {
		return append([]string{"-worker", worker, "-feature", name}, more...)
	}
	refused(t, dir, onlyHuman, "urgent", mark("host:macos-vz/s", "late")...)
	refused(t, dir, onlyHuman, "urgent", mark("cloud/tick-1", "late")...)
	must(t, dir, "urgent", mark("human/jeff", "late")...)
	if out := must(t, dir, "status"); !strings.Contains(out, "\t  urgent: human/jeff\n") {
		t.Fatalf("wf status does not show the mark:\n%s", out)
	}
	refused(t, dir, "only a human clears the mark", "urgent",
		mark("cloud/tick-1", "late", "-clear")...)
	must(t, dir, "urgent", mark("human/jeff", "late", "-clear")...)
	if f := item(t, dir, "late"); f.Urgent != "" ||
		lastEvent(t, dir, "late") != "human/jeff: urgent cleared" {
		t.Fatalf("wf urgent -clear: urgent %q, history %q", f.Urgent, events(f))
	}
	refused(t, dir, "late is not marked urgent", "urgent",
		mark("human/jeff", "late", "-clear")...)

	// One item waits on the human, the limit: an unmarked feature is
	// refused its start, and the marked one's claim prints why it started.
	queued(t, dir, "p", "3", "-kind", "process")
	must(t, dir, "claim", "-worker", "human/jeff", "-feature", "p", "-step", "x1-design")
	start := func(name string) []string {
		return []string{"-worker", "cloud/tick-1", "-feature", name, "-step",
			"f0-branch-setup", "-new-run"}
	}
	refused(t, dir, "the waiting limit: 1 items wait on the human (limit 1): p (FLOW-05)",
		"claim", start("late")...)
	const line = "started past the waiting limit (1 wait, limit 1): marked urgent by " +
		"human/jeff (FLOW-05)"
	if out := must(t, dir, "claim", start("sec")...); out != line+"\n" {
		t.Fatalf("wf claim printed %q, want the start line", out)
	}
	if ev := events(item(t, dir, "sec")); !strings.Contains(strings.Join(ev, "\n"),
		"cloud/tick-1: "+line) {
		t.Fatalf("the start is not in the history: %q", ev)
	}
	// The mark is refused on an item that has started; clearing is not.
	refused(t, dir, "sec is in progress: the waiting limit refuses only a start (FLOW-05)",
		"urgent", mark("human/jeff", "sec")...)
	must(t, dir, "urgent", mark("human/jeff", "sec", "-clear")...)
	must(t, dir, "validate")
}

// TestTrackedDone: wf done completes a tracked item's steps with no gate, and
// says so (FLOW-03).
func TestTrackedDone(t *testing.T) {
	dir := stateDir(t, here)
	queued(t, dir, "m1", "1", "-repos", away)
	step := func(id string) []string {
		return []string{"-worker", "human/jeff", "-feature", "m1", "-step", id}
	}
	must(t, dir, "claim", step("t1-work")...)
	if got := item(t, dir, "m1").Status; got != "in_progress" {
		t.Fatalf("a human's claim left the tracked item %s", got)
	}
	for _, id := range []string{"t1-work", "t2-merge"} {
		out := must(t, dir, "done", step(id)...)
		if !strings.Contains(out, "tracked: wf runs no gate in o/relay (FLOW-03)") {
			t.Fatalf("wf done %s printed %q", id, out)
		}
	}
	if got := item(t, dir, "m1").Status; got != "done" {
		t.Fatalf("after t2-merge the tracked item is %s", got)
	}
}

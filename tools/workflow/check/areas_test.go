package check

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const inside = "diff inside the declared areas (FLOW-03)"

// touch commits the files on feat/demo, each holding text, and pushes.
func touch(t *testing.T, work, text string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(work, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(text+"\n"), 0o644)
	}
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "touch "+strings.Join(files, " "))
	git(t, work, "push", "-q", "origin", "HEAD")
	git(t, work, "fetch", "-q", "origin")
}

// TestInsideAreas: from Implement on, every path the branch changes lies in
// an area the item declares, or is its own design; an undeclared item passes
// (FLOW-03).
func TestInsideAreas(t *testing.T) {
	work := repo(t, withAreas(design, "[driver/container, book]"))
	// A file main has, for the branch to delete and to rename.
	git(t, work, "checkout", "-q", "main")
	touch(t, work, "x", "relay/old.go", "driver/container/moved.go")
	git(t, work, "checkout", "-q", "feat/demo")
	git(t, work, "merge", "-q", "--no-edit", "origin/main")
	f := queueAreas(demo(3), "driver/container", "book")
	env := Env{Repo: work, State: mapped(t, f)}
	check := func() Result {
		t.Helper()
		return result(t, Phase(env, f), inside)
	}
	// Its own design is in the designs area, which it need not declare.
	touch(t, work, "x", "driver/container/run.go", "docs/book/src/guide/output.md")
	if r := check(); !r.OK {
		t.Fatalf("inside its areas: %+v", r)
	}
	touch(t, work, "x", "relay/relay.go", "notes.txt")
	r := check()
	if r.OK || !strings.Contains(r.Info, "relay/relay.go (area relay)") ||
		!strings.Contains(r.Info, "notes.txt (area core)") ||
		strings.Contains(r.Info, "demo.md") || !strings.Contains(r.Info, "FLOW-06") {
		t.Fatalf("outside its areas: %+v", r)
	}
	queueAreas(f, "driver/container", "book", "relay", "core")
	if r := check(); !r.OK {
		t.Fatalf("after the widening: %+v", r)
	}
	// A deleted path is a change to its area, and a renamed path to both.
	queueAreas(f, "driver/container", "book", "core")
	git(t, work, "rm", "-q", "relay/relay.go", "relay/old.go")
	_ = os.MkdirAll(filepath.Join(work, "collector"), 0o755)
	git(t, work, "mv", "driver/container/moved.go", "collector/moved.go")
	git(t, work, "commit", "-q", "-m", "delete and rename")
	if r := check(); r.OK || !strings.Contains(r.Info, "relay/old.go (area relay)") ||
		!strings.Contains(r.Info, "collector/moved.go (area collector)") ||
		strings.Contains(r.Info, "relay/relay.go") {
		t.Fatalf("a deleted and a renamed path: %+v", r)
	}
	insideUndeclared(t, env, f)
}

// insideUndeclared is the second half of TestInsideAreas: an undeclared item
// passes, and the later gates and the refresh run the check too.
func insideUndeclared(t *testing.T, env Env, f *queue.Feature) {
	t.Helper()
	check := func() Result {
		t.Helper()
		return result(t, Phase(env, f), inside)
	}
	// An undeclared item passes: no areas, an area the map lacks, no map.
	for name, change := range map[string]func(){
		"no areas":           func() { f.Areas = nil },
		"an area not mapped": func() { queueAreas(f, "book", "telemetry") },
		"no map":             func() { env.State = nil },
	} {
		change()
		if r := check(); !r.OK || !strings.Contains(r.Info, "undeclared") {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// Review, Guide, Publish and the refresh run it too.
	queueAreas(f, "book")
	env.State = mapped(t, f)
	for _, phase := range []int{4, 5, 6} {
		f.Phase = phase
		if r := check(); r.OK {
			t.Errorf("phase %d passed with paths outside the areas", phase)
		}
	}
	if r := result(t, Refresh(env, f), inside); r.OK {
		t.Error("the refresh passed with paths outside the areas")
	}
	// The refresh compares the queue with the design, as the gates do.
	if r := result(t, Refresh(env, f), matches); r.OK {
		t.Errorf("the refresh passed with the queue's areas not the design's: %+v", r)
	}
	queueAreas(f, "driver/container", "book")
	if r := result(t, Refresh(env, f), matches); !r.OK {
		t.Errorf("the refresh's comparison of the queue with the design: %+v", r)
	}
}

// other makes a pushed branch feat/<name> from main that writes the files,
// and returns its in-progress item declaring the areas.
func other(t *testing.T, work, name, text string, areas []string, files ...string,
) *queue.Feature {
	t.Helper()
	git(t, work, "checkout", "-q", "-b", "feat/"+name, "origin/main")
	touch(t, work, text, files...)
	git(t, work, "checkout", "-q", "feat/demo")
	return &queue.Feature{Feature: name, Status: "in_progress", Round: "feature", Phase: 3,
		Branch: "feat/" + name, Areas: &areas, Declared: "0a1b2c3"}
}

// TestTrialMerge: Review and Publish try a merge with the branch of each
// in-progress feature the item is disjoint from, and report it; the report
// never fails the step (FLOW-03, FLOW-10).
func TestTrialMerge(t *testing.T) {
	work := repo(t, withAreas(design, "[driver/container, book]"))
	touch(t, work, "demo", "driver/container/run.go", "README.md", "relay/stray.go")
	f := queueAreas(demo(4), "driver/container", "book")
	f.Status = "in_progress"
	clean := other(t, work, "clean", "clean", []string{"driver/vz"}, "driver/vz/vz.go")
	ordered := other(t, work, "ordered", "ordered", []string{"driver/qemu", "book"},
		"driver/qemu/q.go", "README.md")
	// It edits a file in relay, which demo's branch has too, outside its
	// areas.
	exclusive := other(t, work, "exclusive", "exclusive", []string{"relay"},
		"relay/stray.go")
	gone := &queue.Feature{Feature: "gone", Status: "in_progress", Round: "feature",
		Phase: 3, Branch: "feat/gone", Areas: &[]string{"driver/hyperv"}}
	// Not tried: a colliding item, a queued one, a process item.
	colliding := other(t, work, "colliding", "c", []string{"driver/container"},
		"driver/container/run.go")
	queued := other(t, work, "queued", "q", []string{"ledger"}, "README.md")
	queued.Status = "queued"
	process := other(t, work, "process", "p", nil, "README.md")
	process.Kind, process.Round = "process", "process"
	env := Env{Repo: work, State: mapped(t, f, clean, ordered, exclusive, gone, colliding,
		queued, process)}
	want := map[string]string{
		"clean": "clean",
		"ordered": "conflicts in ordered files, which the later of the two settles " +
			"at its refresh: README.md",
		"exclusive": "a conflict in the exclusive area relay, so one of the two is " +
			"outside its areas: relay/stray.go",
		"gone": "origin/feat/gone is not on the remote: not tried",
	}
	for _, phase := range []int{4, 6} {
		f.Phase = phase
		rs := Phase(env, f)
		for name, info := range want {
			r := result(t, rs, "trial merge with "+name+" (a report, FLOW-10)")
			if !r.OK || r.Info != info {
				t.Errorf("phase %d, %s: %+v, want %q", phase, name, r, info)
			}
		}
		for _, r := range rs {
			name := strings.TrimPrefix(r.Name, "trial merge with ")
			name, _, _ = strings.Cut(name, " ")
			if slices.Contains([]string{"colliding", "queued", "process"}, name) {
				t.Errorf("phase %d: a trial merge with %s: %+v", phase, name, r)
			}
		}
	}
	f.Phase = 3
	for _, r := range Phase(env, f) {
		if strings.Contains(r.Name, "trial merge") {
			t.Errorf("Implement ran a trial merge: %+v", r)
		}
	}
}

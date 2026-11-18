package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// numbers is each open item's name and number in the checkout, in FLOW-05
// order.
func numbers(t *testing.T, dir string) string {
	t.Helper()
	var out []string
	for _, f := range load(t, dir).Listed(false) {
		out = append(out, f.Feature+":"+strconv.Itoa(f.Priority))
	}
	return strings.Join(out, " ")
}

// closeDone walks the item to done and saves it, as wf done's last step does.
func closeDone(t *testing.T, dir, name string) {
	t.Helper()
	s := load(t, dir)
	f, err := s.Find(name)
	if err != nil {
		t.Fatal(err)
	}
	for f.Status != "done" {
		for i := range f.Steps {
			if f.Steps[i].Status == "pending" {
				f.Status = "in_progress"
				queue.Complete(f, &f.Steps[i], "cloud/run", now)
				break
			}
		}
	}
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
}

// TestNewAndMovePrintPositions: wf new prints where the item went; wf move is
// a human's, places the item and logs one line; wf status prints positions,
// and -all a dash for a done item with no number (queue-positions).
func TestNewAndMovePrintPositions(t *testing.T) {
	dir := stateDir(t, "")
	queued(t, dir, "a", "1")
	queued(t, dir, "b", "2")
	out := must(t, dir, "new", "-worker", "human/jeff", "-feature", "p", "-title", "P",
		"-priority", "3", "-estimate-steps", "4", "-kind", "process")
	if out != "queued\tp\tposition 1\n" {
		t.Fatalf("wf new printed %q", out)
	}
	if got := lastEvent(t, dir, "p"); got != "human/jeff: queued at position 1 (asked 3)" {
		t.Fatalf("p's line is %q", got)
	}
	if got := numbers(t, dir); got != "p:1 a:2 b:3" {
		t.Fatalf("after wf new: %s", got)
	}
	refused(t, dir, "LOOP-04", "move", "-worker", "cloud/run", "-feature", "b", "-to", "2")
	out = must(t, dir, "move", "-worker", "human/jeff", "-feature", "b", "-to", "2",
		"-why", "b first")
	if out != "moved\tb\tposition 2\n" || numbers(t, dir) != "p:1 b:2 a:3" {
		t.Fatalf("wf move printed %q; %s", out, numbers(t, dir))
	}
	if got := lastEvent(t, dir, "b"); got !=
		"human/jeff: moved to position 2 from 3 (asked 2): b first" {
		t.Fatalf("b's line is %q", got)
	}
	if len(load(t, dir).Features[2].History) != 1 {
		t.Fatal("the move logged a line on another item")
	}
	if out := must(t, dir, "status"); !strings.HasPrefix(out, "1\tp\t") ||
		!strings.Contains(out, "\n2\tb\t") || !strings.Contains(out, "\n3\ta\t") {
		t.Fatalf("wf status:\n%s", out)
	}
	closeDone(t, dir, "a")
	if out := must(t, dir, "status", "-all"); !strings.Contains(out, "\n3\ta\tdone") {
		t.Fatalf("wf status -all, a done with no queue/done/:\n%s", out)
	}
	if err := os.MkdirAll(filepath.Join(dir, "queue", "done"), 0o755); err != nil {
		t.Fatal(err)
	}
	closeDone(t, dir, "b")
	out = must(t, dir, "status", "-all")
	if !strings.Contains(out, "\n-\ta\tdone") || !strings.Contains(out, "\n-\tb\tdone") {
		t.Fatalf("wf status -all, once queue/done/ exists:\n%s", out)
	}
	refused(t, dir, "done", "move", "-worker", "human/jeff", "-feature", "b", "-to", "1")
}

// TestMigrateDoneCommand: wf migrate -done moves the done items and prints a
// line for each, once, for a human worker.
func TestMigrateDoneCommand(t *testing.T) {
	dir := stateDir(t, "")
	queued(t, dir, "a", "1")
	queued(t, dir, "b", "2")
	closeDone(t, dir, "a")
	refused(t, dir, "human", "migrate", "-done", "-worker", "cloud/run")
	out := must(t, dir, "migrate", "-done", "-worker", "human/jeff")
	if out != "moved\ta\tqueue/done/a.yaml\n" {
		t.Fatalf("wf migrate -done printed %q", out)
	}
	if numbers(t, dir) != "b:1" {
		t.Fatalf("open: %s", numbers(t, dir))
	}
	must(t, dir, "validate")
	refused(t, dir, "already", "migrate", "-done", "-worker", "human/jeff")
}

// TestCommitPositions: wf commit commits what renumbering changed, leaves a
// file with another session's changes alone, commits a close as a move, and
// settles a replay whose conflicts are only priority lines (queue-positions).
func TestCommitPositions(t *testing.T) {
	t.Run("a renumbered file is committed", commitRenumbered)
	t.Run("a file with other changes is left alone", commitRenumberLeavesOthers)
	t.Run("a close commits the move", commitCloseMoves)
	t.Run("a done item an old wf left is moved", commitMovesLeftDone)
	t.Run("a replay settles priority conflicts", commitReplaysPriorities)
	t.Run("a replayed wf new keeps its place", commitReplaysNew)
}

func commitRenumbered(t *testing.T) {
	a, _ := stateRepos(t, "", "x", "y")
	queued(t, a, "n", "1")
	pushed(t, a, "human/jeff", "queue: n", "queue/n.yaml")
	files := gitT(t, a, "show", "--name-only", "--format=", "HEAD")
	for _, p := range []string{"queue/n.yaml", "queue/x.yaml", "queue/y.yaml"} {
		if !strings.Contains(files, p) {
			t.Fatalf("the commit has %q, want %s", files, p)
		}
	}
	if !strings.Contains(remoteFile(t, a, "queue/y.yaml"), "priority: 3") {
		t.Fatal("y's new number was not pushed")
	}
	if st := gitT(t, a, "status", "--porcelain", "--", "queue"); st != "" {
		t.Fatalf("the checkout is left with %q", st)
	}
}

func commitRenumberLeavesOthers(t *testing.T) {
	a, _ := stateRepos(t, "", "x", "y")
	edit(t, a, "y", func(f *queue.Feature) { f.Title = "another session's edit" })
	queued(t, a, "n", "1")
	pushed(t, a, "human/jeff", "queue: n", "queue/n.yaml")
	files := gitT(t, a, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "queue/x.yaml") || strings.Contains(files, "queue/y.yaml") {
		t.Fatalf("the commit has %q, want x and not y", files)
	}
	if item(t, a, "y").Title != "another session's edit" {
		t.Fatal("the other session's edit is gone")
	}
}

func commitCloseMoves(t *testing.T) {
	a, _ := stateRepos(t, "", "x", "y")
	// A plain commit and a claim commit pass before queue/done/ exists.
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "X" })
	pushed(t, a, "human/jeff", "x: title", "queue/x.yaml")
	claimStep(t, a, "cloud/a", "y", "f0-branch-setup")
	pushed(t, a, "cloud/a", "y: claim", "-claimed", "y/f0-branch-setup")
	must(t, a, "migrate", "-done", "-worker", "human/jeff")
	pushed(t, a, "human/jeff", "queue: done folder", "queue")
	closeDone(t, a, "x")
	pushed(t, a, "cloud/a", "x: closed", "queue/x.yaml")
	files := gitT(t, a, "show", "--name-status", "--format=", "HEAD")
	if !strings.Contains(files, "queue/x.yaml") ||
		!strings.Contains(files, "queue/done/x.yaml") {
		t.Fatalf("the close commit has %q, want the move", files)
	}
	if st := gitT(t, a, "status", "--porcelain"); st != "" {
		t.Fatalf("the checkout is left with %q", st)
	}
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "X done" })
	pushed(t, a, "human/jeff", "x: title", "queue/done/x.yaml")
}

func commitMovesLeftDone(t *testing.T) {
	a, _ := stateRepos(t, "", "x", "y")
	must(t, a, "migrate", "-done", "-worker", "human/jeff")
	write(t, a, "queue/done/.keep", "")
	pushed(t, a, "human/jeff", "queue: done folder", "queue")
	// An old wf closes x where it was.
	text := strings.Replace(gitT(t, a, "show", "HEAD:queue/x.yaml"), "status: queued",
		"status: done", 1)
	write(t, a, "queue/x.yaml", text+"\n")
	gitT(t, a, "commit", "-q", "-am", "x: closed by an old wf")
	edit(t, a, "y", func(f *queue.Feature) { f.Title = "Y" })
	pushed(t, a, "human/jeff", "y: title", "queue/y.yaml")
	if _, err := os.Stat(filepath.Join(a, "queue", "x.yaml")); err == nil {
		t.Fatal("the done x was left in queue/")
	}
	if !strings.Contains(remoteFile(t, a, "queue/done/x.yaml"), "status: done") {
		t.Fatal("x was not moved to queue/done/ on origin")
	}
}

func commitReplaysPriorities(t *testing.T) {
	a, b := stateRepos(t, "", "x", "y", "z")
	must(t, b, "move", "-worker", "human/jeff", "-feature", "y", "-to", "1")
	pushed(t, b, "human/jeff", "y: first", "queue/y.yaml")
	if _, err := wfAt(t, a, now.Add(time.Minute), "move", "-worker", "human/jeff",
		"-feature", "z", "-to", "1"); err != nil {
		t.Fatal(err)
	}
	out := pushed(t, a, "human/jeff", "z: first", "queue/z.yaml")
	if !strings.Contains(out, "replayed") {
		t.Fatalf("wf commit did not replay:\n%s", out)
	}
	gitT(t, a, "fetch", "-q", "origin")
	gitT(t, a, "reset", "-q", "--hard", "origin/workflow")
	if got := numbers(t, a); got != "z:1 y:2 x:3" {
		t.Fatalf("on origin: %s, want z (moved later), then y", got)
	}
	must(t, a, "validate")
}

func commitReplaysNew(t *testing.T) {
	a, b := stateRepos(t, "", "x", "y", "z")
	queued(t, b, "n", "3")
	pushed(t, b, "human/jeff", "queue: n", "queue/n.yaml")
	queued(t, a, "m", "1")
	pushed(t, a, "human/jeff", "queue: m", "queue/m.yaml")
	gitT(t, a, "fetch", "-q", "origin")
	gitT(t, a, "reset", "-q", "--hard", "origin/workflow")
	if got := numbers(t, a); got != "m:1 x:2 n:3 y:4 z:5" {
		t.Fatalf("on origin: %s", got)
	}
}

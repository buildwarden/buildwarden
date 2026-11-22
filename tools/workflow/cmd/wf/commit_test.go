package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// stateRepos is two checkouts, a and b, of the workflow branch of one bare
// origin, as two sessions on two machines have them. Each item is queued
// with wf new in a before b is cloned, and pushed.
func stateRepos(t *testing.T, config string, items ...string) (a, b string) {
	t.Helper()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	a, b = filepath.Join(root, "a"), filepath.Join(root, "b")
	gitT(t, root, "init", "-q", "--bare", "-b", "workflow", origin)
	gitT(t, root, "clone", "-q", origin, a)
	gitT(t, a, "checkout", "-q", "-b", "workflow")
	if err := os.MkdirAll(filepath.Join(a, "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, a, "config.yaml", "run_budget: {steps: 8}\n"+config)
	write(t, a, "queue/.keep", "")
	for i, name := range items {
		queued(t, a, name, string(rune('1'+i)))
	}
	gitT(t, a, "add", ".")
	gitT(t, a, "commit", "-q", "-m", "base")
	gitT(t, a, "push", "-q", "origin", "workflow")
	gitT(t, root, "clone", "-q", "-b", "workflow", origin, b)
	return a, b
}

func write(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commit runs wf commit and returns what it printed, and its exit status: 0
// on success, otherwise the CommitError's code, or 1 for any other error.
func commit(t *testing.T, dir, worker, msg string, more ...string) (string, int) {
	t.Helper()
	out, err := wf(t, dir, "commit", append([]string{"-worker", worker, "-m", msg},
		more...)...)
	if err == nil {
		return out, 0
	}
	var ce *queue.CommitError
	if errors.As(err, &ce) {
		return out + ce.Error(), ce.Code
	}
	return out + err.Error(), 1
}

// pushed commits and pushes the session's change, which must succeed.
func pushed(t *testing.T, dir, worker, msg string, more ...string) string {
	t.Helper()
	out, code := commit(t, dir, worker, msg, more...)
	if code != 0 {
		t.Fatalf("wf commit %q: exit %d: %s", msg, code, out)
	}
	return out
}

// remoteFile is the file as origin's workflow branch has it.
func remoteFile(t *testing.T, dir, name string) string {
	t.Helper()
	gitT(t, dir, "fetch", "-q", "origin")
	return gitT(t, dir, "show", "origin/workflow:"+name)
}

// claimStep claims the item's first step in the checkout, as wf claim does.
func claimStep(t *testing.T, dir, worker, name, step string) {
	t.Helper()
	must(t, dir, "claim", "-worker", worker, "-feature", name, "-step", step)
}

// TestCommitReplay: wf commit pushes queue state and, when the push is
// rejected, replays a finished change or asks a claim again on the state
// that won (FLOW-04; cases 11 and 12 of the loop-parallel design).
func TestCommitReplay(t *testing.T) {
	t.Run("a change is replayed and QUEUE.md regenerated", commitReplaysChange)
	t.Run("a claim is asked again and counted once", commitClaimStands)
	t.Run("a claim is dropped with the refusal", commitClaimDropped)
	t.Run("a conflict in a queue file", commitQueueConflict)
	t.Run("main.yaml keeps the remote's, areas.yaml does not", commitMainAndAreas)
	t.Run("a replay that Validate fails", commitReplayInvalid)
	t.Run("only the named paths are staged", commitStagesOnlyPaths)
	t.Run("nothing to commit", commitNothing)
	t.Run("the remote moves on every try", commitGivesUp)
}

// commitGivesUp: when every push is rejected, wf commit stops after the last
// try with exit 5. A claim is dropped and the checkout left at origin's
// state; a change keeps its commit locally (FLOW-04).
func commitGivesUp(t *testing.T) {
	setup := func(t *testing.T) (a string) {
		a, b := stateRepos(t, "", "x")
		// Before each of a's pushes, b pushes a commit of its own, so the
		// remote has always moved.
		hook := "#!/bin/sh\nunset GIT_DIR GIT_INDEX_FILE GIT_WORK_TREE\n" +
			"cd '" + b + "' && git pull -q --rebase origin workflow && " +
			"git commit -q --allow-empty -m move && git push -q origin workflow\n"
		p := filepath.Join(a, ".git", "hooks", "pre-push")
		if err := os.WriteFile(p, []byte(hook), 0o755); err != nil {
			t.Fatal(err)
		}
		return a
	}
	t.Run("a claim", func(t *testing.T) {
		a := setup(t)
		claimStep(t, a, "cloud/a", "x", "f0-branch-setup")
		out, code := commit(t, a, "cloud/a", "x: claim", "-claimed", "x/f0-branch-setup")
		if code != 5 || !strings.Contains(out, "the claim is dropped; ask wf next again") {
			t.Fatalf("exit %d, want 5 dropping the claim:\n%s", code, out)
		}
		if gitT(t, a, "rev-parse", "HEAD") != gitT(t, a, "rev-parse", "origin/workflow") {
			t.Fatal("the checkout was not left at origin/workflow")
		}
		if x := item(t, a, "x"); x.Status != "queued" {
			t.Fatalf("the dropped claim left x %s", x.Status)
		}
	})
	t.Run("a change", func(t *testing.T) {
		a := setup(t)
		edit(t, a, "x", func(f *queue.Feature) { f.Title = "A's title" })
		out, code := commit(t, a, "human/jeff", "x: title", "queue/x.yaml")
		if code != 5 || !strings.Contains(out, "the commit is kept locally") {
			t.Fatalf("exit %d, want 5 keeping the commit:\n%s", code, out)
		}
		if got := gitT(t, a, "log", "-1", "--format=%s"); got != "x: title" {
			t.Fatalf("the checkout's head is %q, want the kept commit", got)
		}
	})
}

func commitReplaysChange(t *testing.T) {
	a, b := stateRepos(t, "")
	queued(t, b, "two", "2")
	pushed(t, b, "human/jeff", "queue: two", "queue/two.yaml")
	queued(t, a, "one", "1")
	out := pushed(t, a, "human/jeff", "queue: one", "queue/one.yaml")
	if !strings.Contains(out, "replayed") {
		t.Fatalf("wf commit did not say it replayed:\n%s", out)
	}
	md := remoteFile(t, a, "QUEUE.md")
	if !strings.Contains(md, "| one |") || !strings.Contains(md, "| two |") {
		t.Fatalf("QUEUE.md on origin was not regenerated with both items:\n%s", md)
	}
	// Nothing is forced: b's commit is still in origin's history.
	gitT(t, a, "merge-base", "--is-ancestor", gitT(t, b, "rev-parse", "HEAD"),
		"origin/workflow")
	if got := gitT(t, a, "log", "-1", "--format=%s", "origin/workflow"); got != "queue: one" {
		t.Fatalf("origin's head is %q, want the replayed commit", got)
	}
}

func commitClaimStands(t *testing.T) {
	a, b := stateRepos(t, "", "x", "y")
	claimStep(t, b, "cloud/b", "y", "f0-branch-setup")
	pushed(t, b, "cloud/b", "y: claim", "-claimed", "y/f0-branch-setup")
	claimStep(t, a, "cloud/a", "x", "f0-branch-setup")
	// -new-run is accepted and ignored: the replayed claim counts its run
	// once, from the worker.
	pushed(t, a, "cloud/a", "x: claim", "-claimed", "x/f0-branch-setup", "-new-run")
	x := item(t, a, "x")
	if x.Budget.Spent.Steps != 1 || x.Budget.Spent.Runs != 1 {
		t.Fatalf("x spent %+v, want the claim counted once", x.Budget.Spent)
	}
	if st, _ := x.Step("f0-branch-setup"); st.Lease.ClaimedBy != "cloud/a" {
		t.Fatalf("x's step is leased to %q", st.Lease.ClaimedBy)
	}
	if item(t, a, "y").Status != "in_progress" {
		t.Fatal("the claim that won was lost")
	}
}

func commitClaimDropped(t *testing.T) {
	cases := []struct {
		name, want string
		// win is the change of the push that wins, made in checkout b.
		win func(t *testing.T, b string)
	}{
		{"the last slot", "the in-progress limit (limit 1)", func(t *testing.T, b string) {
			claimStep(t, b, "cloud/b", "y", "f0-branch-setup")
			pushed(t, b, "cloud/b", "y: claim", "-claimed", "y/f0-branch-setup")
		}},
		{"a process item's hold", "held by process item p", func(t *testing.T, b string) {
			must(t, b, "new", "-worker", "human/jeff", "-feature", "p", "-title", "P",
				"-priority", "9", "-estimate-steps", "4", "-kind", "process",
				"-holds", "cloud")
			claimStep(t, b, "human/jeff", "p", "x1-design")
			// Its design done, the item holds from its Implement on (FLOW-20).
			edit(t, b, "p", func(f *queue.Feature) {
				queue.Complete(f, &f.Steps[0], "human/jeff", time.Now())
			})
			claimStep(t, b, "cloud/b", "p", "x2-implement")
			pushed(t, b, "cloud/b", "p: implement", "queue/p.yaml")
		}},
		{"a blocking question", "blocking question q1", func(t *testing.T, b string) {
			edit(t, b, "x", func(f *queue.Feature) {
				f.Questions = append(f.Questions, queue.Question{ID: "q1",
					Kind: "blocking", Status: "open", Question: "Which?",
					Options: []string{"a", "b"}, Recommended: "a"})
			})
			pushed(t, b, "human/jeff", "x: q1", "queue/x.yaml")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := stateRepos(t, "limits: {in_progress: 1}\n", "x", "y")
			c.win(t, b)
			claimStep(t, a, "cloud/a", "x", "f0-branch-setup")
			out, code := commit(t, a, "cloud/a", "x: claim", "-claimed",
				"x/f0-branch-setup")
			if code != 4 || !strings.Contains(out, c.want) {
				t.Fatalf("exit %d, want 4 with %q:\n%s", code, c.want, out)
			}
			head := gitT(t, a, "rev-parse", "HEAD")
			if head != gitT(t, a, "rev-parse", "origin/workflow") {
				t.Fatal("the checkout was not left at origin/workflow")
			}
			x := item(t, a, "x")
			if x.Status != "queued" || x.Budget.Spent.Steps != 0 {
				t.Fatalf("the dropped claim left x %s with %d steps", x.Status,
					x.Budget.Spent.Steps)
			}
		})
	}
}

func commitQueueConflict(t *testing.T) {
	a, b := stateRepos(t, "", "x")
	edit(t, b, "x", func(f *queue.Feature) { f.Title = "B's title" })
	pushed(t, b, "human/jeff", "x: b", "queue/x.yaml")
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "A's title" })
	out, code := commit(t, a, "human/jeff", "x: a", "queue/x.yaml")
	if code != 5 || !strings.Contains(out, "queue/x.yaml") {
		t.Fatalf("exit %d, want 5 naming queue/x.yaml:\n%s", code, out)
	}
	if got := gitT(t, a, "log", "-1", "--format=%s"); got != "x: a" {
		t.Fatalf("the session's commit was not restored: HEAD is %q", got)
	}
	if item(t, a, "x").Title != "A's title" {
		t.Fatal("the session's change is gone")
	}
}

func commitMainAndAreas(t *testing.T) {
	a, b := stateRepos(t, here, "x")
	write(t, b, "main.yaml", "sha: bbbbbbb\nci: green\n")
	pushed(t, b, "cloud/b", "main: green", "main.yaml")
	write(t, a, "main.yaml", "sha: aaaaaaa\nci: red\n")
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "A's title" })
	out := pushed(t, a, "cloud/a", "main and x", "main.yaml", "queue/x.yaml")
	if !strings.Contains(out, "main.yaml: kept the remote's; record main again") {
		t.Fatalf("wf commit did not say main.yaml was kept:\n%s", out)
	}
	if got := remoteFile(t, a, "main.yaml"); !strings.Contains(got, "bbbbbbb") {
		t.Fatalf("main.yaml on origin is %q, want the remote's", got)
	}
	if !strings.Contains(remoteFile(t, a, "queue/x.yaml"), "A's title") {
		t.Fatal("the rest of the change was not replayed")
	}

	write(t, b, "areas.yaml", areasYAML("o/here", "b/"))
	pushed(t, b, "human/jeff", "areas: b", "areas.yaml")
	write(t, a, "areas.yaml", areasYAML("o/here", "a/"))
	out, code := commit(t, a, "human/jeff", "areas: a", "areas.yaml")
	if code != 5 || !strings.Contains(out, "areas.yaml") {
		t.Fatalf("exit %d, want 5 naming areas.yaml:\n%s", code, out)
	}
}

// areasYAML is a map of one exclusive area beside the catch-all.
func areasYAML(repo, path string) string {
	return "version: 1\nrepository: " + repo + "\ncatch_all: core\nareas:\n" +
		"  core: {mode: exclusive}\n  one: {mode: exclusive, paths: [" + path + "]}\n"
}

func commitReplayInvalid(t *testing.T) {
	a, b := stateRepos(t, "", "x", "y")
	if err := os.Remove(filepath.Join(b, "queue", "y.yaml")); err != nil {
		t.Fatal(err)
	}
	pushed(t, b, "human/jeff", "queue: y gone", "queue/y.yaml")
	edit(t, a, "x", func(f *queue.Feature) { f.CollidesWith = []string{"y"} })
	out, code := commit(t, a, "human/jeff", "x: collides with y", "queue/x.yaml")
	if code != 5 || !strings.Contains(out, `collides_with "y"`) {
		t.Fatalf("exit %d, want 5 with Validate's line:\n%s", code, out)
	}
	if got := gitT(t, a, "log", "-1", "--format=%s"); got != "x: collides with y" {
		t.Fatalf("the session's commit was not restored: HEAD is %q", got)
	}
}

func commitStagesOnlyPaths(t *testing.T) {
	a, b := stateRepos(t, "", "x", "y")
	write(t, a, "notes.txt", "another session's file\n")
	edit(t, a, "y", func(f *queue.Feature) { f.Title = "another session's edit" })
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "mine" })
	pushed(t, a, "human/jeff", "x: mine", "queue/x.yaml")
	files := gitT(t, a, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "queue/x.yaml") || strings.Contains(files, "notes.txt") ||
		strings.Contains(files, "queue/y.yaml") {
		t.Fatalf("the commit has %q, want only the path given and QUEUE.md", files)
	}

	// A rejected push would reset the checkout: with another session's
	// uncommitted work in it, wf commit stops and names the files.
	queued(t, b, "z", "3")
	pushed(t, b, "human/jeff", "queue: z", "queue/z.yaml")
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "mine again" })
	out, code := commit(t, a, "human/jeff", "x: again", "queue/x.yaml")
	if code != 5 || !strings.Contains(out, "notes.txt") ||
		!strings.Contains(out, "queue/y.yaml") {
		t.Fatalf("exit %d, want 5 naming notes.txt and queue/y.yaml:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(a, "notes.txt")); err != nil {
		t.Fatalf("the other session's file was reset away: %v", err)
	}
	if item(t, a, "y").Title != "another session's edit" {
		t.Fatal("the other session's edit was reset away")
	}
}

func commitNothing(t *testing.T) {
	a, _ := stateRepos(t, "", "x")
	before := gitT(t, a, "rev-parse", "HEAD")
	out := pushed(t, a, "human/jeff", "nothing")
	if !strings.Contains(out, "nothing to commit") {
		t.Fatalf("wf commit printed %q", out)
	}
	if gitT(t, a, "rev-parse", "HEAD") != before {
		t.Fatal("wf commit made a commit with nothing to commit")
	}
}

// TestCommitAreasHumanOnly: a change to areas.yaml is the human's word, and
// wf commit makes it only for a human worker (FLOW-05).
func TestCommitAreasHumanOnly(t *testing.T) {
	a, _ := stateRepos(t, here)
	write(t, a, "areas.yaml", areasYAML("o/here", "a/"))
	for _, w := range []string{"cloud/run", "host:macbook/s"} {
		out, code := commit(t, a, w, "areas: a", "areas.yaml")
		if code == 0 || !strings.Contains(out, "areas.yaml") {
			t.Fatalf("%s: exit %d, want a refusal:\n%s", w, code, out)
		}
	}
	if gitT(t, a, "status", "--porcelain", "--", "areas.yaml") != "?? areas.yaml" {
		t.Fatal("the refused commit staged areas.yaml")
	}
	pushed(t, a, "human/jeff", "areas: a", "areas.yaml")
	if !strings.Contains(remoteFile(t, a, "areas.yaml"), "a/") {
		t.Fatal("the human's map change was not pushed")
	}
}

// TestCommitAreasUnpushedCommit: a worker that is not human does not push an
// earlier unpushed commit that changed areas.yaml under its own (FLOW-05).
func TestCommitAreasUnpushedCommit(t *testing.T) {
	a, _ := stateRepos(t, here, "x")
	write(t, a, "areas.yaml", areasYAML("o/here", "a/"))
	gitT(t, a, "add", "areas.yaml")
	gitT(t, a, "commit", "-q", "-m", "hand areas")
	hand := gitT(t, a, "rev-parse", "--short", "HEAD")
	before := gitT(t, a, "rev-parse", "origin/workflow")
	edit(t, a, "x", func(f *queue.Feature) { f.Title = "A's title" })
	out, code := commit(t, a, "cloud/run", "x: title", "queue/x.yaml")
	if code == 0 || !strings.Contains(out, "areas.yaml") || !strings.Contains(out, hand) {
		t.Fatalf("exit %d, want a refusal naming areas.yaml and %s:\n%s", code, hand, out)
	}
	gitT(t, a, "fetch", "-q", "origin")
	if gitT(t, a, "rev-parse", "origin/workflow") != before {
		t.Fatal("the refused commit pushed the unpushed map change")
	}
	pushed(t, a, "human/jeff", "x: title", "queue/x.yaml")
	if !strings.Contains(remoteFile(t, a, "areas.yaml"), "a/") {
		t.Fatal("the human's push did not carry the map change")
	}
}

// TestCommitAreasProgram: while a program trunk is configured, a worker that
// is not human commits a change to areas.yaml when it commits an area-map
// record with it, one for each map change the push carries; without one, or
// with no program trunk configured, it is refused (FLOW-05, FLOW-20, a16).
func TestCommitAreasProgram(t *testing.T) {
	a, _ := stateRepos(t, programHere)
	write(t, a, "areas.yaml", areasYAML("o/here", "a/"))
	out, code := commit(t, a, "cloud/run", "areas: a", "areas.yaml")
	if code == 0 || !strings.Contains(out, "area-map record") {
		t.Fatalf("an unrecorded map change: exit %d, want a refusal:\n%s", code, out)
	}
	refused(t, a, "area-map record says why", "record", "-worker", "cloud/run", "-kind",
		"area-map", "-note", "split a")
	must(t, a, "record", "-worker", "cloud/run", "-kind", "area-map", "-ref",
		"docs/design/relay-split.md", "-note", "area a, as the design names it")
	out, code = commit(t, a, "cloud/run", "areas: a", "areas.yaml")
	if code == 0 || !strings.Contains(out, "area-map record") {
		t.Fatalf("a record left uncommitted: exit %d, want a refusal:\n%s", code, out)
	}
	pushed(t, a, "cloud/run", "areas: a", "areas.yaml", queue.RecordsFile)
	if !strings.Contains(remoteFile(t, a, "areas.yaml"), "a/") {
		t.Fatal("the recorded map change was not pushed")
	}
	// A second change needs a second record: the first is pushed.
	write(t, a, "areas.yaml", areasYAML("o/here", "b/"))
	out, code = commit(t, a, "cloud/run", "areas: b", "areas.yaml", queue.RecordsFile)
	if code == 0 || !strings.Contains(out, "area-map record") {
		t.Fatalf("a second change on the first record: exit %d:\n%s", code, out)
	}
	must(t, a, "record", "-worker", "cloud/run", "-kind", "area-map", "-ref",
		queue.AreaMapEdges, "-note", "b reaches a, as area-edges.sh shows")
	pushed(t, a, "cloud/run", "areas: b", "areas.yaml", queue.RecordsFile)

	// With no program trunk configured, the map stays the human's.
	h, _ := stateRepos(t, here)
	write(t, h, "areas.yaml", areasYAML("o/here", "a/"))
	if err := queue.AppendRecord(h, queue.Record{By: "cloud/run", Kind: "area-map",
		Ref: queue.AreaMapEdges, Note: "edges"}); err != nil {
		t.Fatal(err)
	}
	out, code = commit(t, h, "cloud/run", "areas: a", "areas.yaml", queue.RecordsFile)
	if code == 0 || !strings.Contains(out, "human's word") {
		t.Fatalf("no program trunk: exit %d, want the human-only refusal:\n%s", code, out)
	}
}

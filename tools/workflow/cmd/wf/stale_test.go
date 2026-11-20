package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// gitT runs git in dir and returns what it printed.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.autocrlf", "GIT_CONFIG_VALUE_0=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// change writes each file in the checkout with the text, commits them as msg
// on the branch that is checked out, and pushes it.
func change(t *testing.T, work, msg, text string, files ...string) {
	t.Helper()
	for _, f := range files {
		err := os.WriteFile(filepath.Join(work, f), []byte(text+"\n"), 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, work, "add", ".")
	gitT(t, work, "commit", "-q", "-m", msg)
	gitT(t, work, "push", "-q", "origin", "HEAD")
	gitT(t, work, "fetch", "-q", "--prune", "origin")
}

// codeRepo is a checkout of the code, on main, with a bare origin that has
// main and, for each name, a pushed branch feat/<name> that changes
// <name>.txt and shared.txt.
func codeRepo(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	gitT(t, root, "init", "-q", "--bare", "-b", "main", origin)
	gitT(t, root, "clone", "-q", origin, work)
	gitT(t, work, "checkout", "-q", "-b", "main")
	change(t, work, "base", "base", "base.txt", "shared.txt")
	for _, n := range names {
		gitT(t, work, "checkout", "-q", "-b", "feat/"+n, "main")
		change(t, work, n, n, n+".txt", "shared.txt")
	}
	gitT(t, work, "checkout", "-q", "main")
	return work
}

// moveMain commits to main as msg, changing the files, and pushes it.
func moveMain(t *testing.T, work, msg string, files ...string) {
	t.Helper()
	gitT(t, work, "checkout", "-q", "main")
	change(t, work, msg, msg, files...)
}

// mergePR merges feat/<name> into main as GitHub merges a pull request, and
// deletes the branch on the remote and in the checkout, as GitHub and Close
// do.
func mergePR(t *testing.T, work, name string, pr int) {
	t.Helper()
	gitT(t, work, "checkout", "-q", "main")
	gitT(t, work, "merge", "-q", "--no-ff", "-X", "theirs", "-m",
		"Merge pull request #"+strconv.Itoa(pr)+" from o/feat/"+name, "feat/"+name)
	gitT(t, work, "push", "-q", "origin", "main", ":feat/"+name)
	gitT(t, work, "fetch", "-q", "--prune", "origin")
	gitT(t, work, "branch", "-q", "-D", "feat/"+name)
}

// edit changes the named item in the state checkout and saves it.
func edit(t *testing.T, dir, name string, fn func(f *queue.Feature)) {
	t.Helper()
	s := load(t, dir)
	f, err := s.Find(name)
	if err != nil {
		t.Fatal(err)
	}
	fn(f)
	if err := s.Save(f); err != nil {
		t.Fatal(err)
	}
}

// reach files the item and takes it, in progress, to the phase, completing
// through the step named last when one is given. With pr above 0 it records
// that PR, its CI green for the branch's pushed head.
func reach(t *testing.T, dir, work, name string, prio string, phase, pr int,
	last ...string,
) {
	t.Helper()
	queued(t, dir, name, prio)
	edit(t, dir, name, func(f *queue.Feature) {
		f.Status = "in_progress"
		for f.Phase < phase {
			for i := range f.Steps {
				if f.Steps[i].Status == "pending" && f.Steps[i].Phase == f.Phase {
					queue.Complete(f, &f.Steps[i], "test", now)
					break
				}
			}
		}
		for _, id := range last {
			st, err := f.Step(id)
			if err != nil {
				t.Fatal(err)
			}
			queue.Complete(f, st, "test", now)
		}
		if pr > 0 {
			f.PR = &queue.PR{Number: pr, CI: "green",
				Head: gitT(t, work, "rev-parse", "origin/feat/"+name)}
		}
	})
}

// runStale runs wf stale as the tick does and returns its lines.
func runStale(t *testing.T, dir, work string) []string {
	t.Helper()
	out := must(t, dir, "stale", "-repo", work, "-worker", "cloud/run")
	if strings.TrimSpace(out) == "" {
		return nil
	}
	return strings.Split(strings.TrimSpace(out), "\n")
}

// pending is the item's pending steps, by id.
func pending(t *testing.T, dir, name string) string {
	t.Helper()
	var ids []string
	for _, st := range item(t, dir, name).Steps {
		if st.Status == "pending" {
			ids = append(ids, st.ID)
		}
	}
	return strings.Join(ids, " ")
}

// TestStale: wf stale seeds a refresh on each open PR main has moved past,
// and on a feature that merged main in Publish and has not finished it; it
// runs the merge check first, so a PR the human merged gets none (FLOW-08,
// FLOW-10; cases 18, 33 and 38 of the loop-parallel design).
func TestStale(t *testing.T) {
	t.Run("main moved", staleMainMoved)
	t.Run("in Publish", staleInPublish)
	t.Run("merged", staleAfterMerge)
	t.Run("landed", staleLanded)
	t.Run("needs a worker", func(t *testing.T) {
		refused(t, stateDir(t, here), "-worker", "stale", "-repo", codeRepo(t))
	})
}

func staleMainMoved(t *testing.T) {
	work, dir := codeRepo(t, "a", "x"), stateDir(t, here)
	reach(t, dir, work, "a", "1", 7, 7)
	must(t, dir, "new", "-worker", "human/jeff", "-feature", "x", "-title", "X", "-priority",
		"2", "-estimate-steps", "9", "-kind", "process")
	edit(t, dir, "x", func(f *queue.Feature) {
		f.Status = "in_progress"
		for f.Phase < 3 {
			queue.Complete(f, &f.Steps[len(f.Steps)-1], "test", now)
		}
		f.PR = &queue.PR{Number: 9, CI: "green", Head: gitT(t, work, "rev-parse",
			"origin/feat/x")}
	})
	if got := runStale(t, dir, work); got != nil {
		t.Fatalf("wf stale on current PRs printed %q", got)
	}
	// Whatever moved main, an item's merge or a commit that is no item's,
	// every open PR waits for a refresh.
	moveMain(t, work, "the weekly tidy", "shared.txt", "go.sum")
	main := gitT(t, work, "rev-parse", "--short", "origin/main")
	note := "main at " + main + ": 1 commit since the branch merged it; both change shared.txt"
	// In FLOW-05's order: the process item is ahead of the feature.
	want := []string{"refresh\tx\tx2-refresh\t" + note, "refresh\ta\tf6-refresh\t" + note}
	if got := runStale(t, dir, work); !slices.Equal(got, want) {
		t.Fatalf("wf stale = %q, want %q", got, want)
	}
	for name, id := range map[string]string{"a": "f6-refresh", "x": "x2-refresh"} {
		f := item(t, dir, name)
		st := f.PendingRefresh()
		if st == nil || st.ID != id || st.Note != note || f.PR.CI != "pending" ||
			f.ReadyToMerge() {
			t.Fatalf("%s: refresh %+v, pr %+v", name, st, f.PR)
		}
		if got := lastEvent(t, dir, name); got != "cloud/run: seeded "+id+": "+note {
			t.Fatalf("%s: history %q", name, got)
		}
	}
	// Run twice, nothing is added twice, though main moves again.
	moveMain(t, work, "another commit", "other.txt")
	if got := runStale(t, dir, work); got != nil {
		t.Fatalf("a second wf stale printed %q", got)
	}
	if got := pending(t, dir, "a"); got != "f7-merge f6-refresh" {
		t.Fatalf("a's pending steps %q", got)
	}
	must(t, dir, "validate")
	// Both are in class 1 with a refresh pending; x, queued as a process item
	// ahead of the class-4 a, holds the earlier position.
	if got := must(t, dir, "next", "-worker", "cloud/run"); !strings.HasPrefix(got,
		"next\tx\tx2-refresh\tcloud") {
		t.Fatalf("wf next = %q, want x's refresh first", got)
	}
}

// staleInPublish is case 38, and what gets no refresh.
func staleInPublish(t *testing.T) {
	work := codeRepo(t, "pub", "pubpr", "early", "back")
	dir := stateDir(t, here)
	reach(t, dir, work, "pub", "1", 6, 0, "f6-merge-main", "f6-consolidate")
	reach(t, dir, work, "pubpr", "2", 6, 12, "f6-merge-main")
	reach(t, dir, work, "early", "3", 6, 0)
	reach(t, dir, work, "back", "4", 7, 13)
	must(t, dir, "back", "-worker", "cloud/run", "-feature", "back", "-to", "3", "-why",
		"a merged check fails")
	moveMain(t, work, "another item (#30)", "other.txt")
	main := gitT(t, work, "rev-parse", "--short", "origin/main")
	note := "main at " + main +
		": 1 commit since the branch merged it; no file changed by both"
	want := []string{"refresh\tpub\tf6-refresh\t" + note,
		"refresh\tpubpr\tf6-refresh\t" + note}
	if got := runStale(t, dir, work); !slices.Equal(got, want) {
		t.Fatalf("wf stale = %q, want %q", got, want)
	}
	for _, name := range []string{"pub", "pubpr"} {
		f := item(t, dir, name)
		open, err := f.Step("f6-open-pr")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(open.Needs, "f6-refresh") {
			t.Fatalf("%s: f6-open-pr needs %v, want f6-refresh among them", name,
				open.Needs)
		}
	}
	if f := item(t, dir, "pub"); len(f.PendingRefresh().Needs) != 0 {
		t.Fatalf("pub's refresh needs %v, with its consolidation done",
			f.PendingRefresh().Needs)
	}
	f := item(t, dir, "pubpr")
	if got := f.PendingRefresh().Needs; !slices.Equal(got, []string{"f6-consolidate"}) {
		t.Fatalf("pubpr's refresh needs %v, want its pending consolidation", got)
	}
	if f.PR.CI != "green" {
		t.Fatalf("pubpr's pr.ci is %q: only a PR that waits for the merge is set pending",
			f.PR.CI)
	}
	for _, name := range []string{"early", "back"} {
		if st := item(t, dir, name).PendingRefresh(); st != nil {
			t.Fatalf("%s got the refresh %s", name, st.ID)
		}
	}
	// The four undeclared items collide and three hold the PR slot, which
	// the fixture does not avoid; the steps wf stale wrote are valid.
	for _, err := range load(t, dir).Validate() {
		if !strings.HasPrefix(err.Error(), "limit: ") {
			t.Errorf("wf validate: %v", err)
		}
	}
}

func staleAfterMerge(t *testing.T) {
	work := codeRepo(t, "atmerge", "pub", "closed")
	dir := stateDir(t, here)
	reach(t, dir, work, "atmerge", "1", 7, 21)
	reach(t, dir, work, "pub", "2", 6, 22, "f6-merge-main")
	reach(t, dir, work, "closed", "3", 7, 23)
	mergePR(t, work, "atmerge", 21)
	mergePR(t, work, "pub", 22)
	gitT(t, work, "push", "-q", "origin", ":feat/closed")
	gitT(t, work, "fetch", "-q", "--prune", "origin")
	want := []string{"merged: atmerge (confirm: wf done -feature atmerge -step f7-merge)",
		"gone\tclosed\tPR #23 is not merged and origin/feat/closed is gone: no refresh"}
	if got := runStale(t, dir, work); !slices.Equal(got, want) {
		t.Fatalf("wf stale = %q, want %q", got, want)
	}
	for _, name := range []string{"atmerge", "pub", "closed"} {
		f := item(t, dir, name)
		if f.PendingRefresh() != nil || f.PR.CI != "green" {
			t.Fatalf("%s: pending %q, pr %+v", name, pending(t, dir, name), f.PR)
		}
	}
	// wf stale names the confirmation and does not run it.
	if got := item(t, dir, "atmerge").Phase; got != 7 {
		t.Fatalf("wf stale moved the merged item to phase %d", got)
	}
}

func staleLanded(t *testing.T) {
	work, dir := codeRepo(t, "back"), stateDir(t, here)
	reach(t, dir, work, "back", "1", 7, 31)
	must(t, dir, "back", "-worker", "cloud/run", "-feature", "back", "-to", "3", "-why",
		"a merged check fails")
	mergePR(t, work, "back", 31)
	want := []string{"landed\tback\tPR #31 merged while it was sent back: " +
		"moved to Close with f8-escalate-returned (FLOW-08)"}
	if got := runStale(t, dir, work); !slices.Equal(got, want) {
		t.Fatalf("wf stale = %q, want %q", got, want)
	}
	f := item(t, dir, "back")
	esc, err := f.Step("f8-escalate-returned")
	if err != nil || f.Phase != 8 || esc.Affinity != "human" ||
		!strings.Contains(esc.Note, "a merged check fails") {
		t.Fatalf("phase %d, escalation %+v, %v", f.Phase, esc, err)
	}
	if got := pending(t, dir, "back"); got != "f8-close f8-escalate-returned" {
		t.Fatalf("pending steps %q", got)
	}
	must(t, dir, "validate")
	if got := runStale(t, dir, work); got != nil {
		t.Fatalf("a second wf stale printed %q", got)
	}
}

// TestStaleChecksTheHead: a CI result is for the commit pr.head records, so
// wf stale seeds a refresh on a branch whose head is another commit, or whose
// PR records none, though the branch contains main; a passed refresh makes
// the PR ready again (FLOW-08; cases 36 and 37 of the loop-parallel design).
func TestStaleChecksTheHead(t *testing.T) {
	work, dir := codeRepo(t, "a", "old", "same"), stateDir(t, here)
	for i, name := range []string{"a", "old", "same"} {
		reach(t, dir, work, name, strconv.Itoa(i+1), 7, 40+i)
	}
	edit(t, dir, "old", func(f *queue.Feature) { f.PR.Head = "" })
	// GitHub's Update branch, or a push by hand: a head the loop did not
	// check.
	gitT(t, work, "checkout", "-q", "feat/a")
	change(t, work, "pushed by hand", "by hand", "a.txt")
	head := gitT(t, work, "rev-parse", "--short", "origin/feat/a")
	was := item(t, dir, "a").PR.Head[:7]
	want := []string{
		"refresh\ta\tf6-refresh\torigin/feat/a is at " + head + ", and pr.head records " +
			was + ": pr.ci was read for another commit",
		"refresh\told\tf6-refresh\torigin/feat/old is at " +
			gitT(t, work, "rev-parse", "--short", "origin/feat/old") +
			", and pr.head records no commit: pr.ci was read for another commit",
	}
	if got := runStale(t, dir, work); !slices.Equal(got, want) {
		t.Fatalf("wf stale = %q, want %q", got, want)
	}
	for _, name := range []string{"a", "old"} {
		if f := item(t, dir, name); f.PR.CI != "pending" || f.ReadyToMerge() {
			t.Fatalf("%s: pr %+v, ready %v", name, f.PR, f.ReadyToMerge())
		}
	}
	if f := item(t, dir, "same"); !f.ReadyToMerge() || f.PendingRefresh() != nil {
		t.Fatalf("same: pr %+v, pending %q", f.PR, pending(t, dir, "same"))
	}

	// The refresh passes: the agent records CI for the new head, and wf done
	// runs the refresh's done-when, which this scratch repository cannot
	// pass (it has no design), so the step is completed here as wf done
	// completes it.
	edit(t, dir, "a", func(f *queue.Feature) {
		f.PR.CI, f.PR.Head = "green", gitT(t, work, "rev-parse", "origin/feat/a")
		queue.Complete(f, f.PendingRefresh(), "cloud/run", now)
	})
	if f := item(t, dir, "a"); !f.ReadyToMerge() {
		t.Fatalf("a after its refresh: pr %+v, pending %q", f.PR, pending(t, dir, "a"))
	}
	if got := runStale(t, dir, work); got != nil {
		t.Fatalf("wf stale after the refresh printed %q", got)
	}
	// Before the human merges it, main moves again: it waits again (case 36).
	moveMain(t, work, "another item (#50)", "other.txt")
	runStale(t, dir, work)
	for _, name := range []string{"a", "same"} {
		if f := item(t, dir, name); f.ReadyToMerge() || f.PendingRefresh() == nil {
			t.Fatalf("%s is still ready after main moved: pr %+v", name, f.PR)
		}
	}
	if got := item(t, dir, "a").PendingRefresh().ID; got != "f6-refresh-r2" {
		t.Fatalf("a's second refresh is %s", got)
	}
}

// TestDoneRunsTheRefreshCheck: wf done on a refresh step runs the refresh's
// done-when, and the step stays pending when it fails (FLOW-03, FLOW-10).
func TestDoneRunsTheRefreshCheck(t *testing.T) {
	work, dir := codeRepo(t, "a"), stateDir(t, here)
	reach(t, dir, work, "a", "1", 7, 7)
	moveMain(t, work, "another item (#8)", "other.txt")
	runStale(t, dir, work)
	step := []string{"-repo", work, "-worker", "cloud/run", "-feature", "a", "-step",
		"f6-refresh"}
	must(t, dir, "claim", step...)
	out, err := wf(t, dir, "done", step...)
	if err == nil || !strings.Contains(err.Error(), "f6-refresh done-when not met") ||
		!strings.Contains(out, "FAIL current main merged") {
		t.Fatalf("wf done on a branch behind main: %v\n%s", err, out)
	}
	if got := pending(t, dir, "a"); got != "f7-merge f6-refresh" {
		t.Fatalf("pending steps %q", got)
	}
}

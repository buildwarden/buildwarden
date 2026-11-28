package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// programHere is config.yaml with o/here's program trunk recorded.
const programHere = "repository: o/here\nrepositories:\n" +
	"  - {name: o/here, trunk: main, program_trunk: program/componentization,\n" +
	"    profile: docs/development/loop-profile.yaml}\n"

// TestTrunkCommands: wf trunk prints the branch an item works against, and
// wf main -trunk records the program trunk's reading in program.yaml
// (FLOW-10, FLOW-20).
func TestTrunkCommands(t *testing.T) {
	dir := stateDir(t, programHere)
	queued(t, dir, "p", "1", "-program")
	queued(t, dir, "f", "2")
	if got := must(t, dir, "trunk", "-feature", "p"); got != "program/componentization\n" {
		t.Fatalf("wf trunk -feature p = %q", got)
	}
	if got := must(t, dir, "trunk", "-feature", "f"); got != "main\n" {
		t.Fatalf("wf trunk -feature f = %q", got)
	}
	must(t, dir, "main", "-trunk", "program/componentization", "-sha", "abc1234",
		"-ci", "red")
	data, err := os.ReadFile(filepath.Join(dir, "program.yaml"))
	if err != nil || !strings.Contains(string(data), "ci: red") {
		t.Fatalf("program.yaml = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.yaml")); err == nil {
		t.Fatal("wf main -trunk program/componentization wrote main.yaml")
	}
	refused(t, dir, "neither the trunk nor the program trunk", "main", "-trunk", "dev",
		"-sha", "abc1234", "-ci", "red")

	dir = stateDir(t, here)
	queued(t, dir, "p", "1", "-program")
	if got := must(t, dir, "trunk", "-feature", "p"); got !=
		"main\t(o/here has no program trunk)\n" {
		t.Fatalf("wf trunk with no program trunk = %q", got)
	}
}

// TestSyncProgram: wf sync-program names the sync branch once main is ahead
// of the program trunk, says the program trunk is current otherwise, and
// waits while a program-sync item is open; wf new -escalate files that item
// with a human escalation as its first step (FLOW-20, Decision 14).
func TestSyncProgram(t *testing.T) {
	work := codeRepo(t)
	gitT(t, work, "push", "-q", "origin", "main:refs/heads/program/componentization")
	gitT(t, work, "fetch", "-q", "origin")
	dir := stateDir(t, programHere)
	got := must(t, dir, "sync-program", "-repo", work)
	if !strings.HasPrefix(got, "current\to/here\t") {
		t.Fatalf("wf sync-program on a current program trunk = %q", got)
	}
	moveMain(t, work, "process item", "rules.txt")
	gitT(t, work, "fetch", "-q", "origin")
	head := gitT(t, work, "rev-parse", "origin/main")
	got = must(t, dir, "sync-program", "-repo", work)
	if !strings.HasPrefix(got, "sync\to/here\tsync/program-"+head[:12]+"\t") {
		t.Fatalf("wf sync-program with main ahead = %q", got)
	}

	queued(t, dir, "program-sync-"+head[:12], "1", "-program", "-escalate",
		"rules.txt conflicts: main "+head[:7]+", program trunk")
	f := item(t, dir, "program-sync-"+head[:12])
	if f.Steps[0].ID != "f0-escalate" || f.Steps[0].Affinity != "human" ||
		!strings.Contains(f.Steps[0].Note, "rules.txt conflicts") ||
		f.Steps[1].Needs[0] != "f0-escalate" {
		t.Fatalf("steps %+v, want the escalation first", f.Steps)
	}
	got = must(t, dir, "sync-program", "-repo", work)
	if !strings.HasPrefix(got, "waits\to/here\tprogram-sync-") {
		t.Fatalf("wf sync-program with a program-sync item open = %q", got)
	}
}

// TestSyncProgramChecks: a program-sync item of another repository does not
// stop this one's sync; a program trunk that does not resolve is an error,
// not a sync; and -head records only a head that is the sync's merge and
// nothing more (FLOW-20, Decision 14).
func TestSyncProgramChecks(t *testing.T) {
	work := codeRepo(t)
	dir := stateDir(t, programHere+"  - {name: o/specs, trunk: main}\n")
	refused(t, dir, "does not resolve", "sync-program", "-repo", work)

	gitT(t, work, "push", "-q", "origin", "main:refs/heads/program/componentization")
	moveMain(t, work, "process item", "rules.txt")
	gitT(t, work, "fetch", "-q", "origin")
	head := gitT(t, work, "rev-parse", "origin/main")
	queued(t, dir, "program-sync-0123456789ab", "1", "-repos", "o/specs", "-escalate",
		"a conflict in o/specs")
	got := must(t, dir, "sync-program", "-repo", work)
	if !strings.HasPrefix(got, "sync\to/here\tsync/program-"+head[:12]+"\t") {
		t.Fatalf("wf sync-program with another repository's sync open = %q", got)
	}

	branch := "sync/program-" + head[:12]
	gitT(t, work, "checkout", "-q", "-b", branch, "origin/program/componentization")
	gitT(t, work, "merge", "-q", "--no-ff", "--no-edit", "origin/main")
	sync := gitT(t, work, "rev-parse", "HEAD")
	change(t, work, "more", "more", "more.txt")
	refused(t, dir, "is not a sync head", "sync-program", "-repo", work, "-worker",
		"cloud/run", "-head", "HEAD")
	refused(t, dir, "needs -worker", "sync-program", "-repo", work, "-head", sync)
	got = must(t, dir, "sync-program", "-repo", work, "-worker", "cloud/run", "-head", sync)
	if got != "recorded\tsync\to/here\t"+branch+"\t"+sync[:12]+"\n" {
		t.Fatalf("wf sync-program -head = %q", got)
	}
	rs, err := queue.LoadRecords(dir)
	if err != nil || len(rs) != 1 || rs[0].Kind != "sync" || rs[0].Ref != branch ||
		rs[0].Head != sync {
		t.Fatalf("records %+v, %v", rs, err)
	}
}

// TestRecordCommands: wf record appends to program/records.yaml, record
// -list prints in Decided for you order, and program-body renders a
// repository's records (FLOW-20, Decision 13).
func TestRecordCommands(t *testing.T) {
	dir := stateDir(t, programHere)
	queued(t, dir, "p", "1", "-program")
	refused(t, dir, "record kind", "record", "-worker", "cloud/run", "-kind", "whim",
		"-note", "x")
	must(t, dir, "record", "-worker", "cloud/run", "-kind", "merge", "-feature", "p",
		"-repository", "o/here", "-head", "0123456789abcdef", "-ref", "#42",
		"-note", "loop merge, a merge commit")
	must(t, dir, "record", "-worker", "cloud/run", "-kind", "return", "-feature", "p",
		"-note", "third return to Design")
	got := must(t, dir, "record", "-list")
	if !strings.HasPrefix(got, "- return **p**: third return") ||
		!strings.Contains(got, "- merge **p** o/here #42 at `0123456`") {
		t.Fatalf("wf record -list = %q", got)
	}
	body := must(t, dir, "program-body")
	if !strings.Contains(body, "### merge (1)") || !strings.Contains(body, "### return (1)") {
		t.Fatalf("wf program-body = %q", body)
	}
}

// TestReestimateCommand: wf reestimate answers a program item's first
// runaway for the cloud and records it; a second is refused to the cloud
// (FLOW-13, FLOW-20).
func TestReestimateCommand(t *testing.T) {
	dir := stateDir(t, programHere)
	queued(t, dir, "p", "1", "-program")
	refused(t, dir, "only a program item's first runaway", "reestimate", "-worker",
		"cloud/run", "-feature", "p")
	edit(t, dir, "p", func(f *queue.Feature) { f.Budget.Spent.Steps = 18 })
	if got := must(t, dir, "reestimate", "-worker", "cloud/run", "-feature", "p"); got !=
		"reestimated\tp\t18 steps\n" {
		t.Fatalf("wf reestimate = %q", got)
	}
	if got := must(t, dir, "record", "-list"); !strings.Contains(got, "re-estimate **p**") {
		t.Fatalf("records = %q", got)
	}
	edit(t, dir, "p", func(f *queue.Feature) { f.Budget.Spent.Steps = 36 })
	refused(t, dir, "only a program item's first runaway", "reestimate", "-worker",
		"cloud/run", "-feature", "p")
}

// TestTagNextRCFlags: tag-next -rc names the program item of the component's
// last phase, which must work in the repository, and -feature without -rc
// is refused (FLOW-20, a17 (a)).
func TestTagNextRCFlags(t *testing.T) {
	dir := stateDir(t, programHere)
	queued(t, dir, "p", "1")
	work := t.TempDir()
	refused(t, dir, "names the last phase of an -rc", "tag-next", "-repo", work,
		"-feature", "p")
	refused(t, dir, "is not a program item", "tag-next", "-repo", work, "-rc",
		"-feature", "p")
}

// syncRepo is codeRepo with this repository's profile on main and main
// pushed as program/componentization, then each side committing one file:
// main its mainFile, the program trunk its progFile, each with its own text.
// The sync branch merges origin/main into the program trunk; resolve, when
// the merge conflicts, is the text it concludes the conflict with. It
// returns the checkout, the sync branch and its head.
func syncRepo(t *testing.T, mainFile, progFile, resolve string) (string, string, string) {
	t.Helper()
	work := codeRepo(t)
	profile, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "development",
		"loop-profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	put := func(p, text string) {
		full := filepath.Join(work, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		gitT(t, work, "add", p)
	}
	put("docs/development/loop-profile.yaml", string(profile))
	gitT(t, work, "commit", "-q", "-m", "adopt")
	gitT(t, work, "push", "-q", "origin", "main", "main:refs/heads/program/componentization")
	gitT(t, work, "checkout", "-q", "-b", "pt", "origin/main")
	put(progFile, "p\n")
	gitT(t, work, "commit", "-q", "-m", "program item")
	gitT(t, work, "push", "-q", "origin", "pt:program/componentization")
	gitT(t, work, "checkout", "-q", "main")
	put(mainFile, "m\n")
	gitT(t, work, "commit", "-q", "-m", "process item")
	gitT(t, work, "push", "-q", "origin", "main")
	gitT(t, work, "fetch", "-q", "origin")
	head := gitT(t, work, "rev-parse", "origin/main")
	branch := "sync/program-" + head[:12]
	gitT(t, work, "checkout", "-q", "-b", branch, "origin/program/componentization")
	cmd := exec.Command("git", "merge", "-q", "--no-ff", "--no-edit", "origin/main")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if cmd.Run() != nil {
		put(mainFile, resolve)
		gitT(t, work, "commit", "-q", "--no-edit")
	}
	return work, branch, gitT(t, work, "rev-parse", "HEAD")
}

// TestSyncProgramRefusesGuarded: wf sync-program -head refuses a sync head
// whose resolution edits a path FLOW-14 guards and records nothing, and
// records one whose resolution is an ordinary path (FLOW-01, FLOW-20,
// Decision 4 of sync-pr-tier1).
func TestSyncProgramRefusesGuarded(t *testing.T) {
	work, _, sync := syncRepo(t, "tools/tier1/x.go", "tools/tier1/x.go", "mp\n")
	dir := stateDir(t, programHere)
	refused(t, dir, "resolves tools/tier1/x.go, which FLOW-14 guards", "sync-program",
		"-repo", work, "-worker", "cloud/run", "-head", sync)
	if rs, err := queue.LoadRecords(dir); err != nil || len(rs) != 0 {
		t.Fatalf("records after a refusal: %+v, %v", rs, err)
	}

	work, branch, sync := syncRepo(t, "shared.txt", "shared.txt", "mp\n")
	got := must(t, dir, "sync-program", "-repo", work, "-worker", "cloud/run", "-head", sync)
	if !strings.HasPrefix(got, "recorded\tsync\to/here\t"+branch+"\t"+sync[:12]+"\n") {
		t.Fatalf("wf sync-program -head of an ordinary resolution = %q", got)
	}
}

// TestSyncShapeCommand: wf sync-shape exits 0 on a sync, 1 on a head that is
// not one or whose resolution is guarded, each condition printed, and 2 on a
// usage or git error (FLOW-01, UX-04, Decision 2 of sync-pr-tier1).
func TestSyncShapeCommand(t *testing.T) {
	args := func(work, branch, head string) []string {
		return []string{"-repo", work, "-base", "origin/program/componentization",
			"-branch", branch, "-head", head, "-trunk", "origin/main",
			"-profile", "docs/development/loop-profile.yaml"}
	}
	code := func(err error) int {
		var xe *exitError
		switch {
		case err == nil:
			return 0
		case errors.As(err, &xe):
			return xe.code
		}
		return 1
	}
	work, branch, sync := syncRepo(t, "shared.txt", "shared.txt", "mp\n")
	out, err := wf(t, ".", "sync-shape", args(work, branch, sync)...)
	if code(err) != 0 || !strings.HasPrefix(out, "sync "+sync[:12]+" merges ") ||
		!strings.Contains(out, "resolves shared.txt") {
		t.Fatalf("a sync: %q, %v", out, err)
	}
	out, err = wf(t, ".", "sync-shape", args(work, "feat/x", sync)...)
	if code(err) != 1 || !strings.Contains(out, "not a sync: the branch feat/x") ||
		!strings.Contains(err.Error(), "judged as any PR") {
		t.Fatalf("another branch: %q, %v", out, err)
	}
	if _, err = wf(t, ".", "sync-shape", args(work, branch,
		strings.Repeat("d", 40))...); code(err) != 2 {
		t.Fatalf("an unknown head: %v, want exit 2", err)
	}
	if _, err = wf(t, ".", "sync-shape", "-repo", work); code(err) != 2 {
		t.Fatalf("no flags: %v, want exit 2", err)
	}

	work, branch, sync = syncRepo(t, ".claude/a.md", ".claude/a.md", "mp\n")
	out, err = wf(t, ".", "sync-shape", args(work, branch, sync)...)
	if code(err) != 1 || !strings.Contains(out,
		"not a sync: the resolution edits .claude/a.md, which FLOW-14 guards") {
		t.Fatalf("a guarded resolution: %q, %v", out, err)
	}
}

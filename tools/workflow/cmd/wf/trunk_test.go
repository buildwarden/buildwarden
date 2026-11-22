package main

import (
	"os"
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

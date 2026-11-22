package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// loopMerge merges a branch that writes the file into the program trunk as
// a merge commit, pushes it and fetches.
func loopMerge(t *testing.T, work, name, file string) {
	t.Helper()
	git(t, work, "checkout", "-q", "-b", name, "origin/"+programTrunk)
	full := filepath.Join(work, filepath.FromSlash(file))
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	_ = os.WriteFile(full, []byte(name+"\n"), 0o644)
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", name)
	git(t, work, "checkout", "-q", "-B", "pt", "origin/"+programTrunk)
	git(t, work, "merge", "-q", "--no-ff", "-m", "Merge pull request #1 from o/"+name, name)
	git(t, work, "push", "-q", "origin", "pt:"+programTrunk)
	git(t, work, "fetch", "-q", "origin")
}

// TestTagNext: the next pre-release is the next minor's -dev.N after the
// latest release without a suffix, counting up; a merge that changes no
// consumable gets none, and neither does a head whose checks failed
// (FLOW-20, Decision 15).
func TestTagNext(t *testing.T) {
	g := gateFixture(t, design, nil)
	git(t, g.work, "tag", "v0.6.0", "origin/main")
	loopMerge(t, g.work, "relay-a", "relay/a.go")
	tags, why, err := TagNext(g.env, hereRepo)
	if err != nil || len(tags) != 1 || tags[0] != "v0.7.0-dev.1" {
		t.Fatalf("TagNext = %v, %q, %v; want v0.7.0-dev.1", tags, why, err)
	}
	git(t, g.work, "tag", tags[0], "origin/"+programTrunk)
	if tags, why, _ := TagNext(g.env, hereRepo); len(tags) != 0 ||
		!strings.Contains(why, "is tagged") {
		t.Fatalf("a tagged head: %v, %q", tags, why)
	}
	loopMerge(t, g.work, "relay-b", "relay/b.go")
	if tags, _, _ := TagNext(g.env, hereRepo); len(tags) != 1 ||
		tags[0] != "v0.7.0-dev.2" {
		t.Fatalf("after a second merge: %v", tags)
	}
	g.gh.runs[0].Conclusion = "failure"
	if tags, why, _ := TagNext(g.env, hereRepo); len(tags) != 0 ||
		!strings.Contains(why, "not green") {
		t.Fatalf("a failed check: %v, %q", tags, why)
	}
	g.gh.runs[0].Conclusion = "success"
	loopMerge(t, g.work, "docs-c", "docs/guide.md")
	if tags, why, _ := TagNext(g.env, hereRepo); len(tags) != 0 ||
		!strings.Contains(why, "no consumable") {
		t.Fatalf("a docs-only merge: %v, %q", tags, why)
	}
}

// TestNextDev: with no release the series starts at v0.1.0; a specification
// counts its own tags; the harness is tagged first.
func TestNextDev(t *testing.T) {
	if got := nextDev(nil, ""); got != "v0.1.0-dev.1" {
		t.Errorf("no release: %s", got)
	}
	tags := []string{"relay-claim/v0.1.0", "relay-claim/v0.2.0-dev.3", "v1.0.0"}
	if got := nextDev(tags, "relay-claim/"); got != "relay-claim/v0.2.0-dev.4" {
		t.Errorf("a specification: %s", got)
	}
	got := consumables(specsRepo, []string{"relay-claim/spec.go", "conformance/x.go",
		"README.md"})
	if strings.Join(got, " ") != "conformance/ relay-claim/" {
		t.Errorf("consumables = %v, want the harness first", got)
	}
}

// TestTagNextWaitsForTheSpecsWorkflow: nothing is proposed in the
// specifications repository until specs-prerelease-tags is done.
func TestTagNextWaitsForTheSpecsWorkflow(t *testing.T) {
	s := hereState()
	s.Config.Repositories = append(s.Config.Repositories, queue.RepositoryEntry{
		Name: specsRepo, Trunk: "main", ProgramTrunk: programTrunk})
	tags, why, err := TagNext(Env{State: s}, specsRepo)
	if err != nil || len(tags) != 0 || !strings.Contains(why, specsTagsItem) {
		t.Fatalf("TagNext = %v, %q, %v", tags, why, err)
	}
}

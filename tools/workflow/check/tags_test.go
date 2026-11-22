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
	if got := nextPre(nil, "", "dev"); got != "v0.1.0-dev.1" {
		t.Errorf("no release: %s", got)
	}
	tags := []string{"relay-claim/v0.1.0", "relay-claim/v0.2.0-dev.3", "v1.0.0"}
	if got := nextPre(tags, "relay-claim/", "dev"); got != "relay-claim/v0.2.0-dev.4" {
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

// rcFixture is a program item, demo, whose pull request merged into the
// program trunk with a go.mod that pins the relay-claim specification at
// v0.2.0-dev.1, the latest tag in the specifications repository's clone.
func rcFixture(t *testing.T) *gated {
	t.Helper()
	g := gateFixture(t, design, func(work string) {
		p := filepath.Join(work, "tools", "conformance", "relay-claim", "go.mod")
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		mod := "module github.com/o/here/tools/conformance/relay-claim\n\n" +
			"go 1.26\n\nrequire (\n" +
			"\tgithub.com/buildwarden/buildwarden-specs/relay-claim v0.2.0-dev.1" +
			" // indirect\n" +
			"\tgithub.com/other/thing v1.0.0\n)\n"
		_ = os.WriteFile(p, []byte(mod), 0o644)
		git(t, work, "add", ".")
		git(t, work, "commit", "-q", "-m", "pin")
	})
	g.head = fullHead(t, g.work, "origin/feat/demo")
	g.f.PR.Head = g.head
	g.f.Status = "done"
	git(t, g.work, "tag", "v0.6.0", "origin/main")
	git(t, g.work, "checkout", "-q", "-B", "pt", "origin/"+programTrunk)
	git(t, g.work, "merge", "-q", "--no-ff", "-m", "Merge pull request #42 from o/feat/demo",
		"feat/demo")
	git(t, g.work, "push", "-q", "origin", "pt:"+programTrunk)
	git(t, g.work, "fetch", "-q", "origin")
	g.env.Checkouts = t.TempDir()
	specs := filepath.Join(g.env.Checkouts, "buildwarden", "buildwarden-specs")
	_ = os.MkdirAll(specs, 0o755)
	git(t, specs, "init", "-q", "-b", "main")
	git(t, specs, "commit", "-q", "--allow-empty", "-m", "specs")
	for _, tag := range []string{"relay-claim/v0.1.0", "relay-claim/v0.2.0-dev.1"} {
		git(t, specs, "tag", tag)
	}
	g.env.State.Config.Repositories = append(g.env.State.Config.Repositories,
		queue.RepositoryEntry{Name: specsRepo, Trunk: "main"})
	return g
}

// TestTagNextRC: once the component's last phase has merged on the program
// trunk, its checks are green and it pins the latest pre-release of what it
// consumes, the next tag is the next minor's -rc.N after the latest release
// without a suffix, counting up; an open program item in the repository, a
// pin behind the latest pre-release, a failed check, a tagged head or a last
// phase not merged gets none (FLOW-20, a17 (a)).
func TestTagNextRC(t *testing.T) {
	g := rcFixture(t)
	tags, why, err := TagNextRC(g.env, hereRepo, "demo")
	if err != nil || len(tags) != 1 || tags[0] != "v0.7.0-rc.1" {
		t.Fatalf("TagNextRC = %v, %q, %v; want v0.7.0-rc.1", tags, why, err)
	}
	git(t, g.work, "tag", "v0.7.0-rc.1", "origin/"+programTrunk)
	if tags, why, _ := TagNextRC(g.env, hereRepo, "demo"); len(tags) != 0 ||
		!strings.Contains(why, "is tagged") {
		t.Fatalf("a tagged head: %v, %q", tags, why)
	}
	loopMerge(t, g.work, "relay-b", "relay/b.go")
	if tags, _, _ := TagNextRC(g.env, hereRepo, "demo"); len(tags) != 1 ||
		tags[0] != "v0.7.0-rc.2" {
		t.Fatalf("after a second merge: %v", tags)
	}
	if _, _, err := TagNextRC(g.env, hereRepo, ""); err == nil {
		t.Error("an -rc with no item of the last phase was proposed")
	}

	for name, c := range map[string]struct {
		change func(t *testing.T, g *gated)
		why    string
	}{
		"an open program item": {func(_ *testing.T, g *gated) {
			g.env.State.Features = append(g.env.State.Features, &queue.Feature{
				Feature: "next-phase", Program: true, Status: "queued"})
		}, "still open"},
		"a pin behind the latest pre-release": {func(t *testing.T, g *gated) {
			git(t, filepath.Join(g.env.Checkouts, "buildwarden", "buildwarden-specs"),
				"tag", "relay-claim/v0.2.0-rc.1")
		}, "latest pre-releases"},
		"a failed check": {func(_ *testing.T, g *gated) {
			g.gh.runs[0].Conclusion = "failure"
		}, "not green"},
		"the last phase not merged": {func(_ *testing.T, g *gated) {
			g.f.PR.Head = strings.Repeat("d", 40)
		}, "has not merged"},
	} {
		g := rcFixture(t)
		c.change(t, g)
		tags, why, err := TagNextRC(g.env, hereRepo, "demo")
		if len(tags) != 0 || (!strings.Contains(why, c.why) &&
			(err == nil || !strings.Contains(err.Error(), c.why))) {
			t.Errorf("%s: %v, %q, %v; want none, %q", name, tags, why, err, c.why)
		}
	}
}

// TestVersionOrder: a release orders after its -rc, an -rc after its -dev,
// and the next minor's -dev after both; the specifications directories with
// a -dev tag of their next version take an -rc, the harness first.
func TestVersionOrder(t *testing.T) {
	order := []string{"v0.1.0", "v0.2.0-dev.1", "v0.2.0-dev.10", "v0.2.0-rc.1", "v0.2.0"}
	for i := 1; i < len(order); i++ {
		a, _ := parseVersion(order[i-1])
		b, ok := parseVersion(order[i])
		if !ok || a.compare(b) >= 0 {
			t.Errorf("%s does not order before %s", order[i-1], order[i])
		}
	}
	tags := []string{"relay-claim/v0.1.0", "relay-claim/v0.2.0-dev.2", "collector/v0.1.0",
		"conformance/v0.1.0-dev.1", "collector/v0.1.0-dev.1"}
	if got := strings.Join(devDirs(tags), " "); got != "conformance/ relay-claim/" {
		t.Errorf("devDirs = %s", got)
	}
	if got := nextPre(tags, "relay-claim/", "rc"); got != "relay-claim/v0.2.0-rc.1" {
		t.Errorf("nextPre rc = %s", got)
	}
}

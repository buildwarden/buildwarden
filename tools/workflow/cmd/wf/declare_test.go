package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

const mapRepo = "buildwarden/buildwarden"

// mappedDir is a state checkout whose areas.yaml is the loop-parallel
// design's map and whose config names its repository.
func mappedDir(t *testing.T) string {
	t.Helper()
	dir := stateDir(t, "repository: "+mapRepo+"\n")
	data, err := os.ReadFile(filepath.Join("..", "..", "queue", "testdata", "areas.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "areas.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// designText is a design for the item whose front matter declares the
// drivers and the areas, with its review required.
func designText(name, drivers, areas, review string) string {
	return "---\nfeature: " + name + "\nstatus: draft\ndesign_review: required\n" +
		"drivers: " + drivers + "\nseams: [ledger-format]\nareas: " + areas +
		"\nsurfaces: [cli]\nsecurity_boundary: none\nalters_relationships: false\n" +
		"---\n\n# " + name + "\n" + review
}

// writeDesign commits the text as the item's design on its branch, pushes,
// and returns the commit.
func writeDesign(t *testing.T, work, name, text string) string {
	t.Helper()
	gitT(t, work, "checkout", "-q", "feat/"+name)
	dir := filepath.Join(work, "docs", "design")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{"TEMPLATE.md": "---\nfeature: x\n---\n",
		name + ".md": text} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, work, "add", ".")
	gitT(t, work, "commit", "-q", "-m", "design")
	gitT(t, work, "push", "-q", "origin", "HEAD")
	return gitT(t, work, "rev-parse", "--short", "HEAD")
}

// TestDeclareCommand: wf declare copies the design on the item's branch into
// the queue file and prints the history line; once Design is done an agent
// may only widen inside the design as approved, read at the approved commit,
// and a human may declare any value (FLOW-02, FLOW-06).
func TestDeclareCommand(t *testing.T) {
	work, dir := codeRepo(t, "a"), mappedDir(t)
	queued(t, dir, "a", "1", "-areas", "driver/hyperv")
	edit(t, dir, "a", func(f *queue.Feature) { f.Status, f.Phase = "in_progress", 1 })
	approved := writeDesign(t, work, "a", designText("a", "[hyperv]", "[driver/hyperv]", ""))
	declare := func(worker string) []string {
		return []string{"-repo", work, "-worker", worker, "-feature", "a"}
	}
	refused(t, dir, "wf declare needs -worker", "declare", "-repo", work, "-feature", "a")
	out := must(t, dir, "declare", declare("cloud/run")...)
	const line = "a\tdeclared at " + "%s: seams +ledger-format; alters_relationships " +
		"unset -> false; design_review unset -> required\n"
	if want := strings.Replace(line, "%s", approved, 1); out != want {
		t.Fatalf("wf declare printed %q, want %q", out, want)
	}
	f := item(t, dir, "a")
	if f.Declared != approved || !slices.Equal(*f.Areas, []string{"driver/hyperv"}) ||
		!slices.Equal(f.Seams, []string{"ledger-format"}) || f.DesignReview != "required" {
		t.Fatalf("the queue file after wf declare: %+v", f)
	}
	must(t, dir, "validate")

	// In Implement: an agent adds a driver and its area on the branch. The
	// approved design has neither, so the widening is a design change; cli,
	// which its surfaces imply, is within it.
	review := "\n## Design review\n\n- r1: approved at `" + approved + "`.\n"
	edit(t, dir, "a", func(f *queue.Feature) { f.Phase = 3 })
	writeDesign(t, work, "a", designText("a", "[hyperv, vz]", "[driver/hyperv, driver/vz]",
		review))
	refused(t, dir, "area driver/vz is outside the approved design (at "+approved+")",
		"declare", declare("cloud/run")...)
	refused(t, dir, "a design change: back to Design (FLOW-06)", "declare",
		declare("host:macos-vz/run")...)
	if f := item(t, dir, "a"); f.Declared != approved || len(*f.Areas) != 1 {
		t.Fatalf("a refused wf declare wrote %+v", f)
	}
	head := writeDesign(t, work, "a", designText("a", "[hyperv]", "[driver/hyperv, cli, book]",
		review))
	out = must(t, dir, "declare", declare("cloud/run")...)
	if want := "a\tdeclared at " + head + ": areas +cli (within surfaces: cli) " +
		"+book (within surfaces: cli)\n"; out != want {
		t.Fatalf("wf declare printed %q, want %q", out, want)
	}
	// A human may narrow after Design, and declare what an agent may not.
	head = writeDesign(t, work, "a", designText("a", "[hyperv, vz]", "[driver/vz]", review))
	out = must(t, dir, "declare", declare("human/jeff")...)
	if want := "a\tdeclared at " + head +
		": areas +driver/vz -driver/hyperv -cli -book\n"; out != want {
		t.Fatalf("a human's wf declare printed %q, want %q", out, want)
	}
	must(t, dir, "validate")
}

// TestRelateAndRelations: wf relate is a human's, and wf relations prints
// each pair of open items that collide or are near, with the reason
// (FLOW-05).
func TestRelateAndRelations(t *testing.T) {
	dir := mappedDir(t)
	queued(t, dir, "a", "1", "-areas", "relay")
	queued(t, dir, "b", "2", "-areas", "driver/vz")
	queued(t, dir, "c", "3", "-areas", "driver-core")
	queued(t, dir, "u", "4")
	must(t, dir, "new", "-worker", "human/jeff", "-feature", "x", "-title", "X", "-priority",
		"5", "-estimate-steps", "9", "-kind", "process")
	const pairs = "a, c: near (driver-core reaches relay)\n" +
		"a, u: colliding (u declares no areas)\n" +
		"b, c: near (driver/vz reaches driver-core)\n" +
		"b, u: colliding (u declares no areas)\n" +
		"c, u: colliding (u declares no areas)\n"
	if got := must(t, dir, "relations"); got != pairs {
		t.Fatalf("wf relations = %q, want %q", got, pairs)
	}
	if got := must(t, dir, "relations", "-feature", "c"); got !=
		"c, a: near (driver-core reaches relay)\n"+
			"c, b: near (driver/vz reaches driver-core)\n"+
			"c, u: colliding (u declares no areas)\n" {
		t.Fatalf("wf relations -feature c = %q", got)
	}
	if got := must(t, dir, "relations", "-feature", "x"); got !=
		"x is a process item: outside the relations (FLOW-05)\n" {
		t.Fatalf("wf relations -feature x = %q", got)
	}
	relate := func(worker string, more ...string) []string {
		return append([]string{"-worker", worker, "-feature", "a", "-with", "b"}, more...)
	}
	refused(t, dir, "only a human names a collision (FLOW-05)", "relate",
		relate("cloud/run")...)
	refused(t, dir, "relate needs -with", "relate", "-worker", "human/jeff", "-feature", "a")
	must(t, dir, "relate", relate("human/jeff")...)
	if got := must(t, dir, "relations", "-feature", "b"); !strings.HasPrefix(got,
		"b, a: colliding (named by the human)\n") {
		t.Fatalf("wf relations -feature b after wf relate = %q", got)
	}
	if got := item(t, dir, "a").CollidesWith; !slices.Equal(got, []string{"b"}) {
		t.Fatalf("collides_with %v", got)
	}
	must(t, dir, "validate")
	must(t, dir, "relate", relate("human/jeff", "-clear")...)
	if got := must(t, dir, "relations"); got != pairs {
		t.Fatalf("wf relations after -clear = %q", got)
	}
	lone := mappedDir(t)
	queued(t, lone, "a", "1", "-areas", "relay")
	queued(t, lone, "b", "2", "-areas", "driver/vz")
	if got := must(t, lone, "relations"); got != "none: every pair is disjoint\n" {
		t.Fatalf("wf relations with disjoint items = %q", got)
	}
}

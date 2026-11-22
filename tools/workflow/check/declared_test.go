package check

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// mapped is the queue state the checks read: the loop-parallel design's area
// map for its repository, and the items.
func mapped(t *testing.T, fs ...*queue.Feature) *queue.State {
	t.Helper()
	m, err := queue.LoadAreas(filepath.Join("..", "queue", "testdata", "areas.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := &queue.State{Areas: m, Features: fs}
	s.Config.Repository = m.Repository
	s.Config.Repositories = []queue.RepositoryEntry{{Name: m.Repository, Trunk: "main",
		Profile: profilePath}}
	return s
}

// front is the fixture design with the front-matter line replaced.
func front(text, from, to string) string {
	if !strings.Contains(text, from) {
		panic("the fixture design has no " + from)
	}
	return strings.Replace(text, from, to, 1)
}

// withAreas is the fixture design declaring the areas.
func withAreas(text, areas string) string {
	return front(text, "seams: []", "seams: []\nareas: "+areas)
}

// queueAreas records the areas in the item's queue file.
func queueAreas(f *queue.Feature, areas ...string) *queue.Feature {
	f.Areas = &areas
	return f
}

// result is the first result with the name.
func result(t *testing.T, rs []Result, name string) Result {
	t.Helper()
	for _, r := range rs {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no result %q among %+v", name, rs)
	return Result{}
}

const (
	recorded = "declared recorded in queue (FLOW-02)"
	matches  = "queue records what the design declares (FLOW-03)"
	implied  = "areas cover what the design implies (FLOW-02)"
	inMap    = "declared areas are in the area map (FLOW-05)"
)

// TestDeclaredAtDesign: Design is done only when the queue records what the
// design declares, and its areas cover what its other fields imply; an area
// the map lacks is reported and does not fail it (FLOW-02, FLOW-03).
func TestDeclaredAtDesign(t *testing.T) {
	declared := withAreas(design, "[driver/container, book]")
	work := repo(t, declared)
	f := queueAreas(demo(1), "driver/container", "book")
	env := Env{Repo: work, State: mapped(t, f)}
	if rs := Phase(env, f); !OK(rs) {
		t.Fatalf("a declared design: %s", failures(rs))
	}
	f.Declared = ""
	if got := failures(Phase(env, f)); !strings.Contains(got, recorded) {
		t.Errorf("no declared: %q", got)
	}
	f.Declared = "0a1b2c3"
	for name, change := range map[string]func(f *queue.Feature){
		"areas differ": func(f *queue.Feature) { queueAreas(f, "book") },
		"undeclared":   func(f *queue.Feature) { f.Areas = nil },
		"a repository": func(f *queue.Feature) {
			f.Repos = []string{"buildwarden/buildwarden", "o/elsewhere"}
		},
		"a seam":         func(f *queue.Feature) { f.Seams = []string{"relay-claim"} },
		"a rule changed": func(f *queue.Feature) { f.RulesChanged = []string{"TEST-09"} },
		// The design's related lists no componentization.md (FLOW-20).
		"a program mark": func(f *queue.Feature) { f.Program = true },
	} {
		g := queueAreas(demo(1), "driver/container", "book")
		change(g)
		if got := failures(Phase(env, g)); !strings.Contains(got, matches) {
			t.Errorf("%s: failures %q, want %q", name, got, matches)
		}
	}

	// A design that lists componentization.md in related makes a program
	// item, and the queue must say so (FLOW-20).
	commit(t, work, front(declared, "seams: []", "seams: []\nrelated: [componentization.md]"))
	g := queueAreas(demo(1), "driver/container", "book")
	if got := failures(Phase(env, g)); !strings.Contains(got, "program (FLOW-20") {
		t.Errorf("program mark missing: failures %q", got)
	}
	g.Program = true
	if rs := Phase(env, g); !OK(rs) {
		t.Errorf("a program item marked: %s", failures(rs))
	}

	// An undeclared design skips the implied areas, and passes.
	commit(t, work, design)
	u := demo(1)
	rs := Phase(env, u)
	if !OK(rs) || !strings.Contains(result(t, rs, implied).Info, "undeclared") {
		t.Errorf("an undeclared design: %s; %+v", failures(rs), rs)
	}
	// An area the map lacks is reported, and the step does not fail on it.
	commit(t, work, withAreas(design, "[driver/container, book, telemetry]"))
	g = queueAreas(demo(1), "driver/container", "book", "telemetry")
	rs = Phase(env, g)
	if !OK(rs) || !strings.Contains(result(t, rs, inMap).Info,
		"telemetry is not in the area map") {
		t.Errorf("an area the map lacks: %s; %+v", failures(rs), rs)
	}
	// The driver's area is not declared, and no shared area is.
	commit(t, work, withAreas(design, "[book]"))
	if got := failures(Phase(env, queueAreas(demo(1), "book"))); !strings.Contains(got,
		"driver container: neither its area driver/container nor a shared area") {
		t.Errorf("drivers: [container] with areas: [book]: %q", got)
	}
}

// TestImpliedAreas: each rule of the implied areas, on front matter alone;
// two of the fixtures are merged designs'.
func TestImpliedAreas(t *testing.T) {
	m := mapped(t).Areas
	all := []string{"container", "qemu", "vz", "hyperv", "firecracker"}
	rule := []RuleChange{{Rule: "TEST-09", Change: "add",
		Paths: []string{"docs/development/testing.md", "tools/tier1/rules.yaml"}}}
	for _, tc := range []struct {
		name  string
		fm    Front
		areas []string
		want  []string
	}{
		{"relay-leaf-crl", Front{Drivers: all[:4]}, []string{"relay", "book"}, nil},
		{"http-body-path-drift", Front{Drivers: all}, []string{"relay", "specs"}, nil},
		{"a driver without its area", Front{Drivers: []string{"hyperv"}}, []string{"book"},
			[]string{"driver hyperv: neither its area driver/hyperv nor a shared " +
				"area is declared"}},
		{"a driver with its area", Front{Drivers: []string{"hyperv"}},
			[]string{"driver/hyperv"}, nil},
		{"a driver the map lacks", Front{Drivers: []string{"wsl"}}, []string{"book"},
			[]string{"driver wsl: the area map gives it no area (implies.drivers), " +
				"and no shared area is declared"}},
		{"interface with a shared area", Front{Drivers: []string{"interface"}},
			[]string{"driver-core"}, nil},
		{"interface with every driver", Front{Drivers: []string{"interface"}},
			[]string{"driver/container", "driver/qemu", "driver/vz", "driver/hyperv",
				"driver/firecracker"}, nil},
		{"interface with neither", Front{Drivers: []string{"interface"}},
			[]string{"driver/qemu", "book"},
			[]string{"drivers interface: neither a shared area nor every driver's " +
				"area is declared"}},
		{"a surface", Front{Surfaces: []string{"cli"}}, []string{"cli"},
			[]string{"surface cli: its area book is not declared"}},
		{"a surface covered", Front{Surfaces: []string{"cli", "output"}},
			[]string{"cli", "book"}, nil},
		{"a surface the map lacks", Front{Surfaces: []string{"api"}}, []string{}, nil},
		{"a rule change's paths", Front{RuleChanges: rule}, []string{"rules"},
			[]string{"rule_changes TEST-09: tools/tier1/rules.yaml is in area " +
				"tier1-config, which is not declared"}},
		{"a rule change covered", Front{RuleChanges: rule},
			[]string{"rules", "tier1-config"}, nil},
		{"nothing implied", Front{}, []string{}, nil},
	} {
		tc.fm.Areas = &tc.areas
		if got := impliedAreas(&tc.fm, m); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestQueueMatchesDesignLater: from Plan on each done-when compares the queue
// with the design, for an item the queue records a declaration for; an item
// that finished Design before wf declare passes as it did, with a line that
// says so (FLOW-03; case 24 of the loop-parallel design).
func TestQueueMatchesDesignLater(t *testing.T) {
	seams := front(design, "seams: []", "seams: [relay-claim, collector]")
	seams = front(seams, "design_review: skip", "design_review: required")
	work := repo(t, seams)
	commit(t, work, seams+"\n## Design review\n\n- r1: approved at `"+
		commit(t, work, seams+"\n<!-- as approved -->\n")+"`.\n")
	// As seam-specs and driver-contract are today: no seams, no declared.
	old := func(phase int) *queue.Feature {
		f := demo(phase)
		f.Declared, f.DesignReview = "", "required"
		f.Steps = []queue.Step{{ID: "f2-design-review", Phase: 2, Status: "done"}}
		return f
	}
	env := here(work)
	for _, phase := range []int{2, 3, 4, 5, 6} {
		rs := Phase(env, old(phase))
		if r := result(t, rs, matches); !r.OK ||
			r.Info != "not declared: finished Design before wf declare" {
			t.Errorf("phase %d, never declared: %+v", phase, r)
		}
		f := old(phase)
		f.Declared = "0a1b2c3"
		r := result(t, Phase(env, f), matches)
		if r.OK || !strings.Contains(r.Info, "seams: design [relay-claim collector], "+
			"queue []") {
			t.Errorf("phase %d, declared with the seams missing: %+v", phase, r)
		}
		f.Seams = []string{"collector", "relay-claim"}
		if r := result(t, Phase(env, f), matches); !r.OK {
			t.Errorf("phase %d, declared and equal: %+v", phase, r)
		}
		// alters_relationships written back in the queue alone.
		yes := true
		f.AltersRelationships = &yes
		r = result(t, Phase(env, f), matches)
		if r.OK || !strings.Contains(r.Info, "alters_relationships: design false, "+
			"queue true") {
			t.Errorf("phase %d, alters_relationships written back: %+v", phase, r)
		}
	}
	if rs := Phase(env, old(2)); !OK(rs) {
		t.Errorf("the Plan gate of an item never declared: %s", failures(rs))
	}
}

// TestImpactDeltaAreas: repos and areas are impact fields, so the Plan gate
// fails on a change to either since the approval (FLOW-19).
func TestImpactDeltaAreas(t *testing.T) {
	areas := func(names ...string) *[]string { return &names }
	for _, tc := range []struct {
		name     string
		was, now Front
		want     string
	}{
		{"unchanged", Front{Areas: areas("relay", "book")},
			Front{Areas: areas("book", "relay")}, ""},
		{"an area added", Front{Areas: areas("relay")},
			Front{Areas: areas("relay", "cli")}, "areas [relay] -> [relay cli]"},
		{"declared", Front{}, Front{Areas: areas()}, "areas undeclared -> []"},
		{"undeclared", Front{Areas: areas("relay")}, Front{},
			"areas [relay] -> undeclared"},
		{"a repository", Front{}, Front{Repos: []string{"o/here", "o/specs"}},
			"repos [] -> [o/here o/specs]"},
		{"another home", Front{Repos: []string{"o/here", "o/specs"}},
			Front{Repos: []string{"o/specs", "o/here"}},
			"repos [o/here o/specs] -> [o/specs o/here]"},
	} {
		tc.was.SecurityBoundary, tc.now.SecurityBoundary = "none", "none"
		if got := impactDelta(&tc.was, &tc.now); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}

	req := front(withAreas(design, "[driver/container, book]"), "design_review: skip",
		"design_review: required")
	work := repo(t, req)
	f := queueAreas(demo(2), "driver/container", "book", "cli")
	f.DesignReview = "required"
	f.Steps = []queue.Step{{ID: "f2-design-review", Phase: 2, Status: "done"}}
	env := Env{Repo: work, State: mapped(t, f)}
	approved := commit(t, work, req+"\n<!-- as approved -->\n")
	review := "\n## Design review\n\n- r1: approved at `" + approved + "`.\n"
	commit(t, work, front(req, "[driver/container, book]", "[driver/container, book, cli]")+
		review)
	if got := failures(Phase(env, f)); !strings.Contains(got,
		"areas [driver/container book] -> [driver/container book cli]") {
		t.Errorf("the Plan gate with areas changed since the approval: %q", got)
	}
}

// TestSkipNarrowing: a design that declares fewer repositories or areas than
// intake recorded, or areas where intake recorded none, may not skip its
// review (FLOW-19).
func TestSkipNarrowing(t *testing.T) {
	const skip = "design review skip is valid (FLOW-19)"
	work := repo(t, withAreas(design, "[driver/container, book]"))
	for _, tc := range []struct {
		name   string
		intake func(f *queue.Feature)
		want   string
	}{
		{"as intake recorded", func(f *queue.Feature) {
			queueAreas(f, "driver/container", "book")
		}, ""},
		{"wider than intake", func(f *queue.Feature) { queueAreas(f, "book") }, ""},
		{"an area removed", func(f *queue.Feature) {
			queueAreas(f, "driver/container", "book", "cli")
		}, "it removes the area cli: narrowing needs the design review"},
		{"areas where intake recorded none", func(*queue.Feature) {},
			"it declares areas where the queue records none: narrowing needs the " +
				"design review"},
		{"a repository removed", func(f *queue.Feature) {
			queueAreas(f, "driver/container", "book")
			f.Repos = []string{"buildwarden/buildwarden", "o/specs"}
		}, "it removes the repository o/specs: narrowing needs the design review"},
	} {
		f := demo(1)
		tc.intake(f)
		r := result(t, Phase(Env{Repo: work, State: mapped(t, f)}, f), skip)
		if r.OK != (tc.want == "") || !strings.Contains(r.Info, tc.want) {
			t.Errorf("%s: %+v, want %q", tc.name, r, tc.want)
		}
	}
}

// TestDeclarations: wf declare reads what the design declares on the branch,
// and what a widening is judged in at the commit its last approval names; a
// design whose review was skipped is read on the branch.
func TestDeclarations(t *testing.T) {
	ruled := front(withAreas(design, "[driver/container, book]"), "concepts: [inputs]",
		"concepts: [inputs]\nrepos: [o/here, o/specs]\nrule_changes:\n"+
			"  - {rule: TEST-09, change: add, paths: [docs/development/testing.md]}")
	work := repo(t, ruled)
	env, f := here(work), demo(1)
	d, err := Declarations(env, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Commit) < 7 || !strings.HasPrefix(fullHead(t, work, "feat/demo"), d.Commit) {
		t.Fatalf("the commit read: %q", d.Commit)
	}
	if !slices.Equal(*d.Areas, []string{"driver/container", "book"}) ||
		!slices.Equal(d.Repos, []string{"o/here", "o/specs"}) ||
		!slices.Equal(d.RulesChanged, []string{"TEST-09"}) || len(d.Seams) != 0 {
		t.Fatalf("the declaration read: %+v", d)
	}
	if d.Alters == nil || *d.Alters || d.DesignReview != "skip" {
		t.Fatalf("the declaration read: %+v", d)
	}
	want := queue.Within{OnBranch: true, Areas: d.Areas, Drivers: []string{"container"},
		Surfaces: []string{"output"}, RulePaths: []string{"docs/development/testing.md"}}
	if !reflect.DeepEqual(d.Within, want) {
		t.Fatalf("a skipped review is read on the branch: %+v", d.Within)
	}

	declarationsApproved(t, work, ruled)
}

// declarationsApproved is the second half of TestDeclarations: an agent adds
// a driver on the branch after the approval, and the widening is judged on
// the approved design, which does not have it.
func declarationsApproved(t *testing.T, work, ruled string) {
	t.Helper()
	env, f := here(work), demo(1)
	req := front(ruled, "design_review: skip", "design_review: required")
	approved := commit(t, work, req)
	review := "\n## Design review\n\n- r1: approved at `" + approved + "`.\n"
	commit(t, work, front(front(req, "drivers: [container]", "drivers: [container, vz]"),
		"[driver/container, book]", "[driver/container, book, driver/vz]")+review)
	d, err := Declarations(env, f)
	if err != nil {
		t.Fatal(err)
	}
	if w := d.Within; w.OnBranch || w.Commit != approved ||
		!slices.Equal(w.Drivers, []string{"container"}) ||
		!slices.Equal(*w.Areas, []string{"driver/container", "book"}) {
		t.Fatalf("the approved design: %+v", w)
	}
	if !slices.Contains(*d.Areas, "driver/vz") {
		t.Fatalf("the branch's areas: %v", *d.Areas)
	}
	// With neither an approval nor a skip there is nothing to judge in.
	commit(t, work, req)
	d, err = Declarations(env, f)
	if err != nil || d.Within.OnBranch || d.Within.Commit != "" {
		t.Fatalf("a required review with no approval: %+v, %v", d.Within, err)
	}
}

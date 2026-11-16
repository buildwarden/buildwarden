package queue

import (
	"slices"
	"strings"
	"testing"
)

const commit = "4c1d9e2"

// declaration is what a design declares: the areas, with review "required"
// and no seam, rule change or repository of its own.
func declaration(areas ...string) Declaration {
	no := false
	d := Declaration{Commit: commit, Alters: &no, DesignReview: "required"}
	if areas != nil {
		d.Areas = &areas
	}
	return d
}

// designing is a feature in Design that intake recorded the areas for.
func designing(name string, prio int, areas ...string) *Feature {
	f := working(name, prio, 1, areas...)
	f.DesignReview = ""
	return f
}

// wantDeclared asserts that the declaration is accepted for the worker, and
// that the queue file then records it.
func wantDeclared(t *testing.T, s *State, f *Feature, d Declaration, worker string) {
	t.Helper()
	if err := s.Declare(f, d, worker, now); err != nil {
		t.Fatalf("%s: wf declare by %s: %v", f.Feature, worker, err)
	}
	if f.Declared != d.Commit || !sameAreas(f.Areas, d.Areas) {
		t.Fatalf("%s: declared %q, areas %v", f.Feature, f.Declared, f.Areas)
	}
	wantValidate(t, s)
}

// wantUndeclared asserts that the declaration is refused with the message
// and that nothing changed.
func wantUndeclared(t *testing.T, s *State, f *Feature, d Declaration, worker, why string) {
	t.Helper()
	before := *f
	history := len(f.History)
	err := s.Declare(f, d, worker, now)
	if err == nil || !strings.Contains(err.Error(), why) {
		t.Fatalf("%s: wf declare by %s: %v, want %q", f.Feature, worker, err, why)
	}
	if f.Declared != before.Declared || !sameAreas(f.Areas, before.Areas) ||
		!slices.Equal(f.Repos, before.Repos) || !slices.Equal(f.Seams, before.Seams) ||
		len(f.History) != history {
		t.Fatalf("%s: a refused wf declare changed the item: %+v", f.Feature, f)
	}
}

func lastLog(f *Feature) string { return f.History[len(f.History)-1].Event }

// TestDeclare: wf declare copies what the design declares into the queue
// file, and what it accepts depends on where the item is and who asks
// (FLOW-02, FLOW-06, FLOW-19): each row of the design's table.
func TestDeclare(t *testing.T) {
	t.Run("in Design", declareInDesign)
	t.Run("in Plan", declareInPlan)
	t.Run("widening", declareWidening)
	t.Run("over a limit", declareOverLimit)
	t.Run("a human", declareByHuman)
	t.Run("other items", declareOtherItems)
}

func declareInDesign(t *testing.T) {
	f := designing("a", 1, "relay")
	s := mapped(t, f)
	d := declaration("relay", "book")
	d.Seams, d.RulesChanged = []string{"relay-claim"}, []string{"GLOBAL-08"}
	d.Repos = []string{"buildwarden/buildwarden", "buildwarden/buildwarden-specs"}
	yes := true
	d.Alters = &yes
	wantDeclared(t, s, f, d, "cloud/run")
	if !slices.Equal(f.Seams, d.Seams) || !slices.Equal(f.RulesChanged, d.RulesChanged) ||
		!slices.Equal(f.Repos, d.Repos) || !f.alters() || f.DesignReview != "required" {
		t.Fatalf("the queue file after wf declare: %+v", f)
	}
	const logged = "declared at 4c1d9e2: repos +buildwarden/buildwarden-specs; " +
		"areas +book; seams +relay-claim; rules_changed +GLOBAL-08; " +
		"alters_relationships unset -> true; design_review unset -> required"
	if got := lastLog(f); got != logged {
		t.Fatalf("history %q, want %q", got, logged)
	}

	// Any value, a narrowing too, while the review is required; an area the
	// map lacks is recorded, and the item is undeclared until the map has it.
	wantDeclared(t, s, f, declaration("relay", "telemetry"), "host:macos-vz/run")
	if got := lastLog(f); !strings.Contains(got, "areas +telemetry -book") ||
		!strings.Contains(got, "repos -buildwarden/buildwarden-specs") {
		t.Fatalf("history %q", got)
	}
	if !s.Undeclared(f) {
		t.Fatal("an item that declares an area the map lacks is not undeclared")
	}
	// Declared again with nothing changed, the history says so.
	wantDeclared(t, s, f, declaration("relay", "telemetry"), "cloud/run")
	if got := lastLog(f); got != "declared at 4c1d9e2: nothing changed" {
		t.Fatalf("history %q", got)
	}

	// With design_review: skip, a change that narrows is refused, for every
	// worker: removing an area or a repository the queue has, or declaring
	// areas where the queue has none.
	const narrowing = "narrowing needs the design review (FLOW-19)"
	for name, tc := range map[string]struct {
		queue  *Feature
		design Declaration
	}{
		"an area removed":   {designing("b", 2, "relay", "book"), declaration("relay")},
		"areas where none":  {designing("b", 2), declaration("relay")},
		"an empty list":     {designing("b", 2), declaration([]string{}...)},
		"a repository gone": {designing("b", 2, "relay"), declaration("relay")},
	} {
		if name == "a repository gone" {
			tc.queue.Repos = []string{"buildwarden/buildwarden", "o/specs"}
		}
		if name == "an empty list" {
			tc.design.Areas = &[]string{}
		}
		tc.design.DesignReview = "skip"
		for _, w := range []string{"cloud/run", "human/jeff"} {
			wantUndeclared(t, mapped(t, tc.queue), tc.queue, tc.design, w, narrowing)
		}
		tc.design.DesignReview = "required"
		wantDeclared(t, mapped(t, tc.queue), tc.queue, tc.design, "cloud/run")
	}
	// A skip that widens, or leaves the areas undeclared, is accepted.
	g := designing("g", 3, "relay")
	skip := declaration("relay", "book")
	skip.DesignReview = "skip"
	wantDeclared(t, mapped(t, g), g, skip, "cloud/run")
	skip.Areas = nil
	wantDeclared(t, mapped(t, g), g, skip, "cloud/run")
}

func declareInPlan(t *testing.T) {
	f := working("a", 1, 2, "relay")
	f.Declared = "0a1b2c3"
	s := mapped(t, f)
	for _, w := range []string{"cloud/run", "host:macos-vz/run"} {
		wantUndeclared(t, s, f, declaration("relay", "book"), w,
			"a is in Plan: the Plan gate compares the design with its approval, and "+
				"wf declare changes nothing there (FLOW-19)")
	}
	wantDeclared(t, s, f, declaration("relay", "book"), "human/jeff")
}

// within is the approved design a widening is judged in.
func within(areas []string, drivers, surfaces, rulePaths []string) Within {
	return Within{Commit: "9f8e7d6", Areas: &areas, Drivers: drivers, Surfaces: surfaces,
		RulePaths: rulePaths}
}

func declareWidening(t *testing.T) {
	const change = "a design change: back to Design (FLOW-06)"
	implementing := func(areas ...string) (*State, *Feature) {
		f := working("a", 1, 3, areas...)
		f.Declared, f.DesignReview = "0a1b2c3", "required"
		no := false
		f.AltersRelationships = &no
		return mapped(t, f), f
	}
	widen := func(w Within, areas ...string) Declaration {
		d := declaration(areas...)
		d.Within = w
		return d
	}

	// A driver in the approved design's drivers, with no shared area
	// declared.
	s, f := implementing("driver/hyperv")
	hv := within([]string{"driver/hyperv"}, []string{"hyperv", "vz"}, []string{"cli"},
		[]string{"docs/development/process.md"})
	wantDeclared(t, s, f, widen(hv, "driver/hyperv", "driver/vz"), "cloud/run")
	if got := lastLog(f); got !=
		"declared at 4c1d9e2: areas +driver/vz (within drivers: vz)" {
		t.Fatalf("history %q", got)
	}
	// An area the map gives one of its surfaces, and one that holds a path
	// of a rule change.
	wantDeclared(t, s, f, widen(hv, "driver/hyperv", "driver/vz", "cli", "rules"),
		"host:windows-hyperv/run")
	if got := lastLog(f); got != "declared at 4c1d9e2: areas +cli (within surfaces: cli) "+
		"+rules (within rule_changes: docs/development/process.md)" {
		t.Fatalf("history %q", got)
	}

	// The driver was added to drivers on the branch: the approved design
	// does not have it.
	s, f = implementing("driver/hyperv")
	wantUndeclared(t, s, f, widen(hv, "driver/hyperv", "driver/qemu"), "cloud/run",
		"area driver/qemu is outside the approved design (at 9f8e7d6)")
	wantUndeclared(t, s, f, widen(hv, "driver/hyperv", "driver/qemu"), "cloud/run", change)
	// A design that declares a shared area lists every driver and edits
	// none of their trees.
	s, f = implementing("relay", "book")
	shared := within([]string{"relay", "book"},
		[]string{"container", "qemu", "vz", "hyperv", "firecracker"}, nil, nil)
	wantUndeclared(t, s, f, widen(shared, "relay", "book", "driver/hyperv"), "cloud/run",
		"area driver/hyperv is outside the approved design (at 9f8e7d6): it declares "+
			"the shared area relay")
	// An area the map lacks.
	wantUndeclared(t, s, f, widen(shared, "relay", "book", "telemetry"), "cloud/run",
		"area telemetry is not in the area map")

	// Every other difference is a design change.
	s, f = implementing("driver/hyperv", "book")
	for name, d := range map[string]Declaration{
		"an area removed": widen(hv, "driver/hyperv"),
		"undeclared":      widen(hv),
		"a seam": func() Declaration {
			d := widen(hv, "driver/hyperv", "book")
			d.Seams = []string{"relay-claim"}
			return d
		}(),
		"a rule change": func() Declaration {
			d := widen(hv, "driver/hyperv", "book")
			d.RulesChanged = []string{"GLOBAL-08"}
			return d
		}(),
		"a repository": func() Declaration {
			d := widen(hv, "driver/hyperv", "book")
			d.Repos = []string{"buildwarden/buildwarden", "o/specs"}
			return d
		}(),
		"alters_relationships": func() Declaration {
			d := widen(hv, "driver/hyperv", "book")
			yes := true
			d.Alters = &yes
			return d
		}(),
		"design_review": func() Declaration {
			d := widen(hv, "driver/hyperv", "book")
			d.DesignReview = "skip"
			return d
		}(),
	} {
		t.Run(name, func(t *testing.T) { wantUndeclared(t, s, f, d, "cloud/run", change) })
	}
	// No difference: the commit is recorded.
	wantDeclared(t, s, f, widen(hv, "driver/hyperv", "book"), "cloud/run")

	// A skipped review has no approved commit: the design is read on the
	// branch, and the history says so.
	s, f = implementing("driver/hyperv")
	f.DesignReview = "skip"
	branch := Within{OnBranch: true, Areas: &[]string{"driver/hyperv"},
		Drivers: []string{"hyperv"}, Surfaces: []string{"cli"}}
	d := widen(branch, "driver/hyperv", "cli")
	d.DesignReview = "skip"
	wantDeclared(t, s, f, d, "cloud/run")
	if got := lastLog(f); got != "declared at 4c1d9e2: areas +cli (within surfaces: cli) "+
		"(design on the branch: review skipped)" {
		t.Fatalf("history %q", got)
	}
	// With neither an approval nor a skip there is nothing to judge in.
	s, f = implementing("driver/hyperv")
	wantUndeclared(t, s, f, widen(Within{}, "driver/hyperv", "cli"), "cloud/run",
		"no approved design to judge a widening in")
}

// declareOverLimit is cases 14 and 15: a widening that relates the item to a
// holder is accepted while the item holds nothing, and refused, changing
// nothing, when two colliding items would then hold the PR slot.
func declareOverLimit(t *testing.T) {
	hv := within([]string{"driver/hyperv"}, []string{"hyperv", "vz"}, []string{"cli"}, nil)
	widened := declaration("driver/hyperv", "driver/vz")
	widened.Within = hv
	holder := openPR("vz", 2, 8, "driver/vz")
	holder.Declared = "0a1b2c3"

	// Case 14: in Review, while a vz item holds the PR slot.
	no := false
	a := working("a", 1, 4, "driver/hyperv")
	a.Declared, a.DesignReview, a.AltersRelationships = "0a1b2c3", "required", &no
	s := mapped(t, a, holder)
	wantDeclared(t, s, a, widened, "cloud/run")

	// Case 15: with its own PR open, at its refresh.
	b := openPR("b", 1, 7, "driver/hyperv")
	b.Declared, b.DesignReview, b.AltersRelationships = "0a1b2c3", "required", &no
	refresh(t, b)
	s = mapped(t, b, holder)
	wantUndeclared(t, s, b, widened, "cloud/run", "limit: b holds the PR slot with vz "+
		"(colliding: area driver/vz), limit 1 (FLOW-05)")
	wantUndeclared(t, s, b, widened, "cloud/run", "back to Design (FLOW-06)")
	wantValidate(t, s)
	// A human's declaration is held to the limits too.
	wantUndeclared(t, s, b, widened, "human/jeff", "limit: b holds the PR slot with vz")

	// In Design the refusal is for a blocking question: a redesign that
	// declares alters_relationships past the limit.
	h := alter("h", 1, "relay")
	c := alter("c", 2, "relay")
	s = mapped(t, h, c)
	take(t, s, h)
	finishPlan(t, s, h)
	c.AltersRelationships = &no
	take(t, s, c)
	finishPlan(t, s, c)
	Back(c, "cloud/run", now, "the design must change", 1)
	yes := true
	d := declaration("relay")
	d.Alters = &yes
	wantUndeclared(t, s, c, d, "cloud/run", "limit: 2 features hold an "+
		"alters_relationships slot, limit 1 (FLOW-05): h, c")
	wantUndeclared(t, s, c, d, "cloud/run", "raise a blocking question (FLOW-07)")
}

func declareByHuman(t *testing.T) {
	// After Design a human may give any value: this is how the review
	// narrows, and how an item that finished Design before wf declare is
	// declared, with or without areas.
	f := working("a", 1, 3, "driver/hyperv", "book")
	s := mapped(t, f)
	d := declaration("book")
	d.Seams, d.RulesChanged = []string{"relay-claim"}, []string{"GLOBAL-08"}
	wantDeclared(t, s, f, d, "human/jeff")
	if got := lastLog(f); !strings.Contains(got, "areas -driver/hyperv") {
		t.Fatalf("history %q", got)
	}
	d.Areas = nil
	wantDeclared(t, s, f, d, "human/jeff")
	if f.Areas != nil || !slices.Equal(f.Seams, d.Seams) {
		t.Fatalf("a human's undeclared declaration left %+v", f)
	}
}

func declareOtherItems(t *testing.T) {
	// A process item is outside the relations: its repos are recorded, and
	// nothing else of its design.
	x := process("x", 1, "in_progress")
	s := mapped(t, x)
	d := declaration("loop")
	d.Repos = []string{"buildwarden/buildwarden", "o/specs"}
	d.Seams, d.RulesChanged = []string{"relay-claim"}, []string{"FLOW-05"}
	if err := s.Declare(x, d, "human/jeff", now); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(x.Repos, d.Repos) || x.Declared != commit || x.Areas != nil ||
		x.Seams != nil || x.RulesChanged != nil || x.AltersRelationships != nil {
		t.Fatalf("a process item after wf declare: %+v", x)
	}
	wantValidate(t, s)

	tracked := feature("m1", 2, "queued")
	tracked.Repos, tracked.Round = []string{"o/relay"}, "tracked"
	done := feature("d", 3, "done")
	s = mapped(t, tracked, done)
	wantUndeclared(t, s, tracked, declaration("relay"), "human/jeff",
		"m1 is tracked: wf cannot read its design in o/relay")
	wantUndeclared(t, s, done, declaration("relay"), "human/jeff", "d is done")
	wantUndeclared(t, s, designing("e", 4), Declaration{}, "", "wf declare needs -worker")
}

// TestRelate: only a human names a collision the area map does not show, and
// not one that would put a limit over (FLOW-05).
func TestRelate(t *testing.T) {
	a, b := openPR("a", 1, 7, "relay"), working("b", 2, 4, "driver/vz")
	s := mapped(t, a, b)
	if got := s.Relation(a, b).Kind; got != Disjoint {
		t.Fatalf("a and b are %s, want disjoint", got)
	}
	refused := func(with string, clear bool, worker, why string) {
		t.Helper()
		err := s.Relate(a, with, clear, worker, now)
		if err == nil || !strings.Contains(err.Error(), why) {
			t.Fatalf("wf relate -with %s by %q: %v, want %q", with, worker, err, why)
		}
	}
	for _, w := range []string{"cloud/run", "host:macos-vz/run", ""} {
		refused("b", false, w, "only a human names a collision (FLOW-05)")
	}
	if err := s.Relate(a, "b", false, "human/jeff", now); err != nil {
		t.Fatal(err)
	}
	if got := s.Relation(b, a); got.Kind != Colliding || got.Reason != "named by the human" {
		t.Fatalf("after wf relate: %s", got)
	}
	if !slices.Equal(a.CollidesWith, []string{"b"}) ||
		lastLog(a) != "collides with b: named by the human (FLOW-05)" {
		t.Fatalf("collides_with %v, history %q", a.CollidesWith, lastLog(a))
	}
	wantValidate(t, s)
	refused("b", false, "human/jeff", "a already names b in collides_with")
	refused("a", false, "human/jeff", "a cannot collide with itself")
	refused("z", false, "human/jeff", `no feature "z"`)
	refused("b", true, "cloud/run", "only a human names a collision (FLOW-05)")
	if err := s.Relate(a, "b", true, "human/jeff", now); err != nil {
		t.Fatal(err)
	}
	if len(a.CollidesWith) != 0 || s.Relation(a, b).Kind != Disjoint ||
		lastLog(a) != "no longer collides with b by the human's word (FLOW-05)" {
		t.Fatalf("after -clear: %v, %q", a.CollidesWith, lastLog(a))
	}
	refused("b", true, "human/jeff", "a does not name b in collides_with")
}

// TestRelateOverLimit: a collision that would put two holders of the PR slot
// in one set is refused and changes nothing (FLOW-05).
func TestRelateOverLimit(t *testing.T) {
	a, c := openPR("a", 1, 7, "relay"), openPR("c", 3, 9, "driver/qemu")
	s := mapped(t, a, c)
	err := s.Relate(a, "c", false, "human/jeff", now)
	if err == nil || !strings.Contains(err.Error(),
		"limit: a holds the PR slot with c (colliding: named by the human), limit 1") {
		t.Fatalf("wf relate over the PR slot: %v", err)
	}
	if len(a.CollidesWith) != 0 {
		t.Fatalf("a refused wf relate wrote %v", a.CollidesWith)
	}
	wantValidate(t, s)
}

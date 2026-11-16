package queue

import (
	"fmt"
	"testing"
)

const (
	here  = "buildwarden/buildwarden"
	specs = "buildwarden/buildwarden-specs"
	relay = "buildwarden/buildwarden-relay"
)

// item is a queued feature that declares the areas; with none it is
// declared-empty, not undeclared.
func item(name string, areas ...string) *Feature {
	f := feature(name, 1, "queued")
	if areas == nil {
		areas = []string{}
	}
	f.Areas = &areas
	return f
}

// undeclared is a queued feature that declares no areas.
func undeclared(name string) *Feature { return feature(name, 1, "queued") }

func withAreas(f *Feature, areas ...string) *Feature {
	f.Areas = &areas
	return f
}

func withSeams(f *Feature, seams ...string) *Feature {
	f.Seams, f.Declared = seams, "4c1d9e2"
	return f
}

func withRules(f *Feature, rules ...string) *Feature {
	f.RulesChanged, f.Declared = rules, "4c1d9e2"
	return f
}

func inRepos(f *Feature, repos ...string) *Feature {
	f.Repos = repos
	return f
}

func adopting(f *Feature) *Feature {
	f.Adopts = &Adopts{Consumable: "buildwarden-relay", Version: "v0.3.0"}
	return f
}

func fixing(f *Feature, parent string) *Feature {
	f.Follows = parent
	return f
}

func naming(f *Feature, others ...string) *Feature {
	f.CollidesWith = others
	return f
}

type relationCase struct {
	name   string
	a, b   *Feature
	kind   string
	row    int
	reason string
}

// wantRelation asserts the relation of a and b under s, in both orders.
func wantRelation(t *testing.T, s *State, c relationCase) {
	t.Helper()
	want := Relation{Kind: c.kind, Row: c.row, Reason: c.reason}
	if got := s.Relation(c.a, c.b); got != want {
		t.Errorf("%s: Relation = %+v, want %+v", c.name, got, want)
	}
	if got := s.Relation(c.b, c.a); got != want {
		t.Errorf("%s, the other order: Relation = %+v, want %+v", c.name, got, want)
	}
}

// rowCases is one case per row of the relation table (FLOW-05).
func rowCases() []relationCase {
	return []relationCase{
		{"row 1: either is a process item",
			item("a", "relay"), process("p", 2, "in_progress"), Outside, 1, ""},
		{"row 1: both are process items",
			process("p", 1, "in_progress"), process("q", 2, "queued"), Outside, 1, ""},
		{"row 1 is before row 2",
			naming(item("a", "relay"), "p"), process("p", 2, "queued"),
			Outside, 1, ""},
		{"row 2: one names the other in collides_with",
			naming(item("a", "driver/hyperv"), "b"), item("b", "relay"),
			Colliding, 2, "named by the human"},
		{"row 2 is before row 3",
			withSeams(naming(item("a", "driver/hyperv"), "b"), "relay-claim"),
			withSeams(item("b", "relay"), "relay-claim"),
			Colliding, 2, "named by the human"},
		{"row 3: both list a seam that is not written",
			withSeams(item("a", "driver/hyperv"), "ledger-format", "relay-claim"),
			withSeams(item("b", "relay"), "relay-claim", "ledger-format"),
			Colliding, 3, "seam relay-claim, which has no specification"},
		{"row 3 does not ask about repositories",
			withSeams(item("a", "driver/hyperv"), "relay-claim"),
			withSeams(inRepos(item("b"), relay), "relay-claim"),
			Colliding, 3, "seam relay-claim, which has no specification"},
		{"row 4: a repository in common that has no area map",
			inRepos(item("a", "specs"), here, specs),
			inRepos(item("b", "relay"), here, specs),
			Colliding, 4, specs + " has no area map"},
		{"row 4 is before row 5",
			inRepos(undeclared("a"), here, specs), inRepos(item("b", "relay"), specs),
			Colliding, 4, specs + " has no area map"},
		{"row 5: a repository in common and one is undeclared",
			item("a", "relay"), undeclared("b"), Colliding, 5, "b declares no areas"},
		{"row 5: both undeclared, the first by name",
			undeclared("b"), undeclared("a"), Colliding, 5, "a declares no areas"},
		{"row 6: they share an exclusive area",
			item("a", "relay", "driver-core", "book"),
			item("b", "driver-core", "relay"),
			Colliding, 6, "area driver-core"},
		{"row 6 is before row 7",
			withSeams(item("a", "ledger"), "ledger-format"),
			withSeams(item("b", "ledger"), "ledger-format"),
			Colliding, 6, "area ledger"},
		{"row 7: both list a written seam",
			withSeams(item("a", "driver/hyperv"), "ledger-format"),
			withSeams(item("b", "driver/vz"), "ledger-format"),
			Near, 7, "seam ledger-format"},
		{"row 7 does not ask about repositories",
			withSeams(item("a", "driver/hyperv"), "ledger-format"),
			withSeams(inRepos(item("b"), relay), "ledger-format"),
			Near, 7, "seam ledger-format"},
		{"row 8: both list the same rule",
			withRules(item("a", "driver/hyperv"), "TEST-05", "GLOBAL-08"),
			withRules(item("b", "driver/vz"), "GLOBAL-08", "TEST-05"),
			Near, 8, "rule GLOBAL-08"},
		{"row 8 does not ask about repositories",
			withRules(item("a", "driver/hyperv"), "GLOBAL-08"),
			withRules(inRepos(item("b"), relay), "GLOBAL-08"),
			Near, 8, "rule GLOBAL-08"},
		{"row 9: an area of one reaches an area of the other",
			item("a", "specs", "book"), item("b", "ledger"),
			Near, 9, "specs reaches ledger"},
		{"row 9: in the other direction",
			item("a", "ledger"), item("b", "specs"), Near, 9, "specs reaches ledger"},
		{"row 9: both directions, the first in sorted order",
			item("a", "driver/qemu"), item("b", "driver-core"),
			Near, 9, "driver-core reaches driver/qemu"},
		{"row 10: nothing shared but ordered areas",
			item("a", "book", "rules", "build"), item("b", "build", "book", "rules"),
			Disjoint, 10, ""},
		{"row 10: nothing shared",
			item("a", "driver/hyperv"), item("b", "relay"), Disjoint, 10, ""},
		{"row 10: both declare no area",
			item("a"), item("b"), Disjoint, 10, ""},
		{"row 10: no repository in common",
			item("a", "relay"), inRepos(item("b", "relay"), relay), Disjoint, 10, ""},
	}
}

// exampleCases is one case per line of the design's table of examples
// from this repository.
func exampleCases() []relationCase {
	hyperv := func() *Feature { return item("hyperv-item", "driver/hyperv", "book") }
	s1 := func() *Feature {
		return withSeams(inRepos(undeclared("seam-specs"), here, specs),
			"relay-claim", "collector", "ledger-format")
	}
	k1 := func() *Feature {
		return withSeams(undeclared("driver-contract"), "driver", "relay-claim",
			"collector")
	}
	return []relationCase{
		{"a Hyper-V item and a Go relay release target",
			hyperv(), item("relay-target", "relay", "build", "book"),
			Disjoint, 10, ""},
		{"a Hyper-V item and a vz item, both with a book page",
			hyperv(), item("vz-item", "driver/vz", "book"), Disjoint, 10, ""},
		{"a Hyper-V item and an item in driver-core",
			hyperv(), item("session", "driver-core"),
			Near, 9, "driver-core reaches driver/hyperv"},
		{"an item in relaycfg and a driver item",
			item("claim", "relaycfg"), hyperv(),
			Near, 9, "driver/hyperv reaches relaycfg"},
		{"an item in relaycfg and the relay",
			item("claim", "relaycfg"), item("relay-item", "relay"),
			Near, 9, "relay reaches relaycfg"},
		{"two driver items that each also edit cmd/warden",
			item("hyperv-flag", "driver/hyperv", "cli"),
			item("vz-flag", "driver/vz", "cli"),
			Disjoint, 10, ""},
		{"an item that edits only cmd/warden and an item in relaycfg",
			item("cli-only", "cli", "book"), item("claim", "relaycfg"),
			Disjoint, 10, ""},
		{"an item that edits only cmd/warden and an item in driver-core",
			item("cli-only", "cli", "book"), item("session", "driver-core"),
			Disjoint, 10, ""},
		{"an item that edits only cmd/warden and a driver item",
			item("cli-only", "cli", "book"), hyperv(), Disjoint, 10, ""},
		{"two items that amend GLOBAL-08, in different areas (S2 and K1)",
			withRules(item("s2", "relay", "executor", "rules", "book"), "GLOBAL-08"),
			withRules(item("k1", "driver-core"), "GLOBAL-08"),
			Near, 8, "rule GLOBAL-08"},
		{"two items that change ledger-format, in different areas",
			withSeams(item("a", "ledger"), "ledger-format"),
			withSeams(item("b", "driver/hyperv"), "ledger-format"),
			Near, 7, "seam ledger-format"},
		{"two items that change relay-claim, which has no specification",
			withSeams(item("a", "relay"), "relay-claim"),
			withSeams(item("b", "driver/hyperv"), "relay-claim"),
			Colliding, 3, "seam relay-claim, which has no specification"},
		{"an undeclared item and any feature here",
			undeclared("driver-contract"), hyperv(),
			Colliding, 5, "driver-contract declares no areas"},
		{"seam-specs and driver-contract today, neither declared",
			undeclared("seam-specs"), undeclared("driver-contract"),
			Colliding, 5, "driver-contract declares no areas"},
		{"seam-specs and driver-contract once their seams are recorded",
			s1(), k1(), Colliding, 3, "seam collector, which has no specification"},
		{"the same two, whatever areas they are given",
			withAreas(s1(), "specs", "book"), withAreas(k1(), "driver-core"),
			Colliding, 3, "seam collector, which has no specification"},
		{"an item that changes a tier-1 check and one whose code the check runs over",
			item("check", "tier1"), hyperv(), Disjoint, 10, ""},
		{"an item that adds a rules.yaml entry and one whose code the check runs over",
			item("entry", "tier1-config", "driver/qemu"), hyperv(), Disjoint, 10, ""},
		{"an item that declares an area the map lacks",
			item("conformance", "tools/conformance", "book"), hyperv(),
			Colliding, 5, "conformance declares no areas"},
		{"an item here and an item homed in the relay repository",
			hyperv(), inRepos(item("m1-task"), relay), Disjoint, 10, ""},
		{"K1 here and the relay's M1, both on a seam that is not written",
			withSeams(item("driver-contract", "driver-core"), "driver"),
			withSeams(inRepos(undeclared("relay-m1"), relay), "driver"),
			Colliding, 3, "seam driver, which has no specification"},
		{"two items in a repository with no map",
			inRepos(item("a"), specs), inRepos(item("b"), specs),
			Colliding, 4, specs + " has no area map"},
		{"any item and a process item",
			hyperv(), process("loop-contract", 2, "in_progress"), Outside, 1, ""},
		{"a process item and seam-specs",
			process("loop-contract", 2, "in_progress"), s1(), Outside, 1, ""},
		{"two reconciliation items for two drivers",
			adopting(item("adopt-qemu", "driver/qemu", "book")),
			adopting(item("adopt-vz", "driver/vz", "book")), Disjoint, 10, ""},
		{"a reconciliation item and a fix in the same driver tree",
			adopting(item("adopt-qemu", "driver/qemu", "book")),
			fixing(item("qemu-fix", "driver/qemu"), "old"),
			Colliding, 6, "area driver/qemu"},
		{"two items the review told apart from what the map says",
			item("runtime-cli-project-path", "cli", "book"),
			naming(item("driver-contract", "driver-core"), "runtime-cli-project-path"),
			Colliding, 2, "named by the human"},
	}
}

// TestRelationTable: one case per row of the relation table and per line of
// the examples table, under the design's map.
func TestRelationTable(t *testing.T) {
	s := mapped(t)
	for _, c := range append(rowCases(), exampleCases()...) {
		wantRelation(t, s, c)
	}
}

// TestRelationNoMap: the queue state with no areas.yaml, and a config with no
// repository. The repository is one exclusive area, which is today's
// behaviour, and the reason says so.
func TestRelationNoMap(t *testing.T) {
	s := state()
	s.Config.Repository = here
	for _, c := range []relationCase{
		{"declared items", item("a", "driver/hyperv"), item("b", "relay"),
			Colliding, 4, here + " has no area map"},
		{"an unwritten seam is still row 3", withSeams(item("a"), "ledger-format"),
			withSeams(item("b"), "ledger-format"),
			Colliding, 3, "seam ledger-format, which has no specification"},
		{"a process item", item("a"), process("p", 2, "queued"), Outside, 1, ""},
		{"no repository in common", item("a"), inRepos(item("b"), relay),
			Disjoint, 10, ""},
	} {
		wantRelation(t, s, c)
	}
	s.Config.Repository = ""
	wantRelation(t, s, relationCase{"config.yaml names no repository", item("a", "relay"),
		undeclared("b"), Colliding, 4, "the queue's repository has no area map"})
}

// pairs returns every pair of two different items of fs.
func pairs(fs []*Feature) [][2]*Feature {
	var out [][2]*Feature
	for i, a := range fs {
		for _, b := range fs[i+1:] {
			out = append(out, [2]*Feature{a, b})
		}
	}
	return out
}

// TestRelationUndeclaredIsToday: with no item declaring areas, every pair of
// features in one repository collides, with the map and without it.
func TestRelationUndeclaredIsToday(t *testing.T) {
	var fs []*Feature
	for i, status := range []string{"queued", "in_progress", "in_progress", "done",
		"queued"} {
		fs = append(fs, feature(fmt.Sprintf("f%d", i), i+1, status))
	}
	fs[1].Follows, fs[2].Seams, fs[4].Repos = "f3", []string{"relay-claim"}, []string{here}
	bare := state(fs...)
	bare.Config.Repository = here
	for _, s := range []*State{mapped(t, fs...), bare} {
		for _, p := range pairs(fs) {
			if got := s.Relation(p[0], p[1]); got.Kind != Colliding {
				t.Errorf("%s, %s: %+v, want colliding", p[0].Feature, p[1].Feature,
					got)
			}
		}
	}
}

// TestRelationUnknownArea: an area the map lacks makes the item undeclared,
// whatever else it declares, until the map has the area.
func TestRelationUnknownArea(t *testing.T) {
	s := mapped(t)
	a, b := item("a", "driver/hyperv", "tools/conformance"), item("b", "relay")
	wantRelation(t, s, relationCase{"an area the map lacks", a, b,
		Colliding, 5, "a declares no areas"})
	wantRelation(t, s, relationCase{"and an item that declares nothing", a, item("c"),
		Colliding, 5, "a declares no areas"})
	s.Areas.Areas["tools/conformance"] = Area{Mode: ModeExclusive,
		Paths: []string{"tools/conformance/"}}
	wantRelation(t, s, relationCase{"once the map has the area", a, b, Disjoint, 10, ""})
}

// TestRelationOrderedReachesNothing: an ordered area reaches nothing and is
// reached by nothing (a18 (a)), even in a map that says otherwise, which
// Validate rejects.
func TestRelationOrderedReachesNothing(t *testing.T) {
	s := mapped(t)
	cli := s.Areas.Areas["cli"]
	cli.Reaches = []string{"relaycfg", "driver/hyperv"}
	s.Areas.Areas["cli"] = cli
	hv := s.Areas.Areas["driver/hyperv"]
	hv.Reaches = append(hv.Reaches, "book")
	s.Areas.Areas["driver/hyperv"] = hv
	if len(s.Validate()) == 0 {
		t.Fatal("Validate passes a map whose ordered areas have and are named in reaches")
	}
	wantRelation(t, s, relationCase{"an ordered area with reaches", item("a", "cli"),
		item("b", "relaycfg"), Disjoint, 10, ""})
	wantRelation(t, s, relationCase{"an ordered area named in reaches",
		item("a", "driver/hyperv"), item("b", "book"), Disjoint, 10, ""})
}

// generated is a set of items that differ in kind, repositories, areas, seams,
// rules and the items they name.
func generated() []*Feature {
	areas := [][]string{nil, {}, {"relay"}, {"driver/hyperv", "book"}, {"driver-core", "cli"},
		{"relaycfg", "gone"}, {"book", "rules"}}
	repos := [][]string{nil, {here}, {here, specs}, {relay}, {specs, here}}
	seams := [][]string{nil, {"relay-claim"}, {"ledger-format"},
		{"ledger-format", "collector"}}
	rules := [][]string{nil, {"GLOBAL-08"}, {"TEST-05", "GLOBAL-08"}}
	name := func(i, j, k int) string { return fmt.Sprintf("g-%d-%d-%d", i, j, k) }
	var out []*Feature
	for i, a := range areas {
		for j, r := range repos {
			for k, sm := range seams {
				f := feature(name(i, j, k), len(out)+1, "queued")
				if a != nil {
					f.Areas = &a
				}
				f.Repos, f.Seams, f.RulesChanged = r, sm, rules[(i+j+k)%len(rules)]
				other := name(k, i%len(repos), j%len(seams))
				switch (i + 2*j + 3*k) % 7 {
				case 0:
					f.Kind, f.Round = "process", "process"
				case 1:
					f.CollidesWith = []string{other}
				}
				out = append(out, f)
			}
		}
	}
	return out
}

// TestRelationSymmetric: both orders give the same relation and reason for
// every pair of a generated set, and every relation is one of the four.
func TestRelationSymmetric(t *testing.T) {
	fs := generated()
	bare := state(fs...)
	bare.Config.Repository = here
	seen := map[string]int{}
	for _, s := range []*State{mapped(t, fs...), bare} {
		for _, p := range pairs(fs) {
			ab, ba := s.Relation(p[0], p[1]), s.Relation(p[1], p[0])
			unreasoned := ab.Kind == Outside || ab.Kind == Disjoint
			if ab != ba || (ab.Reason == "") != unreasoned {
				t.Fatalf("%s, %s: %+v one way and %+v the other", p[0].Feature,
					p[1].Feature, ab, ba)
			}
			seen[ab.Kind]++
		}
	}
	for _, kind := range []string{Outside, Colliding, Near, Disjoint} {
		if seen[kind] == 0 {
			t.Errorf("the generated set has no %s pair", kind)
		}
	}
	if len(seen) != 4 {
		t.Errorf("relations seen: %v, want only the four", seen)
	}
}

func TestRelationString(t *testing.T) {
	for _, c := range []struct {
		r    Relation
		want string
	}{
		{Relation{Kind: Colliding, Row: 3,
			Reason: "seam collector, which has no specification"},
			"colliding (seam collector, which has no specification)"},
		{Relation{Kind: Near, Row: 8, Reason: "rule GLOBAL-08"}, "near (rule GLOBAL-08)"},
		{Relation{Kind: Disjoint, Row: 10}, "disjoint"},
		{Relation{Kind: Outside, Row: 1}, "outside"},
	} {
		if got := c.r.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

// TestRelationBadModeCollides: an area whose mode is missing or misspelled is
// taken as exclusive, so only an explicit ordered area relaxes anything; wf
// validate reports the mode (FLOW-05).
func TestRelationBadModeCollides(t *testing.T) {
	for _, mode := range []string{"", "Exclusive", "shared"} {
		s := mapped(t)
		ledger := s.Areas.Areas["ledger"]
		ledger.Mode = mode
		s.Areas.Areas["ledger"] = ledger
		wantRelation(t, s, relationCase{fmt.Sprintf("mode %q", mode),
			item("a", "ledger"), item("b", "ledger"), Colliding, 6, "area ledger"})
	}
}

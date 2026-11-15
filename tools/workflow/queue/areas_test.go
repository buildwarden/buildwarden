package queue

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// designMap is the map of the loop-parallel design for main at ad4ea50
// (testdata/areas.yaml), which x4-state saves as areas.yaml.
func designMap(t *testing.T) *AreaMap {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "areas.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseAreas(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// mapped is a state whose config names the design map's repository and whose
// areas.yaml is that map.
func mapped(t *testing.T, fs ...*Feature) *State {
	t.Helper()
	s := state(fs...)
	s.Areas = designMap(t)
	s.Config.Repository = s.Areas.Repository
	return s
}

// smallMap is a map of three exclusive areas and one ordered, with base as
// its lines before `areas:`.
const smallMap = `version: 1
repository: o/r
catch_all: core
areas:
  a:
    mode: exclusive
    paths: [a/, tools/a/]
    reaches: [b]
  b:
    mode: exclusive
    paths: [b/]
  core:
    mode: exclusive
  docs:
    mode: ordered
    paths: [docs/, README.md]
`

// mapFindings parses a map and returns the parse error, or else the findings
// Validate has for it under a config that names o/r, each without its
// "areas.yaml: " prefix.
func mapFindings(t *testing.T, text string) []string {
	t.Helper()
	m, err := ParseAreas([]byte(text))
	if err != nil {
		return []string{err.Error()}
	}
	s := state()
	s.Config.Repository = "o/r"
	s.Areas = m
	var out []string
	for _, e := range s.Validate() {
		finding, ok := strings.CutPrefix(e.Error(), "areas.yaml: ")
		if !ok {
			t.Fatalf("a finding about the map does not name areas.yaml: %v", e)
		}
		out = append(out, finding)
	}
	return out
}

func TestAreaMapParse(t *testing.T) {
	m := designMap(t)
	exclusive, ordered := 0, 0
	for _, name := range m.Names() {
		if m.Exclusive(name) {
			exclusive++
		} else {
			ordered++
		}
	}
	if exclusive != 15 || ordered != 6 {
		t.Fatalf("the design's map has %d exclusive and %d ordered areas, want 15 and 6",
			exclusive, ordered)
	}
	if got := mapFindings(t, smallMap); len(got) != 0 {
		t.Fatalf("a valid map: %q", got)
	}
	for _, c := range []struct{ name, text, want string }{
		{"unknown key", smallMap + "extra: 1\n", "field extra not found"},
		{"unknown key in an area",
			strings.Replace(smallMap, "  b:\n", "  b:\n    mod: x\n", 1),
			"field mod not found"},
		{"missing mode",
			strings.Replace(smallMap, "  b:\n    mode: exclusive\n", "  b:\n", 1),
			"area b has no mode"},
		{"a catch_all with paths",
			strings.Replace(smallMap, "  core:\n    mode: exclusive\n",
				"  core:\n    mode: exclusive\n    paths: [core/]\n", 1),
			"catch_all core does not name an exclusive area with no paths"},
		{"reaches on an ordered area",
			smallMap + "    reaches: [b]\n",
			"area docs is ordered and has reaches (only an exclusive area may)"},
		{"an ordered area named in reaches",
			strings.Replace(smallMap, "reaches: [b]", "reaches: [b, docs]", 1),
			"area a reaches docs, which is ordered " +
				"(reaches names only exclusive areas)"},
	} {
		got := mapFindings(t, c.text)
		if len(got) != 1 || !strings.Contains(got[0], c.want) {
			t.Errorf("%s: findings %q, want one with %q", c.name, got, c.want)
		}
	}
}

func TestAreaMapMatch(t *testing.T) {
	m := designMap(t)
	for _, c := range []struct{ name, path, want string }{
		{"a directory", "relay/tls/leaf.go", "relay"},
		{"a file", "Dockerfile.relay", "relay"},
		{"a file is not a prefix", "Dockerfile.relay.bak", "core"},
		{"the longest prefix", "tools/relay-vm/hyperv/build.sh", "driver/hyperv"},
		{"the shorter prefix", "tools/relay-vm/init", "driver-core"},
		{"a driver's tree over driver/", "driver/hyperv/relayseed.go", "driver/hyperv"},
		{"driver/ itself", "driver/session.go", "driver-core"},
		{"a file over its directory", "tools/tier1/rules.yaml", "tier1-config"},
		{"the directory beside the file", "tools/tier1/tier1.go", "tier1"},
		{"the catch-all", "newtree/main.go", "core"},
	} {
		got, err := m.AreaOf(c.path)
		if err != nil || got != c.want {
			t.Errorf("%s: AreaOf(%q) = %q, %v; want %q", c.name, c.path, got, err,
				c.want)
		}
	}

	star, err := ParseAreas([]byte(strings.Replace(smallMap, "paths: [b/]",
		"paths: [b/, a/doc-*.md, tools/a/gen-*.sh]", 1)))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, path, want string }{
		{"* in a file name", "a/doc-notes.md", "b"},
		{"* stays in its directory", "a/doc-sub/notes.md", "a"},
		{"* between two texts", "tools/a/gen-keys.sh", "b"},
		{"* needs the text after it", "tools/a/gen-keys.py", "a"},
	} {
		got, err := star.AreaOf(c.path)
		if err != nil || got != c.want {
			t.Errorf("%s: AreaOf(%q) = %q, %v; want %q", c.name, c.path, got, err,
				c.want)
		}
	}

	tie, err := ParseAreas([]byte(strings.Replace(smallMap, "paths: [b/]",
		"paths: [b/, a/]", 1)))
	if err != nil {
		t.Fatal(err)
	}
	const want = "areas.yaml: a/x.go matches areas a and b equally"
	if got, err := tie.AreaOf("a/x.go"); err == nil || err.Error() != want {
		t.Fatalf("a tie: AreaOf = %q, %v; want the error %q", got, err, want)
	}
}

// TestAreasValidate: each thing wf validate fails a map for, beyond those
// TestAreaMapParse has.
func TestAreasValidate(t *testing.T) {
	replace := func(old, new string) string {
		if !strings.Contains(smallMap, old) {
			t.Fatalf("smallMap has no %q", old)
		}
		return strings.Replace(smallMap, old, new, 1)
	}
	for _, c := range []struct {
		name, text string
		want       []string
	}{
		{"another repository", replace("repository: o/r", "repository: o/other"),
			[]string{"repository o/other is not config.yaml's repository o/r"}},
		{"a version wf does not read", replace("version: 1", "version: 2"),
			[]string{"version 2 is not 1"}},
		{"a mode that is neither", replace("mode: ordered", "mode: shared"),
			[]string{"area docs mode \"shared\" is not exclusive or ordered"}},
		{"no catch_all", replace("catch_all: core\n", ""),
			[]string{
				"catch_all is not set (it names an exclusive area with no paths)",
				"area core has no paths (only catch_all may have none)"}},
		{"an ordered catch_all",
			replace("  core:\n    mode: exclusive\n", "  core:\n    mode: ordered\n"),
			[]string{
				"catch_all core does not name an exclusive area with no paths"}},
		{"an area with no paths", replace("    paths: [b/]\n", ""),
			[]string{"area b has no paths (only catch_all may have none)"}},
		{"one pattern in two areas", replace("paths: [b/]", "paths: [b/, tools/a/]"),
			[]string{"areas a and b both list a pattern with the text " +
				"tools/a/ before any *"}},
		{"the same text before *", replace("paths: [b/]", "paths: [b/, tools/a/*.sh]"),
			[]string{"areas a and b both list a pattern with the text " +
				"tools/a/ before any *"}},
		{"reaches an area the map lacks", replace("reaches: [b]", "reaches: [b, gone]"),
			[]string{"area a reaches gone, which the map lacks"}},
		{"implies areas the map lacks", smallMap + "implies:\n  drivers: {x: gone}\n" +
			"  shared: [b, gone]\n  surfaces: {cli: [docs, gone]}\n",
			[]string{
				"implies.drivers.x names gone, which the map lacks",
				"implies.shared names gone, which the map lacks",
				"implies.surfaces.cli names gone, which the map lacks"}},
	} {
		if got := mapFindings(t, c.text); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}

	s := mapped(t)
	wantValidate(t, s)
	s.Config.Repository = ""
	wantValidate(t, s, "areas.yaml: repository buildwarden/buildwarden is not "+
		"config.yaml's repository, which is not set")
}

// TestLoadReadsAreas: areas.yaml is an optional file of the queue state, read
// as hosts.yaml is, and one that does not parse fails Load.
func TestLoadReadsAreas(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("config.yaml", "run_budget: {steps: 8}\nrepository: o/r\n")
	s, err := Load(dir)
	if err != nil || s.Areas != nil {
		t.Fatalf("no areas.yaml: Areas = %+v, err %v; want none", s.Areas, err)
	}
	write("areas.yaml", smallMap)
	if s, err = Load(dir); err != nil {
		t.Fatal(err)
	}
	if s.Areas == nil || !slices.Equal(s.Areas.Names(), []string{"a", "b", "core", "docs"}) {
		t.Fatalf("loaded %+v", s.Areas)
	}
	wantValidate(t, s)
	write("areas.yaml", smallMap+"extra: 1\n")
	if _, err = Load(dir); err == nil || !strings.Contains(err.Error(), "areas.yaml") ||
		!strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("an unknown key: Load err = %v", err)
	}
}

// TestAreasAbsentAndEmpty: an absent areas and areas: [] are two things, the
// item undeclared and the item declared as changing no area, and stay two
// through a save and a load.
func TestAreasAbsentAndEmpty(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "queue"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("run_budget: {steps: 8}\n"),
		0o644)
	s := &State{Dir: dir}
	absent, empty, two := feature("absent", 1, "queued"), feature("empty", 2, "queued"),
		feature("two", 3, "queued")
	empty.Areas = &[]string{}
	two.Areas = &[]string{"relay", "book"}
	for _, f := range []*Feature{absent, empty, two} {
		if err := s.Save(f); err != nil {
			t.Fatal(err)
		}
	}
	text := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, "queue", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if strings.Contains(text("absent"), "areas") {
		t.Fatalf("an undeclared item's file names areas:\n%s", text("absent"))
	}
	if !strings.Contains(text("empty"), "areas: []\n") {
		t.Fatalf("a declared-empty item's file lacks areas: []:\n%s", text("empty"))
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	find := func(name string) *Feature {
		f, err := got.Find(name)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	if a := find("absent").Areas; a != nil {
		t.Fatalf("absent loaded as %v", *a)
	}
	if a := find("empty").Areas; a == nil || len(*a) != 0 {
		t.Fatalf("areas: [] loaded as %v", a)
	}
	if a := find("two").Areas; a == nil || !slices.Equal(*a, []string{"relay", "book"}) {
		t.Fatalf("two areas loaded as %v", a)
	}
}

func TestParseEdges(t *testing.T) {
	got, err := ParseEdges(strings.NewReader("a b\n\ntools/a b/sub\n"))
	if err != nil || !slices.Equal(got, []Edge{{"a", "b"}, {"tools/a", "b/sub"}}) {
		t.Fatalf("ParseEdges = %v, %v", got, err)
	}
	for _, bad := range []string{"a\n", "a b c\n"} {
		if _, err := ParseEdges(strings.NewReader(bad)); err == nil ||
			!strings.Contains(err.Error(), "line 1") {
			t.Fatalf("ParseEdges(%q) err = %v, want one naming line 1", bad, err)
		}
	}
}

// TestAreasCheck: what CheckAreas reports for a tree and a list of edges, and
// which of its lines are differences.
func TestAreasCheck(t *testing.T) {
	m, err := ParseAreas([]byte(smallMap))
	if err != nil {
		t.Fatal(err)
	}
	tree := []string{"a/x.go", "tools/a/gen.sh", "b/y.go", "docs/guide.md", "README.md"}
	edges := []Edge{{"a", "b"}, {"a/sub", "b"}, {"tools/a", "a"}}
	check := func(s *State, files []string, e []Edge, compared bool) AreasCheck {
		t.Helper()
		got, err := s.CheckAreas(files, e, compared)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	s := state()
	s.Areas = m

	if _, err := state().CheckAreas(tree, nil, false); err == nil {
		t.Fatal("a state with no areas.yaml: CheckAreas gave no error")
	}
	for _, c := range []struct {
		name     string
		files    []string
		edges    []Edge
		compared bool
		want     []string
		diffs    int
	}{
		{"the map and the tree agree", tree, edges, true, nil, 0},
		{"no -edges", tree, nil, false, []string{"reaches: not compared"}, 0},
		{"a path in the catch-all", append(slices.Clone(tree), "new/z.go", "top.txt"),
			edges, true, []string{
				"catch-all: new/z.go is in no area's paths (it falls to core)",
				"catch-all: top.txt is in no area's paths (it falls to core)"}, 2},
		{"a pattern that matches nothing", tree[1:], edges, true,
			[]string{"pattern: a/ of area a matches no file"}, 1},
		{"a dependency reaches lacks", tree,
			append(slices.Clone(edges), Edge{"b/sub", "a"}, Edge{"b", "tools/a"}),
			true,
			[]string{"reaches: b lacks a (b depends on tools/a)"}, 1},
		{"a reaches entry nothing supports", tree, nil, true,
			[]string{"reaches: a lists b, which no dependency supports"}, 1},
		{"an edge from or to an ordered area", tree,
			append(slices.Clone(edges), Edge{"docs", "b"}, Edge{"b", "docs/gen"}),
			true, nil, 0},
		{"an edge from the catch-all", tree, append(slices.Clone(edges), Edge{"new", "b"}),
			true, []string{"reaches: core lacks b (new depends on b)"}, 1},
	} {
		got := check(s, c.files, c.edges, c.compared)
		if !slices.Equal(got.Lines, c.want) || got.Differences != c.diffs {
			t.Errorf("%s:\n got %q, %d differences\nwant %q, %d", c.name, got.Lines,
				got.Differences, c.want, c.diffs)
		}
	}

	// An area put in the map ahead of its tree: the open item that declares
	// it is named, and the line is not a difference. A done item's is.
	ahead, other := feature("adds-a", 1, "in_progress"), feature("other", 2, "queued")
	ahead.Areas, other.Areas = &[]string{"a", "docs"}, &[]string{"b"}
	s = state(other, ahead)
	s.Areas = m
	got := check(s, tree[2:], edges, true)
	want := []string{
		"pattern: a/ of area a matches no file: expected until adds-a merges",
		"pattern: tools/a/ of area a matches no file: expected until adds-a merges"}
	if !slices.Equal(got.Lines, want) || got.Differences != 0 {
		t.Fatalf("an area ahead of its tree:\n got %q, %d differences\nwant %q, 0",
			got.Lines, got.Differences, want)
	}
	// A tracked item's areas are not this map's, and a done item adds no tree.
	s.Config.Repository = "o/r"
	ahead.Repos = []string{"o/elsewhere"}
	if got = check(s, tree[2:], edges, true); got.Differences != 2 {
		t.Fatalf("declared by a tracked item: %q, %d differences; want 2", got.Lines,
			got.Differences)
	}
	ahead.Repos, ahead.Status = nil, "done"
	if got = check(s, tree[2:], edges, true); got.Differences != 2 {
		t.Fatalf("after the item is done: %q, %d differences; want 2", got.Lines,
			got.Differences)
	}
}

package coupling

import (
	"fmt"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1"
	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
)

const tmpl = "---\nfeature: \n---\n\n# T\n\n## Problem\n"

// design returns a design document with the given claim and otherwise valid
// FLOW-02 front-matter; with no rules it cites FLOW-01, since rules may not
// be empty.
func design(name string, drivers, seams, rs []string) string {
	if len(rs) == 0 {
		rs = []string{"FLOW-01"}
	}
	list := func(xs []string) string { return "[" + strings.Join(xs, ", ") + "]" }
	return fmt.Sprintf("---\nfeature: %s\nstatus: draft\ndesign_review: required\n"+
		"drivers: %s\nseams: %s\nsurfaces: []\nsecurity_boundary: none\n"+
		"alters_relationships: false\n"+
		"native_validation: []\ntier2_gaps: []\nrules: %s\nconcepts: [x]\n---\n\n# %s\n",
		name, list(drivers), list(seams), list(rs), name)
}

var base = map[string]string{
	"docs/design/TEMPLATE.md": tmpl,
	"README.md":               "readme\n",
	"main.go":                 "package main\n",
}

// checker commits change on a branch off a fresh scratch repo and returns a
// Checker over the diff from main.
func checker(t *testing.T, change map[string]string, explicit ...string) *Checker {
	t.Helper()
	r := gittest.New(t, base)
	r.Commit("change", change)
	d, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := rules.ParseConfig(tier1.RulesYAML)
	if err != nil {
		t.Fatal(err)
	}
	ix := &rules.Index{Rules: []rules.Rule{
		{ID: "BOOT-07", Severity: report.MustReview},
		{ID: "EXEC-05", Severity: report.Should},
	}}
	return &Checker{Diff: d, Cfg: cfg, Index: ix, Designs: LoadDesigns(d, explicit)}
}

func TestDesignUnion(t *testing.T) {
	c := checker(t, map[string]string{
		"docs/design/a.md":        design("a", []string{"qemu"}, nil, []string{"ORCH-01"}),
		"docs/design/b.md":        design("b", []string{"vz"}, []string{"relay-api"}, nil),
		"docs/design/TEMPLATE.md": tmpl + "\nedited\n",
		"main.go":                 "package main\n\nfunc x() {}\n",
	})
	ds := c.Designs
	if len(ds.List) != 2 {
		t.Fatalf("designs %+v; TEMPLATE.md must be ignored", ds.List)
	}
	if !ds.Drivers["qemu"] || !ds.Drivers["vz"] || len(ds.Drivers) != 2 ||
		!ds.Seams["relay-api"] || !ds.Rules["ORCH-01"] {
		t.Errorf("union %+v %+v %+v", ds.Drivers, ds.Seams, ds.Rules)
	}
	if got := c.NoDesignInDiff(); len(got) != 0 {
		t.Errorf("code with a design: %+v", got)
	}
}

func TestExplicitDesign(t *testing.T) {
	r := gittest.New(t, map[string]string{"docs/design/TEMPLATE.md": tmpl,
		"docs/design/old.md": design("old", []string{"vz"}, nil, []string{"X-01"})})
	r.Commit("code", map[string]string{"main.go": "package main\n"})
	d, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	ds := LoadDesigns(d, []string{"docs/design/old.md"})
	if len(ds.List) != 1 || !ds.Drivers["vz"] {
		t.Errorf("explicit design not used: %+v", ds)
	}
}

func TestNoDesignInDiff(t *testing.T) {
	cases := []struct {
		name     string
		change   map[string]string
		noDesign bool
		want     int
	}{
		{"code, no design", map[string]string{"relay/x.go": "package relay\n"}, false, 1},
		{"docs only", map[string]string{"docs/book/x.md": "x\n", "NOTES.md": "n\n"},
			false, 0},
		{"-no-design", map[string]string{"relay/x.go": "package relay\n"}, true, 0},
		{"bad front-matter still counts", map[string]string{
			"relay/x.go":       "package relay\n",
			"docs/design/z.md": "no front-matter\n"}, false, 0},
	}
	for _, tc := range cases {
		c := checker(t, tc.change)
		c.NoDesign = tc.noDesign
		got := c.NoDesignInDiff()
		if len(got) != tc.want {
			t.Errorf("%s: %+v", tc.name, got)
		}
		for _, f := range got {
			bad := f.Rule != "FLOW-01" || f.File != "relay/x.go"
			if bad || f.Severity != report.Must {
				t.Errorf("%s: finding %+v", tc.name, f)
			}
		}
	}
}

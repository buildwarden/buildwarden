package coupling

import (
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1"
	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
)

func d(drivers []string, rs ...string) string { return design("f", drivers, nil, rs) }

const pkg = "package x\n"

func TestCouplingChecks(t *testing.T) {
	cases := []struct {
		name   string
		change map[string]string
		check  func(*Checker) []report.Finding
		want   []string // a substring of each finding's message, in order
	}{
		{"driver claimed", map[string]string{"docs/design/f.md": d([]string{"qemu"}),
			"driver/qemu/x.go": pkg}, (*Checker).DriversClaim, nil},
		{"driver not claimed", map[string]string{"docs/design/f.md": d([]string{"qemu"}),
			"driver/vz/x.go": pkg}, (*Checker).DriversClaim,
			[]string{"reaches driver/vz/"}},
		{"shared with too few drivers", map[string]string{
			"docs/design/f.md": d([]string{"qemu", "vz"}), "relay/x.go": pkg},
			(*Checker).DriversClaim, []string{"must also declare [container hyperv]"}},
		{"shared with every driver", map[string]string{
			"docs/design/f.md":  d([]string{"container", "qemu", "vz", "hyperv"}),
			"driver/session.go": pkg}, (*Checker).DriversClaim, nil},
		{"shared under interface", map[string]string{
			"docs/design/f.md": d([]string{"interface"}), "driver/vz/x.go": pkg,
			"ledger/x.go": pkg}, (*Checker).DriversClaim, nil},
		{"surface without bootstrap.md", map[string]string{"docs/design/f.md": d(nil),
			"cmd/warden-io/trust_linux.go": pkg}, (*Checker).BootstrapSurface,
			[]string{"is on the bootstrap surface"}},
		{"surface with bootstrap.md", map[string]string{
			"docs/design/f.md":              d(nil, "BOOT-07"),
			"docs/development/bootstrap.md": "m\n", "bootstrap/x.go": pkg},
			(*Checker).BootstrapSurface, nil},
		{"bootstrap.md with no design citing BOOT-07", map[string]string{
			"docs/design/f.md":              d(nil, "BOOT-01"),
			"docs/development/bootstrap.md": "m\n"},
			(*Checker).BootstrapSurface, []string{"no design in the diff cites"}},
		{"test changed with code", map[string]string{"relay/x.go": pkg,
			"relay/x_test.go": pkg}, (*Checker).TestsWithChange, nil},
		{"code without test", map[string]string{"relay/x.go": pkg, "relay/a/y.go": pkg,
			"relay/a/testdata/z.go": pkg, "relay/a/y_test.go": pkg},
			(*Checker).TestsWithChange, []string{"changed in relay/ and none"}},
	}
	for _, tc := range cases {
		got := tc.check(checker(t, tc.change))
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d findings, want %d: %+v",
				tc.name, len(got), len(tc.want), got)
			continue
		}
		for i, w := range tc.want {
			if !strings.Contains(got[i].Message, w) {
				t.Errorf("%s: %q lacks %q", tc.name, got[i].Message, w)
			}
		}
	}
}

func TestSeverities(t *testing.T) {
	c := checker(t, map[string]string{"docs/design/f.md": d(nil),
		"cmd/warden-io/jks.go": pkg, "relay/x.go": pkg})
	for _, f := range c.All() {
		want := map[string]report.Severity{"BOOT-07": report.Hint,
			"ORCH-01": report.Must, "GLOBAL-04": report.Hint}[f.Rule]
		if f.Severity != want {
			t.Errorf("%s: severity %q, want %q", f.Rule, f.Severity, want)
		}
	}
}

func TestBootstrapDocSeverity(t *testing.T) {
	c := checker(t, map[string]string{"docs/design/f.md": d(nil),
		"docs/development/bootstrap.md": "m\n"})
	got := c.BootstrapSurface()
	if len(got) != 1 || got[0].Severity != report.MustReview {
		t.Errorf("bootstrap.md with no BOOT-07 design must stay MUST-review: %+v", got)
	}
}

func TestCommentOnlyChange(t *testing.T) {
	r := gittest.New(t, map[string]string{"docs/design/TEMPLATE.md": tmpl,
		"driver/fixable.go": "package driver\n\n// Old says.\nfunc F() {}\n",
		"driver/vz/a.go":    "package vz\n\nfunc G() {}\n"})
	r.Commit("comments", map[string]string{"docs/design/f.md": d(nil),
		"driver/fixable.go": "package driver\n\n// New says, at length.\nfunc F() {\n}\n",
		"driver/vz/a.go":    "package vz\n\nfunc G() { _ = 1 }\n"})
	dd, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := rules.ParseConfig(tier1.RulesYAML)
	c := &Checker{Diff: dd, Cfg: cfg, Index: &rules.Index{}, Designs: LoadDesigns(dd, nil)}
	got := c.DriversClaim()
	if len(got) != 1 || got[0].File != "driver/vz/a.go" {
		t.Errorf("a comment-only change reached the shared package: %+v", got)
	}
}

func TestSeamContract(t *testing.T) {
	c := checker(t, map[string]string{"docs/design/f.md": d(nil), "relaycfg/x.go": pkg})
	if got := c.SeamContract(); len(got) != 0 {
		t.Errorf("no seam has a contract document yet: %+v", got)
	}
	c.Cfg.Seams = []rules.Seam{{Name: "claim", Code: []string{"relaycfg/"},
		Contract: "docs/claim.md"}}
	got := c.SeamContract()
	if len(got) != 1 || got[0].Rule != "GLOBAL-08" || got[0].File != "relaycfg/x.go" {
		t.Errorf("seam change without its contract: %+v", got)
	}
}

// specsBase is a well-formed specs/ tree: the index, the conformance test,
// a prose-only ledger spec and one schema with its document.
var specsBase = map[string]string{
	"docs/design/TEMPLATE.md":         tmpl,
	"specs/README.md":                 "index\n",
	"specs/specs_test.go":             "package specs_test\n",
	"specs/build-ledger/v1.md":        "wire\n",
	"specs/metadata-a/v1.md":          "a\n",
	"specs/metadata-a/v1.schema.json": "{}\n",
	"schemas/v1/b.json":               "{}\n",
}

// specsChecker commits change, with msg as the commit message, on a branch
// off a repo holding specsBase.
func specsChecker(t *testing.T, msg string, change map[string]string) *Checker {
	t.Helper()
	r := gittest.New(t, specsBase)
	r.Commit(msg, change)
	dd, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := rules.ParseConfig(tier1.RulesYAML)
	if err != nil {
		t.Fatal(err)
	}
	return &Checker{Diff: dd, Cfg: cfg, Index: &rules.Index{}, Designs: LoadDesigns(dd, nil)}
}

func TestSchemaDoc(t *testing.T) {
	const (
		md     = "specs/metadata-a/v1.md"
		schema = "specs/metadata-a/v1.schema.json"
	)
	cases := []struct {
		name   string
		change map[string]string
		want   []string // a substring of each finding's message, in order
	}{
		{"schema without its document", map[string]string{schema: `{"a":1}` + "\n"},
			[]string{schema + " changed without " + md}},
		{"document without its schema", map[string]string{md: "a, fixed\n"},
			[]string{md + " changed without " + schema}},
		{"both changed", map[string]string{schema: `{"a":1}` + "\n", md: "a, fixed\n"},
			nil},
		{"ledger spec alone", map[string]string{"specs/build-ledger/v1.md": "wire 2\n"},
			nil},
		{"index and test alone", map[string]string{"specs/README.md": "index 2\n",
			"specs/specs_test.go": "package specs_test\n\n// x\n"}, nil},
		{"schema moved in with its document", map[string]string{"schemas/v1/b.json": "",
			"specs/metadata-b/v1.schema.json": "{}\n",
			"specs/metadata-b/v1.md":          "b\n"},
			nil},
		{"new major with no document", map[string]string{
			"specs/metadata-a/v2.schema.json": "{}\n"},
			[]string{"changed without specs/metadata-a/v2.md",
				"v2.schema.json has no specs/metadata-a/v2.md beside it"}},
		{"schema in a prose-only directory", map[string]string{
			"specs/build-ledger/v1.schema.json": "{}\n"},
			[]string{"prose-only specs/build-ledger/"}},
		{"stray names", map[string]string{"specs/metadata-a/notes.md": "n\n",
			"specs/extra.md": "e\n", "specs/metadata-a/x/v1.md": "x\n"},
			[]string{"specs/extra.md is not", "specs/metadata-a/notes.md is not",
				"specs/metadata-a/x/v1.md is not"}},
	}
	for _, tc := range cases {
		got := specsChecker(t, "change", tc.change).SchemaDoc()
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d findings, want %d: %+v",
				tc.name, len(got), len(tc.want), got)
			continue
		}
		for i, w := range tc.want {
			if !strings.Contains(got[i].Message, w) || got[i].Rule != "RELAY-05" ||
				got[i].Severity != report.Must {
				t.Errorf("%s: %+v, want a RELAY-05 MUST containing %q",
					tc.name, got[i], w)
			}
		}
	}
}

func TestSchemaDocAllowed(t *testing.T) {
	c := specsChecker(t, "fix a typo\n\n"+
		"Tier1-Allow: RELAY-05 specs/metadata-a/*: prose-only typo fix",
		map[string]string{"specs/metadata-a/v1.md": "a, fixed\n"})
	r := &report.Report{}
	r.Add(c.SchemaDoc()...)
	c.Diff.Suppress(r)
	if len(r.Findings) != 0 || len(r.Suppressed) != 1 {
		t.Errorf("findings %+v, suppressed %+v", r.Findings, r.Suppressed)
	}
}

// TestSchemaDocWholeTree: the specs/ layout is checked on HEAD's tree only
// when the diff touches specs/, unless WholeTree (`tier1 -all`) asks for an
// audit of the tree as it stands (design specs-migration, Decision 6).
func TestSchemaDocWholeTree(t *testing.T) {
	bad := map[string]string{}
	for k, v := range specsBase {
		bad[k] = v
	}
	bad["specs/stray.md"] = "s\n"
	r := gittest.New(t, bad)
	r.Commit("unrelated", map[string]string{"README.md": "r\n"})
	dd, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := rules.ParseConfig(tier1.RulesYAML)
	if err != nil {
		t.Fatal(err)
	}
	c := &Checker{Diff: dd, Cfg: cfg, Index: &rules.Index{}, Designs: LoadDesigns(dd, nil)}
	if got := c.SchemaDoc(); len(got) != 0 {
		t.Errorf("diff mode, specs/ untouched: %+v, want none", got)
	}
	c.WholeTree = true
	got := c.SchemaDoc()
	if len(got) != 1 || !strings.Contains(got[0].Message, "specs/stray.md is not") {
		t.Errorf("whole-tree mode: %+v, want the stray file", got)
	}
}

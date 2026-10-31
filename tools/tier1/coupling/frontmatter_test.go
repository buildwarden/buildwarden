package coupling

import (
	"reflect"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/tools/tier1"
	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/internal/gittest"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
)

// validFront is a design named x whose front-matter satisfies FLOW-02, one
// key per line so a case can drop or replace a line.
const validFront = `---
feature: x
status: draft
round: parallel
drivers: [qemu]
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [FLOW-02]
concepts: [front-matter]
---

# x
`

// withLine returns validFront with the line starting "key:" replaced by
// repl, or dropped when repl is empty.
func withLine(key, repl string) string { return edit(validFront, key, repl) }

// edit is withLine over any text.
func edit(text, key, repl string) string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, key+":") {
			if repl == "" {
				continue
			}
			l = repl
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		text      string
		tolerate  []string
		pre       bool
		problems  []string
		tolerated []string
		preStd    bool
	}{
		{name: "valid", text: validFront},
		{name: "feature is the file stem", path: "docs/design/y.md", text: validFront,
			problems: []string{`feature "x", queue says "y"`}},
		{name: "status enum", text: withLine("status", "status: done"),
			problems: []string{`status "done"`}},
		{name: "security_boundary enum",
			text:     withLine("security_boundary", "security_boundary: host"),
			problems: []string{`security_boundary "host"`}},
		{name: "rules empty", text: withLine("rules", "rules: []"),
			problems: []string{"rules is empty"}},
		{name: "concepts empty", text: withLine("concepts", "concepts: []"),
			problems: []string{"concepts is empty"}},
		{name: "native_validation incomplete",
			text: withLine("native_validation", "native_validation: [{host: vz}]"),
			problems: []string{"native_validation {Host:vz Blocking:<nil> Reason:} " +
				"needs host, blocking and reason"}},
		{name: "no front-matter", text: "# x\n", problems: []string{"no front-matter"}},
		{name: "no front-matter, pre-standard", text: "# x\n", pre: true, preStd: true},
		{name: "valid with pre-standard allowed", text: validFront, pre: true},
		{name: "not closed", text: "---\nfeature: x\n",
			problems: []string{"front-matter not closed"}},
		{name: "tolerated key", text: withLine("alters_relationships", ""),
			tolerate:  []string{"alters_relationships"},
			tolerated: []string{"missing alters_relationships"}},
		{name: "tolerance is per key",
			text: edit(withLine("alters_relationships", ""),
				"status", "status: done"),
			tolerate:  []string{"alters_relationships"},
			problems:  []string{`status "done"`},
			tolerated: []string{"missing alters_relationships"}},
	}
	// Every FLOW-02 key, dropped, is reported missing.
	for _, k := range []string{"feature", "status", "round", "drivers", "seams",
		"surfaces", "security_boundary", "alters_relationships", "native_validation",
		"tier2_gaps", "rules", "concepts"} {
		v := Validate("docs/design/x.md", withLine(k, ""), tmpl, nil, false)
		if len(v.Problems) == 0 || v.Problems[0] != "missing "+k {
			t.Errorf("missing %s: problems %q", k, v.Problems)
		}
	}
	for _, c := range cases {
		p := c.path
		if p == "" {
			p = "docs/design/x.md"
		}
		v := Validate(p, c.text, tmpl, c.tolerate, c.pre)
		if v.PreStandard != c.preStd || !reflect.DeepEqual(v.Problems, c.problems) ||
			!reflect.DeepEqual(v.Tolerated, c.tolerated) {
			t.Errorf("%s: got pre %v problems %q tolerated %q; want %v %q %q",
				c.name, v.PreStandard, v.Problems, v.Tolerated,
				c.preStd, c.problems, c.tolerated)
		}
	}
}

func TestProblemKey(t *testing.T) {
	for in, want := range map[string]string{
		"missing alters_relationships":  "alters_relationships",
		`feature "a", queue says "b"`:   "feature",
		`status "done"`:                 "status",
		"rules is empty":                "rules",
		"native_validation {...} needs": "native_validation",
		"":                              "",
	} {
		if got := problemKey(in); got != want {
			t.Errorf("problemKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// loadDiff commits change on a branch over files and returns the diff from
// main, with rules.yaml's config.
func loadDiff(t *testing.T, files, change map[string]string) (*diff.Diff, *rules.Config) {
	t.Helper()
	r := gittest.New(t, files)
	r.Commit("change", change)
	d, err := diff.Load(r.Dir, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := rules.ParseConfig(tier1.RulesYAML)
	if err != nil {
		t.Fatal(err)
	}
	return d, cfg
}

func TestTouchedDesigns(t *testing.T) {
	d, cfg := loadDiff(t, map[string]string{
		"docs/design/TEMPLATE.md": tmpl,
		"docs/design/old.md":      "# Old plan\n",
		"docs/design/moved.md":    "# Moved plan\n\nlong enough to be seen as a rename\n",
		"docs/design/had.md":      edit(validFront, "feature", "feature: had"),
		"docs/design/keep.md":     "# Untouched\n",
	}, map[string]string{
		"docs/design/old.md":       "# Old plan\n\nedited\n",
		"docs/design/moved.md":     "",
		"docs/design/renamed.md":   "# Moved plan\n\nlong enough to be seen as a rename\n",
		"docs/design/had.md":       "# Front-matter removed\n",
		"docs/design/new.md":       "# New, no front-matter\n",
		"docs/design/x.md":         validFront,
		"docs/design/bad.md":       withLine("status", "status: done"),
		"docs/design/TEMPLATE.md":  tmpl + "\nedited\n",
		"docs/design/sub/other.md": "# Not a design\n",
	})
	cfg.DesignTolerate = map[string][]string{"docs/design/bad.md": {"rules"}}
	got := map[string]Verdict{}
	for _, v := range TouchedDesigns(d, cfg) {
		got[v.Path] = v
	}
	want := map[string]Verdict{
		"docs/design/old.md":     {Path: "docs/design/old.md", PreStandard: true},
		"docs/design/renamed.md": {Path: "docs/design/renamed.md", PreStandard: true},
		"docs/design/had.md": {Path: "docs/design/had.md",
			Problems: []string{"no front-matter"}},
		"docs/design/new.md": {Path: "docs/design/new.md",
			Problems: []string{"no front-matter"}},
		"docs/design/x.md": {Path: "docs/design/x.md"},
		"docs/design/bad.md": {Path: "docs/design/bad.md", Problems: []string{
			`feature "x", queue says "bad"`, `status "done"`}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("verdicts\n got %+v\nwant %+v", got, want)
	}
}

func TestAllDesigns(t *testing.T) {
	r := gittest.New(t, map[string]string{
		"docs/design/TEMPLATE.md": tmpl,
		"docs/design/old.md":      "# Pre-standard\n",
		"docs/design/x.md":        validFront,
		"docs/design/archived.md": strings.Replace(withLine("alters_relationships", ""),
			"feature: x", "feature: archived", 1),
		"docs/design/bad.md": "---\nfeature: bad\n---\n",
	})
	d, err := diff.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &rules.Config{DesignTolerate: map[string][]string{
		"docs/design/archived.md": {"alters_relationships"}}}
	vs := AllDesigns(d, cfg)
	var paths []string
	for _, v := range vs {
		paths = append(paths, v.Path)
	}
	if want := []string{"docs/design/archived.md", "docs/design/bad.md",
		"docs/design/old.md", "docs/design/x.md"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths %q, want %q", paths, want)
	}
	if v := vs[0]; len(v.Problems) != 0 || len(v.Tolerated) != 1 {
		t.Errorf("archived: %+v", v)
	}
	if v := vs[1]; len(v.Problems) < 2 {
		t.Errorf("bad: %+v", v)
	}
	if v := vs[2]; !v.PreStandard || len(v.Problems) != 0 {
		t.Errorf("pre-standard: %+v", v)
	}
	if v := vs[3]; len(v.Problems) != 0 || v.PreStandard {
		t.Errorf("valid: %+v", v)
	}
}

func TestFrontMatterFindings(t *testing.T) {
	c := checker(t, map[string]string{
		"docs/design/x.md": withLine("status", "status: done"),
	})
	got := c.FrontMatter()
	if len(got) != 1 || got[0].Rule != "FLOW-02" || got[0].Severity != report.Must ||
		got[0].Check != "design-front-matter" || got[0].File != "docs/design/x.md" {
		t.Fatalf("findings %+v", got)
	}
	fs := Findings([]Verdict{
		{Path: "docs/design/p.md", PreStandard: true},
		{Path: "docs/design/t.md", Tolerated: []string{"missing alters_relationships"}},
		{Path: "docs/design/f.md", Problems: []string{`feature "a", queue says "f"`}},
	}, report.Must)
	if len(fs) != 3 {
		t.Fatalf("findings %+v", fs)
	}
	if fs[0].Rule != "FLOW-12" || fs[0].Severity != report.Hint {
		t.Errorf("pre-standard: %+v", fs[0])
	}
	if fs[1].Rule != "FLOW-02" || fs[1].Severity != report.Hint {
		t.Errorf("tolerated: %+v", fs[1])
	}
	if fs[2].Severity != report.Must || !strings.Contains(fs[2].Message, `file stem "f"`) {
		t.Errorf("feature problem: %+v", fs[2])
	}
}

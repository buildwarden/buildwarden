package coupling

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
	"github.com/buildwarden/buildwarden/tools/workflow/check"
)

// Verdict is one design's FLOW-02 front-matter result.
type Verdict struct {
	Path string
	// PreStandard is set for a design with no front-matter that FLOW-12
	// allows: one written before FLOW-02, not yet retrofitted.
	PreStandard bool
	// Problems are the ways the front-matter breaks FLOW-02; any fails tier 1.
	Problems []string
	// Tolerated are problems rules.yaml's design_tolerate lists for this
	// design, reported without failing.
	Tolerated []string
}

// Validate checks one design's text against FLOW-02 with wf's own validator,
// check.Front.Problems, taking the feature name from the file stem.
// preStandardOK says whether a design with no front-matter is pre-standard
// rather than a problem. tolerate lists the front-matter keys whose problems
// are moved to Tolerated.
func Validate(p, text, tmpl string, tolerate []string, preStandardOK bool) Verdict {
	v := Verdict{Path: p}
	if !strings.HasPrefix(text, "---\n") && preStandardOK {
		v.PreStandard = true
		return v
	}
	d, err := check.ParseDesign(text, tmpl)
	if err != nil {
		v.Problems = []string{err.Error()}
		return v
	}
	for _, pr := range d.Front.Problems(strings.TrimSuffix(path.Base(p), ".md")) {
		if contains(tolerate, problemKey(pr)) {
			v.Tolerated = append(v.Tolerated, pr)
		} else {
			v.Problems = append(v.Problems, pr)
		}
	}
	return v
}

// problemKey is the front-matter key a check.Front.Problems entry is about:
// "missing <key>", or otherwise its first word ("status \"x\"", "rules is
// empty").
func problemKey(problem string) string {
	f := strings.Fields(strings.TrimPrefix(problem, "missing "))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// TouchedDesigns validates, at HEAD, every docs/design/*.md other than
// TEMPLATE.md that the diff adds, modifies or renames. One with no
// front-matter is pre-standard only when it had none at base either: a new
// design, or one whose front-matter was removed, breaks FLOW-02.
func TouchedDesigns(d *diff.Diff, cfg *rules.Config) []Verdict {
	tmpl, _ := d.Show(d.Head, template)
	var out []Verdict
	for _, f := range d.Files {
		if f.Status == 'D' || !isDesign(f.Path) {
			continue
		}
		text, _ := d.Show(d.Head, f.Path)
		old := f.Path
		if f.Old != "" {
			old = f.Old
		}
		before, existed := d.Show(d.Base, old)
		pre := existed && !strings.HasPrefix(before, "---\n")
		out = append(out, Validate(f.Path, text, tmpl, cfg.DesignTolerate[f.Path], pre))
	}
	return out
}

// AllDesigns validates every docs/design/*.md at HEAD other than
// TEMPLATE.md. Any design with no front-matter is listed as pre-standard, so
// the archive-wide check passes until every such design is retrofitted
// (FLOW-12); the per-diff check is what stops a new one.
func AllDesigns(d *diff.Diff, cfg *rules.Config) []Verdict {
	tmpl, _ := d.Show(d.Head, template)
	var out []Verdict
	for _, p := range d.List(d.Head, designDir) {
		if !isDesign(p) {
			continue
		}
		text, _ := d.Show(d.Head, p)
		out = append(out, Validate(p, text, tmpl, cfg.DesignTolerate[p], true))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Findings turns verdicts into report entries: each problem is a FLOW-02
// finding at sev, and each pre-standard design or tolerated problem is a hint.
func Findings(vs []Verdict, sev report.Severity) []report.Finding {
	var out []report.Finding
	add := func(rule string, s report.Severity, p, msg string) {
		out = append(out, report.Finding{Rule: rule, Severity: s, File: p,
			Check: "design-front-matter", Message: msg})
	}
	for _, v := range vs {
		stem := strings.TrimSuffix(path.Base(v.Path), ".md")
		if v.PreStandard {
			add("FLOW-12", report.Hint, v.Path, "no front-matter: the design "+
				"predates FLOW-02; retrofit it when a new design relies on it")
		}
		for _, pr := range v.Problems {
			msg := "front-matter: " + pr
			if problemKey(pr) == "feature" {
				msg += fmt.Sprintf(" (checked against the file stem %q)", stem)
			}
			add("FLOW-02", sev, v.Path, msg)
		}
		for _, pr := range v.Tolerated {
			add("FLOW-02", report.Hint, v.Path, fmt.Sprintf(
				"front-matter: %s (tolerated by rules.yaml design_tolerate)", pr))
		}
	}
	return out
}

// FrontMatter is the FLOW-02 half of design-front-matter: every design the
// diff touches has valid front-matter.
func (c *Checker) FrontMatter() []report.Finding {
	return Findings(TouchedDesigns(c.Diff, c.Cfg), c.Index.Severity("FLOW-02"))
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

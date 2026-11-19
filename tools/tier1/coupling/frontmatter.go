package coupling

import (
	"fmt"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
	"github.com/buildwarden/buildwarden/tools/tier1/rules"
	"github.com/buildwarden/buildwarden/tools/workflow/check"
)

// The design-front-matter check is wf's (tools/workflow/check/frontmatter.go,
// FLOW-02, FLOW-12), which a component's CI runs as wf rules; tier 1 calls it
// here on its diff, so its report is unchanged.

// Verdict is one design's FLOW-02 front-matter result.
type Verdict = check.Verdict

// profileFile is this repository's profile file, whose impact keys a design
// here carries and whose designs key says where designs are (PROFILE-01,
// LOOP-01).
const profileFile = "docs/development/loop-profile.yaml"

// headDesigns reads the profile file at the diff's head, as wf reads it from
// the branch.
func headDesigns(d *diff.Diff) (check.Designs, error) {
	text, ok := d.Show(d.Head, profileFile)
	if !ok {
		return check.Designs{}, fmt.Errorf("no profile file %s at %s", profileFile, d.Head)
	}
	p, err := check.ParseProfile(profileFile, []byte(text))
	if err != nil {
		return check.Designs{}, err
	}
	return check.DesignsOf(p), nil
}

// Validate checks one design's text against FLOW-02 (check.Validate).
func Validate(p, text, tmpl string, prof *check.Profile, tolerate []string,
	preStandardOK bool,
) Verdict {
	return check.Validate(p, text, tmpl, prof, tolerate, preStandardOK)
}

// problemKey is the front-matter key a problem is about (check.ProblemKey).
func problemKey(problem string) string { return check.ProblemKey(problem) }

// TouchedDesigns validates, at HEAD, every design the diff adds, modifies or
// renames (check.TouchedDesigns).
func TouchedDesigns(d *diff.Diff, cfg *rules.Config) []Verdict {
	ds, err := headDesigns(d)
	if err != nil {
		return []Verdict{{Path: profileFile, Problems: []string{err.Error()}}}
	}
	var files []check.Changed
	for _, f := range d.Files {
		files = append(files, check.Changed{Status: f.Status, Path: f.Path, Old: f.Old})
	}
	return check.TouchedDesigns(d, d.Base, d.Head, files, ds, cfg.DesignTolerate)
}

// AllDesigns validates every design at HEAD (check.AllDesigns).
func AllDesigns(d *diff.Diff, cfg *rules.Config) []Verdict {
	ds, err := headDesigns(d)
	if err != nil {
		return []Verdict{{Path: profileFile, Problems: []string{err.Error()}}}
	}
	return check.AllDesigns(d, d.Head, ds, cfg.DesignTolerate)
}

// Findings turns verdicts into report entries: each problem is a FLOW-02
// finding at sev, and each pre-standard design or tolerated problem is a hint.
func Findings(vs []Verdict, sev report.Severity) []report.Finding {
	var out []report.Finding
	for _, f := range check.DesignFindings(vs, string(sev)) {
		out = append(out, report.Finding{Rule: f.Rule,
			Severity: report.Severity(f.Severity),
			File:     f.File, Line: f.Line, Check: f.Check, Message: f.Message})
	}
	return out
}

// FrontMatter is the FLOW-02 half of design-front-matter: every design the
// diff touches has valid front-matter.
func (c *Checker) FrontMatter() []report.Finding {
	return Findings(TouchedDesigns(c.Diff, c.Cfg), c.Index.Severity("FLOW-02"))
}

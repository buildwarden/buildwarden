package check

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Verdict is one design's FLOW-02 front-matter result.
type Verdict struct {
	Path string
	// PreStandard is set for a design with no front-matter that FLOW-12
	// allows: one written before FLOW-02, not yet retrofitted.
	PreStandard bool
	// Problems are the ways the front-matter breaks FLOW-02; any fails.
	Problems []string
	// Tolerated are problems the rule-to-check map's design_tolerate lists
	// for this design, reported without failing.
	Tolerated []string
}

// Tree is the git access the front-matter check needs: a file at a
// revision, and the paths under a directory there.
type Tree interface {
	Show(rev, path string) (string, bool)
	List(rev, dir string) []string
}

// Changed is one path a diff changes: git's status letter, its path, and
// for a rename its old path.
type Changed struct {
	Status    byte
	Path, Old string
}

// Designs is where a component keeps its designs, from its profile file's
// designs key, and the profile that names its front-matter keys.
type Designs struct {
	Dir, Template string
	Profile       *Profile
}

// DesignsOf reads the designs key of a profile; a profile that has none has
// no designs to check.
func DesignsOf(p *Profile) Designs {
	return Designs{Dir: strings.TrimSuffix(p.Designs.Dir, "/"), Template: p.Designs.Template,
		Profile: p}
}

// isDesign reports whether p is a design: a .md file directly in the
// designs directory other than the template.
func (ds Designs) isDesign(p string) bool {
	return ds.Dir != "" && p != ds.Template && path.Dir(p) == ds.Dir &&
		strings.HasSuffix(p, ".md")
}

// Validate checks one design's text against FLOW-02 with Front.Problems,
// taking the feature name from the file stem and the impact keys from the
// profile. preStandardOK says whether a design with no front-matter is
// pre-standard rather than a problem (FLOW-12). tolerate lists the
// front-matter keys whose problems are moved to Tolerated.
func Validate(p, text, tmpl string, prof *Profile, tolerate []string,
	preStandardOK bool,
) Verdict {
	v := Verdict{Path: p}
	if !strings.HasPrefix(text, "---\n") && preStandardOK {
		v.PreStandard = true
		return v
	}
	d, err := ParseDesign(text, tmpl)
	if err != nil {
		v.Problems = []string{err.Error()}
		return v
	}
	for _, pr := range d.Front.Problems(strings.TrimSuffix(path.Base(p), ".md"), prof) {
		if oneOf(ProblemKey(pr), tolerate...) {
			v.Tolerated = append(v.Tolerated, pr)
		} else {
			v.Problems = append(v.Problems, pr)
		}
	}
	return v
}

// ProblemKey is the front-matter key a Front.Problems entry is about:
// "missing <key>", or otherwise its first word ("status \"x\"", "rules is
// empty").
func ProblemKey(problem string) string {
	f := strings.Fields(strings.TrimPrefix(problem, "missing "))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// TouchedDesigns validates, at head, every design the change adds, modifies
// or renames. One with no front-matter is pre-standard only when it had none
// at base either: a new design, or one whose front-matter was removed,
// breaks FLOW-02.
func TouchedDesigns(t Tree, base, head string, files []Changed, ds Designs,
	tolerate map[string][]string,
) []Verdict {
	tmpl, _ := t.Show(head, ds.Template)
	var out []Verdict
	for _, f := range files {
		if f.Status == 'D' || !ds.isDesign(f.Path) {
			continue
		}
		text, _ := t.Show(head, f.Path)
		old := f.Path
		if f.Old != "" {
			old = f.Old
		}
		before, existed := t.Show(base, old)
		pre := existed && !strings.HasPrefix(before, "---\n")
		out = append(out, Validate(f.Path, text, tmpl, ds.Profile, tolerate[f.Path], pre))
	}
	return out
}

// AllDesigns validates every design at head. Any design with no
// front-matter is listed as pre-standard, so the archive-wide check passes
// until every such design is retrofitted (FLOW-12); the per-diff check is
// what stops a new one.
func AllDesigns(t Tree, head string, ds Designs, tolerate map[string][]string) []Verdict {
	tmpl, _ := t.Show(head, ds.Template)
	var out []Verdict
	for _, p := range t.List(head, ds.Dir+"/") {
		if !ds.isDesign(p) {
			continue
		}
		text, _ := t.Show(head, p)
		out = append(out, Validate(p, text, tmpl, ds.Profile, tolerate[p], true))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// DesignFindings turns verdicts into findings at sev: each problem a FLOW-02
// finding, and each pre-standard design or tolerated problem a hint.
func DesignFindings(vs []Verdict, sev string) []Finding {
	var out []Finding
	add := func(rule, s, p, msg string) {
		out = append(out, Finding{Rule: rule, Severity: s, File: p,
			Check: "design-front-matter", Message: msg})
	}
	for _, v := range vs {
		stem := strings.TrimSuffix(path.Base(v.Path), ".md")
		if v.PreStandard {
			add("FLOW-12", Hint, v.Path, "no front-matter: the design "+
				"predates FLOW-02; retrofit it when a new design relies on it")
		}
		for _, pr := range v.Problems {
			msg := "front-matter: " + pr
			if ProblemKey(pr) == "feature" {
				msg += fmt.Sprintf(" (checked against the file stem %q)", stem)
			}
			add("FLOW-02", sev, v.Path, msg)
		}
		for _, pr := range v.Tolerated {
			add("FLOW-02", Hint, v.Path, fmt.Sprintf(
				"front-matter: %s (tolerated by rules.yaml design_tolerate)", pr))
		}
	}
	return out
}

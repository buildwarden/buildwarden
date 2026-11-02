// Package coupling holds tier 1's file-level checks: each compares the paths
// a diff changed with other paths it should have changed with, or with the
// claim in the designs the diff carries (LINT-02, Decision 6).
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

const (
	designDir = "docs/design/"
	template  = designDir + "TEMPLATE.md"
)

// Design is one design the diff carries, parsed at HEAD. Err is set when
// its front-matter does not parse; it then adds nothing to the union.
type Design struct {
	Path   string
	Design *check.Design
	Err    error
}

// Designs is the claim: the union of the carried designs' front-matter.
type Designs struct {
	List    []Design
	Drivers map[string]bool
	Seams   map[string]bool
	Rules   map[string]bool
}

// LoadDesigns parses the designs named by explicit, or else every
// docs/design/*.md the diff adds or modifies other than TEMPLATE.md.
func LoadDesigns(d *diff.Diff, explicit []string) *Designs {
	paths := explicit
	if len(paths) == 0 {
		for _, f := range d.Files {
			if f.Status != 'D' && isDesign(f.Path) {
				paths = append(paths, f.Path)
			}
		}
	}
	tmpl, _ := d.Show(d.Head, template)
	ds := &Designs{Drivers: map[string]bool{}, Seams: map[string]bool{},
		Rules: map[string]bool{}}
	for _, p := range paths {
		one := Design{Path: p}
		text, ok := d.Show(d.Head, p)
		if !ok {
			one.Err = fmt.Errorf("%s does not exist at HEAD", p)
		} else {
			one.Design, one.Err = check.ParseDesign(text, tmpl)
		}
		if one.Err == nil {
			addAll(ds.Drivers, one.Design.Front.Drivers)
			addAll(ds.Seams, one.Design.Front.Seams)
			addAll(ds.Rules, one.Design.Front.Rules)
		}
		ds.List = append(ds.List, one)
	}
	return ds
}

func isDesign(p string) bool {
	return p != template && path.Dir(p)+"/" == designDir && strings.HasSuffix(p, ".md")
}

func addAll(set map[string]bool, xs []string) {
	for _, x := range xs {
		set[x] = true
	}
}

// Checker runs the coupling checks over one diff.
type Checker struct {
	Diff     *diff.Diff
	Cfg      *rules.Config
	Index    *rules.Index
	Designs  *Designs
	NoDesign bool
	// WholeTree also runs the checks of invariants that hold over the whole
	// tree (the specs/ layout) when the diff does not touch them: `tier1
	// -all`, for an audit of HEAD rather than of a change.
	WholeTree bool
	changed   []string // the paths() cache
}

func (c *Checker) finding(rule, check, file, format string, a ...any) report.Finding {
	return report.Finding{Rule: rule, Severity: c.Index.Severity(rule), File: file,
		Check: check, Message: fmt.Sprintf(format, a...)}
}

// isCode reports whether a changed path is code for FLOW-01: anything not
// matched by rules.yaml's not_code.
func (c *Checker) isCode(p string) bool {
	for _, g := range c.Cfg.NotCode {
		if diff.Match(g, p) {
			return false
		}
	}
	return true
}

// NoDesignInDiff is the FLOW-01 half of design-front-matter: code changed and
// the diff carries no design, unless -no-design says none is needed.
func (c *Checker) NoDesignInDiff() []report.Finding {
	if c.NoDesign || len(c.Designs.List) > 0 {
		return nil
	}
	var code []string
	for _, f := range c.Diff.Files {
		if c.isCode(f.Path) {
			code = append(code, f.Path)
		}
	}
	if len(code) == 0 {
		return nil
	}
	sort.Strings(code)
	return []report.Finding{c.finding("FLOW-01", "design-front-matter", code[0],
		"%d code path(s) changed and no docs/design/*.md is in the diff; add or "+
			"update the feature's design, or pass -no-design (the no-design label)",
		len(code))}
}

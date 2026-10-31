package coupling

import (
	"go/scanner"
	"go/token"
	"path"
	"sort"
	"strings"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// All runs every coupling check in this file and both halves of
// design-front-matter.
func (c *Checker) All() []report.Finding {
	var out []report.Finding
	for _, check := range []func() []report.Finding{
		c.NoDesignInDiff, c.FrontMatter, c.DriversClaim, c.BootstrapSurface, c.SchemaDoc,
		c.SeamContract, c.TestsWithChange,
	} {
		out = append(out, check()...)
	}
	return out
}

// paths is every path the diff changed, both sides of a rename included,
// sorted. A Go file whose change is only comments or layout is left out:
// history showed comment fixes in shared driver files failing drivers-claim
// (task 11).
func (c *Checker) paths() []string {
	if c.changed != nil {
		return c.changed
	}
	c.changed = []string{}
	for _, f := range c.Diff.Files {
		if !c.codeChanged(f) {
			continue
		}
		c.changed = append(c.changed, f.Path)
		if f.Old != "" {
			c.changed = append(c.changed, f.Old)
		}
	}
	sort.Strings(c.changed)
	return c.changed
}

// codeChanged reports whether f changes anything but the comments and layout
// of a Go file.
func (c *Checker) codeChanged(f diff.File) bool {
	if f.Status != 'M' || !strings.HasSuffix(f.Path, ".go") {
		return true
	}
	a, okA := c.Diff.Show(c.Diff.Base, f.Path)
	b, okB := c.Diff.Show(c.Diff.Head, f.Path)
	return !okA || !okB || !sameTokens(a, b)
}

// sameTokens reports whether two Go sources have the same token stream,
// ignoring comments and the semicolons the scanner inserts at line ends.
func sameTokens(a, b string) bool {
	ta, tb := goTokens(a), goTokens(b)
	if len(ta) != len(tb) {
		return false
	}
	for i := range ta {
		if ta[i] != tb[i] {
			return false
		}
	}
	return true
}

func goTokens(src string) []string {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, []byte(src), nil, 0)
	var out []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			return out
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		out = append(out, tok.String()+" "+lit)
	}
}

// first returns the first touched path matching any of globs, or "".
func (c *Checker) first(globs ...string) string {
	for _, p := range c.paths() {
		for _, g := range globs {
			if diff.Match(g, p) {
				return p
			}
		}
	}
	return ""
}

// DriversClaim is ORCH-01's automated part: the driver trees the diff reaches
// against the designs' drivers. A shared package needs every driver declared
// (the TEST-02 caveat), and "interface" covers them all. With no design in
// the diff there is no claim to compare, and FLOW-01 reports that instead.
func (c *Checker) DriversClaim() []report.Finding {
	claim := c.Designs.Drivers
	if len(c.Designs.List) == 0 || claim["interface"] {
		return nil
	}
	var out []report.Finding
	for _, d := range c.Cfg.Drivers.Dirs {
		if p := c.first("driver/" + d + "/"); p != "" && !claim[d] {
			out = append(out, c.finding("ORCH-01", "drivers-claim", p,
				"the diff reaches driver/%s/ but the designs' drivers %v lack it",
				d, keys(claim)))
		}
	}
	p := c.first(c.Cfg.Drivers.Shared...)
	if p == "" {
		return out
	}
	var missing []string
	for _, d := range c.Cfg.Drivers.All {
		if !claim[d] {
			missing = append(missing, d)
		}
	}
	if len(missing) > 0 {
		out = append(out, c.finding("ORCH-01", "drivers-claim", p,
			"%s is shared by every driver, so the designs' drivers %v must also "+
				"declare %v (or interface)", p, keys(claim), missing))
	}
	return out
}

// BootstrapSurface is BOOT-07's automated part, both directions. A surface
// change without the model document is a hint: most surface changes are not
// model changes, and judging that is BOOT-07's agent part. On history it
// fired on container-inputs, whose change is not a model change (task 11).
func (c *Checker) BootstrapSurface() []report.Finding {
	b := c.Cfg.Bootstrap
	var out []report.Finding
	if p := c.first(b.Surface...); p != "" && !c.Diff.Touched(b.Doc) {
		f := c.finding("BOOT-07", "bootstrap-surface", p,
			"%s is on the bootstrap surface and %s is not in the diff; tier 2 "+
				"judges whether this is a model change", p, b.Doc)
		f.Severity = report.Hint
		out = append(out, f)
	}
	if c.Diff.Touched(b.Doc) && !c.Designs.Rules["BOOT-07"] {
		out = append(out, c.finding("BOOT-07", "bootstrap-surface", b.Doc,
			"%s changed and no design in the diff cites BOOT-07 in its rules", b.Doc))
	}
	return out
}

// SchemaDoc is RELAY-05's deterministic half: a schema and its document
// change together. A document change with no schema change is a hint, since
// ledger-spec.md is prose for the binary format as well and a prose fix
// changes no schema (PR #62, task 11).
func (c *Checker) SchemaDoc() []report.Finding {
	var out []report.Finding
	for _, pair := range c.Cfg.SchemaDoc {
		code, doc := c.first(pair.Code), c.Diff.Touched(pair.Doc)
		switch {
		case code != "" && !doc:
			out = append(out, c.finding("RELAY-05", "schema-doc", code,
				"%s changed without %s", code, pair.Doc))
		case code == "" && doc:
			f := c.finding("RELAY-05", "schema-doc", pair.Doc,
				"%s changed with nothing under %s", pair.Doc, pair.Code)
			f.Severity = report.Hint
			out = append(out, f)
		}
	}
	return out
}

// SeamContract is GLOBAL-08's automated part for the seams rules.yaml gives a
// contract document.
func (c *Checker) SeamContract() []report.Finding {
	var out []report.Finding
	for _, s := range c.Cfg.Seams {
		if p := c.first(s.Code...); p != "" && !c.Diff.Touched(s.Contract) {
			out = append(out, c.finding("GLOBAL-08", "seam-contract", p,
				"%s is on the %s seam and its contract %s is not in the diff",
				p, s.Name, s.Contract))
		}
	}
	return out
}

// TestsWithChange is GLOBAL-04's mechanical half, a hint that never fails
// (a6): a package's non-test Go files changed and none of its tests did.
func (c *Checker) TestsWithChange() []report.Finding {
	code, tests := map[string]bool{}, map[string]bool{}
	for _, f := range c.Diff.Files {
		p := f.Path
		if !strings.HasSuffix(p, ".go") || strings.Contains("/"+p, "/testdata/") {
			continue
		}
		if strings.HasSuffix(p, "_test.go") {
			tests[path.Dir(p)] = true
		} else if f.Status != 'D' {
			code[path.Dir(p)] = true
		}
	}
	var out []report.Finding
	for _, dir := range keys(code) {
		if !tests[dir] {
			f := c.finding("GLOBAL-04", "tests-with-change", dir+"/",
				"non-test Go code changed in %s/ and none of its tests did", dir)
			f.Severity = report.Hint
			out = append(out, f)
		}
	}
	return out
}

func keys(set map[string]bool) []string {
	out := []string{}
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

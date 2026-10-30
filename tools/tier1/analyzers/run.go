package analyzers

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// Check is an analyzer with the rule it enforces and the directory it
// applies to.
type Check struct {
	Analyzer *analysis.Analyzer
	Rule     string
	ID       string // the check ID in rules.yaml
	Scope    string // a directory ending in "/"
}

// Checks are the source checks tier 1 runs.
var Checks = []Check{
	{LedgerSingleWriter, "RELAY-01", "ledger-single-writer", "relay/"},
	{RelayDiskless, "RELAY-07", "relay-diskless", "relay/"},
	{ExecutorNoCgo, "EXEC-01", "executor-no-cgo", "cmd/warden-io/"},
	{LargeLiteral, "EXEC-05", "large-literal", "cmd/warden-io/"},
}

// Run applies each check to the root-module packages under its scope that
// hold a changed Go file, built for the host GOOS. Findings are not yet
// filtered to changed lines.
func Run(d *diff.Diff, sev func(string) report.Severity) ([]report.Finding, error) {
	dirs := changedDirs(d)
	if len(dirs) == 0 {
		return nil, nil
	}
	root, err := filepath.Abs(d.Dir)
	if err != nil {
		return nil, err
	}
	patterns := make([]string, len(dirs))
	for i, dir := range dirs {
		patterns[i] = "./" + dir
	}
	cfg := &packages.Config{Mode: packages.LoadSyntax, Dir: root,
		Env: append(os.Environ(), "CGO_ENABLED=0")}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading %v: %w", patterns, err)
	}
	var out []report.Finding
	for _, c := range Checks {
		fs, err := runOne(c, scoped(pkgs, root, c.Scope), root, sev)
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
	}
	return out, nil
}

func runOne(c Check, pkgs []*packages.Package, root string,
	sev func(string) report.Severity) ([]report.Finding, error) {
	if len(pkgs) == 0 {
		return nil, nil
	}
	g, err := checker.Analyze([]*analysis.Analyzer{c.Analyzer}, pkgs, nil)
	if err != nil {
		return nil, err
	}
	var out []report.Finding
	for _, act := range g.Roots {
		if act.Err != nil {
			return nil, fmt.Errorf("%s on %s: %w", c.ID, act.Package.PkgPath, act.Err)
		}
		for _, diag := range act.Diagnostics {
			pos := act.Package.Fset.Position(diag.Pos)
			file, err := filepath.Rel(root, pos.Filename)
			if err != nil || strings.HasSuffix(file, "_test.go") {
				continue
			}
			out = append(out, report.Finding{Rule: c.Rule, Severity: sev(c.Rule),
				File: filepath.ToSlash(file), Line: pos.Line, Check: c.ID,
				Message: diag.Message})
		}
	}
	return out, nil
}

// changedDirs is every directory with a changed, non-deleted Go file that
// some check's scope covers.
func changedDirs(d *diff.Diff) []string {
	set := map[string]bool{}
	for _, f := range d.Files {
		if f.Status == 'D' || !strings.HasSuffix(f.Path, ".go") ||
			strings.Contains("/"+f.Path, "/testdata/") {
			continue
		}
		for _, c := range Checks {
			if strings.HasPrefix(f.Path, c.Scope) {
				set[path.Dir(f.Path)] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for dir := range set {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

// scoped returns the loaded packages whose directory is under scope. A
// package that failed to load is skipped: make test and the build report it.
func scoped(pkgs []*packages.Package, root, scope string) []*packages.Package {
	var out []*packages.Package
	for _, p := range pkgs {
		if len(p.GoFiles)+len(p.IgnoredFiles) == 0 || len(p.Errors) > 0 {
			continue
		}
		files := append(append([]string(nil), p.GoFiles...), p.IgnoredFiles...)
		rel, err := filepath.Rel(root, filepath.Dir(files[0]))
		if err == nil && strings.HasPrefix(filepath.ToSlash(rel)+"/", scope) {
			out = append(out, p)
		}
	}
	return out
}

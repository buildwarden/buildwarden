// Package surface extracts warden's user-facing surface, cobra flag names and
// toml config keys, from Go source at two revisions, and checks it against
// the reference docs (UX-01, ORCH-03) and for silent removals (UX-03).
package surface

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/buildwarden/buildwarden/tools/tier1/diff"
	"github.com/buildwarden/buildwarden/tools/tier1/report"
)

// Name is one surface element. Kind is "flag" or "key".
type Name struct {
	Kind, Name, File string
	Line             int
}

func (n Name) id() string { return n.Kind + " " + n.Name }

// Surface is the set of names at one revision, plus every string literal in
// the same files: a removed name that still appears as a literal is taken to
// have an alias or a deprecation entry.
type Surface struct {
	Names    map[string]Name
	Literals map[string]bool
}

// Extract parses files (path to source) without type-checking.
func Extract(files map[string]string) (*Surface, error) {
	s := &Surface{Names: map[string]Name{}, Literals: map[string]bool{}}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, files[p], parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("surface: %w", err)
		}
		s.file(fset, p, f)
	}
	return s, nil
}

func (s *Surface) add(kind, name, file string, line int) {
	n := Name{Kind: kind, Name: name, File: file, Line: line}
	if _, dup := s.Names[n.id()]; !dup {
		s.Names[n.id()] = n
	}
}

func (s *Surface) file(fset *token.FileSet, path string, f *ast.File) {
	flagSets := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
			for i, rhs := range as.Rhs {
				if id, ok := as.Lhs[i].(*ast.Ident); ok && isFlagSet(rhs, nil) {
					flagSets[id.Name] = true
				}
			}
		}
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BasicLit:
			v, err := strconv.Unquote(x.Value)
			if err == nil && x.Kind == token.STRING {
				s.Literals[v] = true
			}
		case *ast.CallExpr:
			if name, ok := flagName(x, flagSets); ok {
				s.add("flag", name, path, fset.Position(x.Pos()).Line)
			}
		case *ast.Field:
			if key := tomlKey(x); key != "" {
				s.add("key", key, path, fset.Position(x.Pos()).Line)
			}
		}
		return true
	})
}

// isFlagSet reports whether e is X.Flags(), X.PersistentFlags(), or an
// identifier assigned from one.
func isFlagSet(e ast.Expr, sets map[string]bool) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return sets[x.Name]
	case *ast.CallExpr:
		sel, ok := x.Fun.(*ast.SelectorExpr)
		return ok && (sel.Sel.Name == "Flags" || sel.Sel.Name == "PersistentFlags")
	}
	return false
}

// notDefiners are pflag.FlagSet methods that take a flag name but do not
// define a flag.
var notDefiners = []string{"Mark", "Set", "Lookup", "Parse", "Add", "Visit", "Changed",
	"Get", "Has", "Arg", "Init", "Print", "Sort", "Shorthand"}

func flagName(call *ast.CallExpr, sets map[string]bool) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isFlagSet(sel.X, sets) {
		return "", false
	}
	for _, p := range notDefiners {
		if strings.HasPrefix(sel.Sel.Name, p) {
			return "", false
		}
	}
	for _, a := range call.Args {
		if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			v, err := strconv.Unquote(lit.Value)
			return v, err == nil
		}
	}
	return "", false
}

func tomlKey(f *ast.Field) string {
	if f.Tag == nil {
		return ""
	}
	tag, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		return ""
	}
	key, _, _ := strings.Cut(reflect.StructTag(tag).Get("toml"), ",")
	if key == "-" {
		return ""
	}
	return key
}

// Check compares the surface in dir's non-test Go files at the diff's base
// and HEAD. sev gives each rule's severity.
func Check(d *diff.Diff, dir string, docs []string,
	sev func(string) report.Severity) ([]report.Finding, error) {
	if !touched(d, dir) {
		return nil, nil
	}
	base, err := load(d, d.Base, dir)
	if err != nil {
		return nil, err
	}
	head, err := load(d, d.Head, dir)
	if err != nil {
		return nil, err
	}
	var added, removed []Name
	for id, n := range head.Names {
		if _, ok := base.Names[id]; !ok {
			added = append(added, n)
		}
	}
	for id, n := range base.Names {
		if _, ok := head.Names[id]; !ok {
			removed = append(removed, n)
		}
	}
	sortNames(added)
	sortNames(removed)
	out := docsFinding(d, docs, added, removed, sev)
	for _, n := range removed {
		if !head.Literals[n.Name] {
			msg := fmt.Sprintf("%s %q was removed with no alias or deprecation; "+
				"keep it, marked deprecated, for a documented window",
				n.Kind, n.Name)
			out = append(out, report.Finding{Rule: "UX-03", Severity: sev("UX-03"),
				File: n.File, Check: "surface-removed", Message: msg})
		}
	}
	return out, nil
}

func docsFinding(d *diff.Diff, docs []string, added, removed []Name,
	sev func(string) report.Severity) []report.Finding {
	if len(added)+len(removed) == 0 {
		return nil
	}
	for _, doc := range docs {
		if d.Touched(doc) {
			return nil
		}
	}
	var names []string
	for _, n := range added {
		names = append(names, "+"+n.id())
	}
	for _, n := range removed {
		names = append(names, "-"+n.id())
	}
	first := append(added, removed...)[0]
	msg := fmt.Sprintf("the surface changed (%s) and neither %s changed",
		strings.Join(names, ", "), strings.Join(docs, " nor "))
	return []report.Finding{{Rule: "UX-01", Severity: sev("UX-01"), File: first.File,
		Check: "surface-docs", Message: msg}}
}

func sortNames(ns []Name) {
	sort.Slice(ns, func(i, j int) bool { return ns[i].id() < ns[j].id() })
}

func touched(d *diff.Diff, dir string) bool {
	for _, f := range d.Files {
		if strings.HasPrefix(f.Path, dir) || strings.HasPrefix(f.Old, dir) {
			return true
		}
	}
	return false
}

// load reads dir's non-test Go files (not its subdirectories) at rev.
func load(d *diff.Diff, rev, dir string) (*Surface, error) {
	files := map[string]string{}
	for _, p := range d.List(rev, dir) {
		rest := strings.TrimPrefix(p, dir)
		if strings.Contains(rest, "/") || !strings.HasSuffix(p, ".go") ||
			strings.HasSuffix(p, "_test.go") {
			continue
		}
		if src, ok := d.Show(rev, p); ok {
			files[p] = src
		}
	}
	return Extract(files)
}

// Package analyzers holds tier 1's source checks as go/analysis passes, and
// runs them over the packages that hold the diff's changed Go files.
package analyzers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// LedgerSingleWriter flags ledger writes outside the ledger's loop (RELAY-01):
// a Write on a Ledger's writer, a call to (*Ledger).sign, or an assignment to
// its prevSigRaw. (*Ledger).writeHeader is allowed because NewLedger calls it
// before the loop starts, so nothing races it.
var LedgerSingleWriter = &analysis.Analyzer{
	Name: "ledgersinglewriter",
	Doc:  "ledger records are written only by (*Ledger).loop (RELAY-01)",
	Run:  runLedgerSingleWriter,
}

var ledgerWriters = map[string]bool{"loop": true, "writeHeader": true}

func runLedgerSingleWriter(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			allowed := ok && isLedgerMethod(pass, fn) && ledgerWriters[fn.Name.Name]
			if !ok || fn.Body == nil || allowed {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ledgerWrite(pass, fn.Name.Name, n)
				return true
			})
		}
	}
	return nil, nil
}

func ledgerWrite(pass *analysis.Pass, in string, n ast.Node) {
	switch x := n.(type) {
	case *ast.CallExpr:
		sel, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		if inner, ok := sel.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Write" &&
			inner.Sel.Name == "writer" && isLedger(pass, inner.X) {
			pass.Reportf(x.Pos(), "%s writes the ledger directly; send it to "+
				"(*Ledger).loop over the channel", in)
		}
		if sel.Sel.Name == "sign" && isLedger(pass, sel.X) {
			pass.Reportf(x.Pos(), "%s signs a ledger record outside (*Ledger).loop",
				in)
		}
	case *ast.AssignStmt:
		for _, lhs := range x.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "prevSigRaw" && isLedger(pass, sel.X) {
				pass.Reportf(lhs.Pos(), "%s advances the ledger chain outside "+
					"(*Ledger).loop", in)
			}
		}
	}
}

func isLedgerMethod(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	return fn.Recv != nil && len(fn.Recv.List) == 1 && isLedger(pass, fn.Recv.List[0].Type)
}

// isLedger reports whether e's type is this package's Ledger or *Ledger.
func isLedger(pass *analysis.Pass, e ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(e)
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Name() == "Ledger" && named.Obj().Pkg() == pass.Pkg
}

// RelayDiskless flags file-creating and file-writing calls (RELAY-07). The
// runner applies it to relay/ only, and never to test files.
var RelayDiskless = &analysis.Analyzer{
	Name: "relaydiskless",
	Doc:  "the relay keeps no build state on disk (RELAY-07)",
	Run:  runRelayDiskless,
}

var diskWriters = map[string]map[string]bool{
	"os": {"Create": true, "WriteFile": true, "OpenFile": true, "CreateTemp": true,
		"Mkdir": true, "MkdirAll": true, "MkdirTemp": true},
	"io/ioutil": {"WriteFile": true, "TempFile": true, "TempDir": true},
}

func runRelayDiskless(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if ok && fn.Pkg() != nil && diskWriters[fn.Pkg().Path()][fn.Name()] {
				pass.Reportf(call.Pos(), "%s.%s writes to disk in the relay; "+
					"stream it to the collector instead",
					fn.Pkg().Name(), fn.Name())
			}
			return true
		})
	}
	return nil, nil
}

// ExecutorNoCgo flags import "C" (EXEC-01). The runner loads packages with
// CGO_ENABLED=0, so a cgo file is among the package's ignored files and is
// parsed here for its imports.
var ExecutorNoCgo = &analysis.Analyzer{
	Name: "executornocgo",
	Doc:  "the executor is pure Go with no cgo (EXEC-01)",
	Run:  runExecutorNoCgo,
}

func runExecutorNoCgo(pass *analysis.Pass) (any, error) {
	files := append([]*ast.File(nil), pass.Files...)
	for _, name := range pass.IgnoredFiles {
		f, err := parser.ParseFile(pass.Fset, name, nil, parser.ImportsOnly)
		if err == nil {
			files = append(files, f)
		}
	}
	for _, f := range files {
		for _, imp := range f.Imports {
			if imp.Path.Value == `"C"` {
				pass.Reportf(imp.Pos(), `import "C" in the executor; it must `+
					`build with CGO_ENABLED=0 for every guest target`)
			}
		}
	}
	return nil, nil
}

// LargeLiteralBytes is the size above which a string literal belongs in a
// //go:embed file (EXEC-05).
const LargeLiteralBytes = 4096

// LargeLiteral flags string literals over LargeLiteralBytes (EXEC-05).
var LargeLiteral = &analysis.Analyzer{
	Name: "largeliteral",
	Doc:  "large blobs go in //go:embed files (EXEC-05)",
	Run:  runLargeLiteral,
}

func runLargeLiteral(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			size := len(lit.Value)
			if v, err := strconv.Unquote(lit.Value); err == nil {
				size = len(v)
			}
			if size > LargeLiteralBytes {
				pass.Reportf(lit.Pos(), "string literal of %d bytes; move it to "+
					"a //go:embed file", size)
			}
			return true
		})
	}
	return nil, nil
}

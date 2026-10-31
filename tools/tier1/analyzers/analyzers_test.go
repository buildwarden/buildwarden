package analyzers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestLedgerSingleWriter(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), LedgerSingleWriter, "relay")
}

func TestRelayDiskless(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), RelayDiskless, "diskless")
}

// TestExecutorNoCgo builds the pass by hand: with CGO_ENABLED=0 the cgo file
// is an ignored file, where analysistest reads no expectations.
func TestExecutorNoCgo(t *testing.T) {
	dir := filepath.Join(analysistest.TestData(), "src", "cgo")
	fset := token.NewFileSet()
	pure, err := parser.ParseFile(fset, filepath.Join(dir, "pure.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{pure},
		IgnoredFiles: []string{filepath.Join(dir, "native.go")},
		Report: func(d analysis.Diagnostic) {
			p := fset.Position(d.Pos)
			got = append(got, fmt.Sprintf("%s:%d", filepath.Base(p.Filename), p.Line))
		}}
	if _, err := runExecutorNoCgo(pass); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "native.go:3" {
		t.Errorf("diagnostics %v, want native.go:3", got)
	}
}

func TestLargeLiteral(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), LargeLiteral, "literal")
}

package driver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVMDriversClaimWithRequestLink pins that the qemu, firecracker and vz
// drivers each claim their relay with the request's build link: the one
// ClaimRelay call in each passes <req>.Link().Claim() for the request it
// also passes. Their relay-start steps boot a real VM, so this is checked on
// the source; Hyper-V's claim is tested end to end over its Provisioner
// (TestRunBuild_ClaimBeforeBuildVM). A nil link is loud on qemu and
// firecracker (the relay refuses "build_link is required") but quiet on vz,
// whose host relay would fall back to --subnet (Review r1 SHOULD 5).
func TestVMDriversClaimWithRequestLink(t *testing.T) {
	for _, pkg := range []string{"qemu", "firecracker", "vz"} {
		t.Run(pkg, func(t *testing.T) {
			calls := claimRelayCalls(t, pkg)
			if len(calls) != 1 {
				t.Fatalf("%d ClaimRelay calls, want 1", len(calls))
			}
			c := calls[0]
			if len(c.Args) != 7 {
				t.Fatalf("ClaimRelay has %d arguments, want 7", len(c.Args))
			}
			req := exprString(c.Args[3])
			if got, want := exprString(c.Args[5]), req+".Link().Claim()"; got != want {
				t.Errorf("ClaimRelay's link argument is %s, want %s", got, want)
			}
		})
	}
}

// claimRelayCalls returns every call to a ClaimRelay method in the package's
// non-test files, whatever their build tags.
func claimRelayCalls(t *testing.T, dir string) []*ast.CallExpr {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var calls []*ast.CallExpr
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "ClaimRelay" {
				calls = append(calls, c)
			}
			return true
		})
	}
	return calls
}

// exprString renders a selector or call chain such as r.req.Link().Claim().
func exprString(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		if len(e.Args) == 0 {
			return exprString(e.Fun) + "()"
		}
	}
	return "<expr>"
}

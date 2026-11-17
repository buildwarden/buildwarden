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
// also passes, or, for a driver on warden's build sequence, reports the
// relay to host.ClaimRelay with claim_values.build_link <req>.Link().Claim().
// Their relay-start steps boot a real VM, so this is checked on the source;
// Hyper-V's claim is tested end to end over its Provisioner
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
			checkClaimLink(t, calls[0])
		})
	}
}

// checkClaimLink checks one ClaimRelay call: a Session's, with the request
// and its link, or a driver.Host's, with a RelayStarted literal.
func checkClaimLink(t *testing.T, c *ast.CallExpr) {
	t.Helper()
	switch len(c.Args) {
	case 2:
		link := buildLinkValue(t, c.Args[1])
		if !strings.HasSuffix(link, "req.Link().Claim()") {
			t.Errorf("relay-started's build_link is %s, want <req>.Link().Claim()",
				link)
		}
	case 7:
		req := exprString(c.Args[3])
		if got, want := exprString(c.Args[5]), req+".Link().Claim()"; got != want {
			t.Errorf("ClaimRelay's link argument is %s, want %s", got, want)
		}
	default:
		t.Fatalf("ClaimRelay has %d arguments, want 2 or 7", len(c.Args))
	}
}

// buildLinkValue is the BuildLink field of the ClaimValues in the
// driver.RelayStarted literal e, rendered.
func buildLinkValue(t *testing.T, e ast.Expr) string {
	t.Helper()
	field := func(e ast.Expr, name string) ast.Expr {
		lit, ok := e.(*ast.CompositeLit)
		if !ok {
			t.Fatalf("%s is not a literal", exprString(e))
		}
		for _, el := range lit.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == name {
					return kv.Value
				}
			}
		}
		t.Fatalf("the literal sets no %s", name)
		return nil
	}
	return exprString(field(field(e, "ClaimValues"), "BuildLink"))
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

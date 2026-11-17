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
// drivers each report their relay to warden with the request's build link:
// the one host.ClaimRelay call in each passes a driver.RelayStarted literal
// whose claim is a tcp endpoint with an address, where warden claims the
// relay, and whose claim_values.build_link is <req>.Link().Claim(). Their
// relay-start steps boot a real VM, so this is checked on the source;
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
			checkRelayStarted(t, calls[0])
		})
	}
}

// checkRelayStarted checks the host.ClaimRelay call c: its relay-started
// value names a tcp claim endpoint with an address, and the request's link.
func checkRelayStarted(t *testing.T, c *ast.CallExpr) {
	t.Helper()
	if len(c.Args) != 2 {
		t.Fatalf("ClaimRelay has %d arguments, want 2 (ctx and the relay-started "+
			"value)", len(c.Args))
	}
	claim := litField(t, c.Args[1], "Claim")
	if got := exprString(litField(t, claim, "Network")); got != "driver.ClaimTCP" {
		t.Errorf("relay-started's claim network is %s, want driver.ClaimTCP", got)
	}
	litField(t, claim, "Address")
	link := exprString(litField(t, litField(t, c.Args[1], "ClaimValues"), "BuildLink"))
	if !strings.HasSuffix(link, "req.Link().Claim()") {
		t.Errorf("relay-started's build_link is %s, want <req>.Link().Claim()", link)
	}
}

// litField is the value of the field name in the composite literal e.
func litField(t *testing.T, e ast.Expr, name string) ast.Expr {
	t.Helper()
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

package vz

import (
	"bufio"
	"fmt"
	"go/build"
	"go/build/constraint"
	"os"
	"slices"
	"testing"
)

// Every target, with cgo on and off, must compile exactly one of the
// bindings and their stub, and must not take in the Objective-C half of the
// bindings without the cgo file that compiles it. go/build applies the
// constraints, file-name suffixes and the cgo rule the go command applies,
// and compiles nothing, so this covers the targets no host here can build.
func TestPlatformFileSelection(t *testing.T) {
	var targets []build.Context
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			for _, cgo := range []bool{true, false} {
				ctx := build.Default
				ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = goos, goarch, cgo
				targets = append(targets, ctx)
			}
		}
	}
	const stub, bindings = "vz_stub.go", "vz_darwin_arm64.go"
	for _, ctx := range targets {
		name := fmt.Sprintf("%s/%s cgo=%v", ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled)
		t.Run(name, func(t *testing.T) {
			pkg, err := ctx.ImportDir(".", 0)
			if err != nil {
				t.Fatalf("ImportDir: %v", err)
			}
			compiled := slices.Concat(pkg.GoFiles, pkg.CgoFiles)
			var got []string
			for _, f := range []string{stub, bindings} {
				if slices.Contains(compiled, f) {
					got = append(got, f)
				}
			}
			if len(got) != 1 {
				t.Errorf("compiles %v of %s and %s, want exactly one",
					got, stub, bindings)
			}
			if ctx.CgoEnabled && len(pkg.MFiles) > 0 && len(pkg.CgoFiles) == 0 {
				t.Errorf("selects Objective-C files %v with no cgo file, "+
					"which the go command refuses", pkg.MFiles)
			}
		})
	}
}

// The two //go:build lines must be exact complements over darwin, arm64 and
// cgo. TestPlatformFileSelection cannot see a dropped cgo term in the
// bindings' line, because their import "C" already leaves them out without
// cgo; this test reads the lines themselves.
func TestPlatformConstraintsComplement(t *testing.T) {
	stub := buildLine(t, "vz_stub.go")
	bindings := buildLine(t, "vz_darwin_arm64.go")
	for mask := range 8 {
		set := map[string]bool{
			"darwin": mask&1 != 0, "arm64": mask&2 != 0, "cgo": mask&4 != 0,
		}
		has := func(tag string) bool { return set[tag] }
		if stub.Eval(has) == bindings.Eval(has) {
			t.Errorf("with %v, stub %q and bindings %q agree; want exactly one",
				set, stub, bindings)
		}
	}
}

// buildLine returns the //go:build constraint of the named file.
func buildLine(t *testing.T, name string) constraint.Expr {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if constraint.IsGoBuild(sc.Text()) {
			expr, err := constraint.Parse(sc.Text())
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return expr
		}
	}
	t.Fatalf("%s has no //go:build line", name)
	return nil
}

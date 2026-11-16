package vz

import (
	"fmt"
	"go/build"
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

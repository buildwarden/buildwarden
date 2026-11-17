package driver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGoMod makes dir/go.mod with the given module line.
func writeGoMod(t *testing.T, dir, module string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "module " + module + "\n\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// F9: the working directory is never the tree. This test's executable is
// built outside any tree, and its working directory is inside one that
// declares the module; ModuleRoot fails with its own FixableError, which
// names the executable and the version and never calls a release a
// development build, and GuestBinary's wrapping keeps it a FixableError.
func TestModuleRootIgnoresTheWorkingDirectory(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := moduleRootAbove(filepath.Dir(exe)); ok {
		t.Skipf("the test binary %s was built inside a tree", exe)
	}
	repo := t.TempDir()
	writeGoMod(t, repo, "github.com/buildwarden/buildwarden")
	t.Chdir(repo)
	orig := WardenVersion
	t.Cleanup(func() { WardenVersion = orig })
	for _, version := range []string{"dev", "0.2.0"} {
		WardenVersion = version
		root, err := ModuleRoot()
		var fe *FixableError
		if !errors.As(err, &fe) {
			t.Fatalf("version %s: got %q, %v; want a FixableError", version, root, err)
		}
		msg := exe + " (version " + version + ") is not inside a buildwarden source tree"
		fix := "run 'make build' in warden's source tree and use its dist/warden"
		if fe.Msg != msg || fe.Fix != fix {
			t.Errorf("got  %q\n     %q\nwant %q\n     %q", fe.Msg, fe.Fix, msg, fix)
		}
		if strings.Contains(strings.ToLower(fe.Msg+fe.Fix), "development") {
			t.Errorf("the error calls warden a development build: %s", fe.Msg)
		}
		_, err = GuestBinary("warden-io", "linux", "amd64")
		if !errors.As(err, &fe) || !strings.HasPrefix(err.Error(),
			"warden-io-linux-amd64 not found next to warden, and no source tree "+
				"to build it from: "+msg) {
			t.Errorf("GuestBinary: %v", err)
		}
	}
}

// The module line is matched whole: a nested module of the tree, a longer
// module path and the line in a comment are not the tree.
func TestModuleRootAbove(t *testing.T) {
	tree := t.TempDir()
	writeGoMod(t, tree, "github.com/buildwarden/buildwarden")
	nested := filepath.Join(tree, "tools", "tier1")
	writeGoMod(t, nested, "github.com/buildwarden/buildwarden/tools/tier1")
	if got, ok := moduleRootAbove(filepath.Join(nested, "cmd")); !ok || got != tree {
		t.Errorf("from the nested module: %q, %v; want %s", got, ok, tree)
	}
	for _, module := range []string{
		"github.com/buildwarden/buildwarden/tools/tier1",
		"github.com/buildwarden/buildwarden-fork",
		"example.com/x // module github.com/buildwarden/buildwarden",
	} {
		dir := t.TempDir()
		writeGoMod(t, dir, module)
		if got, ok := moduleRootAbove(dir); ok {
			t.Errorf("module %s matched at %s", module, got)
		}
	}
}

// The setter is the one other way the tree is named.
func TestSetModuleRootForTests(t *testing.T) {
	SetModuleRootForTests("/src/buildwarden")
	t.Cleanup(func() { SetModuleRootForTests("") })
	if got, err := ModuleRoot(); err != nil || got != "/src/buildwarden" {
		t.Errorf("got %q, %v", got, err)
	}
}

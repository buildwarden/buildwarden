package hyperv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// TestMain names the source tree for the whole package: a test binary is
// built outside any tree, and the tests that stage a guest seed cross-build
// warden-io from it (driver.GuestBinary). The tree is the module this
// package's directory is in, the one go test runs the binary from; no test
// names it by itself.
func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	for err == nil {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			err = fmt.Errorf("no go.mod above the test's directory")
		}
		dir = parent
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "finding the source tree: %v\n", err)
		os.Exit(1)
	}
	driver.SetModuleRootForTests(dir)
	os.Exit(m.Run())
}

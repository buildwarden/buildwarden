package driver

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewWorkDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not", "yet", "there")
	req := &BuildRequest{TempDir: root}

	a, err := NewWorkDir(req, "container-abc")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewWorkDir(req, "container-abc")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("two work dirs share %s", a)
	}
	for _, d := range []string{a, b} {
		if filepath.Dir(d) != root {
			t.Errorf("%s not created under --tmpdir %s", d, root)
		}
		if !strings.HasPrefix(filepath.Base(d), "warden-container-abc-") {
			t.Errorf("%s not tagged warden-container-abc-", d)
		}
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			t.Fatalf("%s: %v", d, err)
		}
		if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm&0o077 != 0 {
			t.Errorf("%s mode %o is readable by others", d, perm)
		}
	}
}

func TestNewWorkDirDefaultsToOSTemp(t *testing.T) {
	d, err := NewWorkDir(&BuildRequest{}, "vz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)
	if filepath.Dir(d) != filepath.Clean(os.TempDir()) {
		t.Errorf("%s not under os.TempDir() %s", d, os.TempDir())
	}
}

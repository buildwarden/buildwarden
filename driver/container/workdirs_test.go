package container

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// removeWorkDirs removes the work directories of the builds dead reports,
// and only those (DRV-24, DRV-27).
func TestRemoveWorkDirs(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"warden-container-dead0001-111",
		"warden-container-dead0001-222", "warden-container-live0001-333",
		"something-else"} {
		if err := os.Mkdir(filepath.Join(tmp, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	n, err := removeWorkDirs(io.Discard, tmp, func(id string) bool { return id == "dead0001" })
	if err != nil || n != 2 {
		t.Fatalf("removeWorkDirs = %d, %v; want 2, nil", n, err)
	}
	left, _ := os.ReadDir(tmp)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != "something-else" ||
		names[1] != "warden-container-live0001-333" {
		t.Errorf("left %q", names)
	}
}

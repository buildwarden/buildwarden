//go:build windows

package driver

import (
	"os"
	"testing"
)

// On Windows a lock file that a build has open cannot be removed, so a
// warden clean cannot unlink it between the build's open and its lock:
// Remove of the clean's lock leaves the file to the build that opened it.
func TestBuildLockRemoveLeavesAnOpenFile(t *testing.T) {
	inLockDir(t)
	l, err := TakeBuildLock("fake", "dddddddd")
	if err != nil {
		t.Fatal(err)
	}
	other, err := os.Open(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := l.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path()); err != nil {
		t.Fatalf("a lock file another process has open was removed: %v", err)
	}
}

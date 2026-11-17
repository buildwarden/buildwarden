//go:build !windows

package driver

import (
	"os"
	"testing"
)

// Remove leaves a lock file the path no longer names: one a new build made
// after this one's was removed by another process.
func TestBuildLockRemoveKeepsAnotherFile(t *testing.T) {
	inLockDir(t)
	l, err := TakeBuildLock("fake", "cccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(l.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.Path(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path()); err != nil {
		t.Fatalf("another build's lock file was removed: %v", err)
	}
}

// A lock file unlinked between the open and the lock (a warden clean that
// won the race) is detected, and the lock is taken again on a new file that
// the path names, so the build is never held under no name.
func TestBuildLockRetakenWhenUnlinked(t *testing.T) {
	inLockDir(t)
	opens := 0
	afterBuildLockOpen = func(path string) {
		opens++
		if opens == 1 {
			if err := os.Remove(path); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(func() { afterBuildLockOpen = func(string) {} })
	l, err := TakeBuildLock("fake", "aaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if opens != 2 {
		t.Errorf("opened %d times, want 2: the unlinked file was kept", opens)
	}
	if !namesFile(l.Path(), l.f) {
		t.Error("the lock held is not the file its path names")
	}
}

package driver

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostLockExcludes(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // Linux
	t.Setenv("HOME", t.TempDir())           // macOS UserCacheDir
	t.Setenv("LocalAppData", t.TempDir())   // Windows

	unlock, err := HostLock("test")
	if err != nil {
		t.Fatal(err)
	}
	var second atomic.Bool
	done := make(chan struct{})
	go func() {
		// A separate open file description, so flock contends even
		// in-process, as it would across warden processes.
		u2, err := HostLock("test")
		if err != nil {
			t.Error(err)
			close(done)
			return
		}
		second.Store(true)
		u2()
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	if second.Load() {
		t.Fatal("second HostLock acquired while the first was held")
	}
	unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("second HostLock never acquired after release")
	}
	if !second.Load() {
		t.Fatal("second HostLock did not run")
	}
}

func TestTryHostLock(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())

	unlock, ok, err := TryHostLock("try")
	if err != nil || !ok {
		t.Fatalf("first TryHostLock: ok=%v err=%v", ok, err)
	}
	if _, ok2, err2 := TryHostLock("try"); ok2 || err2 != nil {
		t.Fatalf("second TryHostLock while held: ok=%v err=%v, want false, nil", ok2, err2)
	}
	unlock()
	u, ok3, err3 := TryHostLock("try")
	if err3 != nil || !ok3 {
		t.Fatalf("TryHostLock after release: ok=%v err=%v", ok3, err3)
	}
	u()
}

// TestLockDir checks that locks live in LockDir, where drivers keep state
// tied to a lock (the Firecracker slots' pid files).
func TestLockDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	unlock, err := HostLock("lockdir-probe")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := os.Stat(filepath.Join(LockDir(), "lockdir-probe.lock")); err != nil {
		t.Fatalf("lock file not in LockDir: %v", err)
	}
}

// inLockDir gives the test a lock directory of its own.
func inLockDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
}

// A build lock is named for its driver and build, and a held one is seen as
// held, so a build is live exactly while its warden holds it.
func TestBuildLockHeld(t *testing.T) {
	inLockDir(t)
	l, err := TakeBuildLock("fake", "k3v9x2ab")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(LockDir(), "build-fake-k3v9x2ab.lock")
	if l.Path() != want {
		t.Fatalf("lock at %s, want %s", l.Path(), want)
	}
	if other, held, err := TryBuildLock(want); other != nil || !held || err != nil {
		t.Fatalf("TryBuildLock while held: %v %v %v, want nil, true, nil",
			other, held, err)
	}
	l.Release()
	other, held, err := TryBuildLock(want)
	if other == nil || held || err != nil {
		t.Fatalf("TryBuildLock after release: %v %v %v", other, held, err)
	}
	other.Release()
}

// A build lock is removed while it is held; a lock file that is gone is
// reported as such, not as held.
func TestBuildLockRemove(t *testing.T) {
	inLockDir(t)
	l, err := TakeBuildLock("fake", "bbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path()); !os.IsNotExist(err) {
		t.Fatalf("lock file still there: %v", err)
	}
	if other, held, err := TryBuildLock(l.Path()); other != nil || held || err != nil {
		t.Fatalf("TryBuildLock of a removed lock: %v %v %v, want nil, false, nil",
			other, held, err)
	}
}

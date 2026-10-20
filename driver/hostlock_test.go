package driver

import (
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

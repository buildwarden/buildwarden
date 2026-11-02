//go:build linux

package firecracker

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealTryLock checks the lock setup and doctor take on a user's slot lock
// files: a held lock is reported, a free one taken, and a missing file is
// free without being created.
func TestRealTryLock(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.lock")
	unlock, held, err := realHost{}.tryLock(missing)
	if err != nil || held {
		t.Fatalf("missing lock: held %v, err %v", held, err)
	}
	unlock()
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("tryLock created %s", missing)
	}

	path := filepath.Join(dir, "slot.lock")
	if err := os.WriteFile(path, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	unlock, held, err = realHost{}.tryLock(path)
	if err != nil || held {
		t.Fatalf("free lock: held %v, err %v", held, err)
	}
	// flock locks belong to the open file, so a second open conflicts even in
	// this process.
	if _, held, err := (realHost{}).tryLock(path); err != nil || !held {
		t.Fatalf("held lock: held %v, err %v", held, err)
	}
	unlock()
	unlock, held, err = realHost{}.tryLock(path)
	if err != nil || held {
		t.Fatalf("released lock: held %v, err %v", held, err)
	}
	unlock()
}

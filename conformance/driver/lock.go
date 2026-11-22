package driverconformance

import (
	"os"
	"path/filepath"
	"strings"
)

// The suite follows the build-lock convention (DRV-55 to DRV-58): it holds a
// lock for each build it runs, in this user's lock directory, so that an
// orchestrator's clean leaves its builds alone, and it passes the IDs of the
// held locks as keep_ids.

// lockDir is <user cache directory>/buildwarden/locks (DRV-55).
func lockDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "buildwarden", "locks")
}

// heldLock is the lock of one build the suite runs.
type heldLock struct {
	base string // the files' path without their extension
	f    *os.File
}

// takeLock takes the lock of build id of the driver name.
func takeLock(name, id string) (*heldLock, error) {
	dir := lockDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base := filepath.Join(dir, "build-"+name+"-"+id)
	f, err := os.OpenFile(base+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return &heldLock{base: base, f: f}, nil
}

// path is the build's file with extension ext: ".state" is its state file.
func (h *heldLock) path(ext string) string { return h.base + ext }

// release removes the build's state file and lock, and releases the lock.
func (h *heldLock) release() {
	if h.f == nil {
		return
	}
	_ = os.Remove(h.path(".state"))
	unlockAndRemove(h.base+".lock", h.f)
	h.f = nil
}

// heldLocks are the IDs of the driver name's builds whose locks are held.
func heldLocks(name string) []string {
	prefix := "build-" + name + "-"
	entries, _ := os.ReadDir(lockDir())
	var ids []string
	for _, e := range entries {
		id, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok || !strings.HasSuffix(id, ".lock") {
			continue
		}
		id = strings.TrimSuffix(id, ".lock")
		if isHeld(filepath.Join(lockDir(), e.Name())) {
			ids = append(ids, id)
		}
	}
	return ids
}

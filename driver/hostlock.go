package driver

import (
	"fmt"
	"os"
	"path/filepath"
)

// HostLock takes a host-wide exclusive lock named name, blocking until it is
// free, and returns the function that releases it. It serializes steps that
// concurrent warden processes would otherwise race on, where the container
// runtime's own check is not atomic. The lock is an OS file lock, so it is
// released if the process dies while holding it.
func HostLock(name string) (func(), error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "buildwarden", "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock %s: %w", name, err)
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("taking lock %s: %w", name, err)
	}
	return func() {
		_ = unlockFile(f)
		f.Close()
	}, nil
}

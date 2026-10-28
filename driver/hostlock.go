package driver

import (
	"errors"
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
	return hostLock(name, true)
}

// TryHostLock is HostLock without waiting: ok is false (and err nil) when
// another process holds the lock.
func TryHostLock(name string) (unlock func(), ok bool, err error) {
	unlock, err = hostLock(name, false)
	if errors.Is(err, errLockHeld) {
		return nil, false, nil
	}
	return unlock, err == nil, err
}

var errLockHeld = errors.New("lock is held")

func hostLock(name string, wait bool) (func(), error) {
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
	if err := lockFile(f, wait); err != nil {
		f.Close()
		if !wait && isLockHeld(err) {
			return nil, errLockHeld
		}
		return nil, fmt.Errorf("taking lock %s: %w", name, err)
	}
	return func() {
		_ = unlockFile(f)
		f.Close()
	}, nil
}

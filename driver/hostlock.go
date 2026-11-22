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

// LockDir is the private directory host locks live in. Drivers keep state
// tied to a lock beside it (the Firecracker slots' pid files).
func LockDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "buildwarden", "locks")
}

func hostLock(name string, wait bool) (func(), error) {
	dir := LockDir()
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

// BuildLock is the lock warden holds for the life of one build:
// build-<driver>-<id>.lock in LockDir (specs/driver-contract/v1.md, The build
// lock). A held lock means the build is live. Unlike HostLock it is checked
// against its path once taken, so that a lock file another process unlinked
// between this one's open and its lock is never held under no name, and it
// can be removed while it is held.
type BuildLock struct {
	f    *os.File
	path string
}

// BuildLockName is the base name, without .lock, of the lock of the build id
// of the driver called driverName; the build's record and state file take the
// same base name.
func BuildLockName(driverName, id string) string {
	return "build-" + driverName + "-" + id
}

// afterBuildLockOpen runs between a build lock's open and its lock; tests set
// it to unlink the file there, as a concurrent warden clean could.
var afterBuildLockOpen = func(string) {}

// TakeBuildLock takes the lock of the build id of the driver driverName,
// creating its file, and waits while another process holds it.
func TakeBuildLock(driverName, id string) (*BuildLock, error) {
	dir := LockDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	l, _, err := lockBuildFile(filepath.Join(dir, BuildLockName(driverName, id)+".lock"),
		true, true)
	return l, err
}

// TryBuildLock locks the existing build lock file path without waiting. held
// is true when another process holds it, which means the build is live; l is
// nil, with no error, when the file is gone.
func TryBuildLock(path string) (l *BuildLock, held bool, err error) {
	return lockBuildFile(path, false, false)
}

// lockBuildFile opens and locks path, then checks that the path still names
// the file it locked, and starts again with a new open when it does not (when
// create is set; otherwise the file is gone and l is nil).
func lockBuildFile(path string, create, wait bool) (*BuildLock, bool, error) {
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	for {
		f, err := os.OpenFile(path, flags, 0o600)
		if errors.Is(err, os.ErrNotExist) && !create {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("opening lock %s: %w", path, err)
		}
		afterBuildLockOpen(path)
		if err := lockFile(f, wait); err != nil {
			f.Close()
			if !wait && isLockHeld(err) {
				return nil, true, nil
			}
			return nil, false, fmt.Errorf("taking lock %s: %w", path, err)
		}
		if namesFile(path, f) {
			return &BuildLock{f: f, path: path}, false, nil
		}
		_ = unlockFile(f)
		f.Close()
		if !create {
			return nil, false, nil
		}
	}
}

// namesFile reports whether path names the open file f.
func namesFile(path string, f *os.File) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	named, err := os.Stat(path)
	return err == nil && os.SameFile(held, named)
}

// Path is the lock file's path.
func (l *BuildLock) Path() string { return l.path }

// Release releases the lock and leaves its file.
func (l *BuildLock) Release() {
	_ = unlockFile(l.f)
	l.f.Close()
}

// Remove removes the lock file, after checking that the path still names the
// file this lock holds, and releases the lock. On Unix the file is unlinked
// while the lock is held. Windows cannot remove a file that is open, so the
// lock is released first, and a file another process has opened since is
// left in place, its build live.
func (l *BuildLock) Remove() error {
	if !namesFile(l.path, l.f) {
		l.Release()
		return nil
	}
	return removeLockFile(l.path, l.f)
}

//go:build windows

package driverconformance

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0,
		new(windows.Overlapped))
}

func unlock(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}

// unlockAndRemove releases the lock on path and removes it: Windows removes
// no open file.
func unlockAndRemove(path string, f *os.File) {
	unlock(f)
	_ = f.Close()
	_ = os.Remove(path)
}

// isHeld reports whether another open file holds the lock on path.
func isHeld(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if lockFile(f) != nil {
		return true
	}
	unlock(f)
	return false
}

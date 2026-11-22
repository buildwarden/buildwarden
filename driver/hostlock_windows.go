//go:build windows

package driver

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File, wait bool) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	ol := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, ol)
}

func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}

func isLockHeld(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

// removeLockFile releases f's lock and closes it, then removes path. A
// process that opened the file since holds it open, which makes the removal
// fail with a sharing violation: the file is then that build's, and stays.
func removeLockFile(path string, f *os.File) error {
	_ = unlockFile(f)
	f.Close()
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		return nil
	}
	return err
}

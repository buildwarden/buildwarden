//go:build !windows

package driver

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(f *os.File, wait bool) error {
	how := unix.LOCK_EX
	if !wait {
		how |= unix.LOCK_NB
	}
	return unix.Flock(int(f.Fd()), how)
}

func unlockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }

func isLockHeld(err error) bool { return errors.Is(err, unix.EWOULDBLOCK) }

// removeLockFile unlinks path while f, open on it, holds its lock, then
// releases it.
func removeLockFile(path string, f *os.File) error {
	err := os.Remove(path)
	_ = unlockFile(f)
	f.Close()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

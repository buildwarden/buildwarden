//go:build !windows

package driverconformance

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

// unlockAndRemove unlinks path while f holds its lock, then releases it.
func unlockAndRemove(path string, f *os.File) {
	_ = os.Remove(path)
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	_ = f.Close()
}

// isHeld reports whether another open file holds the lock on path.
func isHeld(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return true
	}
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return false
}

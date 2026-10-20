//go:build windows

package hyperv

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/buildwarden/buildwarden/driver"
)

// startBackgroundRefresh starts a detached `warden image prepare --driver
// hyperv` (a re-prepare of the pinned base) logging to <cache>/prepare.log,
// unless a prepare is already running. It reports the log path and whether a
// refresh was started. Failures are not fatal to the build that asked.
func startBackgroundRefresh() (logPath string, started bool) {
	unlock, free, err := driver.TryHostLock(prepareLockName)
	if err != nil || !free {
		return "", false
	}
	unlock() // the child takes the lock itself; a race only means it exits early
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	logPath = filepath.Join(hypervCacheDir(), "prepare.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return "", false
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", false
	}
	defer log.Close()
	cmd := exec.Command(exe, "image", "prepare", "--driver", "hyperv")
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return "", false
	}
	_ = cmd.Process.Release()
	return logPath, true
}

package driver

import (
	"os"
	"os/exec"
	"path/filepath"
)

// StartDetached starts this warden binary with args as a background process
// that outlives the caller (a periodic image refresh), with its output in
// logPath (truncated). It returns once the process has started.
func StartDetached(logPath string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = detachedAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

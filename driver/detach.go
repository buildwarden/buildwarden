package driver

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ConfigFile is the absolute path of the config file this warden read, empty
// when it read none. cmd/warden records it as the configuration loads, as it
// records WardenVersion.
var ConfigFile string

// StartDetached starts this warden binary with args as a background process
// that outlives the caller (a periodic image refresh), with its output in
// logPath (truncated). It returns once the process has started. The child
// reads the config file its parent read (detachedArgs).
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
	cmd := exec.Command(exe, detachedArgs(args)...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = detachedAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// detachedArgs puts --config <ConfigFile> in front of args when a file was
// read, so that the background process reads that file and not whatever the
// variable or the working directory would give it; with none read it adds
// nothing.
func detachedArgs(args []string) []string {
	if ConfigFile == "" {
		return args
	}
	return append([]string{"--config", ConfigFile}, args...)
}

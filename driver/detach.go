package driver

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigFile is the absolute path of the config file this warden read, empty
// when it read none. cmd/warden records it as the configuration loads, as it
// records WardenVersion.
var ConfigFile string

// executable is os.Executable; tests replace it.
var executable = os.Executable

// StartDetached starts this warden binary with args as a background process
// that outlives the caller (a periodic image refresh), with its output in
// logPath (truncated). It returns once the process has started. The file run
// is exactly the one os.Executable names, with no search and no extension
// lookup. The child reads the config file its parent read (detachedArgs). A
// start that fails is also written to the log.
func StartDetached(logPath string, args ...string) error {
	exe, err := executable()
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
	if err := startDetached(exe, log, detachedArgs(args)); err != nil {
		fmt.Fprintf(log, "[warden] could not start %s: %v\n", exe, err)
		return err
	}
	return nil
}

// startDetached runs exe with args, its stdout and stderr on log, and
// releases it. It uses os.StartProcess rather than os/exec because os/exec,
// on Windows, resolves every path through PATHEXT, absolute ones included:
// an extension-less exe would be run as <exe>.exe, or not at all.
func startDetached(exe string, log *os.File, args []string) error {
	null, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer null.Close()
	p, err := os.StartProcess(exe, append([]string{exe}, args...), &os.ProcAttr{
		Files: []*os.File{null, log, log},
		Env:   os.Environ(),
		Sys:   detachedAttr(),
	})
	if err != nil {
		return err
	}
	return p.Release()
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

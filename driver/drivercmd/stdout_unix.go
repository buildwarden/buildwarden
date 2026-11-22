//go:build unix

package drivercmd

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// redirectStdout moves the message stream off descriptor 1, which then
// names standard error, and returns it.
func redirectStdout() (*os.File, error) {
	fd, err := unix.Dup(1)
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	if err := unix.Dup2(2, 1); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "messages"), nil
}

// ignoreSIGPIPE makes a write to a closed pipe an error and not the end of
// the process (DRV-51).
func ignoreSIGPIPE() { signal.Ignore(syscall.SIGPIPE) }

//go:build windows

package drivercmd

import (
	"os"

	"golang.org/x/sys/windows"
)

// redirectStdout makes standard error the process's standard output, for
// its children too, and returns the original output as the message stream.
// The stream is a duplicate of the output handle, as on Unix: the original
// is still held by the old os.Stdout, whose finalizer closes it once that
// file is collected, and a stream on the same handle would then fail with
// "The handle is invalid" partway through a build.
func redirectStdout() (*os.File, error) {
	orig, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return nil, err
	}
	self := windows.CurrentProcess()
	var out windows.Handle
	if err := windows.DuplicateHandle(self, orig, self, &out, 0, false,
		windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	errh, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		return nil, err
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, errh); err != nil {
		return nil, err
	}
	msgs := os.NewFile(uintptr(out), "messages")
	os.Stdout = os.Stderr
	return msgs, nil
}

// ignoreSIGPIPE does nothing: Windows has no SIGPIPE, and a write to a
// closed pipe is already an error.
func ignoreSIGPIPE() {}

//go:build windows

package driver

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// detachedAttr starts a process with no console and in its own process group,
// so it survives the caller's console closing or Ctrl-C.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

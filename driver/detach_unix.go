//go:build !windows

package driver

import "syscall"

// detachedAttr starts a process in its own session, so it survives the
// caller's terminal closing or Ctrl-C.
func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

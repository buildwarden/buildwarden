//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// ownGroup starts cmd in a process group of its own, so that the terminal's
// signals reach warden only (DRV-52).
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killGroup kills the process group cmd leads.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
	}
}

//go:build unix

package qemu

import "syscall"

// procGroupAttr puts the spawned QEMU process in its own process group so the
// driver can signal and terminate it (and any children) as a unit on teardown.
func procGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

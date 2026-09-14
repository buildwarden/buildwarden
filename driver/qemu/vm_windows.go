//go:build windows

package qemu

import "syscall"

// procGroupAttr is a no-op on Windows. The qemu driver is not a supported
// Windows host driver (use the hyperv driver there), and Windows has no
// Setpgid. Returning nil leaves default process attributes so the package and
// the warden CLI that imports it still compile on Windows.
func procGroupAttr() *syscall.SysProcAttr {
	return nil
}

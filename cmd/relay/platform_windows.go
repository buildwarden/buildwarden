//go:build windows

package main

// disableCoreDumps is a no-op on Windows. There is no RLIMIT_CORE; crash-dump
// behaviour is governed by Windows Error Reporting policy, not a per-process
// resource limit.
func disableCoreDumps() {}

// isUnixSocket is always false on Windows. Host mode relies on fd-inheritance
// socketpair ingress with a userspace (gvisor) netstack, which is a Unix-only
// transport. Windows relays use the TCP/IP baseline (vm/container modes).
func isUnixSocket(fd int) bool { return false }

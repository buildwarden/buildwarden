//go:build unix

package main

import "golang.org/x/sys/unix"

// disableCoreDumps prevents key material from appearing in core dumps by
// setting RLIMIT_CORE to zero.
func disableCoreDumps() {
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0})
}

// isUnixSocket reports whether the given inherited fd is an AF_UNIX socket,
// which is how host (fd ingress) mode is auto-detected.
func isUnixSocket(fd int) bool {
	_, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil {
		return false
	}
	sa, err := unix.Getsockname(fd)
	if err != nil {
		// If Getsockname fails but SO_TYPE succeeded, it's still a socket.
		// On macOS socketpairs, Getsockname may return an empty address.
		return true
	}
	_, ok := sa.(*unix.SockaddrUnix)
	return ok
}

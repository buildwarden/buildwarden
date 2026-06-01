//go:build darwin && arm64

package main

import "runtime"

func init() {
	// Pin the main goroutine to the OS main thread. The Virtualization.framework
	// requires the main run loop to be serviced for IPSW restore and other
	// long-running operations.
	runtime.LockOSThread()
}

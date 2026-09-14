//go:build windows

package main

import (
	"fmt"
	"os"
)

// runHostMode is unsupported on Windows. Host mode drives a userspace (gvisor)
// netstack over an inherited socketpair fd carrying raw Ethernet frames, which
// is a Unix-only transport. On Windows, run the relay in vm or container mode,
// which use the portable TCP/IP baseline.
//
// The signature mirrors the unix implementation in mode_host.go so the mode
// switch in main.go links on every platform.
func runHostMode(fd int, subnetCIDR, outDir, ctxDir, sigDir, captureMode string) int {
	fmt.Fprintln(os.Stderr,
		"relay: host mode (fd ingress) is not supported on windows; "+
			"use --mode=vm or --mode=container (TCP/IP baseline)")
	return 2
}

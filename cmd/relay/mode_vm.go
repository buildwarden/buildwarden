package main

import (
	"fmt"
	"os"

	"warden/relay"
)

// runVMMode starts the relay inside a dedicated VM (e.g. Alpine relay VM
// for QEMU driver). Binds directly to interfaces. No SSRF filter needed
// because the hypervisor provides isolation.
func runVMMode(outDir, ctxDir, sigDir, captureMode string) int {
	r, err := relay.Start(relay.Config{
		LedgerDir:   outDir,
		ContextDir:  ctxDir,
		CaptureMode: captureMode,
		SignalDir:   sigDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting relay: %v\n", err)
		return 1
	}

	if err := r.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		return 1
	}
	return 0
}

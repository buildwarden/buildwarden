package main

import (
	"fmt"
	"os"

	"github.com/buildwarden/buildwarden/relay"
)

// runVMMode starts the relay inside a dedicated VM (e.g. Alpine relay VM
// for QEMU driver). Binds directly to interfaces. No SSRF filter needed
// because the hypervisor provides isolation.
func runVMMode(outDir, ctxDir, sigDir, captureMode string) int {
	cfg := relay.Config{
		LedgerDir:   outDir,
		ContextDir:  ctxDir,
		CaptureMode: captureMode,
		SignalDir:   sigDir,
	}

	if err := loadUpstreamCA(outDir, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error configuring upstream TLS: %v\n", err)
		return 1
	}

	r, err := relay.Start(cfg)
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

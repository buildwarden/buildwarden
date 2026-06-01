package main

import (
	"fmt"
	"os"

	"github.com/buildwarden/buildwarden/relay"
)

// runContainerMode starts the relay as a sidecar container on an isolated
// Docker network. Binds directly to interfaces. No SSRF filter needed
// because iptables rules (applied by init container) provide isolation.
func runContainerMode(outDir, ctxDir, captureMode string) int {
	r, err := relay.Start(relay.Config{
		LedgerDir:   outDir,
		ContextDir:  ctxDir,
		CaptureMode: captureMode,
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

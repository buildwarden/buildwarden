package main

import (
	"fmt"
	"os"

	"github.com/buildwarden/buildwarden/relay"
)

// runVMMode starts the relay inside a dedicated VM (e.g. Alpine relay VM
// for QEMU driver). Binds directly to interfaces. No SSRF filter needed
// because the hypervisor provides isolation.
func runVMMode(outDir, ctxDir, sigDir, captureMode, scriptRel string) int {
	cfg := vmConfig(outDir, ctxDir, sigDir, captureMode, scriptRel)

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

// vmConfig assembles the relay config for VM mode. When OUTPUT_SINK_URL is set
// (drivers with no host-shared filesystem, e.g. Hyper-V), the relay streams its
// outputs and ready/complete signals to that collector over HTTP instead of the
// local filesystem; OUTPUT_SINK_TOKEN is the bearer token presented on every
// collector write. Both empty (qemu/vz, which share a host path) selects the
// local sink, preserving today's behavior. These env vars are delivered to the
// VM per build (baked seed or, on Hyper-V, fetched over the isolated network);
// the relay itself only consumes them.
func vmConfig(outDir, ctxDir, sigDir, captureMode, scriptRel string) relay.Config {
	return relay.Config{
		LedgerDir:       outDir,
		ContextDir:      ctxDir,
		CaptureMode:     captureMode,
		SignalDir:       sigDir,
		BuildScriptPath: scriptRel,
		OutputSinkURL:   os.Getenv("OUTPUT_SINK_URL"),
		OutputSinkToken: os.Getenv("OUTPUT_SINK_TOKEN"),
	}
}

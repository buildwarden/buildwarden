package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/relay"
)

// runContainerMode starts the relay as a sidecar container on an isolated
// Docker network. Binds directly to interfaces. No SSRF filter needed
// because iptables rules (applied by init container) provide isolation.
func runContainerMode(outDir, ctxDir, captureMode string) int {
	cfg := relay.Config{
		LedgerDir:   outDir,
		ContextDir:  ctxDir,
		CaptureMode: captureMode,
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

func loadUpstreamCA(ledgerDir string, cfg *relay.Config) error {
	bundlePath := filepath.Join(ledgerDir, "upstream-ca-bundle.pem")
	data, err := os.ReadFile(bundlePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading upstream CA bundle: %w", err)
	}
	cfg.UpstreamCACerts = append(cfg.UpstreamCACerts, data)

	if os.Getenv("RELAY_SYSTEM_CA") == "false" {
		f := false
		cfg.UpstreamSystemCA = &f
	}
	return nil
}

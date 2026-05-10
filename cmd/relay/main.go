package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run())
}

func run() int {
	outDir := os.Getenv("LEDGER_DIR")
	if outDir == "" {
		outDir = "/ledger"
	}

	if err := os.MkdirAll(filepath.Join(outDir, "payloads"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating ledger directory: %v\n", err)
		return 1
	}

	if mode := os.Getenv("CAPTURE_MODE"); mode != "" && mode != "none" {
		if err := os.MkdirAll(filepath.Join(outDir, "captures"), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating captures directory: %v\n", err)
			return 1
		}
		SetCaptureMode(mode)
	}

	ctxDir := os.Getenv("CONTEXT_DIR")
	if ctxDir == "" {
		ctxDir = "/context"
	}
	SetContextDir(ctxDir)

	ledgerFile, err := os.Create(filepath.Join(outDir, "ledger"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating ledger file: %v\n", err)
		return 1
	}
	defer ledgerFile.Close()

	l, err := NewLedger(LedgerConfig{
		Writer:      ledgerFile,
		Environment: map[string]any{"type": "container"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing ledger: %v\n", err)
		return 1
	}
	SetLedger(l)
	SetOutDir(outDir)

	if err := DetectSelfIP(); err != nil {
		fmt.Fprintf(os.Stderr, "error detecting relay IP: %v\n", err)
		return 1
	}
	DetectUpstreamDNS()

	// Generate ephemeral CA for this build.
	if err := GenerateCA(); err != nil {
		fmt.Fprintf(os.Stderr, "error generating CA: %v\n", err)
		return 1
	}

	// Write CA cert for the orchestrator to inject into build container.
	caPath := filepath.Join(outDir, "ca.cert.pem")
	if err := os.WriteFile(caPath, CA_CERT, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing CA cert: %v\n", err)
		return 1
	}

	listenIP := net.IPv4zero
	errs := make(chan error, 3)

	go func() { errs <- RunDns(net.TCPAddr{IP: listenIP, Port: 53}) }()
	go func() { errs <- RunHttp(net.TCPAddr{IP: listenIP, Port: 80}) }()
	go func() { errs <- RunHttps(net.TCPAddr{IP: listenIP, Port: 443}) }()

	fmt.Fprintf(os.Stderr, "relay: listening on :53/udp :80/tcp :443/tcp\n")

	// Block until any listener fails.
	if err := <-errs; err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		l.Finish()
		return 1
	}

	return 0
}

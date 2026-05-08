package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"warden/relay"
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

	ledgerFile, err := os.Create(filepath.Join(outDir, "ledger"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating ledger file: %v\n", err)
		return 1
	}
	defer ledgerFile.Close()

	err = relay.NewLedger(ledgerFile, map[string]any{
		"type": "container",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing ledger: %v\n", err)
		return 1
	}

	relay.SetOutDir(outDir)

	// Write public cert files.
	certPEM := relay.PublicCertPEM()
	certPath := filepath.Join(outDir, "ledger.cert.pem")
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing cert PEM: %v\n", err)
		return 1
	}

	listenIP := net.IPv4zero
	errs := make(chan error, 3)

	go func() { errs <- relay.RunDns(net.TCPAddr{IP: listenIP, Port: 53}) }()
	go func() { errs <- relay.RunHttp(net.TCPAddr{IP: listenIP, Port: 80}) }()
	go func() { errs <- relay.RunHttps(net.TCPAddr{IP: listenIP, Port: 443}) }()

	fmt.Fprintf(os.Stderr, "relay: listening on :53/udp :80/tcp :443/tcp\n")

	// Block until any listener fails.
	if err := <-errs; err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		relay.FinishLedger()
		return 1
	}

	return 0
}

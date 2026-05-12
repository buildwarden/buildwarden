package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/fxamacker/cbor/v2"
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

	// Record build environment as the first ledger entry (if provided).
	// The orchestrator writes these files before starting the relay.
	if err := recordEnvironmentFromVolume(outDir, l); err != nil {
		fmt.Fprintf(os.Stderr, "error recording environment: %v\n", err)
		return 1
	}

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

// recordEnvironmentFromVolume reads the environment payload and metadata
// from the ledger volume and writes it as the first ledger entry. This
// MUST be the first record after the header — the relay writes it during
// startup before accepting any network traffic.
//
// Expected files:
//   - <ledgerDir>/environment/payload   (raw bytes to hash — e.g. OCI manifest)
//   - <ledgerDir>/environment/metadata  (CBOR-encoded metadata)
//
// If the environment directory does not exist, this is a no-op (the
// environment record is optional for backwards compatibility).
func recordEnvironmentFromVolume(ledgerDir string, l *Ledger) error {
	envDir := filepath.Join(ledgerDir, "environment")
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		return nil
	}

	payload, err := os.ReadFile(filepath.Join(envDir, "payload"))
	if err != nil {
		return fmt.Errorf("reading environment payload: %w", err)
	}
	if len(payload) == 0 {
		return fmt.Errorf("environment payload is empty")
	}

	metaBytes, err := os.ReadFile(filepath.Join(envDir, "metadata"))
	if err != nil {
		return fmt.Errorf("reading environment metadata: %w", err)
	}

	// Validate that metadata is valid CBOR.
	var meta map[string]any
	if err := cbor.Unmarshal(metaBytes, &meta); err != nil {
		return fmt.Errorf("invalid environment metadata CBOR: %w", err)
	}

	// Write as the first record: open + close with environment schema.
	openMeta, _ := cbor.Marshal(map[string]any{
		"type": "environment",
	})
	openSig := l.Open(schemaEnvCtr, openMeta)

	hashBlock := l.ComputeHashBlock(payload)
	l.Close(openSig, -int64(len(payload)), hashBlock, schemaEnvCtr, metaBytes)

	fmt.Fprintf(os.Stderr, "relay: environment recorded (%d bytes)\n",
		len(payload))
	return nil
}

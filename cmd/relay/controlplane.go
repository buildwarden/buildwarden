package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
)

const maxOutputBytes = 256 * 1024 * 1024 // 256 MB cap on build output

var outputBytesWritten atomic.Int64

// RunControlPlane starts the operational HTTP server on :8300.
// Serves: health check, CA cert, stdout streaming, completion signal.
// This is NOT part of the transparent proxy — it's the agent-to-relay
// control channel. None of this data enters the ledger.
func RunControlPlane(ledgerDir string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok")) //nolint:errcheck
	})

	mux.HandleFunc("/ca.pem", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(CA_CERT) //nolint:errcheck
	})

	mux.HandleFunc("/v1/output", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		remaining := maxOutputBytes - outputBytesWritten.Load()
		if remaining <= 0 {
			http.Error(w, "output limit exceeded", http.StatusRequestEntityTooLarge)
			return
		}
		outPath := filepath.Join(ledgerDir, "build-output.log")
		f, err := os.OpenFile(outPath,
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer f.Close()
		n, _ := io.Copy(f, io.LimitReader(r.Body, remaining))
		outputBytesWritten.Add(n)
		w.WriteHeader(200)
	})

	mux.HandleFunc("/v1/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		log.Printf("relay: build complete: %s", string(body))
		w.WriteHeader(200)
	})

	ln, err := net.Listen("tcp", ":8300")
	if err != nil {
		return err
	}
	return (&http.Server{Handler: mux}).Serve(ln)
}


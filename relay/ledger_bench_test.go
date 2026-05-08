package relay

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"testing"
)

// BenchmarkEd25519Sign measures raw Ed25519 signing (the serial loop's core cost).
func BenchmarkEd25519Sign(b *testing.B) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	input := make([]byte, 300)
	rand.Read(input) //nolint:errcheck
	digest := sha512.Sum512(input)

	b.ResetTimer()
	for b.Loop() {
		ed25519.Sign(priv, digest[:])
	}
}

// BenchmarkHashPayload measures multi-hash computation for various payload sizes.
func BenchmarkHashPayload(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"256B", 256},
		{"4KB", 4096},
		{"64KB", 65536},
		{"1MB", 1 << 20},
	}
	for _, s := range sizes {
		data := make([]byte, s.size)
		rand.Read(data) //nolint:errcheck
		b.Run(s.name, func(b *testing.B) {
			for b.Loop() {
				hs := newHasherSet(defaultHashes)
				hs.write(data)
				hs.sums()
			}
		})
	}
}

// BenchmarkLedgerOpen measures the synchronous Open path (enqueue + sign + write).
func BenchmarkLedgerOpen(b *testing.B) {
	setupBenchLedger(b)
	meta := map[string]any{"method": "GET", "url": "https://example.com/pkg.tar.gz"}

	b.ResetTimer()
	for b.Loop() {
		ledger.Open(meta)
	}
}

// BenchmarkLedgerCheckpoint measures checkpoint with a small raw payload (request headers).
func BenchmarkLedgerCheckpoint(b *testing.B) {
	setupBenchLedger(b)
	openSig := ledger.Open(map[string]any{"method": "GET"})
	payload := []byte("GET /pkg.tar.gz HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\n\r\n")

	b.ResetTimer()
	for b.Loop() {
		ledger.Checkpoint(openSig, "out", payload, nil)
	}
}

// BenchmarkLedgerCheckpointHashed measures checkpoint with pre-computed hashes.
func BenchmarkLedgerCheckpointHashed(b *testing.B) {
	setupBenchLedger(b)
	openSig := ledger.Open(map[string]any{"method": "GET"})
	hashes := map[string]string{
		"blake2b_256": "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8",
		"sha256":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"sha1":        "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"md5":         "d41d8cd98f00b204e9800998ecf8427e",
	}

	b.ResetTimer()
	for b.Loop() {
		ledger.CheckpointHashed(openSig, "out", 1024, hashes)
	}
}

// BenchmarkLedgerFullRequest simulates a complete HTTP GET flow (open + 2 checkpoints + close).
func BenchmarkLedgerFullRequest(b *testing.B) {
	setupBenchLedger(b)
	reqHeaders := []byte("GET /pkg.tar.gz HTTP/1.1\r\nHost: example.com\r\n\r\n")
	respHeaders := []byte("HTTP/1.1 200 OK\r\nContent-Length: 1024\r\n\r\n")
	bodyHashes := map[string]string{
		"blake2b_256": "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8",
		"sha256":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"sha1":        "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"md5":         "d41d8cd98f00b204e9800998ecf8427e",
	}

	b.ResetTimer()
	for b.Loop() {
		sig := ledger.Open(map[string]any{
			"method": "GET", "url": "https://example.com/pkg.tar.gz",
		})
		ledger.Checkpoint(sig, "out", reqHeaders, nil)
		ledger.Checkpoint(sig, "in", respHeaders, nil)
		ledger.CloseHashed(sig, "in", 1024, bodyHashes, map[string]any{"status": 200})
	}
}

// BenchmarkLedgerThroughput measures sustained entries/sec with mixed operations.
func BenchmarkLedgerThroughput(b *testing.B) {
	setupBenchLedger(b)
	hashes := map[string]string{
		"blake2b_256": "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8",
		"sha256":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"sha1":        "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"md5":         "d41d8cd98f00b204e9800998ecf8427e",
	}

	b.ResetTimer()
	for b.Loop() {
		sig := ledger.Open(nil)
		ledger.CheckpointHashed(sig, "out", 64, hashes)
		ledger.CheckpointHashed(sig, "in", 256, hashes)
		ledger.CloseHashed(sig, "in", 4096, hashes, nil)
	}
}

func setupBenchLedger(b *testing.B) {
	b.Helper()
	if err := NewLedger(io.Discard, map[string]any{"type": "bench"}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		FinishLedger()
		ledger = nil
	})
}

// BenchmarkBase64Decode measures the base64 decode cost for prev_sig (256 bytes RSA sig).
func BenchmarkBase64Decode(b *testing.B) {
	// RSA-2048 signature is 256 bytes, base64 encoded is 344 chars
	raw := make([]byte, 256)
	rand.Read(raw) //nolint:errcheck
	encoded := base64.StdEncoding.EncodeToString(raw)

	b.ResetTimer()
	for b.Loop() {
		base64.StdEncoding.DecodeString(encoded) //nolint:errcheck
	}
}

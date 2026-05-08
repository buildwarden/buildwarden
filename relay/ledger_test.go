package relay

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestLedgerBasicFlow(t *testing.T) { //nolint:gocyclo
	var buf bytes.Buffer
	err := NewLedger(&buf, map[string]any{"type": "container", "digest": "sha256:abc123"})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Open a channel (synchronous — returns signature)
	openSig := ledger.Open(map[string]any{
		"method": "GET", "url": "https://example.com/pkg.tar.gz",
	})
	if openSig == "" {
		t.Fatal("Open returned empty signature")
	}

	// Checkpoint: request headers out
	reqHeaders := "GET /pkg.tar.gz HTTP/1.1\r\nHost: example.com\r\n\r\n"
	ledger.Checkpoint(openSig, "out", []byte(reqHeaders), nil)

	// Checkpoint: response headers in
	respHeaders := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n"
	ledger.Checkpoint(openSig, "in", []byte(respHeaders), nil)

	// Close: response body in
	ledger.Close(openSig, "in", []byte("hello"), map[string]any{"status": 200})

	FinishLedger()

	// Parse and verify the ledger
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines (header + open + 2 checkpoints + close),"+
			" got %d", len(lines))
	}

	// Verify header
	var header HeaderEntry
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header.EntryType != "header" {
		t.Errorf("header entry_type = %q, want %q", header.EntryType, "header")
	}
	if header.Version != "2.0" {
		t.Errorf("header version = %q, want %q", header.Version, "2.0")
	}
	if header.SignatureScheme != "ed25519-sha512" {
		t.Errorf("header signature_scheme = %q, want %q",
			header.SignatureScheme, "ed25519-sha512")
	}
	if len(header.Hashes) != 4 {
		t.Errorf("header hashes count = %d, want 4", len(header.Hashes))
	}

	// Verify entries have correct types and sequencing
	types := []string{"open", "checkpoint", "checkpoint", "close"}
	var entries []LedgerEntry
	for i, line := range lines[1:] {
		var e LedgerEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal entry %d: %v", i, err)
		}
		if e.EntryType != types[i] {
			t.Errorf("entry %d type = %q, want %q", i, e.EntryType, types[i])
		}
		if e.Seq != int64(i+1) {
			t.Errorf("entry %d seq = %d, want %d", i, e.Seq, i+1)
		}
		entries = append(entries, e)
	}

	// Verify open signature is referenced by checkpoints and close
	for i, e := range entries[1:] {
		if e.OpenSignature != openSig {
			t.Errorf("entry %d open_signature mismatch", i+1)
		}
	}

	// Verify close has payload with correct size
	closeEntry := entries[3]
	if closeEntry.Payload == nil {
		t.Fatal("close entry has nil payload")
	}
	if closeEntry.Payload.Size != 5 {
		t.Errorf("close payload size = %d, want 5", closeEntry.Payload.Size)
	}

	// Verify signature chain is valid using the public key from header
	pubKey := extractPublicKey(t, header)
	verifySigChain(t, header, entries, pubKey)

	// Reset global for other tests
	ledger = nil
}

func TestLedgerMultipleChannels(t *testing.T) {
	var buf bytes.Buffer
	err := NewLedger(&buf, map[string]any{"type": "container"})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Open two channels
	sig1 := ledger.Open(map[string]any{"url": "https://a.com"})
	sig2 := ledger.Open(map[string]any{"url": "https://b.com"})

	if sig1 == sig2 {
		t.Error("two opens produced the same signature")
	}

	// Close them in reverse order
	ledger.Close(sig2, "in", []byte("body2"), nil)
	ledger.Close(sig1, "in", []byte("body1"), nil)

	FinishLedger()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// header + 2 opens + 2 closes = 5
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	ledger = nil
}

func extractPublicKey(t *testing.T, header HeaderEntry) ed25519.PublicKey {
	t.Helper()
	pub, _ := ledger.key.Public().(ed25519.PublicKey)
	return pub
}

func verifySigChain(
	t *testing.T, header HeaderEntry, entries []LedgerEntry, pubKey ed25519.PublicKey,
) {
	t.Helper()

	// Verify header signature
	var headerInput []byte
	headerInput = append(headerInput, []byte("header")...)
	headerInput = append(headerInput, sizeBytes(header.Payload.Size)...)
	headerInput = append(headerInput,
		rawHashBytesOrdered(header.Payload.Hashes, header.Hashes)...)

	verifySignature(t, "header", headerInput, header.Signature, pubKey)

	// Verify entry chain
	prevSig := header.Signature
	for i, e := range entries {
		var sigInput []byte
		prevSigBytes, _ := base64.StdEncoding.DecodeString(prevSig)
		sigInput = append(sigInput, prevSigBytes...)

		switch e.EntryType {
		case "open":
			sigInput = append(sigInput, []byte("open")...)
		case "checkpoint", "close":
			openSigBytes, _ := base64.StdEncoding.DecodeString(e.OpenSignature)
			sigInput = append(sigInput, openSigBytes...)
			sigInput = append(sigInput, []byte(e.EntryType)...)
			sigInput = append(sigInput, []byte(e.Direction)...)
			sigInput = append(sigInput, sizeBytes(e.Payload.Size)...)
			sigInput = append(sigInput,
				rawHashBytesOrdered(e.Payload.Hashes, defaultHashes)...)
		}

		verifySignature(t, entryLabel(i, e), sigInput, e.Signature, pubKey)
		prevSig = e.Signature
	}
}

func verifySignature(
	t *testing.T, label string, input []byte, sigB64 string,
	pubKey ed25519.PublicKey,
) {
	t.Helper()
	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("%s: decode signature: %v", label, err)
	}
	digest := sha512.Sum512(input)
	if !ed25519.Verify(pubKey, digest[:], sigBytes) {
		t.Errorf("%s: signature verification failed", label)
	}
}

func rawHashBytesOrdered(hashes map[string]string, order []string) []byte {
	var out []byte
	for _, name := range order {
		b, _ := decodeHex(hashes[name])
		out = append(out, b...)
	}
	return out
}

func decodeHex(s string) ([]byte, error) {
	return hexDecode(s), nil
}

func hexDecode(s string) []byte {
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		b[i/2] = hexVal(s[i])<<4 | hexVal(s[i+1])
	}
	return b
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func entryLabel(i int, e LedgerEntry) string {
	return e.EntryType + "[" + string(rune('0'+i)) + "]"
}

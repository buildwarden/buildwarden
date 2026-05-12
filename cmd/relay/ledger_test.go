package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/binary"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestLedgerHeader(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "container", "digest": "sha256:abc"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	l.Finish()

	data := buf.Bytes()

	// Magic
	if string(data[0:4]) != "BLDL" {
		t.Fatalf("magic = %q, want BLDL", data[0:4])
	}
	if data[4] != 0x01 {
		t.Fatalf("version = %d, want 1", data[4])
	}

	// Signature scheme (null-terminated)
	schemeEnd := bytes.IndexByte(data[5:], 0x00)
	if schemeEnd < 0 {
		t.Fatal("no null terminator for signature scheme")
	}
	scheme := string(data[5 : 5+schemeEnd])
	if scheme != "ed25519-sha512" {
		t.Fatalf("scheme = %q, want ed25519-sha512", scheme)
	}
	off := 5 + schemeEnd + 1

	// Sizes
	sigSize := binary.BigEndian.Uint16(data[off:])
	off += 2
	hashBlockSize := binary.BigEndian.Uint16(data[off:])
	off += 2
	pubKeyLen := binary.BigEndian.Uint16(data[off:])
	off += 2

	if sigSize != 64 {
		t.Fatalf("sigSize = %d, want 64", sigSize)
	}
	if hashBlockSize != 100 { // 32+32+20+16
		t.Fatalf("hashBlockSize = %d, want 100", hashBlockSize)
	}
	if pubKeyLen != 32 {
		t.Fatalf("pubKeyLen = %d, want 32", pubKeyLen)
	}

	// Public key
	pubKey := data[off : off+int(pubKeyLen)]
	off += int(pubKeyLen)
	prefixEnd := off

	if !bytes.Equal(pubKey, []byte(l.PublicKey())) {
		t.Fatal("public key mismatch")
	}

	// Verify header signature
	sig := data[off : off+int(sigSize)]
	off += int(sigSize)

	digest := sha512.Sum512(data[:prefixEnd])
	if !ed25519.Verify(l.PublicKey(), digest[:], sig) {
		t.Fatal("header signature verification failed")
	}

	// CBOR metadata
	metaLen := binary.BigEndian.Uint32(data[off:])
	off += 4
	metaBytes := data[off : off+int(metaLen)]

	var meta HeaderMeta
	if err := cbor.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal header meta: %v", err)
	}
	if len(meta.Hashes) != 4 {
		t.Fatalf("meta.Hashes len = %d, want 4", len(meta.Hashes))
	}
	if len(meta.Schemas) != 6 {
		t.Fatalf("meta.Schemas len = %d, want 6", len(meta.Schemas))
	}
}

func TestLedgerBasicFlow(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Open
	openSig := l.Open(SchemaNoMetadata, nil)
	if len(openSig) != 64 {
		t.Fatalf("open sig len = %d, want 64", len(openSig))
	}

	// Checkpoint out (request headers)
	headers := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	hb := l.ComputeHashBlock(headers)
	l.Checkpoint(openSig, -int64(len(headers)), hb, SchemaNoMetadata, nil)

	// Close in (response body)
	body := []byte("hello world")
	hb2 := l.ComputeHashBlock(body)
	l.Close(openSig, int64(len(body)), hb2, SchemaNoMetadata, nil)

	l.Finish()

	// Verify the full signature chain
	verifyLedgerChain(t, buf.Bytes(), l.PublicKey())
}

func TestLedgerArtifact(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	openSig := l.Open(SchemaNoMetadata, nil)

	// Artifact with payload
	artifact := []byte("binary content")
	hb := l.ComputeHashBlock(artifact)
	meta, _ := cbor.Marshal(map[string]any{"name": "output.bin", "context": map[string]any{}})
	l.Artifact(openSig, -int64(len(artifact)), hb, 3, meta) // schema index 3 = artifact

	l.Finish()
	verifyLedgerChain(t, buf.Bytes(), l.PublicKey())
}

func TestLedgerEmptyArtifact(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	openSig := l.Open(SchemaNoMetadata, nil)

	// Empty artifact (payload=0, no hash block)
	meta, _ := cbor.Marshal(map[string]any{"name": "marker", "context": map[string]any{}})
	l.Artifact(openSig, 0, nil, 3, meta)

	l.Finish()
	verifyLedgerChain(t, buf.Bytes(), l.PublicKey())
}

func TestLedgerWithMetadata(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	openMeta, _ := cbor.Marshal(map[string]any{
		"method": "GET", "url": "https://example.com/file", "protocol": "HTTP/1.1",
	})
	openSig := l.Open(0, openMeta) // schema index 0 = http-open

	body := []byte("response")
	hb := l.ComputeHashBlock(body)
	bodyMeta, _ := cbor.Marshal(map[string]any{"status": 200})
	l.Close(openSig, int64(len(body)), hb, 2, bodyMeta) // schema index 2 = http-body

	l.Finish()
	verifyLedgerChain(t, buf.Bytes(), l.PublicKey())
}

func TestLedgerStreamingHasher(t *testing.T) {
	data := []byte("hello world, this is a streaming hash test")

	// Compute via streaming
	sh := NewStreamingHasher(defaultHashes)
	sh.Write(data[:10])  //nolint:errcheck
	sh.Write(data[10:])  //nolint:errcheck
	block, size := sh.Finish()

	if size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", size, len(data))
	}

	// Compute via one-shot for comparison
	var expected []byte
	for _, name := range defaultHashes {
		h := newHash(name)
		h.Write(data)
		expected = h.Sum(expected)
	}

	if !bytes.Equal(block, expected) {
		t.Fatal("streaming hash block doesn't match one-shot")
	}
}

// verifyLedgerChain parses the binary ledger and verifies every signature.
func verifyLedgerChain(t *testing.T, data []byte, pubKey ed25519.PublicKey) {
	t.Helper()
	off := 0

	// Parse header
	if string(data[off:off+4]) != "BLDL" {
		t.Fatal("bad magic")
	}
	off += 5 // magic + version

	// Signature scheme
	nullIdx := bytes.IndexByte(data[off:], 0x00)
	off += nullIdx + 1

	sigSize := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	hashBlockSize := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	pubKeyLen := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	off += pubKeyLen // skip public key bytes
	prefixEnd := off

	// Verify header signature
	headerSig := data[off : off+sigSize]
	off += sigSize
	digest := sha512.Sum512(data[:prefixEnd])
	if !ed25519.Verify(pubKey, digest[:], headerSig) {
		t.Fatal("header signature invalid")
	}

	// Skip header CBOR metadata
	metaLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4 + metaLen

	prevSig := headerSig

	// Parse records
	recordNum := 0
	for off < len(data) {
		recordType := data[off]
		off++

		// Build signature input
		var sigInput []byte
		sigInput = append(sigInput, recordType)
		sigInput = append(sigInput, data[off:off+sigSize]...)

		// Verify prev_sig field matches
		if !bytes.Equal(data[off:off+sigSize], prevSig) {
			t.Fatalf("record %d: prev_sig mismatch", recordNum)
		}
		off += sigSize

		// Open signature (not present for open records)
		if recordType != RecordOpen {
			sigInput = append(sigInput, data[off:off+sigSize]...)
			off += sigSize
		}

		// Payload size
		payloadSize := int64(binary.BigEndian.Uint64(data[off:]))
		sigInput = binary.BigEndian.AppendUint64(sigInput, uint64(payloadSize))
		off += 8

		// Hash block (present only when payload != 0)
		if payloadSize != 0 {
			sigInput = append(sigInput, data[off:off+hashBlockSize]...)
			off += hashBlockSize
		}

		// Record signature
		recSig := data[off : off+sigSize]
		off += sigSize

		d := sha512.Sum512(sigInput)
		if !ed25519.Verify(pubKey, d[:], recSig) {
			t.Fatalf("record %d (type 0x%02x): signature invalid",
			recordNum, recordType)
		}

		// Schema index + optional metadata
		schemaIdx := data[off]
		off++
		if schemaIdx != SchemaNoMetadata {
			mLen := int(binary.BigEndian.Uint32(data[off:]))
			off += 4 + mLen
		}

		prevSig = recSig
		recordNum++
	}

	if recordNum == 0 {
		t.Fatal("no records found")
	}
}

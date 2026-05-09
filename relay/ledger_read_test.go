package relay

import (
	"bytes"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestLedgerReadHeader(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	l.Finish()

	h, _, err := ReadHeader(buf.Bytes())
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if h.Version != 0x01 {
		t.Errorf("version = %d, want 1", h.Version)
	}
	if h.SigScheme != "ed25519-sha512" {
		t.Errorf("scheme = %q", h.SigScheme)
	}
	if h.SigSize != 64 {
		t.Errorf("sigSize = %d", h.SigSize)
	}
	if h.HashBlockSize != 100 {
		t.Errorf("hashBlockSize = %d", h.HashBlockSize)
	}
	if len(h.Meta.Hashes) != 4 {
		t.Errorf("meta hashes = %d", len(h.Meta.Hashes))
	}
	if len(h.Meta.Schemas) != 5 {
		t.Errorf("meta schemas = %d", len(h.Meta.Schemas))
	}
}

func TestLedgerVerifyBasicFlow(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	openSig := l.Open(SchemaNoMetadata, nil)
	body := []byte("response data")
	hb := l.ComputeHashBlock(body)
	l.Close(openSig, int64(len(body)), hb, SchemaNoMetadata, nil)
	l.Finish()

	result, err := Verify(buf.Bytes())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Valid {
		t.Fatalf("ledger invalid, sigErrors=%d", result.SigErrors)
	}
	if result.TotalRecords != 2 {
		t.Fatalf("records = %d, want 2", result.TotalRecords)
	}
	if result.Unclosed != 0 {
		t.Fatalf("unclosed = %d", result.Unclosed)
	}
}

func TestLedgerVerifyHTTPFlow(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "container"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Open with http-open metadata
	openMeta, _ := cbor.Marshal(map[string]any{
		"method": "GET", "url": "https://example.com/pkg.tar.gz", "protocol": "HTTP/1.1",
	})
	openSig := l.Open(0, openMeta)

	// Checkpoint: request headers out
	reqHeaders := []byte("GET /pkg.tar.gz HTTP/1.1\r\nHost: example.com\r\n\r\n")
	hb1 := l.ComputeHashBlock(reqHeaders)
	headersMeta, _ := cbor.Marshal(map[string]any{
		"headers": [][]string{{"X-Request-Id", "abc123"}},
	})
	l.Checkpoint(openSig, -int64(len(reqHeaders)), hb1, 1, headersMeta)

	// Checkpoint: response headers in
	respHeaders := []byte("HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\n")
	hb2 := l.ComputeHashBlock(respHeaders)
	l.Checkpoint(openSig, int64(len(respHeaders)), hb2, 1, headersMeta)

	// Close: response body in
	body := []byte("hello world")
	hb3 := l.ComputeHashBlock(body)
	bodyMeta, _ := cbor.Marshal(map[string]any{"status": 200})
	l.Close(openSig, int64(len(body)), hb3, 2, bodyMeta)

	l.Finish()

	result, err := Verify(buf.Bytes())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Valid {
		t.Fatalf("ledger invalid, sigErrors=%d", result.SigErrors)
	}
	if result.TotalRecords != 4 {
		t.Fatalf("records = %d, want 4", result.TotalRecords)
	}
	if result.Unclosed != 0 {
		t.Fatalf("unclosed = %d", result.Unclosed)
	}

	// Verify record types
	expected := []byte{RecordOpen, RecordCheckpoint, RecordCheckpoint, RecordClose}
	for i, rec := range result.Records {
		if rec.Type != expected[i] {
			t.Errorf("record %d type = 0x%02x, want 0x%02x", i, rec.Type, expected[i])
		}
	}

	// Verify directions
	if result.Records[1].Direction() != "out" {
		t.Errorf("record 1 direction = %q, want out", result.Records[1].Direction())
	}
	if result.Records[2].Direction() != "in" {
		t.Errorf("record 2 direction = %q, want in", result.Records[2].Direction())
	}
	if result.Records[3].Direction() != "in" {
		t.Errorf("record 3 direction = %q, want in", result.Records[3].Direction())
	}
}

func TestLedgerVerifyArtifact(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	openSig := l.Open(SchemaNoMetadata, nil)
	artifact := []byte("build output binary")
	hb := l.ComputeHashBlock(artifact)
	meta, _ := cbor.Marshal(map[string]any{"name": "app", "context": map[string]any{}})
	l.Artifact(openSig, -int64(len(artifact)), hb, 3, meta)
	l.Finish()

	result, err := Verify(buf.Bytes())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Valid {
		t.Fatalf("ledger invalid, sigErrors=%d", result.SigErrors)
	}
	if result.Unclosed != 0 {
		t.Fatalf("unclosed = %d, want 0", result.Unclosed)
	}
	if result.Records[1].Type != RecordArtifact {
		t.Fatalf("record 1 type = 0x%02x, want artifact", result.Records[1].Type)
	}
}

func TestLedgerVerifyTamperDetection(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	openSig := l.Open(SchemaNoMetadata, nil)
	body := []byte("data")
	hb := l.ComputeHashBlock(body)
	l.Close(openSig, int64(len(body)), hb, SchemaNoMetadata, nil)
	l.Finish()

	// Tamper: flip a byte in the middle of the ledger
	data := buf.Bytes()
	tampered := make([]byte, len(data))
	copy(tampered, data)
	tampered[len(tampered)/2] ^= 0xFF

	result, err := Verify(tampered)
	if err != nil {
		// Parse error is acceptable for tampered data
		return
	}
	if result.Valid {
		t.Fatal("tampered ledger should not verify as valid")
	}
	if result.SigErrors == 0 {
		t.Fatal("expected signature errors for tampered ledger")
	}
}

func TestLedgerVerifyMultipleChannels(t *testing.T) {
	var buf bytes.Buffer
	l, err := NewLedger(LedgerConfig{
		Writer:      &buf,
		Environment: map[string]any{"type": "test"},
	})
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	sig1 := l.Open(SchemaNoMetadata, nil)
	sig2 := l.Open(SchemaNoMetadata, nil)

	body1 := []byte("first")
	body2 := []byte("second")
	l.Close(sig2, int64(len(body2)), l.ComputeHashBlock(body2), SchemaNoMetadata, nil)
	l.Close(sig1, int64(len(body1)), l.ComputeHashBlock(body1), SchemaNoMetadata, nil)
	l.Finish()

	result, err := Verify(buf.Bytes())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Valid {
		t.Fatalf("ledger invalid, sigErrors=%d", result.SigErrors)
	}
	if result.TotalRecords != 4 {
		t.Fatalf("records = %d, want 4", result.TotalRecords)
	}
	if result.Unclosed != 0 {
		t.Fatalf("unclosed = %d", result.Unclosed)
	}
}

func TestLedgerIsValidLedger(t *testing.T) {
	if IsValidLedger([]byte("BLDL\x01rest")) != true {
		t.Error("should detect valid ledger")
	}
	if IsValidLedger([]byte(`{"entry_type":"header"`)) != false {
		t.Error("should not detect JSON as valid ledger")
	}
	if IsValidLedger([]byte("BLD")) != false {
		t.Error("should not detect short data as valid ledger")
	}
}

func TestLedgerRecordHelpers(t *testing.T) {
	r := &Record{PayloadSize: 100}
	if r.Direction() != "in" {
		t.Errorf("positive direction = %q", r.Direction())
	}
	if r.AbsPayloadSize() != 100 {
		t.Errorf("abs = %d", r.AbsPayloadSize())
	}

	r.PayloadSize = -50
	if r.Direction() != "out" {
		t.Errorf("negative direction = %q", r.Direction())
	}
	if r.AbsPayloadSize() != 50 {
		t.Errorf("abs = %d", r.AbsPayloadSize())
	}

	r.PayloadSize = 0
	if r.Direction() != "" {
		t.Errorf("zero direction = %q", r.Direction())
	}

	r.Type = RecordArtifact
	if r.TypeName() != "artifact" {
		t.Errorf("typeName = %q", r.TypeName())
	}
}

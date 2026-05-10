package ledger

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// ─── Constants ────────────────────────────────────────────────────────

func TestRecordTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  byte
		want byte
	}{
		{"RecordOpen", RecordOpen, 0x01},
		{"RecordCheckpoint", RecordCheckpoint, 0x02},
		{"RecordClose", RecordClose, 0x03},
		{"RecordArtifact", RecordArtifact, 0x04},
		{"SchemaNoMetadata", SchemaNoMetadata, 0xFF},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = 0x%02x, want 0x%02x", tt.name, tt.got, tt.want)
		}
	}
}

// ─── Record helper methods ────────────────────────────────────────────

func TestRecord_Direction(t *testing.T) {
	tests := []struct {
		name        string
		payloadSize int64
		want        string
	}{
		{"positive payload", 1024, "in"},
		{"negative payload", -512, "out"},
		{"zero payload", 0, ""},
		{"large positive", math.MaxInt64, "in"},
		{"small negative", -1, "out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Record{PayloadSize: tt.payloadSize}
			if got := r.Direction(); got != tt.want {
				t.Errorf("Direction() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecord_AbsPayloadSize(t *testing.T) {
	tests := []struct {
		name        string
		payloadSize int64
		want        int64
	}{
		{"positive", 100, 100},
		{"negative", -200, 200},
		{"zero", 0, 0},
		{"negative one", -1, 1},
		{"large positive", math.MaxInt64, math.MaxInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Record{PayloadSize: tt.payloadSize}
			if got := r.AbsPayloadSize(); got != tt.want {
				t.Errorf("AbsPayloadSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRecord_TypeName(t *testing.T) {
	tests := []struct {
		name     string
		recType  byte
		want     string
		contains string // if non-empty, use Contains check instead
	}{
		{"open", RecordOpen, "open", ""},
		{"checkpoint", RecordCheckpoint, "checkpoint", ""},
		{"close", RecordClose, "close", ""},
		{"artifact", RecordArtifact, "artifact", ""},
		{"unknown 0x00", 0x00, "", "unknown"},
		{"unknown 0xFE", 0xFE, "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Record{Type: tt.recType}
			got := r.TypeName()
			if tt.contains != "" {
				if len(got) == 0 {
					t.Errorf("TypeName() empty, want %q",
						tt.contains)
				}
			} else if got != tt.want {
				t.Errorf("TypeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ─── IsValidLedger ────────────────────────────────────────────────────

func TestIsValidLedger(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid magic+version", []byte("BLDL\x01"), true},
		{"valid with trailing data", []byte("BLDL\x01extra"), true},
		{"wrong version", []byte("BLDL\x02"), false},
		{"wrong magic", []byte("XXXX\x01"), false},
		{"too short 4 bytes", []byte("BLDL"), false},
		{"too short 3 bytes", []byte("BLD"), false},
		{"empty", []byte{}, false},
		{"nil", nil, false},
		{"just magic no version", []byte("BLDL"), false},
		{"version zero", []byte("BLDL\x00"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidLedger(tt.data); got != tt.want {
				t.Errorf("IsValidLedger() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─── ReadHeader ───────────────────────────────────────────────────────

// buildTestHeader constructs a valid binary ledger header for testing.
// It uses a real Ed25519 keypair so the header signature is valid.
func buildTestHeader(t *testing.T) ([]byte, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	sigScheme := "ed25519-sha512"
	sigSize := ed25519.SignatureSize   // 64
	hashBlockSize := sha512.Size       // 64
	pubKeyLen := ed25519.PublicKeySize // 32

	// Build prefix: magic + version + sig scheme (null-terminated) + sizes + pubkey
	var prefix []byte
	prefix = append(prefix, "BLDL"...)
	prefix = append(prefix, 0x01) // version
	prefix = append(prefix, []byte(sigScheme)...)
	prefix = append(prefix, 0x00) // null terminator
	prefix = binary.BigEndian.AppendUint16(prefix, uint16(sigSize))
	prefix = binary.BigEndian.AppendUint16(prefix, uint16(hashBlockSize))
	prefix = binary.BigEndian.AppendUint16(prefix, uint16(pubKeyLen))
	prefix = append(prefix, pub...)

	// Sign the prefix
	digest := sha512.Sum512(prefix)
	sig := ed25519.Sign(priv, digest[:])

	// CBOR metadata
	meta := HeaderMeta{
		Hashes:  []string{"sha512"},
		Schemas: []string{},
	}
	metaBytes, err := cbor.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	// Assemble complete header
	var header []byte
	header = append(header, prefix...)
	header = append(header, sig...)
	header = binary.BigEndian.AppendUint32(header, uint32(len(metaBytes)))
	header = append(header, metaBytes...)

	return header, pub, priv
}

func TestReadHeader_Valid(t *testing.T) {
	data, pub, _ := buildTestHeader(t)

	h, n, err := ReadHeader(data)
	if err != nil {
		t.Fatalf("ReadHeader() error: %v", err)
	}
	if n != len(data) {
		t.Errorf("consumed %d bytes, want %d", n, len(data))
	}
	if h.Version != 0x01 {
		t.Errorf("Version = 0x%02x, want 0x01", h.Version)
	}
	if h.SigScheme != "ed25519-sha512" {
		t.Errorf("SigScheme = %q, want %q", h.SigScheme, "ed25519-sha512")
	}
	if h.SigSize != ed25519.SignatureSize {
		t.Errorf("SigSize = %d, want %d", h.SigSize, ed25519.SignatureSize)
	}
	if h.HashBlockSize != sha512.Size {
		t.Errorf("HashBlockSize = %d, want %d", h.HashBlockSize, sha512.Size)
	}
	if h.PubKeyLen != ed25519.PublicKeySize {
		t.Errorf("PubKeyLen = %d, want %d", h.PubKeyLen, ed25519.PublicKeySize)
	}
	if len(h.PubKey) != ed25519.PublicKeySize {
		t.Fatalf("PubKey length = %d, want %d", len(h.PubKey), ed25519.PublicKeySize)
	}
	for i := range pub {
		if h.PubKey[i] != pub[i] {
			t.Fatalf("PubKey mismatch at byte %d", i)
		}
	}
	if len(h.Signature) != ed25519.SignatureSize {
		t.Errorf("Signature length = %d, want %d", len(h.Signature), ed25519.SignatureSize)
	}
	if len(h.PrefixBytes) == 0 {
		t.Error("PrefixBytes is empty")
	}
	if len(h.Meta.Hashes) != 1 || h.Meta.Hashes[0] != "sha512" {
		t.Errorf("Meta.Hashes = %v, want [sha512]", h.Meta.Hashes)
	}
}

func TestReadHeader_WithTrailingData(t *testing.T) {
	data, _, _ := buildTestHeader(t)
	trailing := []byte("extra trailing bytes here")
	data = append(data, trailing...)

	_, n, err := ReadHeader(data)
	if err != nil {
		t.Fatalf("ReadHeader() error: %v", err)
	}
	// n should be the header size, not include trailing data
	if n == len(data) {
		t.Error("consumed all bytes including trailing data")
	}
	if n+len(trailing) != len(data) {
		t.Errorf("consumed %d bytes, expected %d", n, len(data)-len(trailing))
	}
}

func TestReadHeader_Errors(t *testing.T) {
	validHeader, _, _ := buildTestHeader(t)

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "nil data",
			data:    nil,
			wantErr: "too short for magic",
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: "too short for magic",
		},
		{
			name:    "4 bytes only",
			data:    []byte("BLDL"),
			wantErr: "too short for magic",
		},
		{
			name:    "bad magic",
			data:    []byte("XXXX\x01"),
			wantErr: "invalid magic",
		},
		{
			name:    "no null terminator for sig scheme",
			data:    append([]byte("BLDL\x01"), toBytes("ed25519-sha512")...),
			wantErr: "no null terminator",
		},
		{
			name:    "truncated after sig scheme",
			data:    append([]byte("BLDL\x01ed25519-sha512\x00"), 0x00),
			wantErr: "too short for size fields",
		},
		{
			name: "truncated public key",
			data: func() []byte {
				d := make([]byte, 0, 50)
				d = append(d, "BLDL"...)
				d = append(d, 0x01)
				d = append(d, "ed25519-sha512\x00"...)
				d = binary.BigEndian.AppendUint16(d, 64)  // sigSize
				d = binary.BigEndian.AppendUint16(d, 64)  // hashBlockSize
				d = binary.BigEndian.AppendUint16(d, 255) // pubKeyLen (too large)
				return d
			}(),
			wantErr: "too short for public key",
		},
		{
			name: "truncated header signature",
			data: func() []byte {
				// Use valid header but chop off the signature
				d := make([]byte, 0, 100)
				d = append(d, "BLDL"...)
				d = append(d, 0x01)
				d = append(d, "ed25519-sha512\x00"...)
				d = binary.BigEndian.AppendUint16(d, 64) // sigSize
				d = binary.BigEndian.AppendUint16(d, 64) // hashBlockSize
				d = binary.BigEndian.AppendUint16(d, 32) // pubKeyLen
				d = append(d, make([]byte, 32)...)       // pubkey
				d = append(d, make([]byte, 10)...)       // partial sig
				return d
			}(),
			wantErr: "too short for header signature",
		},
		{
			name: "truncated metadata length",
			data: func() []byte {
				// Valid prefix + full sig but no metadata length
				d := make([]byte, 0, 200)
				d = append(d, "BLDL"...)
				d = append(d, 0x01)
				d = append(d, "ed25519-sha512\x00"...)
				d = binary.BigEndian.AppendUint16(d, 64) // sigSize
				d = binary.BigEndian.AppendUint16(d, 64) // hashBlockSize
				d = binary.BigEndian.AppendUint16(d, 32) // pubKeyLen
				d = append(d, make([]byte, 32)...)       // pubkey
				d = append(d, make([]byte, 64)...)       // full sig
				d = append(d, 0x00, 0x00)                // partial metadata len
				return d
			}(),
			wantErr: "too short for metadata length",
		},
		{
			name: "truncated metadata body",
			data: func() []byte {
				d := make([]byte, 0, 200)
				d = append(d, "BLDL"...)
				d = append(d, 0x01)
				d = append(d, "ed25519-sha512\x00"...)
				d = binary.BigEndian.AppendUint16(d, 64) // sigSize
				d = binary.BigEndian.AppendUint16(d, 64) // hashBlockSize
				d = binary.BigEndian.AppendUint16(d, 32) // pubKeyLen
				d = append(d, make([]byte, 32)...)       // pubkey
				d = append(d, make([]byte, 64)...)       // sig
				d = binary.BigEndian.AppendUint32(d, 100) // metadata len
				d = append(d, make([]byte, 5)...)         // only 5 bytes
				return d
			}(),
			wantErr: "too short for metadata",
		},
		{
			name: "invalid CBOR metadata",
			data: func() []byte {
				d := make([]byte, 0, 200)
				d = append(d, "BLDL"...)
				d = append(d, 0x01)
				d = append(d, "ed25519-sha512\x00"...)
				d = binary.BigEndian.AppendUint16(d, 64) // sigSize
				d = binary.BigEndian.AppendUint16(d, 64) // hashBlockSize
				d = binary.BigEndian.AppendUint16(d, 32) // pubKeyLen
				d = append(d, make([]byte, 32)...)       // pubkey
				d = append(d, make([]byte, 64)...)       // sig
				garbage := []byte{0xFF, 0xFF, 0xFF}
				d = binary.BigEndian.AppendUint32(d, uint32(len(garbage)))
				d = append(d, garbage...)
				return d
			}(),
			wantErr: "decode header metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadHeader(tt.data)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !containsStr(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want %q",
					err.Error(), tt.wantErr)
			}
		})
	}

	// Verify the valid header we use as a base does parse
	_, _, err := ReadHeader(validHeader)
	if err != nil {
		t.Errorf("valid header should parse, got: %v", err)
	}
}

// toBytes is a helper that returns []byte from a string.
func toBytes(s string) []byte { return []byte(s) }

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ─── ReadRecord ───────────────────────────────────────────────────────

func TestReadRecord_OpenRecord(t *testing.T) {
	sigSize := 64
	hashBlockSize := 64

	// Build a minimal open record:
	// type(1) + prevSig(64) + payloadSize(8) + sig(64) + schemaIndex(1)
	// No openSig for open records, no hash block when payloadSize == 0
	var data []byte
	data = append(data, RecordOpen)
	data = append(data, make([]byte, sigSize)...) // prevSig
	data = binary.BigEndian.AppendUint64(data, 0) // payloadSize = 0
	data = append(data, make([]byte, sigSize)...)  // signature
	data = append(data, SchemaNoMetadata)          // no metadata

	r, n, err := ReadRecord(data, sigSize, hashBlockSize)
	if err != nil {
		t.Fatalf("ReadRecord() error: %v", err)
	}
	if n != len(data) {
		t.Errorf("consumed %d bytes, want %d", n, len(data))
	}
	if r.Type != RecordOpen {
		t.Errorf("Type = 0x%02x, want 0x%02x", r.Type, RecordOpen)
	}
	if r.OpenSig != nil {
		t.Error("OpenSig should be nil for open records")
	}
	if r.PayloadSize != 0 {
		t.Errorf("PayloadSize = %d, want 0", r.PayloadSize)
	}
	if r.HashBlock != nil {
		t.Error("HashBlock should be nil when PayloadSize == 0")
	}
	if r.SchemaIndex != SchemaNoMetadata {
		t.Errorf("SchemaIndex = 0x%02x, want 0x%02x", r.SchemaIndex, SchemaNoMetadata)
	}
	if r.Metadata != nil {
		t.Error("Metadata should be nil when SchemaIndex == 0xFF")
	}
}

func TestReadRecord_CheckpointWithPayload(t *testing.T) {
	sigSize := 64
	hashBlockSize := 64

	// Build a checkpoint record with positive payload (has openSig + hashBlock)
	var data []byte
	data = append(data, RecordCheckpoint)
	data = append(data, make([]byte, sigSize)...)          // prevSig
	data = append(data, make([]byte, sigSize)...)          // openSig
	data = binary.BigEndian.AppendUint64(data, 4096)       // payloadSize > 0
	data = append(data, make([]byte, hashBlockSize)...)    // hashBlock
	data = append(data, make([]byte, sigSize)...)          // signature
	data = append(data, SchemaNoMetadata)                  // no metadata

	r, n, err := ReadRecord(data, sigSize, hashBlockSize)
	if err != nil {
		t.Fatalf("ReadRecord() error: %v", err)
	}
	if n != len(data) {
		t.Errorf("consumed %d bytes, want %d", n, len(data))
	}
	if r.Type != RecordCheckpoint {
		t.Errorf("Type = 0x%02x, want 0x%02x", r.Type, RecordCheckpoint)
	}
	if r.OpenSig == nil {
		t.Error("OpenSig should be set for non-open records")
	}
	if r.PayloadSize != 4096 {
		t.Errorf("PayloadSize = %d, want 4096", r.PayloadSize)
	}
	if r.HashBlock == nil {
		t.Error("HashBlock should be set when PayloadSize != 0")
	}
	if len(r.HashBlock) != hashBlockSize {
		t.Errorf("HashBlock length = %d, want %d", len(r.HashBlock), hashBlockSize)
	}
}

func TestReadRecord_NegativePayload(t *testing.T) {
	sigSize := 64
	hashBlockSize := 64

	// Build a close record with negative payload
	var data []byte
	data = append(data, RecordClose)
	data = append(data, make([]byte, sigSize)...) // prevSig
	data = append(data, make([]byte, sigSize)...) // openSig
	// Encode -512 as int64 via two's complement
	neg512 := int64(-512)
	data = binary.BigEndian.AppendUint64(data, uint64(neg512))
	data = append(data, make([]byte, hashBlockSize)...) // hashBlock
	data = append(data, make([]byte, sigSize)...)       // signature
	data = append(data, SchemaNoMetadata)               // no metadata

	r, _, err := ReadRecord(data, sigSize, hashBlockSize)
	if err != nil {
		t.Fatalf("ReadRecord() error: %v", err)
	}
	if r.PayloadSize != -512 {
		t.Errorf("PayloadSize = %d, want -512", r.PayloadSize)
	}
	if r.Direction() != "out" {
		t.Errorf("Direction() = %q, want %q", r.Direction(), "out")
	}
	if r.AbsPayloadSize() != 512 {
		t.Errorf("AbsPayloadSize() = %d, want 512", r.AbsPayloadSize())
	}
}

func TestReadRecord_WithMetadata(t *testing.T) {
	sigSize := 64
	hashBlockSize := 64

	metadata := []byte{0xA0} // empty CBOR map

	var data []byte
	data = append(data, RecordArtifact)
	data = append(data, make([]byte, sigSize)...)          // prevSig
	data = append(data, make([]byte, sigSize)...)          // openSig
	data = binary.BigEndian.AppendUint64(data, 0)          // payloadSize = 0
	data = append(data, make([]byte, sigSize)...)          // signature
	data = append(data, 0x00)                              // schemaIndex = 0
	data = binary.BigEndian.AppendUint32(data, uint32(len(metadata)))
	data = append(data, metadata...)

	r, n, err := ReadRecord(data, sigSize, hashBlockSize)
	if err != nil {
		t.Fatalf("ReadRecord() error: %v", err)
	}
	if n != len(data) {
		t.Errorf("consumed %d bytes, want %d", n, len(data))
	}
	if r.SchemaIndex != 0x00 {
		t.Errorf("SchemaIndex = 0x%02x, want 0x00", r.SchemaIndex)
	}
	if r.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}
	if len(r.Metadata) != len(metadata) {
		t.Errorf("Metadata length = %d, want %d", len(r.Metadata), len(metadata))
	}
}

func TestReadRecord_Errors(t *testing.T) {
	sigSize := 64
	hashBlockSize := 64

	tests := []struct {
		name    string
		data    []byte
		wantErr string
		wantEOF bool
	}{
		{
			name:    "empty data returns EOF",
			data:    []byte{},
			wantEOF: true,
		},
		{
			name:    "truncated prevSig",
			data:    append([]byte{RecordOpen}, make([]byte, 10)...),
			wantErr: "too short for prev_sig",
		},
		{
			name: "truncated openSig for non-open record",
			data: func() []byte {
				d := []byte{RecordCheckpoint}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = append(d, make([]byte, 10)...)      // partial openSig
				return d
			}(),
			wantErr: "too short for open_sig",
		},
		{
			name: "truncated payload size",
			data: func() []byte {
				d := []byte{RecordOpen}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = append(d, 0x00, 0x00)               // partial payloadSize
				return d
			}(),
			wantErr: "too short for payload_size",
		},
		{
			name: "truncated hash block",
			data: func() []byte {
				d := []byte{RecordOpen}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = binary.BigEndian.AppendUint64(d, 1) // payloadSize != 0
				d = append(d, make([]byte, 10)...)      // partial hash block
				return d
			}(),
			wantErr: "too short for hash_block",
		},
		{
			name: "truncated record signature",
			data: func() []byte {
				d := []byte{RecordOpen}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = binary.BigEndian.AppendUint64(d, 0) // payloadSize = 0
				d = append(d, make([]byte, 10)...)      // partial signature
				return d
			}(),
			wantErr: "too short for record signature",
		},
		{
			name: "truncated schema index",
			data: func() []byte {
				d := []byte{RecordOpen}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = binary.BigEndian.AppendUint64(d, 0) // payloadSize = 0
				d = append(d, make([]byte, sigSize)...) // signature
				return d                                // no schema index
			}(),
			wantErr: "too short for schema index",
		},
		{
			name: "truncated metadata length",
			data: func() []byte {
				d := []byte{RecordOpen}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = binary.BigEndian.AppendUint64(d, 0) // payloadSize = 0
				d = append(d, make([]byte, sigSize)...) // signature
				d = append(d, 0x00) // schemaIndex=0 (metadata)
				d = append(d, 0x00)                     // partial metadata length
				return d
			}(),
			wantErr: "too short for metadata length",
		},
		{
			name: "truncated metadata body",
			data: func() []byte {
				d := []byte{RecordOpen}
				d = append(d, make([]byte, sigSize)...) // prevSig
				d = binary.BigEndian.AppendUint64(d, 0) // payloadSize = 0
				d = append(d, make([]byte, sigSize)...) // signature
				d = append(d, 0x00)                     // schemaIndex = 0
				d = binary.BigEndian.AppendUint32(d, 50)
				d = append(d, make([]byte, 5)...) // only 5 of 50 bytes
				return d
			}(),
			wantErr: "too short for metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadRecord(tt.data, sigSize, hashBlockSize)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantEOF {
				if err != io.EOF {
					t.Errorf("error = %v, want io.EOF", err)
				}
				return
			}
			if !containsStr(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want %q",
					err.Error(), tt.wantErr)
			}
		})
	}
}

// ─── Verify (end-to-end with real signatures) ─────────────────────────

// buildSignedRecord builds a signed record for Verify end-to-end tests.
func buildSignedRecord(
	t *testing.T,
	priv ed25519.PrivateKey,
	recType byte,
	prevSig []byte,
	openSig []byte,
	payloadSize int64,
	hashBlock []byte,
	schemaIndex byte,
	metadata []byte,
	sigSize int,
) ([]byte, []byte) {
	t.Helper()

	// Build signature input
	var sigInput []byte
	sigInput = append(sigInput, recType)
	sigInput = append(sigInput, prevSig...)
	if recType != RecordOpen {
		sigInput = append(sigInput, openSig...)
	}
	sigInput = binary.BigEndian.AppendUint64(sigInput, uint64(payloadSize))
	if payloadSize != 0 {
		sigInput = append(sigInput, hashBlock...)
	}

	digest := sha512.Sum512(sigInput)
	sig := ed25519.Sign(priv, digest[:])

	// Build record bytes
	var rec []byte
	rec = append(rec, recType)
	rec = append(rec, prevSig...)
	if recType != RecordOpen {
		rec = append(rec, openSig...)
	}
	rec = binary.BigEndian.AppendUint64(rec, uint64(payloadSize))
	if payloadSize != 0 {
		rec = append(rec, hashBlock...)
	}
	rec = append(rec, sig...)
	rec = append(rec, schemaIndex)
	if schemaIndex != SchemaNoMetadata {
		rec = binary.BigEndian.AppendUint32(rec, uint32(len(metadata)))
		rec = append(rec, metadata...)
	}

	return rec, sig
}

func TestVerify_SingleOpenRecord(t *testing.T) {
	headerBytes, _, priv := buildTestHeader(t)

	// Parse header to get the header signature (which is the prevSig for first record)
	h, _, err := ReadHeader(headerBytes)
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}

	recBytes, _ := buildSignedRecord(t, priv,
		RecordOpen,
		h.Signature,
		nil, // no openSig for open records
		0,   // no payload
		nil, // no hash block
		SchemaNoMetadata,
		nil,
		h.SigSize,
	)

	var ledger []byte
	ledger = append(ledger, headerBytes...)
	ledger = append(ledger, recBytes...)

	result, err := Verify(ledger)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if !result.Valid {
		t.Errorf("Valid = false, want true (SigErrors=%d)", result.SigErrors)
	}
	if result.TotalRecords != 1 {
		t.Errorf("TotalRecords = %d, want 1", result.TotalRecords)
	}
	if result.SigErrors != 0 {
		t.Errorf("SigErrors = %d, want 0", result.SigErrors)
	}
	if len(result.Records) != 1 {
		t.Fatalf("Records length = %d, want 1", len(result.Records))
	}
	if result.Records[0].Type != RecordOpen {
		t.Errorf("Records[0].Type = 0x%02x, want 0x%02x",
			result.Records[0].Type, RecordOpen)
	}
}

func TestVerify_OpenAndCloseRecords(t *testing.T) {
	headerBytes, _, priv := buildTestHeader(t)

	h, _, err := ReadHeader(headerBytes)
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}

	// Open record
	openRecBytes, openSig := buildSignedRecord(t, priv,
		RecordOpen,
		h.Signature,
		nil,
		0,
		nil,
		SchemaNoMetadata,
		nil,
		h.SigSize,
	)

	// Close record chained after open
	closeRecBytes, _ := buildSignedRecord(t, priv,
		RecordClose,
		openSig,
		openSig, // openSig references the open record
		0,
		nil,
		SchemaNoMetadata,
		nil,
		h.SigSize,
	)

	var ledgerData []byte
	ledgerData = append(ledgerData, headerBytes...)
	ledgerData = append(ledgerData, openRecBytes...)
	ledgerData = append(ledgerData, closeRecBytes...)

	result, err := Verify(ledgerData)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if !result.Valid {
		t.Errorf("Valid = false, want true (SigErrors=%d)", result.SigErrors)
	}
	if result.TotalRecords != 2 {
		t.Errorf("TotalRecords = %d, want 2", result.TotalRecords)
	}
	if result.Unclosed != 0 {
		t.Errorf("Unclosed = %d, want 0", result.Unclosed)
	}
}

func TestVerify_UnclosedChannel(t *testing.T) {
	headerBytes, _, priv := buildTestHeader(t)

	h, _, err := ReadHeader(headerBytes)
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}

	// Just an open record, never closed
	openRecBytes, _ := buildSignedRecord(t, priv,
		RecordOpen,
		h.Signature,
		nil,
		0,
		nil,
		SchemaNoMetadata,
		nil,
		h.SigSize,
	)

	var ledgerData []byte
	ledgerData = append(ledgerData, headerBytes...)
	ledgerData = append(ledgerData, openRecBytes...)

	result, err := Verify(ledgerData)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if result.Unclosed != 1 {
		t.Errorf("Unclosed = %d, want 1", result.Unclosed)
	}
}

func TestVerify_DetectsBrokenChain(t *testing.T) {
	headerBytes, _, priv := buildTestHeader(t)

	h, _, err := ReadHeader(headerBytes)
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}

	// Build an open record with a wrong prevSig (broken chain)
	wrongPrevSig := make([]byte, h.SigSize)
	wrongPrevSig[0] = 0xFF // guaranteed different from real header sig

	recBytes, _ := buildSignedRecord(t, priv,
		RecordOpen,
		wrongPrevSig,
		nil,
		0,
		nil,
		SchemaNoMetadata,
		nil,
		h.SigSize,
	)

	var ledgerData []byte
	ledgerData = append(ledgerData, headerBytes...)
	ledgerData = append(ledgerData, recBytes...)

	result, err := Verify(ledgerData)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// Should have a chain error (prevSig mismatch)
	if result.SigErrors == 0 {
		t.Error("SigErrors = 0, want > 0 for broken chain")
	}
	if result.Valid {
		t.Error("Valid = true, want false for broken chain")
	}
}

func TestVerify_HeaderOnly(t *testing.T) {
	headerBytes, _, _ := buildTestHeader(t)

	result, err := Verify(headerBytes)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if result.TotalRecords != 0 {
		t.Errorf("TotalRecords = %d, want 0", result.TotalRecords)
	}
	if !result.Valid {
		t.Errorf("Valid = false, want true for valid header-only ledger")
	}
}

func TestVerify_InvalidHeader(t *testing.T) {
	_, err := Verify([]byte("garbage"))
	if err == nil {
		t.Fatal("expected error for invalid header, got nil")
	}
}

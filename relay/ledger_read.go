package relay

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// Header holds the parsed header of a binary ledger.
type Header struct {
	Version       byte
	SigScheme     string
	SigSize       int
	HashBlockSize int
	PubKeyLen     int
	PubKey        []byte
	Signature     []byte
	Meta          HeaderMeta
	PrefixBytes   []byte // raw binary prefix (for signature verification)
}

// Record holds a parsed record from a binary ledger.
type Record struct {
	Type        byte
	PrevSig     []byte
	OpenSig     []byte // nil for open records
	PayloadSize int64
	HashBlock   []byte // nil when PayloadSize == 0
	Signature   []byte
	SchemaIndex byte
	Metadata    []byte // raw CBOR bytes; nil when SchemaIndex == 0xFF
}

// Direction returns "in", "out", or "" based on the payload size sign.
func (r *Record) Direction() string {
	if r.PayloadSize > 0 {
		return "in"
	}
	if r.PayloadSize < 0 {
		return "out"
	}
	return ""
}

// AbsPayloadSize returns the absolute value of the payload size.
func (r *Record) AbsPayloadSize() int64 {
	if r.PayloadSize < 0 {
		return -r.PayloadSize
	}
	return r.PayloadSize
}

// TypeName returns the human-readable record type name.
func (r *Record) TypeName() string {
	switch r.Type {
	case RecordOpen:
		return "open"
	case RecordCheckpoint:
		return "checkpoint"
	case RecordClose:
		return "close"
	case RecordArtifact:
		return "artifact"
	default:
		return fmt.Sprintf("unknown(0x%02x)", r.Type)
	}
}

// VerifyResult holds the outcome of verifying a ledger.
type VerifyResult struct {
	Header       Header
	Records      []Record
	Valid        bool
	SigErrors    int
	Unclosed     int
	TotalRecords int
}

// ReadHeader parses the header from a binary ledger.
// Returns the header and the number of bytes consumed.
func ReadHeader(data []byte) (*Header, int, error) {
	if len(data) < 5 {
		return nil, 0, errors.New("data too short for magic+version")
	}
	if string(data[0:4]) != "BLDL" {
		return nil, 0, errors.New("invalid magic bytes")
	}

	h := &Header{Version: data[4]}
	off := 5

	// Signature scheme (null-terminated)
	nullIdx := bytes.IndexByte(data[off:], 0x00)
	if nullIdx < 0 {
		return nil, 0, errors.New("no null terminator for signature scheme")
	}
	h.SigScheme = string(data[off : off+nullIdx])
	off += nullIdx + 1

	// Sizes
	if off+6 > len(data) {
		return nil, 0, errors.New("data too short for size fields")
	}
	h.SigSize = int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	h.HashBlockSize = int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	h.PubKeyLen = int(binary.BigEndian.Uint16(data[off:]))
	off += 2

	// Public key
	if off+h.PubKeyLen > len(data) {
		return nil, 0, errors.New("data too short for public key")
	}
	h.PubKey = make([]byte, h.PubKeyLen)
	copy(h.PubKey, data[off:off+h.PubKeyLen])
	off += h.PubKeyLen

	h.PrefixBytes = data[:off]

	// Header signature
	if off+h.SigSize > len(data) {
		return nil, 0, errors.New("data too short for header signature")
	}
	h.Signature = make([]byte, h.SigSize)
	copy(h.Signature, data[off:off+h.SigSize])
	off += h.SigSize

	// CBOR metadata
	if off+4 > len(data) {
		return nil, 0, errors.New("data too short for metadata length")
	}
	metaLen := int(binary.BigEndian.Uint32(data[off:]))
	off += 4
	if off+metaLen > len(data) {
		return nil, 0, errors.New("data too short for metadata")
	}
	if err := cbor.Unmarshal(data[off:off+metaLen], &h.Meta); err != nil {
		return nil, 0, fmt.Errorf("decode header metadata: %w", err)
	}
	off += metaLen

	return h, off, nil
}

// ReadRecord parses a single record from data at the given offset.
// Returns the record and the number of bytes consumed.
func ReadRecord(data []byte, sigSize, hashBlockSize int) (*Record, int, error) {
	if len(data) < 1 {
		return nil, 0, io.EOF
	}

	r := &Record{Type: data[0]}
	off := 1

	// Previous signature
	if off+sigSize > len(data) {
		return nil, 0, errors.New("data too short for prev_sig")
	}
	r.PrevSig = make([]byte, sigSize)
	copy(r.PrevSig, data[off:off+sigSize])
	off += sigSize

	// Open signature (not present for open records)
	if r.Type != RecordOpen {
		if off+sigSize > len(data) {
			return nil, 0, errors.New("data too short for open_sig")
		}
		r.OpenSig = make([]byte, sigSize)
		copy(r.OpenSig, data[off:off+sigSize])
		off += sigSize
	}

	// Payload size
	if off+8 > len(data) {
		return nil, 0, errors.New("data too short for payload_size")
	}
	r.PayloadSize = int64(binary.BigEndian.Uint64(data[off:]))
	off += 8

	// Hash block (only when payload != 0)
	if r.PayloadSize != 0 {
		if off+hashBlockSize > len(data) {
			return nil, 0, errors.New("data too short for hash_block")
		}
		r.HashBlock = make([]byte, hashBlockSize)
		copy(r.HashBlock, data[off:off+hashBlockSize])
		off += hashBlockSize
	}

	// Record signature
	if off+sigSize > len(data) {
		return nil, 0, errors.New("data too short for record signature")
	}
	r.Signature = make([]byte, sigSize)
	copy(r.Signature, data[off:off+sigSize])
	off += sigSize

	// Schema index
	if off >= len(data) {
		return nil, 0, errors.New("data too short for schema index")
	}
	r.SchemaIndex = data[off]
	off++

	// Metadata (if present)
	if r.SchemaIndex != SchemaNoMetadata {
		if off+4 > len(data) {
			return nil, 0, errors.New("data too short for metadata length")
		}
		metaLen := int(binary.BigEndian.Uint32(data[off:]))
		off += 4
		if off+metaLen > len(data) {
			return nil, 0, errors.New("data too short for metadata")
		}
		r.Metadata = make([]byte, metaLen)
		copy(r.Metadata, data[off:off+metaLen])
		off += metaLen
	}

	return r, off, nil
}

// Verify parses and verifies an entire binary ledger.
func Verify(data []byte) (*VerifyResult, error) {
	header, off, err := ReadHeader(data)
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	result := &VerifyResult{Header: *header}

	// Verify header signature
	if !verifyEd25519Sig(header.PubKey, header.PrefixBytes, header.Signature) {
		result.SigErrors++
	}

	prevSig := header.Signature
	openChannels := make(map[string]bool) // track open channels by hex(open_sig)

	for off < len(data) {
		rec, n, err := ReadRecord(data[off:], header.SigSize, header.HashBlockSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read record %d: %w", result.TotalRecords, err)
		}
		off += n
		result.TotalRecords++

		// Verify prev_sig matches chain
		if !bytes.Equal(rec.PrevSig, prevSig) {
			result.SigErrors++
		}

		// Reconstruct signature input and verify
		sigInput := buildRecordSigInput(rec)
		if !verifyEd25519Sig(header.PubKey, sigInput, rec.Signature) {
			result.SigErrors++
		}

		// Track channel state
		sigKey := string(rec.Signature)
		switch rec.Type {
		case RecordOpen:
			openChannels[sigKey] = true
		case RecordClose, RecordArtifact:
			if rec.OpenSig != nil {
				delete(openChannels, string(rec.OpenSig))
			}
		}

		result.Records = append(result.Records, *rec)
		prevSig = rec.Signature
	}

	result.Unclosed = len(openChannels)
	result.Valid = result.SigErrors == 0

	return result, nil
}

// IsValidLedger checks if data begins with valid ledger magic bytes.
func IsValidLedger(data []byte) bool {
	return len(data) >= 5 && string(data[0:4]) == "BLDL" && data[4] == 0x01
}

func buildRecordSigInput(r *Record) []byte {
	var buf []byte
	buf = append(buf, r.Type)
	buf = append(buf, r.PrevSig...)
	if r.Type != RecordOpen {
		buf = append(buf, r.OpenSig...)
	}
	buf = binary.BigEndian.AppendUint64(buf, uint64(r.PayloadSize))
	if r.PayloadSize != 0 {
		buf = append(buf, r.HashBlock...)
	}
	return buf
}

func verifyEd25519Sig(pubKey, input, sig []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	digest := sha512.Sum512(input)
	return ed25519.Verify(ed25519.PublicKey(pubKey), digest[:], sig)
}

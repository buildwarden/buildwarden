package relay

import (
	"crypto/ed25519"
	crypto_md5 "crypto/md5"
	"crypto/rand"
	crypto_sha1 "crypto/sha1"
	crypto_sha256 "crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"log"
	"sync/atomic"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// Record type byte values per the BuildWarden Ledger Specification.
const (
	RecordOpen       byte = 0x01
	RecordCheckpoint byte = 0x02
	RecordClose      byte = 0x03
	RecordArtifact   byte = 0x04
)

// SchemaNoMetadata indicates no metadata is attached to a record.
const SchemaNoMetadata byte = 0xFF

// Ledger implements the BuildWarden binary ledger specification.
// All writes are serialized through a single channel.
type Ledger struct {
	key           ed25519.PrivateKey
	writer        io.Writer
	entries       chan entryRequest
	done          chan struct{}
	seq           atomic.Int64
	hashes        []string
	sigSize       int
	hashBlockSize int
	prevSigRaw    []byte
	sigBuf        []byte // reusable buffer for signature input
}

type entryRequest struct {
	entry  entryInput
	result chan<- []byte // nil for fire-and-forget; returns raw signature for open
}

type entryInput struct {
	Type        byte
	OpenSig     []byte // for checkpoint/close/artifact
	PayloadSize int64  // signed: negative=out, positive=in, 0=none
	HashBlock   []byte // raw concatenated hashes (nil when payload=0)
	SchemaIndex byte
	Metadata    []byte // pre-encoded CBOR (nil when SchemaIndex=0xFF)
}

// HeaderMeta is the CBOR metadata written after the header signature.
type HeaderMeta struct {
	Hashes      []string       `cbor:"hashes"`
	Schemas     []string       `cbor:"schemas"`
	Environment map[string]any `cbor:"environment"`
}

// LedgerConfig holds parameters for creating a new ledger.
type LedgerConfig struct {
	Writer      io.Writer
	Environment map[string]any
	Hashes      []string   // hash algorithm names in order
	Schemas     []string   // schema URLs in order
}

var defaultHashes = []string{"blake2b_256", "sha256", "sha1", "md5"}

var defaultSchemas = []string{
	"https://github.com/buildwarden/buildwarden/schemas/http-open.json",
	"https://github.com/buildwarden/buildwarden/schemas/http-headers.json",
	"https://github.com/buildwarden/buildwarden/schemas/http-body.json",
	"https://github.com/buildwarden/buildwarden/schemas/artifact.json",
	"https://github.com/buildwarden/buildwarden/schemas/redacted.json",
}

// NewLedger creates a new binary ledger, writes the header, and starts the
// serialization loop. The returned Ledger is ready for Open/Checkpoint/Close/Artifact calls.
func NewLedger(cfg LedgerConfig) (*Ledger, error) {
	if cfg.Hashes == nil {
		cfg.Hashes = defaultHashes
	}
	if cfg.Schemas == nil {
		cfg.Schemas = defaultSchemas
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	hashBlockSize := 0
	for _, name := range cfg.Hashes {
		hashBlockSize += hashOutputSize(name)
	}

	l := &Ledger{
		key:           priv,
		writer:        cfg.Writer,
		entries:       make(chan entryRequest, 256),
		done:          make(chan struct{}),
		hashes:        cfg.Hashes,
		sigSize:       ed25519.SignatureSize, // 64
		hashBlockSize: hashBlockSize,
		sigBuf:        make([]byte, 0, 512),
	}

	if err := l.writeHeader(cfg); err != nil {
		return nil, err
	}

	go l.loop()
	return l, nil
}

// PublicKey returns the raw Ed25519 public key bytes.
func (l *Ledger) PublicKey() ed25519.PublicKey {
	pub, ok := l.key.Public().(ed25519.PublicKey)
	if !ok {
		panic("unexpected key type")
	}
	return pub
}

// Open writes an open record synchronously and returns the record's raw signature
// (the channel identifier).
func (l *Ledger) Open(schemaIndex byte, metadata []byte) []byte {
	result := make(chan []byte, 1)
	l.entries <- entryRequest{
		entry: entryInput{
			Type:        RecordOpen,
			PayloadSize: 0,
			SchemaIndex: schemaIndex,
			Metadata:    metadata,
		},
		result: result,
	}
	return <-result
}

// Checkpoint writes a checkpoint record (fire-and-forget).
func (l *Ledger) Checkpoint(
	openSig []byte, payloadSize int64, hashBlock []byte,
	schemaIndex byte, metadata []byte,
) {
	l.entries <- entryRequest{
		entry: entryInput{
			Type:        RecordCheckpoint,
			OpenSig:     openSig,
			PayloadSize: payloadSize,
			HashBlock:   hashBlock,
			SchemaIndex: schemaIndex,
			Metadata:    metadata,
		},
	}
}

// Close writes a close record (fire-and-forget).
func (l *Ledger) Close(
	openSig []byte, payloadSize int64, hashBlock []byte,
	schemaIndex byte, metadata []byte,
) {
	l.entries <- entryRequest{
		entry: entryInput{
			Type:        RecordClose,
			OpenSig:     openSig,
			PayloadSize: payloadSize,
			HashBlock:   hashBlock,
			SchemaIndex: schemaIndex,
			Metadata:    metadata,
		},
	}
}

// Artifact writes an artifact record (fire-and-forget). Closes the channel.
func (l *Ledger) Artifact(
	openSig []byte, payloadSize int64, hashBlock []byte,
	schemaIndex byte, metadata []byte,
) {
	l.entries <- entryRequest{
		entry: entryInput{
			Type:        RecordArtifact,
			OpenSig:     openSig,
			PayloadSize: payloadSize,
			HashBlock:   hashBlock,
			SchemaIndex: schemaIndex,
			Metadata:    metadata,
		},
	}
}

// Finish drains the entry channel and waits for the loop to exit.
func (l *Ledger) Finish() {
	close(l.entries)
	<-l.done
}

// ComputeHashBlock computes the concatenated hash block for the given data.
func (l *Ledger) ComputeHashBlock(data []byte) []byte {
	block := make([]byte, 0, l.hashBlockSize)
	for _, name := range l.hashes {
		h := newHash(name)
		h.Write(data)
		block = h.Sum(block)
	}
	return block
}

// --- internal ---

func (l *Ledger) writeHeader(cfg LedgerConfig) error {
	pub := l.PublicKey()

	// Build binary prefix
	var prefix []byte
	// Magic + version
	prefix = append(prefix, 'B', 'L', 'D', 'L', 0x01)
	// Signature scheme (null-terminated)
	prefix = append(prefix, []byte("ed25519-sha512")...)
	prefix = append(prefix, 0x00)
	// Signature size (uint16 big-endian)
	prefix = binary.BigEndian.AppendUint16(prefix, uint16(l.sigSize))
	// Hash block size (uint16 big-endian)
	prefix = binary.BigEndian.AppendUint16(prefix, uint16(l.hashBlockSize))
	// Public key length (uint16 big-endian)
	prefix = binary.BigEndian.AppendUint16(prefix, uint16(len(pub)))
	// Public key bytes
	prefix = append(prefix, pub...)

	// Sign the binary prefix
	sig := l.sign(prefix)
	l.prevSigRaw = sig

	// Encode header CBOR metadata
	meta := HeaderMeta{
		Hashes:      cfg.Hashes,
		Schemas:     cfg.Schemas,
		Environment: cfg.Environment,
	}
	metaBytes, err := cbor.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode header metadata: %w", err)
	}

	// Write: prefix + signature + metadata-length + metadata
	var buf []byte
	buf = append(buf, prefix...)
	buf = append(buf, sig...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(metaBytes)))
	buf = append(buf, metaBytes...)

	if _, err := l.writer.Write(buf); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	return nil
}

func (l *Ledger) loop() {
	defer close(l.done)

	for req := range l.entries {
		e := req.entry
		l.seq.Add(1)

		// Build signature input
		l.sigBuf = l.sigBuf[:0]
		l.sigBuf = append(l.sigBuf, e.Type)
		l.sigBuf = append(l.sigBuf, l.prevSigRaw...)

		if e.Type != RecordOpen {
			l.sigBuf = append(l.sigBuf, e.OpenSig...)
		}

		l.sigBuf = binary.BigEndian.AppendUint64(l.sigBuf, uint64(e.PayloadSize))

		if e.PayloadSize != 0 {
			l.sigBuf = append(l.sigBuf, e.HashBlock...)
		}

		sig := l.sign(l.sigBuf)

		// Write record bytes
		var rec []byte
		rec = append(rec, e.Type)
		rec = append(rec, l.prevSigRaw...)
		if e.Type != RecordOpen {
			rec = append(rec, e.OpenSig...)
		}
		rec = binary.BigEndian.AppendUint64(rec, uint64(e.PayloadSize))
		if e.PayloadSize != 0 {
			rec = append(rec, e.HashBlock...)
		}
		rec = append(rec, sig...)
		rec = append(rec, e.SchemaIndex)

		if e.SchemaIndex != SchemaNoMetadata {
			rec = binary.BigEndian.AppendUint32(rec, uint32(len(e.Metadata)))
			rec = append(rec, e.Metadata...)
		}

		if _, err := l.writer.Write(rec); err != nil {
			log.Printf("error writing ledger record: %v", err)
		}

		l.prevSigRaw = sig
		if req.result != nil {
			req.result <- sig
		}
	}
}

func (l *Ledger) sign(input []byte) []byte {
	digest := sha512.Sum512(input)
	return ed25519.Sign(l.key, digest[:])
}

func hashOutputSize(name string) int {
	switch name {
	case "blake2b_256":
		return blake2b.Size256
	case "sha256":
		return 32
	case "sha1":
		return 20
	case "md5":
		return 16
	default:
		panic("unsupported hash: " + name)
	}
}

func newHash(name string) hash.Hash {
	switch name {
	case "blake2b_256":
		h, _ := blake2b.New256(nil)
		return h
	case "sha256":
		return crypto_sha256.New()
	case "sha1":
		return crypto_sha1.New()
	case "md5":
		return crypto_md5.New()
	default:
		panic("unsupported hash: " + name)
	}
}

// HashData computes individual hashes for data, returning them in order.
// Useful for callers that need both the hash block and individual hex values.
func HashData(data []byte, hashes []string) [][]byte {
	result := make([][]byte, len(hashes))
	for i, name := range hashes {
		h := newHash(name)
		h.Write(data)
		result[i] = h.Sum(nil)
	}
	return result
}

// StreamingHasher computes all configured hashes incrementally.
type StreamingHasher struct {
	hashes []hash.Hash
	size   int64
}

// NewStreamingHasher creates a hasher for the given algorithm names.
func NewStreamingHasher(names []string) *StreamingHasher {
	h := &StreamingHasher{hashes: make([]hash.Hash, len(names))}
	for i, name := range names {
		h.hashes[i] = newHash(name)
	}
	return h
}

func (s *StreamingHasher) Write(p []byte) (int, error) {
	for _, h := range s.hashes {
		h.Write(p)
	}
	s.size += int64(len(p))
	return len(p), nil
}

// Finish returns the concatenated hash block and total bytes written.
func (s *StreamingHasher) Finish() (hashBlock []byte, size int64) {
	for _, h := range s.hashes {
		hashBlock = h.Sum(hashBlock)
	}
	return hashBlock, s.size
}

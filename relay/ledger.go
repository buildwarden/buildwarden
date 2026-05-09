package relay

import (
	"crypto/ed25519"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash"
	"io"
	"log"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/blake2b"
)

// Ledger implements the BuildWarden ledger v2 specification.
// All writes are serialized through a single channel.
type Ledger struct {
	key         ed25519.PrivateKey
	writer      io.Writer
	enc         *json.Encoder
	entries     chan entryRequest
	done        chan struct{}
	seq         atomic.Int64
	hashes      []string
	headerSig   string
	prevSigRaw  []byte // cached decoded bytes of the previous signature
	sigBuf      []byte // reusable buffer for signature input construction
}

// entryRequest is the union type sent through the single channel.
type entryRequest struct {
	entry  entryInput
	// nil for fire-and-forget (checkpoint/close), non-nil for synchronous open
	result chan<- string
}

type entryInput struct {
	Type          string
	OpenSignature string // for checkpoint/close
	Direction     string // for checkpoint/close
	Size          int64  // payload size
	PreHashed     map[string]string // pre-computed hashes
	Metadata      map[string]any
}

// LedgerEntry is the JSON structure written to the ledger file.
type LedgerEntry struct {
	EntryType     string         `json:"entry_type"`
	OpenSignature string         `json:"open_signature,omitempty"`
	Direction     string         `json:"direction,omitempty"`
	Payload       *PayloadRecord `json:"payload,omitempty"`
	Signature     string         `json:"signature"`
	Seq           int64          `json:"seq"`
	Timestamp     string         `json:"timestamp"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// HeaderEntry is the JSON structure for the ledger header.
type HeaderEntry struct {
	EntryType       string         `json:"entry_type"`
	Version         string         `json:"version"`
	Format          string         `json:"format"`
	SignatureScheme string         `json:"signature_scheme"`
	Hashes          []string       `json:"hashes"`
	Environment     map[string]any `json:"environment"`
	Payload         *PayloadRecord `json:"payload"`
	Signature       string         `json:"signature"`
}

// PayloadRecord holds the content identity of a payload.
type PayloadRecord struct {
	Size   int64             `json:"size"`
	Hashes map[string]string `json:"hashes"`
}

var ledger *Ledger

var defaultHashes = []string{"blake2b_256", "sha256", "sha1", "md5"}

func NewLedger(w io.Writer, environment map[string]any) error {
	l := &Ledger{
		writer:  w,
		enc:     json.NewEncoder(w),
		entries: make(chan entryRequest, 256),
		done:    make(chan struct{}),
		hashes:  defaultHashes,
		sigBuf:  make([]byte, 0, 512),
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("error generating private key for ledger: %w", err)
	}
	l.key = priv

	if err := l.writeHeader(environment); err != nil {
		return fmt.Errorf("error writing ledger header: %w", err)
	}

	go l.loop()
	ledger = l
	return nil
}

func FinishLedger() {
	if ledger != nil {
		close(ledger.entries)
		<-ledger.done
	}
}

// PublicCertPEM returns the public key in PEM format.
func PublicCertPEM() []byte {
	if ledger == nil {
		return nil
	}
	pub, _ := ledger.key.Public().(ed25519.PublicKey)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: []byte(pub),
	})
}

// Open records an open entry synchronously and returns the entry's signature
// (which serves as the channel identifier for subsequent checkpoint/close entries).
func (l *Ledger) Open(metadata map[string]any) string {
	result := make(chan string, 1)
	l.entries <- entryRequest{
		entry: entryInput{
			Type:     "open",
			Metadata: metadata,
		},
		result: result,
	}
	return <-result
}

// Checkpoint records a checkpoint entry (fire-and-forget).
func (l *Ledger) Checkpoint(
	openSig string, direction string, payload []byte, metadata map[string]any,
) {
	pr := l.computePayload(payload)
	l.entries <- entryRequest{
		entry: entryInput{
			Type:          "checkpoint",
			OpenSignature: openSig,
			Direction:     direction,
			Size:          pr.Size,
			PreHashed:     pr.Hashes,
			Metadata:      metadata,
		},
	}
}

// CheckpointHashed records a checkpoint entry with pre-computed hashes (fire-and-forget).
func (l *Ledger) CheckpointHashed(
	openSig string, direction string, size int64, hashes map[string]string,
) {
	l.entries <- entryRequest{
		entry: entryInput{
			Type:          "checkpoint",
			OpenSignature: openSig,
			Direction:     direction,
			Size:          size,
			PreHashed:     hashes,
		},
	}
}

// Close records a close entry (fire-and-forget).
func (l *Ledger) Close(openSig string, direction string, payload []byte, metadata map[string]any) {
	pr := l.computePayload(payload)
	l.entries <- entryRequest{
		entry: entryInput{
			Type:          "close",
			OpenSignature: openSig,
			Direction:     direction,
			Size:          pr.Size,
			PreHashed:     pr.Hashes,
			Metadata:      metadata,
		},
	}
}

// CloseHashed records a close entry with pre-computed hashes (fire-and-forget).
func (l *Ledger) CloseHashed(
	openSig string, direction string, size int64,
	hashes map[string]string, metadata map[string]any,
) {
	l.entries <- entryRequest{
		entry: entryInput{
			Type:          "close",
			OpenSignature: openSig,
			Direction:     direction,
			Size:          size,
			PreHashed:     hashes,
			Metadata:      metadata,
		},
	}
}

func (l *Ledger) writeHeader(environment map[string]any) error {
	pub, _ := l.key.Public().(ed25519.PublicKey)
	certBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: []byte(pub),
	})

	payload := l.computePayload(certBytes)

	// Signature input for header: entry_type + size + hashes (no prev_sig)
	var sigInput []byte
	sigInput = append(sigInput, []byte("header")...)
	sigInput = append(sigInput, sizeBytes(payload.Size)...)
	sigInput = append(sigInput, l.rawHashBytes(payload.Hashes)...)

	sig, sigRaw, err := l.sign(sigInput)
	if err != nil {
		return err
	}

	l.seq.Store(0)
	l.prevSigRaw = sigRaw

	header := HeaderEntry{
		EntryType:       "header",
		Version:         "2.0",
		Format:          "json",
		SignatureScheme: "ed25519-sha512",
		Hashes:          l.hashes,
		Environment:     environment,
		Payload:         payload,
		Signature:       sig,
	}

	j, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("error marshalling header: %w", err)
	}
	if _, err := l.writer.Write(append(j, '\n')); err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	l.headerSig = sig
	return nil
}

func (l *Ledger) loop() {
	defer close(l.done)

	for req := range l.entries {
		e := req.entry
		seq := l.seq.Add(1)

		// Reuse sigBuf: reset length, keep capacity
		l.sigBuf = l.sigBuf[:0]
		l.sigBuf = append(l.sigBuf, l.prevSigRaw...)

		switch e.Type {
		case "open":
			l.sigBuf = append(l.sigBuf, "open"...)

		case "checkpoint", "close":
			openSigBytes, _ := base64.StdEncoding.DecodeString(e.OpenSignature)
			l.sigBuf = append(l.sigBuf, openSigBytes...)
			l.sigBuf = append(l.sigBuf, e.Type...)
			l.sigBuf = append(l.sigBuf, e.Direction...)

			payload := &PayloadRecord{Size: e.Size, Hashes: e.PreHashed}
			l.sigBuf = appendSizeBytes(l.sigBuf, payload.Size)
			l.sigBuf = l.appendRawHashBytes(l.sigBuf, payload.Hashes)

			sig, sigRaw, err := l.sign(l.sigBuf)
			if err != nil {
				log.Printf("error signing %s entry: %v", e.Type, err)
				if req.result != nil {
					req.result <- ""
				}
				continue
			}

			l.writeEntry(LedgerEntry{
				EntryType:     e.Type,
				OpenSignature: e.OpenSignature,
				Direction:     e.Direction,
				Payload:       payload,
				Signature:     sig,
				Seq:           seq,
				Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
				Metadata:      e.Metadata,
			})
			l.prevSigRaw = sigRaw
			if req.result != nil {
				req.result <- sig
			}
			continue
		}

		// Open entry path (no payload)
		sig, sigRaw, err := l.sign(l.sigBuf)
		if err != nil {
			log.Printf("error signing open entry: %v", err)
			if req.result != nil {
				req.result <- ""
			}
			continue
		}

		l.writeEntry(LedgerEntry{
			EntryType: "open",
			Signature: sig,
			Seq:       seq,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Metadata:  e.Metadata,
		})
		l.prevSigRaw = sigRaw
		if req.result != nil {
			req.result <- sig
		}
	}
}

func (l *Ledger) writeEntry(entry LedgerEntry) {
	if err := l.enc.Encode(entry); err != nil {
		log.Printf("error writing ledger entry: %v", err)
	}
}

func (l *Ledger) sign(input []byte) (string, []byte, error) {
	digest := sha512.Sum512(input)
	raw := ed25519.Sign(l.key, digest[:])
	return base64.StdEncoding.EncodeToString(raw), raw, nil
}

func (l *Ledger) computePayload(data []byte) *PayloadRecord {
	hashes := make(map[string]string, len(l.hashes))
	for _, name := range l.hashes {
		h := newHash(name)
		h.Write(data)
		hashes[name] = hex.EncodeToString(h.Sum(nil))
	}
	return &PayloadRecord{
		Size:   int64(len(data)),
		Hashes: hashes,
	}
}

func (l *Ledger) rawHashBytes(hashes map[string]string) []byte {
	var out []byte
	for _, name := range l.hashes {
		b, _ := hex.DecodeString(hashes[name])
		out = append(out, b...)
	}
	return out
}

func (l *Ledger) appendRawHashBytes(dst []byte, hashes map[string]string) []byte {
	for _, name := range l.hashes {
		b, _ := hex.DecodeString(hashes[name])
		dst = append(dst, b...)
	}
	return dst
}

func sizeBytes(size int64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(size))
	return buf
}

func appendSizeBytes(dst []byte, size int64) []byte {
	return binary.LittleEndian.AppendUint64(dst, uint64(size))
}

func newHash(name string) hash.Hash {
	switch name {
	case "blake2b_256":
		h, _ := blake2b.New256(nil)
		return h
	case "sha256":
		return sha256.New()
	case "sha1":
		return sha1.New()
	case "md5":
		return md5.New()
	default:
		panic("unsupported hash: " + name)
	}
}

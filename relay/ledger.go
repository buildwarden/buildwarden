package relay

import (
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
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
	key       *rsa.PrivateKey
	writer    io.Writer
	entries   chan entryRequest
	done      chan struct{}
	seq       atomic.Int64
	hashes    []string
	headerSig string
}

// entryRequest is the union type sent through the single channel.
type entryRequest struct {
	entry  entryInput
	result chan<- string // nil for fire-and-forget (checkpoint/close), non-nil for synchronous open
}

type entryInput struct {
	Type          string
	OpenSignature string // for checkpoint/close
	Direction     string // for checkpoint/close
	Payload       []byte // raw payload bytes (nil for open or pre-hashed)
	Size          int64  // payload size (used with PreHashed)
	PreHashed     map[string]string // pre-computed hashes (nil means compute from Payload)
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
	EntryType   string         `json:"entry_type"`
	Version     string         `json:"version"`
	Format      string         `json:"format"`
	Hashes      []string       `json:"hashes"`
	Environment map[string]any `json:"environment"`
	Payload     *PayloadRecord `json:"payload"`
	Signature   string         `json:"signature"`
}

// PayloadRecord holds the content identity of a payload.
type PayloadRecord struct {
	Size   int64             `json:"size"`
	Hashes map[string]string `json:"hashes"`
}

var ledger *Ledger

var defaultHashes = []string{"blake2b_256", "sha256", "sha1", "md5"}

func NewLedger(w io.Writer, environment map[string]any) error {
	var err error
	l := &Ledger{
		writer:  w,
		entries: make(chan entryRequest, 256),
		done:    make(chan struct{}),
		hashes:  defaultHashes,
	}

	l.key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("error generating private key for ledger: %w", err)
	}

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

// PublicCertPEM returns the public certificate in PEM format.
func PublicCertPEM() []byte {
	if ledger == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&ledger.key.PublicKey),
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
func (l *Ledger) Checkpoint(openSig string, direction string, payload []byte, metadata map[string]any) {
	l.entries <- entryRequest{
		entry: entryInput{
			Type:          "checkpoint",
			OpenSignature: openSig,
			Direction:     direction,
			Payload:       payload,
			Metadata:      metadata,
		},
	}
}

// CheckpointHashed records a checkpoint entry with pre-computed hashes (fire-and-forget).
func (l *Ledger) CheckpointHashed(openSig string, direction string, size int64, hashes map[string]string) {
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
	l.entries <- entryRequest{
		entry: entryInput{
			Type:          "close",
			OpenSignature: openSig,
			Direction:     direction,
			Payload:       payload,
			Metadata:      metadata,
		},
	}
}

// CloseHashed records a close entry with pre-computed hashes (fire-and-forget).
func (l *Ledger) CloseHashed(openSig string, direction string, size int64, hashes map[string]string, metadata map[string]any) {
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
	certBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&l.key.PublicKey),
	})

	payload := l.computePayload(certBytes)

	// Signature input for header: entry_type + size + hashes (no prev_sig)
	var sigInput []byte
	sigInput = append(sigInput, []byte("header")...)
	sigInput = append(sigInput, sizeBytes(payload.Size)...)
	sigInput = append(sigInput, l.rawHashBytes(payload.Hashes)...)

	sig, err := l.sign(sigInput)
	if err != nil {
		return err
	}

	// Store the initial prev_sig for the loop
	l.seq.Store(0)

	header := HeaderEntry{
		EntryType:   "header",
		Version:     "2.0",
		Format:      "json",
		Hashes:      l.hashes,
		Environment: environment,
		Payload:     payload,
		Signature:   sig,
	}

	j, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("error marshalling header: %w", err)
	}
	if _, err := l.writer.Write(append(j, '\n')); err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	// Store the header signature to bootstrap the chain in loop().
	l.headerSig = sig
	return nil
}

func (l *Ledger) loop() {
	prevSig := l.headerSig
	defer close(l.done)

	for req := range l.entries {
		e := req.entry
		seq := l.seq.Add(1)

		var sigInput []byte
		prevSigBytes, _ := base64.StdEncoding.DecodeString(prevSig)
		sigInput = append(sigInput, prevSigBytes...)

		switch e.Type {
		case "open":
			sigInput = append(sigInput, []byte("open")...)

		case "checkpoint", "close":
			openSigBytes, _ := base64.StdEncoding.DecodeString(e.OpenSignature)
			sigInput = append(sigInput, openSigBytes...)
			sigInput = append(sigInput, []byte(e.Type)...)
			sigInput = append(sigInput, []byte(e.Direction)...)

			var payload *PayloadRecord
			if e.PreHashed != nil {
				payload = &PayloadRecord{Size: e.Size, Hashes: e.PreHashed}
			} else {
				payload = l.computePayload(e.Payload)
			}
			sigInput = append(sigInput, sizeBytes(payload.Size)...)
			sigInput = append(sigInput, l.rawHashBytes(payload.Hashes)...)

			sig, err := l.sign(sigInput)
			if err != nil {
				log.Printf("error signing %s entry: %v", e.Type, err)
				if req.result != nil {
					req.result <- ""
				}
				continue
			}

			entry := LedgerEntry{
				EntryType:     e.Type,
				OpenSignature: e.OpenSignature,
				Direction:     e.Direction,
				Payload:       payload,
				Signature:     sig,
				Seq:           seq,
				Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
				Metadata:      e.Metadata,
			}
			l.writeEntry(entry)
			prevSig = sig
			if req.result != nil {
				req.result <- sig
			}
			continue
		}

		// Open entry path (no payload)
		sig, err := l.sign(sigInput)
		if err != nil {
			log.Printf("error signing open entry: %v", err)
			if req.result != nil {
				req.result <- ""
			}
			continue
		}

		entry := LedgerEntry{
			EntryType: "open",
			Signature: sig,
			Seq:       seq,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Metadata:  e.Metadata,
		}
		l.writeEntry(entry)
		prevSig = sig
		if req.result != nil {
			req.result <- sig
		}
	}
}

func (l *Ledger) writeEntry(entry LedgerEntry) {
	j, err := json.Marshal(entry)
	if err != nil {
		log.Printf("error marshalling entry: %v", err)
		return
	}
	if _, err := l.writer.Write(append(j, '\n')); err != nil {
		log.Printf("error writing ledger entry: %v", err)
	}
}

func (l *Ledger) sign(input []byte) (string, error) {
	digest := sha512.Sum512(input)
	sig, err := rsa.SignPKCS1v15(rand.Reader, l.key, crypto.SHA512, digest[:])
	if err != nil {
		return "", fmt.Errorf("RSA sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
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

func sizeBytes(size int64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(size))
	return buf
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

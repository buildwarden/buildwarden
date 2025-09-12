package relay

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type Ledger struct {
	key         *rsa.PrivateKey
	writer      io.Writer
	opens       chan OpenEntry
	checkpoints chan CheckpointEntry
	closes      chan CloseEntry
	exit        chan bool
	done        chan bool
	opensigs    map[*http.Request]string
}

type LedgerEntry struct {
	EntryType string   `json:"entry_type"`
	Payload   *Payload `json:"payload"`
	Signature string   `json:"signature"`
}

type OpenEntry struct {
	LedgerEntry
	Request *http.Request `json:"-"`
}

type CheckpointEntry struct {
	LedgerEntry
	OpenSignature string `json:"open_signature"`
}

type CloseEntry struct {
	LedgerEntry
	OpenSignature string `json:"open_signature"`
}

type Payload struct {
	Direction string `json:"direction"`
	Size      int64  `json:"size"`
	Hash      string `json:"hash"`
}

func NewLedger(w io.Writer) error {
	var err error
	ledger = &Ledger{
		writer:      w,
		opens:       make(chan OpenEntry),
		checkpoints: make(chan CheckpointEntry),
		closes:      make(chan CloseEntry),
		exit:        make(chan bool),
		done:        make(chan bool),
		opensigs:    make(map[*http.Request]string),
	}

	ledger.key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("error generating private key for ledger: %w", err)
	}

	go ledger.Loop()
	return nil
}

func FinishLedger() {
	if ledger != nil {
		ledger.Finish()
	}
}

func (l *Ledger) RecordOpenEvent(req *http.Request) {
	ent := OpenEntry{
		LedgerEntry: LedgerEntry{
			EntryType: "open",
			Payload: &Payload{
				Direction: "out",
				Size:      req.ContentLength,
			},
		},
		Request: req,
	}

	hash := sha512.New()
	hash.Write([]byte(req.Proto))
	hash.Write([]byte(req.Method))
	hash.Write([]byte(req.Host))
	ent.Payload.Hash = base64.StdEncoding.EncodeToString(hash.Sum(nil))

	l.opens <- ent
}

func (l *Ledger) RecordCloseEvent(res *http.Response) {
	sig, ok := l.opensigs[res.Request]
	if !ok {
		log.Printf("could not find open signature for response: %+v\n", res)
	}

	ent := CloseEntry{
		LedgerEntry: LedgerEntry{
			EntryType: "close",
			Payload: &Payload{
				Direction: "in",
				Size:      res.ContentLength,
			},
		},
		OpenSignature: sig,
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("error reading http.Response body: %v\n", err)
	}
	res.Body = io.NopCloser(bytes.NewReader(body))

	hash := sha512.New()
	hash.Write([]byte(res.Proto))
	hash.Write([]byte(res.Request.Method))
	hash.Write([]byte(res.Request.Host))
	hash.Write([]byte(body))
	ent.Payload.Hash = base64.StdEncoding.EncodeToString(hash.Sum(nil))

	l.closes <- ent
}

func (l *Ledger) Loop() {
	var lastsig string
	for {
		select {
		case open, more := <-l.opens:
			if more {
				l.writeOpen(open, lastsig)
				lastsig = open.Signature
			} else {
				l.done <- true
			}
		case ckp, more := <-l.checkpoints:
			if more {
				l.writeCheckpoint(ckp, lastsig)
				lastsig = ckp.Signature
			} else {
				l.done <- true
			}
		case close, more := <-l.closes:
			if more {
				l.writeClose(close, lastsig)
				lastsig = close.Signature
			} else {
				l.done <- true
			}
		}
	}
}

func (l *Ledger) writeOpen(o OpenEntry, prev string) {
	input := sha512.New()

	prevsig, err := base64.StdEncoding.DecodeString(prev)
	if err != nil {
		log.Printf("error decoding previous signature: %v\n", err)
	}
	input.Write(prevsig)

	buf := &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, o.Payload.Size)
	if err != nil {
		log.Printf("error converting payload size to bytes: %v\n", err)
		return
	}
	input.Write(buf.Bytes())

	hash, err := base64.StdEncoding.DecodeString(o.Payload.Hash)
	if err != nil {
		log.Printf("error decoding base64 hash: %v\n", err)
		return
	}
	input.Write(hash)

	sig, err := rsa.SignPKCS1v15(rand.Reader, l.key, crypto.SHA512, input.Sum(nil))
	if err != nil {
		log.Printf("error signing entry: %v\n", err)
		return
	}

	sigstr := base64.StdEncoding.EncodeToString(sig)
	l.opensigs[o.Request] = sigstr

	o.Signature = sigstr

	j, err := json.Marshal(o)
	if err != nil {
		log.Printf("error mashalling OpenEntry: %v\n", err)
		return
	}

	_, err = l.writer.Write(append(j, '\n'))
	if err != nil {
		log.Printf("error writing ledger entry: %v\n", err)
		return
	}
}

func (l *Ledger) writeCheckpoint(c CheckpointEntry, prev string) {
	input := sha512.New()

	prevsig, err := base64.StdEncoding.DecodeString(prev)
	if err != nil {
		log.Printf("error decoding previous signature: %v\n", err)
	}
	input.Write(prevsig)

	buf := &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, c.Payload.Size)
	if err != nil {
		log.Printf("error converting payload size to bytes: %v\n", err)
		return
	}
	input.Write(buf.Bytes())

	hash, err := base64.StdEncoding.DecodeString(c.Payload.Hash)
	if err != nil {
		log.Printf("error decoding base64 hash: %v\n", err)
		return
	}
	input.Write(hash)

	sig, err := rsa.SignPKCS1v15(rand.Reader, l.key, crypto.SHA512, input.Sum(nil))
	if err != nil {
		log.Printf("error signing entry: %v\n", err)
		return
	}

	sigstr := base64.StdEncoding.EncodeToString(sig)

	c.Signature = sigstr

	j, err := json.Marshal(c)
	if err != nil {
		log.Printf("error mashalling OpenEntry: %v\n", err)
		return
	}

	_, err = l.writer.Write(append(j, '\n'))
	if err != nil {
		log.Printf("error writing ledger entry: %v\n", err)
		return
	}
}

func (l *Ledger) writeClose(c CloseEntry, prev string) {
	input := sha512.New()

	prevsig, err := base64.StdEncoding.DecodeString(prev)
	if err != nil {
		log.Printf("error decoding previous signature: %v\n", err)
	}
	input.Write(prevsig)

	buf := &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, c.Payload.Size)
	if err != nil {
		log.Printf("error converting payload size to bytes: %v\n", err)
		return
	}
	input.Write(buf.Bytes())

	hash, err := base64.StdEncoding.DecodeString(c.Payload.Hash)
	if err != nil {
		log.Printf("error decoding base64 hash: %v\n", err)
		return
	}
	input.Write(hash)

	sig, err := rsa.SignPKCS1v15(rand.Reader, l.key, crypto.SHA512, input.Sum(nil))
	if err != nil {
		log.Printf("error signing entry: %v\n", err)
		return
	}

	sigstr := base64.StdEncoding.EncodeToString(sig)

	c.Signature = sigstr

	j, err := json.Marshal(c)
	if err != nil {
		log.Printf("error mashalling OpenEntry: %v\n", err)
		return
	}

	_, err = l.writer.Write(append(j, '\n'))
	if err != nil {
		log.Printf("error writing ledger entry: %v\n", err)
		return
	}
}

func (l *Ledger) Finish() {
	close(l.opens)
	close(l.checkpoints)
	close(l.closes)

	for i := 0; i < 3; i++ {
		<-ledger.done
	}
}

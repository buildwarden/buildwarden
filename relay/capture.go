package relay

import (
	"bytes"
	"io"
	"log"
)

// Captured payloads (--capture headers|bodies|all) stream to the collector,
// which lands them content-addressed (payloads/<hash>, linked from
// captures/<name>). The capture hash is blake2b-256 regardless of the
// ledger's hash suite, matching the historical payload naming.
var captureHashes = []string{"blake2b_256"}

// capturingReadCloser tees a response body to a streaming capture upload as
// the client reads it. If the client stops early, Close drains the rest so the
// capture is always complete.
type capturingReadCloser struct {
	source io.ReadCloser
	pw     *io.PipeWriter
	done   chan error
	closed bool
}

func (r *Relay) newCapturingReadCloser(
	source io.ReadCloser, baseName, suffix string,
) *capturingReadCloser {
	pr, pw := io.Pipe()
	c := &capturingReadCloser{source: source, pw: pw, done: make(chan error, 1)}
	name := captureName(baseName, suffix)
	go func() {
		err := r.sink.putCapture(name, pr, NewStreamingHasher(captureHashes))
		_ = pr.CloseWithError(err)
		c.done <- err
	}()
	return c
}

func (c *capturingReadCloser) Read(p []byte) (int, error) {
	n, err := c.source.Read(p)
	if n > 0 && c.pw != nil {
		if _, werr := c.pw.Write(p[:n]); werr != nil {
			c.abandon(werr)
		}
	}
	if err == io.EOF {
		c.finish()
	}
	return n, err
}

func (c *capturingReadCloser) Close() error {
	if c.pw != nil {
		_, _ = io.Copy(c.pw, c.source)
		c.finish()
	}
	return c.source.Close()
}

// finish ends the upload body and waits for the collector to confirm.
func (c *capturingReadCloser) finish() {
	if c.pw == nil || c.closed {
		return
	}
	c.closed = true
	_ = c.pw.Close()
	if err := <-c.done; err != nil {
		log.Printf("capture: upload failed: %v", err)
	}
	c.pw = nil
}

// abandon stops capturing after an upload failure without disturbing the
// client's read of the body.
func (c *capturingReadCloser) abandon(err error) {
	log.Printf("capture: upload failed: %v", err)
	_ = c.pw.CloseWithError(err)
	c.pw = nil
}

// savePayloadBytes captures an in-memory payload (request/response headers).
func (r *Relay) savePayloadBytes(data []byte, baseName, suffix string) {
	if len(data) == 0 {
		return
	}
	if err := r.sink.putCapture(captureName(baseName, suffix),
		bytes.NewReader(data), NewStreamingHasher(captureHashes)); err != nil {
		log.Printf("capture: upload failed: %v", err)
	}
}

func captureName(baseName, suffix string) string {
	if suffix != "" {
		return baseName + "." + suffix
	}
	return baseName
}

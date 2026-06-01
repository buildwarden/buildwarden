package relay

import (
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/blake2b"
)

type capturingReadCloser struct {
	source   io.ReadCloser
	tmp      *os.File
	baseName string
	suffix   string
	hasher   *StreamingHasher
	outDir   string
	done     bool
}

func (r *Relay) newCapturingReadCloser(
	source io.ReadCloser, baseName, suffix string,
) *capturingReadCloser {
	tmp, err := os.CreateTemp(filepath.Join(r.outDir, "payloads"), "cap-*")
	if err != nil {
		log.Printf("capture: error creating temp file: %v", err)
		return &capturingReadCloser{source: source, done: true}
	}
	return &capturingReadCloser{
		source:   source,
		tmp:      tmp,
		baseName: baseName,
		suffix:   suffix,
		hasher:   NewStreamingHasher([]string{"blake2b_256"}),
		outDir:   r.outDir,
	}
}

func (c *capturingReadCloser) Read(p []byte) (int, error) {
	n, err := c.source.Read(p)
	if n > 0 && c.tmp != nil {
		c.tmp.Write(p[:n])    //nolint:errcheck
		c.hasher.Write(p[:n]) //nolint:errcheck
	}
	if err == io.EOF && !c.done {
		c.done = true
		c.finalize()
	}
	return n, err
}

func (c *capturingReadCloser) Close() error {
	if !c.done {
		c.done = true
		buf := make([]byte, 32*1024)
		for {
			n, err := c.source.Read(buf)
			if n > 0 && c.tmp != nil {
				c.tmp.Write(buf[:n])    //nolint:errcheck
				c.hasher.Write(buf[:n]) //nolint:errcheck
			}
			if err != nil {
				break
			}
		}
		c.finalize()
	}
	return c.source.Close()
}

func (c *capturingReadCloser) finalize() {
	if c.tmp == nil {
		return
	}
	c.tmp.Close()
	hashBlock, _ := c.hasher.Finish()
	savePayloadFile(c.outDir, c.tmp.Name(), hashBlock, c.baseName, c.suffix)
}

func (r *Relay) savePayloadBytes(data []byte, baseName, suffix string) {
	if len(data) == 0 {
		return
	}
	hash := primaryHashBytes(data)
	payloadPath := filepath.Join(r.outDir, "payloads", hash)
	if _, err := os.Stat(payloadPath); os.IsNotExist(err) {
		os.WriteFile(payloadPath, data, 0644) //nolint:errcheck
	}
	symName := baseName
	if suffix != "" {
		symName += "." + suffix
	}
	symPath := filepath.Join(r.outDir, "captures", symName)
	os.Symlink(filepath.Join("..", "payloads", hash), symPath) //nolint:errcheck
}

func savePayloadFile(outDir, tmpPath string, hashBlock []byte, baseName, suffix string) {
	hash := hex.EncodeToString(hashBlock[:32])
	payloadPath := filepath.Join(outDir, "payloads", hash)
	if _, err := os.Stat(payloadPath); os.IsNotExist(err) {
		os.Rename(tmpPath, payloadPath) //nolint:errcheck
	} else {
		os.Remove(tmpPath) //nolint:errcheck
	}
	symName := baseName
	if suffix != "" {
		symName += "." + suffix
	}
	symPath := filepath.Join(outDir, "captures", symName)
	os.Symlink(filepath.Join("..", "payloads", hash), symPath) //nolint:errcheck
}

func primaryHashBytes(data []byte) string {
	h, _ := blake2b.New256(nil)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

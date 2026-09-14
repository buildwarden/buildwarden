// Package collector receives build outputs (artifacts, ledger, build log)
// streamed over HTTP by a relay's httpSink and lands them in an output
// directory. It is the ingress counterpart to the relay's egress sink and a
// standalone, reusable primitive: run it via cmd/collector, or embed it
// in-process (as the Hyper-V driver does). Together with the relay (egress /
// wire witness) and warden-io (in-guest agent) it is one of three composable
// pieces a third-party orchestrator can reuse.
//
// Protocol (all but /healthz require Authorization: Bearer <token>):
//
//	PUT  /v1/artifacts/<name>   stream body -> <dir>/artifacts/<name>
//	PUT  /v1/ledger             stream body -> <dir>/ledger
//	POST /v1/output             append body -> <dir>/build-output.log
//	GET  /healthz               liveness
//
// Bodies stream straight to disk with a fixed buffer, so multi-GB artifacts
// (container images, large wheels) transfer in constant memory.
package collector

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// Config configures a Collector.
type Config struct {
	// OutputDir is where received outputs are written. Required.
	OutputDir string
	// Token is the bearer token every write must present. Strongly recommended;
	// empty disables auth (only acceptable on a fully trusted local link).
	Token string
	// MaxArtifactBytes caps a single artifact; 0 uses the default.
	MaxArtifactBytes int64
	// Logf, if set, receives one-line progress logs.
	Logf func(format string, args ...any)
}

const defaultMaxArtifactBytes = 4 * 1024 * 1024 * 1024 // 4 GiB

// Collector lands streamed build outputs into OutputDir.
type Collector struct {
	cfg          Config
	maxArtifact  int64
	bytesWritten atomic.Int64
}

// New validates cfg and returns a Collector.
func New(cfg Config) (*Collector, error) {
	if cfg.OutputDir == "" {
		return nil, fmt.Errorf("collector: OutputDir is required")
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("collector: creating output dir: %w", err)
	}
	max := cfg.MaxArtifactBytes
	if max <= 0 {
		max = defaultMaxArtifactBytes
	}
	return &Collector{cfg: cfg, maxArtifact: max}, nil
}

// BytesWritten reports the total bytes landed so far (across all outputs).
func (c *Collector) BytesWritten() int64 { return c.bytesWritten.Load() }

func (c *Collector) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// Handler returns the HTTP handler implementing the collector protocol.
func (c *Collector) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/v1/artifacts/", c.auth(c.handleArtifact))
	mux.HandleFunc("/v1/ledger", c.auth(c.handleLedger))
	mux.HandleFunc("/v1/output", c.auth(c.handleOutput))
	return mux
}

// auth wraps h with constant-time-ish bearer-token enforcement.
func (c *Collector) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if c.cfg.Token != "" {
			got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(c.cfg.Token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		h(w, req)
	}
}

func (c *Collector) handleArtifact(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut && req.Method != http.MethodPost {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(req.URL.Path, "/v1/artifacts/")
	if !isSafeRelPath(name) {
		http.Error(w, "invalid artifact name", http.StatusBadRequest)
		return
	}
	dst := filepath.Join(c.cfg.OutputDir, "artifacts", filepath.FromSlash(name))
	n, err := c.streamToFile(dst, req.Body, c.maxArtifact)
	if err != nil {
		c.writeErr(w, err)
		return
	}
	c.logf("collector: artifact %s (%d bytes)", name, n)
	w.WriteHeader(http.StatusOK)
}

func (c *Collector) handleLedger(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut && req.Method != http.MethodPost {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	dst := filepath.Join(c.cfg.OutputDir, "ledger")
	n, err := c.streamToFile(dst, req.Body, c.maxArtifact)
	if err != nil {
		c.writeErr(w, err)
		return
	}
	c.logf("collector: ledger (%d bytes)", n)
	w.WriteHeader(http.StatusOK)
}

func (c *Collector) handleOutput(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost && req.Method != http.MethodPut {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	dst := filepath.Join(c.cfg.OutputDir, "build-output.log")
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		c.writeErr(w, err)
		return
	}
	defer f.Close()
	n, err := c.copy(f, req.Body, c.maxArtifact)
	if err != nil {
		c.writeErr(w, err)
		return
	}
	c.logf("collector: build-output +%d bytes", n)
	w.WriteHeader(http.StatusOK)
}

// streamToFile writes body to dst (creating parent dirs) with a size cap,
// streaming in constant memory. A partial write is cleaned up on error.
func (c *Collector) streamToFile(dst string, body io.Reader, max int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	n, err := c.copy(f, body, max)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(dst)
		return n, err
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return n, closeErr
	}
	return n, nil
}

// copy streams src->dst with a fixed buffer, enforcing a byte cap.
func (c *Collector) copy(dst io.Writer, src io.Reader, max int64) (int64, error) {
	buf := make([]byte, 64*1024)
	var total int64
	for {
		nr, rerr := src.Read(buf)
		if nr > 0 {
			total += int64(nr)
			if total > max {
				return total, &limitError{max: max}
			}
			if _, werr := dst.Write(buf[:nr]); werr != nil {
				return total, werr
			}
			c.bytesWritten.Add(int64(nr))
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}

type limitError struct{ max int64 }

func (e *limitError) Error() string {
	return fmt.Sprintf("payload exceeds limit of %d bytes", e.max)
}

func (c *Collector) writeErr(w http.ResponseWriter, err error) {
	var le *limitError
	if errors.As(err, &le) {
		http.Error(w, le.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// isSafeRelPath allows "/"-separated segments of safe chars, no traversal,
// no absolute path, no leading/trailing separators. Mirrors the relay's
// artifact-name policy so names round-trip.
func isSafeRelPath(p string) bool {
	if p == "" || len(p) > 1024 {
		return false
	}
	if p[0] == '/' || p[len(p)-1] == '/' {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		if seg[0] == '.' {
			return false
		}
		for _, ch := range seg {
			ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.'
			if !ok {
				return false
			}
		}
	}
	return true
}

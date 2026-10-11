// Package collector is the host side of the relay's single output path. Every
// driver runs one collector per build: the relay streams every output
// (artifacts, captures, the ledger, build output) and every signal (ready,
// heartbeat, complete) to it over HTTP, and pulls every build input (the build
// script and context files) from it. The relay therefore never needs a shared
// filesystem, and the collector is the one place a deployment would swap local
// disk for remote storage.
//
// Protocol (all but /healthz require Authorization: Bearer <token>):
//
//	PUT  /v1/artifacts/<name>   stream body -> <dir>/artifacts/<name>
//	PUT  /v1/captures/<name>    stream body -> <dir>/payloads/<hash>,
//	                            linked from <dir>/captures/<name>
//	                            (hash from the Warden-Hash trailer)
//	PUT  /v1/ledger             stream body -> <dir>/ledger
//	POST /v1/output             append body -> <dir>/build-output.log
//	GET  /v1/build-script       the build script staged for this build
//	GET  /v1/context/<path>     a context file, or a newline listing when
//	                            <path> is a directory (Warden-Context header)
//	POST /v1/ready              relay is up
//	POST /v1/heartbeat          the build guest is alive
//	POST /v1/complete           build finished (JSON exit code)
//	GET  /v1/status             last completion, else {"state":"running"}
//	GET  /healthz               liveness
//
// Bodies stream straight to disk with a fixed buffer, so multi-GB artifacts
// transfer in constant memory.
package collector

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HashTrailer is the HTTP trailer a relay sets on a capture upload, carrying
// the hex content hash of the body it just streamed. The collector names the
// content-addressed payload after it.
const HashTrailer = "Warden-Hash"

// ContextKindHeader tells the relay whether GET /v1/context returned a file
// ("file") or a directory listing ("list").
const ContextKindHeader = "Warden-Context"

// Config configures a Collector.
type Config struct {
	// OutputDir is where received outputs are written. Required.
	OutputDir string
	// Token is the bearer token every request must present. Empty disables
	// auth (only acceptable on a fully trusted local link).
	Token string
	// BuildScriptPath is the host path of the build script served on
	// GET /v1/build-script. Empty makes the endpoint 404.
	BuildScriptPath string
	// ContextDir is the host build context served on GET /v1/context/. Empty
	// makes the endpoint 404.
	ContextDir string
	// MaxArtifactBytes caps a single upload; 0 uses the default.
	MaxArtifactBytes int64
	// Logf, if set, receives one-line progress logs.
	Logf func(format string, args ...any)
	// OutputTee, if set, receives a copy of build output as it lands in
	// build-output.log, so the embedding driver can forward the guest's build
	// console live. Leave nil (--quiet) to suppress. Tee errors are ignored.
	OutputTee io.Writer
}

const defaultMaxArtifactBytes = 4 * 1024 * 1024 * 1024 // 4 GiB

// Collector lands streamed build outputs into OutputDir.
type Collector struct {
	cfg          Config
	maxArtifact  int64
	bytesWritten atomic.Int64

	ready         chan struct{}
	readyOnce     sync.Once
	lastHeartbeat atomic.Int64 // unix nanos; 0 = none yet

	mu         sync.Mutex
	completion *CompletionSignal
	done       chan CompletionSignal
	doneClosed bool
}

// New creates a Collector, creating OutputDir if needed.
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
	return &Collector{
		cfg:         cfg,
		maxArtifact: max,
		ready:       make(chan struct{}),
		done:        make(chan CompletionSignal, 1),
	}, nil
}

// BytesWritten reports the total bytes landed so far.
func (c *Collector) BytesWritten() int64 { return c.bytesWritten.Load() }

func (c *Collector) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// Handler returns the collector's HTTP handler.
func (c *Collector) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/v1/artifacts/", c.auth(c.handleArtifact))
	mux.HandleFunc("/v1/captures/", c.auth(c.handleCapture))
	mux.HandleFunc("/v1/ledger", c.auth(c.handleLedger))
	mux.HandleFunc("/v1/output", c.auth(c.handleOutput))
	mux.HandleFunc("/v1/build-script", c.auth(c.handleBuildScript))
	mux.HandleFunc("/v1/context/", c.auth(c.handleContext))
	mux.HandleFunc("/v1/ready", c.auth(c.handleReady))
	mux.HandleFunc("/v1/heartbeat", c.auth(c.handleHeartbeat))
	mux.HandleFunc("/v1/complete", c.auth(c.handleComplete))
	mux.HandleFunc("/v1/status", c.handleStatus)
	return mux
}

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

func isWrite(req *http.Request) bool {
	return req.Method == http.MethodPut || req.Method == http.MethodPost
}

func (c *Collector) handleArtifact(w http.ResponseWriter, req *http.Request) {
	if !isWrite(req) {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(req.URL.Path, "/v1/artifacts/")
	if !IsSafeArtifactPath(name) {
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

// handleCapture lands a captured payload content-addressed, the layout
// `warden inspect --extract` reads: payloads/<hash> plus captures/<name>
// linking to it. The hash arrives as a trailer because the relay only knows it
// once the body has streamed.
func (c *Collector) handleCapture(w http.ResponseWriter, req *http.Request) {
	if !isWrite(req) {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(req.URL.Path, "/v1/captures/")
	name = sanitizeCaptureName(name)
	if name == "" {
		http.Error(w, "invalid capture name", http.StatusBadRequest)
		return
	}
	payloads := filepath.Join(c.cfg.OutputDir, "payloads")
	if err := os.MkdirAll(payloads, 0o755); err != nil {
		c.writeErr(w, err)
		return
	}
	tmp, err := os.CreateTemp(payloads, "incoming-*")
	if err != nil {
		c.writeErr(w, err)
		return
	}
	_, err = c.copy(tmp, req.Body, c.maxArtifact)
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	hash := req.Trailer.Get(HashTrailer)
	if err == nil && !isHexHash(hash) {
		err = fmt.Errorf("missing or invalid %s trailer", HashTrailer)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		c.writeErr(w, err)
		return
	}
	payload := filepath.Join(payloads, hash)
	if _, statErr := os.Stat(payload); statErr == nil {
		_ = os.Remove(tmp.Name()) // identical content already landed
	} else if err := os.Rename(tmp.Name(), payload); err != nil {
		_ = os.Remove(tmp.Name())
		c.writeErr(w, err)
		return
	}
	if err := linkCapture(c.cfg.OutputDir, name, hash); err != nil {
		c.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// linkCapture points captures/<name> at ../payloads/<hash>. Where symlinks
// are unavailable (unprivileged Windows) it falls back to a copy, which
// `warden inspect` reads as a regular file.
func linkCapture(outDir, name, hash string) error {
	dir := filepath.Join(outDir, "captures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	link := filepath.Join(dir, name)
	_ = os.Remove(link)
	if os.Symlink(filepath.Join("..", "payloads", hash), link) == nil {
		return nil
	}
	src, err := os.Open(filepath.Join(outDir, "payloads", hash))
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(link)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

func (c *Collector) handleLedger(w http.ResponseWriter, req *http.Request) {
	if !isWrite(req) {
		http.Error(w, "PUT only", http.StatusMethodNotAllowed)
		return
	}
	// The relay streams the ledger for the whole build. If the stream breaks
	// (the relay died), keep what arrived: every record is individually
	// signed, so a truncated ledger still verifies up to its last record.
	dst := filepath.Join(c.cfg.OutputDir, "ledger")
	f, err := os.Create(dst)
	if err != nil {
		c.writeErr(w, err)
		return
	}
	n, err := c.copy(f, req.Body, c.maxArtifact)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		c.logf("collector: ledger stream ended early after %d bytes: %v", n, err)
		c.writeErr(w, err)
		return
	}
	c.logf("collector: ledger (%d bytes)", n)
	w.WriteHeader(http.StatusOK)
}

func (c *Collector) handleOutput(w http.ResponseWriter, req *http.Request) {
	if !isWrite(req) {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	f, err := os.OpenFile(filepath.Join(c.cfg.OutputDir, "build-output.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		c.writeErr(w, err)
		return
	}
	defer f.Close()
	var dst io.Writer = f
	if c.cfg.OutputTee != nil {
		dst = io.MultiWriter(f, ignoreErrWriter{c.cfg.OutputTee})
	}
	if _, err := c.copy(dst, req.Body, c.maxArtifact); err != nil {
		c.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ignoreErrWriter makes a best-effort tee: a failing console never fails the
// request or the log write it rides alongside.
type ignoreErrWriter struct{ w io.Writer }

func (e ignoreErrWriter) Write(p []byte) (int, error) {
	_, _ = e.w.Write(p)
	return len(p), nil
}

func (c *Collector) handleBuildScript(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if c.cfg.BuildScriptPath == "" {
		http.Error(w, "no build script", http.StatusNotFound)
		return
	}
	c.serveFile(w, c.cfg.BuildScriptPath)
}

// handleContext serves the build context with the relay's historical
// semantics: a file's bytes, or for a directory (or an empty / trailing-slash
// path) a newline-separated list of the safe file paths beneath it.
func (c *Collector) handleContext(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if c.cfg.ContextDir == "" {
		http.Error(w, "no build context", http.StatusNotFound)
		return
	}
	rel := strings.TrimPrefix(req.URL.Path, "/v1/context/")
	trimmed := strings.TrimSuffix(rel, "/")
	if trimmed != "" && !IsSafeContextPath(trimmed) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	full := filepath.Join(c.cfg.ContextDir, filepath.FromSlash(trimmed))
	info, err := os.Stat(full)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !info.IsDir() && !strings.HasSuffix(rel, "/") && rel != "" {
		w.Header().Set(ContextKindHeader, "file")
		c.serveFile(w, full)
		return
	}
	if !info.IsDir() {
		http.Error(w, "not a directory", http.StatusNotFound)
		return
	}
	var files []string
	_ = filepath.Walk(full, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		r, _ := filepath.Rel(c.cfg.ContextDir, p)
		r = filepath.ToSlash(r)
		if IsSafeContextPath(r) {
			files = append(files, r)
		}
		return nil
	})
	sort.Strings(files)
	body := strings.Join(files, "\n")
	if len(files) > 0 {
		body += "\n"
	}
	w.Header().Set(ContextKindHeader, "list")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func (c *Collector) serveFile(w http.ResponseWriter, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "unavailable", http.StatusNotFound)
		return
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil {
		w.Header().Set("Content-Length", fmt.Sprint(fi.Size()))
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func (c *Collector) streamToFile(dst string, body io.Reader, max int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	n, err := c.copy(f, body, max)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return n, err
}

// copy streams src into dst with a fixed buffer, enforcing max.
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

func isHexHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// MarkHeartbeat records guest liveness (exposed for tests and embedders).
func (c *Collector) MarkHeartbeat() { c.lastHeartbeat.Store(time.Now().UnixNano()) }

// LastHeartbeat returns the time of the last heartbeat, or the zero time.
func (c *Collector) LastHeartbeat() time.Time {
	n := c.lastHeartbeat.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

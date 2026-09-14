package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// OutputSink abstracts where the relay's outputs land. The localSink writes to
// the filesystem exactly as the relay always has (content-addressed payloads,
// symlinked artifacts, a local ledger file and build-output.log). The httpSink
// streams the same outputs to a remote collector over HTTP, for drivers (e.g.
// Hyper-V) that run the relay inside a VM with no host-shared filesystem.
//
// The abstraction is deliberately narrow: it covers only the four write points
// that must leave the guest (artifacts, the ledger, build output, and the
// ready/complete signals). Everything else about ledger semantics, hashing and
// the content-addressing layout is unchanged and lives outside this seam.
type OutputSink interface {
	// PutArtifact consumes the artifact body and returns the ledger hash block
	// and total size. It MUST hash the body with a StreamingHasher fed from the
	// same byte stream, so the ledger record is byte-identical regardless of
	// where the bytes are stored. Constant memory: the body is never fully
	// buffered.
	PutArtifact(name string, body io.Reader, hasher *StreamingHasher) (hashBlock []byte, size int64, err error)

	// LedgerWriter returns the io.Writer the ledger is serialized into. For the
	// localSink this is the on-disk ledger file; for the httpSink it is an
	// in-memory buffer flushed on FlushLedger.
	LedgerWriter() (io.Writer, error)

	// FlushLedger is called once after the ledger has been Finish()ed. The
	// localSink is a no-op (the file is already written); the httpSink PUTs the
	// buffered ledger bytes to the collector.
	FlushLedger() error

	// OutputWriter returns the destination for a single /v1/output POST. The
	// caller streams into the returned writer, then calls the returned finish
	// func exactly once. For the localSink the writer is the appended
	// build-output.log; for the httpSink it streams a POST /v1/output request.
	OutputWriter() (w io.Writer, finish func() error, err error)

	// PostReady signals the relay is up and serving. localSink no-ops.
	PostReady() error

	// PostComplete signals the build finished. localSink no-ops.
	PostComplete(exitCode int, message, errStr string) error
}

// --- localSink: byte-identical to the relay's historical filesystem behavior ---
type localSink struct {
	r *Relay
}

func newLocalSink(r *Relay) *localSink { return &localSink{r: r} }// PutArtifact reproduces the exact historical filesystem path: stream the body
// to a temp file under payloads/ while hashing, content-address to
// payloads/<primaryHash>, and symlink artifacts/<name> -> ../payloads/<hash>.
func (s *localSink) PutArtifact(name string, body io.Reader, hasher *StreamingHasher) ([]byte, int64, error) {
	r := s.r
	artifactsDir := filepath.Join(r.outDir, "artifacts")
	_ = os.MkdirAll(artifactsDir, 0755)
	payloadsDir := filepath.Join(r.outDir, "payloads")
	_ = os.MkdirAll(payloadsDir, 0755)

	tmpFile, err := os.CreateTemp(payloadsDir, "artifact-*")
	if err != nil {
		return nil, 0, err
	}
	// Hash and write in one pass, constant memory. TeeReader feeds the hasher
	// every byte that flows to the temp file.
	tee := io.TeeReader(body, hasher)
	if _, err := io.Copy(tmpFile, tee); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return nil, 0, err
	}
	tmpFile.Close()

	hashBlock, size := hasher.Finish()
	primaryHash := hashHex(hashBlock)
	payloadPath := filepath.Join(payloadsDir, primaryHash)
	_ = os.Rename(tmpFile.Name(), payloadPath)

	symPath := filepath.Join(artifactsDir, name)
	_ = os.MkdirAll(filepath.Dir(symPath), 0755)
	_ = os.Symlink(filepath.Join("..", "payloads", primaryHash), symPath)

	return hashBlock, size, nil
}

func (s *localSink) LedgerWriter() (io.Writer, error) {
	f, err := os.Create(filepath.Join(s.r.cfg.LedgerDir, "ledger"))
	if err != nil {
		return nil, fmt.Errorf("creating ledger file: %w", err)
	}
	s.r.ledgerFile = f
	return f, nil
}

func (s *localSink) FlushLedger() error {
	if s.r.ledgerFile != nil {
		return s.r.ledgerFile.Close()
	}
	return nil
}

func (s *localSink) OutputWriter() (io.Writer, func() error, error) {
	outPath := filepath.Join(s.r.outDir, "build-output.log")
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}

func (s *localSink) PostReady() error                            { return nil }
func (s *localSink) PostComplete(int, string, string) error      { return nil }

// --- httpSink: streams the same outputs to a remote collector ---

type httpSink struct {
	baseURL string
	token   string
	client  *http.Client

	mu        sync.Mutex
	ledgerBuf bytes.Buffer // ledger is small; buffer then PUT on flush
}

func newHTTPSink(baseURL, token string) *httpSink {
	return &httpSink{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{},
	}
}

func (s *httpSink) authReq(req *http.Request) {
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
}

// PutArtifact streams the body straight into a PUT /v1/artifacts/<name> request
// while hashing the identical byte stream via io.TeeReader. The request body is
// the tee'd reader, so http.Client reads it in constant memory (64KB transport
// buffer) and every byte is hashed as it flows to the wire. The whole artifact
// is never buffered, so multi-GB artifacts transfer with bounded memory.
func (s *httpSink) PutArtifact(name string, body io.Reader, hasher *StreamingHasher) ([]byte, int64, error) {
	url := s.baseURL + "/v1/artifacts/" + name
	tee := io.TeeReader(body, hasher)
	req, err := http.NewRequest(http.MethodPut, url, tee)
	if err != nil {
		return nil, 0, err
	}
	s.authReq(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("collector artifact PUT %s: status %d", name, resp.StatusCode)
	}
	hashBlock, size := hasher.Finish()
	return hashBlock, size, nil
}

func (s *httpSink) LedgerWriter() (io.Writer, error) {
	// Serialize the ledger into an in-memory buffer; PUT it on FlushLedger.
	return &s.ledgerBuf, nil
}

func (s *httpSink) FlushLedger() error {
	s.mu.Lock()
	data := append([]byte(nil), s.ledgerBuf.Bytes()...)
	s.mu.Unlock()
	url := s.baseURL + "/v1/ledger"
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	s.authReq(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("collector ledger PUT: status %d", resp.StatusCode)
	}
	return nil
}

// OutputWriter streams a POST /v1/output. It returns a pipe: the caller writes
// build output into pw, the request reads from pr, and finish() closes the pipe
// and waits for the request to complete. Constant memory, no buffering.
func (s *httpSink) OutputWriter() (io.Writer, func() error, error) {
	pr, pw := io.Pipe()
	url := s.baseURL + "/v1/output"
	req, err := http.NewRequest(http.MethodPost, url, pr)
	if err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return nil, nil, err
	}
	s.authReq(req)

	type result struct {
		resp *http.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := s.client.Do(req)
		done <- result{resp, err}
	}()

	finish := func() error {
		// Closing the write end signals EOF to the request body reader.
		_ = pw.Close()
		res := <-done
		if res.err != nil {
			return res.err
		}
		defer res.resp.Body.Close()
		_, _ = io.Copy(io.Discard, res.resp.Body)
		if res.resp.StatusCode != http.StatusOK {
			return fmt.Errorf("collector output POST: status %d", res.resp.StatusCode)
		}
		return nil
	}
	return pw, finish, nil
}

func (s *httpSink) PostReady() error {
	return s.postSignal(http.MethodPost, "/v1/ready", nil)
}

func (s *httpSink) PostComplete(exitCode int, message, errStr string) error {
	payload := map[string]any{
		"exit_code": exitCode,
		"message":   message,
		"error":     errStr,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.postSignal(http.MethodPost, "/v1/complete", data)
}

func (s *httpSink) postSignal(method, path string, body []byte) error {
	url := s.baseURL + path
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))
	}
	s.authReq(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("collector %s: status %d", path, resp.StatusCode)
	}
	return nil
}

// limitedReader caps the number of bytes read from r. On the first read that
// would exceed remaining it returns errArtifactTooLarge and sets exceeded, so
// the handler can distinguish a size-limit rejection from a transport error.
// The cap moved here from handleArtifactPost's inline loop so both sinks
// enforce it identically on a single streamed pass.
type limitedReader struct {
	r         io.Reader
	remaining int64
	exceeded  bool
}

var errArtifactTooLarge = fmt.Errorf("artifact size limit exceeded")

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		l.exceeded = true
		return 0, errArtifactTooLarge
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.r.Read(p)
	l.remaining -= int64(n)
	return n, err
}

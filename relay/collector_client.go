package relay

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/buildwarden/buildwarden/collector"
)

// collectorClient is the relay's only output (and input) path: every artifact,
// capture, ledger, build-output stream and signal goes to the build's
// collector over HTTP (normally on the orchestrator's tunnel, see relaycfg),
// and the build script and context files are fetched from it. The relay keeps
// no build state on disk, so it runs identically as a host process, a
// container or a diskless VM.
//
// It deliberately uses its own http.Client rather than the relay's
// SSRF-filtered upstream transport: the collector is a trusted host endpoint
// (often on loopback or a private host-only address) that the filter would
// otherwise refuse, and build traffic never reaches this client.
type collectorClient struct {
	baseURL string
	token   string
	client  *http.Client

	ledgerFinish func() error // ends the streaming ledger upload
}

func newCollectorClient(baseURL, token string, transport http.RoundTripper) *collectorClient {
	return &collectorClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Transport: transport},
	}
}

func (c *collectorClient) newRequest(
	ctx context.Context, method, path string, body io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func (c *collectorClient) do(req *http.Request, okStatus ...int) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	for _, s := range okStatus {
		if resp.StatusCode == s {
			return nil
		}
	}
	return fmt.Errorf("collector %s %s: HTTP %d", req.Method, req.URL.Path, resp.StatusCode)
}

// hashingBody feeds every byte it reads into a StreamingHasher and, at EOF,
// finalizes the hash and (optionally) publishes it as the request trailer, so
// the collector can content-address a body the relay could not hash ahead of
// time. Constant memory: the body is never buffered.
type hashingBody struct {
	r       io.Reader
	hasher  *StreamingHasher
	trailer http.Header

	hashBlock []byte
	size      int64
	finished  bool
}

func (h *hashingBody) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	if n > 0 {
		_, _ = h.hasher.Write(p[:n])
	}
	if err == io.EOF && !h.finished {
		h.finished = true
		h.hashBlock, h.size = h.hasher.Finish()
		if h.trailer != nil {
			h.trailer.Set(collector.HashTrailer, hashHex(h.hashBlock))
		}
	}
	return n, err
}

// putArtifact streams body to PUT /v1/artifacts/<name>, hashing the identical
// byte stream for the ledger record.
func (c *collectorClient) putArtifact(
	name string, body io.Reader, hasher *StreamingHasher,
) ([]byte, int64, error) {
	hb := &hashingBody{r: body, hasher: hasher}
	req, err := c.newRequest(
		context.Background(), http.MethodPut, "/v1/artifacts/"+escapePath(name), hb)
	if err != nil {
		return nil, 0, err
	}
	if err := c.do(req, http.StatusOK); err != nil {
		return nil, 0, err
	}
	if !hb.finished {
		return nil, 0, fmt.Errorf("artifact %s: body not fully sent", name)
	}
	return hb.hashBlock, hb.size, nil
}

// putCapture streams a captured payload to PUT /v1/captures/<name>, with its
// content hash as the trailer the collector names the payload after.
func (c *collectorClient) putCapture(
	name string, body io.Reader, hasher *StreamingHasher,
) error {
	hb := &hashingBody{
		r: body, hasher: hasher, trailer: http.Header{collector.HashTrailer: nil},
	}
	req, err := c.newRequest(
		context.Background(), http.MethodPut, "/v1/captures/"+url.PathEscape(name), hb)
	if err != nil {
		return err
	}
	req.Trailer = hb.trailer
	req.ContentLength = -1 // chunked, so the trailer can follow the body
	return c.do(req, http.StatusOK)
}

// putEnvironment lands the environment payload as <output>/environment.json.
func (c *collectorClient) putEnvironment(payload []byte) error {
	req, err := c.newRequest(context.Background(), http.MethodPut, "/v1/environment",
		bytes.NewReader(payload))
	if err != nil {
		return err
	}
	return c.do(req, http.StatusOK)
}

// openLedger starts streaming the ledger to the collector: one long PUT whose
// body is everything the ledger writes, so a relay that dies mid-build still
// leaves every record written so far. flushLedger ends the stream.
func (c *collectorClient) openLedger() (io.Writer, error) {
	w, finish, err := c.stream(http.MethodPut, "/v1/ledger")
	if err != nil {
		return nil, err
	}
	c.ledgerFinish = finish
	return w, nil
}

func (c *collectorClient) flushLedger() error {
	if c.ledgerFinish == nil {
		return nil
	}
	return c.ledgerFinish()
}

// outputWriter streams one POST /v1/output. The caller writes into w, then
// calls finish exactly once to end the body and await the response.
func (c *collectorClient) outputWriter() (w io.Writer, finish func() error, err error) {
	return c.stream(http.MethodPost, "/v1/output")
}

// stream opens a streaming request whose body is whatever is written to w;
// finish ends the body and awaits the response. Constant memory.
func (c *collectorClient) stream(
	method, path string,
) (w io.Writer, finish func() error, err error) {
	pr, pw := io.Pipe()
	req, err := c.newRequest(context.Background(), method, path, pr)
	if err != nil {
		_ = pw.Close()
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() {
		err := c.do(req, http.StatusOK)
		if err == nil {
			err = io.ErrClosedPipe // a reply before finish ends the stream
		}
		_ = pr.CloseWithError(err) // unblock a writer if the request ended early
		done <- err
	}()
	var once sync.Once
	var result error
	return pw, func() error {
		once.Do(func() {
			_ = pw.Close()
			result = <-done
			if result == io.ErrClosedPipe {
				result = nil
			}
		})
		return result
	}, nil
}

// fetch GETs an input from the collector. It returns the body, the
// Warden-Context kind header, and the HTTP status.
func (c *collectorClient) fetch(path string) ([]byte, string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, "", resp.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtifactBytes))
	return data, resp.Header.Get(collector.ContextKindHeader), resp.StatusCode, err
}

// signal POSTs a small, bounded control message. Signals must never hang the
// relay: it serves regardless of whether a signal lands, so they fail fast
// and the caller logs.
func (c *collectorClient) signal(path string, payload any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, http.StatusOK, http.StatusNoContent)
}

func (c *collectorClient) postReady() error     { return c.signal("/v1/ready", nil) }
func (c *collectorClient) postHeartbeat() error { return c.signal("/v1/heartbeat", nil) }
func (c *collectorClient) postComplete(exitCode int, message, errStr string) error {
	return c.signal("/v1/complete", map[string]any{
		"exit_code": exitCode, "message": message, "error": errStr,
	})
}

// escapePath escapes each segment of a slash-separated relative path.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// hashHex renders the primary (first 32-byte) hash of a hash block as hex,
// the content address used for payload files.
func hashHex(hashBlock []byte) string {
	return hex.EncodeToString(hashBlock[:32])
}

// limitedReader caps the bytes read from r. The first read past the cap
// returns errArtifactTooLarge and sets exceeded, so a size-limit rejection can
// be told apart from a transport error.
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

package relay

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// Config holds parameters for starting a relay.
type Config struct {
	LedgerDir   string
	ContextDir  string
	CaptureMode string
	SelfIP      net.IP
	UpstreamDNS string

	// Injected listeners. When non-nil, the relay uses these instead of
	// creating its own. This enables relay-on-host mode for VM drivers.
	DNSPacketConn net.PacketConn
	HTTPListener  net.Listener
	HTTPSListener net.Listener
}

// Relay is the core proxy that intercepts network traffic and writes a ledger.
type Relay struct {
	cfg Config

	caCert   []byte
	caKey    []byte
	mitmCert *tls.Certificate

	ledger     *Ledger
	outDir     string
	contextDir string

	captureConfig CaptureConfig
	captureSeq    atomic.Int64

	reqSigs      map[*http.Request][]byte
	reqBaseNames map[*http.Request]string
	reqSigsMu    sync.Mutex

	certCache   map[string]*tls.Certificate
	certCacheMu sync.RWMutex

	selfIP      net.IP
	upstreamDNS string

	errs chan error
}

// CaptureConfig controls what payload data is saved to disk.
type CaptureConfig struct {
	Headers bool
	Bodies  bool
}

var reservedHosts = map[string]bool{
	"artifacts": true,
	"cwd":       true,
}

const (
	schemaHTTPOpen    byte = 0
	schemaHTTPHeaders byte = 1
	schemaHTTPBody    byte = 2
	schemaArtifact    byte = 3
	schemaRedacted    byte = 4
	schemaEnvCtr      byte = 5
)

// Start initializes and starts the relay. It begins listening on DNS, HTTP,
// and HTTPS ports. Call Wait() to block until a listener fails.
func Start(cfg Config) (*Relay, error) {
	if err := os.MkdirAll(filepath.Join(cfg.LedgerDir, "payloads"), 0755); err != nil {
		return nil, fmt.Errorf("creating ledger directory: %w", err)
	}

	if cfg.CaptureMode != "" && cfg.CaptureMode != "none" {
		if err := os.MkdirAll(filepath.Join(cfg.LedgerDir, "captures"), 0755); err != nil {
			return nil, fmt.Errorf("creating captures directory: %w", err)
		}
	}

	r := &Relay{
		cfg:          cfg,
		outDir:       cfg.LedgerDir,
		contextDir:   cfg.ContextDir,
		reqSigs:      make(map[*http.Request][]byte),
		reqBaseNames: make(map[*http.Request]string),
		certCache:    make(map[string]*tls.Certificate),
		errs:         make(chan error, 3),
	}

	r.setCaptureMode(cfg.CaptureMode)

	// Create ledger file
	ledgerFile, err := os.Create(filepath.Join(cfg.LedgerDir, "ledger"))
	if err != nil {
		return nil, fmt.Errorf("creating ledger file: %w", err)
	}

	r.ledger, err = NewLedger(LedgerConfig{
		Writer:      ledgerFile,
		Environment: map[string]any{"type": "container"},
	})
	if err != nil {
		return nil, fmt.Errorf("initializing ledger: %w", err)
	}

	// Record build environment
	if err := r.recordEnvironmentFromVolume(); err != nil {
		return nil, fmt.Errorf("recording environment: %w", err)
	}

	// Detect or use provided self IP
	if cfg.SelfIP != nil {
		r.selfIP = cfg.SelfIP
	} else {
		if err := r.detectSelfIP(); err != nil {
			return nil, fmt.Errorf("detecting self IP: %w", err)
		}
	}

	// Detect or use provided upstream DNS
	if cfg.UpstreamDNS != "" {
		r.upstreamDNS = cfg.UpstreamDNS
	} else {
		r.detectUpstreamDNS()
	}

	// Generate ephemeral CA
	if err := r.generateCA(); err != nil {
		return nil, fmt.Errorf("generating CA: %w", err)
	}

	// Write CA cert for orchestrator to inject
	caPath := filepath.Join(cfg.LedgerDir, "ca.cert.pem")
	if err := os.WriteFile(caPath, r.caCert, 0644); err != nil {
		return nil, fmt.Errorf("writing CA cert: %w", err)
	}

	// Start listeners
	go func() { r.errs <- r.runDNS() }()
	go func() { r.errs <- r.runHTTP() }()
	go func() { r.errs <- r.runHTTPS() }()

	log.Printf("relay: listening on :53/udp :80/tcp :443/tcp")
	return r, nil
}

// Wait blocks until any listener fails.
func (r *Relay) Wait() error {
	err := <-r.errs
	r.ledger.Finish()
	return err
}

// Stop shuts down the relay.
func (r *Relay) Stop() {
	r.ledger.Finish()
}

// CACert returns the PEM-encoded ephemeral CA certificate.
func (r *Relay) CACert() []byte {
	return r.caCert
}

// CAFingerprint returns the SHA-256 fingerprint of the ephemeral CA cert.
func (r *Relay) CAFingerprint() string {
	if r.mitmCert == nil || r.mitmCert.Leaf == nil {
		return ""
	}
	h := sha256.Sum256(r.mitmCert.Leaf.Raw)
	return hex.EncodeToString(h[:])
}

func (r *Relay) setCaptureMode(mode string) {
	switch mode {
	case "headers":
		r.captureConfig = CaptureConfig{Headers: true}
	case "bodies":
		r.captureConfig = CaptureConfig{Bodies: true}
	case "all":
		r.captureConfig = CaptureConfig{Headers: true, Bodies: true}
	}
}

func (r *Relay) generateCA() error {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generating CA key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "BuildWarden Ephemeral CA",
			Organization: []string{"BuildWarden"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &caKey.PublicKey, caKey,
	)
	if err != nil {
		return fmt.Errorf("creating CA certificate: %w", err)
	}

	r.caCert = pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: certDER},
	)
	r.caKey = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(caKey),
		},
	)

	tlsCert, err := tls.X509KeyPair(r.caCert, r.caKey)
	if err != nil {
		return fmt.Errorf("parsing CA keypair: %w", err)
	}
	tlsCert.Leaf, _ = x509.ParseCertificate(tlsCert.Certificate[0])
	r.mitmCert = &tlsCert
	return nil
}

func (r *Relay) detectSelfIP() error {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return fmt.Errorf("cannot enumerate interfaces: %w", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To4()
		if ip != nil && !ip.IsLoopback() {
			r.selfIP = ip
			return nil
		}
	}
	return fmt.Errorf("no non-loopback IPv4 address found")
}

func (r *Relay) detectUpstreamDNS() {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		r.upstreamDNS = "8.8.8.8:53"
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			ns := fields[1]
			if ns != r.selfIP.String() && ns != "127.0.0.1" {
				r.upstreamDNS = ns + ":53"
				return
			}
		}
	}
	r.upstreamDNS = "8.8.8.8:53"
}

func (r *Relay) recordEnvironmentFromVolume() error {
	envDir := filepath.Join(r.cfg.LedgerDir, "environment")
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		return nil
	}

	payload, err := os.ReadFile(filepath.Join(envDir, "payload"))
	if err != nil {
		return fmt.Errorf("reading environment payload: %w", err)
	}
	if len(payload) == 0 {
		return fmt.Errorf("environment payload is empty")
	}

	metaBytes, err := os.ReadFile(filepath.Join(envDir, "metadata"))
	if err != nil {
		return fmt.Errorf("reading environment metadata: %w", err)
	}

	var meta map[string]any
	if err := cbor.Unmarshal(metaBytes, &meta); err != nil {
		return fmt.Errorf("invalid environment metadata CBOR: %w", err)
	}

	openMeta, _ := cbor.Marshal(map[string]any{
		"type": "environment",
	})
	openSig := r.ledger.Open(schemaEnvCtr, openMeta)

	hashBlock := r.ledger.ComputeHashBlock(payload)
	r.ledger.Close(openSig, -int64(len(payload)), hashBlock, schemaEnvCtr, metaBytes)

	log.Printf("relay: environment recorded (%d bytes)", len(payload))
	return nil
}

func (r *Relay) onRequest(req *http.Request) (*http.Request, *http.Response) {
	host := strings.Split(req.Host, ":")[0]

	if host == "artifacts" && req.Method == "POST" {
		return r.handleArtifactPost(req)
	}

	if host == "cwd" && req.Method == "GET" {
		return r.handleContextGet(req)
	}

	seq := r.captureSeq.Add(1)
	baseName := captureBaseName(seq, req.Method, host, req.URL.Path)

	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   req.Method,
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := r.ledger.Open(schemaHTTPOpen, openMeta)

	reqHeaders, err := httputil.DumpRequest(req, false)
	if err != nil {
		log.Printf("error dumping request headers: %v", err)
		reqHeaders = []byte{}
	}
	hb := r.ledger.ComputeHashBlock(reqHeaders)
	headersMeta := buildHeadersMeta(req.Header)
	r.ledger.Checkpoint(
		openSig, -int64(len(reqHeaders)), hb, schemaHTTPHeaders, headersMeta,
	)

	if r.captureConfig.Headers {
		r.savePayloadBytes(reqHeaders, baseName, "request-headers")
	}

	if req.Body != nil && req.ContentLength != 0 {
		if r.captureConfig.Bodies {
			req.Body = r.newCapturingReadCloser(req.Body, baseName, "request-body")
		}
		hasher := NewStreamingHasher(r.ledger.hashes)
		req.Body = &hashingReadCloser{
			source: req.Body,
			hasher: hasher,
			onClose: func(hashBlock []byte, size int64) {
				bodyMeta, _ := cbor.Marshal(map[string]any{})
				r.ledger.Checkpoint(
					openSig, -size, hashBlock, schemaHTTPBody, bodyMeta,
				)
			},
		}
	}

	r.reqSigsMu.Lock()
	r.reqSigs[req] = openSig
	r.reqBaseNames[req] = baseName
	r.reqSigsMu.Unlock()
	return req, nil
}

func (r *Relay) onResponse(res *http.Response, req *http.Request) *http.Response {
	if res == nil {
		return res
	}

	r.reqSigsMu.Lock()
	openSig, ok := r.reqSigs[req]
	baseName := r.reqBaseNames[req]
	if ok {
		delete(r.reqSigs, req)
		delete(r.reqBaseNames, req)
	}
	r.reqSigsMu.Unlock()
	if !ok {
		log.Printf("ledger: no open signature for response to %s", req.URL)
		return res
	}

	respHeaders, err := httputil.DumpResponse(res, false)
	if err != nil {
		log.Printf("error dumping response headers: %v", err)
		respHeaders = []byte{}
	}
	hb := r.ledger.ComputeHashBlock(respHeaders)
	headersMeta := buildHeadersMeta(res.Header)
	r.ledger.Checkpoint(
		openSig, int64(len(respHeaders)), hb, schemaHTTPHeaders, headersMeta,
	)

	if r.captureConfig.Headers {
		r.savePayloadBytes(respHeaders, baseName, "response-headers")
	}

	if res.StatusCode == http.StatusNotModified ||
		res.StatusCode == http.StatusNoContent ||
		(res.StatusCode >= 100 && res.StatusCode < 200) {
		bodyMeta, _ := cbor.Marshal(map[string]any{"status": res.StatusCode})
		r.ledger.Close(openSig, 0, nil, schemaHTTPBody, bodyMeta)
		return res
	}

	res.Body = newFairReader(res.Body, res.ContentLength)

	if r.captureConfig.Bodies {
		res.Body = r.newCapturingReadCloser(res.Body, baseName, "")
	}

	res.Body = r.newLedgerBody(res.Body, openSig, res.StatusCode)

	return res
}

func (r *Relay) handleArtifactPost(req *http.Request) (*http.Request, *http.Response) {
	artifactName := strings.TrimPrefix(req.URL.Path, "/")
	if artifactName == "" {
		artifactName = "unnamed"
	}
	if !isSafePath(artifactName) {
		return req, newTextResponse(req, http.StatusBadRequest, "invalid artifact name\n")
	}

	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   "POST",
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := r.ledger.Open(schemaHTTPOpen, openMeta)

	reqHeaders, err := httputil.DumpRequest(req, false)
	if err != nil {
		reqHeaders = []byte{}
	}
	hb := r.ledger.ComputeHashBlock(reqHeaders)
	headersMeta := buildHeadersMeta(req.Header)
	r.ledger.Checkpoint(
		openSig, -int64(len(reqHeaders)), hb, schemaHTTPHeaders, headersMeta,
	)

	artifactsDir := filepath.Join(r.outDir, "artifacts")
	_ = os.MkdirAll(artifactsDir, 0755)
	payloadsDir := filepath.Join(r.outDir, "payloads")
	_ = os.MkdirAll(payloadsDir, 0755)

	tmpFile, err := os.CreateTemp(payloadsDir, "artifact-*")
	if err != nil {
		log.Printf("artifact: error creating temp file: %v", err)
		return req, newTextResponse(req, http.StatusInternalServerError, "storage error")
	}

	hasher := NewStreamingHasher(r.ledger.hashes)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := req.Body.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n]) //nolint:errcheck
			tmpFile.Write(buf[:n]) //nolint:errcheck
		}
		if readErr != nil {
			break
		}
	}
	tmpFile.Close()
	req.Body.Close()

	hashBlock, size := hasher.Finish()

	primaryHash := hex.EncodeToString(hashBlock[:32])
	payloadPath := filepath.Join(payloadsDir, primaryHash)
	os.Rename(tmpFile.Name(), payloadPath) //nolint:errcheck

	symPath := filepath.Join(artifactsDir, artifactName)
	os.MkdirAll(filepath.Dir(symPath), 0755) //nolint:errcheck
	os.Symlink(filepath.Join("..", "payloads", primaryHash), symPath) //nolint:errcheck

	artMeta, _ := cbor.Marshal(map[string]any{
		"name":    artifactName,
		"context": map[string]any{},
	})
	r.ledger.Artifact(openSig, -size, hashBlock, schemaArtifact, artMeta)

	log.Printf("artifact: stored %s (%d bytes, hash:%s)", artifactName, size, primaryHash[:12])

	return req, newTextResponse(req, http.StatusOK,
		fmt.Sprintf("artifact stored: %s (%d bytes)\n", artifactName, size))
}

func (r *Relay) handleContextGet(req *http.Request) (*http.Request, *http.Response) {
	filePath := strings.TrimPrefix(req.URL.Path, "/")
	if !isSafeContextPath(filePath) {
		return req, newTextResponse(req, http.StatusForbidden, "forbidden\n")
	}

	fullPath := filepath.Join(r.contextDir, filePath)
	if !strings.HasPrefix(fullPath, r.contextDir+"/") {
		return req, newTextResponse(req, http.StatusForbidden, "forbidden\n")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return req, newTextResponse(req, http.StatusNotFound,
			fmt.Sprintf("not found: %s\n", filePath))
	}

	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   "GET",
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := r.ledger.Open(schemaHTTPOpen, openMeta)

	hashBlock := r.ledger.ComputeHashBlock(data)
	closeMeta, _ := cbor.Marshal(map[string]any{"path": filePath})
	r.ledger.Close(openSig, int64(len(data)), hashBlock, schemaHTTPBody, closeMeta)

	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/octet-stream"}},
		Body:          io.NopCloser(strings.NewReader(string(data))),
		ContentLength: int64(len(data)),
	}
	return req, resp
}

// --- helpers ---

type ledgerBody struct {
	source  io.ReadCloser
	openSig []byte
	status  int
	hasher  *StreamingHasher
	ledger  *Ledger
	done    bool
}

func (r *Relay) newLedgerBody(source io.ReadCloser, openSig []byte, status int) *ledgerBody {
	return &ledgerBody{
		source:  source,
		openSig: openSig,
		status:  status,
		hasher:  NewStreamingHasher(r.ledger.hashes),
		ledger:  r.ledger,
	}
}

func (lb *ledgerBody) Read(p []byte) (int, error) {
	n, err := lb.source.Read(p)
	if n > 0 {
		lb.hasher.Write(p[:n]) //nolint:errcheck
	}
	if err == io.EOF && !lb.done {
		lb.done = true
		lb.finish()
	}
	return n, err
}

func (lb *ledgerBody) Close() error {
	if !lb.done {
		lb.done = true
		buf := make([]byte, 32*1024)
		for {
			n, err := lb.source.Read(buf)
			if n > 0 {
				lb.hasher.Write(buf[:n]) //nolint:errcheck
			}
			if err != nil {
				break
			}
		}
		lb.finish()
	}
	return lb.source.Close()
}

func (lb *ledgerBody) finish() {
	hashBlock, size := lb.hasher.Finish()
	bodyMeta, _ := cbor.Marshal(map[string]any{"status": lb.status})
	lb.ledger.Close(lb.openSig, size, hashBlock, schemaHTTPBody, bodyMeta)
}

type hashingReadCloser struct {
	source  io.ReadCloser
	hasher  *StreamingHasher
	onClose func(hashBlock []byte, size int64)
	done    bool
}

func (h *hashingReadCloser) Read(p []byte) (int, error) {
	n, err := h.source.Read(p)
	if n > 0 {
		h.hasher.Write(p[:n]) //nolint:errcheck
	}
	if err == io.EOF && !h.done {
		h.done = true
		hashBlock, size := h.hasher.Finish()
		h.onClose(hashBlock, size)
	}
	return n, err
}

func (h *hashingReadCloser) Close() error {
	if !h.done {
		h.done = true
		buf := make([]byte, 32*1024)
		for {
			n, err := h.source.Read(buf)
			if n > 0 {
				h.hasher.Write(buf[:n]) //nolint:errcheck
			}
			if err != nil {
				break
			}
		}
		hashBlock, size := h.hasher.Finish()
		h.onClose(hashBlock, size)
	}
	return h.source.Close()
}

type capturingReadCloser struct {
	source   io.ReadCloser
	tmp      *os.File
	baseName string
	suffix   string
	hasher   *StreamingHasher
	outDir   string
	done     bool
}

func (r *Relay) newCapturingReadCloser(source io.ReadCloser, baseName, suffix string) *capturingReadCloser {
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

func newTextResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"text/plain"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func captureBaseName(seq int64, method, host, path string) string {
	slug := strings.ReplaceAll(host, ":", "-")
	p := strings.TrimPrefix(path, "/")
	p = strings.ReplaceAll(p, "/", "-")
	if len(p) > 80 {
		p = p[:80]
	}
	if p != "" {
		slug += "-" + p
	}
	return fmt.Sprintf("%03d-%s-%s", seq, method, slug)
}

func buildHeadersMeta(h http.Header) []byte {
	var headers [][]string
	for name, values := range h {
		if isStandardHeader(name) {
			continue
		}
		for _, v := range values {
			if isAuthHeader(name) {
				v = "<redacted>"
			}
			headers = append(headers, []string{name, v})
		}
	}
	if headers == nil {
		headers = [][]string{}
	}
	meta, _ := cbor.Marshal(map[string]any{"headers": headers})
	return meta
}

var standardHeaders = map[string]bool{
	"Content-Length":     true,
	"Content-Type":      true,
	"Transfer-Encoding": true,
	"Connection":        true,
	"Host":              true,
	"Accept":            true,
	"Accept-Encoding":   true,
	"Accept-Language":   true,
	"Cache-Control":     true,
	"Date":              true,
	"Server":            true,
	"User-Agent":        true,
	"Vary":              true,
	"Etag":              true,
	"Last-Modified":     true,
	"If-Modified-Since": true,
	"If-None-Match":     true,
	"Content-Encoding":  true,
	"Location":          true,
}

func isStandardHeader(name string) bool {
	return standardHeaders[http.CanonicalHeaderKey(name)]
}

var authHeaders = map[string]bool{
	"Authorization":       true,
	"Cookie":              true,
	"Set-Cookie":          true,
	"Proxy-Authorization": true,
}

func isAuthHeader(name string) bool {
	return authHeaders[http.CanonicalHeaderKey(name)]
}

const maxPathLen = 255

func isSafePath(p string) bool {
	if p == "" || len(p) > maxPathLen {
		return false
	}
	if p[0] == '/' || p[len(p)-1] == '/' {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if !isSafeSegment(seg) {
			return false
		}
	}
	return true
}

func isSafeSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if s[0] == '.' || s[0] == '-' || s[0] == '_' {
		return false
	}
	last := s[len(s)-1]
	if last == '.' || last == '-' || last == '_' {
		return false
	}
	for _, c := range s {
		if !isSafeChar(c) {
			return false
		}
	}
	return true
}

func isSafeChar(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.'
}

func isSafeContextPath(p string) bool {
	if p == "" || len(p) > maxPathLen {
		return false
	}
	if p[0] == '/' || p[len(p)-1] == '/' {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		last := seg[len(seg)-1]
		if last == '.' || last == '-' || last == '_' {
			return false
		}
		for _, c := range seg {
			if !isSafeChar(c) {
				return false
			}
		}
	}
	return true
}

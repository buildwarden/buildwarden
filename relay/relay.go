package relay

import (
	"bytes"
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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/ledger"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Config holds parameters for starting a relay.
type Config struct {
	// CollectorURL is the base URL of the build's collector. Required: it is
	// the relay's only output path and the source of the build script and
	// context files.
	CollectorURL string
	// CollectorToken is the bearer token presented on every collector request.
	CollectorToken string
	// CollectorTransport, when set, carries collector requests (the
	// orchestrator's tunnel); nil dials CollectorURL directly.
	CollectorTransport http.RoundTripper

	CaptureMode string
	SelfIP      net.IP
	UpstreamDNS string

	// Environment, when set, is recorded as the ledger's first entry.
	Environment *relaycfg.Environment

	// SSRF controls whether the safe dialer blocks loopback/metadata IPs.
	// Required for host-mode relay; container/VM modes rely on iptables/hypervisor.
	SSRF bool

	// BlockedSelfIP is set to the relay's virtual gateway IP in FD mode.
	// Prevents the upstream transport from connecting back to ourselves.
	BlockedSelfIP net.IP

	// UpstreamCACerts are additional PEM-encoded CA certificates trusted
	// when making outbound TLS connections to upstream servers.
	UpstreamCACerts [][]byte

	// UpstreamSystemCA controls whether the system certificate pool is
	// included when UpstreamCACerts are configured. Defaults to true.
	UpstreamSystemCA *bool

	// Injected listeners. When non-nil, the relay uses these instead of
	// creating its own. This enables relay-on-host mode for VM drivers.
	DNSPacketConn net.PacketConn
	HTTPListener  net.Listener
	HTTPSListener net.Listener

	// ControlListener, when non-nil, is used for the control plane HTTP
	// server instead of binding :8300. Used in FD mode where the control
	// plane is served through the netstack.
	ControlListener net.Listener
}

// Relay is the core proxy that intercepts network traffic and writes a ledger.
type Relay struct {
	cfg Config

	caCert   []byte
	caKey    []byte
	mitmCert *tls.Certificate

	ledger *Ledger
	sink   *collectorClient
	// flushOnce guards the ledger finish+flush so it runs exactly once whether
	// triggered by build completion (/exit), a listener failure (Wait), or Stop.
	flushOnce sync.Once

	captureConfig CaptureConfig
	captureSeq    atomic.Int64

	reqSigs      map[*http.Request][]byte
	reqBaseNames map[*http.Request]string
	reqSigsMu    sync.Mutex

	certCache   map[string]*tls.Certificate
	certCacheMu sync.RWMutex

	selfIP        net.IP
	blockedSelfIP net.IP
	upstreamDNS   string

	transport *http.Transport

	lastActivity       atomic.Int64
	outputBytesWritten atomic.Int64

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
	schemaEnv         byte = 5
)

const maxOutputBytes = 256 * 1024 * 1024

// maxEnvironmentBytes bounds the environment payload (a descriptor or image
// manifest, never a disk); the collector enforces the same cap.
const maxEnvironmentBytes = collector.MaxEnvironmentBytes
const maxArtifactBytes = 2 * 1024 * 1024 * 1024

// Start initializes and starts the relay. It begins listening on DNS, HTTP,
// HTTPS, and control plane ports. Call Wait() to block until a listener fails.
func Start(cfg Config) (*Relay, error) {
	if cfg.CollectorURL == "" {
		return nil, fmt.Errorf("relay: CollectorURL is required")
	}

	r := &Relay{
		cfg: cfg,
		sink: newCollectorClient(
			cfg.CollectorURL, cfg.CollectorToken, cfg.CollectorTransport),
		reqSigs:      make(map[*http.Request][]byte),
		reqBaseNames: make(map[*http.Request]string),
		certCache:    make(map[string]*tls.Certificate),
		errs:         make(chan error, 4),
	}

	headerEnv, err := environmentLabel(cfg.Environment)
	if err != nil {
		return nil, fmt.Errorf("relay: %w", err)
	}

	r.setCaptureMode(cfg.CaptureMode)
	r.buildTransport()

	ledgerWriter, err := r.sink.openLedger()
	if err != nil {
		return nil, fmt.Errorf("opening ledger stream: %w", err)
	}
	r.ledger, err = NewLedger(LedgerConfig{
		Writer:      ledgerWriter,
		Environment: headerEnv,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing ledger: %w", err)
	}

	// Record build environment
	if err := r.recordEnvironment(); err != nil {
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

	if cfg.BlockedSelfIP != nil {
		r.blockedSelfIP = cfg.BlockedSelfIP
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

	// Start listeners
	go r.runHeartbeat()
	go func() { r.errs <- r.runDNS() }()
	go func() { r.errs <- r.runHTTP() }()
	go func() { r.errs <- r.runHTTPS() }()
	go func() { r.errs <- r.runControlPlane() }()

	log.Printf("relay: listening on :53/udp :80/tcp :443/tcp :8300/tcp")

	if err := r.sink.postReady(); err != nil {
		log.Printf("relay: PostReady failed: %v", err)
	}

	return r, nil
}

// Wait blocks until any listener fails.
func (r *Relay) Wait() error {
	err := <-r.errs
	r.finishAndFlush()
	return err
}

// Stop shuts down the relay.
func (r *Relay) Stop() {
	r.finishAndFlush()
}

// finishAndFlush finalizes the ledger and PUTs it to the collector exactly
// once, whichever path triggers it (build completion, a listener failure via
// Wait, or Stop). The completion path calls it before signalling, since the
// orchestrator tears the relay down as soon as it sees completion.
func (r *Relay) finishAndFlush() {
	r.flushOnce.Do(func() {
		r.ledger.Finish()
		if err := r.sink.flushLedger(); err != nil {
			log.Printf("relay: flushing ledger: %v", err)
		}
	})
}

// LedgerPublicKeyHex returns the hex Ed25519 key that signs this ledger.
func (r *Relay) LedgerPublicKeyHex() string {
	return hex.EncodeToString(r.ledger.PublicKey())
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

// Ledger returns the underlying ledger writer.
func (r *Relay) Ledger() *Ledger {
	return r.ledger
}

// TouchActivity records that the relay is actively processing traffic.
func (r *Relay) TouchActivity() {
	r.lastActivity.Store(time.Now().Unix())
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
	if env := os.Getenv("UPSTREAM_DNS"); env != "" {
		if !strings.Contains(env, ":") {
			env += ":53"
		}
		r.upstreamDNS = env
		return
	}

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

// environmentLabel returns the header's unsigned environment label: the type
// the payload names in its own (signed) bytes, so the label can only echo the
// record. A claim declaring a different type is refused. Nil when the build
// has no environment record.
func environmentLabel(env *relaycfg.Environment) (map[string]any, error) {
	if env == nil {
		return nil, nil
	}
	if len(env.Payload) == 0 {
		return nil, fmt.Errorf("environment payload is empty")
	}
	if len(env.Payload) > maxEnvironmentBytes {
		return nil, fmt.Errorf("environment payload is %d bytes (limit %d)",
			len(env.Payload), maxEnvironmentBytes)
	}
	t, err := ledger.EnvironmentType(env.Payload)
	if err != nil {
		return nil, err
	}
	if env.Type != "" && env.Type != t {
		return nil, fmt.Errorf(
			"environment declared as %q but its payload names %q", env.Type, t)
	}
	return map[string]any{"type": t}, nil
}

// recordEnvironment writes the environment record (open + close carrying the
// payload's hash) and lands the exact hashed bytes with the collector, so the
// record can be verified from the output directory.
func (r *Relay) recordEnvironment() error {
	env := r.cfg.Environment
	if env == nil {
		return nil
	}
	var meta map[string]any
	if err := cbor.Unmarshal(env.Metadata, &meta); err != nil {
		return fmt.Errorf("invalid environment metadata CBOR: %w", err)
	}
	if err := r.sink.putEnvironment(env.Payload); err != nil {
		return fmt.Errorf("landing environment payload: %w", err)
	}

	openMeta, _ := cbor.Marshal(map[string]any{
		"type": "environment",
	})
	openSig := r.ledger.Open(schemaEnv, openMeta)

	hashBlock := r.ledger.ComputeHashBlock(env.Payload)
	r.ledger.Close(openSig, -int64(len(env.Payload)), hashBlock, schemaEnv, env.Metadata)

	log.Printf("relay: environment recorded (%d bytes)", len(env.Payload))
	return nil
}

// handleArtifacts serves the relay's guest API on the reserved host
// "artifacts": heartbeat, exit, the CA, the build script, the bootstrap
// config, and artifact uploads.
func (r *Relay) handleArtifacts(req *http.Request) (*http.Request, *http.Response) {
	switch req.URL.Path {
	case "/heartbeat":
		r.TouchActivity()
		return req, newTextResponse(req, http.StatusOK, "ok\n")
	case "/exit":
		code := 0
		if q := req.URL.Query().Get("code"); q != "" {
			_, _ = fmt.Sscanf(q, "%d", &code)
		}
		r.complete(code)
		return req, newTextResponse(req, http.StatusOK, "ok\n")
	case "/ca.pem":
		return r.handleCACertGet(req)
	case "/build-script":
		return r.handleBuildScript(req)
	case "/bootstrap":
		return r.handleBootstrap(req)
	default:
		if req.Method == "POST" {
			return r.handleArtifactPost(req)
		}
		return req, newTextResponse(req, http.StatusNotFound, "not found\n")
	}
}

func (r *Relay) onRequest(req *http.Request) (*http.Request, *http.Response) {
	host := strings.Split(req.Host, ":")[0]

	if host == "artifacts" {
		return r.handleArtifacts(req)
	}

	if host == "cwd" && req.Method == "GET" {
		return r.handleContextGet(req)
	}

	r.TouchActivity()

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
	if !collector.IsSafeArtifactPath(artifactName) {
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

	// Stream the body to the collector, hashing the identical byte stream for
	// the ledger record (constant memory).
	hasher := NewStreamingHasher(r.ledger.hashes)
	limited := &limitedReader{r: req.Body, remaining: maxArtifactBytes + 1}
	hashBlock, size, err := r.sink.putArtifact(artifactName, limited, hasher)
	req.Body.Close()
	if err != nil {
		if limited.exceeded {
			return req, newTextResponse(req, http.StatusRequestEntityTooLarge,
				"artifact size limit exceeded\n")
		}
		log.Printf("artifact: error storing %s: %v", artifactName, err)
		return req, newTextResponse(req, http.StatusInternalServerError, "storage error")
	}

	artMeta, _ := cbor.Marshal(map[string]any{
		"name":    artifactName,
		"context": map[string]any{},
	})
	r.ledger.Artifact(openSig, -size, hashBlock, schemaArtifact, artMeta)

	primaryHash := hashHex(hashBlock)
	log.Printf("artifact: stored %s (%d bytes, hash:%s)", artifactName, size, primaryHash[:12])

	return req, newTextResponse(req, http.StatusOK,
		fmt.Sprintf("artifact stored: %s (%d bytes)\n", artifactName, size))
}

func (r *Relay) handleCACertGet(req *http.Request) (*http.Request, *http.Response) {
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/x-pem-file"}},
		Body:          io.NopCloser(strings.NewReader(string(r.caCert))),
		ContentLength: int64(len(r.caCert)),
	}
	return req, resp
}

// handleBuildScript serves the canonical build script the guest fetches from
// http://artifacts/build-script, pulled from the collector. The exact bytes
// served are recorded in the ledger, so the build instructions are part of
// the witnessed record.
func (r *Relay) handleBuildScript(req *http.Request) (*http.Request, *http.Response) {
	data, _, status, err := r.sink.fetch("/v1/build-script")
	if err != nil || status != http.StatusOK {
		return req, newTextResponse(req, http.StatusNotFound, "build script unavailable\n")
	}
	r.recordServed(req, data, "build-script")
	return req, newBytesResponse(req, data, "application/octet-stream")
}

// handleBootstrap serves the build's bootstrap config (package bootstrap) at
// http://artifacts/bootstrap, pulled from the collector and recorded like the
// build script. A collector without one gives 404, which the executor reads as
// the empty config.
func (r *Relay) handleBootstrap(req *http.Request) (*http.Request, *http.Response) {
	data, _, status, err := r.sink.fetch("/v1/bootstrap")
	if err != nil || status != http.StatusOK {
		return req, newTextResponse(req, http.StatusNotFound, "no bootstrap config\n")
	}
	r.recordServed(req, data, "bootstrap")
	return req, newBytesResponse(req, data, "application/json")
}

// handleContextGet serves the build context at http://cwd/<path>: a file's
// bytes (recorded in the ledger), or a newline listing for a directory, the
// root, or a trailing-slash path.
func (r *Relay) handleContextGet(req *http.Request) (*http.Request, *http.Response) {
	filePath := strings.TrimPrefix(req.URL.Path, "/")
	trimmed := strings.TrimSuffix(filePath, "/")
	if trimmed != "" && !collector.IsSafeContextPath(trimmed) {
		return req, newTextResponse(req, http.StatusForbidden, "forbidden\n")
	}

	data, kind, status, err := r.sink.fetch("/v1/context/" + escapePath(filePath))
	if err != nil || status != http.StatusOK {
		return req, newTextResponse(req, http.StatusNotFound,
			fmt.Sprintf("not found: %s\n", filePath))
	}
	if kind == "list" {
		return req, newBytesResponse(req, data, "text/plain")
	}
	r.recordServed(req, data, filePath)
	return req, newBytesResponse(req, data, "application/octet-stream")
}

// recordServed records a relay-served input (build script, context file) in
// the ledger as an inbound GET body.
func (r *Relay) recordServed(req *http.Request, data []byte, path string) {
	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   "GET",
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := r.ledger.Open(schemaHTTPOpen, openMeta)
	hashBlock := r.ledger.ComputeHashBlock(data)
	closeMeta, _ := cbor.Marshal(map[string]any{"path": path})
	r.ledger.Close(openSig, int64(len(data)), hashBlock, schemaHTTPBody, closeMeta)
}

func newBytesResponse(req *http.Request, data []byte, contentType string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {contentType}},
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
		Request:       req,
	}
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
	"Content-Length":    true,
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

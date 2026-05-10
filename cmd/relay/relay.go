package main

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
	"github.com/miekg/dns"
	"golang.org/x/crypto/blake2b"
)

// CA_CERT and CA_KEY hold the PEM-encoded ephemeral CA certificate and key.
var CA_CERT []byte
var CA_KEY []byte

// mitmCert holds the parsed ephemeral CA for TLS interception.
var mitmCert *tls.Certificate

// GenerateCA creates an ephemeral CA certificate for this build.
func GenerateCA() error {
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

	CA_CERT = pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: certDER},
	)
	CA_KEY = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(caKey),
		},
	)

	tlsCert, err := tls.X509KeyPair(CA_CERT, CA_KEY)
	if err != nil {
		return fmt.Errorf("parsing CA keypair: %w", err)
	}
	tlsCert.Leaf, _ = x509.ParseCertificate(tlsCert.Certificate[0])
	mitmCert = &tlsCert
	return nil
}

// CAFingerprint returns the SHA-256 fingerprint of the ephemeral CA cert.
func CAFingerprint() string {
	if mitmCert == nil || mitmCert.Leaf == nil {
		return ""
	}
	h := sha256.Sum256(mitmCert.Leaf.Raw)
	return hex.EncodeToString(h[:])
}

// reqSigs maps in-flight requests to their ledger open signatures.
var (
	reqSigs      = make(map[*http.Request][]byte)
	reqBaseNames = make(map[*http.Request]string)
	reqSigsMu    sync.Mutex
)

// reservedHosts are hostnames intercepted by the relay rather than forwarded.
var reservedHosts = map[string]bool{
	"artifacts": true,
	"cwd":       true,
}

// contextDir is the path to the mounted build context, served via "cwd".
var contextDir string

func SetContextDir(dir string) { contextDir = dir }

// selfIP is the relay's own IP, detected from network interfaces at startup.
var selfIP net.IP

func DetectSelfIP() error {
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
			selfIP = ip
			return nil
		}
	}
	return fmt.Errorf("no non-loopback IPv4 address found")
}

// outDir is the ledger output directory, set by SetOutDir.
var outDir string

var activeLedger *Ledger

// CaptureConfig controls what payload data is saved to disk.
type CaptureConfig struct {
	Headers bool
	Bodies  bool
}

var captureConfig CaptureConfig
var captureSeq atomic.Int64

// SetCaptureMode configures payload capture from a mode string.
func SetCaptureMode(mode string) {
	switch mode {
	case "headers":
		captureConfig = CaptureConfig{Headers: true}
	case "bodies":
		captureConfig = CaptureConfig{Bodies: true}
	case "all":
		captureConfig = CaptureConfig{Headers: true, Bodies: true}
	}
}

// Schema indices matching the default schema list order.
const (
	schemaHTTPOpen    byte = 0
	schemaHTTPHeaders byte = 1
	schemaHTTPBody    byte = 2
	schemaArtifact    byte = 3
	schemaRedacted    byte = 4
)

func SetOutDir(dir string) { outDir = dir }

// SetLedger sets the active ledger writer (called from cmd/relay/main.go).
func SetLedger(l *Ledger) { activeLedger = l }

// upstreamDNS is the upstream resolver address, detected at startup.
var upstreamDNS string

// DetectUpstreamDNS reads /etc/resolv.conf to find the upstream resolver
// before we start our own DNS server (avoiding self-referential loops).
func DetectUpstreamDNS() {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		upstreamDNS = "8.8.8.8:53"
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			ns := fields[1]
			if ns != selfIP.String() && ns != "127.0.0.1" {
				upstreamDNS = ns + ":53"
				return
			}
		}
	}
	upstreamDNS = "8.8.8.8:53"
}

func RunDns(addr net.TCPAddr) error {
	dnsClient := &dns.Client{Timeout: 5 * time.Second}

	server := &dns.Server{Addr: addr.String(), Net: "udp"}
	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		for _, q := range m.Question {
			name := strings.TrimSuffix(q.Name, ".")

			if reservedHosts[name] {
				if q.Qtype == dns.TypeA {
					m.Answer = append(m.Answer, &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    0,
						},
						A: selfIP,
					})
				}
				if err := w.WriteMsg(m); err != nil {
					log.Printf("DNS proxy error: %s\n", err)
				}
				return
			}
		}

		// Forward the entire query upstream (preserves all question types).
		resp, _, err := dnsClient.Exchange(r, upstreamDNS)
		if err != nil {
			log.Printf("DNS forward error (%s): %v", upstreamDNS, err)
			m.Rcode = dns.RcodeServerFailure
			w.WriteMsg(m) //nolint:errcheck
			return
		}
		resp.Id = r.Id
		if err := w.WriteMsg(resp); err != nil {
			log.Printf("DNS proxy error: %s\n", err)
		}
	})
	// Also listen on TCP (fallback for clients behind UDP-blocking networks).
	tcpServer := &dns.Server{Addr: addr.String(), Net: "tcp"}
	go tcpServer.ListenAndServe() //nolint:errcheck

	return server.ListenAndServe()
}

func RunHttp(addr net.TCPAddr) error {
	listener, err := net.Listen("tcp", addr.String())
	if err != nil {
		return fmt.Errorf("error listening for http: %w", err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("HTTP listener.Accept error: %v\n", err)
			continue
		}
		go serveHTTPConn(conn)
	}
}

func RunHttps(addr net.TCPAddr) error {
	listener, err := net.Listen("tcp", addr.String())
	if err != nil {
		return fmt.Errorf("error listening for https: %w", err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("HTTPS listener.Accept error: %v\n", err)
			continue
		}
		go serveTLSConn(conn)
	}
}

func onRequest(req *http.Request) (*http.Request, *http.Response) {
	if activeLedger == nil {
		return req, nil
	}

	host := strings.Split(req.Host, ":")[0]

	// Intercept artifact submissions (POST to reserved hosts).
	if host == "artifacts" && req.Method == "POST" {
		return handleArtifactPost(req)
	}

	// Serve build context files (GET from "cwd" hostname).
	if host == "cwd" && req.Method == "GET" {
		return handleContextGet(req)
	}

	seq := captureSeq.Add(1)
	baseName := captureBaseName(seq, req.Method, host, req.URL.Path)

	// Synchronous open — blocks until signature is on the ledger.
	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   req.Method,
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := activeLedger.Open(schemaHTTPOpen, openMeta)

	// Checkpoint: request headers (direction: out = negative size)
	reqHeaders, err := httputil.DumpRequest(req, false)
	if err != nil {
		log.Printf("error dumping request headers: %v", err)
		reqHeaders = []byte{}
	}
	hb := activeLedger.ComputeHashBlock(reqHeaders)
	headersMeta := buildHeadersMeta(req.Header)
	activeLedger.Checkpoint(
		openSig, -int64(len(reqHeaders)), hb, schemaHTTPHeaders, headersMeta,
	)

	if captureConfig.Headers {
		savePayloadBytes(reqHeaders, baseName, "request-headers")
	}

	// If request has a body (POST/PUT), stream it through hashers.
	if req.Body != nil && req.ContentLength != 0 {
		if captureConfig.Bodies {
			req.Body = newCapturingReadCloser(req.Body, baseName, "request-body")
		}
		hasher := NewStreamingHasher(activeLedger.hashes)
		req.Body = &hashingReadCloser{
			source: req.Body,
			hasher: hasher,
			onClose: func(hashBlock []byte, size int64) {
				bodyMeta, _ := cbor.Marshal(map[string]any{})
				activeLedger.Checkpoint(
					openSig, -size, hashBlock, schemaHTTPBody, bodyMeta,
				)
			},
		}
	}

	// Store the open signature and capture base name for the response handler.
	reqSigsMu.Lock()
	reqSigs[req] = openSig
	reqBaseNames[req] = baseName
	reqSigsMu.Unlock()
	return req, nil
}

func hasPathTraversal(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

func handleArtifactPost(req *http.Request) (*http.Request, *http.Response) {
	artifactName := strings.TrimPrefix(req.URL.Path, "/")
	if artifactName == "" {
		artifactName = "unnamed"
	}
	if hasPathTraversal(artifactName) {
		resp := newTextResponse(req, http.StatusBadRequest,
			"invalid artifact name\n")
		return req, resp
	}

	// Record in ledger.
	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   "POST",
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := activeLedger.Open(schemaHTTPOpen, openMeta)

	// Checkpoint request headers.
	reqHeaders, err := httputil.DumpRequest(req, false)
	if err != nil {
		reqHeaders = []byte{}
	}
	hb := activeLedger.ComputeHashBlock(reqHeaders)
	headersMeta := buildHeadersMeta(req.Header)
	activeLedger.Checkpoint(
		openSig, -int64(len(reqHeaders)), hb, schemaHTTPHeaders, headersMeta,
	)

	// Stream body to disk and hash simultaneously.
	artifactsDir := filepath.Join(outDir, "artifacts")
	_ = os.MkdirAll(artifactsDir, 0755)
	payloadsDir := filepath.Join(outDir, "payloads")
	_ = os.MkdirAll(payloadsDir, 0755)

	tmpFile, err := os.CreateTemp(payloadsDir, "artifact-*")
	if err != nil {
		log.Printf("artifact: error creating temp file: %v", err)
		resp := newTextResponse(
			req, http.StatusInternalServerError, "storage error",
		)
		return req, resp
	}

	hasher := NewStreamingHasher(activeLedger.hashes)
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

	// Rename payload file to its primary hash.
	primaryHash := hex.EncodeToString(hashBlock[:32]) // first hash is blake2b_256 (32 bytes)
	payloadPath := filepath.Join(payloadsDir, primaryHash)
	os.Rename(tmpFile.Name(), payloadPath) //nolint:errcheck

	// Create symlink: artifacts/<name> -> ../payloads/<hash>
	symPath := filepath.Join(artifactsDir, artifactName)
	os.MkdirAll(filepath.Dir(symPath), 0755) //nolint:errcheck
	os.Symlink( //nolint:errcheck
		filepath.Join("..", "payloads", primaryHash), symPath,
	)

	// Write artifact record (closes the channel).
	artMeta, _ := cbor.Marshal(map[string]any{
		"name":    artifactName,
		"context": map[string]any{},
	})
	activeLedger.Artifact(openSig, -size, hashBlock, schemaArtifact, artMeta)

	log.Printf("artifact: stored %s (%d bytes, hash:%s)",
		artifactName, size, primaryHash[:12])

	resp := newTextResponse(req, http.StatusOK,
		fmt.Sprintf("artifact stored: %s (%d bytes)\n",
			artifactName, size))
	return req, resp
}

func handleContextGet(
	req *http.Request,
) (*http.Request, *http.Response) {
	filePath := strings.TrimPrefix(req.URL.Path, "/")
	if filePath == "" {
		resp := newTextResponse(req, http.StatusBadRequest, "no path\n")
		return req, resp
	}
	if hasPathTraversal(filePath) {
		resp := newTextResponse(req, http.StatusForbidden, "forbidden\n")
		return req, resp
	}

	fullPath := filepath.Join(contextDir, filePath)
	if !strings.HasPrefix(fullPath, contextDir+"/") {
		resp := newTextResponse(req, http.StatusForbidden, "forbidden\n")
		return req, resp
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		resp := newTextResponse(
			req, http.StatusNotFound,
			fmt.Sprintf("not found: %s\n", filePath))
		return req, resp
	}

	openMeta, _ := cbor.Marshal(map[string]any{
		"method":   "GET",
		"url":      req.URL.String(),
		"protocol": req.Proto,
	})
	openSig := activeLedger.Open(schemaHTTPOpen, openMeta)

	hashBlock := activeLedger.ComputeHashBlock(data)
	closeMeta, _ := cbor.Marshal(map[string]any{
		"path": filePath,
	})
	activeLedger.Close(openSig, int64(len(data)), hashBlock,
		schemaHTTPBody, closeMeta)

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

func onResponse(
	res *http.Response, req *http.Request,
) *http.Response {
	if activeLedger == nil || res == nil {
		return res
	}

	reqSigsMu.Lock()
	openSig, ok := reqSigs[req]
	baseName := reqBaseNames[req]
	if ok {
		delete(reqSigs, req)
		delete(reqBaseNames, req)
	}
	reqSigsMu.Unlock()
	if !ok {
		log.Printf("ledger: no open signature for response to %s",
			req.URL)
		return res
	}

	// Checkpoint: response headers (direction: in = positive size)
	respHeaders, err := httputil.DumpResponse(res, false)
	if err != nil {
		log.Printf("error dumping response headers: %v", err)
		respHeaders = []byte{}
	}
	hb := activeLedger.ComputeHashBlock(respHeaders)
	headersMeta := buildHeadersMeta(res.Header)
	activeLedger.Checkpoint(
		openSig, int64(len(respHeaders)), hb, schemaHTTPHeaders, headersMeta,
	)

	if captureConfig.Headers {
		savePayloadBytes(respHeaders, baseName, "response-headers")
	}

	// For responses with no body (304, 204, 1xx), close immediately.
	if res.StatusCode == http.StatusNotModified ||
		res.StatusCode == http.StatusNoContent ||
		(res.StatusCode >= 100 && res.StatusCode < 200) {
		bodyMeta, _ := cbor.Marshal(map[string]any{"status": res.StatusCode})
		activeLedger.Close(openSig, 0, nil, schemaHTTPBody, bodyMeta)
		return res
	}

	// Apply bandwidth fairness.
	res.Body = newFairReader(res.Body, res.ContentLength)

	// Capture response body to disk if configured.
	if captureConfig.Bodies {
		res.Body = newCapturingReadCloser(res.Body, baseName, "")
	}

	// Stream body through hashers; close entry written on EOF.
	res.Body = newLedgerBody(res.Body, openSig, res.StatusCode)

	return res
}

// newTextResponse creates a simple text response.
func newTextResponse(
	req *http.Request, status int, body string,
) *http.Response {
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

// ledgerBody wraps a response body, streaming bytes through hashers.
type ledgerBody struct {
	source  io.ReadCloser
	openSig []byte
	status  int
	hasher  *StreamingHasher
	done    bool
}

func newLedgerBody(
	source io.ReadCloser, openSig []byte, status int,
) *ledgerBody {
	return &ledgerBody{
		source:  source,
		openSig: openSig,
		status:  status,
		hasher:  NewStreamingHasher(activeLedger.hashes),
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
	activeLedger.Close(lb.openSig, size, hashBlock, schemaHTTPBody, bodyMeta)
}

// hasherSet runs multiple hash algorithms in parallel via goroutines.
type hasherSet struct {
	names   []string
	writers []*io.PipeWriter
	results []chan string
}

func newHasherSet(names []string) *hasherSet {
	hs := &hasherSet{
		names:   names,
		writers: make([]*io.PipeWriter, len(names)),
		results: make([]chan string, len(names)),
	}
	for i, name := range names {
		pr, pw := io.Pipe()
		hs.writers[i] = pw
		hs.results[i] = make(chan string, 1)
		go func(r io.Reader, n string, ch chan<- string) {
			h := newHash(n)
			io.Copy(h, r) //nolint:errcheck
			ch <- hex.EncodeToString(h.Sum(nil))
		}(pr, name, hs.results[i])
	}
	return hs
}

func (hs *hasherSet) write(p []byte) {
	for _, w := range hs.writers {
		w.Write(p) //nolint:errcheck
	}
}

func (hs *hasherSet) sums() map[string]string {
	for _, w := range hs.writers {
		w.Close()
	}
	result := make(map[string]string, len(hs.names))
	for i, name := range hs.names {
		result[name] = <-hs.results[i]
	}
	return result
}

// hashingReadCloser wraps a request body, hashing bytes as they are read.
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

// buildHeadersMeta creates CBOR metadata for the http-headers schema.
// Includes non-standard headers, with auth-related values redacted.
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
		// No interesting headers — still attach schema with empty list.
		headers = [][]string{}
	}
	meta, _ := cbor.Marshal(map[string]any{"headers": headers})
	return meta
}

var standardHeaders = map[string]bool{
	"Content-Length":    true,
	"Content-Type":     true,
	"Transfer-Encoding": true,
	"Connection":       true,
	"Host":             true,
	"Accept":           true,
	"Accept-Encoding":  true,
	"Accept-Language":  true,
	"Cache-Control":    true,
	"Date":             true,
	"Server":           true,
	"User-Agent":       true,
	"Vary":             true,
	"Etag":             true,
	"Last-Modified":    true,
	"If-Modified-Since": true,
	"If-None-Match":    true,
	"Content-Encoding": true,
	"Location":         true,
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

// captureBaseName builds a human-readable base name for capture symlinks.
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

// savePayloadBytes writes data to payloads/{hash} and creates a capture symlink.
func savePayloadBytes(data []byte, baseName, suffix string) {
	if len(data) == 0 {
		return
	}
	hash := primaryHash(data)
	payloadPath := filepath.Join(outDir, "payloads", hash)
	if _, err := os.Stat(payloadPath); os.IsNotExist(err) {
		os.WriteFile(payloadPath, data, 0644) //nolint:errcheck
	}
	symName := baseName
	if suffix != "" {
		symName += "." + suffix
	}
	symPath := filepath.Join(outDir, "captures", symName)
	os.Symlink(filepath.Join("..", "payloads", hash), symPath) //nolint:errcheck
}

// savePayloadFile renames a temp file to payloads/{hash} and creates a symlink.
func savePayloadFile(tmpPath string, hashBlock []byte, baseName, suffix string) {
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

func primaryHash(data []byte) string {
	h, _ := blake2b.New256(nil)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// capturingReadCloser wraps a body reader, writing all bytes to a temp file
// for later capture storage. It composes with hashingReadCloser/ledgerBody.
type capturingReadCloser struct {
	source   io.ReadCloser
	tmp      *os.File
	baseName string
	suffix   string
	hasher   *StreamingHasher
	done     bool
}

func newCapturingReadCloser(
	source io.ReadCloser, baseName, suffix string,
) *capturingReadCloser {
	tmp, err := os.CreateTemp(filepath.Join(outDir, "payloads"), "cap-*")
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
	}
}

func (c *capturingReadCloser) Read(p []byte) (int, error) {
	n, err := c.source.Read(p)
	if n > 0 && c.tmp != nil {
		c.tmp.Write(p[:n])  //nolint:errcheck
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
				c.tmp.Write(buf[:n])  //nolint:errcheck
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
	savePayloadFile(c.tmp.Name(), hashBlock, c.baseName, c.suffix)
}

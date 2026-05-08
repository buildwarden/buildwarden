// Package relay provides a transparent HTTPS and DNS proxy that emits a BuildWarden ledger.
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
	"time"

	"github.com/miekg/dns"
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
	reqSigs   = make(map[*http.Request]string)
	reqSigsMu sync.Mutex
)

// reservedHosts are hostnames intercepted by the relay rather than forwarded.
var reservedHosts = map[string]bool{
	"artifacts": true,
}

// outDir is the ledger output directory, set by SetOutDir.
var outDir string

// emptyHashes are the well-known hashes of an empty payload.
var emptyHashes = map[string]string{
	"blake2b_256": "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8",
	"sha256":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	"sha1":        "da39a3ee5e6b4b0d3255bfef95601890afd80709",
	"md5":         "d41d8cd98f00b204e9800998ecf8427e",
}

func SetOutDir(dir string) { outDir = dir }

func RunDns(addr net.TCPAddr) error {
	server := &dns.Server{Addr: addr.String(), Net: "udp"}
	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		for _, q := range m.Question {
			name := strings.TrimSuffix(q.Name, ".")

			// Reserved hostnames resolve to the relay itself.
			if reservedHosts[name] {
				if q.Qtype == dns.TypeA {
					m.Answer = append(m.Answer, &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    0,
						},
						A: net.ParseIP("10.0.87.2"),
					})
				}
				if err := w.WriteMsg(m); err != nil {
					log.Printf("DNS proxy error: %s\n", err)
				}
				return
			}

			addrs, err := net.LookupHost(q.Name)
			if err != nil {
				log.Printf("DNS proxy error: %s\n", err)
				return
			}
			if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA {
				continue
			}
			v4 := q.Qtype == dns.TypeA

			for _, a := range addrs {
				ip := net.ParseIP(a)
				if v4 && ip.To4() != nil {
					m.Answer = append(m.Answer, &dns.A{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    0,
						},
						A: ip,
					})
				} else if !v4 && ip.To4() == nil {
					m.Answer = append(m.Answer, &dns.AAAA{
						Hdr: dns.RR_Header{
							Name:   q.Name,
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    0,
						},
						AAAA: ip,
					})
				}
			}
		}
		if err := w.WriteMsg(m); err != nil {
			log.Printf("DNS proxy error: %s\n", err)
			return
		}
	})
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
	if ledger == nil {
		return req, nil
	}

	// Intercept artifact submissions (POST to reserved hosts).
	host := strings.Split(req.Host, ":")[0]
	if reservedHosts[host] && req.Method == "POST" {
		return handleArtifactPost(req)
	}

	// Synchronous open — blocks until signature is on the ledger.
	openSig := ledger.Open(map[string]any{
		"method":   req.Method,
		"url":      req.URL.String(),
		"protocol": req.Proto,
		"host":     req.Host,
	})

	// Checkpoint: request headers (direction: out)
	reqHeaders, err := httputil.DumpRequest(req, false)
	if err != nil {
		log.Printf("error dumping request headers: %v", err)
		reqHeaders = []byte{}
	}
	ledger.Checkpoint(openSig, "out", reqHeaders, nil)

	// If request has a body (POST/PUT), stream it through hashers.
	if req.Body != nil && req.ContentLength != 0 {
		hashers := newHasherSet(ledger.hashes)
		req.Body = &hashingReadCloser{
			source:  req.Body,
			hashers: hashers,
			onClose: func(size int64) {
				ledger.CheckpointHashed(
					openSig, "out", size, hashers.sums(),
				)
			},
		}
	}

	// Store the open signature for the response handler.
	reqSigsMu.Lock()
	reqSigs[req] = openSig
	reqSigsMu.Unlock()
	return req, nil
}

func handleArtifactPost(req *http.Request) (*http.Request, *http.Response) {
	artifactName := strings.TrimPrefix(req.URL.Path, "/")
	if artifactName == "" {
		artifactName = "unnamed"
	}

	// Record in ledger.
	openSig := ledger.Open(map[string]any{
		"method": "POST",
		"url":    req.URL.String(),
		"host":   req.Host,
		"schema": "artifact",
		"path":   artifactName,
	})

	// Checkpoint request headers.
	reqHeaders, err := httputil.DumpRequest(req, false)
	if err != nil {
		reqHeaders = []byte{}
	}
	ledger.Checkpoint(openSig, "out", reqHeaders, nil)

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

	hashers := newHasherSet(ledger.hashes)
	var size int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := req.Body.Read(buf)
		if n > 0 {
			hashers.write(buf[:n])
			tmpFile.Write(buf[:n]) //nolint:errcheck
			size += int64(n)
		}
		if readErr != nil {
			break
		}
	}
	tmpFile.Close()
	req.Body.Close()

	// Rename payload file to its primary hash.
	sums := hashers.sums()
	primaryHash := sums["sha256"]
	payloadPath := filepath.Join(payloadsDir, primaryHash)
	os.Rename(tmpFile.Name(), payloadPath) //nolint:errcheck

	// Create symlink: artifacts/<name> -> ../payloads/<hash>
	symPath := filepath.Join(artifactsDir, artifactName)
	os.MkdirAll(filepath.Dir(symPath), 0755) //nolint:errcheck
	os.Symlink( //nolint:errcheck
		filepath.Join("..", "payloads", primaryHash), symPath,
	)

	// Record body checkpoint and close in ledger.
	ledger.CheckpointHashed(openSig, "out", size, sums)
	ledger.CloseHashed(openSig, "in", 0, emptyHashes,
		map[string]any{"status": 200})

	log.Printf("artifact: stored %s (%d bytes, sha256:%s)",
		artifactName, size, primaryHash[:12])

	resp := newTextResponse(req, http.StatusOK,
		fmt.Sprintf("artifact stored: %s (%d bytes)\n",
			artifactName, size))
	return req, resp
}

func onResponse(
	res *http.Response, req *http.Request,
) *http.Response {
	if ledger == nil || res == nil {
		return res
	}

	reqSigsMu.Lock()
	openSig, ok := reqSigs[req]
	if ok {
		delete(reqSigs, req)
	}
	reqSigsMu.Unlock()
	if !ok {
		log.Printf("ledger: no open signature for response to %s",
			req.URL)
		return res
	}

	// Checkpoint: response headers (direction: in)
	respHeaders, err := httputil.DumpResponse(res, false)
	if err != nil {
		log.Printf("error dumping response headers: %v", err)
		respHeaders = []byte{}
	}
	ledger.Checkpoint(openSig, "in", respHeaders, nil)

	// For responses with no body (304, 204, 1xx), close immediately.
	if res.StatusCode == http.StatusNotModified ||
		res.StatusCode == http.StatusNoContent ||
		(res.StatusCode >= 100 && res.StatusCode < 200) {
		ledger.CloseHashed(openSig, "in", 0, emptyHashes,
			map[string]any{"status": res.StatusCode})
		return res
	}

	// Apply bandwidth fairness.
	res.Body = newFairReader(res.Body, res.ContentLength)

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
	openSig string
	status  int
	hashers *hasherSet
	size    int64
	done    bool
}

func newLedgerBody(
	source io.ReadCloser, openSig string, status int,
) *ledgerBody {
	return &ledgerBody{
		source:  source,
		openSig: openSig,
		status:  status,
		hashers: newHasherSet(ledger.hashes),
	}
}

func (lb *ledgerBody) Read(p []byte) (int, error) {
	n, err := lb.source.Read(p)
	if n > 0 {
		lb.hashers.write(p[:n])
		lb.size += int64(n)
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
				lb.hashers.write(buf[:n])
				lb.size += int64(n)
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
	metadata := map[string]any{"status": lb.status}
	ledger.CloseHashed(
		lb.openSig, "in", lb.size, lb.hashers.sums(), metadata,
	)
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
	hashers *hasherSet
	size    int64
	onClose func(int64)
	done    bool
}

func (h *hashingReadCloser) Read(p []byte) (int, error) {
	n, err := h.source.Read(p)
	if n > 0 {
		h.hashers.write(p[:n])
		h.size += int64(n)
	}
	if err == io.EOF && !h.done {
		h.done = true
		h.onClose(h.size)
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
				h.hashers.write(buf[:n])
				h.size += int64(n)
			}
			if err != nil {
				break
			}
		}
		h.onClose(h.size)
	}
	return h.source.Close()
}

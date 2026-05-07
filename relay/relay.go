// Package relay provides a transparent HTTPS and DNS proxy that emits a BuildWarden ledger.
package relay

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/inconshreveable/go-vhost"
	"github.com/miekg/dns"
)

var CA_CERT = goproxy.CA_CERT
var CA_KEY = goproxy.CA_KEY

type reqContext struct {
	openSig string
}

var httpCannotReachDest = []byte("HTTP/1.1 500 Cannot reach destination\r\n\r\n")

// reservedHosts are hostnames intercepted by the relay rather than forwarded.
var reservedHosts = map[string]bool{
	"artifacts": true,
}

// outDir is the ledger output directory, set by SetOutDir.
var outDir string

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
	proxy := goproxy.NewProxyHttpServer()
	proxy.NonproxyHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Host == "" {
			log.Print("HTTP proxy error: got HTTP request with no host header.\n")
			return
		}
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
		proxy.ServeHTTP(w, req)
	})
	proxy.OnRequest().HandleConnect(goproxy.AlwaysReject)
	proxy.OnRequest().Do(goproxy.FuncReqHandler(onRequest))
	proxy.OnResponse().Do(goproxy.FuncRespHandler(onResponse))

	return http.ListenAndServe(addr.String(), proxy)
}

func RunHttps(addr net.TCPAddr) error {
	proxy := goproxy.NewProxyHttpServer()
	proxy.NonproxyHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Host == "" {
			log.Printf("HTTPS proxy error: got request with no host header.\n")
			return
		}
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
		proxy.ServeHTTP(w, req)
	})
	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	proxy.OnRequest().Do(goproxy.FuncReqHandler(onRequest))
	proxy.OnResponse().Do(goproxy.FuncRespHandler(onResponse))

	listener, err := net.Listen("tcp", addr.String())
	if err != nil {
		return fmt.Errorf("error listening for https connections: %w", err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("HTTPS listener.Accept error: %v\n", err)
			continue
		}
		go proxyTlsConnection(conn, proxy)
	}
}

func onRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	if ledger == nil {
		return req, nil
	}

	// Intercept artifact submissions (POST to reserved hosts).
	host := strings.Split(req.Host, ":")[0]
	if reservedHosts[host] && req.Method == "POST" {
		return handleArtifactPost(req, ctx)
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
				ledger.CheckpointHashed(openSig, "out", size, hashers.sums())
			},
		}
	}

	// Store the open signature for the response handler.
	ctx.UserData = &reqContext{openSig: openSig}
	return req, nil
}

func handleArtifactPost(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
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
		resp := goproxy.NewResponse(req, "text/plain", http.StatusInternalServerError, "storage error")
		return req, resp
	}

	hashers := newHasherSet(ledger.hashes)
	var size int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := req.Body.Read(buf)
		if n > 0 {
			hashers.write(buf[:n])
			tmpFile.Write(buf[:n])
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
	os.Rename(tmpFile.Name(), payloadPath)

	// Create symlink: artifacts/<name> -> ../payloads/<hash>
	symPath := filepath.Join(artifactsDir, artifactName)
	os.MkdirAll(filepath.Dir(symPath), 0755)
	os.Symlink(filepath.Join("..", "payloads", primaryHash), symPath)

	// Record body checkpoint and close in ledger.
	ledger.CheckpointHashed(openSig, "out", size, sums)
	ledger.CloseHashed(openSig, "in", 0, map[string]string{
		"blake2b_256": "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8",
		"sha256":      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"sha1":        "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"md5":         "d41d8cd98f00b204e9800998ecf8427e",
	}, map[string]any{"status": 200})

	log.Printf("artifact: stored %s (%d bytes, sha256:%s)", artifactName, size, primaryHash[:12])

	resp := goproxy.NewResponse(req, "text/plain", http.StatusOK,
		fmt.Sprintf("artifact stored: %s (%d bytes)\n", artifactName, size))
	return req, resp
}

func onResponse(res *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if ledger == nil || res == nil {
		return res
	}

	rc, ok := ctx.UserData.(*reqContext)
	if !ok || rc == nil {
		log.Printf("ledger: no open signature for response to %s", ctx.Req.URL)
		return res
	}

	// Checkpoint: response headers (direction: in)
	respHeaders, err := httputil.DumpResponse(res, false)
	if err != nil {
		log.Printf("error dumping response headers: %v", err)
		respHeaders = []byte{}
	}
	ledger.Checkpoint(rc.openSig, "in", respHeaders, nil)

	// Replace the response body with a streaming wrapper that tees to
	// the hashers. The close entry is written when the body is fully read.
	res.Body = newLedgerBody(res.Body, rc.openSig, res.StatusCode)

	return res
}

// ledgerBody wraps a response body, streaming bytes through to the reader
// while simultaneously hashing them. When the body is fully consumed (or the
// upstream finishes), it writes the close entry to the ledger.
type ledgerBody struct {
	source  io.ReadCloser
	openSig string
	status  int
	hashers *hasherSet
	size    int64
	done    bool
}

func newLedgerBody(source io.ReadCloser, openSig string, status int) *ledgerBody {
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
		// Drain remaining bytes through hashers.
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
	ledger.CloseHashed(lb.openSig, "in", lb.size, lb.hashers.sums(), metadata)
}

// hasherSet runs multiple hash algorithms in parallel via goroutines.
// Each algorithm has its own goroutine reading from a pipe.
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
			io.Copy(h, r)
			ch <- hex.EncodeToString(h.Sum(nil))
		}(pr, name, hs.results[i])
	}
	return hs
}

func (hs *hasherSet) write(p []byte) {
	for _, w := range hs.writers {
		w.Write(p)
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
// When closed, it fires the onClose callback with the total size.
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
		// Drain remaining bytes.
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

func proxyTlsConnection(c net.Conn, proxy *goproxy.ProxyHttpServer) {
	tlsConn, err := vhost.TLS(c)
	if err != nil {
		log.Printf("HTTPS failure: %v\n", err)
		return
	}
	if tlsConn.Host() == "" {
		log.Print("HTTPS failure: cannot support non-SNI clients\n")
		return
	}
	connectReq := &http.Request{
		Method: "CONNECT",
		URL: &url.URL{
			Opaque: tlsConn.Host(),
			Host:   net.JoinHostPort(tlsConn.Host(), "443"),
		},
		Host:       tlsConn.Host(),
		Header:     make(http.Header),
		RemoteAddr: c.RemoteAddr().String(),
	}
	resp := dumbResponseWriter{tlsConn}
	proxy.ServeHTTP(resp, connectReq)
}

type dumbResponseWriter struct {
	net.Conn
}

func (dumb dumbResponseWriter) Header() http.Header {
	panic("Header() should not be called on this ResponseWriter")
}

func (dumb dumbResponseWriter) Write(buf []byte) (int, error) {
	if bytes.Equal(buf, []byte("HTTP/1.0 200 OK\r\n\r\n")) {
		return len(buf), nil
	}
	return dumb.Conn.Write(buf)
}

func (dumb dumbResponseWriter) WriteHeader(code int) {
	panic("WriteHeader() should not be called on this ResponseWriter")
}

func (dumb dumbResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return dumb, bufio.NewReadWriter(bufio.NewReader(dumb), bufio.NewWriter(dumb)), nil
}

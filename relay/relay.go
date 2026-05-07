// Package relay provides a transparent HTTPS and DNS proxy that emits a BuildWarden ledger.
package relay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

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

func RunDns(addr net.TCPAddr) error {
	server := &dns.Server{Addr: addr.String(), Net: "udp"}
	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		for _, q := range m.Question {
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

	// If request has a body (POST/PUT), checkpoint it as out.
	if req.Body != nil && req.ContentLength != 0 {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			log.Printf("error reading request body: %v", err)
		} else if len(body) > 0 {
			ledger.Checkpoint(openSig, "out", body, nil)
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
	}

	// Store the open signature for the response handler.
	ctx.UserData = &reqContext{openSig: openSig}
	return req, nil
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
	source    io.ReadCloser
	openSig   string
	status    int
	buf       bytes.Buffer
	done      bool
}

func newLedgerBody(source io.ReadCloser, openSig string, status int) *ledgerBody {
	return &ledgerBody{
		source:  source,
		openSig: openSig,
		status:  status,
	}
}

func (lb *ledgerBody) Read(p []byte) (int, error) {
	n, err := lb.source.Read(p)
	if n > 0 {
		lb.buf.Write(p[:n])
	}
	if err == io.EOF && !lb.done {
		lb.done = true
		lb.finish()
	}
	return n, err
}

func (lb *ledgerBody) Close() error {
	// If the client closes before reading everything, drain the rest
	// from upstream so we can hash the complete response.
	if !lb.done {
		lb.done = true
		remaining, _ := io.ReadAll(lb.source)
		lb.buf.Write(remaining)
		lb.finish()
	}
	return lb.source.Close()
}

func (lb *ledgerBody) finish() {
	metadata := map[string]any{"status": lb.status}
	ledger.Close(lb.openSig, "in", lb.buf.Bytes(), metadata)
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

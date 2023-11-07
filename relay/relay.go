// Package relay provides a transparent HTTPS and DNS proxy that emits a BuildWarden ledger.
package relay

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/elazarl/goproxy"
	"github.com/inconshreveable/go-vhost"
	"github.com/miekg/dns"
)

var CA_CERT = goproxy.CA_CERT
var CA_KEY = goproxy.CA_KEY

var ledger *Ledger
var httpCannotReachDest = []byte("HTTP/1.1 500 Cannot reach destination\r\n\r\n")

// TODO: Detect upstream DNS automatically.
// TODO: Support fake DNS requests (always respond with a given IP).
var DefaultDns = []string{"127.0.0.1:53"}

func RunDns(addr net.TCPAddr) error {
	server := &dns.Server{Addr: addr.String(), Net: "udp"}
	dns.HandleFunc(".", func(w dns.ResponseWriter, m *dns.Msg) {
		r, err := dns.Exchange(m, DefaultDns[0]) // FIXME: Add DNS fallback.
		if err != nil {
			log.Printf("DNS proxy error: %s\n", err)
			return
		}
		_ = w.WriteMsg(r)
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
	proxy.OnRequest().Do(goproxy.FuncReqHandler(
		func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			ledger.RecordOpenEvent(req)
			return req, nil
		}))
	proxy.OnResponse().Do(goproxy.FuncRespHandler(
		func(res *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			ledger.RecordCloseEvent(res)
			return res
		}))

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
	proxy.OnRequest().Do(goproxy.FuncReqHandler(
		func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			ledger.RecordOpenEvent(req)
			return req, nil
		}))
	proxy.OnResponse().Do(goproxy.FuncRespHandler(
		func(res *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			ledger.RecordCloseEvent(res)
			return res
		}))

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

func dial(ctx context.Context, proxy *goproxy.ProxyHttpServer, network, addr string) (
	c net.Conn, err error) {
	if proxy.Tr.DialContext != nil {
		return proxy.Tr.DialContext(ctx, network, addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

func connectDial(ctx context.Context, proxy *goproxy.ProxyHttpServer, network, addr string) (
	c net.Conn, err error) {
	if proxy.ConnectDial == nil {
		return dial(ctx, proxy, network, addr)
	}
	return proxy.ConnectDial(network, addr)
}

type dumbResponseWriter struct {
	net.Conn
}

func (dumb dumbResponseWriter) Header() http.Header {
	panic("Header() should not be called on this ResponseWriter")
}

func (dumb dumbResponseWriter) Write(buf []byte) (int, error) {
	if bytes.Equal(buf, []byte("HTTP/1.0 200 OK\r\n\r\n")) {
		// throw away the HTTP OK response from the faux CONNECT request.
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

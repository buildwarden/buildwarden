package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"sync"
	"time"
)

// transport is the shared HTTP transport for forwarding requests upstream.
var transport = &http.Transport{
	TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	MaxIdleConnsPerHost: 16,
	IdleConnTimeout:     90 * time.Second,
}

// signHost generates a TLS certificate for the given hostname, signed by the
// ephemeral MITM CA. Certs are cached to avoid repeated generation.
func signHost(hostname string) (*tls.Certificate, error) {
	certCacheMu.RLock()
	if cert, ok := certCache[hostname]; ok {
		certCacheMu.RUnlock()
		return cert, nil
	}
	certCacheMu.RUnlock()

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{hostname},
	}
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
		template.DNSNames = nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader, template, mitmCert.Leaf, &key.PublicKey, mitmCert.PrivateKey,
	)
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER, mitmCert.Certificate[0]},
		PrivateKey:  key,
	}

	certCacheMu.Lock()
	certCache[hostname] = cert
	certCacheMu.Unlock()
	return cert, nil
}

var (
	certCache   = make(map[string]*tls.Certificate)
	certCacheMu sync.RWMutex
)

// serveHTTPConn handles a single plain-HTTP connection, supporting keep-alive.
func serveHTTPConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)

	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		// Ensure the URL is absolute for the transport.
		if req.URL.Host == "" {
			req.URL.Host = req.Host
		}
		if req.URL.Scheme == "" {
			req.URL.Scheme = "http"
		}

		resp := roundTrip(req)
		keepAlive := shouldKeepAlive(req, resp)
		if !keepAlive {
			resp.Header.Set("Connection", "close")
		}
		resp.Write(conn) //nolint:errcheck
		resp.Body.Close()
		if !keepAlive {
			return
		}
	}
}

// serveTLSConn handles a single MITM'd TLS connection, supporting keep-alive.
func serveTLSConn(conn net.Conn) {
	defer conn.Close()

	tlsConn := tls.Server(conn, &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return signHost(hello.ServerName)
		},
	})
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("TLS handshake error: %v", err)
		return
	}

	br := bufio.NewReader(tlsConn)
	host := tlsConn.ConnectionState().ServerName

	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		// Reconstruct the full URL for upstream.
		if req.URL.Host == "" {
			req.URL.Host = host
		}
		if req.URL.Scheme == "" {
			req.URL.Scheme = "https"
		}
		req.RequestURI = ""

		resp := roundTrip(req)
		keepAlive := shouldKeepAlive(req, resp)
		if !keepAlive {
			resp.Header.Set("Connection", "close")
		}
		// http.Response.Write correctly omits body for 304/204/1xx.
		resp.Write(tlsConn) //nolint:errcheck
		resp.Body.Close()
		if !keepAlive {
			return
		}
	}
}

// roundTrip forwards a request upstream, calling the ledger hooks.
// Returns a response (always non-nil; on error returns a 502).
func roundTrip(req *http.Request) *http.Response {
	req, interceptResp := onRequest(req)
	if interceptResp != nil {
		return interceptResp
	}

	req.RequestURI = ""
	resp, err := transport.RoundTrip(req)
	if err != nil {
		log.Printf("upstream error: %v", err)
		resp = &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     http.Header{"Content-Type": {"text/plain"}},
			Body:       io.NopCloser(nil),
		}
	}

	resp = onResponse(resp, req)
	return resp
}

// shouldKeepAlive determines if the connection should stay open.
func shouldKeepAlive(req *http.Request, resp *http.Response) bool {
	if req.Close {
		return false
	}
	if resp.Header.Get("Connection") == "close" {
		return false
	}
	// HTTP/1.1 defaults to keep-alive; HTTP/1.0 requires explicit.
	if req.ProtoAtLeast(1, 1) {
		return true
	}
	return req.Header.Get("Connection") == "keep-alive"
}

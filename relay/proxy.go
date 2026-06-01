package relay

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"
)

func (r *Relay) buildTransport() {
	tlsCfg := &tls.Config{}

	if len(r.cfg.UpstreamCACerts) > 0 {
		includeSystem := r.cfg.UpstreamSystemCA == nil || *r.cfg.UpstreamSystemCA
		var pool *x509.CertPool
		if includeSystem {
			var err error
			pool, err = x509.SystemCertPool()
			if err != nil {
				pool = x509.NewCertPool()
			}
		} else {
			pool = x509.NewCertPool()
		}
		for _, pem := range r.cfg.UpstreamCACerts {
			pool.AppendCertsFromPEM(pem)
		}
		tlsCfg.RootCAs = pool
		log.Printf("relay: loaded %d upstream CA bundle(s)", len(r.cfg.UpstreamCACerts))
	}

	t := &http.Transport{
		TLSClientConfig:     tlsCfg,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	}
	if r.cfg.SSRF {
		t.DialContext = r.safeDialContext
	}
	r.transport = t
}

func (r *Relay) safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		if r.isBlockedIP(ip.IP) {
			return nil, fmt.Errorf(
			"blocked: connection to %s (%s) denied", host, ip.IP)

		}
	}

	var dialer net.Dialer
	for _, ip := range ips {
		address := net.JoinHostPort(ip.IP.String(), port)
		conn, err := dialer.DialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("all addresses for %s failed", host)
}

func (r *Relay) isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return true
	}
	if ip.Equal(net.ParseIP("100.100.100.200")) {
		return true
	}
	if r.blockedSelfIP != nil && ip.Equal(r.blockedSelfIP) {
		return true
	}
	return false
}

func (r *Relay) signHost(hostname string) (*tls.Certificate, error) {
	r.certCacheMu.RLock()
	if cert, ok := r.certCache[hostname]; ok {
		r.certCacheMu.RUnlock()
		return cert, nil
	}
	r.certCacheMu.RUnlock()

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
		rand.Reader, template, r.mitmCert.Leaf, &key.PublicKey, r.mitmCert.PrivateKey,
	)
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER, r.mitmCert.Certificate[0]},
		PrivateKey:  key,
	}

	r.certCacheMu.Lock()
	r.certCache[hostname] = cert
	r.certCacheMu.Unlock()
	return cert, nil
}

// ServeHTTPConn handles a single plain-HTTP connection, supporting keep-alive.
// Exported for use by mode-specific ingress (FD mode).
func (r *Relay) ServeHTTPConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)

	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		if req.URL.Host == "" {
			req.URL.Host = req.Host
		}
		if req.URL.Scheme == "" {
			req.URL.Scheme = "http"
		}

		resp := r.roundTrip(req)
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

// ServeTLSConn handles a single MITM'd TLS connection, supporting keep-alive.
// Exported for use by mode-specific ingress (FD mode).
func (r *Relay) ServeTLSConn(conn net.Conn) {
	defer conn.Close()

	tlsConn := tls.Server(conn, &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return r.signHost(hello.ServerName)
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
		if req.URL.Host == "" {
			req.URL.Host = host
		}
		if req.URL.Scheme == "" {
			req.URL.Scheme = "https"
		}
		req.RequestURI = ""

		resp := r.roundTrip(req)
		keepAlive := shouldKeepAlive(req, resp)
		if !keepAlive {
			resp.Header.Set("Connection", "close")
		}
		resp.Write(tlsConn) //nolint:errcheck
		resp.Body.Close()
		if !keepAlive {
			return
		}
	}
}

func (r *Relay) roundTrip(req *http.Request) *http.Response {
	req, interceptResp := r.onRequest(req)
	if interceptResp != nil {
		return interceptResp
	}

	req.RequestURI = ""
	resp, err := r.transport.RoundTrip(req)
	if err != nil {
		log.Printf("upstream error: %v", err)
		resp = &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     http.Header{"Content-Type": {"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("upstream unreachable\n")),
		}
	}

	resp = r.onResponse(resp, req)
	return resp
}

func shouldKeepAlive(req *http.Request, resp *http.Response) bool {
	if req.Close {
		return false
	}
	if resp.Header.Get("Connection") == "close" {
		return false
	}
	if req.ProtoAtLeast(1, 1) {
		return true
	}
	return req.Header.Get("Connection") == "keep-alive"
}

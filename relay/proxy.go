package relay

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
	"time"
)

var transport = &http.Transport{
	TLSClientConfig:     &tls.Config{},
	MaxIdleConnsPerHost: 16,
	IdleConnTimeout:     90 * time.Second,
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

func (r *Relay) serveHTTPConn(conn net.Conn) {
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

func (r *Relay) serveTLSConn(conn net.Conn) {
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

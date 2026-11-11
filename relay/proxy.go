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
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/miekg/dns"
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

	// DisableCompression: the guest's request goes upstream as sent and its
	// response comes back as sent. Otherwise, for a guest that sends no
	// Accept-Encoding, the transport asks for gzip and decompresses, and the
	// response it returns has no length and is written with no framing, so
	// the guest never sees the body end on a kept-alive connection.
	t := &http.Transport{
		TLSClientConfig:     tlsCfg,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	if r.cfg.SSRF || r.cfg.BuildLink.IsValid() {
		t.DialContext = r.safeDialContext
	} else {
		t.DialContext = r.upstreamDialContext
	}
	r.transport = t
}

// upstreamDialContext dials addr with its host resolved through the relay's
// resolver list, with no filter: the plain transport's dialer and a direct
// CollectorURL's.
func (r *Relay) upstreamDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := r.lookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	return dialFirst(ctx, network, host, port, ips)
}

// dialFirst dials each of ips on port in turn and returns the first
// connection.
func dialFirst(
	ctx context.Context, network, host, port string, ips []net.IPAddr,
) (net.Conn, error) {
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

// lookupIPAddr resolves host through the relay's resolver list only, the same
// forwarding path the guest's DNS queries take (forwardDNS: the list in
// order, the next entry on a per-query timeout). An IP literal is returned as
// is. It is not a net.Resolver with a custom Dial because Go's resolver
// retries across the host's own resolv.conf servers and attempts on its own
// deadline, which would ask the list again from the top instead of in order.
func (r *Relay) lookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	type result struct {
		ips []net.IPAddr
		err error
	}
	qtypes := []uint16{dns.TypeA, dns.TypeAAAA}
	results := make(chan result, len(qtypes))
	for _, qt := range qtypes {
		go func() {
			ips, err := r.lookupType(ctx, host, qt)
			results <- result{ips, err}
		}()
	}
	var ips []net.IPAddr
	var firstErr error
	for range qtypes {
		res := <-results
		ips = append(ips, res.ips...)
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}
	if len(ips) == 0 {
		if firstErr == nil {
			firstErr = fmt.Errorf("lookup %s: no such host", host)
		}
		return nil, firstErr
	}
	// IPv4 first, so a v4-only upstream path dials what it can reach first.
	slices.SortStableFunc(ips, func(a, b net.IPAddr) int {
		return boolInt(a.IP.To4() == nil) - boolInt(b.IP.To4() == nil)
	})
	return ips, nil
}

// lookupType asks the resolver list for host's records of type qt.
func (r *Relay) lookupType(ctx context.Context, host string, qt uint16) ([]net.IPAddr, error) {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(host), qt)
	resp := r.forwardDNS(ctx, r.dnsClient(), req)
	if resp == nil {
		return nil, fmt.Errorf("lookup %s: no upstream resolver answered", host)
	}
	if resp.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("lookup %s: %s", host, dns.RcodeToString[resp.Rcode])
	}
	var ips []net.IPAddr
	for _, rr := range resp.Answer {
		switch rec := rr.(type) {
		case *dns.A:
			ips = append(ips, net.IPAddr{IP: rec.A})
		case *dns.AAAA:
			ips = append(ips, net.IPAddr{IP: rec.AAAA})
		}
	}
	return ips, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *Relay) safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := r.lookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		if r.blockedDial(ip.IP) {
			return nil, fmt.Errorf(
				"blocked: connection to %s (%s) denied", host, ip.IP)

		}
	}

	return dialFirst(ctx, network, host, port, ips)
}

// blockedDial reports whether the upstream dialer refuses ip: anything inside
// the build link in every mode that sets it, and isBlockedIP's set when SSRF
// is on.
func (r *Relay) blockedDial(ip net.IP) bool {
	if a, ok := netip.AddrFromSlice(ip); ok && r.cfg.BuildLink.IsValid() &&
		r.cfg.BuildLink.Contains(a.Unmap()) {
		return true
	}
	return r.cfg.SSRF && isBlockedIP(ip)
}

func isBlockedIP(ip net.IP) bool {
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
	return false
}

// The CRL every leaf names, served on the reserved host by handleCACRLGet.
const (
	caCRLPath = "/ca.crl"
	caCRLURL  = "http://artifacts" + caCRLPath
)

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
		// Schannel fails a leaf with nowhere to check revocation.
		CRLDistributionPoints: []string{caCRLURL},
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

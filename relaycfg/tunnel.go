package relaycfg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// acceptTunnel (relay side) hijacks an authorized POST /v1/tunnel, switches
// it to a raw connection, and returns an HTTP/2 client over it: the relay
// issues its collector requests on a connection the host opened.
func acceptTunnel(w http.ResponseWriter) (http.RoundTripper, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "tunnel unsupported", http.StatusInternalServerError)
		return nil, fmt.Errorf("relaycfg: response writer cannot hijack")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
		"Connection: Upgrade\r\nUpgrade: " + tunnelUpgrade + "\r\n\r\n")
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	cc, err := (&http2.Transport{AllowHTTP: true}).NewClientConn(
		&bufferedConn{conn, rw.Reader})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return cc, nil
}

// OpenTunnel (orchestrator side) connects to the relay's claim endpoint at
// addr, retrying until the relay listens or ctx ends, upgrades the connection,
// and serves handler (the collector) to the relay over it as HTTP/2. The
// returned channel is closed when the tunnel ends; closing the returned conn
// tears it down.
func OpenTunnel(
	ctx context.Context, addr, token string, handler http.Handler,
) (net.Conn, <-chan struct{}, error) {
	var lastErr error
	for {
		conn, err := dialTunnel(ctx, addr, token)
		if err == nil {
			done := make(chan struct{})
			go func() {
				(&http2.Server{}).ServeConn(
					conn, &http2.ServeConnOpts{Handler: handler})
				close(done)
			}()
			return conn, done, nil
		}
		if errors.Is(err, ErrRejected) {
			return nil, nil, err
		}
		lastErr = err
		select {
		case <-time.After(250 * time.Millisecond):
			continue
		case <-ctx.Done():
		}
		return nil, nil, fmt.Errorf(
			"relaycfg: relay at %s never accepted the tunnel: %w (last error: %v)",
			addr, ctx.Err(), lastErr)
	}
}

func dialTunnel(ctx context.Context, addr, token string) (net.Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	auth := ""
	if token != "" {
		auth = "Authorization: Bearer " + token + "\r\n"
	}
	req := "POST " + tunnelPath + " HTTP/1.1\r\nHost: relay\r\n" + auth +
		"Connection: Upgrade\r\nUpgrade: " + tunnelUpgrade +
		"\r\nContent-Length: 0\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, err // e.g. a forwarded port with nothing behind it yet
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		conn.Close()
		return nil, fmt.Errorf("%w: tunnel HTTP %d: %s", ErrRejected, resp.StatusCode, msg)
	}
	_ = conn.SetDeadline(time.Time{})
	return &bufferedConn{conn, br}, nil
}

// bufferedConn reads through r first, so bytes buffered while parsing the
// upgrade handshake are not lost to the HTTP/2 layer.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

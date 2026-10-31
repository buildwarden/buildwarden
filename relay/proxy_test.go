package relay

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/collector"
)

// gzipUpstream serves page, gzipped only when the request asks for gzip, as
// a CDN does for text.
func gzipUpstream(t *testing.T, page string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = io.WriteString(w, page)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, page)
		_ = zw.Close()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// rawGet sends one keep-alive GET over a fresh connection to the relay's HTTP
// listener, as curl or PowerShell would (no Go client compression), and reads
// the body to its end. A response the client cannot see the end of fails
// here on the read deadline instead of hanging.
func rawGet(t *testing.T, r *Relay, host, acceptEncoding string) (*http.Response, []byte) {
	t.Helper()
	conn, err := net.Dial("tcp", r.cfg.HTTPListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := "GET /simple/six/ HTTP/1.1\r\nHost: " + host + "\r\n"
	if acceptEncoding != "" {
		req += "Accept-Encoding: " + acceptEncoding + "\r\n"
	}
	if _, err := io.WriteString(conn, req+"\r\n"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("response body never ended (Content-Length %d, "+
			"Transfer-Encoding %v, Close %v): %v",
			resp.ContentLength, resp.TransferEncoding, resp.Close, err)
	}
	return resp, body
}

// The relay passes the upstream response through as sent: a client that
// asks for no encoding gets a plain, framed body, and one that asks for gzip
// gets the upstream's gzip bytes.
func TestRelayPassesResponseEncodingThrough(t *testing.T) {
	page := strings.Repeat("<a href=\"six-1.17.0.tar.gz\">six</a>\n", 400)
	up := gzipUpstream(t, page)
	host := strings.TrimPrefix(up.URL, "http://")
	r, _, _, _ := startTestRelay(t, collector.Config{}, "")

	resp, body := rawGet(t, r, host, "")
	if string(body) != page {
		t.Fatalf("plain body = %d bytes, want the %d-byte page", len(body), len(page))
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q for a client that asked for none", enc)
	}

	resp, body = rawGet(t, r, host, "gzip")
	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("gzip body: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil || string(plain) != page {
		t.Fatalf("gunzipped body = %d bytes (%v), want the %d-byte page",
			len(plain), err, len(page))
	}
}

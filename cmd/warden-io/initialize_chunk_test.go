package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPostBuildOutputChunkGuards(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	if postBuildOutputChunk(client, "", []byte("x")) {
		t.Error("empty gateway must return false")
	}
	if postBuildOutputChunk(client, "127.0.0.1", nil) {
		t.Error("empty data must return false")
	}
}

// TestPostBuildOutputChunkDelivers verifies a chunk is POSTed verbatim and a
// non-200 is reported as failure. postBuildOutputChunk builds
// http://<gateway>:8300/v1/output, so the test server is bound to that exact
// port; if it is already in use the delivery leg is skipped rather than failed.
func TestPostBuildOutputChunkDelivers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:8300")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.1:8300 (in use?): %v", err)
	}
	var got strings.Builder
	status := http.StatusOK
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Write(b)
		w.WriteHeader(status)
	})}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	if !postBuildOutputChunk(client, "127.0.0.1", []byte("hello\nworld\n")) {
		t.Fatal("expected success delivering a chunk")
	}
	if got.String() != "hello\nworld\n" {
		t.Errorf("server got %q, want %q", got.String(), "hello\nworld\n")
	}
	status = http.StatusInternalServerError
	if postBuildOutputChunk(client, "127.0.0.1", []byte("more")) {
		t.Error("expected failure on non-200")
	}
}

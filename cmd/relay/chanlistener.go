package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
)

// readOnlyControlListener is a channel-backed net.Listener that serves
// only read-only control plane endpoints (health check, CA cert).
// No write operations are exposed to the build VM.
type readOnlyControlListener struct {
	ch   chan net.Conn
	addr net.Addr
}

func newReadOnlyControlListener(ip net.IP) *readOnlyControlListener {
	ln := &readOnlyControlListener{
		ch:   make(chan net.Conn, 16),
		addr: &net.TCPAddr{IP: ip, Port: 8300},
	}
	go ln.serve()
	return ln
}

func (cl *readOnlyControlListener) deliver(conn net.Conn) {
	select {
	case cl.ch <- conn:
	default:
		conn.Close()
	}
}

func (cl *readOnlyControlListener) Accept() (net.Conn, error) {
	conn, ok := <-cl.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (cl *readOnlyControlListener) Close() error {
	close(cl.ch)
	return nil
}

func (cl *readOnlyControlListener) Addr() net.Addr {
	return cl.addr
}

func (cl *readOnlyControlListener) serve() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	mux.HandleFunc("/ca.pem", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(CA_CERT) //nolint:errcheck
	})
	mux.HandleFunc("/v1/output", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		remaining := maxOutputBytes - outputBytesWritten.Load()
		if remaining <= 0 {
			http.Error(w, "output limit exceeded", http.StatusRequestEntityTooLarge)
			return
		}
		// Stream build output to stderr for visibility + cap it.
		n, _ := io.Copy(os.Stderr, io.LimitReader(r.Body, remaining))
		outputBytesWritten.Add(n)
		w.WriteHeader(200)
	})
	mux.HandleFunc("/v1/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		code := r.URL.Query().Get("code")
		log.Printf("relay: build complete (code=%s)", code)
		WriteExitCode(0)
		if code != "" && code != "0" {
			var c int
			fmt.Sscanf(code, "%d", &c) //nolint:errcheck
			WriteExitCode(c)
		}
		w.WriteHeader(200)
	})
	(&http.Server{Handler: mux}).Serve(cl) //nolint:errcheck
}

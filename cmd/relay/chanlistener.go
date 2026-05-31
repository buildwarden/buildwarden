package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"warden/relay"
)

// chanListener is a channel-backed net.Listener for the FD mode control plane.
type chanListener struct {
	ch   chan net.Conn
	addr net.Addr
}

func newChanListener(ip net.IP, port int) *chanListener {
	return &chanListener{
		ch:   make(chan net.Conn, 16),
		addr: &net.TCPAddr{IP: ip, Port: port},
	}
}

func (cl *chanListener) deliver(conn net.Conn) {
	select {
	case cl.ch <- conn:
	default:
		conn.Close()
	}
}

func (cl *chanListener) Accept() (net.Conn, error) {
	conn, ok := <-cl.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (cl *chanListener) Close() error {
	close(cl.ch)
	return nil
}

func (cl *chanListener) Addr() net.Addr {
	return cl.addr
}

// serveReadOnlyControlPlane serves health, CA, and output endpoints through
// the channel listener. In FD mode, the control plane is accessed through
// the netstack rather than a separate port binding.
func serveReadOnlyControlPlane(ln net.Listener, r *relay.Relay) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	mux.HandleFunc("/ca.pem", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(r.CACert()) //nolint:errcheck
	})
	mux.HandleFunc("/v1/output", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		n, _ := io.Copy(os.Stderr, io.LimitReader(req.Body, 256*1024*1024))
		_ = n
		w.WriteHeader(200)
	})
	mux.HandleFunc("/v1/complete", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		code := req.URL.Query().Get("code")
		log.Printf("relay: build complete (code=%s)", code)
		w.WriteHeader(200)
	})
	(&http.Server{Handler: mux}).Serve(ln) //nolint:errcheck
}

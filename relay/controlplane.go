package relay

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

func (r *Relay) runControlPlane() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok")) //nolint:errcheck
	})

	mux.HandleFunc("/ca.pem", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(r.caCert) //nolint:errcheck
	})

	mux.HandleFunc("/v1/output", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		remaining := maxOutputBytes - r.outputBytesWritten.Load()
		if remaining <= 0 {
			http.Error(w, "output limit exceeded", http.StatusRequestEntityTooLarge)
			return
		}

		// Precedence: an explicit OutputWriter override wins (used by drivers
		// and tests); otherwise route through the output sink (local file, or
		// a streamed POST /v1/output to the collector).
		if r.cfg.OutputWriter != nil {
			n, _ := io.Copy(r.cfg.OutputWriter, io.LimitReader(req.Body, remaining))
			r.outputBytesWritten.Add(n)
			w.WriteHeader(200)
			return
		}

		dst, finish, err := r.sink.OutputWriter()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		n, _ := io.Copy(dst, io.LimitReader(req.Body, remaining))
		r.outputBytesWritten.Add(n)
		if err := finish(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(200)
	})

	mux.HandleFunc("/v1/complete", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		code := req.URL.Query().Get("code")
		log.Printf("relay: build complete (code=%s)", code)
		exitCode := 0
		if code != "" && code != "0" {
			var c int
			_, _ = fmt.Sscanf(code, "%d", &c)
			exitCode = c
		}
		r.writeExitCode(exitCode)
		w.WriteHeader(200)
	})

	var ln net.Listener
	if r.cfg.ControlListener != nil {
		ln = r.cfg.ControlListener
	} else {
		var err error
		ln, err = net.Listen("tcp", ":8300")
		if err != nil {
			return err
		}
	}
	return (&http.Server{Handler: mux}).Serve(ln)
}

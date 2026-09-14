// Command collector receives build outputs (artifacts, ledger, build log)
// streamed by a relay's httpSink and writes them into an output directory.
//
// It is the standalone counterpart to cmd/relay: a reusable primitive a
// third-party orchestrator can run to collect a build's outputs without
// adopting the rest of BuildWarden. Drivers embed the collector package
// in-process instead of spawning this binary.
//
// The bearer token is read from WARDEN_COLLECTOR_TOKEN (not a flag, so it does
// not appear in the process list). Bind to loopback (default) or an isolated
// interface only.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/buildwarden/buildwarden/collector"
)

func main() {
	dir := flag.String("dir", "", "output directory (required)")
	addr := flag.String("addr", "127.0.0.1:8400", "listen address (bind loopback or an isolated interface only)")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "collector: --dir is required")
		os.Exit(2)
	}

	token := os.Getenv("WARDEN_COLLECTOR_TOKEN")
	if token == "" {
		log.Printf("collector: WARNING no WARDEN_COLLECTOR_TOKEN set; auth disabled")
	}

	c, err := collector.New(collector.Config{
		OutputDir: *dir,
		Token:     token,
		Logf:      log.Printf,
	})
	if err != nil {
		log.Fatalf("collector: %v", err)
	}

	log.Printf("collector: listening on %s -> %s", *addr, *dir)
	if err := (&http.Server{Addr: *addr, Handler: c.Handler()}).ListenAndServe(); err != nil {
		log.Fatalf("collector: %v", err)
	}
}

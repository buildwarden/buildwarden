// Command collector is the standalone host side of a build: it claims a relay
// that is already running (as a container, VM or host process, started by
// any orchestrator), serves the build's inputs to it, lands every output in an
// output directory, and exits with the build's result.
//
// It is the reusable counterpart to cmd/relay for third-party orchestrators
// that want the ledger without adopting the rest of BuildWarden; warden's own
// drivers run the same code in-process (driver.Session). Like every driver it
// only ever connects TO the relay's claim port, so it needs no listener.
//
// The relay's claim token, if it was started with one, is read from
// WARDEN_CLAIM_TOKEN (not a flag, so it does not appear in the process list).
//
//	collector --relay 127.0.0.1:8301 --dir out --context . --script build.sh
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

func main() {
	relay := flag.String("relay", "", "host:port of the relay's claim endpoint (required)")
	dir := flag.String("dir", "", "output directory (required)")
	ctxDir := flag.String("context", "", "build context served to the guest")
	script := flag.String("script", "", "build script served at http://artifacts/build-script")
	capture := flag.String("capture", "", "capture payloads (none, headers, bodies, all)")
	upstreamCAs := flag.String("upstream-ca", "", "extra upstream CA files/dirs, comma-separated")
	quiet := flag.Bool("quiet", false, "do not echo the guest build console")
	flag.Parse()

	if *relay == "" || *dir == "" {
		fmt.Fprintln(os.Stderr, "collector: --relay and --dir are required")
		os.Exit(2)
	}
	req := &driver.BuildRequest{
		OutputDir:        *dir,
		ContextDir:       *ctxDir,
		Script:           *script,
		CaptureMode:      *capture,
		Quiet:            *quiet,
		UpstreamSystemCA: true,
	}
	if *upstreamCAs != "" {
		req.UpstreamCACerts = strings.Split(*upstreamCAs, ",")
	}
	if err := run(*relay, req); err != nil {
		fmt.Fprintf(os.Stderr, "collector: %v\n", err)
		os.Exit(1)
	}
}

func run(relayAddr string, req *driver.BuildRequest) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   req.OutputDir,
		ContextDir:  req.ContextDir,
		BuildScript: req.Script,
		OutputTee:   driver.ConsoleTee(req),
	})
	if err != nil {
		return err
	}
	defer sess.Close()

	id, err := sess.ClaimRelay(ctx, relayAddr, os.Getenv(relaycfg.TokenEnv), req, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "collector: relay claimed (ledger key %s); waiting for the build\n",
		id.LedgerPublicKey)
	if err := sess.Wait(ctx); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "collector: build complete -> %s\n", req.OutputDir)
	return nil
}

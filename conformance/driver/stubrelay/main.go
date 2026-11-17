// Command stubrelay is the reference fake driver's relay: it accepts one
// claim by the claim protocol (relaycfg), reaches the collector the claim
// names, and plays the build guest by the directives of the build script
// the collector serves:
//
//	# stub: output <text>   a line of build output
//	# stub: sleep <d>       a pause, a Go duration
//	# stub: exit <n>        completion with exit code n
//	# stub: vanish          the relay ends at once, with no completion
//
// A script that ends without exit completes with 0. It sends a heartbeat
// every second until it completes, writes no ledger, and stays up after the
// completion until it is killed, as a relay does.
//
//	stubrelay --listen 127.0.0.1:0
//
// It prints "listening <address>" on stdout once it listens, and takes the
// claim token from WARDEN_CLAIM_TOKEN.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/buildwarden/buildwarden/relaycfg"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "the claim address")
	flag.Parse()
	log.SetPrefix("stubrelay: ")
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("listening %s\n", ln.Addr())
	done := make(chan struct{})
	err = relaycfg.Serve(ln, os.Getenv(relaycfg.TokenEnv),
		func(cfg relaycfg.Config, rt http.RoundTripper) (relaycfg.Identity, error) {
			g := &guest{cfg: cfg, client: &http.Client{Transport: rt}}
			if rt == nil {
				g.client = http.DefaultClient
			}
			id, err := identity()
			if err != nil {
				return relaycfg.Identity{}, err
			}
			go func() {
				g.play(context.Background())
				close(done)
			}()
			return id, nil
		})
	if err != nil {
		log.Fatal(err)
	}
	log.Print("claimed")
	<-done
	// A relay stays up until it is torn down.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

// identity is a fresh CA certificate and ledger key, as a relay reports
// them; the stub signs nothing with either.
func identity() (relaycfg.Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return relaycfg.Identity{}, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1),
		Subject:   pkix.Name{CommonName: "stubrelay CA"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return relaycfg.Identity{}, err
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return relaycfg.Identity{}, err
	}
	return relaycfg.Identity{
		CACert: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE",
			Bytes: der})),
		LedgerPublicKey: hex.EncodeToString(pub)}, nil
}

// guest plays the build environment against the collector.
type guest struct {
	cfg    relaycfg.Config
	client *http.Client
}

func (g *guest) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, g.cfg.CollectorURL+path,
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if g.cfg.CollectorToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.CollectorToken)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, data)
	}
	return data, nil
}

// play fetches the build script and follows its directives.
func (g *guest) play(ctx context.Context) {
	script, err := g.do(ctx, http.MethodGet, "/v1/build-script", nil)
	if err != nil {
		log.Printf("build script: %v", err)
		script = nil
	}
	if _, err := g.do(ctx, http.MethodPost, "/v1/ready", nil); err != nil {
		log.Printf("ready: %v", err)
	}
	beat, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		for {
			_, _ = g.do(beat, http.MethodPost, "/v1/heartbeat", nil)
			select {
			case <-beat.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	code := 0
	sc := bufio.NewScanner(bytes.NewReader(script))
	for sc.Scan() {
		directive, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "# stub:")
		if !ok {
			continue
		}
		verb, arg, _ := strings.Cut(strings.TrimSpace(directive), " ")
		switch verb {
		case "output":
			_, _ = g.do(ctx, http.MethodPost, "/v1/output", []byte(arg+"\n"))
		case "sleep":
			d, _ := time.ParseDuration(arg)
			time.Sleep(d)
		case "vanish":
			log.Print("vanishing")
			os.Exit(3)
		case "exit":
			code, _ = strconv.Atoi(arg)
		}
		if verb == "exit" {
			break
		}
	}
	body, _ := json.Marshal(map[string]any{"exit_code": code})
	if _, err := g.do(ctx, http.MethodPost, "/v1/complete", body); err != nil {
		log.Printf("complete: %v", err)
	}
}

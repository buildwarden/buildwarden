package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/buildwarden/buildwarden/relay"
	"github.com/buildwarden/buildwarden/relaycfg"
)

func main() {
	os.Exit(run())
}

// The relay binary boots unconfigured in every mode: it binds its guest-facing
// listeners, then waits for the orchestrator to claim it (relaycfg) with the
// build's collector and settings. Modes differ only in how guest traffic
// arrives:
//
//	host       Ethernet frames on an inherited socketpair fd (vz), SSRF on
//	vm         the relay is init in its own VM (qemu, hyperv)
//	container  sidecar container on an isolated network (container driver)
func run() int {
	// Prevent key material from appearing in core dumps (platform-specific).
	disableCoreDumps()

	mode := flag.String("mode", "",
		"Relay mode: 'host', 'vm', or 'container' (auto-detected if empty)")
	fdNum := flag.Int("fd", 3,
		"File descriptor number for host mode fd ingress")
	subnet := flag.String("subnet", "10.100.0.0/30",
		"Subnet for host mode (gateway=.1, guest=.2)")
	claimAddr := flag.String("claim-addr", ":"+strconv.Itoa(relaycfg.DefaultPort),
		"Address to accept the orchestrator's claim on")
	flag.Parse()

	if *mode == "" {
		*mode = "container"
		if isUnixSocket(*fdNum) {
			*mode = "host"
		}
	}

	switch *mode {
	case "host":
		return runHostMode(*fdNum, *subnet, *claimAddr)
	case "vm", "container":
		// Binds the standard ports directly. No SSRF filter: isolation comes
		// from the relay VM's iptables or the container's network namespace.
		return serveClaim(*claimAddr, relay.Config{}, nil)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		return 1
	}
}

// serveClaim waits for the single claim, starts the relay with base merged
// with the claimed config, and then runs it until it stops. cleanup, if set,
// runs once the relay has exited.
func serveClaim(claimAddr string, base relay.Config, cleanup func()) int {
	if cleanup != nil {
		defer cleanup()
	}
	ln, err := net.Listen("tcp", claimAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "relay: claim listener: %v\n", err)
		return 1
	}
	log.Printf("relay: awaiting claim on %s", ln.Addr())

	var r *relay.Relay
	err = relaycfg.Serve(ln, os.Getenv(relaycfg.TokenEnv),
		func(c relaycfg.Config, transport http.RoundTripper) (relaycfg.Identity, error) {
			cfg := applyClaim(base, c)
			cfg.CollectorTransport = transport
			started, err := relay.Start(cfg)
			if err != nil {
				return relaycfg.Identity{}, err
			}
			r = started
			return relaycfg.Identity{
				CACert:          string(r.CACert()),
				LedgerPublicKey: r.LedgerPublicKeyHex(),
			}, nil
		})
	if err != nil {
		fmt.Fprintf(os.Stderr, "relay: %v\n", err)
		return 1
	}
	log.Printf("relay: claimed")

	if err := r.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		return 1
	}
	return 0
}

// applyClaim overlays the orchestrator's per-build config on the mode's base.
func applyClaim(base relay.Config, c relaycfg.Config) relay.Config {
	cfg := base
	cfg.CollectorURL = c.CollectorURL
	cfg.CollectorToken = c.CollectorToken
	cfg.CaptureMode = c.CaptureMode
	cfg.Environment = c.Environment
	for _, pem := range c.UpstreamCACerts {
		cfg.UpstreamCACerts = append(cfg.UpstreamCACerts, []byte(pem))
	}
	cfg.UpstreamSystemCA = c.UpstreamSystemCA
	return cfg
}

// parseSubnet parses a CIDR and returns gateway (.1) and guest (.2) IPs.
func parseSubnet(cidr string) (gw, guest net.IP, mask net.IPMask, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, nil, err
	}

	_, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, nil, nil, fmt.Errorf("only IPv4 supported")
	}

	base := ip.Mask(ipNet.Mask).To4()
	if base == nil {
		return nil, nil, nil, fmt.Errorf("invalid IPv4 address")
	}

	g := make(net.IP, 4)
	copy(g, base)
	addToIP(g, 1)

	h := make(net.IP, 4)
	copy(h, base)
	addToIP(h, 2)

	return g, h, ipNet.Mask, nil
}

func addToIP(ip net.IP, n int) {
	v := int(ip[0])<<24 | int(ip[1])<<16 | int(ip[2])<<8 | int(ip[3])
	v += n
	ip[0] = byte(v >> 24)
	ip[1] = byte(v >> 16)
	ip[2] = byte(v >> 8)
	ip[3] = byte(v)
}

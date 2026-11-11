package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
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
//	vm         the relay is init in its own VM (qemu, firecracker, hyperv)
//	container  sidecar container on an isolated network (container driver)
//
// Each mode has a start step that runs inside the claim, before the relay
// starts and before the claim is answered, and configures the build segment
// from the claim's build_link (see relaycfg's package doc).
func run() int {
	// Prevent key material from appearing in core dumps (platform-specific).
	disableCoreDumps()

	mode := flag.String("mode", "",
		"Relay mode: 'host', 'vm', or 'container' (auto-detected if empty)")
	fdNum := flag.Int("fd", 3,
		"File descriptor number for host mode fd ingress")
	subnet := flag.String("subnet", "10.100.0.0/30",
		"Host mode's segment when the claim carries no build_link (gateway=.1, guest=.2)")
	buildIf := flag.String("build-if", "",
		"vm mode: the build interface (default: the first non-lo entry of /sys/class/net)")
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
	case "vm":
		// Binds the standard ports directly. No SSRF filter: isolation comes
		// from the relay VM's iptables; the build link is still blocked.
		iface, err := openBuildIface(*buildIf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "relay: build interface: %v\n", err)
			return 1
		}
		return serveClaim(*claimAddr, vmLink{iface: iface, serveDHCP: openVMDHCP}.start)
	case "container":
		return serveClaim(*claimAddr, containerStart)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		return 1
	}
}

// startStep is a mode's part of the claim: it configures the build segment
// from the claim and returns the relay.Config base the claim is overlaid on,
// and a cleanup to run once the relay exits (nil for none). It runs before
// relay.Start and so before the claim is answered. An error wrapping
// relaycfg.ErrRefused refuses the claim and leaves the relay claimable; a
// step that fails cleans up after itself.
type startStep func(relaycfg.Config) (relay.Config, func(), error)

// containerStart is container mode's step. The container runtime assigns the
// relay's address, which the relay has no capability to change, so a claim
// that names a build link is refused.
func containerStart(c relaycfg.Config) (relay.Config, func(), error) {
	if c.BuildLink != nil {
		return relay.Config{}, nil, fmt.Errorf(
			"%w: build_link is not supported in container mode "+
				"(the container runtime assigns the relay's address)",
			relaycfg.ErrRefused)
	}
	return relay.Config{}, nil, nil
}

// resolvConfPath is the relay's platform resolver configuration, the source
// of the absent-field default.
const resolvConfPath = "/etc/resolv.conf"

// upstreamResolvers returns the relay's resolver list and its source for
// claim c on the mode's base: the claim's upstream_dns when it carries one
// (validated by relaycfg.Serve), used exclusively; otherwise the usable
// nameservers of the relay's own resolv.conf, read by readConf. No usable
// server is an error wrapping relaycfg.ErrRefused that names upstream_dns, so
// the claim is refused and the relay stays claimable.
func upstreamResolvers(
	c relaycfg.Config, base relay.Config, readConf func() ([]byte, error),
) ([]netip.Addr, string, error) {
	if c.UpstreamDNS != nil {
		list, err := relaycfg.ParseUpstreamDNS(c.UpstreamDNS, c.BuildLink)
		if err != nil {
			return nil, "", fmt.Errorf("%w: upstream_dns: %w",
				relaycfg.ErrRefused, err)
		}
		return list, relay.SourceClaim, nil
	}
	conf, err := readConf()
	if err != nil {
		return nil, "", fmt.Errorf(
			"%w: upstream_dns is absent and the relay's default cannot be read: %w; "+
				"send upstream_dns in the claim", relaycfg.ErrRefused, err)
	}
	list := relay.ResolvConfServers(string(conf), base.SelfIP, base.BuildLink)
	if len(list) == 0 {
		return nil, "", fmt.Errorf(
			"%w: upstream_dns is absent and %s names no usable nameserver "+
				"(the relay's own address, 127.0.0.1 and the build link "+
				"are excluded); send upstream_dns in the claim",
			relaycfg.ErrRefused, resolvConfPath)
	}
	return list, relay.SourceResolvConf, nil
}

func readResolvConf() ([]byte, error) { return os.ReadFile(resolvConfPath) }

// serveClaim waits for the single claim, runs the mode's step, resolves the
// relay's upstream resolvers (upstreamResolvers), starts the relay with the
// step's base merged with the claimed config, and then runs it until it
// stops.
func serveClaim(claimAddr string, step startStep) int {
	var cleanup func()
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()
	ln, err := net.Listen("tcp", claimAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "relay: claim listener: %v\n", err)
		return 1
	}
	log.Printf("relay: awaiting claim on %s", ln.Addr())

	var r *relay.Relay
	err = relaycfg.Serve(ln, os.Getenv(relaycfg.TokenEnv),
		func(c relaycfg.Config, transport http.RoundTripper) (relaycfg.Identity, error) {
			base, done, err := step(c)
			if err != nil {
				return relaycfg.Identity{}, err
			}
			if base.SelfIP == nil {
				// Container mode: the runtime's address, which the
				// resolv.conf default excludes and Start would detect anyway.
				if base.SelfIP, err = relay.DetectSelfIP(); err != nil {
					return relaycfg.Identity{},
						fmt.Errorf("detecting self IP: %w", err)
				}
			}
			servers, source, err := upstreamResolvers(c, base, readResolvConf)
			if err != nil {
				if done != nil {
					done()
				}
				return relaycfg.Identity{}, err
			}
			cfg := applyClaim(base, c)
			cfg.UpstreamDNS, cfg.UpstreamDNSSource = servers, source
			cfg.CollectorTransport = transport
			started, err := relay.Start(cfg)
			if err != nil {
				if done != nil {
					done()
				}
				return relaycfg.Identity{}, err
			}
			r, cleanup = started, done
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

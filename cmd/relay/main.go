package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/fxamacker/cbor/v2"
)

func main() {
	os.Exit(run())
}

func run() int {
	ingress := flag.String("ingress", "interface",
		"Network ingress mode: 'interface' (bind ports) or 'fd' (socketpair)")
	fdNum := flag.Int("fd", 3,
		"File descriptor number for fd ingress mode")
	subnet := flag.String("subnet", "10.100.0.0/30",
		"Subnet for fd ingress (gateway=.1, guest=.2)")
	flag.Parse()

	outDir := os.Getenv("LEDGER_DIR")
	if outDir == "" {
		outDir = "/ledger"
	}

	if err := os.MkdirAll(filepath.Join(outDir, "payloads"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating ledger directory: %v\n", err)
		return 1
	}

	if mode := os.Getenv("CAPTURE_MODE"); mode != "" && mode != "none" {
		if err := os.MkdirAll(filepath.Join(outDir, "captures"), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating captures directory: %v\n", err)
			return 1
		}
		SetCaptureMode(mode)
	}

	ctxDir := os.Getenv("CONTEXT_DIR")
	if ctxDir == "" {
		ctxDir = "/context"
	}
	SetContextDir(ctxDir)

	sigDir := os.Getenv("SIGNAL_DIR")
	if sigDir != "" {
		_ = os.MkdirAll(sigDir, 0755)
		SetSignalDir(sigDir)
	}

	ledgerFile, err := os.Create(filepath.Join(outDir, "ledger"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating ledger file: %v\n", err)
		return 1
	}
	defer ledgerFile.Close()

	l, err := NewLedger(LedgerConfig{
		Writer:      ledgerFile,
		Environment: map[string]any{"type": "container"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing ledger: %v\n", err)
		return 1
	}
	SetLedger(l)
	SetOutDir(outDir)

	if err := recordEnvironmentFromVolume(outDir, l); err != nil {
		fmt.Fprintf(os.Stderr, "error recording environment: %v\n", err)
		return 1
	}

	if err := GenerateCA(); err != nil {
		fmt.Fprintf(os.Stderr, "error generating CA: %v\n", err)
		return 1
	}

	caPath := filepath.Join(outDir, "ca.cert.pem")
	if err := os.WriteFile(caPath, CA_CERT, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing CA cert: %v\n", err)
		return 1
	}

	switch *ingress {
	case "fd":
		return runFDIngress(*fdNum, *subnet, outDir, l)
	case "interface":
		return runInterfaceIngress(outDir, l)
	default:
		fmt.Fprintf(os.Stderr, "unknown ingress mode: %s\n", *ingress)
		return 1
	}
}

// runFDIngress starts the relay using a userspace network stack reading
// Ethernet frames from an inherited socketpair FD.
func runFDIngress(fd int, subnetCIDR string, outDir string, l *Ledger) int {
	gwIP, guestIP, mask, err := parseSubnet(subnetCIDR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid subnet: %v\n", err)
		return 1
	}

	selfIP = gwIP
	blockedSelfIP = gwIP
	DetectUpstreamDNS()

	// Channel-based listener for the control plane (port 8300).
	// Exposes only read-only endpoints (health, CA) — no write operations.
	ctrlLn := newReadOnlyControlListener(gwIP)

	ing, err := NewFDIngress(FDIngressConfig{
		FD:         fd,
		GatewayIP:  gwIP,
		GuestIP:    guestIP,
		SubnetMask: mask,
		ConnHandler: func(conn net.Conn, dstPort uint16) {
			switch dstPort {
			case 443:
				serveTLSConn(conn)
			case 8300:
				ctrlLn.deliver(conn)
			default:
				serveHTTPConn(conn)
			}
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating fd ingress: %v\n", err)
		return 1
	}
	defer ing.Close()

	dnsConn, err := ing.ListenUDP(53)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listening udp/53: %v\n", err)
		return 1
	}

	errs := make(chan error, 2)

	go RunHeartbeat()
	go func() { errs <- RunDnsWithConn(dnsConn) }()

	fmt.Fprintf(os.Stderr,
		"relay: fd ingress on fd %d, gateway %s, guest %s\n",
		fd, gwIP, guestIP)

	if err := <-errs; err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		l.Finish()
		return 1
	}
	return 0
}

// runInterfaceIngress starts the relay in the traditional mode, binding
// directly to network interfaces (container/VM mode).
func runInterfaceIngress(outDir string, l *Ledger) int {
	if err := DetectSelfIP(); err != nil {
		fmt.Fprintf(os.Stderr, "error detecting relay IP: %v\n", err)
		return 1
	}
	DetectUpstreamDNS()

	listenIP := selfIP
	if listenIP == nil {
		listenIP = net.IPv4zero
	}
	errs := make(chan error, 4)

	go RunHeartbeat()

	go func() { errs <- RunDns(net.TCPAddr{IP: listenIP, Port: 53}) }()
	go func() { errs <- RunHttp(net.TCPAddr{IP: listenIP, Port: 80}) }()
	go func() { errs <- RunHttps(net.TCPAddr{IP: listenIP, Port: 443}) }()
	go func() { errs <- RunControlPlane(outDir) }()

	fmt.Fprintf(os.Stderr, "relay: listening on :53/udp :80/tcp :443/tcp :8300/tcp\n")

	if err := <-errs; err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		l.Finish()
		return 1
	}
	return 0
}

// parseSubnet parses a /30 CIDR and returns gateway (.1) and guest (.2) IPs.
func parseSubnet(cidr string) (gateway, guest net.IP, mask net.IPMask, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, nil, err
	}

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, nil, nil, fmt.Errorf("only IPv4 supported")
	}
	_ = ones

	base := ip.Mask(ipNet.Mask).To4()
	if base == nil {
		return nil, nil, nil, fmt.Errorf("invalid IPv4 address")
	}

	// For any subnet, gateway = base+1, guest = base+2
	gw := make(net.IP, 4)
	copy(gw, base)
	addToIP(gw, 1)

	g := make(net.IP, 4)
	copy(g, base)
	addToIP(g, 2)

	return gw, g, ipNet.Mask, nil
}

func addToIP(ip net.IP, n int) {
	v := int(ip[0])<<24 | int(ip[1])<<16 | int(ip[2])<<8 | int(ip[3])
	v += n
	ip[0] = byte(v >> 24)
	ip[1] = byte(v >> 16)
	ip[2] = byte(v >> 8)
	ip[3] = byte(v)
}

// recordEnvironmentFromVolume reads the environment payload and metadata
// from the ledger volume and writes it as the first ledger entry. This
// MUST be the first record after the header — the relay writes it during
// startup before accepting any network traffic.
//
// Expected files:
//   - <ledgerDir>/environment/payload   (raw bytes to hash — e.g. OCI manifest)
//   - <ledgerDir>/environment/metadata  (CBOR-encoded metadata)
//
// If the environment directory does not exist, this is a no-op (the
// environment record is optional for backwards compatibility).
func recordEnvironmentFromVolume(ledgerDir string, l *Ledger) error {
	envDir := filepath.Join(ledgerDir, "environment")
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		return nil
	}

	payload, err := os.ReadFile(filepath.Join(envDir, "payload"))
	if err != nil {
		return fmt.Errorf("reading environment payload: %w", err)
	}
	if len(payload) == 0 {
		return fmt.Errorf("environment payload is empty")
	}

	metaBytes, err := os.ReadFile(filepath.Join(envDir, "metadata"))
	if err != nil {
		return fmt.Errorf("reading environment metadata: %w", err)
	}

	// Validate that metadata is valid CBOR.
	var meta map[string]any
	if err := cbor.Unmarshal(metaBytes, &meta); err != nil {
		return fmt.Errorf("invalid environment metadata CBOR: %w", err)
	}

	// Write as the first record: open + close with environment schema.
	openMeta, _ := cbor.Marshal(map[string]any{
		"type": "environment",
	})
	openSig := l.Open(schemaEnvCtr, openMeta)

	hashBlock := l.ComputeHashBlock(payload)
	l.Close(openSig, -int64(len(payload)), hashBlock, schemaEnvCtr, metaBytes)

	fmt.Fprintf(os.Stderr, "relay: environment recorded (%d bytes)\n",
		len(payload))
	return nil
}

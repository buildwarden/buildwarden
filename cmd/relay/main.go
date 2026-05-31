package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func main() {
	os.Exit(run())
}

func run() int {
	mode := flag.String("mode", "",
		"Relay mode: 'host', 'vm', or 'container' (auto-detected if empty)")
	fdNum := flag.Int("fd", 3,
		"File descriptor number for host mode fd ingress")
	subnet := flag.String("subnet", "10.100.0.0/30",
		"Subnet for host mode (gateway=.1, guest=.2)")
	flag.Parse()

	if *mode == "" {
		*mode = detectMode(*fdNum)
	}

	outDir := os.Getenv("LEDGER_DIR")
	if outDir == "" {
		outDir = "/ledger"
	}

	ctxDir := os.Getenv("CONTEXT_DIR")
	if ctxDir == "" {
		ctxDir = "/context"
	}

	sigDir := os.Getenv("SIGNAL_DIR")
	captureMode := os.Getenv("CAPTURE_MODE")

	switch *mode {
	case "host":
		return runHostMode(*fdNum, *subnet, outDir, ctxDir, sigDir, captureMode)
	case "vm":
		return runVMMode(outDir, ctxDir, sigDir, captureMode)
	case "container":
		return runContainerMode(outDir, ctxDir, captureMode)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		return 1
	}
}

// detectMode auto-detects the relay operating mode.
func detectMode(fdNum int) string {
	// Check if inherited FD exists and is an AF_UNIX socket → host mode
	if isUnixSocket(fdNum) {
		return "host"
	}

	// PID 1 + /shared/relay.env → VM mode (relay is init in its own VM)
	if os.Getpid() == 1 {
		if _, err := os.Stat("/shared/relay.env"); err == nil {
			return "vm"
		}
	}

	// Default: container mode
	return "container"
}

func isUnixSocket(fd int) bool {
	_, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil {
		return false
	}
	var sa unix.Sockaddr
	sa, err = unix.Getsockname(fd)
	if err != nil {
		// If Getsockname fails but SO_TYPE succeeded, it's still a socket.
		// On macOS socketpairs, Getsockname may return an empty address.
		return true
	}
	_, isUnix := sa.(*unix.SockaddrUnix)
	return isUnix
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

package relay

import (
	"fmt"
	"math/rand"
	"net"
)

func EphemeralPort(ip net.IP, kind string) (int, error) {
	if kind == "udp" {
		conn, err := net.ListenUDP(kind, &net.UDPAddr{IP: ip})
		if err != nil {
			return 0, fmt.Errorf("error finding a udp port to listen on: %w", err)
		}
		addr, ok := conn.LocalAddr().(*net.UDPAddr)
		if !ok {
			return 0, fmt.Errorf("failed reading port from successful udp connection")
		}
		conn.Close()
		return addr.Port, nil
	}
	firstEphemeralPort := 49152
	maxPort := 65535
	numEphemeralPorts := maxPort - firstEphemeralPort
	offset := rand.Intn(numEphemeralPorts)
	for p := offset; p < numEphemeralPorts; p++ {
		port := firstEphemeralPort + p%numEphemeralPorts
		conn, err := net.Listen(kind, fmt.Sprintf("%s:%d", ip.String(), port))
		if err != nil {
			continue // In use.
		}
		_ = conn.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no open ports")
}

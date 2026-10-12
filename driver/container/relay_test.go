package container

import (
	"net"
	"strconv"
	"testing"
)

func TestIsPortConflict(t *testing.T) {
	for in, want := range map[string]bool{
		// nerdctl / finch
		`level=fatal msg="failed to load networking flags: bind for 127.0.0.1:50111 failed: port is already allocated"`: true,
		// docker
		"Error response from daemon: driver failed programming external connectivity on endpoint x: Bind for 127.0.0.1:50111 failed: port is already allocated": true,
		// podman
		"Error: rootlessport listen tcp 127.0.0.1:50111: bind: address already in use": true,
		"Error: no such image: ghcr.io/buildwarden/relay:dev":                          false,
		"": false,
	} {
		if got := isPortConflict(in); got != want {
			t.Errorf("isPortConflict(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestFreeLoopbackPort(t *testing.T) {
	p, err := freeLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("port %d out of range", p)
	}
	// The probe listener is closed, so the port is bindable again.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
	if err != nil {
		t.Fatalf("port %d not released: %v", p, err)
	}
	l.Close()
}

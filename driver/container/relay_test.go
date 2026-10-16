package container

import "testing"

func TestIsPortConflict(t *testing.T) {
	for in, want := range map[string]bool{
		// nerdctl / finch
		`level=fatal msg="failed to load networking flags: ` +
			`bind for 127.0.0.1:50111 failed: port is already allocated"`: true,
		// docker
		"Error response from daemon: driver failed programming " +
			"external connectivity on endpoint x: Bind for " +
			"127.0.0.1:50111 failed: port is already allocated": true,
		// podman
		"Error: rootlessport listen tcp 127.0.0.1:50111: " +
			"bind: address already in use": true,
		"Error: no such image: ghcr.io/buildwarden/relay:dev": false,
		"": false,
	} {
		if got := isPortConflict(in); got != want {
			t.Errorf("isPortConflict(%q) = %v; want %v", in, got, want)
		}
	}
}

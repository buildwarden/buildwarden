package hyperv

import (
	"net/netip"
	"testing"
)

// TestParseEnsureNetworkReadback: EnsureNetwork's KEY=value lines, amid other
// output, give the chosen NAT, whether it was reused, and its prefix and the
// host's address in it.
func TestParseEnsureNetworkReadback(t *testing.T) {
	tests := []struct {
		name, out  string
		wantNAT    string
		wantReused bool
		wantPrefix string
		wantHost   string
	}{
		{"fresh per-build nat",
			"NAT=warden-abc123-nat\nREUSED=0\nPREFIX=192.168.240.0/20\nHOSTIP=192.168.240.1\n",
			"warden-abc123-nat", false, "192.168.240.0/20", "192.168.240.1"},
		{"reused dev nat on the user's prefix",
			"noise\r\nNAT=warden-dev-nat\r\nREUSED=1\r\nPREFIX=10.200.0.0/24\r\n" +
				"HOSTIP=169.254.3.4\r\nHOSTIP=10.200.0.9\r\n",
			"warden-dev-nat", true, "10.200.0.0/24", "10.200.0.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseNetReadback(tt.out).ensureResources("warden-abc123")
			if err != nil {
				t.Fatal(err)
			}
			if res.NATName != tt.wantNAT || res.Reused != tt.wantReused ||
				res.NATPrefix != netip.MustParsePrefix(tt.wantPrefix) ||
				res.HostIP != netip.MustParseAddr(tt.wantHost) ||
				res.BuildSwitch != "warden-abc123" {
				t.Errorf("got %+v", res)
			}
		})
	}
}

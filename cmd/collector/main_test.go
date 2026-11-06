package main

import (
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/relaycfg"
)

func TestParseBuildLink(t *testing.T) {
	tests := []struct {
		in   string
		want *relaycfg.BuildLink
		err  string
	}{
		{"", nil, ""},
		{"100.127.255.252/30", &relaycfg.BuildLink{Prefix: "100.127.255.252/30",
			Relay: "100.127.255.253", Guest: "100.127.255.254"}, ""},
		{"192.0.2.0/24", &relaycfg.BuildLink{Prefix: "192.0.2.0/24",
			Relay: "192.0.2.1", Guest: "192.0.2.2"}, ""},
		{"192.0.2.1/24", nil, "did you mean 192.0.2.0/24"},
		{"192.0.2.0/31", nil, "length"},
		{"127.0.0.0/30", nil, "loopback"},
		{"nonsense", nil, "netip.ParsePrefix"},
	}
	for _, tt := range tests {
		got, err := parseBuildLink(tt.in)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%q: err = %v, want %q", tt.in, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tt.in, err)
			continue
		}
		if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
			t.Errorf("%q = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

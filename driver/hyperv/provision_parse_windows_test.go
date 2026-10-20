//go:build windows

package hyperv

import "testing"

func TestParseEnsureNetworkOutput(t *testing.T) {
	tests := []struct {
		name       string
		out        string
		wantNAT    string
		wantReused bool
	}{
		{
			name:       "fresh per-build nat",
			out:        "NAT=warden-abc123-nat REUSED=0\n",
			wantNAT:    "warden-abc123-nat",
			wantReused: false,
		},
		{
			name:       "reused durable nat",
			out:        "NAT=warden-dev-nat REUSED=1\n",
			wantNAT:    "warden-dev-nat",
			wantReused: true,
		},
		{
			name:       "trailing line amid other output",
			out:        "some noise\r\nmore noise\r\nNAT=warden-dev-nat REUSED=1\r\n",
			wantNAT:    "warden-dev-nat",
			wantReused: true,
		},
		{
			name:       "no marker",
			out:        "OK\n",
			wantNAT:    "",
			wantReused: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNAT, gotReused := parseEnsureNetworkOutput(tt.out)
			if gotNAT != tt.wantNAT {
				t.Errorf("nat = %q, want %q", gotNAT, tt.wantNAT)
			}
			if gotReused != tt.wantReused {
				t.Errorf("reused = %v, want %v", gotReused, tt.wantReused)
			}
		})
	}
}

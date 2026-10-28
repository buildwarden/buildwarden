package driver

import (
	"testing"
	"time"
)

func TestParseAge(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"7d": 7 * 24 * time.Hour, "1.5d": 36 * time.Hour, "36h": 36 * time.Hour,
		" 90m ": 90 * time.Minute, "0d": 0,
	} {
		got, err := ParseAge(in)
		if err != nil || got != want {
			t.Errorf("ParseAge(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "-1d", "7days", "soon"} {
		if _, err := ParseAge(bad); err == nil {
			t.Errorf("ParseAge(%q) succeeded, want an error", bad)
		}
	}
}

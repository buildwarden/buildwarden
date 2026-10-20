package driver

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseAge parses an age such as "7d", "36h" or "90m": a Go duration, plus a
// "d" (24 h) suffix for whole or fractional days.
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid age %q", s)
		}
		return time.Duration(n * float64(24*time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid age %q", s)
	}
	return d, nil
}

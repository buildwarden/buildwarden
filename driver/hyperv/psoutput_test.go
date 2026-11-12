package hyperv

import (
	"fmt"
	"strings"
	"testing"
)

// TestPSTail: the tail keeps the last psTailLines non-empty lines, trimmed.
func TestPSTail(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "line%d\r\n\r\n", i)
	}
	got := psTail(b.String())
	if !strings.HasPrefix(got, "line3; line4") || !strings.HasSuffix(got, "line11; line12") ||
		strings.Count(got, "line") != psTailLines {
		t.Errorf("psTail = %q", got)
	}
	if got := psTail(" \r\n\n"); got != "" {
		t.Errorf("blank output: %q", got)
	}
}

// TestPSDoneCheck: only output whose last non-empty line is DONE passes.
func TestPSDoneCheck(t *testing.T) {
	for _, out := range []string{"DONE", "A=1\r\nDONE\r\n", "A=1\nDONE\n\n"} {
		if err := psDoneCheck(out); err != nil {
			t.Errorf("%q: %v", out, err)
		}
	}
	for _, out := range []string{"", "A=1\n", "DONE\nA=1\n", "NOTDONE\n"} {
		if err := psDoneCheck(out); err == nil {
			t.Errorf("%q passed", out)
		}
	}
}

package driver

import (
	"regexp"
	"testing"
)

func TestRandAlphaNum_Length(t *testing.T) {
	for _, n := range []int{0, 1, 8, 32, 100} {
		got := RandAlphaNum(n)
		if len(got) != n {
			t.Errorf("RandAlphaNum(%d) length = %d, want %d", n, len(got), n)
		}
	}
}

func TestRandAlphaNum_Charset(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9]*$`)
	for i := 0; i < 20; i++ {
		got := RandAlphaNum(64)
		if !valid.MatchString(got) {
			t.Errorf("RandAlphaNum(64) = %q, contains invalid chars", got)
		}
	}
}

func TestRandAlphaNum_Uniqueness(t *testing.T) {
	a := RandAlphaNum(16)
	b := RandAlphaNum(16)
	if a == b {
		t.Errorf("two RandAlphaNum(16) calls produced identical values: %q", a)
	}
}

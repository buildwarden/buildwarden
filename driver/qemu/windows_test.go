package qemu

import (
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func TestIsWindowsGuest(t *testing.T) {
	cases := []struct {
		guestOS string
		want    bool
	}{
		{"windows", true},
		{"Windows", true},
		{"WINDOWS", true},
		{"linux", false},
		{"", false},
	}
	for _, c := range cases {
		got := isWindowsGuest(&driver.BuildRequest{GuestOS: c.guestOS})
		if got != c.want {
			t.Errorf("isWindowsGuest(%q) = %v, want %v", c.guestOS, got, c.want)
		}
	}
}

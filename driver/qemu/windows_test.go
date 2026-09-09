package qemu

import (
	"strings"
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

func TestWardenRunPS1(t *testing.T) {
	got := wardenRunPS1()
	for _, want := range []string{
		"warden-io.exe",
		"initialize",
		"--gateway=" + relayGatewayIP,
		"--ip=" + buildGuestCIDR,
		"Stop-Computer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wardenRunPS1() missing %q; got:\n%s", want, got)
		}
	}
}

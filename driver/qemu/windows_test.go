package qemu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/winguest"
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

// TestWindowsSeedDirRunScript checks the qemu Windows seed carries the
// shared launcher for the build's link, which passes initialize no --ip.
func TestWindowsSeedDirRunScript(t *testing.T) {
	link, err := buildlink.Parse("100.126.255.252/30")
	if err != nil {
		t.Fatal(err)
	}
	seedDir, err := windowsSeedDir(t.TempDir(), "amd64", link)
	if err != nil {
		t.Fatalf("windowsSeedDir: %v", err)
	}
	ps, err := os.ReadFile(filepath.Join(seedDir, "warden-run.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ps) != winguest.WardenRunPS1(link) {
		t.Error("staged warden-run.ps1 is not the shared bootstrap for the build's link")
	}
	if !strings.Contains(string(ps), "initialize --gateway=100.126.255.253 ") ||
		strings.Contains(string(ps), "--ip") {
		t.Errorf("staged warden-run.ps1 does not run initialize --gateway alone:\n%s", ps)
	}
}

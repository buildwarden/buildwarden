package main

import (
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver/vz"
)

func TestFormatVZImages(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	imgs := []vz.Image{
		{Dir: "/c/macos-90a1", Prepared: t0,
			IPSW: "UniversalMac_27.0.1_26A100_Restore.ipsw"},
		{Dir: "/c/macos-0old", Prepared: t0.Add(-time.Hour)},
		{Dir: "/c/macos-a139", Prepared: t0.Add(-2 * time.Hour), Active: true,
			IPSW: "UniversalMac_27.2_26B123_Restore.ipsw"},
		{Dir: "/c/macos-f11e", Prepared: t0.Add(-3 * time.Hour),
			IPSW: "UniversalMac_27.2_26B123_Restore.ipsw"},
	}
	want := []string{
		"vz      macos    /c/macos-a139  active  UniversalMac_27.2_26B123_Restore.ipsw",
		"vz      macos    /c/macos-90a1  older   UniversalMac_27.0.1_26A100_Restore.ipsw",
		"vz      macos    /c/macos-0old  older",
		"vz      macos    /c/macos-f11e  older   UniversalMac_27.2_26B123_Restore.ipsw",
	}
	got := formatVZImages(imgs)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := formatVZImages(nil); len(got) != 0 {
		t.Fatalf("no images: %q", got)
	}
}

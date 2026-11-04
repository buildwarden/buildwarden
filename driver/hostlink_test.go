package driver

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

func TestHostPrefixes(t *testing.T) {
	got, err := HostPrefixes()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if !p.IsValid() {
			t.Errorf("invalid prefix %v", p)
		}
	}
}

func TestCheckHostOverlap(t *testing.T) {
	link := buildlink.Default()
	host := []netip.Prefix{netip.MustParsePrefix("192.168.1.2/24"),
		netip.MustParsePrefix("100.64.1.1/10")}
	if err := CheckHostOverlap(link, host); err != nil {
		t.Fatalf("a containing network refused: %v", err)
	}
	host = append(host, netip.MustParsePrefix("100.127.255.254/30"))
	var fe *FixableError
	err := CheckHostOverlap(link, host)
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "100.127.255.254/30") ||
		!strings.Contains(fe.Fix, "build_link") || !strings.Contains(fe.Fix,
		"--build-link") {
		t.Fatalf("err = %#v", err)
	}
}

func TestCheckLauncherStamp(t *testing.T) {
	dir := t.TempDir()
	stamp := filepath.Join(dir, "link-channel")
	var fe *FixableError
	if err := CheckLauncherStamp(stamp, "the initramfs", "rebuild.sh"); !errors.As(err, &fe) ||
		fe.Fix != "rebuild.sh" || !strings.Contains(fe.Msg, "the initramfs") {
		t.Fatalf("missing stamp: %#v", err)
	}
	if err := os.WriteFile(stamp, []byte("WARDEN_LINK_CHANNEL=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckLauncherStamp(stamp, "the initramfs", "rebuild.sh"); err != nil {
		t.Fatalf("current stamp: %v", err)
	}
}

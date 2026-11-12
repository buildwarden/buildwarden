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

func TestLauncherStampBlocks(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.link-channel")
	stale := filepath.Join(dir, "stale.link-channel")
	current := filepath.Join(dir, "current.link-channel")
	for p, body := range map[string]string{stale: "WARDEN_LINK_CHANNEL=0\n",
		current: "WARDEN_LINK_CHANNEL=1\n"} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name     string
		st       LauncherStamp
		blocks   bool
		blockTag bool
	}{
		{"required missing", LauncherStamp{Path: missing, Required: true}, true, true},
		{"required stale", LauncherStamp{Path: stale, Required: true}, true, true},
		{"required current", LauncherStamp{Path: current, Required: true}, false, false},
		{"required not found", LauncherStamp{Required: true}, false, false},
		{"direct-boot missing", LauncherStamp{Path: missing}, false, false},
		{"direct-boot stale", LauncherStamp{Path: stale}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.st.What, tc.st.Rebuild = "the asset", "rebuild.sh"
			if got := tc.st.Blocks(); got != tc.blocks {
				t.Errorf("Blocks() = %v, want %v", got, tc.blocks)
			}
			report := tc.st.Report()
			if got := strings.HasPrefix(report, "BLOCKED: "); got != tc.blockTag {
				t.Errorf("Report() = %q, BLOCKED prefix %v, want %v",
					report, got, tc.blockTag)
			}
		})
	}
}

// TestLauncherStampWant: a stamp with Want compares against it, not
// buildlink.Channel, in Check, Blocks and Report alike.
func TestLauncherStampWant(t *testing.T) {
	dir := t.TempDir()
	st := LauncherStamp{What: "the disk", Rebuild: "refresh.sh", Required: true, Want: 2,
		Path: filepath.Join(dir, "disk.link-channel")}
	for _, tc := range []struct {
		body  string // "" leaves no stamp
		state string
	}{{"", "missing"}, {"WARDEN_LINK_CHANNEL=1\n", "stale"},
		{"WARDEN_LINK_CHANNEL=2\n", "current"}} {
		if tc.body != "" {
			if err := os.WriteFile(st.Path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		err := st.Check()
		var fe *FixableError
		if (tc.state == "current") != (err == nil) ||
			(err != nil && (!errors.As(err, &fe) || fe.Fix != "refresh.sh")) {
			t.Errorf("%s: Check = %v", tc.state, err)
		}
		if st.Blocks() != (tc.state != "current") ||
			!strings.Contains(st.Report(), tc.state) {
			t.Errorf("%s: Blocks = %v, Report = %q", tc.state, st.Blocks(),
				st.Report())
		}
	}
}

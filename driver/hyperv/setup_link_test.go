package hyperv

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// TestSetupRefusesNATOverlappingLink checks the refusal comes before setup
// touches anything (it runs on any OS, before the platform check).
func TestSetupRefusesNATOverlappingLink(t *testing.T) {
	configured, _ := buildlink.Parse("192.168.240.4/30")
	tests := []struct {
		nat  string
		link buildlink.Link
		want string
	}{
		{"100.127.0.0/16", buildlink.Link{}, "overlaps the build link 100.127.255.252/30"},
		{DefaultNATPrefix, configured, "overlaps the build link 192.168.240.4/30"},
		{"bogus", buildlink.Link{}, "invalid NAT prefix"},
	}
	for _, tt := range tests {
		err := RunSetup(context.Background(), io.Discard,
			SetupOptions{NATPrefix: tt.nat, BuildLink: tt.link})
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.nat, err, tt.want)
		}
	}
	if err := checkNATPrefix(DefaultNATPrefix, buildlink.Default()); err != nil {
		t.Errorf("the defaults overlap: %v", err)
	}
}

func TestRelayBootStamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WARDEN_HYPERV_CACHE_DIR", dir)
	t.Setenv("WARDEN_RELAY_BOOT_VHDX", "")
	if st := relayBootStamp(); st.Path != "" || st.Check() == nil {
		t.Fatalf("no disk: %+v", st)
	}
	disk := filepath.Join(dir, "relay-boot.vhdx")
	if err := os.WriteFile(disk, []byte("vhdx"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := relayBootStamp()
	if st.Path != filepath.Join(dir, "relay-boot.link-channel") {
		t.Fatalf("stamp path = %s", st.Path)
	}
	var fe *driver.FixableError
	if err := st.Check(); !errors.As(err, &fe) || fe.Fix != RefreshRelayBootFix {
		t.Fatalf("an unstamped disk: %v", err)
	}
	if !st.Blocks() || !strings.Contains(st.Report(), "BLOCKED") ||
		!strings.Contains(st.Report(), "missing") {
		t.Errorf("an unstamped disk's doctor line: %s", st.Report())
	}
	// A disk stamped 1 predates the relay seed: refused, naming refresh-cache.sh.
	if err := os.WriteFile(st.Path, []byte("WARDEN_LINK_CHANNEL=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.Check(); !errors.As(err, &fe) || fe.Fix != RefreshRelayBootFix ||
		!strings.Contains(err.Error(), "holds link channel 1; this warden needs 2") {
		t.Fatalf("a disk stamped 1: %v", err)
	}
	if !st.Blocks() || !strings.Contains(st.Report(), "stale") ||
		!strings.Contains(st.Report(), "refresh-cache.sh") {
		t.Errorf("a disk stamped 1's doctor line: %s", st.Report())
	}
	if err := os.WriteFile(st.Path, []byte("WARDEN_LINK_CHANNEL=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.Check(); err != nil || st.Blocks() {
		t.Fatalf("a disk stamped 2: %v", err)
	}
	override := filepath.Join(t.TempDir(), "my-relay.vhdx")
	if err := os.WriteFile(override, []byte("vhdx"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARDEN_RELAY_BOOT_VHDX", override)
	if got := relayBootStamp().Path; got != strings.TrimSuffix(override, ".vhdx")+
		".link-channel" {
		t.Fatalf("override stamp path = %s", got)
	}
}

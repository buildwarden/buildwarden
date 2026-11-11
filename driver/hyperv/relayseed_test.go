package hyperv

import (
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func validSeed() RelaySeed {
	return RelaySeed{IP: netip.MustParseAddr("192.168.240.2"), PrefixLen: 20,
		Gateway: netip.MustParseAddr("192.168.240.1"), Token: strings.Repeat("0f", 32)}
}

// TestRelaySeedStage: the staged seed holds exactly the relay's pre-claim
// values (RELAY-08), in the init's key syntax, readable by its owner only.
func TestRelaySeedStage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "relay-seed")
	if err := validSeed().Stage(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, RelaySeedFile)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "warden.seed=1\nwarden.net=static\nwarden.nat_ip=192.168.240.2\n" +
		"warden.nat_prefix=20\nwarden.nat_gw=192.168.240.1\nwarden.claim_token=" +
		strings.Repeat("0f", 32) + "\n"
	if string(b) != want {
		t.Errorf("seed =\n%s\nwant\n%s", b, want)
	}
	for _, key := range []string{"dns", "upstream", "build_link", "collector"} {
		if strings.Contains(string(b), key) {
			t.Errorf("the seed carries %q, which travels in the claim", key)
		}
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("seed mode = %v, %v; want 0600", fi.Mode(), err)
		}
	}
}

// TestRelaySeedValidate: what the relay init would stop on is refused on the
// host, and nothing is staged.
func TestRelaySeedValidate(t *testing.T) {
	cases := map[string]func(*RelaySeed){
		"no IP":         func(s *RelaySeed) { s.IP = netip.Addr{} },
		"IPv6 IP":       func(s *RelaySeed) { s.IP = netip.MustParseAddr("fd00::2") },
		"no gateway":    func(s *RelaySeed) { s.Gateway = netip.Addr{} },
		"IPv6 gateway":  func(s *RelaySeed) { s.Gateway = netip.MustParseAddr("fd00::1") },
		"prefix 0":      func(s *RelaySeed) { s.PrefixLen = 0 },
		"prefix 31":     func(s *RelaySeed) { s.PrefixLen = 31 },
		"short token":   func(s *RelaySeed) { s.Token = "abc" },
		"upper token":   func(s *RelaySeed) { s.Token = strings.Repeat("0F", 32) },
		"non-hex token": func(s *RelaySeed) { s.Token = strings.Repeat("zz", 32) },
	}
	for name, mut := range cases {
		s := validSeed()
		mut(&s)
		dir := filepath.Join(t.TempDir(), "seed")
		if err := s.Stage(dir); err == nil {
			t.Errorf("%s: staged", name)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s: the seed dir was created", name)
		}
	}
	for _, n := range []int{1, 30} {
		s := validSeed()
		s.PrefixLen = n
		if err := s.Validate(); err != nil {
			t.Errorf("prefix %d refused: %v", n, err)
		}
	}
}

package hyperv

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

// The relay seed is the Hyper-V relay VM's boot channel (BOOT-06, RELAY-08):
// a per-build ISO, volume label RelaySeedLabel, holding one file of
// warden.<key>=<value> lines that tools/relay-vm/init reads before it
// configures anything. Gen2 firmware gives the host no per-boot kernel
// command line, so this is where the relay's pre-claim values reach it, and
// only those: its own address, prefix and gateway on the NAT, and the claim
// token. The build link, the collector and the upstream resolvers travel in
// the claim.
const (
	// RelaySeedLabel is the relay seed ISO's volume label.
	RelaySeedLabel = "WARDENRELAY"
	// RelaySeedFile is the file the relay init reads from the seed.
	RelaySeedFile = "warden-relay.conf"
	// relaySeedVersion is the seed format version the init requires.
	relaySeedVersion = 1
)

// RelaySeed is the relay VM's pre-claim values for one build.
type RelaySeed struct {
	// IP is the relay's static address on the NAT, where it accepts the claim.
	IP netip.Addr
	// PrefixLen is the NAT prefix's length.
	PrefixLen int
	// Gateway is the host's address on the NAT, the relay's default route.
	Gateway netip.Addr
	// Token is the per-build claim token (driver.RandomToken).
	Token string
}

// Validate applies the relay init's checks, so a seed the init would stop on
// is refused on the host instead: IPv4 addresses, a prefix of 1 to 30 and a
// token of 64 lowercase hex digits.
func (s RelaySeed) Validate() error {
	switch {
	case !s.IP.Is4():
		return fmt.Errorf("relay seed: relay address %v is not an IPv4 address", s.IP)
	case !s.Gateway.Is4():
		return fmt.Errorf("relay seed: gateway %v is not an IPv4 address", s.Gateway)
	case s.PrefixLen < 1 || s.PrefixLen > 30:
		return fmt.Errorf("relay seed: prefix length %d is not 1 to 30", s.PrefixLen)
	case !isClaimToken(s.Token):
		return fmt.Errorf("relay seed: the claim token is not 64 lowercase hex digits")
	}
	return nil
}

// isClaimToken reports whether t is what driver.RandomToken emits.
func isClaimToken(t string) bool {
	if len(t) != 64 {
		return false
	}
	for _, c := range t {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// render is the seed file's content: the format version, a static upstream
// and its three values, and the claim token, one key per line.
func (s RelaySeed) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "warden.seed=%d\n", relaySeedVersion)
	b.WriteString("warden.net=static\n")
	fmt.Fprintf(&b, "warden.nat_ip=%s\n", s.IP)
	fmt.Fprintf(&b, "warden.nat_prefix=%d\n", s.PrefixLen)
	fmt.Fprintf(&b, "warden.nat_gw=%s\n", s.Gateway)
	fmt.Fprintf(&b, "warden.claim_token=%s\n", s.Token)
	return b.String()
}

// Stage validates the seed and writes RelaySeedFile into dir, which it
// creates. The file holds the claim token, so it is readable by its owner
// only; dir lives under the build's work directory and goes with it.
func (s RelaySeed) Stage(dir string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("relay seed dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, RelaySeedFile), []byte(s.render()),
		0o600); err != nil {
		return fmt.Errorf("relay seed: %w", err)
	}
	return nil
}

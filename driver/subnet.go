package driver

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/lesiw/ctrctl"
)

// WardenBaseNet is a /24 in the CGNAT range (100.64.0.0/10) which is
// reserved for carrier-grade NAT and unlikely to collide with user networks.
const WardenBaseNet = "100.64.87"

// Subnet holds the allocated network addresses for a build.
type Subnet struct {
	CIDR    string
	RelayIP string
	BuildIP string
}

// AllocateNetwork creates a bridge network called name on a free /29 within
// the warden base /24 and returns its addresses. Creation itself is the
// availability test: the runtime refuses a subnet that overlaps an existing
// network.
//
// That refusal is NOT atomic (nerdctl, at least, lets concurrent creates of
// the same subnet all succeed), and two bridges sharing a subnet break host
// access to the relay, since the host reaches it by routing to its IP. So
// allocation runs under a host-wide lock, which makes it atomic across
// warden processes on this host.
func AllocateNetwork(name string) (Subnet, error) {
	unlock, err := HostLock("network")
	if err != nil {
		return Subnet{}, err
	}
	defer unlock()
	for block := 0; block < 32; block++ {
		base := block * 8
		sub := Subnet{
			CIDR:    fmt.Sprintf("%s.%d/29", WardenBaseNet, base),
			RelayIP: fmt.Sprintf("%s.%d", WardenBaseNet, base+2),
			BuildIP: fmt.Sprintf("%s.%d", WardenBaseNet, base+3),
		}
		_, err := ctrctl.NetworkCreate(
			&ctrctl.NetworkCreateOpts{
				Driver: "bridge",
				Subnet: sub.CIDR,
			},
			name,
		)
		if err != nil {
			continue
		}
		return sub, nil
	}
	return Subnet{}, fmt.Errorf(
		"no available /29 subnet in %s.0/24", WardenBaseNet)
}

var anumRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

// RandAlphaNum produces a cryptographically-random alphanumeric string.
func RandAlphaNum(n int) string {
	b := make([]rune, n)
	for i := range b {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(len(anumRunes))))
		if err != nil {
			// Since Go 1.24 the standard library treats a failing crypto/rand
			// source as fatal, so this is not an ordinary failure, and an
			// error return would give every caller a case it cannot handle.
			panic(err) //nolint:forbidigo // a crypto/rand failure is fatal (Go 1.24+)
		}
		b[i] = anumRunes[j.Uint64()]
	}
	return string(b)
}

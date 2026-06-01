package driver

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os/exec"
	"strings"

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

// AllocateSubnet finds an available /29 within the warden base /24.
// It probes each of the 32 possible /29 blocks and returns the first
// one that doesn't conflict with existing container networks.
func AllocateSubnet(cli []string) (Subnet, error) {
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
			"warden-probe",
		)
		if err != nil {
			continue
		}
		_, _ = ctrctl.NetworkRm(nil, "warden-probe")
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
			panic(err)
		}
		b[i] = anumRunes[j.Uint64()]
	}
	return string(b)
}

// ExtractFromImage reads the FROM line from a Containerfile and returns
// the image reference.
func ExtractFromImage(containerfile string) (string, error) {
	data, err := exec.Command("head", "-50", containerfile).Output()
	if err != nil {
		return "", fmt.Errorf("reading containerfile: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "FROM ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("no FROM directive found in %s", containerfile)
}

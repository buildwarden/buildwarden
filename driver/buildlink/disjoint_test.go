package buildlink_test

import (
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
)

// TestDisjointFromOwnRanges reads the ranges from the packages that own them,
// so a moved range cannot silently start overlapping the default.
func TestDisjointFromOwnRanges(t *testing.T) {
	ranges := []buildlink.Range{
		{Name: "the container network", CIDR: driver.ContainerBaseCIDR},
		{Name: "the firecracker upstream", CIDR: firecracker.UpstreamCIDR},
		{Name: "the hyperv NAT", CIDR: hyperv.DefaultNATPrefix},
		{Name: "qemu user-mode", CIDR: qemu.UserNetCIDR},
	}
	if err := buildlink.Default().Disjoint(ranges); err != nil {
		t.Fatalf("default overlaps: %v", err)
	}
	overlapping := map[string]string{
		"the container network":    "100.64.87.8/30",
		"the firecracker upstream": "100.64.90.0/30",
		"the hyperv NAT":           "192.168.240.4/30",
		"qemu user-mode":           "10.0.2.0/30",
	}
	for name, cidr := range overlapping {
		l, err := buildlink.Parse(cidr)
		if err != nil {
			t.Fatal(err)
		}
		err = l.Disjoint(ranges)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v, want it to name %s", cidr, err, name)
		}
	}
}

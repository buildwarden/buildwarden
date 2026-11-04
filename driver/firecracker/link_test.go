package firecracker

import (
	"testing"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// TestDirectBootLinkArgs checks the direct-boot command line carries the
// build link's keys for the default and a configured link.
func TestDirectBootLinkArgs(t *testing.T) {
	configured, _ := buildlink.Parse("100.126.255.252/30")
	for _, tt := range []struct {
		link buildlink.Link
		keys string
	}{
		{buildlink.Default(),
			"warden.link_ip=100.127.255.254/30 warden.link_gw=100.127.255.253"},
		{configured, "warden.link_ip=100.126.255.254/30 warden.link_gw=100.126.255.253"},
	} {
		c := buildConfig(0, buildBoot{Kernel: "k", Initrd: "i",
			LinkArgs: tt.link.BootArgs()},
			1, 1)
		if got, want := c.effectiveArgs(), baseArgs+" "+tt.keys; got != want {
			t.Errorf("%s: args %q, want %q", tt.link, got, want)
		}
	}
}

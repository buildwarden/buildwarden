package hyperv

import (
	"errors"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

// TestParseDefaultRouteDNS: the default route's interface's IPv4 servers in
// order, the interface named in the source and in each refusal.
func TestParseDefaultRouteDNS(t *testing.T) {
	list, source, err := parseDefaultRouteDNS("ALIAS=Wi-Fi 2\r\nDNS=10.9.0.53\r\n" +
		"DNS=fe80::1\r\nDNS=10.9.0.54\r\n")
	if err != nil || resolvers.Join(list) != "10.9.0.53, 10.9.0.54" ||
		source != "interface Wi-Fi 2 (the IPv4 default route's)" {
		t.Fatalf("got %v from %q, %v", list, source, err)
	}
	for out, want := range map[string]string{
		"ROUTE=none\n":                "no IPv4 default route",
		"":                            "no IPv4 default route",
		"ALIAS=Ethernet\n":            "interface Ethernet, which holds",
		"ALIAS=Ethernet\nDNS=bogus\n": "lists no IPv4 DNS server",
	} {
		_, _, err := parseDefaultRouteDNS(out)
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, want) ||
			!strings.Contains(fe.Fix, "[vm] dns") {
			t.Errorf("%q: err = %v", out, err)
		}
	}
}

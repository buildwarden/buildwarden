package relay

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestBlockedDialBuildLink(t *testing.T) {
	links := []string{"100.127.255.252/30", "100.126.255.252/30"}
	for _, cidr := range links {
		link := netip.MustParsePrefix(cidr)
		for _, ssrf := range []bool{false, true} {
			r := &Relay{cfg: Config{BuildLink: link, SSRF: ssrf}}
			a := link.Addr()
			for i := 0; i < 4; i++ {
				if !r.blockedDial(net.IP(a.AsSlice())) {
					t.Errorf("%s ssrf=%v: %s not blocked", cidr, ssrf, a)
				}
				a = a.Next()
			}
			// The rest of 100.64.0.0/10 is not the link; SSRF does not
			// block it either (IsPrivate is false for RFC 6598).
			if r.blockedDial(net.ParseIP("100.127.255.1")) {
				t.Errorf("%s ssrf=%v: an address outside the link blocked",
					cidr, ssrf)
			}
			if got := r.blockedDial(net.ParseIP("10.1.2.3")); got != ssrf {
				t.Errorf("%s ssrf=%v: RFC 1918 blocked = %v", cidr, ssrf, got)
			}
		}
	}
	// Container mode: no link, no SSRF, nothing blocked.
	r := &Relay{cfg: Config{}}
	for _, ip := range []string{"100.127.255.253", "10.0.0.1", "127.0.0.1"} {
		if r.blockedDial(net.ParseIP(ip)) {
			t.Errorf("no link: %s blocked", ip)
		}
	}
}

// TestSafeDialRefusesBuildLink checks the refusal happens on the dialed
// address, before any connection is attempted, with SSRF off.
func TestSafeDialRefusesBuildLink(t *testing.T) {
	r := &Relay{cfg: Config{BuildLink: netip.MustParsePrefix("100.127.255.252/30")}}
	r.buildTransport()
	if r.transport.DialContext == nil {
		t.Fatal("no dialer installed for a build link with SSRF off")
	}
	_, err := r.transport.DialContext(context.Background(), "tcp", "100.127.255.254:80")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("dial into the link: err = %v, want blocked", err)
	}
	none := &Relay{cfg: Config{}}
	none.buildTransport()
	if none.transport.DialContext != nil {
		t.Fatal("container mode gained a filtering dialer")
	}
}

func TestPickNameserver(t *testing.T) {
	link := netip.MustParsePrefix("100.127.255.252/30")
	self := net.ParseIP("100.127.255.253")
	tests := []struct {
		name, conf string
		link       netip.Prefix
		want       string
	}{
		{"first usable", "nameserver 10.0.0.2\nnameserver 1.1.1.1\n", link, "10.0.0.2:53"},
		{"skips link", "nameserver 100.127.255.254\nnameserver 9.9.9.9\n", link,
			"9.9.9.9:53"},
		{"skips self and loopback", "nameserver 100.127.255.253\nnameserver 127.0.0.1\n" +
			"nameserver 8.8.4.4\n", link, "8.8.4.4:53"},
		{"no link set", "nameserver 100.127.255.254\n", netip.Prefix{},
			"100.127.255.254:53"},
		{"none left", "search example\nnameserver 100.127.255.252\n", link, "8.8.8.8:53"},
		{"ipv6", "nameserver fd00::1\n", link, "[fd00::1]:53"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickNameserver(tt.conf, self, tt.link); got != tt.want {
				t.Errorf("pickNameserver = %q, want %q", got, tt.want)
			}
		})
	}
}

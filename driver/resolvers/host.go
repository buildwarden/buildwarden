package resolvers

import (
	"fmt"
	"net/netip"
	"strings"
)

const (
	// ResolvConf is the resolver file both Unix host sources read.
	ResolvConf = "/etc/resolv.conf"
	// ResolvedUpstream is systemd-resolved's list of the servers its stub
	// forwards to.
	ResolvedUpstream = "/run/systemd/resolve/resolv.conf"
	// ResolvedStub is systemd-resolved's stub listener, the only nameserver
	// of /etc/resolv.conf when the stub fronts the upstreams.
	ResolvedStub = "127.0.0.53"
)

// ReadFile reads a file; the sources take it so tests pass canned files.
type ReadFile func(path string) ([]byte, error)

// ParseResolvConf returns the IPv4 nameservers of a resolv.conf, in order.
// The relay's resolvers are IPv4 (upstream_dns), so an IPv6 line, scoped or
// not, is not one.
func ParseResolvConf(conf string) []netip.Addr {
	var out []netip.Addr
	for _, line := range strings.Split(conf, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		if a, err := netip.ParseAddr(f[1]); err == nil && a.Is4() {
			out = append(out, a)
		}
	}
	return out
}

// Linux is a Linux host's resolvers and the file they came from: the
// nameservers of /etc/resolv.conf, or systemd-resolved's upstream list when
// its stub is the only one listed (the stub is loopback, so it is never the
// relay's).
func Linux(read ReadFile) (list []netip.Addr, source string, err error) {
	b, err := read(ResolvConf)
	if err != nil {
		return nil, ResolvConf, fmt.Errorf("reading %s: %w", ResolvConf, err)
	}
	list = ParseResolvConf(string(b))
	if len(list) == 1 && list[0].String() == ResolvedStub {
		if up, err := read(ResolvedUpstream); err == nil {
			return ParseResolvConf(string(up)), ResolvedUpstream, nil
		}
	}
	return list, ResolvConf, nil
}

// Mac is a macOS host's resolvers and the file they came from: the
// nameservers of /etc/resolv.conf, the primary resolvers macOS writes there.
func Mac(read ReadFile) (list []netip.Addr, source string, err error) {
	b, err := read(ResolvConf)
	if err != nil {
		return nil, ResolvConf, fmt.Errorf("reading %s: %w", ResolvConf, err)
	}
	return ParseResolvConf(string(b)), ResolvConf, nil
}

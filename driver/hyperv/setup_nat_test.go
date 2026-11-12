package hyperv

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// natProv is a Provisioner whose EnsureNetwork returns a set result and
// records the spec it was given.
type natProv struct {
	fakeProv
	res  NetworkResources
	spec NetworkSpec
}

func (f *natProv) EnsureNetwork(_ context.Context, spec NetworkSpec) (*NetworkResources, error) {
	f.spec = spec
	res := f.res
	return &res, nil
}

// TestSetupRefusesNATDurableBuildsReject: setup reports ready only on the NAT
// durable builds verify (<network>-nat on --nat-prefix). A NAT of another
// name that owns the prefix is refused, naming it and how to clear it, rather
// than adopted, so `warden setup` does not succeed on a network every
// durable build then refuses with "run warden setup".
func TestSetupRefusesNATDurableBuildsReject(t *testing.T) {
	opts := SetupOptions{}
	opts.applyDefaults()
	prefix := netip.MustParsePrefix(DefaultNATPrefix)
	host := netip.MustParseAddr(DefaultHostIP)
	tests := []struct {
		name string
		res  NetworkResources
		want []string // in the message and the fix; none: accepted
	}{
		{"the dev NAT",
			NetworkResources{NATName: "warden-dev-nat", NATPrefix: prefix,
				HostIP: host},
			nil},
		{"another NAT owns the prefix",
			NetworkResources{NATName: "warden-b1-nat", NATPrefix: prefix, HostIP: host,
				Reused: true},
			[]string{
				"the NAT warden-b1-nat already owns --nat-prefix " +
					"192.168.240.0/20",
				"use only the NAT warden-dev-nat",
				"Remove-NetNat -Name warden-b1-nat",
				"pass another --nat-prefix"}},
		{"the dev NAT on another prefix",
			NetworkResources{NATName: "warden-dev-nat",
				NATPrefix: netip.MustParsePrefix("10.200.0.0/24"), Reused: true},
			[]string{
				"already exists on 10.200.0.0/24, not --nat-prefix " +
					"192.168.240.0/20",
				"Remove-NetNat -Name warden-dev-nat"}},
	}
	for _, tt := range tests {
		p := &natProv{res: tt.res}
		res, err := ensureSetupNAT(context.Background(), p, opts)
		if p.spec.NATName != "warden-dev-nat" || p.spec.DevNAT != "" ||
			p.spec.NATPrefix != DefaultNATPrefix {
			t.Errorf("%s: EnsureNetwork got %+v", tt.name, p.spec)
		}
		if tt.want == nil {
			if err != nil || res == nil || res.NATName != "warden-dev-nat" {
				t.Errorf("%s: res %+v, err %v; want the dev NAT",
					tt.name, res, err)
			}
			continue
		}
		var fe *driver.FixableError
		if !errors.As(err, &fe) || res != nil {
			t.Errorf("%s: res %+v, err %v; want a refusal with a fix",
				tt.name, res, err)
			continue
		}
		for _, w := range tt.want {
			if !strings.Contains(fe.Msg+"\n"+fe.Fix, w) {
				t.Errorf("%s: %q / %q lacks %q", tt.name, fe.Msg, fe.Fix, w)
			}
		}
	}
}

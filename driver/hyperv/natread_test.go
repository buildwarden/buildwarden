package hyperv

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// TestParseEnsureNetworkReadback: EnsureNetwork's KEY=value lines, amid other
// output, give the chosen NAT, whether it was reused, and its prefix and the
// host's address in it.
func TestParseEnsureNetworkReadback(t *testing.T) {
	tests := []struct {
		name, out  string
		wantNAT    string
		wantReused bool
		wantPrefix string
		wantHost   string
	}{
		{"fresh per-build nat",
			"NAT=warden-abc123-nat\nREUSED=0\nPREFIX=192.168.240.0/20\n" +
				"HOSTIP=192.168.240.1\n",
			"warden-abc123-nat", false, "192.168.240.0/20", "192.168.240.1"},
		{"reused dev nat on the user's prefix",
			"noise\r\nNAT=warden-dev-nat\r\nREUSED=1\r\nPREFIX=10.200.0.0/24\r\n" +
				"HOSTIP=169.254.3.4\r\nHOSTIP=10.200.0.9\r\n",
			"warden-dev-nat", true, "10.200.0.0/24", "10.200.0.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseNetReadback(tt.out).ensureResources("warden-abc123")
			if err != nil {
				t.Fatal(err)
			}
			if res.NATName != tt.wantNAT || res.Reused != tt.wantReused ||
				res.NATPrefix != netip.MustParsePrefix(tt.wantPrefix) ||
				res.HostIP != netip.MustParseAddr(tt.wantHost) ||
				res.BuildSwitch != "warden-abc123" {
				t.Errorf("got %+v", res)
			}
		})
	}
}

// TestNATAddressingRefusal: a NAT whose host vNIC holds no address in its
// prefix is refused, naming setup.
func TestNATAddressingRefusal(t *testing.T) {
	r := parseNetReadback("NAT=warden-dev-nat\nREUSED=1\nPREFIX=10.200.0.0/24\n" +
		"HOSTIP=192.168.240.1\n")
	_, err := r.ensureResources("warden-x")
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "warden setup --driver hyperv") ||
		!strings.Contains(fe.Msg, "no address in its prefix 10.200.0.0/24") {
		t.Fatalf("err = %v", err)
	}
	if _, err := parseNetReadback("OK\n").ensureResources("warden-x"); err == nil {
		t.Error("output with no NAT accepted")
	}
}

// TestDurableResources: the durable NAT read back and its slot switches in
// index order (not the NAT switch, not another network's); each refusal names
// setup.
func TestDurableResources(t *testing.T) {
	const base = "NAT=warden-dev-nat\nPREFIX=10.200.0.0/24\nHOSTIP=10.200.0.1\n"
	res, err := parseNetReadback(base + "SLOT=warden-dev-10\tPrivate\n" +
		"SLOT=warden-dev-2\tPrivate\nSLOT=warden-dev-nat\tInternal\n" +
		"SLOT=warden-dev-x\tPrivate\nSLOT=warden-dev-0\tPrivate\n").
		durableResources("warden-dev")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Slots, ",") != "warden-dev-0,warden-dev-2,warden-dev-10" ||
		res.NATPrefix != netip.MustParsePrefix("10.200.0.0/24") ||
		res.HostIP != netip.MustParseAddr("10.200.0.1") ||
		res.NATName != "warden-dev-nat" {
		t.Errorf("got %+v", res)
	}
	for name, tt := range map[string]struct{ out, want string }{
		"no NAT": {"SLOT=warden-dev-0\tPrivate\n",
			"NAT \"warden-dev-nat\" is missing"},
		"no NAT switch": {base + "NATSWITCH=missing\nSLOT=warden-dev-0\tPrivate\n",
			"no vSwitch"},
		"no slots": {base + "SLOT=warden-dev-nat\tInternal\n",
			"has no build switches"},
		"internal slot": {base + "SLOT=warden-dev-0\tInternal\n", "isolation compromised"},
		"no host in pfx": {"NAT=warden-dev-nat\nPREFIX=10.200.0.0/24\n" +
			"SLOT=warden-dev-0\tPrivate\n", "no address in its prefix"},
	} {
		_, err := parseNetReadback(tt.out).durableResources("warden-dev")
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, tt.want) ||
			!strings.Contains(fe.Fix, "warden setup --driver hyperv") &&
				!strings.Contains(fe.Fix, "Remove-VMSwitch") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// TestEnsureNetworkScriptNATOrder: the dev switch's NAT is taken first, then
// one owning the requested prefix, then the named NAT, and only then is one
// made; a reused NAT is never re-addressed; setup's spec makes no Private
// switch.
func TestEnsureNetworkScriptNATOrder(t *testing.T) {
	s, err := ensureNetworkScript(NetworkSpec{SwitchName: "warden-abc",
		NATPrefix: DefaultNATPrefix, HostIP: DefaultHostIP, DevNAT: "warden-dev-nat"})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"Get-NetNat -Name 'warden-dev-nat'",
		"InternalIPInterfaceAddressPrefix -eq '192.168.240.0/20'",
		"Get-NetNat -Name 'warden-abc-nat'", "$reused = 0", "New-NetIPAddress",
		"New-NetNat -Name $natName", `"PREFIX=`, `"HOSTIP=`}
	at := 0
	for _, w := range order {
		i := strings.Index(s[at:], w)
		if i < 0 {
			t.Fatalf("script lacks %q after offset %d:\n%s", w, at, s)
		}
		at += i
	}
	if !strings.Contains(s, "if ('warden-abc')") {
		t.Error("the per-build switch is not made")
	}
	setup, err := ensureNetworkScript(NetworkSpec{NATName: "warden-dev-nat",
		NATPrefix: "10.200.0.0/24", HostIP: "10.200.0.1"})
	if err != nil || !strings.Contains(setup, "if ('')") ||
		!strings.Contains(setup, "$natName = 'warden-dev-nat'") {
		t.Errorf("setup script (%v):\n%s", err, setup)
	}
	if _, err := ensureNetworkScript(NetworkSpec{NATPrefix: DefaultNATPrefix}); err == nil {
		t.Error("a spec with no switch and no NAT name rendered")
	}
}

// TestNATAddressingTwoHostIPs (native F5): two host addresses in the NAT
// prefix are refused as ambiguous, never one picked; one in it beside an old
// address outside it is the host IP, and the old one is named as stray.
func TestNATAddressingTwoHostIPs(t *testing.T) {
	_, err := parseNetReadback("NAT=warden-dev-nat\nPREFIX=192.168.250.0/24\n" +
		"HOSTIP=192.168.250.1\nHOSTIP=192.168.250.7\n").ensureResources("warden-x")
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "192.168.250.1, 192.168.250.7") ||
		!strings.Contains(fe.Msg, "ambiguous") ||
		!strings.Contains(fe.Fix, "Remove-NetIPAddress -InterfaceAlias "+
			"'vEthernet (warden-dev-nat)'") {
		t.Fatalf("err = %v", err)
	}

	r := parseNetReadback("NAT=warden-dev-nat\nPREFIX=192.168.250.0/24\n" +
		"HOSTIP=192.168.240.1\nHOSTIP=192.168.250.1\n")
	res, err := r.ensureResources("warden-x")
	if err != nil || res.HostIP != netip.MustParseAddr("192.168.250.1") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if got := r.strayHostIPs(res.HostIP); len(got) != 1 || got[0] != "192.168.240.1" {
		t.Errorf("strayHostIPs = %v", got)
	}
}

// TestEnsureNetworkScriptRemovesOldAddresses (native F5): when the standup
// makes the NAT, every other IPv4 address on its own vNIC is removed before
// the host IP is added, and no other interface is named.
func TestEnsureNetworkScriptRemovesOldAddresses(t *testing.T) {
	s, err := ensureNetworkScript(NetworkSpec{NATName: "warden-dev-nat",
		NATPrefix: "192.168.250.0/24", HostIP: "192.168.250.1"})
	if err != nil {
		t.Fatal(err)
	}
	create := s[strings.Index(s, "$reused = 0"):]
	remove := "Get-NetIPAddress -InterfaceAlias $alias -AddressFamily IPv4 " +
		"-ErrorAction SilentlyContinue |\n        Where-Object { $_.IPAddress -ne " +
		"'192.168.250.1' } |\n        Remove-NetIPAddress -Confirm:$false"
	i := strings.Index(create, remove)
	if i < 0 || i > strings.Index(create, "New-NetIPAddress") ||
		i < strings.Index(create, `$alias = "vEthernet ($natName)"`) {
		t.Fatalf("the create branch does not remove the other addresses first:\n%s", s)
	}
	if strings.Count(s, "Remove-NetIPAddress") != 1 ||
		strings.Count(create, "Remove-NetIPAddress") != 1 {
		t.Errorf("addresses are removed outside the create branch:\n%s", s)
	}
}

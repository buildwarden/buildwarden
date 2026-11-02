package firecracker

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

var update = flag.Bool("update", false, "rewrite golden files")

// golden compares got with testdata/<name>, or rewrites it under -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from its golden file (go test -update to accept):\n%s", name,
			got)
	}
}

func TestSlotNamesAndAddresses(t *testing.T) {
	for _, tt := range []struct {
		slot        Slot
		host, relay string
	}{
		{0, "100.64.88.1", "100.64.88.2"},
		{63, "100.64.88.253", "100.64.88.254"},
		{64, "100.64.89.1", "100.64.89.2"},
		{255, "100.64.91.253", "100.64.91.254"},
	} {
		if got := tt.slot.HostAddr(); got != tt.host {
			t.Errorf("slot %d host = %s, want %s", tt.slot, got, tt.host)
		}
		if got := tt.slot.RelayAddr(); got != tt.relay {
			t.Errorf("slot %d relay = %s, want %s", tt.slot, got, tt.relay)
		}
	}
	s := Slot(7)
	if s.Bridge() != "wfc7b" || s.RelayTap() != "wfc7r" || s.GuestTap() != "wfc7g" ||
		s.UpstreamTap() != "wfc7u" || s.LockName() != "firecracker-slot-7" ||
		s.ClaimAddr() != "100.64.88.30:8301" {
		t.Errorf("slot 7 names: %s %s %s %s %s %s", s.Bridge(), s.RelayTap(), s.GuestTap(),
			s.UpstreamTap(), s.LockName(), s.ClaimAddr())
	}
}

func TestValidateSlotCount(t *testing.T) {
	for n, ok := range map[int]bool{0: false, 1: true, 16: true, 256: true, 257: false,
		-1: false} {
		err := ValidateSlotCount(n)
		if (err == nil) != ok {
			t.Errorf("ValidateSlotCount(%d) = %v", n, err)
		}
		var fe *driver.FixableError
		if err != nil && !errors.As(err, &fe) {
			t.Errorf("ValidateSlotCount(%d) is not fixable: %v", n, err)
		}
	}
}

func setupGoldenParams() setupParams {
	return setupParams{slots: 2, user: "jeff", uid: 1000, uplink: "eth0", uplinkMTU: 1280}
}

// TestSetupStepsGolden pins the whole setup command list for two slots on a
// 1280-byte uplink (TEST-04).
func TestSetupStepsGolden(t *testing.T) {
	var b strings.Builder
	for _, s := range setupSteps(setupGoldenParams()) {
		switch {
		case s.probe != nil:
			b.WriteString("unless " + strings.Join(s.probe, " ") + ": ")
		case s.untilFail:
			b.WriteString("until-fail: ")
		case s.ignore:
			b.WriteString("may-fail: ")
		}
		b.WriteString(strings.Join(s.args, " ") + "\n")
	}
	golden(t, "setup-2-slots.txt", []byte(b.String()))
}

func TestSetupStepsRules(t *testing.T) {
	steps := setupSteps(setupGoldenParams())
	var lines []string
	for _, s := range steps {
		lines = append(lines, strings.Join(s.args, " "))
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{
		`nft add rule bridge warden_fc input iifname wfc* drop`,
		`nft add rule bridge warden_fc forward iifname wfc1r oifname wfc1g accept`,
		`sysctl -qw net.ipv4.conf.wfc0b.rp_filter=1 net.ipv4.conf.wfc0u.rp_filter=1 ` +
			`net.ipv4.conf.wfc0b.arp_ignore=8`,
		`sysctl -qw net.ipv6.conf.wfc0g.disable_ipv6=1`,
		`iptables -w -A WARDEN-FC-IN -i wfc0u -s 100.64.88.2 -m conntrack --ctstate ` +
			`ESTABLISHED -j ACCEPT`,
		`iptables -w -A WARDEN-FC-FWD -i wfc1u -j DROP`,
		`--tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu`,
		`--mss 1241:65535 -j TCPMSS --set-mss 1240`,
		`iptables -w -A WARDEN-FC-OUT -o wfc1u -d 100.64.88.6 -p tcp --dport 8301 -m ` +
			`conntrack --ctstate NEW -m owner --uid-owner 1000 -j ACCEPT`,
		`iptables -w -A WARDEN-FC-OUT -o wfc+ -j DROP`,
		`iptables -w -t raw -A WARDEN-FC-RAW -i wfc1b -j CT --zone 2`,
		`iptables -w -t nat -A WARDEN-FC-NAT -s 100.64.88.0/22 -o eth0 -j MASQUERADE`,
		`ip6tables -w -A WARDEN-FC6 -i wfc+ -j DROP`,
		`ip6tables -w -I OUTPUT 1 -j WARDEN-FC6`,
		`iptables -w -I FORWARD 1 -j WARDEN-FC-FWD`,
		`ip tuntap add dev wfc1u mode tap user jeff`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("setup lacks %q", want)
		}
	}
	if last := lines[len(lines)-1]; last != "sysctl -qw net.ipv4.ip_forward=1" {
		t.Errorf("ip_forward is not enabled last: %q", last)
	}
	// The anti-spoof drop follows the slot's own accepts and precedes the
	// catch-all, so only the relay's address leaves its upstream.
	accept := strings.Index(all, "-A WARDEN-FC-FWD -i wfc0u -s 100.64.88.2 -o eth0 -j ACCEPT")
	drop := strings.Index(all, "-A WARDEN-FC-FWD -i wfc0u -j DROP")
	if accept < 0 || drop < accept {
		t.Errorf("anti-spoof order: accept at %d, drop at %d", accept, drop)
	}
}

func TestRunStepsProbesAndFailures(t *testing.T) {
	h := newFakeHost()
	h.fail["ip link show wfc0g"] = true
	h.fail["iptables -w -F GONE"] = true
	steps := []step{
		{args: []string{"ip", "link", "add", "wfc0b"}, probe: []string{"ip", "link",
			"show", "wfc0b"}},
		{args: []string{"ip", "tuntap", "add", "wfc0g"}, probe: []string{"ip", "link",
			"show", "wfc0g"}},
		tolerant("iptables", "-w", "-F", "GONE"),
	}
	created, verified, err := runSteps(h, steps)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0] != "wfc0g" || len(verified) != 1 ||
		verified[0] != "wfc0b" {
		t.Fatalf("created %v verified %v", created, verified)
	}
	if ranContains(h, "ip link add wfc0b") {
		t.Fatal("recreated an interface that exists")
	}
	h.fail["nft add table bridge warden_fc"] = true
	if _, _, err := runSteps(h, []step{cmd("nft", "add", "table", "bridge",
		"warden_fc")}); err == nil {
		t.Fatal("a failing required step did not fail setup")
	}
}

// postureHost is a fake root host ready for setup.
func postureHost() *fakeHost {
	h := newFakeHost()
	h.uid = 0
	h.env["SUDO_USER"] = "jeff"
	h.files["/dev/kvm"] = ""
	h.fail["systemctl is-active -q firewalld"] = true
	h.outputs["iptables -w -S FORWARD"] = "-P FORWARD DROP\n"
	h.outputs["nft -j list chains"] = `{"nftables":[{"metainfo":{}}]}`
	h.outputs["ip route show default"] = "default via 172.20.0.1 dev eth0 proto kernel\n"
	h.files["/sys/class/net/eth0/mtu"] = "1280\n"
	return h
}

func TestSetupRefusesUnsafePosture(t *testing.T) {
	for name, tt := range map[string]struct {
		mut     func(*fakeHost)
		wantFix string
	}{
		"not root":     {func(h *fakeHost) { h.uid = 1000 }, "sudo warden setup"},
		"no sudo user": {func(h *fakeHost) { delete(h.env, "SUDO_USER") }, "sudo"},
		"no kvm group": {func(h *fakeHost) { h.inKVM = false }, "usermod -aG kvm jeff"},
		"no kvm":       {func(h *fakeHost) { delete(h.files, "/dev/kvm") }, "kvm_intel"},
		"nft missing":  {func(h *fakeHost) { h.missing["nft"] = true }, "install"},
		"firewalld": {func(h *fakeHost) {
			delete(h.fail,
				"systemctl is-active -q firewalld")
		}, "--zone=trusted"},
		"forward accept": {func(h *fakeHost) {
			h.outputs["iptables -w -S "+
				"FORWARD"] = "-P FORWARD ACCEPT\n"
		}, "iptables -P FORWARD DROP"},
		"nft drops": {func(h *fakeHost) {
			h.outputs["nft -j list "+
				"chains"] = nftDropJSON
		}, `nft insert rule inet filter forward iifname ` +
			`"wfc*u" accept`},
		"no default": {func(h *fakeHost) { h.outputs["ip route show default"] = "" },
			"default route"},
		"too many slots": {nil, "--slots between 1 and 256"},
	} {
		t.Run(name, func(t *testing.T) {
			h := postureHost()
			opts := SetupOptions{Slots: 2}
			if tt.mut != nil {
				tt.mut(h)
			} else {
				opts.Slots = 300
			}
			err := setup(h, &bytes.Buffer{}, opts)
			var fe *driver.FixableError
			if !errors.As(err, &fe) || !strings.Contains(fe.Fix, tt.wantFix) {
				t.Fatalf("setup = %v, want a fix containing %q", err, tt.wantFix)
			}
			if ranContains(h, "ip_forward") {
				t.Fatal("refused setup still changed the host")
			}
		})
	}
}

func TestSetupRuns(t *testing.T) {
	h := postureHost()
	h.fail["ip link show wfc1b"] = true
	var out bytes.Buffer
	if err := setup(h, &out, SetupOptions{Slots: 2}); err != nil {
		t.Fatal(err)
	}
	if !ranContains(h, "sysctl -qw net.ipv4.ip_forward=1") ||
		!ranContains(h, "--set-mss 1240") || !ranContains(h,
		"ip link add wfc1b type bridge") {
		t.Fatalf("setup ran:\n%s", strings.Join(h.ran, "\n"))
	}
	if !strings.Contains(out.String(),
		"2 slot(s) for jeff (uid 1000), uplink eth0 (mtu 1280)") {
		t.Fatalf("setup printed %q", out.String())
	}
}

func TestUplinkFromRoute(t *testing.T) {
	if got := uplinkFromRoute("default via 10.0.0.1 dev enp3s0 proto dhcp metric " +
		"100\n"); got != "enp3s0" {
		t.Fatalf("uplink = %q", got)
	}
	if got := uplinkFromRoute(""); got != "" {
		t.Fatalf("uplink of no route = %q", got)
	}
}

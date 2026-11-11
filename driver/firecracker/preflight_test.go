package firecracker

import (
	"bytes"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/resolvers"
)

func TestParseVersion(t *testing.T) {
	for out, want := range map[string][3]int{
		"Firecracker v1.17.0\n\nSupported snapshot data format versions: 8.0.0\n": {1, 17,
			0},
		"Firecracker v1.9.1-dev": {1, 9, 1},
	} {
		got, ok := parseVersion(out)
		if !ok || got != want {
			t.Errorf("parseVersion(%q) = %v, %v", out, got, ok)
		}
	}
	if _, ok := parseVersion("firecracker"); ok {
		t.Error("no version parsed as one")
	}
	if !versionAtLeast([3]int{1, 17, 0}, minVersion) || !versionAtLeast([3]int{2, 0, 0},
		minVersion) ||
		versionAtLeast([3]int{1, 16, 9}, minVersion) {
		t.Error("versionAtLeast")
	}
}

func fixOf(t *testing.T, err error) string {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("not a fixable error: %v", err)
	}
	return fe.Fix
}

// TestPreflightMessages checks each preflight failure says what to do
// (UX-04), in the order a user would fix them.
func TestPreflightMessages(t *testing.T) {
	supportedHost(t)
	ok := func() *fakeHost {
		h := newFakeHost().withSlots(2, 1000)
		h.files["/dev/kvm"] = ""
		h.outputs["firecracker --version"] = "Firecracker v1.17.0\n"
		h.files[resolvers.ResolvConf] = "nameserver 8.8.8.8\n"
		return h
	}
	none := func(net.IP) bool { return false }
	for name, tt := range map[string]struct {
		mut  func(*fakeHost)
		want string
	}{
		"no kvm":     {func(h *fakeHost) { delete(h.files, "/dev/kvm") }, "kvm_intel"},
		"kvm not rw": {func(h *fakeHost) { h.noRW = true }, "usermod -aG kvm"},
		"no firecracker": {func(h *fakeHost) { h.missing["firecracker"] = true },
			"v1.17.0"},
		"old firecracker": {func(h *fakeHost) {
			h.outputs["firecracker --version"] = "Firecracker v1.10.1\n"
		}, "releases"},
	} {
		t.Run(name, func(t *testing.T) {
			h := ok()
			tt.mut(h)
			_, err := preflight(h, none, buildlink.Default(), nil)
			if fix := fixOf(t, err); !strings.Contains(fix, tt.want) {
				t.Fatalf("fix %q lacks %q", fix, tt.want)
			}
		})
	}
	if err := checkDiskTools(func() *fakeHost {
		h := newFakeHost()
		h.missing["genisoimage"], h.missing["mkisofs"] = true, true
		return h
	}()); !strings.Contains(fixOf(t, err), "genisoimage") {
		t.Fatal("missing ISO tool not named")
	}
}

// TestRequestPreflightDNS: the resolvers preflight chooses replace req.DNS,
// which ClaimRelay sends as upstream_dns: the host's when [vm] dns is unset,
// and [vm] dns itself, with no host resolv.conf to read, when it is set.
func TestRequestPreflightDNS(t *testing.T) {
	supportedHost(t)
	prev := assetsCheck
	assetsCheck = func() (assets, error) { return assets{}, nil }
	t.Cleanup(func() { assetsCheck = prev })
	h := newFakeHost().withSlots(1, 1000)
	h.files["/dev/kvm"] = ""
	h.outputs["firecracker --version"] = "Firecracker v1.17.0\n"
	h.files[resolvers.ResolvConf] = "nameserver 10.0.0.2\nnameserver 127.0.0.53\n"
	none := func(net.IP) bool { return false }
	req := &driver.BuildRequest{}
	if _, err := requestPreflight(h, none, req); err != nil ||
		resolvers.Join(req.DNS) != "10.0.0.2" {
		t.Fatalf("host selection: %v %v", req.DNS, err)
	}
	delete(h.files, resolvers.ResolvConf)
	req = &driver.BuildRequest{DNS: addrs("10.20.0.53", "10.20.0.54")}
	if _, err := requestPreflight(h, none, req); err != nil ||
		resolvers.Join(req.DNS) != "10.20.0.53, 10.20.0.54" {
		t.Fatalf("[vm] dns: %v %v", req.DNS, err)
	}
}

func TestOwnedSlots(t *testing.T) {
	h := newFakeHost().withSlots(3, 1000)
	h.files["/sys/class/net/wfc1g/owner"] = "1001\n"
	got := ownedSlots(h, 1000)
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("owned = %v, want [0 2]", got)
	}
	root := newFakeHost().withSlots(1, 1000)
	root.uid, root.env["SUDO_USER"] = 0, "jeff"
	if got, err := checkSlots(root); err != nil || len(got) != 1 {
		t.Fatalf("sudo doctor sees %v, %v; want the invoking user's slot", got, err)
	}
	_, err := checkSlots(newFakeHost())
	if !strings.Contains(fixOf(t, err), "warden setup --driver firecracker") {
		t.Fatal("no-slot fix does not name setup")
	}
}

func TestDoctorReport(t *testing.T) {
	supportedHost(t)
	h := newFakeHost().withSlots(1, 1000)
	h.files["/dev/kvm"] = ""
	h.outputs["firecracker --version"] = "Firecracker v1.17.0\n"
	h.files[resolvers.ResolvConf] = "nameserver 8.8.8.8\n"
	h.fail["systemctl is-active -q firewalld"] = true
	h.noRefl = true
	for key, v := range sysctlWants([]Slot{0}) {
		h.files["/proc/sys/"+key] = v + "\n"
	}
	var out bytes.Buffer
	doctor(h, &out, func(net.IP) bool { return false }, buildlink.Default(), nil)
	s := out.String()
	for _, want := range []string{
		"ok    /dev/kvm", "ok    firecracker", "ok    slot sysctls",
		"ok    network slots owned by this user: 1 (wfc0 to wfc0)",
		"skip  rules and FORWARD policy", "warn  reflink", "ok    upstream DNS for the " +
			"relay (from /etc/resolv.conf): 8.8.8.8",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, s)
		}
	}
	out.Reset()
	doctor(h, &out, func(net.IP) bool { return false }, buildlink.Default(),
		addrs("10.20.0.53"))
	want := "ok    upstream DNS for the relay (from [vm] dns): 10.20.0.53"
	if !strings.Contains(out.String(), want) {
		t.Errorf("doctor output lacks %q:\n%s", want, out.String())
	}
	h.files["/proc/sys/net/ipv4/ip_forward"] = "0\n"
	out.Reset()
	if !doctor(h, &out, func(net.IP) bool { return false }, buildlink.Default(), nil) ||
		!strings.Contains(out.String(), "FAIL  slot sysctls") {
		t.Fatalf("ip_forward=0 not reported:\n%s", out.String())
	}
}

func TestCheckRules(t *testing.T) {
	h := newFakeHost()
	for _, j := range ruleJumps {
		h.outputs[strings.Join(j.cmd("-t", j.table, "-S", j.hook), " ")] =
			"-P " + j.hook + " DROP\n-A " + j.hook + " -j " + j.chain + "\n-A " +
				j.hook + " -j OTHER\n"
	}
	if err := checkRules(h); err != nil {
		t.Fatal(err)
	}
	h.outputs["iptables -w -t filter -S OUTPUT"] = "-P OUTPUT ACCEPT\n-A OUTPUT -j " +
		"DOCKER\n-A OUTPUT -j WARDEN-FC-OUT\n"
	if err := checkRules(h); err == nil || !strings.Contains(err.Error(), "WARDEN-FC-OUT") {
		t.Fatalf("a jump that is not first passed: %v", err)
	}
}

func TestBuildIDOf(t *testing.T) {
	for name, want := range map[string]string{
		"warden-firecracker-a1B2c3D4-123456": "a1B2c3D4",
		"a1B2c3D4":                           "a1B2c3D4",
		"reflink-probe-1":                    "",
		"warden-qemu-x-1":                    "",
	} {
		if got := buildIDOf(name); got != want {
			t.Errorf("buildIDOf(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestDoctorUnderSudo checks that `sudo warden doctor` reports on the user
// who ran sudo, not root, and writes nothing into that user's cache.
func TestDoctorUnderSudo(t *testing.T) {
	supportedHost(t)
	h := postureHost().withSlots(2, 1000)
	h.inKVM = false
	h.outputs["firecracker --version"] = "Firecracker v1.17.0\n"
	h.files[resolvers.ResolvConf] = "nameserver 8.8.8.8\n"
	// Slot 0's VMM outlived its warden; slot 1's build is still running.
	for _, s := range []Slot{0, 1} {
		h.files[slotLockPath(s)] = ""
		h.files[filepath.Join(filepath.FromSlash(jeffLocks), s.LockName()+".pids")] =
			"4242 987654 - /usr/local/bin/firecracker\n"
	}
	h.heldLocks[slotLockPath(1)] = true
	h.files["/proc/4242/stat"] = statLine
	h.links["/proc/4242/exe"] = "/usr/local/bin/firecracker"
	h.noRefl = true
	var out bytes.Buffer
	doctor(h, &out, func(net.IP) bool { return false }, buildlink.Default(), nil)
	s := out.String()
	for _, want := range []string{
		"FAIL  /dev/kvm", "sudo usermod -aG kvm jeff",
		"network slots owned by this user: 2",
		"skip  reflink copies on the cache filesystem",
		"warn  orphaned VMMs", "VMMs with no warden: slot 0 pid 4242\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("sudo doctor output lacks %q:\n%s", want, s)
		}
	}
	if len(h.locked) != 1 || h.locked[0] != slotLockPath(0) ||
		len(h.unlocked) != 1 {
		t.Errorf("sudo doctor locked %v, released %v; want slot 0's existing lock",
			h.locked, h.unlocked)
	}
}

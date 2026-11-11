package main

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// inConfigFiles points the user config at a fresh home and runs from a fresh
// project directory, writing each non-empty body as that level's file.
func inConfigFiles(t *testing.T, user, project string) string {
	t.Helper()
	home, proj := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if user != "" {
		dir := filepath.Join(home, ".config", "warden")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, dir, "config.toml", user)
	}
	if project != "" {
		write(t, proj, "warden.toml", project)
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return proj
}

func vmDNSFile(entries string) string { return "[vm]\ndns = [" + entries + "]\n" }

func addrList(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

func TestVMDNSAtLoad(t *testing.T) {
	tests := []struct {
		name, user, project string
		want                []netip.Addr
		source              string
	}{
		{"unset", "", "", nil, ""},
		{"user config", vmDNSFile(`"10.20.0.53", "10.20.0.54"`), "",
			addrList("10.20.0.53", "10.20.0.54"), "config.toml"},
		{"project over user", vmDNSFile(`"10.20.0.53", "10.20.0.54"`),
			vmDNSFile(`"10.30.0.53"`), addrList("10.30.0.53"), "warden.toml"},
		{"project without the key keeps the user's", vmDNSFile(`"10.20.0.53"`),
			"[vm]\ncpus = 2\n", addrList("10.20.0.53"), "config.toml"},
		// Loopback is left to the drivers' rules: vz's relay runs on the host.
		{"loopback accepted at load", "", vmDNSFile(`"127.0.0.1"`),
			addrList("127.0.0.1"), "warden.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inConfigFiles(t, tt.user, tt.project)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(cfg.VM.DNSAddrs, tt.want) ||
				cfg.VM.DNSSource != tt.source {
				t.Fatalf("[vm] dns = %v (%q), want %v (%q)", cfg.VM.DNSAddrs,
					cfg.VM.DNSSource, tt.want, tt.source)
			}
		})
	}
}

func TestBadVMDNSFailsAtLoad(t *testing.T) {
	for entries, want := range map[string]string{
		`"fd00::53"`:                  "not IPv4",
		`"0.0.0.0"`:                   "unspecified",
		`"224.0.0.251"`:               "multicast",
		`"255.255.255.255"`:           "broadcast",
		`"dns.example.com"`:           "not an IP address",
		`"10.20.0.53", "10.20.0.300"`: "entry 1",
		``:                            "is empty",
	} {
		inConfigFiles(t, "", vmDNSFile(entries))
		_, err := LoadConfig()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "[vm] dns in warden.toml") ||
			!strings.Contains(fe.Msg, want) ||
			!strings.Contains(fe.Fix, "remove [vm] dns") {
			t.Errorf("dns = [%s]: err = %#v, want it to say %q", entries, err, want)
		}
	}
	// The empty list's fix is the one way to get the default.
	inConfigFiles(t, "", vmDNSFile(""))
	_, err := LoadConfig()
	var fe *driver.FixableError
	if !errors.As(err, &fe) ||
		!strings.Contains(fe.Fix, "to use the host's resolvers") {
		t.Errorf("dns = []: err = %#v", err)
	}
}

func TestBuildRequestDNSForVMDriversOnly(t *testing.T) {
	defer func() { flagDriver = "" }()
	proj := inConfigFiles(t, "", vmDNSFile(`"10.20.0.53", "10.20.0.54"`))
	write(t, proj, "Dockerfile", "FROM alpine")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	for d, want := range map[string][]netip.Addr{
		"qemu":        addrList("10.20.0.53", "10.20.0.54"),
		"vz":          addrList("10.20.0.53", "10.20.0.54"),
		"hyperv":      addrList("10.20.0.53", "10.20.0.54"),
		"firecracker": addrList("10.20.0.53", "10.20.0.54"),
		"container":   nil,
		"":            nil,
	} {
		cfg.Runtime.Driver = d
		req, err := newRequest(cfg, []string{proj})
		if err != nil {
			t.Fatalf("%q: %v", d, err)
		}
		if !slices.Equal(req.DNS, want) {
			t.Errorf("%q: BuildRequest.DNS = %v, want %v", d, req.DNS, want)
		}
	}
}

func TestSetupSlots(t *testing.T) {
	defer func() { flagDriver, flagSetupSlots = "", 0 }()
	inConfigFiles(t, "", "")
	// Out-of-range counts are refused before any host work, so these run on
	// every platform without touching it.
	for _, c := range []struct {
		driver string
		slots  int
		want   string
	}{
		{"hyperv", 65, "--slots 65 is out of range (1-64)"},
		{"hyperv", -1, "--slots -1 is out of range (1-64)"},
		{"firecracker", 257, "--slots 257 is out of range (1-256)"},
		{"qemu", 4, "--slots is only used by --driver hyperv and firecracker"},
		{"vz", 4, "--slots is only used by --driver hyperv and firecracker"},
		{"container", 4, "--slots is only used by --driver hyperv and firecracker"},
	} {
		flagDriver, flagSetupSlots = c.driver, c.slots
		err := setupCmd.RunE(setupCmd, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("--driver %s --slots %d: err = %v, want %q", c.driver, c.slots,
				err, c.want)
		}
	}
	help := setupCmd.Flags().Lookup("slots").Usage
	for _, want := range []string{"hyperv: build switches to create, 1-64 (default 4)",
		"firecracker: network slots to create, 1-256 (default 16)"} {
		if !strings.Contains(help, want) {
			t.Errorf("--slots help %q does not say %q", help, want)
		}
	}
}

func TestDoctorResolvers(t *testing.T) {
	ok := func(buildlink.Link, []netip.Addr) ([]netip.Addr, string, error) {
		return addrList("10.20.0.53", "10.20.0.54"), "[vm] dns", nil
	}
	refused := func(buildlink.Link, []netip.Addr) ([]netip.Addr, string, error) {
		return nil, "/etc/resolv.conf", driver.Fixable("set [vm] dns",
			"the host has only resolvers it cannot reach")
	}
	var out strings.Builder
	if doctorResolvers(&out, ok, buildlink.Link{}, nil) ||
		out.String() != "relay resolvers: 10.20.0.53, 10.20.0.54 (from [vm] dns)\n" {
		t.Errorf("usable list: %q", out.String())
	}
	out.Reset()
	if !doctorResolvers(&out, refused, buildlink.Link{}, nil) ||
		out.String() != "relay resolvers (from /etc/resolv.conf): BLOCKED: the "+
			"host has only resolvers it cannot reach\n"+
			"relay resolvers: to fix: set [vm] dns\n" {
		t.Errorf("refusal: %q", out.String())
	}
}

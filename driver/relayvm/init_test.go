package relayvm

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The relay VM init and its Hyper-V build script are shell, checked here as
// text: nothing runs them. Their boot paths are exercised on native hosts.

func readTool(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// code drops the comment lines, so a check reads only what the shell runs.
func code(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// seedModules are the relay seed's storage, SCSI CD-ROM and ISO 9660
// modules, dependencies first, as build.sh copies and the init loads them.
var seedModules = []string{"scsi_common", "scsi_mod", "scsi_transport_fc", "hv_storvsc",
	"cdrom", "sr_mod", "isofs"}

func TestRelayInitKeyReader(t *testing.T) {
	s := code(readTool(t, "tools/relay-vm/init"))
	// One reader for both sources, and no other loop over the command line.
	if n := strings.Count(s, "\nread_keys() {"); n != 1 {
		t.Errorf("the init defines read_keys %d times, want once", n)
	}
	for _, call := range []string{"read_keys /proc/cmdline < /proc/cmdline",
		`read_keys "the relay seed" < /tmp/warden-relay.conf`} {
		if !strings.Contains(s, call) {
			t.Errorf("the init lacks %q", call)
		}
	}
	if n := strings.Count(s, "/proc/cmdline"); n != 2 {
		t.Errorf("the init reads /proc/cmdline outside read_keys (%d mentions)", n)
	}
	for _, banned := range []string{"eval ", ". /", "source ", "UPSTREAM_DNS",
		"NAT_IP=1", "NAT_IP=2", "NAT_IP=9"} {
		if strings.Contains(s, banned) {
			t.Errorf("the init contains %q", banned)
		}
	}
	if !strings.Contains(s, "set -f") {
		t.Error("the init does not turn off glob expansion of the words it reads")
	}
	// Each value is checked, and warden.dns stops the init naming the claim
	// field that replaced it.
	for _, want := range []string{
		"dhcp|static) ;;",
		`is_quad "$v" || stop "warden.nat_ip=`,
		`is_quad "$v" || stop "warden.nat_gw=`,
		"[1-9]|[12][0-9]|30) ;;",
		"*[!0-9a-f]*)",
		`[ "${#v}" -eq 64 ]`,
		`[ "$v" = 1 ] ||`,
		"no relay seed (WARDENRELAY)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the init lacks the check %q", want)
		}
	}
	if !regexp.MustCompile(`warden\.dns=\*\)\s*\n\s*stop "[^"]*" \\\n\s*"[^"]*upstream_dns`).
		MatchString(s) {
		t.Error("warden.dns does not stop the init naming upstream_dns")
	}
	// A static upstream needs all three values, and a failed lease stops.
	if !strings.Contains(s, "warden.net=static needs warden.nat_ip, warden.nat_prefix "+
		"and warden.nat_gw") ||
		!regexp.MustCompile(`udhcpc [^\n]*\|\|\s*stop "no DHCP lease on`).MatchString(s) {
		t.Error("the init does not stop on a partial static upstream or a failed lease")
	}
}

func TestRelayInitSeedOnlyOnHyperV(t *testing.T) {
	s := code(readTool(t, "tools/relay-vm/init"))
	block := regexp.MustCompile(`(?s)\nif \[ "\$HYPERV" = 1 \]; then\n(.*?)\nfi\n`).
		FindAllStringSubmatch(s, -1)
	seed := ""
	for _, m := range block {
		if strings.Contains(m[1], "read_seed") {
			seed = m[1]
		}
	}
	if seed == "" {
		t.Fatal("no HYPERV block reads the seed")
	}
	if n := strings.Count(s, "\n    read_seed\n"); n != 1 ||
		!strings.Contains(seed, "\n    read_seed\n") {
		t.Errorf("read_seed is called %d times, or outside the HYPERV block", n)
	}
	for _, m := range seedModules {
		if !strings.Contains(seed, m) {
			t.Errorf("the HYPERV block does not load %s before reading the seed", m)
		}
	}
	for _, want := range []string{"-lt 100", "mount -t iso9660 -o ro /dev/sr0",
		"cp /run/seed/warden-relay.conf /tmp/warden-relay.conf", "umount /run/seed"} {
		if !strings.Contains(s, want) {
			t.Errorf("read_seed lacks %q", want)
		}
	}
}

func TestRelayInitWritesNoNetworkValue(t *testing.T) {
	s := readTool(t, "tools/relay-vm/init")
	// The udhcpc lease script writes the lease's resolvers, which the relay's
	// absent-field default reads; nothing else writes resolv.conf.
	lease := regexp.MustCompile(`(?s)<< 'DHCP'\n.*?\nDHCP\n`)
	if !lease.MatchString(s) {
		t.Fatal("no udhcpc lease script heredoc")
	}
	if strings.Contains(lease.ReplaceAllString(s, ""), "resolv.conf") {
		t.Error("the init writes resolv.conf outside the udhcpc lease script")
	}
	if m := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`).
		FindAllString(s, -1); len(m) > 0 {
		t.Errorf("the init carries IPv4 literals %v", m)
	}
	// Console lines name keys, never the token.
	for i, l := range strings.Split(code(s), "\n") {
		if regexp.MustCompile(`\b(say|stop|echo)\b`).MatchString(l) &&
			strings.Contains(l, "CLAIM_TOKEN") {
			t.Errorf("line %d prints the token: %s", i+1, l)
		}
	}
	tok := regexp.MustCompile(`(?s)warden\.claim_token=\*\)(.*?);;`).FindStringSubmatch(s)
	if tok == nil {
		t.Fatal("no claim_token case")
	}
	if regexp.MustCompile(`stop "[^"]*\$v`).MatchString(tok[1]) {
		t.Error("the claim_token check prints the value it refuses")
	}
}

func TestRelayBuildShipsSeedModules(t *testing.T) {
	s := code(readTool(t, "tools/relay-vm/hyperv/build.sh"))
	for _, m := range []string{"drivers/scsi/scsi_common", "drivers/scsi/scsi_mod",
		"drivers/scsi/scsi_transport_fc", "drivers/scsi/hv_storvsc",
		"drivers/cdrom/cdrom", "drivers/scsi/sr_mod", "fs/isofs/isofs"} {
		if !strings.Contains(s, m+".ko.gz") {
			t.Errorf("build.sh does not copy %s.ko.gz", m)
		}
	}
	// Dependencies first, in the order the init loads them.
	last := -1
	for _, m := range seedModules {
		i := strings.Index(s, "/"+m+".ko.gz")
		if i < last {
			t.Errorf("build.sh lists %s before a module it depends on", m)
		}
		last = i
	}
}

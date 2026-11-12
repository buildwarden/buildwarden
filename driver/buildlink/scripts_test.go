package buildlink

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The launcher scripts are shell, checked here as text: nothing runs them.
// Their choice between lease and keys is exercised on native hosts.

func readTool(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestStampScriptsWriteChannel: each tool that builds a launcher asset
// compared with Channel writes that stamp, so a bump cannot miss one. The
// Hyper-V relay boot VHDX's refresh-cache.sh writes driver/hyperv's own
// relayBootChannel instead, checked there.
func TestStampScriptsWriteChannel(t *testing.T) {
	want := fmt.Sprintf("%s=%d", StampKey, Channel)
	for script, writes := range map[string][]string{
		"tools/build-vm/build-initramfs.sh":       {`echo "` + want + `" >`},
		"tools/firecracker-vm/build-initramfs.sh": {`echo "` + want + `" >`},
	} {
		s := readTool(t, script)
		for _, w := range writes {
			if !strings.Contains(s, w) {
				t.Errorf("%s lacks %q", script, w)
			}
		}
		if n := strings.Count(s, StampKey+"="); n != 1 {
			t.Errorf("%s names %s %d times, want once", script, StampKey, n)
		}
	}
}

// leaseScript extracts the udhcpc script a build-initramfs.sh installs.
func leaseScript(t *testing.T, s string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<<'LEASE'\n(.*?)\nLEASE\n`).FindStringSubmatch(s)
	if m == nil {
		t.Fatal("no lease script heredoc")
	}
	return m[1]
}

// linkBlock extracts the part of a direct-boot init that takes the build
// link: from its stop() helper to the default route. The two inits keep a
// copy each, since each initramfs is built from its own tools/ directory
// (and Firecracker's CI cache key hashes tools/firecracker-vm, not
// tools/build-vm), so this pins the copies against drift.
func linkBlock(t *testing.T, s string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)\nstop\(\) \{\n.*?\nip route add default via "\$LINK_GW"\n`).
		FindString(s)
	if m == "" {
		t.Fatal("no build-link block")
	}
	return m
}

// TestDirectBootInitLinkBlocks: the two direct-boot inits take the build link
// the same way.
func TestDirectBootInitLinkBlocks(t *testing.T) {
	if linkBlock(t, readTool(t, "tools/build-vm/init")) !=
		linkBlock(t, readTool(t, "tools/firecracker-vm/init")) {
		t.Error("the two direct-boot inits take the build link differently; " +
			"keep their link blocks (stop() to the default route) identical")
	}
}

func TestDirectBootInits(t *testing.T) {
	qemuBuild := readTool(t, "tools/build-vm/build-initramfs.sh")
	fcBuild := readTool(t, "tools/firecracker-vm/build-initramfs.sh")
	lease := leaseScript(t, qemuBuild)
	if leaseScript(t, fcBuild) != lease {
		t.Error("the two build scripts install different lease scripts")
	}
	for _, init := range []string{"tools/build-vm/init", "tools/firecracker-vm/init"} {
		s := readTool(t, init)
		for _, want := range []string{
			`udhcpc -i "$IF" -f -q -n `,
			"-s /etc/warden-lease.sh",
			"$(cat /proc/cmdline)",
			"warden.link_ip=*)",
			"warden.link_gw=*)",
			`echo "init: no DHCP answer; using warden.link_ip"`,
			`echo "init: link $LINK_IP/$LINK_LEN via $LINK_GW from $SOURCE"`,
			"SOURCE=dhcp",
			"SOURCE=cmdline",
			"disagrees with warden.link_ip=",
			`initialize --gateway="$LINK_GW"`,
		} {
			if !strings.Contains(s, want) {
				t.Errorf("%s lacks %q", init, want)
			}
		}
		for _, banned := range []string{"eval ", ". /proc/cmdline", "source ",
			"10.0.0.", "100.127.255."} {
			if strings.Contains(s, banned) {
				t.Errorf("%s contains %q", init, banned)
			}
		}
	}
	if !strings.Contains(readTool(t, "tools/build-vm/init"), "af_packet") ||
		!strings.Contains(qemuBuild, "net/packet/af_packet.ko.gz") ||
		!regexp.MustCompile(`(?s)for cmd in .*udhcpc.*; do`).MatchString(qemuBuild) {
		t.Error("tools/build-vm does not ship and load udhcpc's af_packet and applet")
	}
	if !strings.Contains(readTool(t, "tools/firecracker-vm/kernel-required.config"),
		"CONFIG_PACKET=y") {
		t.Error("the firecracker kernel no longer builds in CONFIG_PACKET")
	}
	writes := regexp.MustCompile(`>\s*(\S+)`).FindAllStringSubmatch(lease, -1)
	if len(writes) != 1 || writes[0][1] != "/tmp/warden-lease" {
		t.Errorf("the lease script writes %v, want only /tmp/warden-lease", writes)
	}
	for _, banned := range []string{"ip addr", "ip route", "resolv.conf", "ifconfig"} {
		if strings.Contains(lease, banned) {
			t.Errorf("the lease script configures something: %q", banned)
		}
	}
}

// TestDirectBootBusyboxPins: the two direct-boot build scripts fetch the same
// Alpine busybox-static, each for its own architecture, and check it. Each
// keeps its own fetch (Firecracker's CI cache key hashes only
// tools/firecracker-vm), so this makes a re-pin touch both.
func TestDirectBootBusyboxPins(t *testing.T) {
	const (
		qemuBuild = "tools/build-vm/build-initramfs.sh"
		fcBuild   = "tools/firecracker-vm/build-initramfs.sh"
		check     = `echo "$BUSYBOX_SHA256  $BUSYBOX_APK" | `
	)
	sha := regexp.MustCompile(`^[0-9a-f]{64}$`)
	pins := map[string]map[string]string{}
	for _, script := range []string{qemuBuild, fcBuild} {
		s := readTool(t, script)
		pins[script] = map[string]string{}
		for _, name := range []string{
			"ALPINE_VERSION", "BUSYBOX_VERSION", "BUSYBOX_SHA256",
		} {
			m := regexp.MustCompile(`(?m)^`+name+`="([^"]*)"$`).
				FindAllStringSubmatch(s, -1)
			if len(m) != 1 {
				t.Fatalf("%s sets %s %d times, want once", script, name, len(m))
			}
			pins[script][name] = m[0][1]
		}
		if !sha.MatchString(pins[script]["BUSYBOX_SHA256"]) {
			t.Errorf("%s: BUSYBOX_SHA256 %q is not a sha256",
				script, pins[script]["BUSYBOX_SHA256"])
		}
		if !strings.Contains(s, check+"sha256sum -c -") {
			t.Errorf("%s does not check BUSYBOX_SHA256", script)
		}
	}
	for _, name := range []string{"ALPINE_VERSION", "BUSYBOX_VERSION"} {
		if q, f := pins[qemuBuild][name], pins[fcBuild][name]; q != f {
			t.Errorf("%s differs: tools/build-vm %q, "+
				"tools/firecracker-vm %q; re-pin both", name, q, f)
		}
	}
	s := readTool(t, qemuBuild)
	if strings.Contains(s, "$RELAY_CACHE/busybox") {
		t.Error("tools/build-vm reads busybox from the relay cache, " +
			"which no longer has it")
	}
	if !strings.Contains(s, check+"shasum -a 256 -c -") {
		t.Error("tools/build-vm cannot check its busybox without sha256sum (macOS)")
	}
}

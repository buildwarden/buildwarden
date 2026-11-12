package firecracker

import (
	"encoding/json"
	"strings"
	"testing"
)

func configJSON(t *testing.T, c vmConfig) []byte {
	t.Helper()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// TestVMConfigGolden pins the three Firecracker configs: the relay with its
// entropy device and two NICs, and both build VM boots with one NIC, no
// vsock, no MMDS and no API socket settings (TEST-04).
func TestVMConfigGolden(t *testing.T) {
	s := Slot(3)
	relay := relayConfig(s, "/a/vmlinux", "/w/relay-initrd",
		relayArgs(s, "TOKEN"))
	golden(t, "relay.json", configJSON(t, relay))

	disk := buildConfig(s, buildBoot{Kernel: "/a/vmlinux", Rootfs: "/c/work/id/rootfs.raw",
		PartUUID: partUUIDLinux, Seed: "/w/seed.iso"}, 4, 4096)
	golden(t, "build-disk.json", configJSON(t, disk))

	direct := buildConfig(s, buildBoot{Kernel: "/a/vmlinux", Initrd: "/w/build-initrd"}, 2,
		1024)
	golden(t, "build-direct.json", configJSON(t, direct))
}

func TestKernelCommandLines(t *testing.T) {
	got := relayArgs(Slot(64), "tok")
	want := "console=ttyS0 reboot=k panic=1 pci=off warden.net=static " +
		"warden.nat_ip=100.64.89.2 " +
		"warden.nat_prefix=30 warden.nat_gw=100.64.89.1 " +
		"warden.claim_token=tok"
	if got != want {
		t.Fatalf("relay args:\n got %s\nwant %s", got, want)
	}
	if strings.Contains(got, "warden.dns") {
		t.Fatal("the relay's resolvers are on its command line, not in its claim")
	}
	disk := buildConfig(0, buildBoot{Kernel: "k", Rootfs: "r", PartUUID: "abc", Seed: "s"}, 1,
		1)
	if got := disk.effectiveArgs(); got != baseArgs+" root=PARTUUID=abc rw" {
		t.Fatalf("disk args = %q", got)
	}
	direct := buildConfig(0, buildBoot{Kernel: "k", Initrd: "i"}, 1, 1)
	if got := direct.effectiveArgs(); got != baseArgs {
		t.Fatalf("direct args = %q", got)
	}
	if strings.Contains(direct.effectiveArgs(), "token") {
		t.Fatal("the build VM's command line carries a secret")
	}
}

func TestConsoleTailAndPrefix(t *testing.T) {
	var b strings.Builder
	p := &prefixWriter{prefix: "[relay] ", w: &b}
	_, _ = p.Write([]byte("one\ntw"))
	_, _ = p.Write([]byte("o\nthree\n"))
	if b.String() != "[relay] one\n[relay] two\n[relay] three\n" {
		t.Fatalf("prefixed = %q", b.String())
	}
}

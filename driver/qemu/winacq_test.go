package qemu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsURL(t *testing.T) {
	for _, u := range []string{"http://x/y.iso", "https://x/y.iso"} {
		if !isURL(u) {
			t.Errorf("isURL(%q) = false, want true", u)
		}
	}
	for _, p := range []string{"/path/to.iso", "./rel.iso", "C:\\win.iso"} {
		if isURL(p) {
			t.Errorf("isURL(%q) = true, want false", p)
		}
	}
}

func TestVirtioWinSource(t *testing.T) {
	if virtioWinSource("") != virtioWinStableURL {
		t.Error("empty virtio ref should map to the stable channel URL")
	}
	if got := virtioWinSource("/local/virtio.iso"); got != "/local/virtio.iso" {
		t.Errorf("virtioWinSource passthrough wrong: %q", got)
	}
}

func TestCachedISOPath(t *testing.T) {
	p := cachedISOPath("https://example.com/win/Win11_ARM64.iso")
	if filepath.Ext(p) != ".iso" {
		t.Errorf("cachedISOPath should end in .iso: %q", p)
	}
	if strings.ContainsAny(filepath.Base(p), "/:?&=") {
		t.Errorf("cachedISOPath base should be sanitized: %q", filepath.Base(p))
	}
	// Long URLs collapse to a bounded, hashed name.
	long := "https://example.com/" + strings.Repeat("a", 200) + ".iso"
	if b := filepath.Base(cachedISOPath(long)); len(b) > 120 {
		t.Errorf("cachedISOPath base too long (%d): %q", len(b), b)
	}
}

func TestResolveWindowsISO(t *testing.T) {
	if _, err := resolveWindowsISO(""); err == nil {
		t.Error("resolveWindowsISO(\"\") should error")
	}
	if _, err := resolveWindowsISO("/no/such/file.iso"); err == nil {
		t.Error("resolveWindowsISO on a missing path should error")
	}
	f := filepath.Join(t.TempDir(), "win.iso")
	if err := os.WriteFile(f, []byte("iso"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveWindowsISO(f)
	if err != nil {
		t.Fatalf("resolveWindowsISO(existing) errored: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolveWindowsISO should return an absolute path: %q", got)
	}
}

func TestWindowsInstallArgs(t *testing.T) {
	m := &WindowsMedia{
		InstallISO:      "/cache/win.iso",
		VirtioISO:       "/cache/virtio-win.iso",
		AutounattendISO: "/cache/autounattend.iso",
	}
	// With writable UEFI vars (pflash path).
	args := windowsInstallArgs("aarch64", "hvf",
		"/cache/windows-arm64.qcow2", "/fw/code.fd", "/fw/vars.fd", m)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"virt,accel=hvf",
		"if=pflash,format=raw,readonly=on,file=/fw/code.fd",
		"if=pflash,format=raw,file=/fw/vars.fd",
		"file=/cache/windows-arm64.qcow2,format=qcow2,if=virtio",
		"/cache/win.iso",
		"/cache/virtio-win.iso",
		"/cache/autounattend.iso",
		"usb-storage,bus=xhci.0,drive=cd0",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("windowsInstallArgs missing %q\nargs: %s", want, joined)
		}
	}

	// Without vars template, falls back to -bios (no pflash).
	noVars := strings.Join(
		windowsInstallArgs("aarch64", "hvf", "/b.qcow2", "/fw/code.fd", "", m), " ")
	if !strings.Contains(noVars, "-bios /fw/code.fd") {
		t.Error("windowsInstallArgs should fall back to -bios without vars")
	}
	if strings.Contains(noVars, "if=pflash") {
		t.Error("windowsInstallArgs should not use pflash without a vars file")
	}
}

func TestQemuArchOf(t *testing.T) {
	if qemuArchOf("arm64") != "aarch64" || qemuArchOf("amd64") != "x86_64" {
		t.Error("qemuArchOf mapping wrong")
	}
}

func TestAcquireWindowsMediaLocal(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "win.iso")
	virtio := filepath.Join(dir, "virtio-win.iso")
	for _, f := range []string{iso, virtio} {
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	media, err := AcquireWindowsMedia(WindowsPrepOptions{
		ISO:       iso,
		VirtioISO: virtio,
		Arch:      "arm64",
		WorkDir:   filepath.Join(dir, "work"),
	})
	if err != nil {
		t.Fatalf("AcquireWindowsMedia: %v", err)
	}
	if media.InstallISO != iso {
		t.Errorf("InstallISO = %q, want %q", media.InstallISO, iso)
	}
	if media.VirtioISO != virtio {
		t.Errorf("VirtioISO = %q, want %q", media.VirtioISO, virtio)
	}
	if _, err := os.Stat(media.AutounattendISO); err != nil {
		t.Errorf("Autounattend ISO not created: %v", err)
	}
	// The materialized answer file should carry the bypass keys.
	xml, err := os.ReadFile(filepath.Join(dir, "work", "answer", "Autounattend.xml"))
	if err != nil {
		t.Fatalf("reading Autounattend.xml: %v", err)
	}
	if !strings.Contains(string(xml), "BypassTPMCheck") {
		t.Error("materialized Autounattend.xml missing bypass keys")
	}
}

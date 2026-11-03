package linuxguest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSeedGolden pins the seed's text files, with qemu's poweroff tail, to the
// bytes the qemu driver wrote before this code moved here (TEST-04).
func TestSeedGolden(t *testing.T) {
	dir := t.TempDir()
	agent := filepath.Join(dir, "agent")
	if err := os.WriteFile(agent, []byte("io"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed, err := writeSeed(filepath.Join(dir, "cidata"), agent, PowerOff)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"meta-data", "network-config", "user-data"} {
		got, _ := os.ReadFile(filepath.Join(seed, name))
		want, err := os.ReadFile(filepath.Join("testdata", "seed", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the golden fixture:\n%s", name, got)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(seed, "warden-io")); string(b) != "io" {
		t.Errorf("warden-io = %q", b)
	}
}

func TestUserDataTail(t *testing.T) {
	for _, last := range []string{PowerOff, Reboot} {
		ud := userData(last)
		if !strings.HasSuffix(ud, "  - "+last+"\n") {
			t.Errorf("user-data does not end with %q:\n%s", last, ud)
		}
	}
}

func TestCloudImageURL(t *testing.T) {
	for ref, want := range map[string]string{
		"ubuntu:24.04":    "noble-server-cloudimg-",
		"ubuntu":          "noble-server-cloudimg-",
		"debian:bookworm": "debian-12-generic-",
		"alpine:3.21":     "generic_alpine-3.21.0-",
		"fedora:40":       "",
	} {
		got := cloudImageURL(ref)
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("cloudImageURL(%q) = %q, want it to contain %q", ref, got, want)
		}
	}
}

func TestBuiltinISOHasVolumeAndFiles(t *testing.T) {
	iso := buildISO9660([]fileEntry{{name: "user-data", data: []byte("hello")}}, "CIDATA")
	if len(iso)%2048 != 0 {
		t.Fatalf("ISO size %d is not a whole number of sectors", len(iso))
	}
	pvd := iso[16*2048:]
	if pvd[0] != 1 || string(pvd[1:6]) != "CD001" {
		t.Fatal("no primary volume descriptor at sector 16")
	}
	if !bytes.HasPrefix(pvd[40:], []byte("CIDATA")) {
		t.Fatalf("volume id = %q", pvd[40:72])
	}
	if !bytes.Contains(iso, []byte("hello")) {
		t.Fatal("file data missing")
	}
}

package firecracker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

func TestDescribeBuild(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	kernel := write("vmlinux", "k")
	raw := write("ubuntu_24_04-0123456789ab.raw", "disk")
	initrd := write("initramfs.cpio.gz", "i")
	agent := write("warden-io-linux-amd64", "io")
	write("cidata/meta-data", "m")
	write("cidata/network-config", "n")
	write("cidata/user-data", "u")
	write("cidata/warden-io", "io")
	script := write("build.sh", "echo")
	cmdline := write("kernel-cmdline", baseArgs+"\n")

	for _, tt := range []struct {
		name      string
		rec       buildRecord
		wantRoles []string
		wantSeed  []string
		wantKind  string // the second layer's source
	}{
		{
			name: "disk path",
			rec: buildRecord{Kernel: kernel, Raw: raw, SeedDir: filepath.Join(dir,
				"cidata"),
				Script: script, Cmdline: cmdline},
			wantRoles: []string{ledger.LayerRoleKernel, ledger.LayerRoleDisk},
			wantSeed: []string{driver.SeedBuildScript, seedKernelCmdline, "meta-data",
				"network-config", "user-data", "warden-io"},
			wantKind: ledger.SourcePrepared,
		},
		{
			name: "direct boot",
			rec: buildRecord{Kernel: kernel, Initrd: initrd, Agent: agent,
				Script: script, Cmdline: cmdline},
			wantRoles: []string{ledger.LayerRoleKernel, ledger.LayerRoleInitrd},
			wantSeed:  []string{driver.SeedBuildScript, seedKernelCmdline, seedAgent},
			wantKind:  ledger.SourceLocal,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cache := &driver.LayerCache{Path: filepath.Join(t.TempDir(),
				"layers.json")}
			sum, _ := driver.HashFileSHA256(raw)
			if err := cache.Record(raw, sum, "", rawSource("cafe")); err != nil {
				t.Fatal(err)
			}
			env, err := describeBuild(cache, tt.rec)
			if err != nil {
				t.Fatal(err)
			}
			d, err := ledger.DecodeVMDiskChain(env.Payload)
			if err != nil {
				t.Fatal(err)
			}
			checkDescriptor(t, d, tt.wantRoles, tt.wantSeed, tt.wantKind)
		})
	}
}

func checkDescriptor(t *testing.T, d *ledger.VMDiskChain, wantRoles, wantSeed []string,
	wantKind string) {
	t.Helper()
	if d.Driver != "firecracker" || d.GuestOS != "linux" || d.Arch != "amd64" {
		t.Fatalf("descriptor = %+v", d)
	}
	if len(d.Layers) != len(wantRoles) {
		t.Fatalf("layers = %+v", d.Layers)
	}
	for i, role := range wantRoles {
		if d.Layers[i].Role != role {
			t.Errorf("layer %d role = %s, want %s", i,
				d.Layers[i].Role, role)
		}
	}
	if d.Layers[1].Source.Kind != wantKind {
		t.Errorf("layer 1 source = %+v, want %s", d.Layers[1].Source,
			wantKind)
	}
	if wantKind == ledger.SourcePrepared &&
		(d.Layers[1].Format != "raw" ||
			d.Layers[1].Source.Inputs[0].SHA256 != "cafe") {
		t.Errorf("raw layer = %+v", d.Layers[1])
	}
	if len(d.Seed) != len(wantSeed) {
		t.Fatalf("seed = %+v, want %v", d.Seed, wantSeed)
	}
	for i, name := range wantSeed {
		if d.Seed[i].Name != name {
			t.Errorf("seed %d = %s, want %s", i, d.Seed[i].Name, name)
		}
	}
}

package qemu

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
	base := write("windows-arm64.qcow2", "disk")
	vars := write("windows-arm64-vars.fd", "vars")
	kernel := write("vmlinuz", "k")
	initrd := write("initramfs", "i")
	write("seed/warden-io.exe", "io")
	write("seed/warden-run.ps1", "run")
	script := write("build.ps1", "echo")

	tests := []struct {
		name      string
		cfg       buildVMConfig
		wantOS    string
		wantRoles []string
		wantSeed  int
	}{
		{
			name: "windows disk",
			cfg: buildVMConfig{Arch: "aarch64", Windows: true, BaseImage: base,
				BaseVarsFD: vars, SeedDir: filepath.Join(dir, "seed")},
			wantOS:    "windows",
			wantRoles: []string{ledger.LayerRoleDisk, ledger.LayerRoleNVRAM},
			wantSeed:  3,
		},
		{
			name:      "direct kernel boot",
			cfg:       buildVMConfig{Arch: "x86_64", Kernel: kernel, Initrd: initrd},
			wantOS:    "linux",
			wantRoles: []string{ledger.LayerRoleKernel, ledger.LayerRoleInitrd},
			wantSeed:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &driver.LayerCache{
				Path: filepath.Join(t.TempDir(), "layers.json")}
			env, err := describeBuild(cache, &tt.cfg, script)
			if err != nil {
				t.Fatal(err)
			}
			d, err := ledger.DecodeVMDiskChain(env.Payload)
			if err != nil {
				t.Fatal(err)
			}
			wantArch := goArchOf(tt.cfg.Arch)
			if d.Driver != "qemu" || d.GuestOS != tt.wantOS || d.Arch != wantArch {
				t.Fatalf("descriptor = %+v", d)
			}
			if len(d.Layers) != len(tt.wantRoles) {
				t.Fatalf("layers = %+v, want roles %v", d.Layers, tt.wantRoles)
			}
			for i, role := range tt.wantRoles {
				l := d.Layers[i]
				if l.Role != role || l.Source.Kind != ledger.SourceLocal {
					t.Errorf("layer %d = %+v, want local %s", i, l, role)
				}
			}
			if len(d.Seed) != tt.wantSeed {
				t.Errorf("seed = %+v, want %d entries", d.Seed, tt.wantSeed)
			}
		})
	}
}

func TestDescribeBuildUsesRecordedProvenance(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "ubuntu_24_04.qcow2")
	if err := os.WriteFile(base, []byte("cloud"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, _ := driver.HashFileSHA256(base)
	cache := &driver.LayerCache{Path: filepath.Join(dir, "layers.json")}
	url := "https://cloud-images.ubuntu.com/noble/current/x.img"
	if err := cache.Record(base, sum, "",
		ledger.LayerSource{Kind: ledger.SourceURL, URL: url}); err != nil {
		t.Fatal(err)
	}
	env, err := describeBuild(cache, &buildVMConfig{Arch: "x86_64", BaseImage: base}, "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ledger.DecodeVMDiskChain(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if src := d.Layers[0].Source; src.Kind != ledger.SourceURL || src.URL != url {
		t.Fatalf("source = %+v, want the recorded download URL", src)
	}
}

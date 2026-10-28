package vz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/ledger"
)

const testIPSWURL = "https://updates.cdn-apple.com/x/UniversalMac_Restore.ipsw"

// preparedImageFixture lays out a prepared image dir and its IPSW, records the
// image as prepared (twice, as a re-prepare does), and returns the cache, the
// disk path and a build script.
func preparedImageFixture(t *testing.T) (c *ImageCache, disk, script string) {
	t.Helper()
	cacheDir := t.TempDir()
	imgDir := filepath.Join(cacheDir, "macos-abc")
	files := map[string]string{
		filepath.Join(imgDir, "disk.img"):        "apfs",
		filepath.Join(imgDir, "aux-storage"):     "nvram",
		filepath.Join(imgDir, "hardware-model"):  "hw",
		filepath.Join(imgDir, "machine-id"):      "id",
		filepath.Join(cacheDir, "ipsw-abc.ipsw"): "ipsw",
	}
	script = filepath.Join(t.TempDir(), "build.sh")
	files[script] = "make"
	for p, content := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c = &ImageCache{CacheDir: cacheDir}
	ipsw := filepath.Join(cacheDir, "ipsw-abc.ipsw")
	if err := c.recordPreparedImage(imgDir, ipswInput(testIPSWURL, ipsw)); err != nil {
		t.Fatal(err)
	}
	// A re-prepare without inputs keeps the recorded IPSW input.
	if err := c.recordPreparedImage(imgDir, nil); err != nil {
		t.Fatal(err)
	}
	return c, filepath.Join(imgDir, "disk.img"), script
}

func TestDescribeBuildAndPreparedProvenance(t *testing.T) {
	c, disk, script := preparedImageFixture(t)
	env, err := describeBuild(c.LayerCache(), disk, script)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ledger.DecodeVMDiskChain(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.Driver != "vz" || d.GuestOS != "macos" || d.Arch != "arm64" {
		t.Fatalf("descriptor = %+v", d)
	}
	wantRoles := []string{ledger.LayerRoleDisk, ledger.LayerRoleNVRAM,
		ledger.LayerRolePlatform, ledger.LayerRolePlatform}
	if len(d.Layers) != len(wantRoles) {
		t.Fatalf("layers = %+v", d.Layers)
	}
	for i, l := range d.Layers {
		if l.Role != wantRoles[i] || l.Source.Kind != ledger.SourcePrepared {
			t.Errorf("layer %d = %+v, want prepared %s", i, l, wantRoles[i])
		}
	}
	in := d.Layers[0].Source.Inputs
	if len(in) != 1 || in[0].URL != testIPSWURL || in[0].SHA256 == "" {
		t.Fatalf("disk inputs = %+v, want the hashed IPSW", in)
	}
	if len(d.Seed) != 1 || d.Seed[0].Name != "build-script" {
		t.Fatalf("seed = %+v", d.Seed)
	}
}

package relaysource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// bundledPlaces makes the running binary's directory and the source tree two
// directories of the test's, each holding the files named.
func bundledPlaces(t *testing.T, beside, tree []string) (exeDir, root string) {
	t.Helper()
	exeDir, root = t.TempDir(), t.TempDir()
	for _, f := range beside {
		write(t, filepath.Join(exeDir, f), f)
	}
	for _, f := range tree {
		write(t, filepath.Join(root, filepath.FromSlash(f)), f)
	}
	oldExe, oldRoot := executable, moduleRoot
	executable = func() (string, error) {
		return filepath.Join(exeDir, "warden-driver-x"), nil
	}
	moduleRoot = func() (string, error) { return root, nil }
	t.Cleanup(func() { executable, moduleRoot = oldExe, oldRoot })
	return exeDir, root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

var applianceFiles = []File{
	{Role: RelayKernel, Release: "relay-vm-vmlinuz", Source: "out/vmlinuz"},
	{Role: RelayInitramfs, Release: "relay-vm-initramfs.cpio.gz", Source: "out/initramfs"},
}

// Bundled takes every file from one place, beside the binary first, and
// never mixes the two: a place that lacks one file is passed over whole
// (GLOBAL-11).
func TestBundledOnePlace(t *testing.T) {
	both := []string{"relay-vm-vmlinuz", "relay-vm-initramfs.cpio.gz"}
	tree := []string{"out/vmlinuz", "out/initramfs"}
	for _, tc := range []struct {
		name         string
		beside, tree []string
		want         string // the origin, or "" for not found
	}{
		{"beside the binary", both, tree, "bundled"},
		{"the source tree", nil, tree, "source tree"},
		{"one file beside, both in the tree", both[:1], tree, "source tree"},
		{"one in each place", both[:1], tree[1:], ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exeDir, root := bundledPlaces(t, tc.beside, tc.tree)
			src := Bundled(RelayAppliance, applianceFiles...)
			r, err := src.Relay(context.Background())
			if tc.want == "" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("Relay = %+v, %v; want ErrNotFound", r, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := exeDir
			if tc.want == "source tree" {
				dir = root
			}
			if r.Origin != tc.want || r.Form != RelayAppliance {
				t.Errorf("relay %+v, want from %s", r, tc.want)
			}
			for role, f := range r.Files {
				in := filepath.Dir(f.Path)
				if in != dir && filepath.Dir(in) != dir {
					t.Errorf("%s from %s, not from %s", role, f.Path, dir)
				}
			}
		})
	}
}

// A file the user named is used as given, and never looked for elsewhere: a
// path that does not exist is the error, with the bundled files present
// (GLOBAL-10).
func TestPathIsUsedExclusively(t *testing.T) {
	bundledPlaces(t, []string{"relay-boot.vhdx"}, nil)
	named := filepath.Join(t.TempDir(), "mine.vhdx")
	write(t, named, "disk")
	r, err := Path(RelayAppliance, RelayDisk, named, "WARDEN_RELAY_BOOT_VHDX").Relay(
		context.Background())
	if err != nil || r.Files[RelayDisk].Path != named || r.Origin != "WARDEN_RELAY_BOOT_VHDX" {
		t.Fatalf("Relay = %+v, %v", r, err)
	}
	_, err = Path(RelayAppliance, RelayDisk, named+".gone", "WARDEN_RELAY_BOOT_VHDX").Relay(
		context.Background())
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Errorf("a named file that does not exist: %v, want a FixableError", err)
	}
}

// An image the user named is the relay's image, as given.
func TestImage(t *testing.T) {
	r, err := Image("registry.example/relay:v1", "WARDEN_IMAGE").Relay(context.Background())
	if err != nil || r.Form != RelayOCIImage || r.Image != "registry.example/relay:v1" ||
		r.Origin != "WARDEN_IMAGE" || len(r.Files) != 0 {
		t.Errorf("Relay = %+v, %v", r, err)
	}
}

// Verify passes a file whose hash matches the published one, refuses one
// that does not with the file and its origin named, and does not hash a file
// that has no published hash.
func TestVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay")
	write(t, path, "the relay")
	sum := sha256.Sum256([]byte("the relay"))
	good := hex.EncodeToString(sum[:])
	r := &Relay{Form: RelayHostBinary, Origin: "pinned",
		Files: map[RelayRole]RelayFile{RelayBinary: {Path: path, SHA256: good},
			RelayKernel: {Path: filepath.Join(t.TempDir(), "absent")}}}
	if err := r.Verify(); err != nil {
		t.Errorf("a matching hash: %v", err)
	}
	r.Files[RelayBinary] = RelayFile{Path: path, SHA256: "00" + good[2:]}
	err := r.Verify()
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !containsAll(fe.Msg, path, "pinned", good) {
		t.Errorf("a mismatching hash: %v, want a FixableError naming the file, "+
			"its origin and its hash", err)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

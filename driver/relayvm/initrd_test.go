package relayvm

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendFileCPIO(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base")
	_ = os.WriteFile(base, []byte("BASE"), 0o644)
	src := filepath.Join(dir, "relay")
	_ = os.WriteFile(src, []byte("ELF-relay-bytes"), 0o755)
	out := filepath.Join(dir, "out")
	if err := AppendFileCPIO(base, out, src, "sbin/relay"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if !bytes.HasPrefix(b, []byte("BASE")) {
		t.Fatal("base archive not preserved first")
	}
	zr, err := gzip.NewReader(bytes.NewReader(b[4:]))
	if err != nil {
		t.Fatal(err)
	}
	cpio, _ := io.ReadAll(zr)
	if !bytes.HasPrefix(cpio, []byte("070701")) {
		t.Fatalf("not a newc archive: %q", cpio[:12])
	}
	// Header is 110 bytes; namesize 11 ("sbin/relay\0") pads to 124.
	if got := string(cpio[110:120]); got != "sbin/relay" {
		t.Fatalf("entry name = %q", got)
	}
	if got := string(cpio[124 : 124+15]); got != "ELF-relay-bytes" {
		t.Fatalf("entry data = %q", got)
	}
	if !bytes.Contains(cpio, []byte("TRAILER!!!")) {
		t.Fatal("missing trailer")
	}
	if len(cpio)%4 != 0 {
		t.Fatal("archive not 4-byte aligned")
	}
}

// TestAppendFileCPIOGolden pins the composed initrd to the bytes the qemu
// driver produced before this code moved here (TEST-04), so the move changes
// nothing a qemu relay boots.
func TestAppendFileCPIOGolden(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base")
	_ = os.WriteFile(base, []byte("BASE-INITRAMFS"), 0o644)
	src := filepath.Join(dir, "relay")
	_ = os.WriteFile(src, []byte("ELF-relay-bytes\x00\x01\x02"), 0o755)
	out := filepath.Join(dir, "out")
	if err := AppendFileCPIO(base, out, src, "sbin/relay"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	want, err := os.ReadFile(filepath.Join("testdata", "relay-initrd.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("initrd differs from the golden fixture:\n got %x\nwant %x", got, want)
	}
}

func TestFindAllFromOnePlace(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	_ = os.WriteFile(a, nil, 0o644)
	at := func(x Asset) string { return filepath.Join(dir, x.Release) }
	if got, ok := findIn([]Asset{{Release: "a"}}, at); !ok || got[0] != a {
		t.Fatalf("findIn = %v, %v", got, ok)
	}
	if _, ok := findIn([]Asset{{Release: "a"}, {Release: "missing"}}, at); ok {
		t.Fatal("findIn accepted a set with a missing asset")
	}
}

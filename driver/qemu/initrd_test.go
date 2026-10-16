package qemu

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
	if err := appendFileCPIO(base, out, src, "sbin/relay"); err != nil {
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

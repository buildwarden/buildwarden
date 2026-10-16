package qemu

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/buildwarden/buildwarden/driver"
)

// relayInitrd returns the relay VM kernel and an initrd that is the prebuilt
// base initramfs (tools/relay-vm) with the current relay binary appended at
// /sbin/relay as a second cpio archive. The kernel unpacks concatenated
// archives in order, so the base image never has to be rebuilt when the relay
// changes, and the VM needs no shared disk to find its binary.
func (d *Driver) relayInitrd(workDir string) (kernel, initrd string, err error) {
	kernel, base, err := d.resolveRelayAssets()
	if err != nil {
		return "", "", err
	}
	relayBin, err := driver.GuestBinary("relay", "linux", runtime.GOARCH)
	if err != nil {
		return "", "", err
	}
	initrd = filepath.Join(workDir, "relay-initrd")
	if err := appendFileCPIO(base, initrd, relayBin, "sbin/relay"); err != nil {
		return "", "", fmt.Errorf("composing relay initrd: %w", err)
	}
	return kernel, initrd, nil
}

// appendFileCPIO writes base followed by a gzip'd newc cpio archive holding
// src at path (mode 0755).
func appendFileCPIO(base, out, src, path string) error {
	dst, err := os.Create(out)
	if err != nil {
		return err
	}
	defer dst.Close()
	in, err := os.Open(base)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, in)
	in.Close()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(dst)
	if err := writeNewc(zw, path, 0o100755, data); err != nil {
		return err
	}
	if err := writeNewc(zw, "TRAILER!!!", 0, nil); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return dst.Close()
}

// writeNewc writes one SVR4 "newc" cpio entry.
func writeNewc(w io.Writer, name string, mode uint32, data []byte) error {
	nlink := 1
	if name == "TRAILER!!!" {
		nlink = 0
	}
	hdr := fmt.Sprintf("070701%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
		0, mode, 0, 0, nlink, 0, len(data), 0, 0, 0, 0, len(name)+1, 0)
	buf := append([]byte(hdr), name...)
	buf = append(buf, 0)
	buf = pad4(buf)
	if _, err := w.Write(buf); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := w.Write(make([]byte, (4-len(data)%4)%4))
	return err
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

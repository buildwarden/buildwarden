//go:build unix

package vz

import (
	"runtime"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// The relay's socket is a duplicate: closing it, and collecting it, leaves
// the pair's own descriptors open for Close, which then closes each once.
func TestRelaySocketIsADuplicate(t *testing.T) {
	vn, err := NewVirtualNetwork(buildlink.Default())
	if err != nil {
		t.Fatal(err)
	}
	f, err := vn.RelaySocket()
	if err != nil {
		t.Fatal(err)
	}
	if int(f.Fd()) == vn.RelaySocketFD || int(f.Fd()) == vn.BuildSocketFD {
		t.Fatalf("RelaySocket returned the pair's own fd %d", f.Fd())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	f = nil
	runtime.GC()
	for _, fd := range []int{vn.RelaySocketFD, vn.BuildSocketFD} {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != nil {
			t.Fatalf("fd %d closed by the relay socket: %v", fd, err)
		}
	}
	if err := vn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}


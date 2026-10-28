//go:build windows

package hyperv

import (
	"os"

	"golang.org/x/sys/windows"
)

// finalPath returns the real location of an existing path, as the file system
// resolves it (GetFinalPathNameByHandle), and whether it could be read.
func finalPath(p string) (string, bool) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", false
	}
	h, err := windows.CreateFile(name, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || int(n) > len(buf) {
		return "", false
	}
	return windows.UTF16ToString(buf[:n]), true
}

// probeNewFile creates dir if needed and a short-lived file in it, and returns
// where that file really landed. A directory can resolve normally while new
// files in it are redirected, so the probe is a file, not the directory.
func probeNewFile(dir string) (string, bool) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	f, err := os.CreateTemp(dir, ".warden-probe-*")
	if err != nil {
		return "", false
	}
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name) //nolint:errcheck
	return finalPath(name)
}

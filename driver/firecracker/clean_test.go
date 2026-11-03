package firecracker

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func TestConvertLockOf(t *testing.T) {
	for name, want := range map[string]string{
		"noble-server-0123456789ab.raw.tmp": "firecracker-convert-0123456789ab",
		"noble-0123456789ab.raw":            "",
		"noble-0123456789AB.raw.tmp":        "",
		"0123456789ab.raw.tmp":              "",
		"x-123456789ab.raw.tmp":             "",
	} {
		got, ok := convertLockOf(name)
		if got != want || ok != (want != "") {
			t.Errorf("convertLockOf(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

// TestReapConversions checks that clean removes a killed conversion's partial
// image and leaves a running conversion's, and every finished image, alone.
func TestReapConversions(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // Linux
	t.Setenv("HOME", t.TempDir())           // macOS UserCacheDir
	t.Setenv("LocalAppData", t.TempDir())   // Windows
	if err := os.MkdirAll(ImageDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(ImageDir(), "noble-0123456789ab.raw.tmp")
	running := filepath.Join(ImageDir(), "jammy-aaaaaaaaaaaa.raw.tmp")
	done := filepath.Join(ImageDir(), "noble-0123456789ab.raw")
	for _, p := range []string{stale, running, done} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	unlock, err := driver.HostLock(convertLockName("aaaaaaaaaaaa"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	var out bytes.Buffer
	if n := reapConversions(&out); n != 1 {
		t.Fatalf("removed %d, want 1:\n%s", n, out.String())
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("the killed conversion's partial image is still there")
	}
	for _, p := range []string{running, done} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed", filepath.Base(p))
		}
	}
}

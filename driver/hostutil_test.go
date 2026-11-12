package driver

import (
	"path/filepath"
	"testing"
)

func TestGuestCachePath(t *testing.T) {
	cache := filepath.Join("c", "warden")
	a := filepath.Join(t.TempDir(), "checkout-a")
	b := filepath.Join(t.TempDir(), "checkout-b")
	const name = "warden-io-windows-amd64.exe"

	pa := guestCachePath(cache, a, name)
	if got := guestCachePath(cache, a+string(filepath.Separator), name); got != pa {
		t.Errorf("same tree, trailing separator: %q, want %q", got, pa)
	}
	if pb := guestCachePath(cache, b, name); pb == pa {
		t.Errorf("two trees share %q", pa)
	}
	if filepath.Base(pa) != name {
		t.Errorf("base %q, want %q", filepath.Base(pa), name)
	}
	if dir := filepath.Dir(filepath.Dir(pa)); dir != filepath.Join(cache, "bin") {
		t.Errorf("cache dir %q, want under %q", dir, filepath.Join(cache, "bin"))
	}
}

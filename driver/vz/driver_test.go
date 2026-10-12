package vz

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// A build with no prepared image must fail at once, naming the prepare
// command, rather than starting a multi-GB restore it cannot finish.
func TestResolveImageWithoutPreparedImageIsFixable(t *testing.T) {
	d := &Driver{Cache: &ImageCache{CacheDir: t.TempDir()}}
	_, err := d.resolveImage("")
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("resolveImage = %v, want a FixableError", err)
	}
	if !strings.Contains(fe.Fix, PrepareCommand) {
		t.Errorf("fix %q does not name %q", fe.Fix, PrepareCommand)
	}
}

func TestResolveImageMissingPathIsFixable(t *testing.T) {
	d := &Driver{Cache: &ImageCache{CacheDir: t.TempDir()}}
	_, err := d.resolveImage("/no/such/disk.img")
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("resolveImage = %v, want a FixableError", err)
	}
}

func TestRequireRoot(t *testing.T) {
	err := RequireRoot()
	if os.Geteuid() == 0 {
		if err != nil {
			t.Fatalf("RequireRoot as root = %v", err)
		}
		return
	}
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "sudo") {
		t.Fatalf("RequireRoot = %v, want a FixableError telling the user to use sudo", err)
	}
}

func TestDefaultCacheDirIgnoresSudoUserWhenNotRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	t.Setenv("SUDO_USER", "someone-else")
	home, _ := os.UserHomeDir()
	if got := DefaultCacheDir(); !strings.HasPrefix(got, home) {
		t.Errorf("DefaultCacheDir() = %s, want it under %s", got, home)
	}
}

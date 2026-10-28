package hyperv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRedirected(t *testing.T) {
	const pkg = `C:\Users\u\AppData\Local\Packages\Claude_x\LocalCache\Local\warden\hyperv\f`
	for _, tc := range []struct {
		want, final string
		red         bool
	}{
		{`C:\Users\u\AppData\Local\warden\hyperv\f`, `\\?\` + pkg, true},
		{`C:\Users\u\AppData\Local\warden\hyperv\f`,
			`\\?\C:\Users\u\AppData\Local\warden\hyperv\f`, false},
		{`c:\users\u\appdata\local\warden\hyperv\f`,
			`\\?\C:\Users\u\AppData\Local\warden\hyperv\f`, false}, // case only
		{`C:\cache\f`, `\\?\D:\real\f`, false}, // a user's own link elsewhere
		{pkg, `\\?\` + pkg, false},             // asked for the package path itself
	} {
		if got := redirected(tc.want, tc.final); got != tc.red {
			t.Errorf("redirected(%q, %q) = %v, want %v",
				tc.want, tc.final, got, tc.red)
		}
	}
}

// A normal directory (the test's temp dir is never redirected) passes both
// checks, and the probe leaves nothing behind.
func TestCheckVisibleOnPlainDir(t *testing.T) {
	dir := t.TempDir()
	if err := checkDirVisible(dir, "cache", cacheVisibleFix); err != nil {
		t.Fatalf("checkDirVisible: %v", err)
	}
	f := filepath.Join(dir, "disk.vhdx")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkFilesVisible(cacheVisibleFix, f); err != nil {
		t.Fatalf("checkFilesVisible: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("probe left files behind: %v", entries)
	}
}

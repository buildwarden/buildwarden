package firecracker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/ledger"
)

func TestCheckImageInfo(t *testing.T) {
	for name, tt := range map[string]struct {
		json, format string
	}{
		"qcow2": {`{"format":"qcow2","virtual-size":3758096384}`, "qcow2"},
		"raw":   {`{"format":"raw"}`, "raw"},
		"backing file": {`{"format":"qcow2","backing-filename":"/home/u/.ssh/id_rsa"}`,
			""},
		"data file": {`{"format":"qcow2",` +
			`"format-specific":{"type":"qcow2",` +
			`"data":{"data-file":"/etc/shadow"}}}`, ""},
		"vmdk":        {`{"format":"vmdk"}`, ""},
		"unparseable": {`not json`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := checkImageInfo("img", []byte(tt.json))
			if tt.format == "" {
				if err == nil {
					t.Fatalf("accepted as %q", got)
				}
				var fe *driver.FixableError
				if name != "unparseable" && !errors.As(err, &fe) {
					t.Fatalf("refusal is not fixable: %v", err)
				}
				return
			}
			if err != nil || got != tt.format {
				t.Fatalf("checkImageInfo = %q, %v", got, err)
			}
		})
	}
}

func TestRawCacheKeyAndProvenance(t *testing.T) {
	sum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if got := rawName("/c/images/ubuntu_24_04.qcow2",
		sum); got != "ubuntu_24_04-0123456789ab.raw" {
		t.Fatalf("rawName = %q", got)
	}
	if got := rawName("/x/my disk.v2.img", sum); got != "my_disk_v2-0123456789ab.raw" {
		t.Fatalf("rawName = %q", got)
	}
	src := rawSource(sum)
	if src.Kind != ledger.SourcePrepared || src.PreparedBy != driver.PreparedBy() ||
		len(src.Inputs) != 1 || src.Inputs[0].SHA256 != sum {
		t.Fatalf("rawSource = %+v", src)
	}
}

// TestConverted checks that a cached raw image is reused only while it is
// recorded as converted from the same source hash.
func TestConverted(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LocalAppData", os.Getenv("XDG_CACHE_HOME"))
	t.Setenv("HOME", os.Getenv("XDG_CACHE_HOME"))
	if err := os.MkdirAll(ImageDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(ImageDir(), "x-0123456789ab.raw")
	if converted(raw, "aa") {
		t.Fatal("a missing raw image counted as converted")
	}
	if err := os.WriteFile(raw, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if converted(raw, "aa") {
		t.Fatal("an unrecorded raw image counted as converted")
	}
	sum, _ := driver.HashFileSHA256(raw)
	if err := LayerCache().Record(raw, sum, "", rawSource("aa")); err != nil {
		t.Fatal(err)
	}
	if !converted(raw, "aa") || converted(raw, "bb") {
		t.Fatal("converted does not follow the recorded source hash")
	}
}

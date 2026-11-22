package container

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/relaysource"
)

// The relay's source: the image relay-image names, with its origin; a build
// from the source tree for dev, and by default in a dev build; else the
// release's tag (driver/relaysource).
func TestRelaySource(t *testing.T) {
	for _, tc := range []struct {
		name, version, image string
		want                 string // the source's type
	}{
		{"an image named", "v1.2.0", "registry.example/relay:v1", "image"},
		{"dev named", "v1.2.0", "dev", "source"},
		{"a dev build", "dev", "", "source"},
		{"a release", "v1.2.0", "", "release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := Version
			Version = tc.version
			t.Cleanup(func() { Version = old })
			b := &build{d: &runner{relayOrigin: "WARDEN_IMAGE"},
				req: &driver.BuildRequest{RelayImage: tc.image}}
			src := b.relaySource()
			got := "image"
			switch src.(type) {
			case sourceRelay:
				got = "source"
			case releaseRelay:
				got = "release"
			}
			if got != tc.want {
				t.Fatalf("source %T, want %s", src, tc.want)
			}
			if got == "image" {
				r, err := src.Relay(context.Background())
				oci := relaysource.RelayOCIImage
				if err != nil || r.Image != tc.image ||
					r.Origin != "WARDEN_IMAGE" || r.Form != oci {
					t.Errorf("relay %+v, %v", r, err)
				}
			}
		})
	}
}

// An image that cannot be pulled fails the build; the release's image is
// not tried in its place (GLOBAL-10).
func TestRelayImageOverrideIsExclusive(t *testing.T) {
	_, log := fakeCLI(t, `"pull "*) exit 1 ;;`)
	b := &build{d: &runner{relayOrigin: "WARDEN_IMAGE"},
		req: &driver.BuildRequest{RelayImage: "registry.example/relay:v1"}}
	if err := b.resolveRelayImage(); err == nil {
		t.Fatal("an image that cannot be pulled was accepted")
	}
	data, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(data)); got != "pull registry.example/relay:v1" {
		t.Errorf("the runtime was asked %q, want one pull of the image named", got)
	}
}

package vz

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// TestWardenBootScriptTakesLease checks the prepared image's launcher bakes
// no link address and takes the relay's DHCP lease.
func TestWardenBootScriptTakesLease(t *testing.T) {
	s := wardenBootScript
	for _, want := range []string{
		`ipconfig set "$IFACE" DHCP`,
		`ipconfig getifaddr "$IFACE"`,
		`ipconfig getoption "$IFACE" subnet_mask`,
		`ipconfig getoption "$IFACE" router`,
		`exec "$AGENT" initialize --gateway="$GW" --ip="$SELF"`,
		"set State:/Network/Service/warden/IPv4",
		"set State:/Network/Service/warden/DNS",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("warden-boot lacks %q", want)
		}
	}
	for _, baked := range []string{"10.0.0.", "100.127.255.", "255.255.255.252"} {
		if strings.Contains(s, baked) {
			t.Errorf("warden-boot bakes %q", baked)
		}
	}
}

func TestImageStamp(t *testing.T) {
	for _, tc := range []struct {
		name string
		// setup lays out the cache and returns the image directory to check.
		setup func(t *testing.T, cache string) string
		fix   func(fix, cache, dir string) bool
	}{
		{name: "the active image",
			setup: func(t *testing.T, cache string) string {
				dir := makeImage(t, cache, "macos-a", t0)
				makeImage(t, cache, "macos-b", t0.Add(time.Hour))
				c := &ImageCache{CacheDir: cache}
				if err := c.SetActive(dir); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			fix: func(fix, _, _ string) bool { return fix == PrepareCommand }},
		{name: "the fallback active image (no pointer)",
			setup: func(t *testing.T, cache string) string {
				makeImage(t, cache, "macos-a", t0)
				return makeImage(t, cache, "macos-b", t0.Add(time.Hour))
			},
			fix: func(fix, _, _ string) bool { return fix == PrepareCommand }},
		{name: "a cache image that is not active",
			setup: func(t *testing.T, cache string) string {
				makeImage(t, cache, "macos-b", t0.Add(time.Hour))
				return makeImage(t, cache, "macos-a", t0)
			},
			fix: func(fix, _, dir string) bool {
				const omit = "omit --image to build from the active image"
				return strings.Contains(fix, PrepareCommand+" "+dir) &&
					strings.Contains(fix, omit)
			}},
		{name: "custom --image",
			setup: func(t *testing.T, _ string) string {
				return makeImage(t, t.TempDir(), "x", t0)
			},
			fix: isCustomImageFix},
		{name: "sibling with the cache as prefix",
			setup: func(t *testing.T, cache string) string {
				sibling := filepath.Base(cache) + "-other"
				return makeImage(t, filepath.Dir(cache), sibling, t0)
			},
			fix: isCustomImageFix},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := filepath.Join(t.TempDir(), "images")
			if err := os.MkdirAll(cache, 0o755); err != nil {
				t.Fatal(err)
			}
			c := &ImageCache{CacheDir: cache}
			dir := tc.setup(t, cache)
			var fe *driver.FixableError
			err := imageStamp(dir, c).Check()
			if !errors.As(err, &fe) || !tc.fix(fe.Fix, cache, dir) {
				t.Fatalf("an image prepared before the change: %#v", err)
			}
			if err := writeLinkStamp(dir); err != nil {
				t.Fatal(err)
			}
			if err := imageStamp(dir, c).Check(); err != nil {
				t.Fatalf("a re-prepared image: %v", err)
			}
			stamp := imageStamp(dir, c).Path
			if n, _ := buildlink.ReadStamp(stamp); n != buildlink.Channel {
				t.Fatalf("stamp channel %d", n)
			}
		})
	}
}

func isCustomImageFix(fix, cache, _ string) bool { return fix == customImageFix(cache) }

// TestLauncherStamps checks doctor reports the active image's stamp.
func TestLauncherStamps(t *testing.T) {
	cache := t.TempDir()
	c := &ImageCache{CacheDir: cache}
	s := launcherStamps(c)
	if len(s) != 1 || s[0].Path != "" || s[0].Rebuild != PrepareCommand {
		t.Fatalf("no image: %+v", s)
	}
	a := makeImage(t, cache, "macos-a", t0)
	makeImage(t, cache, "macos-b", t0.Add(time.Hour))
	if err := c.SetActive(a); err != nil {
		t.Fatal(err)
	}
	s = launcherStamps(c)
	if len(s) != 1 || s[0].Path != filepath.Join(a, linkStampName) ||
		s[0].Rebuild != PrepareCommand {
		t.Fatalf("stamps %+v, want the active image's", s)
	}
}

func TestSubnetOf(t *testing.T) {
	link, _ := buildlink.Parse("100.126.255.252/30")
	s := subnetOf(link)
	if s.RelayIP.String() != "100.126.255.253" || s.BuildIP.String() != "100.126.255.254" ||
		s.Gateway.String() != "100.126.255.253" || s.Netmask.String() != "fffffffc" {
		t.Fatalf("subnet = %+v", s)
	}
}

// TestRunVMsOrder pins BOOT-01's ordering on vz: the build VM boots only after
// the relay step (network, relay, claim) has returned, never after it failed.
func TestRunVMsOrder(t *testing.T) {
	sess, err := driver.StartSession(driver.SessionConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var log []string
	step := func(name string) func() (func(), error) {
		return func() (func(), error) {
			log = append(log, name)
			return func() { log = append(log, "stop "+name) }, nil
		}
	}
	_ = runVMs(ctx, sess, step("relay"), step("build"))
	if got := strings.Join(log, ","); got != "relay,build,stop build,stop relay" {
		t.Fatalf("order = %s", got)
	}
	log = nil
	failed := func() (func(), error) { return nil, errors.New("claim refused") }
	if err := runVMs(ctx, sess, failed, step("build")); err == nil || len(log) != 0 {
		t.Fatalf("after a failed claim: %v %v", err, log)
	}
}

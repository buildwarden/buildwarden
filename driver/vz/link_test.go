package vz

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	dir := t.TempDir()
	var fe *driver.FixableError
	if err := imageStamp(dir).Check(); !errors.As(err, &fe) || fe.Fix != PrepareCommand {
		t.Fatalf("an image prepared before the change: %v", err)
	}
	if err := writeLinkStamp(dir); err != nil {
		t.Fatal(err)
	}
	if err := imageStamp(dir).Check(); err != nil {
		t.Fatalf("a re-prepared image: %v", err)
	}
	if n, _ := buildlink.ReadStamp(imageStamp(dir).Path); n != buildlink.Channel {
		t.Fatalf("stamp channel %d", n)
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

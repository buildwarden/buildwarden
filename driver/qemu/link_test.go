package qemu

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

func TestDirectBootAppend(t *testing.T) {
	configured, _ := buildlink.Parse("100.126.255.252/30")
	tests := []struct {
		arch string
		link buildlink.Link
		want string
	}{
		{"x86_64", buildlink.Default(), "console=ttyS0 " +
			"warden.link_ip=100.127.255.254/30 warden.link_gw=100.127.255.253"},
		{"aarch64", configured, "console=ttyAMA0 " +
			"warden.link_ip=100.126.255.254/30 warden.link_gw=100.126.255.253"},
	}
	for _, tt := range tests {
		req := &driver.BuildRequest{BuildLink: tt.link}
		cfg := &buildVMConfig{Arch: tt.arch, BootArgs: req.Link().BootArgs()}
		if got := directBootAppend(cfg); got != tt.want {
			t.Errorf("%s: -append %q, want %q", tt.arch, got, tt.want)
		}
	}
	if got := directBootAppend(&buildVMConfig{Arch: "x86_64"}); got != "console=ttyS0" {
		t.Errorf("no link: %q", got)
	}
}

type fakeVM struct {
	name string
	log  *[]string
}

func (f fakeVM) stop() { *f.log = append(*f.log, "stop "+f.name) }

// TestRunVMsOrder pins BOOT-01's ordering: the build VM boots only after the
// relay step (boot and claim) has returned, and never when it failed.
func TestRunVMsOrder(t *testing.T) {
	sess, err := driver.StartSession(driver.SessionConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Wait returns at once

	var log []string
	d := &Driver{
		relayStep: func() (stopper, error) {
			log = append(log, "relay claimed")
			return fakeVM{"relay", &log}, nil
		},
		buildStep: func() (stopper, error) {
			log = append(log, "build booted")
			return fakeVM{"build", &log}, nil
		},
	}
	_ = d.runVMs(ctx, sess, &driver.BuildRequest{}, nil, &buildVMConfig{}, "", "", "")
	want := "relay claimed,build booted,stop build,stop relay"
	if got := strings.Join(log, ","); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}

	log = nil
	d.relayStep = func() (stopper, error) { return nil, errors.New("claim refused") }
	if err := d.runVMs(ctx, sess, &driver.BuildRequest{}, nil, &buildVMConfig{},
		"", "", ""); err == nil || len(log) != 0 {
		t.Fatalf("after a failed claim: err %v, log %v", err, log)
	}
}

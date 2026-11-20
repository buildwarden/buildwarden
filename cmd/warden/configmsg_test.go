package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
	"github.com/buildwarden/buildwarden/driver/hyperv"
)

// The two messages that name the config file for a bad value of a driver's
// key under its [vm] old name name the file that was read, by its path: with
// --config it is not ./warden.toml. The driver reads the key, through the
// kit, as warden runs a linked driver.
func TestBadVMValueNamesTheFileRead(t *testing.T) {
	t.Run("[vm] image_refresh_after", func(t *testing.T) {
		inConfigFiles(t, "")
		path := write(t, t.TempDir(), "ci.toml",
			"[vm]\nimage_refresh_after = \"soon\"\n")
		probe := &driver.Start{Contract: 1, Probe: true, Mode: driver.ModeBuild,
			GuestOS: "linux", ConfigFile: path, Args: []string{}}
		_, err := callLinkedRun(context.Background(), linkedDrivers["qemu"], "qemu",
			&Config{}, &driver.BuildRequest{}, probe, nil)
		if err == nil || !strings.HasPrefix(err.Error(),
			path+" [vm] image_refresh_after: ") {
			t.Errorf("the probe = %v, want the error to name %s", err, path)
		}
	})
	t.Run("[vm] windows_image_update", func(t *testing.T) {
		inConfigFiles(t, "")
		path := write(t, t.TempDir(), "ci.toml",
			"[vm]\nwindows_image_update = \"sometimes\"\n")
		set, err := drivercmd.Resolve("hyperv", hyperv.Settings,
			drivercmd.CmdImagePrepare, nil, path, os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		_, err = prepareHyperVWindows(context.Background(), "", prepareOptionsFrom(set))
		want := path + ` [vm] windows_image_update = "sometimes"`
		wantFixable(t, err, want, `set it to "auto" or "reuse"`)
	})
}

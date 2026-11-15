package main

import (
	"context"
	"strings"
	"testing"
)

// The two messages that name the config file for a bad [vm] value name the
// file that was read, by its path: with --config it is not ./warden.toml.
func TestBadVMValueNamesTheFileRead(t *testing.T) {
	t.Run("[vm] image_refresh_after", func(t *testing.T) {
		wd := inConfigFiles(t, "")
		write(t, wd, "Dockerfile", "FROM alpine\n")
		path := write(t, t.TempDir(), "ci.toml",
			"[vm]\nimage_refresh_after = \"soon\"\n")
		giveConfigFlag(t, path)
		cfg, err := resolveConfig()
		if err != nil {
			t.Fatalf("resolveConfig: %v", err)
		}
		_, err = newRequest(cfg, nil)
		if err == nil || !strings.HasPrefix(err.Error(),
			path+" [vm] image_refresh_after: ") {
			t.Errorf("newRequest = %v, want the error to name %s", err, path)
		}
	})
	t.Run("[vm] windows_image_update", func(t *testing.T) {
		inConfigFiles(t, "")
		path := write(t, t.TempDir(), "ci.toml",
			"[vm]\nwindows_image_update = \"sometimes\"\n")
		giveConfigFlag(t, path)
		cfg, err := resolveConfig()
		if err != nil {
			t.Fatalf("resolveConfig: %v", err)
		}
		_, err = prepareHyperVWindows(context.Background(), "", cfg)
		want := path + ` [vm] windows_image_update = "sometimes"`
		wantFixable(t, err, want, `set it to "auto" or "reuse"`)
	})
}

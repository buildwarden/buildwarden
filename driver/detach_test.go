package driver

import (
	"slices"
	"testing"
)

// The background image prepare is given the config file its build read, and
// nothing when the build read none.
func TestDetachedArgs(t *testing.T) {
	orig := ConfigFile
	t.Cleanup(func() { ConfigFile = orig })
	args := []string{"image", "prepare", "--driver", "qemu"}

	ConfigFile = "/src/ci/warden-ci.toml"
	want := append([]string{"--config", "/src/ci/warden-ci.toml"}, args...)
	if got := detachedArgs(args); !slices.Equal(got, want) {
		t.Errorf("with a file: %q, want %q", got, want)
	}
	ConfigFile = ""
	if got := detachedArgs(args); !slices.Equal(got, args) {
		t.Errorf("with none: %q, want %q", got, args)
	}
}

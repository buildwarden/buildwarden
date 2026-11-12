package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// inConfigDirs points the user config at a fresh home and runs from a fresh
// project directory, writing each non-empty build_link into its file.
func inConfigDirs(t *testing.T, user, project string) {
	t.Helper()
	home, proj := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if user != "" {
		dir := filepath.Join(home, ".config", "warden")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, dir, "config.toml", "[vm]\nbuild_link = \""+user+"\"\n")
	}
	if project != "" {
		write(t, proj, "warden.toml", "[vm]\nbuild_link = \""+project+"\"\n")
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestResolveBuildLinkPrecedence(t *testing.T) {
	defer func() { flagBuildLink = "" }()
	tests := []struct {
		name, user, project, flag string
		want, source              string
	}{
		{"default with no config file", "", "", "", buildlink.DefaultCIDR, "default"},
		{"user config", "100.126.0.0/30", "", "", "100.126.0.0/30", "config.toml"},
		{"project over user", "100.126.0.0/30", "100.125.0.0/30", "",
			"100.125.0.0/30", "warden.toml"},
		{"flag over both", "100.126.0.0/30", "100.125.0.0/30", "100.124.0.4/30",
			"100.124.0.4/30", "--build-link"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inConfigDirs(t, tt.user, tt.project)
			flagBuildLink = tt.flag
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			link, source, err := resolveBuildLink(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if link.String() != tt.want || source != tt.source {
				t.Fatalf("resolved %s (%s), want %s (%s)", link, source, tt.want,
					tt.source)
			}
		})
	}
}

func TestBadBuildLinkFailsAtLoad(t *testing.T) {
	for _, bad := range []string{"100.126.0.1/30", "100.126.0.0/29", "10.0.2.0/30",
		"127.0.0.0/30", "nonsense"} {
		inConfigDirs(t, "", bad)
		_, err := LoadConfig()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg,
			"[vm] build_link in warden.toml") ||
			!strings.Contains(fe.Fix, exampleBuildLink) {
			t.Errorf("%s: err = %#v", bad, err)
		}
	}
}

// TestBuildLinkInHyperVNATLoads: the Hyper-V NAT is read back from the host
// and checked by the hyperv driver, so config load does not refuse a link
// in its default prefix (native F3: that refusal named the default while the
// host's NAT was elsewhere).
func TestBuildLinkInHyperVNATLoads(t *testing.T) {
	for _, link := range []string{"192.168.250.4/30", "192.168.240.252/30"} {
		inConfigDirs(t, "", link)
		cfg, err := LoadConfig()
		if err != nil || cfg.VM.BuildLink != link {
			t.Errorf("%s: cfg %+v, err = %v", link, cfg.VM, err)
		}
	}
}

func TestBuildLinkFlagChecked(t *testing.T) {
	defer func() { flagBuildLink = "" }()
	inConfigDirs(t, "", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	for flag, want := range map[string]string{
		"100.64.87.8/30":     "the container driver's network",
		"100.64.90.0/30":     "the firecracker driver's upstream range",
		"10.0.2.0/30":        "qemu's user-mode network",
		"100.126.0.2/30":     "did you mean 100.126.0.0/30",
	} {
		flagBuildLink = flag
		_, _, err := resolveBuildLink(cfg)
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "--build-link") ||
			!strings.Contains(fe.Msg, want) {
			t.Errorf("%s: err = %v, want it to name %q", flag, err, want)
		}
	}
}

func TestBuildLinkFlagNeedsAVMDriver(t *testing.T) {
	defer func() { flagBuildLink, flagDriver = "", "" }()
	inConfigDirs(t, "", "")
	flagBuildLink = buildlink.DefaultCIDR
	for _, d := range []string{"", "container"} {
		if err := validateFlagsForDriver(d); err == nil ||
			!strings.Contains(err.Error(), "--build-link") {
			t.Errorf("build/shell with driver %q: err = %v", d, err)
		}
		flagDriver = d
		if err := doctorCmd.RunE(doctorCmd, nil); err == nil ||
			!strings.Contains(err.Error(), "--build-link") {
			t.Errorf("doctor with driver %q: err = %v", d, err)
		}
	}
	for _, d := range []string{"qemu", "vz", "hyperv", "firecracker"} {
		if err := validateFlagsForDriver(d); err != nil {
			t.Errorf("--build-link refused by %s: %v", d, err)
		}
	}
}

func TestBuildLinkFlagOnlyWhereItApplies(t *testing.T) {
	for _, c := range []struct {
		name string
		has  bool
		ok   bool
	}{
		{"build", buildCmd.Flags().Lookup("build-link") != nil, true},
		{"shell", shellCmd.Flags().Lookup("build-link") != nil, true},
		{"doctor", doctorCmd.Flags().Lookup("build-link") != nil, true},
		{"setup", setupCmd.Flags().Lookup("build-link") != nil, false},
		{"image prepare", imagePrepareCmd.Flags().Lookup("build-link") != nil, false},
	} {
		if c.has != c.ok {
			t.Errorf("%s: has --build-link = %v, want %v", c.name, c.has, c.ok)
		}
	}
}

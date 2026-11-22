package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

func buildLinkFile(link string) string {
	if link == "" {
		return ""
	}
	return "[vm]\nbuild_link = \"" + link + "\"\n"
}

func TestResolveBuildLinkPrecedence(t *testing.T) {
	defer func() { flagBuildLink = "" }()
	tests := []struct {
		name, file, flag string
		named            string // the file's name when --config names it elsewhere
		want, source     string
	}{
		{name: "default with no config file", want: buildlink.DefaultCIDR,
			source: "default"},
		{name: "the config file", file: "100.126.0.0/30", want: "100.126.0.0/30",
			source: "warden.toml"},
		{name: "flag over the file", file: "100.125.0.0/30", flag: "100.124.0.4/30",
			want: "100.124.0.4/30", source: "--build-link"},
		{name: "a file --config names, under its own name", file: "100.125.0.0/30",
			named: "ci.toml", want: "100.125.0.0/30", source: "ci.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.named == "" {
				inConfigFiles(t, buildLinkFile(tt.file))
			} else {
				inConfigFiles(t, "")
				giveConfigFlag(t,
					write(t, t.TempDir(), tt.named, buildLinkFile(tt.file)))
			}
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
		inConfigFiles(t, buildLinkFile(bad))
		_, err := LoadConfig()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Msg,
			"[vm] build_link in "+configPath(t)) ||
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
		inConfigFiles(t, buildLinkFile(link))
		cfg, err := LoadConfig()
		if err != nil || cfg.VM.BuildLink != link {
			t.Errorf("%s: cfg %+v, err = %v", link, cfg.VM, err)
		}
	}
}

func TestBuildLinkFlagChecked(t *testing.T) {
	defer func() { flagBuildLink = "" }()
	inConfigFiles(t, "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	for flag, want := range map[string]string{
		"100.64.87.8/30": "the container driver's network",
		"100.64.90.0/30": "the firecracker driver's upstream range",
		"10.0.2.0/30":    "qemu's user-mode network",
		"100.126.0.2/30": "did you mean 100.126.0.0/30",
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
	inConfigFiles(t, "")
	flagBuildLink = buildlink.DefaultCIDR
	for _, d := range []string{"", "container"} {
		if err := validateFlagsForDriver(isVMDriver(d)); err == nil ||
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
		if err := validateFlagsForDriver(isVMDriver(d)); err != nil {
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

package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/buildwarden/buildwarden/driver/hyperv"
)

// precedenceLevel is one level of a key: the config file's (file holds the
// TOML), or a variable's or a flag's (set sets it). want is the value the
// key resolves to when this is the highest level set.
type precedenceLevel struct {
	name string
	file string
	set  func(t *testing.T)
	want string
}

// resetPrecedenceFlags clears, when the test ends, the flags this file sets
// that inConfigFiles does not reset, and the variables it reads.
func resetPrecedenceFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"WARDEN_IMAGE", "WARDEN_VERBOSE", "NO_COLOR"} {
		t.Setenv(name, "")
	}
	noColor := color.NoColor
	t.Cleanup(func() {
		flagOutput, flagCapture, flagNetwork, flagBuildLink = "", "", "", ""
		flagColor, flagImgSource = "", ""
		flagCPUs, flagMemoryMB = 0, 0
		flagNoCompress, flagVerbose = false, false
		color.NoColor = noColor
	})
}

func setEnv(name, v string) func(*testing.T) {
	return func(t *testing.T) { t.Setenv(name, v) }
}

func setFlag(p *string, v string) func(*testing.T) {
	return func(*testing.T) { *p = v }
}

func setIntFlag(p *int, v int) func(*testing.T) {
	return func(*testing.T) { *p = v }
}

// The precedence, key by key, as the design's table has it: for every key,
// each level that exists replaces the levels below it, flag over variable
// over the config file over the default. Each row sets the levels from the
// file up to one, and the value is that level's. [relay] upstream_ca_certs
// is not here: its variable replaces the key, and both set is the
// transitional error (TestUpstreamCAPaths, TestUpstreamCAsBothSet).
func TestPrecedenceKeyByKey(t *testing.T) {
	str := func(s string) string { return strconv.Quote(s) }
	resolved := func(f func(*Config) (string, error)) func(*testing.T, *Config) string {
		return func(t *testing.T, cfg *Config) string {
			v, err := f(cfg)
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			return v
		}
	}
	keys := []struct {
		key    string
		def    string
		get    func(*testing.T, *Config) string
		levels []precedenceLevel
	}{
		{"[runtime] driver", "",
			func(_ *testing.T, c *Config) string { return c.Runtime.Driver },
			[]precedenceLevel{
				{name: "file", file: "[runtime]\ndriver = \"qemu\"\n",
					want: "qemu"},
				{name: "WARDEN_DRIVER", set: setEnv("WARDEN_DRIVER", "vz"),
					want: "vz"},
				{name: "--driver", set: setFlag(&flagDriver, "hyperv"),
					want: "hyperv"},
			}},
		{"[runtime] cli", "",
			func(_ *testing.T, c *Config) string { return c.Runtime.CLI },
			[]precedenceLevel{
				{name: "file", file: "[runtime]\ncli = \"podman\"\n",
					want: "podman"},
				{name: "WARDEN_CTR_CLI", set: setEnv("WARDEN_CTR_CLI", "docker"),
					want: "docker"},
				{name: "--runtime", set: setFlag(&flagRuntime, "finch"),
					want: "finch"},
			}},
		{"[runtime] relay_image", "",
			func(_ *testing.T, c *Config) string { return c.Runtime.RelayImage },
			[]precedenceLevel{
				{name: "file", file: "[runtime]\nrelay_image = \"file/relay:1\"\n",
					want: "file/relay:1"},
				{name: "WARDEN_IMAGE", set: setEnv("WARDEN_IMAGE", "env/relay:2"),
					want: "env/relay:2"},
			}},
		{"[relay] system_ca_bundle", "true",
			func(_ *testing.T, c *Config) string {
				return fmt.Sprint(resolveSystemCA(c))
			},
			[]precedenceLevel{
				{name: "file", file: "[relay]\nsystem_ca_bundle = false\n",
					want: "false"},
			}},
		{"[build] output_dir", "warden-output",
			func(_ *testing.T, c *Config) string { return resolveOutputDir(c) },
			[]precedenceLevel{
				{name: "file", file: "[build]\noutput_dir = \"from-file\"\n",
					want: "from-file"},
				{name: "-o", set: setFlag(&flagOutput, "from-flag"),
					want: "from-flag"},
			}},
		{"[build] capture", "", resolved(resolveCapture),
			[]precedenceLevel{
				{name: "file", file: "[build]\ncapture = \"headers\"\n",
					want: "headers"},
				{name: "--capture", set: setFlag(&flagCapture, "bodies"),
					want: "bodies"},
			}},
		{"[build] tmpdir", ".", func(t *testing.T, c *Config) string {
			v, err := resolveTempDir(c)
			if err != nil {
				t.Fatal(err)
			}
			return filepath.Base(v)
		}, []precedenceLevel{
			{name: "file",
				file: "[build]\ntmpdir = " + str(filepath.Join("x", "file-tmp")) +
					"\n", want: "file-tmp"},
			{name: "--tmpdir",
				set:  setFlag(&flagTempDir, filepath.Join("x", "flag-tmp")),
				want: "flag-tmp"},
		}},
		{"[vm] cpus", "0", func(_ *testing.T, c *Config) string {
			return fmt.Sprint(vmFlagOr(flagCPUs, c.VM.CPUs, true))
		}, []precedenceLevel{
			{name: "file", file: "[vm]\ncpus = 3\n", want: "3"},
			{name: "--cpus", set: setIntFlag(&flagCPUs, 5), want: "5"},
		}},
		{"[vm] memory_mb", "0", func(_ *testing.T, c *Config) string {
			return fmt.Sprint(vmFlagOr(flagMemoryMB, c.VM.MemoryMB, true))
		}, []precedenceLevel{
			{name: "file", file: "[vm]\nmemory_mb = 3072\n", want: "3072"},
			{name: "--memory", set: setIntFlag(&flagMemoryMB, 5120), want: "5120"},
		}},
		{"[vm] network", "", func(_ *testing.T, c *Config) string {
			return resolveNetwork(c, "hyperv")
		}, []precedenceLevel{
			{name: "file", file: "[vm]\nnetwork = \"file-net\"\n", want: "file-net"},
			{name: "--network", set: setFlag(&flagNetwork, "flag-net"),
				want: "flag-net"},
		}},
		{"[vm] build_link", "default", func(t *testing.T, c *Config) string {
			_, source, err := resolveBuildLink(c)
			if err != nil {
				t.Fatal(err)
			}
			return source
		}, []precedenceLevel{
			{name: "file", file: "[vm]\nbuild_link = " + str(exampleBuildLink) + "\n",
				want: projectConfig},
			{name: "--build-link", set: setFlag(&flagBuildLink, exampleBuildLink),
				want: "--build-link"},
		}},
		{"[vm] dns", "[]", func(_ *testing.T, c *Config) string {
			return fmt.Sprint(c.VM.DNSAddrs)
		}, []precedenceLevel{
			{name: "file", file: vmDNSFile(`"10.20.0.53"`), want: "[10.20.0.53]"},
		}},
		{"[vm] image_refresh_after", "0s", resolved(func(c *Config) (string, error) {
			d, err := configAge(c, "image_refresh_after", c.VM.ImageRefreshAfter)
			return d.String(), err
		}), []precedenceLevel{
			{name: "file", file: "[vm]\nimage_refresh_after = \"7d\"\n",
				want: "168h0m0s"},
		}},
		{"[vm] image_min_grace", "0s", resolved(func(c *Config) (string, error) {
			d, err := configAge(c, "image_min_grace", c.VM.ImageMinGrace)
			return d.String(), err
		}), []precedenceLevel{
			{name: "file", file: "[vm]\nimage_min_grace = \"36h\"\n", want: "36h0m0s"},
		}},
		{"[vm] windows_image", "", func(_ *testing.T, c *Config) string {
			src, _, _ := hypervImageSource(flagImgSource, c, hyperv.ImageStatus{})
			return src
		}, []precedenceLevel{
			{name: "file",
				file: "[vm]\nwindows_image = \"https://example.com/file.iso\"\n",
				want: "https://example.com/file.iso"},
			{name: "--source",
				set:  setFlag(&flagImgSource, "https://example.com/flag.iso"),
				want: "https://example.com/flag.iso"},
		}},
		{"[vm] windows_image_update", "", func(_ *testing.T, c *Config) string {
			return c.VM.WindowsImageUpdate
		}, []precedenceLevel{
			{name: "file", file: "[vm]\nwindows_image_update = \"reuse\"\n",
				want: "reuse"},
		}},
		{"[output] color", "auto", func(_ *testing.T, c *Config) string {
			return c.Output.Color
		}, []precedenceLevel{
			{name: "file", file: "[output]\ncolor = \"always\"\n", want: "always"},
			{name: "NO_COLOR", set: setEnv("NO_COLOR", "1"), want: "never"},
			{name: "--color", set: setFlag(&flagColor, "always"), want: "always"},
		}},
	}
	for _, k := range keys {
		for n := 0; n <= len(k.levels); n++ {
			name, want := "default", k.def
			if n > 0 {
				name, want = k.levels[n-1].name, k.levels[n-1].want
			}
			t.Run(k.key+" from "+name, func(t *testing.T) {
				body := ""
				for _, l := range k.levels[:n] {
					body += l.file
				}
				inConfigFiles(t, body)
				resetPrecedenceFlags(t)
				for _, l := range k.levels[:n] {
					if l.set != nil {
						l.set(t)
					}
				}
				cfg, err := resolveConfig()
				if err != nil {
					t.Fatalf("resolveConfig: %v", err)
				}
				if got := k.get(t, cfg); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

// The levels that can turn a setting one way only, asserted as far as they
// can say (the design's table hands on what they cannot): --no-compress turns
// compression off and cannot undo the file's false; -v and WARDEN_VERBOSE
// turn verbose on.
func TestPrecedenceOneWayLevels(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		set        func(*testing.T)
		get        func(*Config) bool
		want       bool
	}{
		{"compress by default", "", nil, resolveCompress, true},
		{"compress = false", "[build]\ncompress = false\n", nil, resolveCompress, false},
		{"--no-compress over compress = true", "[build]\ncompress = true\n",
			func(*testing.T) { flagNoCompress = true }, resolveCompress, false},
		{"verbose by default", "", nil, verbose, false},
		{"verbose = true", "[output]\nverbose = true\n", nil, verbose, true},
		{"WARDEN_VERBOSE over verbose = false", "[output]\nverbose = false\n",
			setEnv("WARDEN_VERBOSE", "1"), verbose, true},
		{"-v over verbose = false", "[output]\nverbose = false\n",
			func(*testing.T) { flagVerbose = true }, verbose, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inConfigFiles(t, tc.file)
			resetPrecedenceFlags(t)
			if tc.set != nil {
				tc.set(t)
			}
			cfg, err := resolveConfig()
			if err != nil {
				t.Fatalf("resolveConfig: %v", err)
			}
			if got := tc.get(cfg); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func verbose(c *Config) bool { return c.Output.Verbose }

// TestLoadConfig_Defaults is UX-07's automated part: with no file named, none
// in the working directory and no variable set, every setting is its default
// and no file is read.
func TestLoadConfig_Defaults(t *testing.T) {
	inConfigFiles(t, "")
	resetPrecedenceFlags(t)
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	want := &Config{Output: OutputConfig{Color: "auto"}}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got  %+v\nwant %+v", cfg, want)
	}
	if line := configLine(cfg); !strings.HasPrefix(line, "config: none") {
		t.Errorf("config line %q", line)
	}
}

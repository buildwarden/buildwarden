package drivercmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// The bundled drivers' declarations as the design's settings table gives
// them, which the kit's configuration cases are written against.
var (
	containerSettings = []Setting{
		{Name: "runtime", Type: String, Flag: "runtime", Key: "driver.container.cli",
			Aliases: []string{"runtime.cli"}, Env: "WARDEN_CTR_CLI",
			Commands: EveryCommand,
			Usage:    "container runtime (finch, docker, podman)"},
		{Name: "relay-image", Type: String, Key: "driver.container.relay_image",
			Aliases: []string{"runtime.relay_image"}, Env: "WARDEN_IMAGE",
			Commands: []string{CmdRun}, Usage: "relay image reference, or dev"},
	}
	ageSettings = func() []Setting {
		return []Setting{
			{Name: "image-refresh-after", Type: Age, Common: "image_refresh_after",
				Aliases:  []string{"vm.image_refresh_after"},
				Commands: []string{CmdRun},
				Usage:    "age at which a prepared Windows layer is refreshed"},
			{Name: "image-min-grace", Type: Age, Common: "image_min_grace",
				Aliases:  []string{"vm.image_min_grace"},
				Commands: []string{CmdRun},
				Usage:    "least evaluation grace a prepared image must have"},
		}
	}
	qemuSettings = append(ageSettings(),
		Setting{Name: "force", Type: Bool, Flag: "force",
			Commands: []string{CmdImagePrepare}, Usage: "re-acquire"},
		Setting{Name: "edition", Type: String, Flag: "edition",
			Commands: []string{CmdImagePrepare}, Usage: "Windows edition"})
	hypervSettings = append(ageSettings(),
		Setting{Name: "network", Type: String, Flag: "network",
			Key:     "driver.hyperv.network",
			Aliases: []string{"vm.network"}, Commands: []string{CmdRun, CmdDoctor,
				CmdSetup},
			Usage: "durable build switch name"},
		Setting{Name: "windows-image", Type: String, Key: "driver.hyperv.windows_image",
			Aliases: []string{"vm.windows_image"}, Commands: []string{CmdRun},
			Usage: "Windows image"},
		Setting{Name: "windows-image-update", Type: String,
			Key:      "driver.hyperv.windows_image_update",
			Aliases:  []string{"vm.windows_image_update"},
			Commands: []string{CmdRun}, Usage: "Windows image update policy"},
		Setting{Name: "slots", Type: Int, Flag: "slots", Commands: []string{CmdSetup},
			Usage: "build slots"})
)

// writeFile writes a config file in a test directory and returns its path.
func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "warden.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustResolve(t *testing.T, name string, settings []Setting, command string,
	args []string, file string, environ []string) driver.Settings {
	t.Helper()
	s, err := Resolve(name, settings, command, args, file, environ)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return s
}

func fixable(t *testing.T, err error, want ...string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v (%T), want a FixableError", err, err)
	}
	for _, w := range want {
		if !strings.Contains(fe.Msg, w) {
			t.Errorf("error %q does not name %q", fe.Msg, w)
		}
	}
}

// DRV-C8, DRV-C9, DRV-C11: the container driver's runtime by the precedence
// of main's request table (flag, variable, file), with the file written
// once under the old name and once under the new, to the same result.
func TestResolvePrecedenceContainer(t *testing.T) {
	for _, names := range []struct{ cli, image string }{
		{"[runtime]\ncli = \"podman\"\nrelay_image = \"file/relay:1\"\n", "old"},
		{"[driver.container]\ncli = \"podman\"\nrelay_image = \"file/relay:1\"\n", "new"},
	} {
		file := writeFile(t, names.cli)
		for _, tc := range []struct {
			name, args, env, want, wantImage, source string
		}{
			{name: "file", want: "podman", wantImage: "file/relay:1", source: "cli"},
			{name: "variable", env: "docker", want: "docker",
				wantImage: "file/relay:1",
				source:    "WARDEN_CTR_CLI"},
			{name: "flag", args: "--runtime=finch", env: "docker", want: "finch",
				wantImage: "file/relay:1", source: "--runtime"},
			{name: "empty flag", args: "--runtime=", env: "docker", want: "docker",
				wantImage: "file/relay:1", source: "WARDEN_CTR_CLI"},
		} {
			t.Run(names.image+"/"+tc.name, func(t *testing.T) {
				var args []string
				if tc.args != "" {
					args = []string{tc.args}
				}
				env := []string{"WARDEN_IMAGE="}
				if tc.env != "" {
					env = append(env, "WARDEN_CTR_CLI="+tc.env)
				}
				s := mustResolve(t, "container", containerSettings, CmdRun, args,
					file, env)
				if v, ok := s.String("runtime"); !ok || v != tc.want {
					t.Errorf("runtime = %q, %v; want %q", v, ok, tc.want)
				}
				if v, _ := s.String("relay-image"); v != tc.wantImage {
					t.Errorf("relay-image = %q, want %q", v, tc.wantImage)
				}
				if src := s.Source("runtime"); !strings.Contains(src, tc.source) {
					t.Errorf("Source = %q, want it to name %q", src, tc.source)
				}
			})
		}
	}
}

// DRV-C8: the five steps for a common key, with the worked example of
// v1.md as cases, for hyperv, qemu and the container driver.
func TestResolveWorkedExample(t *testing.T) {
	file := writeFile(t, `[driver]
image_refresh_after = "3d"

[driver.hyperv]
image_refresh_after = "12h"

[driver.qemu]
image_min_grace = "6h"

[driver.container]
cli = "podman"
`)
	day := 24 * time.Hour
	for _, tc := range []struct {
		driverName string
		settings   []Setting
		setting    string
		want       time.Duration
		set        bool
		step       string
	}{
		{"hyperv", hypervSettings, "image-refresh-after", 12 * time.Hour, true,
			"[driver.hyperv]"},
		{"hyperv", hypervSettings, "image-min-grace", 0, false, ""},
		{"qemu", qemuSettings, "image-refresh-after", 3 * day, true,
			"[driver] image_refresh_after"},
		{"qemu", qemuSettings, "image-min-grace", 6 * time.Hour, true, "[driver.qemu]"},
	} {
		t.Run(tc.driverName+"/"+tc.setting, func(t *testing.T) {
			s := mustResolve(t, tc.driverName, tc.settings, CmdRun, nil, file, nil)
			v, ok := s.Age(tc.setting)
			if ok != tc.set || v != tc.want {
				t.Fatalf("%s = %v, %v; want %v, %v", tc.setting, v, ok, tc.want,
					tc.set)
			}
			if !strings.Contains(s.Source(tc.setting), tc.step) {
				t.Errorf("Source = %q, want %q", s.Source(tc.setting), tc.step)
			}
		})
	}
	s := mustResolve(t, "container", containerSettings, CmdRun, nil, file, nil)
	if v, _ := s.String("runtime"); v != "podman" {
		t.Errorf("container runtime = %q, want podman", v)
	}
	s = mustResolve(t, "container", containerSettings, CmdRun, nil, file,
		[]string{"WARDEN_CTR_CLI=docker"})
	if v, _ := s.String("runtime"); v != "docker" {
		t.Errorf("container runtime with WARDEN_CTR_CLI = %q, want docker", v)
	}
	s = mustResolve(t, "container", containerSettings, CmdRun, []string{"--runtime=finch"},
		file, []string{"WARDEN_CTR_CLI=docker"})
	if v, _ := s.String("runtime"); v != "finch" {
		t.Errorf("container runtime with --runtime = %q, want finch", v)
	}
}

// DRV-C8, GLOBAL-10: a value in the driver's own table that cannot be used
// is refused, naming its place, and [driver]'s is not tried; an empty string
// there hides [driver]'s value.
func TestResolveOwnTableHidesDriver(t *testing.T) {
	file := writeFile(t, "[driver]\nimage_refresh_after = \"3d\"\n"+
		"[driver.qemu]\nimage_refresh_after = \"soon\"\n")
	_, err := Resolve("qemu", qemuSettings, CmdRun, nil, file, nil)
	fixable(t, err, file, "[driver.qemu] image_refresh_after", `invalid age "soon"`)

	file = writeFile(t, "[driver]\nimage_refresh_after = \"3d\"\n"+
		"[driver.qemu]\nimage_refresh_after = \"\"\n")
	s := mustResolve(t, "qemu", qemuSettings, CmdRun, nil, file, nil)
	if v, ok := s.Age("image-refresh-after"); !ok || v != 0 {
		t.Errorf("image-refresh-after = %v, %v; want the zero age from [driver.qemu]", v,
			ok)
	}
}

// DRV-C4, DRV-C5: a driver that lists no common key reads nothing in
// [driver]; another driver's table and warden's tables are ignored.
func TestResolveIgnoresWhatItDoesNotOwn(t *testing.T) {
	file := writeFile(t, `[driver]
image_refresh_after = 5
image_min_grace = [1]

[driver.qemu]
image_refresh_after = true

[build]
teardown_timeout = "60s"

[vm]
cpus = 4

[unknown]
x = { a = 1,
  b = 2, }
`)
	s := mustResolve(t, "container", containerSettings, CmdRun, nil, file, nil)
	if _, ok := s.String("runtime"); ok {
		t.Error("runtime is set by a file that does not set it")
	}
	s = mustResolve(t, "hyperv", hypervSettings[2:], CmdRun, nil, file, nil)
	if _, ok := s.String("network"); ok {
		t.Error("network is set by a file that does not set it")
	}
}

// DRV-C11: each of the seven old names is read as its key, with one warning
// naming the new name; a file that sets both names is refused naming both.
func TestResolveOldNames(t *testing.T) {
	settings := map[string][]Setting{"container": containerSettings,
		"qemu": qemuSettings, "hyperv": hypervSettings}
	for _, tc := range []struct{ driverName, setting, oldName, newName, value string }{
		{"container", "runtime", "runtime.cli", "driver.container.cli", "podman"},
		{"container", "relay-image", "runtime.relay_image", "driver.container.relay_image",
			"dev"},
		{"qemu", "image-refresh-after", "vm.image_refresh_after",
			"driver.image_refresh_after", "7d"},
		{"qemu", "image-min-grace", "vm.image_min_grace", "driver.image_min_grace", "36h"},
		{"hyperv", "image-refresh-after", "vm.image_refresh_after",
			"driver.image_refresh_after", "7d"},
		{"hyperv", "image-min-grace", "vm.image_min_grace", "driver.image_min_grace",
			"36h"},
		{"hyperv", "network", "vm.network", "driver.hyperv.network", "warden-ci"},
		{"hyperv", "windows-image", "vm.windows_image", "driver.hyperv.windows_image",
			"w.vhdx"},
		{"hyperv", "windows-image-update", "vm.windows_image_update",
			"driver.hyperv.windows_image_update", "reuse"},
	} {
		t.Run(tc.driverName+"/"+tc.oldName, func(t *testing.T) {
			toml := func(key string) string {
				i := strings.LastIndex(key, ".")
				return fmt.Sprintf("[%s]\n%s = %s\n", key[:i], key[i+1:],
					quote(tc.value))
			}
			read := func(file string) (any, []string) {
				s := mustResolve(t, tc.driverName, settings[tc.driverName], CmdRun,
					nil, file,
					[]string{"WARDEN_CTR_CLI=", "WARDEN_IMAGE="})
				if v, ok := s.Age(tc.setting); ok {
					return v, s.Warnings()
				}
				v, ok := s.String(tc.setting)
				if !ok {
					t.Fatalf("%s not set", tc.setting)
				}
				return v, s.Warnings()
			}
			oldFile, newFile := writeFile(t, toml(tc.oldName)), writeFile(t,
				toml(tc.newName))
			oldV, oldW := read(oldFile)
			newV, newW := read(newFile)
			if oldV != newV {
				t.Errorf("old name gives %v, new name %v", oldV, newV)
			}
			if len(newW) != 0 {
				t.Errorf("new name warns: %q", newW)
			}
			want := "[warden] warning: " + oldFile + ": " + label(tc.oldName) +
				" is deprecated; use " + label(tc.newName)
			if len(oldW) != 1 || oldW[0] != want {
				t.Errorf("warnings = %q, want [%q]", oldW, want)
			}
			both := writeFile(t, toml(tc.oldName)+toml(tc.newName))
			_, err := Resolve(tc.driverName, settings[tc.driverName], CmdRun, nil,
				both, nil)
			fixable(t, err, both, label(tc.oldName), label(tc.newName))
		})
	}
}

// DRV-C1, DRV-C4, DRV-C9, DRV-C11: a declaration that breaks the placement
// rules is refused, and not as the user's error.
func TestResolveRefusesDeclarations(t *testing.T) {
	base := Setting{Name: "x", Type: String, Commands: []string{CmdRun}, Usage: "u"}
	with := func(f func(*Setting)) []Setting {
		s := base
		f(&s)
		return []Setting{s}
	}
	for name, tc := range map[string]struct {
		driverName string
		settings   []Setting
	}{
		"key outside its table": {"nomad",
			with(func(s *Setting) { s.Key = "driver.other.x" })},
		"key at the top": {"nomad",
			with(func(s *Setting) { s.Key = "runtime.x" })},
		"key with a common name": {"nomad",
			with(func(s *Setting) { s.Key = "driver.nomad.image_min_grace" })},
		"unknown common key": {"nomad",
			with(func(s *Setting) { s.Common = "cpus"; s.Type = Age })},
		"key and common": {"nomad",
			with(func(s *Setting) {
				s.Key, s.Common = "driver.nomad.x", "image_min_grace"
			})},
		"alias not listed": {"nomad",
			with(func(s *Setting) {
				s.Key, s.Aliases = "driver.nomad.cli", []string{"runtime.cli"}
			})},
		"alias of another key": {"container",
			with(func(s *Setting) {
				s.Key, s.Aliases = "driver.container.x", []string{"runtime.cli"}
			})},
		"variable of its own": {"nomad",
			with(func(s *Setting) { s.Key = "driver.nomad.x"; s.Env = "NOMAD_X" })},
		"flag of the wrong form": {"nomad", with(func(s *Setting) { s.Flag = "X" })},
		"bad driver name":        {"Nomad", with(func(s *Setting) { s.Flag = "x" })},
		"neither flag nor key":   {"nomad", with(func(*Setting) {})},
		"no commands": {"nomad",
			with(func(s *Setting) { s.Flag = "x"; s.Commands = nil })},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(tc.driverName, tc.settings, CmdRun, nil, "", nil)
			var fe *driver.FixableError
			if err == nil || errors.As(err, &fe) {
				t.Fatalf("err = %v, want a declaration error", err)
			}
		})
	}
}

// DRV-C2, DRV-C6: a value of the wrong type is refused with the file and key
// named, and so is an element of args the driver did not declare.
func TestResolveRefusesValues(t *testing.T) {
	file := writeFile(t, "[driver.container]\ncli = 5\n")
	_, err := Resolve("container", containerSettings, CmdRun, nil, file, nil)
	fixable(t, err, file, "[driver.container] cli", "want a string, not an integer")

	file = writeFile(t, "[runtime]\ncli = 5\n")
	_, err = Resolve("container", containerSettings, CmdRun, nil, file, nil)
	fixable(t, err, file, "[runtime] cli")

	_, err = Resolve("hyperv", hypervSettings, CmdSetup, []string{"--slots=many"}, "", nil)
	fixable(t, err, "--slots", `"many" is not an integer`)

	_, err = Resolve("container", containerSettings, CmdRun, []string{"--regoin=eu"}, "", nil)
	fixable(t, err, "--regoin=eu")

	// A flag of another command of the same driver is not declared for this one.
	_, err = Resolve("hyperv", hypervSettings, CmdRun, []string{"--slots=2"}, "", nil)
	fixable(t, err, "--slots=2")

	_, err = Resolve("qemu", qemuSettings, CmdImagePrepare, []string{"--force=false"}, "", nil)
	fixable(t, err, "--force")
}

// DRV-C2: a switch is on when present; the last value of a flag wins.
func TestResolveArgs(t *testing.T) {
	s := mustResolve(t, "qemu", qemuSettings, CmdImagePrepare,
		[]string{"--edition=a", "--force", "--edition=Windows Server 2025"}, "", nil)
	if v, _ := s.String("edition"); v != "Windows Server 2025" {
		t.Errorf("edition = %q", v)
	}
	if v, ok := s.Bool("force"); !v || !ok {
		t.Errorf("force = %v, %v", v, ok)
	}
	s = mustResolve(t, "qemu", qemuSettings, CmdImagePrepare, nil, "", nil)
	if _, ok := s.Bool("force"); ok {
		t.Error("force is set with no --force")
	}
}

// DRV-C3: no file is read that was not named, whatever is in the working
// directory or WARDEN_CONFIG.
func TestResolveReadsOnlyTheNamedFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "warden.toml")
	if err := os.WriteFile(bad, []byte("[driver.container\ncli = 5"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	s := mustResolve(t, "container", containerSettings, CmdRun, nil, "",
		[]string{"WARDEN_CONFIG=" + bad})
	if _, ok := s.String("runtime"); ok {
		t.Error("runtime is set with no config file named")
	}
}

// DRV-C6: a named file that does not exist, or is not TOML, is refused on a
// command with a setting that has a key, and not read as if none had been
// named; on a command whose settings have no key it is not opened.
func TestResolveNamedFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.toml")
	_, err := Resolve("container", containerSettings, CmdRun, nil, missing, nil)
	fixable(t, err, missing, "does not exist")

	notTOML := writeFile(t, "this is not toml")
	_, err = Resolve("container", containerSettings, CmdDoctor, nil, notTOML, nil)
	fixable(t, err, notTOML)

	if _, err := Resolve("hyperv", hypervSettings, CmdSetup, nil, missing, nil); err != nil {
		// setup's network has a key, so the file is read.
		fixable(t, err, missing)
	} else {
		t.Error("setup read no file, though network has a key")
	}
	if _, err := Resolve("qemu", qemuSettings, CmdImagePrepare, nil, missing,
		nil); err != nil {
		t.Errorf("image-prepare has no keyed setting but opened the file: %v", err)
	}
}

// A Raw key is decoded whole from the one table that sets it.
func TestResolveRawDecode(t *testing.T) {
	settings := []Setting{{Name: "pools", Type: Raw, Key: "driver.nomad.pools",
		Commands: []string{CmdRun}, Usage: "pools"}}
	file := writeFile(t,
		"[driver.nomad]\npools = [{name = \"a\", size = 2}, {name = \"b\", size = 3}]\n")
	s := mustResolve(t, "nomad", settings, CmdRun, nil, file, nil)
	var pools []struct {
		Name string
		Size int
	}
	ok, err := s.Decode("pools", &pools)
	if err != nil || !ok {
		t.Fatalf("Decode: %v, %v", ok, err)
	}
	if len(pools) != 2 || pools[1].Name != "b" || pools[1].Size != 3 {
		t.Errorf("pools = %+v", pools)
	}
}

func TestResolveTypes(t *testing.T) {
	settings := []Setting{
		{Name: "link", Type: CIDR, Key: "driver.nomad.link", Commands: []string{CmdRun},
			Usage: "u"},
		{Name: "dns", Type: IPList, Flag: "dns", Key: "driver.nomad.dns",
			Commands: []string{CmdRun}, Usage: "u"},
		{Name: "dir", Type: Path, Key: "driver.nomad.dir", Commands: []string{CmdRun},
			Usage: "u"},
		{Name: "n", Type: Int, Key: "driver.nomad.n", Commands: []string{CmdRun},
			Usage: "u"},
	}
	file := writeFile(t, "[driver.nomad]\nlink = \"198.51.100.0/30\"\n"+
		"dns = [\"192.0.2.53\"]\ndir = \"/x\"\nn = 7\n")
	s := mustResolve(t, "nomad", settings, CmdRun, []string{"--dns=1.1.1.1, 8.8.8.8"}, file,
		nil)
	if p, _ := s.CIDR("link"); p != netip.MustParsePrefix("198.51.100.0/30") {
		t.Errorf("link = %v", p)
	}
	if l, _ := s.IPList("dns"); len(l) != 2 || l[1] != netip.MustParseAddr("8.8.8.8") {
		t.Errorf("dns = %v", l)
	}
	if p, _ := s.Path("dir"); p != "/x" {
		t.Errorf("dir = %q", p)
	}
	if n, _ := s.Int("n"); n != 7 {
		t.Errorf("n = %d", n)
	}
	bad := writeFile(t, "[driver.nomad]\ndns = [\"x\"]\n")
	_, err := Resolve("nomad", settings, CmdRun, nil, bad, nil)
	fixable(t, err, "[driver.nomad] dns", `"x" is not an IP address`)
}

// DRV-C10: in reap and attach nothing is opened until a setting is read; a
// named file that no longer exists is read as if none had been named, said
// on stderr; one that is there and cannot be used is the Settings' Err.
func TestResolveReapAttachLazy(t *testing.T) {
	for _, command := range []string{CmdReap, CmdAttach} {
		t.Run(command, func(t *testing.T) {
			notTOML := writeFile(t, "this is not toml")
			s, err := Resolve("container", containerSettings, command, nil, notTOML,
				nil)
			if err != nil {
				t.Fatalf("Resolve opened the file before a read: %v", err)
			}
			if s.Err() != nil {
				t.Fatalf("Err before a read: %v", s.Err())
			}
			if _, ok := s.String("runtime"); ok {
				t.Error("runtime is set from a file that is not TOML")
			}
			fixable(t, s.Err(), notTOML)

			missing := filepath.Join(t.TempDir(), "gone.toml")
			s, err = Resolve("container", containerSettings, command,
				[]string{"--runtime=docker"}, missing, nil)
			if err != nil {
				t.Fatal(err)
			}
			var notices bytes.Buffer
			if r, ok := s.(*resolved); ok {
				r.notices = &notices
			}
			if v, _ := s.String("runtime"); v != "docker" || s.Err() != nil {
				t.Errorf("runtime = %q, Err %v; want docker from args", v, s.Err())
			}
			if !strings.Contains(notices.String(), missing) {
				t.Errorf("stderr %q does not say the file is gone",
					notices.String())
			}
		})
	}
}

// DRV-C11: the warnings are printed once per command of warden: in a probe,
// and with no probe only in a command warden does not probe first; never in
// the run or clean that follows a probe, nor in reap or attach.
func TestCallWarnings(t *testing.T) {
	file := writeFile(t, "[runtime]\ncli = \"podman\"\n")
	d := kitDriver(t)
	d.Describe.Name = "container"
	d.Describe.Settings = containerSettings
	d.Clean = func(context.Context, *CleanRequest) (int, error) { return 0, nil }
	for _, tc := range []struct {
		command string
		probe   bool
		want    int
	}{
		{CmdRun, true, 1},
		{CmdRun, false, 0},
		{CmdClean, true, 1},
		{CmdClean, false, 0},
		{CmdDoctor, false, 1},
		{CmdReap, false, 0},
		{CmdAttach, false, 0},
	} {
		t.Run(tc.command+map[bool]string{true: " probe"}[tc.probe], func(t *testing.T) {
			var stderr bytes.Buffer
			req := &Request{Contract: 1, Probe: tc.probe, ConfigFile: file,
				BuildID:   "k3v9x2ab",
				StateFile: filepath.Join(t.TempDir(), "s.state"),
				KeepIDs:   []string{},
				Environ:   []string{}, Streams: Streams{Stdout: &bytes.Buffer{},
					Stderr: &stderr}}
			var host driver.Host
			if tc.command == CmdRun {
				req.Start = &driver.Start{Contract: 1, Probe: tc.probe,
					ConfigFile: file}
				host = &nullHost{}
			}
			if _, err := Call(context.Background(), d, tc.command, req,
				host); err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(stderr.String(), "is deprecated"); got != tc.want {
				t.Errorf("%d warnings, want %d: %q", got, tc.want, stderr.String())
			}
		})
	}
}

// The same settings through runMain, through Call and through Resolve, so
// that a linked driver and a driver binary read alike.
func TestResolveSameThroughEveryPath(t *testing.T) {
	file := writeFile(t, "[driver]\nimage_refresh_after = \"3d\"\n")
	env := []string{}
	viaResolve := mustResolve(t, "qemu", qemuSettings, CmdRun, nil, file, env)
	want, _ := viaResolve.Age("image-refresh-after")

	var got []time.Duration
	d := &Driver{Describe: Describe{Name: "qemu", Settings: qemuSettings,
		Capabilities: Capabilities{Keep: "none", Containerfile: "translate",
			GuestOS: []string{"linux"}}},
		Preflight: func(_ context.Context, start *driver.Start) error {
			v, _ := start.Settings.Age("image-refresh-after")
			got = append(got, v)
			return nil
		},
		Run: func(context.Context, *driver.Start, driver.Host) ([]string,
			error) {
			return nil, nil
		},
		Reap: func(context.Context, *ReapRequest) (int, error) { return 0, nil }}
	start := &driver.Start{Contract: 1, Probe: true, ConfigFile: file, Mode: "build",
		GuestOS: "linux", ContextDir: "/c"}
	if _, err := Call(context.Background(), d, CmdRun, &Request{Start: start, Environ: env},
		nil); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"start","contract":1,"probe":true,"mode":"build","guest_os":"linux",` +
		`"context_dir":"/c","config_file":` + quote(file) + `}` + "\n"
	var out, stderr bytes.Buffer
	if code := runMain(d, []string{CmdRun}, env, strings.NewReader(line), &out,
		&stderr); code != 0 {
		t.Fatalf("runMain exit %d: %s %s", code, out.String(), stderr.String())
	}
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Errorf("Call and runMain read %v, Resolve %v", got, want)
	}
	if !slices.Contains(strings.Fields(out.String()), `{"type":"stopped"}`) {
		t.Errorf("probe output %q, want stopped", out.String())
	}
}

// DRV-C12: each of the seven keys under its new name, its old name and, for
// the two ages, each driver's override, written in another ASCII letter
// case (the key, its table, or both, in each TOML form), is refused naming
// the file, the spelling and the key to write; before the value is checked,
// so a value the driver would refuse gets this error and not its own.
func TestResolveRefusesAnotherCase(t *testing.T) {
	settings := map[string][]Setting{"container": containerSettings,
		"qemu": qemuSettings, "hyperv": hypervSettings}
	const old = " (old name "
	for _, tc := range []struct {
		name, driverName, body, spelling, reads, fix string
	}{
		{"old name, both", "container", "[Runtime]\nCLI = \"podman\"\n", "[Runtime] CLI",
			"[driver.container] cli" + old + "[runtime] cli)",
			"write it as [driver.container] cli"},
		{"old name, key", "container", "[runtime]\nCLI = \"./rt\"\n", "[runtime] CLI",
			"[driver.container] cli" + old + "[runtime] cli)",
			"write it as [driver.container] cli"},
		{"old name, table", "container", "[RUNTIME]\ncli = \"sh\"\n", "[RUNTIME] cli",
			"[driver.container] cli" + old + "[runtime] cli)",
			"write it as [driver.container] cli"},
		{"old name, dotted", "container", "runtime.CLI = \"podman\"\n", "[runtime] CLI",
			"[driver.container] cli" + old + "[runtime] cli)",
			"write it as [driver.container] cli"},
		{"old name, inline", "container", "Runtime = { CLI = \"podman\" }\n",
			"[Runtime] CLI", "[driver.container] cli" + old + "[runtime] cli)",
			"write it as [driver.container] cli"},
		{"new name, table", "container", "[Driver.Container]\ncli = \"podman\"\n",
			"[Driver.Container] cli", "[driver.container] cli",
			"write it as [driver.container] cli"},
		{"new name, key", "container", "[driver.container]\nRelay_Image = \"dev\"\n",
			"[driver.container] Relay_Image", "[driver.container] relay_image",
			"write it as [driver.container] relay_image"},
		{"old relay image", "container", "[runtime]\nRELAY_IMAGE = \"dev\"\n",
			"[runtime] RELAY_IMAGE",
			"[driver.container] relay_image" + old + "[runtime] relay_image)",
			"write it as [driver.container] relay_image"},
		{"override", "qemu", "[driver.Qemu]\nimage_refresh_after = \"7d\"\n",
			"[driver.Qemu] image_refresh_after", "[driver.qemu] image_refresh_after",
			"write it as [driver.qemu] image_refresh_after"},
		{"common key", "qemu", "[Driver]\nimage_min_grace = \"bad\"\n",
			"[Driver] image_min_grace", "[driver] image_min_grace",
			"write it as [driver] image_min_grace"},
		{"old age", "qemu", "[vm]\nImage_Min_Grace = \"1d\"\n", "[vm] Image_Min_Grace",
			"[driver] image_min_grace" + old + "[vm] image_min_grace)",
			"write it as [driver] image_min_grace"},
		{"hyperv override", "hyperv", "[driver.HyperV]\nimage_min_grace = \"1d\"\n",
			"[driver.HyperV] image_min_grace", "[driver.hyperv] image_min_grace",
			"write it as [driver.hyperv] image_min_grace"},
		{"hyperv old age", "hyperv", "[VM]\nimage_refresh_after = \"1d\"\n",
			"[VM] image_refresh_after",
			"[driver] image_refresh_after" + old + "[vm] image_refresh_after)",
			"write it as [driver] image_refresh_after"},
		{"hyperv network", "hyperv", "[vm]\nNetwork = 3\n", "[vm] Network",
			"[driver.hyperv] network" + old + "[vm] network)",
			"write it as [driver.hyperv] network"},
		{"hyperv image", "hyperv", "[driver.hyperv]\nWindows_Image = \"w\"\n",
			"[driver.hyperv] Windows_Image", "[driver.hyperv] windows_image",
			"write it as [driver.hyperv] windows_image"},
		{"hyperv image update", "hyperv", "[Vm]\nwindows_image_update = \"reuse\"\n",
			"[Vm] windows_image_update",
			"[driver.hyperv] windows_image_update" + old +
				"[vm] windows_image_update)",
			"write it as [driver.hyperv] windows_image_update"},
		// Two spellings in one table: one message, every time.
		{"two spellings", "container", "[runtime]\ncli = \"docker\"\nCLI = \"podman\"\n",
			"[runtime] holds cli and CLI",
			"[driver.container] cli" + old + "[runtime] cli)",
			"keep one, written as [driver.container] cli"},
		// Before DRV-C11's refusal of both names, and with the key to
		// write already there, the fix is to remove the other spelling.
		{"beside the new name", "container",
			"[Runtime]\nCLI = \"podman\"\n[driver.container]\ncli = \"docker\"\n",
			"[Runtime] CLI", "[driver.container] cli" + old + "[runtime] cli)",
			"remove [Runtime] CLI; [driver.container] cli is already set"},
		{"three spellings", "container",
			"[Runtime]\ncli = 1\n[RUNTIME]\ncli = 2\n",
			"[RUNTIME] cli and [Runtime] cli",
			"[driver.container] cli" + old + "[runtime] cli)",
			"keep one, written as [driver.container] cli"},
	} {
		t.Run(tc.driverName+"/"+tc.name, func(t *testing.T) {
			file := writeFile(t, tc.body)
			_, err := Resolve(tc.driverName, settings[tc.driverName], CmdRun, nil,
				file, []string{})
			want := file + ": " + tc.spelling + ": keys are case-sensitive, and the " +
				tc.driverName + " driver reads " + tc.reads
			var fe *driver.FixableError
			if !errors.As(err, &fe) {
				t.Fatalf("err = %v, want a FixableError", err)
			}
			if fe.Msg != want || fe.Fix != tc.fix {
				t.Errorf("error\n %q\n %q\nwant\n %q\n %q", fe.Msg, fe.Fix, want,
					tc.fix)
			}
		})
	}
}

// DRV-C12: two spellings in one table are refused with the same message on
// every run, whatever order the decoder gives its keys in.
func TestResolveAnotherCaseIsStable(t *testing.T) {
	file := writeFile(t, "[runtime]\nCLI = \"podman\"\ncli = \"docker\"\nCli = \"x\"\n")
	var first string
	for i := range 100 {
		_, err := Resolve("container", containerSettings, CmdRun, nil, file, []string{})
		if err == nil {
			t.Fatal("not refused")
		}
		if i == 0 {
			first = err.Error()
			if !strings.Contains(first, "[runtime] holds cli, CLI and Cli") {
				t.Fatalf("err = %q, want the three spellings in byte order", first)
			}
		}
		if err.Error() != first {
			t.Fatalf("run %d: %q, run 0: %q", i, err, first)
		}
	}
}

// DRV-C5, DRV-C12: what equals no listed place with ASCII letters folded is
// ignored: an unlisted key in upper case, a quoted key whose non-ASCII
// letter folds to a listed one's only under Unicode's folding, and another
// driver's key in another case while that driver is not selected.
func TestResolveIgnoresOtherKeysInAnyCase(t *testing.T) {
	file := writeFile(t, `[Driver]
CPUS = 4
[driver.HyperV]
NETWORK = "x"
[Runtime]
CLI = "podman"
[vm]
"window`+"ſ"+`_image" = "w"
[driver.container]
IMAGE_MIN_GRACE = 1
`)
	for name, set := range map[string][]Setting{"qemu": qemuSettings,
		"hyperv": hypervSettings[:2]} {
		s := mustResolve(t, name, set, CmdRun, nil, file, []string{})
		if _, ok := s.Age("image-refresh-after"); ok {
			t.Errorf("%s: image-refresh-after set by a file that does not set it",
				name)
		}
	}
	h := mustResolve(t, "hyperv", hypervSettings[3:4], CmdRun, nil, file, []string{})
	if _, ok := h.String("windows-image"); ok {
		t.Error("a non-ASCII letter is folded")
	}
}

// DRV-C4: a declaration with two places equal when ASCII letters are folded
// is the driver's defect, refused and not as the user's error.
func TestResolveRefusesPlacesEqualFolded(t *testing.T) {
	for name, set := range map[string][]Setting{
		"two keys": {
			{Name: "a", Type: String, Key: "driver.nomad.region",
				Commands: []string{CmdRun}, Usage: "u"},
			{Name: "b", Type: String, Key: "driver.nomad.Region",
				Commands: []string{CmdRun}, Usage: "u"}},
		"a key and an override": {
			{Name: "a", Type: Age, Common: "image_min_grace",
				Commands: []string{CmdRun}, Usage: "u"},
			{Name: "b", Type: String, Key: "driver.nomad.Image_Min_Grace",
				Commands: []string{CmdRun}, Usage: "u"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve("nomad", set, CmdRun, nil, "", nil)
			var fe *driver.FixableError
			if err == nil || errors.As(err, &fe) {
				t.Fatalf("err = %v, want a declaration error", err)
			}
		})
	}
}

// DRV-C7, DRV-C12: the refusal is the same through Call, as a linked driver
// runs, and through Main, as a driver binary runs: in the probe of run, and
// first in a command that is not probed (doctor).
func TestAnotherCaseThroughCallAndMain(t *testing.T) {
	file := writeFile(t, "[Runtime]\nCLI = \"podman\"\n")
	want := "[Runtime] CLI: keys are case-sensitive, and the container driver " +
		"reads [driver.container] cli (old name [runtime] cli)"
	d := kitDriver(t)
	d.Describe.Name = "container"
	d.Describe.Settings = containerSettings
	ran := false
	d.Preflight = func(context.Context, *driver.Start) error { ran = true; return nil }
	start := &driver.Start{Contract: 1, Probe: true, ConfigFile: file, Mode: "build",
		GuestOS: "linux", ContextDir: "/c"}
	_, err := Call(context.Background(), d, CmdRun, &Request{Start: start,
		Environ: []string{}}, nil)
	fixable(t, err, file, want)
	if ran {
		t.Error("the probe ran Preflight after the refusal")
	}
	_, err = Call(context.Background(), d, CmdDoctor, &Request{Contract: 1,
		ConfigFile: file, Environ: []string{}}, nil)
	fixable(t, err, want)

	for _, args := range [][]string{{CmdRun}, {CmdDoctor}} {
		req := `{"contract":1,"config_file":` + quote(file) + `}`
		if args[0] == CmdRun {
			req = `{"type":"start","contract":1,"probe":true,"mode":"build",` +
				`"guest_os":"linux","context_dir":"/c","config_file":` +
				quote(file) + "}\n"
		}
		code, stdout, res := invoke(t, d, args, req)
		if code != 1 {
			t.Errorf("%s: exit %d, want 1", args[0], code)
		}
		e, _ := res["error"].(map[string]any)
		if e == nil {
			// run reports its error on stdout, as an error message.
			if !strings.Contains(stdout, `"code":"fixable"`) ||
				!strings.Contains(stdout, "keys are case-sensitive") {
				t.Errorf("%s: stdout %q, want the fixable refusal", args[0],
					stdout)
			}
			continue
		}
		if msg, _ := e["msg"].(string); e["code"] != "fixable" ||
			!strings.Contains(msg, want) {
			t.Errorf("%s: error %v, want the fixable refusal", args[0], e)
		}
	}
}

// DRV-C10, DRV-C12: in reap and attach the state file is authoritative, so
// with one a miscased key in the recorded file is never refused; with none,
// the recorded file is read and the key in another case refused.
func TestAnotherCaseInReapAndAttach(t *testing.T) {
	file := writeFile(t, "[Runtime]\nCLI = \"podman\"\n")
	d := kitDriver(t)
	d.Describe.Name = "container"
	d.Describe.Settings = containerSettings
	runtimeOf := func(stateFile string, set driver.Settings) error {
		var st struct{ Runtime string }
		found, err := LoadState(stateFile, &st)
		if err != nil || found {
			return err
		}
		set.String("runtime")
		return set.Err()
	}
	d.Reap = func(_ context.Context, req *ReapRequest) (int, error) {
		return 0, runtimeOf(req.StateFile, req.Settings)
	}
	d.Attach = func(_ context.Context, req *AttachRequest) (int, error) {
		return 0, runtimeOf(req.StateFile, req.Settings)
	}
	for _, command := range []string{CmdReap, CmdAttach} {
		for _, withState := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/state=%v", command, withState), func(t *testing.T) {
				stateFile := filepath.Join(t.TempDir(), "k3v9x2ab.state")
				if withState {
					if err := SaveState(stateFile, map[string]string{
						"Runtime": "docker"}); err != nil {
						t.Fatal(err)
					}
				}
				req := `{"contract":1,"build_id":"k3v9x2ab","state_file":` +
					quote(stateFile) + `,"config_file":` + quote(file) + `}`
				code, _, res := invoke(t, d, []string{command, "k3v9x2ab"}, req)
				e, _ := res["error"].(map[string]any)
				msg, _ := e["msg"].(string)
				switch {
				case withState && (code != 0 || e != nil):
					t.Errorf("exit %d, error %v; want 0 with a state file",
						code, e)
				case !withState && (code != 1 || e["code"] != "fixable" ||
					!strings.Contains(msg, "keys are case-sensitive")):
					t.Errorf("exit %d, error %v; want the fixable refusal",
						code, e)
				}
			})
		}
	}
}

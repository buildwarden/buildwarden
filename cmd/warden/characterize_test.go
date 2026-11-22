package main

// Characterization tests (GLOBAL-04): what warden does today, written down
// as goldens before the driver contract moves its code, so that each later
// change is diffed against them. They hold the help of every command, the
// command lines refused before any work, the request each driver is handed,
// the inputs staged for each driver, the image a VM build resolves from a
// Containerfile, and the output of image list and image verify. A golden is
// rewritten with `go test ./cmd/warden -run Characterize -update`; a change
// to one is a change of behaviour, and the design that makes it names it.

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/container"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/ledger"
)

var update = flag.Bool("update", false, "rewrite the characterization goldens")

// goldenDir is taken before any test changes the working directory.
var goldenDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(wd, "testdata", "characterize")
}()

// golden compares got with testdata/characterize/<name>.golden, or writes
// the file under -update. The default build link is shown as $DEFAULT_LINK:
// it is written once, in driver/buildlink (TestNoStrayLinkLiterals). The test
// binary's directory, where a driver beside warden is looked for, is shown as
// $WARDEN_DIR.
func golden(t *testing.T, name, got string) {
	t.Helper()
	got = strings.ReplaceAll(got, buildlink.DefaultCIDR, "$DEFAULT_LINK")
	if env := hostLookupEnv(); env.selfDir != "" {
		got = strings.ReplaceAll(got, env.selfDir, "$WARDEN_DIR")
	}
	got = hostNeutral(got)
	path := filepath.Join(goldenDir, name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from its golden:\n--- got\n%s\n--- want\n%s",
			path, got, want)
	}
}

// resetCommandFlags puts every flag of every command back to its default,
// unset, now and when the test ends, so that a command line run through
// rootCmd sees only its own flags.
func resetCommandFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		var walk func(c *cobra.Command)
		walk = func(c *cobra.Command) {
			for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
				fs.VisitAll(func(f *pflag.Flag) {
					_ = f.Value.Set(f.DefValue)
					f.Changed = false
				})
			}
			for _, sub := range c.Commands() {
				walk(sub)
			}
		}
		walk(rootCmd)
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}
	reset()
	t.Cleanup(reset)
}

// runWarden runs one command line through rootCmd, as main does, and returns
// what cobra printed and the error it returned.
func runWarden(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetCommandFlags(t)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetFlagErrorFunc(flagError)
	fixableArgs(rootCmd)
	err := execute(args)
	return out.String(), err
}

// helpLines are the command lines whose help is characterized: every command.
var helpLines = [][]string{
	{"--help"},
	{"build", "--help"},
	{"shell", "--help"},
	{"doctor", "--help"},
	{"setup", "--help"},
	{"clean", "--help"},
	{"image", "--help"},
	{"image", "prepare", "--help"},
	{"image", "list", "--help"},
	{"image", "verify", "--help"},
	{"inspect", "--help"},
	{"hyperv", "--help"},
	{"hyperv", "service", "--help"},
	{"hyperv", "service", "install", "--help"},
	{"hyperv", "service", "uninstall", "--help"},
	{"hyperv", "service", "run", "--help"},
}

func TestCharacterizeHelp(t *testing.T) {
	for _, line := range helpLines {
		name := "help-" + strings.Join(line[:len(line)-1], "-")
		if len(line) == 1 {
			name = "help-root"
		}
		t.Run(name, func(t *testing.T) {
			inConfigFiles(t, "")
			out, err := runWarden(t, line...)
			if err != nil {
				t.Fatalf("%v: %v", line, err)
			}
			golden(t, name, out)
		})
	}
}

var qemuBuild = []string{"build", "--driver", "qemu"}

// refusals are command lines warden refuses before it creates, stages or
// runs anything; files is the build context's content.
var refusals = []struct {
	args  []string
	files map[string]string
	conf  string
}{
	{args: []string{"build", "--cpus", "2"}},
	{args: []string{"build", "--memory", "512"}},
	{args: []string{"build", "--script", "x.sh"}},
	{args: []string{"build", "--image", "x.qcow2"}},
	{args: []string{"build", "--keep-vms"}},
	{args: []string{"build", "--build-link", "10.9.0.0/30"}},
	{args: []string{"build", "--network", "x"}},
	{args: []string{"build", "--driver", "qemu", "--network", "x"}},
	{args: []string{"build", "--driver", "firecracker", "--network", "x"}},
	{args: []string{"build", "--driver", "vz", "--guest-os", "windows"}},
	{args: []string{"build", "--driver", "firecracker", "--guest-os", "windows"}},
	{args: []string{"build", "--guest-os", "plan9"}},
	{args: []string{"build", "--driver", "nomad"}},
	{args: []string{"build", "--timeout", "soon"}},
	{args: []string{"build", "--capture", "everything"}},
	{args: []string{"build", "--driver", "qemu", "--build-link", "10.9.0.0/24"}},
	{args: []string{"build", "--regoin", "eu"}},
	{args: []string{"build", "a", "b"}},
	{args: []string{"build"}, files: map[string]string{}},
	{args: []string{"build", "--driver", "qemu"}, files: map[string]string{}},
	{args: []string{"build", "--driver", "hyperv", "--guest-os", "windows"},
		files: map[string]string{"Dockerfile": "FROM alpine\n"}},
	{args: []string{"build"}, conf: "[runtime]\ndriver = \"nomad\"\n"},
	{args: []string{"build"}, conf: "[runtime]\nrelay_image = \"-x\"\n"},
	{args: qemuBuild, conf: "[vm]\nimage_refresh_after = \"soon\"\n"},
	{args: qemuBuild, conf: "[vm]\nimage_min_grace = \"soon\"\n"},
	{args: qemuBuild, conf: "[vm]\nbuild_link = \"10.9.0.0/24\"\n"},
	// Behaviour 19: a driver's key in another letter case, one for each
	// driver that owns one of the seven keys.
	{args: []string{"build"}, conf: "[Runtime]\nCLI = \"podman\"\n"},
	{args: qemuBuild, conf: "[driver.Qemu]\nimage_refresh_after = \"3d\"\n"},
	{args: []string{"build", "--driver", "hyperv"},
		conf: "[vm]\nnetwork = \"a\"\nNetwork = \"b\"\n"},
	{args: []string{"build"}, conf: "[build]\ncapture = \"everything\"\n"},
	{args: []string{"build"}, conf: "not toml ["},
	{args: []string{"shell", "--cpus", "2"}},
	{args: []string{"image", "prepare", "--driver", "container"}},
	{args: []string{"image", "prepare", "--driver", "qemu", "--os", "macos"}},
	{args: []string{"image", "prepare", "--driver", "qemu", "--update"}},
	{args: []string{"image", "verify", "--driver", "container"}},
	{args: []string{"image", "list", "extra"}},
	{args: []string{"frobnicate"}},
}

func TestCharacterizeRefusals(t *testing.T) {
	var b strings.Builder
	for _, r := range refusals {
		dir := inConfigFiles(t, r.conf)
		files := r.files
		if files == nil {
			files = map[string]string{"Dockerfile": "FROM alpine\n",
				"build.sh": "true\n"}
		}
		for name, body := range files {
			write(t, dir, name, body)
		}
		_, err := runWarden(t, r.args...)
		fmt.Fprintf(&b, "warden %s", strings.Join(r.args, " "))
		if r.conf != "" {
			fmt.Fprintf(&b, "  (warden.toml: %q)", r.conf)
		}
		b.WriteString("\n")
		b.WriteString(describeErr(err, dir))
	}
	golden(t, "refusals", b.String())
}

// describeErr writes an error as its message and its fix, with the build
// context's directory shown as $CTX.
func describeErr(err error, dir string) string {
	if err == nil {
		return "  accepted\n"
	}
	msg, fix := err.Error(), ""
	var fe *driver.FixableError
	if errors.As(err, &fe) {
		msg, fix = fe.Msg, fe.Fix
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  error: %s\n", placeholders(msg, dir))
	if fix != "" {
		fmt.Fprintf(&b, "  fix:   %s\n", placeholders(fix, dir))
	}
	return b.String()
}

// placeholderPath matches a path under a placeholder directory, up to the
// first space, quote or end of line.
var placeholderPath = regexp.MustCompile(`\$(CTX|WARDEN_DIR)\\[^\s"'` + "`" + `]*`)

// driverExe matches a driver binary's Windows file name.
var driverExe = regexp.MustCompile(`(warden-driver-[a-z0-9-]+)\.exe\b`)

// hostNeutral writes what differs on a Windows host the way the goldens
// hold it: a path under $CTX or $WARDEN_DIR with forward slashes, and a
// driver's file name without .exe. Elsewhere it returns s unchanged.
func hostNeutral(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}
	s = placeholderPath.ReplaceAllStringFunc(s, func(p string) string {
		return strings.ReplaceAll(p, `\`, "/")
	})
	return driverExe.ReplaceAllString(s, "$1")
}

// placeholders shows the per-test directory as $CTX, the same on every run.
func placeholders(s, dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		s = strings.ReplaceAll(s, real, "$CTX")
	}
	return strings.ReplaceAll(s, dir, "$CTX")
}

// requestCases are the build configurations whose requests are
// characterized, per driver.
var requestCases = []struct {
	name  string
	args  []string
	conf  string
	files map[string]string
}{
	{name: "defaults"},
	{name: "flags", args: []string{"--capture", "headers", "--output", "out",
		"--no-compress", "--timeout", "10m", "--tmpdir", "tmp", "--quiet"}},
	{name: "vm flags", args: []string{"--image", "base.qcow2", "--cpus", "8",
		"--memory", "16384", "--keep-vms", "--build-link", "10.9.0.0/30",
		"--script", "build.sh"}},
	{name: "config file", conf: "[build]\ncapture = \"bodies\"\noutput_dir = \"o\"\n" +
		"compress = false\ntmpdir = \"t\"\n[relay]\nsystem_ca_bundle = false\n" +
		"[runtime]\nrelay_image = \"dev\"\n" +
		"[vm]\ncpus = 2\nmemory_mb = 2048\nnetwork = \"warden-ci\"\n" +
		"image_refresh_after = \"3d\"\nimage_min_grace = \"6h\"\n" +
		"build_link = \"10.9.0.4/30\"\ndns = [\"10.20.0.53\"]\n"},
	{name: "network flag", args: []string{"--network", "warden-x"}},
	// Behaviour 19: each of the seven keys in another letter case, refused by
	// the driver that owns it and ignored by the others (Behaviour 13).
	{name: "[Runtime] cli", conf: "[Runtime]\ncli = \"podman\"\n"},
	{name: "[runtime] Relay_Image", conf: "[runtime]\nRelay_Image = \"dev\"\n"},
	{name: "[VM] image_refresh_after", conf: "[VM]\nimage_refresh_after = \"3d\"\n"},
	{name: "[vm] Image_Min_Grace", conf: "[vm]\nImage_Min_Grace = \"6h\"\n"},
	{name: "[vm] Network", conf: "[vm]\nNetwork = \"warden-ci\"\n"},
	{name: "[Vm] windows_image", conf: "[Vm]\nwindows_image = \"w.vhdx\"\n"},
	{name: "[vm] WINDOWS_IMAGE_UPDATE",
		conf: "[vm]\nWINDOWS_IMAGE_UPDATE = \"reuse\"\n"},
	{name: "windows guest", args: []string{"--guest-os", "windows"},
		files: map[string]string{"build.ps1": "exit 0\r\n"}},
}

// requestDrivers are the drivers whose requests are characterized.
var requestDrivers = []string{"container", "qemu", "vz", "hyperv", "firecracker"}

func TestCharacterizeRequests(t *testing.T) {
	for _, d := range requestDrivers {
		t.Run(d, func(t *testing.T) {
			var b strings.Builder
			for _, rc := range requestCases {
				dir := inConfigFiles(t, rc.conf)
				files := rc.files
				if files == nil {
					files = map[string]string{"Dockerfile": "FROM alpine\n",
						"build.sh": "true\n"}
				}
				for name, body := range files {
					write(t, dir, name, body)
				}
				args := append([]string{"build", "--driver", d}, rc.args...)
				args = append(args, dir)
				fmt.Fprintf(&b, "== %s: warden %s\n", rc.name,
					placeholders(strings.Join(args, " "), dir))
				if rc.conf != "" {
					fmt.Fprintf(&b, "warden.toml:\n%s", indent(rc.conf))
				}
				req, err := parsedRequest(t, args...)
				if err != nil {
					b.WriteString(describeErr(err, dir))
					continue
				}
				b.WriteString(requestTable(req, dir))
			}
			golden(t, "request-"+d, b.String())
		})
	}
}

// parsedRequest parses a build command line as warden does and returns the
// request a linked driver is handed, without running the build: warden's
// request, with the settings the driver resolves in its probe from the
// line's driver flags and the config file put in it, as the kit does.
func parsedRequest(t *testing.T, args ...string) (*driver.BuildRequest, error) {
	t.Helper()
	resetCommandFlags(t)
	prepareDriverFlags(args)
	c, rest, err := rootCmd.Find(args)
	if err != nil {
		return nil, err
	}
	if err := c.ParseFlags(rest); err != nil {
		return nil, flagError(c, err)
	}
	cfg, err := resolveConfig()
	if err != nil {
		return nil, err
	}
	sd := linkedSel(cfg)
	req, err := newRequest(cfg, sd, c.Flags().Args())
	if err != nil {
		return nil, err
	}
	if sd.linked == nil {
		// The container driver's binary: the request its run builds from
		// start, with its relay image (container.run).
		set, err := drivercmd.Resolve(sd.name(), container.Settings, drivercmd.CmdRun,
			driverFlags.argsFor(drivercmd.CmdRun), absConfigFile(cfg), os.Environ())
		if err != nil {
			return nil, err
		}
		req.RelayImage, _ = set.String("relay-image")
		return req, nil
	}
	set, err := drivercmd.Resolve(sd.name(), sd.linked.Settings, drivercmd.CmdRun,
		driverFlags.argsFor(drivercmd.CmdRun), absConfigFile(cfg), os.Environ())
	if err != nil {
		return nil, err
	}
	settingsToRequest(set, req)
	return req, nil
}

func indent(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("    " + l + "\n")
	}
	return b.String()
}

// requestFields are the fields of driver.BuildRequest that main hands a
// driver, in their order there. A field added later is not shown, so that the
// table stays what main sent.
var requestFields = []string{"Script", "ContextDir", "Image", "GuestOS",
	"Containerfile", "OutputDir", "CaptureMode", "Compress", "Quiet", "Timeout",
	"CPUs", "MemoryMB", "Network", "KeepVMs", "ImageRefreshAfter", "ImageMinGrace",
	"TempDir", "RelayImage", "UpstreamCACerts", "UpstreamSystemCA", "BuildLink",
	"DNS", "Env"}

// requestTable prints a request's fields, one per line, with the context
// directory shown as $CTX.
func requestTable(req *driver.BuildRequest, dir string) string {
	var b strings.Builder
	v := reflect.ValueOf(*req)
	for _, name := range requestFields {
		f := v.FieldByName(name)
		val := fmt.Sprintf("%v", f.Interface())
		if name == "UpstreamCACerts" {
			val = fmt.Sprintf("%d bytes", f.Len())
		}
		fmt.Fprintf(&b, "  %-17s %s\n", name, placeholders(val, dir))
	}
	return b.String()
}

// stageCases are the build contexts whose staged inputs are characterized.
var stageCases = []struct {
	name  string
	args  []string
	files map[string]string
}{
	{name: "Dockerfile", files: map[string]string{
		"Dockerfile": "FROM alpine\nRUN echo hi\n"}},
	{name: "Containerfile", files: map[string]string{
		"Containerfile": "FROM alpine\nRUN echo hi\n"}},
	{name: "build.sh", files: map[string]string{"build.sh": "echo sh\n"}},
	{name: "Dockerfile and build.sh", files: map[string]string{
		"Dockerfile": "FROM alpine\nRUN echo hi\n", "build.sh": "echo sh\n"}},
	{name: "--script and Dockerfile", args: []string{"--script", "own.sh"},
		files: map[string]string{"Dockerfile": "FROM alpine\nRUN echo hi\n",
			"own.sh": "echo own\n"}},
	{name: "windows build.ps1", args: []string{"--guest-os", "windows"},
		files: map[string]string{"build.ps1": "Write-Output ps\r\n",
			"Dockerfile": "FROM alpine\nRUN echo hi\n"}},
}

func TestCharacterizeStagedInputs(t *testing.T) {
	for _, d := range requestDrivers {
		t.Run(d, func(t *testing.T) {
			var b strings.Builder
			for _, sc := range stageCases {
				dir := inConfigFiles(t, "")
				for name, body := range sc.files {
					write(t, dir, name, body)
				}
				out := filepath.Join(t.TempDir(), "out")
				args := append([]string{"build", "--driver", d, "--output", out},
					sc.args...)
				args = append(args, dir)
				fmt.Fprintf(&b, "== %s\n", sc.name)
				req, err := parsedRequest(t, args...)
				if err == nil {
					err = stageForCharacterization(t, d, req)
				}
				if err != nil {
					b.WriteString(describeErr(err, dir))
					continue
				}
				b.WriteString(listTree(t, out))
			}
			golden(t, "staged-"+d, b.String())
		})
	}
}

// stagedName is the directory of the output the staged script is copied to,
// so that the golden shows it.
const stagedName = "staged"

// stageForCharacterization stages a build's script and records its inputs as
// warden does for driver d before the driver starts anything, and copies the
// staged script to <output>/staged/<its name>. On main each driver did this
// itself: the container driver translated the Containerfile and recorded it,
// vz dropped the Containerfile, and the other three called
// driver.StageBuildScript and driver.RecordInputs; the goldens are main's.
func stageForCharacterization(t *testing.T, d string, req *driver.BuildRequest) error {
	t.Helper()
	unstage, err := stageBuild(req, testCaps(d),
		driver.RandAlphaNum(8), nil)
	if err != nil {
		return err
	}
	defer unstage()
	dst := filepath.Join(req.OutputDir, stagedName, filepath.Base(req.BuildScript))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return driver.CopyFile(req.BuildScript, dst)
}

// listTree prints every file under root with its content, in path order.
func listTree(t *testing.T, root string) string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(root, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "  %s:\n%s", f, indent(string(data)))
	}
	return b.String()
}

// The image a qemu or firecracker build resolves when its context has a
// Containerfile: the --image flag, else the Containerfile's FROM resolved to
// a local image, with --script or a build.sh named by path too.
func TestCharacterizeContainerfileImage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a FROM that names a local image is an absolute Unix path")
	}
	var b strings.Builder
	for _, tc := range []struct {
		name, from string
		args       []string
		path       string // the build path, relative to the context; "" is the context
	}{
		{name: "FROM a local image", from: "$CTX/base.qcow2"},
		{name: "FROM a local image, --script", from: "$CTX/base.qcow2",
			args: []string{"--script", "$CTX/own.sh"}},
		{name: "FROM a local image, build.sh by path", from: "$CTX/base.qcow2",
			path: "build.sh"},
		{name: "FROM a local image, --image", from: "$CTX/base.qcow2",
			args: []string{"--image", "$CTX/other.qcow2"}},
		{name: "FROM scratch, --script", from: "scratch",
			args: []string{"--script", "$CTX/own.sh"}},
		{name: "FROM an unresolvable reference, --script",
			from: "registry.invalid/none:1",
			args: []string{"--script", "$CTX/own.sh"}},
		{name: "FROM a missing local image", from: "$CTX/missing.qcow2"},
	} {
		dir := inConfigFiles(t, "")
		sub := func(s string) string { return strings.ReplaceAll(s, "$CTX", dir) }
		write(t, dir, "Dockerfile", "FROM "+sub(tc.from)+"\nRUN true\n")
		write(t, dir, "build.sh", "true\n")
		write(t, dir, "own.sh", "true\n")
		write(t, dir, "base.qcow2", "")
		write(t, dir, "other.qcow2", "")
		path := dir
		if tc.path != "" {
			path = filepath.Join(dir, tc.path)
		}
		var args []string
		for _, a := range tc.args {
			args = append(args, sub(a))
		}
		for _, d := range []string{"qemu", "firecracker"} {
			line := append(append([]string{"build", "--driver", d}, args...), path)
			fmt.Fprintf(&b, "%s (%s): ", tc.name, d)
			req, err := parsedRequest(t, line...)
			if err != nil {
				b.WriteString(strings.TrimSpace(describeErr(err, dir)) + "\n")
				continue
			}
			unstage, err := stageBuild(req, testCaps(d),
				driver.RandAlphaNum(8), nil)
			if err != nil {
				t.Fatal(err)
			}
			unstage()
			image := linuxguest.RequestImage(req)
			fmt.Fprintf(&b, "%q\n", placeholders(image, dir))
		}
	}
	golden(t, "containerfile-image", b.String())
}

// inCache points warden's cache at a new directory and returns warden's
// cache root in it.
func inCache(t *testing.T) string {
	t.Helper()
	inConfigFiles(t, "")
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache) // Linux; macOS reads HOME, which is the test's
	t.Setenv("LocalAppData", cache)   // Windows
	return driver.CacheDir()
}

// cacheFile writes a file under warden's cache root.
func cacheFile(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The output of image list and image verify over a cache with images and
// layers of qemu and firecracker. The cache's place and the drivers listed
// differ by platform; Linux's are characterized.
func TestCharacterizeImageCommands(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the cache's place and the drivers listed are Linux's")
	}
	var b strings.Builder
	show := func(title string, args ...string) {
		out := captureStdout(t, func() {
			_, err := runWarden(t, args...)
			if err != nil {
				fmt.Fprintf(os.Stdout, "error: %v\n", err)
			}
		})
		fmt.Fprintf(&b, "== %s: warden %s\n%s", title, strings.Join(args, " "), out)
	}
	inCache(t)
	show("empty cache", "image", "list")
	show("empty cache", "image", "verify")

	root := inCache(t)
	linux := cacheFile(t, root, "images/alpine-3.21.qcow2", "linux image")
	cacheFile(t, root, "images/windows-server-2025.qcow2", "windows image")
	fc := cacheFile(t, root, "firecracker/images/alpine.raw", "fc image")
	for _, c := range []struct {
		lc   *driver.LayerCache
		path string
	}{{qemu.LayerCache(), linux}, {firecracker.LayerCache(), fc}} {
		sum, err := driver.HashFileSHA256(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.lc.Record(c.path, sum, "",
			ledger.LayerSource{Kind: ledger.SourceLocal}); err != nil {
			t.Fatal(err)
		}
	}
	var listing strings.Builder
	show("prepared images", "image", "list")
	show("layers unchanged", "image", "verify")
	show("one driver", "image", "verify", "--driver", "qemu")
	if err := os.WriteFile(linux, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fc); err != nil {
		t.Fatal(err)
	}
	show("one layer changed, one missing", "image", "verify")
	listing.WriteString(strings.ReplaceAll(b.String(), root, "$CACHE"))
	golden(t, "image-commands", listing.String())
}

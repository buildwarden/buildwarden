package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/relaycfg"
)

type Config struct {
	Runtime RuntimeConfig `toml:"runtime"`
	Relay   RelayConfig   `toml:"relay"`
	Build   BuildCfg      `toml:"build"`
	VM      VMConfig      `toml:"vm"`
	Output  OutputConfig  `toml:"output"`
	// File is the absolute path of the one config file that was read, empty
	// when none was, and FileSource how it was chosen (configFromFlag,
	// configFromEnv or configFromDir). Every message that names the file by
	// a path prints File through driver.EscapeOutside: the path can be the
	// working directory's, which a repository or an archive names.
	File       string `toml:"-"`
	FileSource string `toml:"-"`
}

// VMConfig holds warden's keys in [vm]: the defaults for the VM flags
// (--cpus, --memory; a flag always wins), the build link and the resolvers.
// The table's other keys are old names of the drivers' keys, which the
// drivers read (v1.md, Old names).
type VMConfig struct {
	CPUs     int `toml:"cpus"`
	MemoryMB int `toml:"memory_mb"`
	// BuildLink is the relay-to-build link (an advanced option; empty means
	// buildlink.DefaultCIDR), checked as the file loads. BuildLinkSource is
	// the base name of the config file when it set the key, the label
	// warden doctor prints.
	BuildLink       string `toml:"build_link"`
	BuildLinkSource string `toml:"-"`
	// DNS is [vm] dns, the relay's upstream resolvers for every VM driver, in
	// order and used exclusively; unset means the host's. It is checked as
	// the file loads into DNSAddrs, and DNSSource is the config file's base
	// name when it set the key.
	DNS       []string     `toml:"dns"`
	DNSAddrs  []netip.Addr `toml:"-"`
	DNSSource string       `toml:"-"`
}

type RelayConfig struct {
	UpstreamCACerts []string `toml:"upstream_ca_certs"`
	SystemCABundle  *bool    `toml:"system_ca_bundle"`
}

// RuntimeConfig is warden's key in [runtime]: the driver. The table's other
// keys are the container driver's old names, which it reads (v1.md, Old
// names).
type RuntimeConfig struct {
	// Driver selects the driver, by name or, from the flag or the variable,
	// by path; DriverSource says which set it: sourceFlag, sourceEnv, or
	// empty for the config file.
	Driver       string `toml:"driver"`
	DriverSource string `toml:"-"`
}

// projectConfig is the config file read when none is named: the one of this
// name in the working directory, when it exists.
const projectConfig = "warden.toml"

// How the config file was chosen, as Config.FileSource holds it and as the
// config: line and the errors print it.
const (
	configFromFlag = "--config"
	configFromEnv  = "WARDEN_CONFIG"
	configFromDir  = "found in the working directory"
)

type BuildCfg struct {
	Dockerfile string `toml:"dockerfile"`
	Context    string `toml:"context"`
	Capture    string `toml:"capture"`
	OutputDir  string `toml:"output_dir"`
	Compress   *bool  `toml:"compress"`
	// TempDir is where each build's private work dir is created; --tmpdir
	// wins, and empty means the OS temp dir.
	TempDir string `toml:"tmpdir"`
	// TeardownTimeout is [build] teardown_timeout, how long a driver binary
	// is given to answer stop and then to reap; zero is not set. It is
	// decoded apart from the typed fields (teardownTimeout).
	TeardownTimeout time.Duration `toml:"-"`
}

// The teardown wait's default and bounds ([build] teardown_timeout).
const (
	defaultTeardown = 60 * time.Second
	minTeardown     = time.Second
	maxTeardown     = time.Hour
)

// teardown is the teardown wait: [build] teardown_timeout, else 60 s.
func (b BuildCfg) teardown() time.Duration {
	if b.TeardownTimeout == 0 {
		return defaultTeardown
	}
	return b.TeardownTimeout
}

// teardownTimeout reads [build] teardown_timeout from the file's bytes, apart
// from Config's typed fields, so that a value of the wrong type gets the
// key's own message. It is refused as the file loads (UX-08), and the
// default is not used in its place (GLOBAL-10).
func teardownTimeout(path string, data []byte) (time.Duration, error) {
	var f struct {
		Build struct {
			TeardownTimeout any `toml:"teardown_timeout"`
		} `toml:"build"`
	}
	if _, err := toml.Decode(string(data), &f); err != nil {
		return 0, nil // the typed decode reports the syntax
	}
	v := f.Build.TeardownTimeout
	if v == nil {
		return 0, nil
	}
	shown := driver.EscapeOutside(path)
	fix := "set it to a duration from 1s to 1h, such as \"90s\", or remove it to " +
		"wait the default 60s"
	s, ok := v.(string)
	if !ok {
		return 0, driver.Fixable(fix, "[build] teardown_timeout in %s: %v is not a "+
			"duration", shown, v)
	}
	d, err := time.ParseDuration(s)
	switch {
	case err != nil:
		return 0, driver.Fixable(fix, "[build] teardown_timeout in %s: %q is not a "+
			"duration", shown, s)
	case d < minTeardown:
		return 0, driver.Fixable(fix, "[build] teardown_timeout in %s: %q is below the "+
			"minimum of 1s", shown, s)
	case d > maxTeardown:
		return 0, driver.Fixable(fix, "[build] teardown_timeout in %s: %q is above the "+
			"maximum of 1h", shown, s)
	}
	return d, nil
}

type OutputConfig struct {
	Color   string `toml:"color"`
	Verbose bool   `toml:"verbose"`
}

// configFile chooses the one config file a command reads and says how it was
// chosen: the file --config names, else the one WARDEN_CONFIG names, else
// warden.toml in the working directory when it exists, else none (an empty
// path). The path is absolute. A source that a higher one replaces is not
// looked at, so a named file is the only one that is ever opened. Nothing
// here falls through on a failure: an empty --config is an error because the
// next source down can be the warden.toml of a repository the caller meant
// not to use, and a working directory's warden.toml that cannot be examined
// is an error, not "no file".
func configFile() (path, source string, err error) {
	return configFileFrom(rootFlagChanged("config"), flagConfig)
}

// configFileFrom is configFile with --config as given: given says whether it
// was, and value is its path.
func configFileFrom(given bool, value string) (path, source string, err error) {
	if given {
		if value == "" {
			return "", "", driver.Fixable("pass --config the path of a config "+
				"file, or drop the flag to use WARDEN_CONFIG or ./warden.toml",
				"--config was given an empty path")
		}
		path, err = filepath.Abs(value)
		return path, configFromFlag, err
	}
	if named := os.Getenv(configFromEnv); named != "" {
		path, err = filepath.Abs(named)
		return path, configFromEnv, err
	}
	if path, err = filepath.Abs(projectConfig); err != nil {
		return "", "", err
	}
	switch _, err := os.Stat(path); {
	case err == nil:
		return path, configFromDir, nil
	case errors.Is(err, fs.ErrNotExist):
		return "", "", nil
	default:
		return "", "", unreadableConfig(path, configFromDir, err)
	}
}

// readConfigFile returns the bytes of the config file configFile chose, or
// the error that says why it cannot be read. No other file is tried in its
// place. The working directory's file must be a regular file once links are
// followed: nobody named it, and a repository can make it a link to a device
// (/dev/zero, a terminal) that would exhaust memory or wait for ever. For the
// same reason it is read up to maxProjectConfig and no further, counted as
// read: a link to a procfs file such as /proc/self/pagemap passes as an
// empty regular file and gives far more. A file named with --config or
// WARDEN_CONFIG is read whole, whatever kind it is.
func readConfigFile(path, source string) ([]byte, error) {
	info, err := os.Stat(path)
	switch {
	case err != nil:
	case info.IsDir():
		err = errIsDirectory
	case source == configFromDir && !info.Mode().IsRegular():
		err = errNotRegular
	}
	var data []byte
	switch {
	case err != nil:
	case source == configFromDir:
		data, err = readProjectConfig(path)
	default:
		data, err = os.ReadFile(path)
	}
	if err == nil {
		return data, nil
	}
	if errors.Is(err, errTooLarge) {
		return nil, driver.Fixable("make it 1 MiB or smaller, or name it with "+
			"--config or WARDEN_CONFIG, which reads it whatever its size",
			"cannot read config file %s (%s): %v", driver.EscapeOutside(path),
			source, err)
	}
	if source == configFromDir {
		return nil, unreadableConfig(path, source, err)
	}
	shown := driver.EscapeOutside(path)
	fix := "pass --config an existing file, or drop it to use WARDEN_CONFIG " +
		"or ./warden.toml"
	if source == configFromEnv {
		fix = "set WARDEN_CONFIG to an existing file, or unset it to use " +
			"./warden.toml"
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, driver.Fixable(fix, "config file %s (from %s) does not exist",
			shown, source)
	case errors.Is(err, errIsDirectory):
		return nil, driver.Fixable(fix, "config file %s (from %s) is a directory",
			shown, source)
	}
	return nil, unreadableConfig(path, source, err)
}

// maxProjectConfig is how many bytes of the working directory's warden.toml
// are read: 1 MiB, far past any real config file.
const maxProjectConfig = 1 << 20

// configBlock is how much each read of the working directory's warden.toml
// asks for: 64 KiB, a multiple of 8, because /proc/self/pagemap refuses a
// read of any other length.
const configBlock = 1 << 16

// errTooLarge marks a working directory's warden.toml that gives more than
// maxProjectConfig bytes, 1 MiB, when read.
var errTooLarge = errors.New("larger than 1 MiB")

// readProjectConfig reads the working directory's warden.toml through
// readCapped.
func readProjectConfig(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readCapped(f)
}

// readCapped reads r to its end, or errTooLarge once it has given more than
// maxProjectConfig bytes. Every read asks for a whole configBlock, so it reads
// at most maxProjectConfig+configBlock bytes, and the count is of the bytes
// read: a procfs file that stat reports as empty, or a reader that never
// ends, costs no more than that. A read error before the cap is returned as
// it is.
func readCapped(r io.Reader) ([]byte, error) {
	var data []byte
	block := make([]byte, configBlock)
	for {
		n, err := r.Read(block)
		data = append(data, block[:n]...)
		switch {
		case len(data) > maxProjectConfig:
			return nil, errTooLarge
		case errors.Is(err, io.EOF):
			return data, nil
		case err != nil:
			return nil, err
		}
	}
}

// errIsDirectory marks a config file path that names a directory.
var errIsDirectory = errors.New("is a directory")

// errNotRegular marks a working directory's warden.toml that is a device, a
// pipe or a socket, or a link to one.
var errNotRegular = errors.New("not a regular file")

// unreadableConfig is the error for a config file that exists and cannot be
// read; the reason is the operating system's. For a named file its source
// follows "from", as in the other errors that name one.
func unreadableConfig(path, source string, err error) error {
	if source != configFromDir {
		source = "from " + source
	}
	return driver.Fixable("make it readable, or name another file with "+
		"--config or WARDEN_CONFIG",
		"cannot read config file %s (%s): %s", driver.EscapeOutside(path),
		source, osReason(err))
}

// osReason is the operating system's reason for a failed file operation,
// without the operation and the path that *fs.PathError puts before it,
// escaped for printing.
func osReason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return driver.EscapeOutside(err.Error())
}

// configLine says which config file a command read and how it was chosen.
// Every command that reads configuration prints it, so that the working
// directory's file is never used without a word. The path is escaped: a
// directory whose name holds a newline or an escape sequence would
// otherwise print a second line, or erase this one, in a terminal.
func configLine(cfg *Config) string {
	if cfg.File == "" {
		return "config: none (no --config, no WARDEN_CONFIG, no " +
			"./warden.toml): defaults apply"
	}
	return "config: " + driver.EscapeOutside(cfg.File) + " (" + cfg.FileSource + ")"
}

// LoadConfig reads the one config file (configFile) and applies the
// environment variables over it. It reads no other file and merges none.
// WARDEN_UPSTREAM_CA_CERTS is not applied here: it replaces [relay]
// upstream_ca_certs, and resolveUpstreamCAs reads both.
func LoadConfig() (*Config, error) {
	path, source, err := configFile()
	if err != nil {
		return nil, err
	}
	return loadConfigFile(path, source)
}

// loadConfigFile is LoadConfig for the file configFile chose: path, empty for
// none, and how it was chosen.
func loadConfigFile(path, source string) (*Config, error) {
	cfg := &Config{
		Output: OutputConfig{Color: "auto"},
	}
	var data []byte
	var err error
	if path != "" {
		if data, err = readConfigFile(path, source); err != nil {
			return nil, err
		}
	}
	// TRANSITIONAL(one-config-file)
	if err := formerUserConfig(path); err != nil {
		return nil, err
	}
	if path != "" {
		if err := decodeConfig(path, data, cfg); err != nil {
			return nil, err
		}
		cfg.File, cfg.FileSource = path, source
	}

	// Environment variable overrides. WARDEN_CTR_CLI and WARDEN_IMAGE are the
	// container driver's, which it reads (DRV-C9).
	if d := os.Getenv("WARDEN_DRIVER"); d != "" {
		cfg.Runtime.Driver, cfg.Runtime.DriverSource = d, sourceEnv
	}
	if os.Getenv("NO_COLOR") != "" {
		cfg.Output.Color = "never"
	}
	if os.Getenv("WARDEN_VERBOSE") != "" {
		cfg.Output.Verbose = true
	}

	return cfg, nil
}

// decodeConfig decodes the config file's bytes into cfg and makes the checks
// that are made as the file loads ([vm] build_link, [vm] dns). Its messages
// and the source labels it sets print the path escaped.
func decodeConfig(file string, data []byte, cfg *Config) error {
	path := driver.EscapeOutside(file)
	md, err := toml.Decode(string(data), cfg)
	if err != nil {
		return driver.Fixable("fix the TOML syntax in "+path, "parsing %s: %v", path, err)
	}
	if cfg.Build.TeardownTimeout, err = teardownTimeout(file, data); err != nil {
		return err
	}
	if md.IsDefined("vm", "build_link") {
		if _, err := checkBuildLink(cfg.VM.BuildLink); err != nil {
			return driver.Fixable(
				fmt.Sprintf("set [vm] build_link in %s to a valid /30, e.g. "+
					"build_link = %q, or remove it to use the default",
					path, exampleBuildLink),
				"[vm] build_link in %s: %v", path, err)
		}
		cfg.VM.BuildLinkSource = driver.EscapeOutside(filepath.Base(file))
	}
	if md.IsDefined("vm", "dns") {
		if len(cfg.VM.DNS) == 0 {
			return driver.Fixable("remove [vm] dns from "+path+" to use the host's "+
				"resolvers", "[vm] dns in %s is empty", path)
		}
		// The claim's own check of upstream_dns. The build link and each
		// driver's reachability rule wait for the driver, at build start and
		// in doctor: a loopback resolver is usable by vz's host-side relay.
		addrs, err := relaycfg.ParseUpstreamDNS(cfg.VM.DNS, nil)
		if err != nil {
			return driver.Fixable(
				fmt.Sprintf("set [vm] dns in %s to IPv4 addresses of DNS "+
					"servers, e.g. dns = [%q], or remove [vm] dns to use the "+
					"host's resolvers", path, exampleDNS),
				"[vm] dns in %s: %v", path, err)
		}
		cfg.VM.DNSAddrs = addrs
		cfg.VM.DNSSource = driver.EscapeOutside(filepath.Base(file))
	}
	return nil
}

// exampleDNS is the resolver address [vm] dns fixes suggest.
const exampleDNS = "10.20.0.53"

// exampleBuildLink is the valid non-default value fixes suggest.
const exampleBuildLink = "100.126.255.252/30"

// linkRanges are the fixed ranges BuildWarden itself uses, read from the
// packages that own them; a build link must be disjoint from all of them. The
// Hyper-V NAT is not one: it is read back from the host, and the hyperv
// driver checks the link against it at build start and in doctor.
func linkRanges() []buildlink.Range {
	return []buildlink.Range{
		{Name: "the container driver's network", CIDR: driver.ContainerBaseCIDR},
		{Name: "the firecracker driver's upstream range", CIDR: firecracker.UpstreamCIDR},
		{Name: "qemu's user-mode network", CIDR: qemu.UserNetCIDR},
	}
}

// checkBuildLink parses a configured build link and checks it against
// BuildWarden's own ranges.
func checkBuildLink(s string) (buildlink.Link, error) {
	link, err := buildlink.Parse(s)
	if err != nil {
		return buildlink.Link{}, err
	}
	if err := link.Disjoint(linkRanges()); err != nil {
		return buildlink.Link{}, err
	}
	return link, nil
}

// ResolvePath handles the single-argument path logic:
//   - If path is a file: use it as Dockerfile, parent dir as context
//   - If path is a directory: use it as context, discover Dockerfile inside
//   - If path is empty: use CWD as context, discover Dockerfile
func ResolvePath(path string) (dockerfile, contextDir string, err error) {
	if path == "" {
		path = "."
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		return "", "", driver.Fixable("", "cannot access %q: %v", path, statErr)
	}

	if !info.IsDir() {
		// Path is a file — treat as Dockerfile, parent is context
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", "", err
		}
		return abs, filepath.Dir(abs), nil
	}

	// Path is a directory — discover Dockerfile inside it
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}

	df, err := discoverDockerfile(abs)
	if err != nil {
		return "", "", err
	}
	return df, abs, nil
}

func discoverDockerfile(dir string) (string, error) {
	for _, name := range []string{"Dockerfile", "Containerfile"} {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", driver.Fixable("pass the Dockerfile's path, or the directory that holds it",
		"no Dockerfile or Containerfile found in %s", dir)
}

// resolveBuildInput resolves the build context and (optional) Dockerfile for
// any driver. A driver without capabilities.vm always builds a Dockerfile. VM drivers
// build whichever the context provides, with the precedence the drivers apply
// (driver.StageBuildScript): --script, then a Dockerfile (Linux guests), then
// build.sh / build.ps1 in the context. A path may name the context directory
// or a file in it (a Dockerfile, or a build script).
func resolveBuildInput(
	path string, isVM bool, guestOS, script string,
) (dockerfile, contextDir, buildScript string, err error) {
	if !isVM {
		dockerfile, contextDir, err = ResolvePath(path)
		return dockerfile, contextDir, "", err
	}
	return resolveVMBuildInput(path, guestOS, script)
}

// resolveVMBuildInput resolves the context dir and either a Dockerfile or a
// build script for a VM driver, applying driver.StageBuildScript's precedence:
// --script, then a Dockerfile (Linux guests), then build.sh / build.ps1.
func resolveVMBuildInput(
	path, guestOS, script string,
) (dockerfile, contextDir, buildScript string, err error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", "", driver.Fixable("", "cannot access %q: %v", path, err)
	}
	contextDir = abs
	if !info.IsDir() {
		contextDir = filepath.Dir(abs)
		if isScriptName(abs) && script == "" {
			script = abs
		}
	}

	windows := guestOS == "windows"
	if !windows {
		dockerfile = vmDockerfile(abs, contextDir, info.IsDir())
	}
	if script != "" || dockerfile != "" {
		return dockerfile, contextDir, script, nil
	}
	if err := requireBuildFile(contextDir, windows); err != nil {
		return "", "", "", err
	}
	return "", contextDir, "", nil
}

// vmDockerfile returns the Dockerfile to build for a Linux VM guest: the path
// itself when it names one, else one discovered in the context dir, or "" when
// there is none.
func vmDockerfile(abs, contextDir string, isDir bool) string {
	if !isDir && !isScriptName(abs) {
		return abs
	}
	if df, err := discoverDockerfile(contextDir); err == nil {
		return df
	}
	return ""
}

// requireBuildFile confirms the context holds the build file a VM guest needs
// when neither a --script nor a Dockerfile was resolved.
func requireBuildFile(contextDir string, windows bool) error {
	want := "build.sh"
	if windows {
		want = "build.ps1"
	}
	if _, err := os.Stat(filepath.Join(contextDir, want)); err != nil {
		hint := " or a Dockerfile"
		if windows {
			hint = ""
		}
		return driver.Fixable("",
			"nothing to build in %s: pass --script, or add %s%s",
			contextDir, want, hint)
	}
	return nil
}

func isScriptName(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".sh", ".ps1":
		return true
	}
	return false
}

func validateCapture(c string) error {
	switch c {
	case "", "none", "headers", "bodies", "all":
		return nil
	default:
		return driver.Fixable("",
			"invalid capture mode %q (valid: none, headers, bodies, all)", c)
	}
}

// isVMDriver reports whether d is a VM driver, which takes the VM flags.
func isVMDriver(d string) bool {
	switch d {
	case "qemu", "vz", "hyperv", "firecracker":
		return true
	}
	return false
}

func validateFlagsForDriver(isVM bool) error {
	for _, f := range []struct {
		set  bool
		name string
	}{
		{flagScript != "", "--script"},
		{flagImage != "", "--image"},
		{flagCPUs != 0, "--cpus"},
		{flagMemoryMB != 0, "--memory"},
		{flagKeepVMs, "--keep-vms"},
		{flagBuildLink != "", "--build-link"},
	} {
		if f.set && !isVM {
			return driver.Fixable("",
				"%s requires a VM driver (--driver qemu, vz, hyperv or "+
					"firecracker)", f.name)
		}
	}
	return nil
}

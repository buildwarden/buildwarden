package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"os/exec"
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
	// a path prints File.
	File       string `toml:"-"`
	FileSource string `toml:"-"`
}

// VMConfig holds defaults for the VM-driver flags (--cpus, --memory,
// --network); a flag always wins.
type VMConfig struct {
	CPUs     int    `toml:"cpus"`
	MemoryMB int    `toml:"memory_mb"`
	Network  string `toml:"network"`
	// ImageRefreshAfter / ImageMinGrace: ages such as "7d" or "36h".
	ImageRefreshAfter string `toml:"image_refresh_after"`
	ImageMinGrace     string `toml:"image_min_grace"`
	// WindowsImage is the hyperv base image (a URL or local path) used when
	// image prepare is given no --source; empty means Microsoft's current
	// evaluation download. WindowsImageUpdate is "auto" (default: a refresh
	// picks up new media at that URL) or "reuse" (keep the current base).
	WindowsImage       string `toml:"windows_image"`
	WindowsImageUpdate string `toml:"windows_image_update"`
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

type RuntimeConfig struct {
	CLI string `toml:"cli"`
	// CLISource says where CLI was named: cliFromFlag, cliFromEnv, or the
	// path of the config file that set it. It words the error that refuses
	// an unsupported runtime; nothing that decides what is run reads it.
	CLISource  string `toml:"-"`
	Driver     string `toml:"driver"`
	RelayImage string `toml:"relay_image"`
}

// What RuntimeConfig.CLISource holds for a runtime named outside a config
// file.
const (
	cliFromFlag = "--runtime"
	cliFromEnv  = "WARDEN_CTR_CLI"
)

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
	if f := rootCmd.PersistentFlags().Lookup("config"); f != nil && f.Changed {
		if flagConfig == "" {
			return "", "", driver.Fixable("pass --config the path of a config "+
				"file, or drop the flag to use WARDEN_CONFIG or ./warden.toml",
				"--config was given an empty path")
		}
		path, err = filepath.Abs(flagConfig)
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
// place.
func readConfigFile(path, source string) ([]byte, error) {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		err = errIsDirectory
	}
	var data []byte
	if err == nil {
		data, err = os.ReadFile(path)
	}
	if err == nil {
		return data, nil
	}
	if source == configFromDir {
		return nil, unreadableConfig(path, source, err)
	}
	fix := "pass --config an existing file, or drop it to use WARDEN_CONFIG " +
		"or ./warden.toml"
	if source == configFromEnv {
		fix = "set WARDEN_CONFIG to an existing file, or unset it to use " +
			"./warden.toml"
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, driver.Fixable(fix, "config file %s (from %s) does not exist",
			path, source)
	case errors.Is(err, errIsDirectory):
		return nil, driver.Fixable(fix, "config file %s (from %s) is a directory",
			path, source)
	}
	return nil, unreadableConfig(path, source, err)
}

// errIsDirectory marks a config file path that names a directory.
var errIsDirectory = errors.New("is a directory")

// unreadableConfig is the error for a config file that exists and cannot be
// read; the reason is the operating system's. For a named file its source
// follows "from", as in the other errors that name one.
func unreadableConfig(path, source string, err error) error {
	if source != configFromDir {
		source = "from " + source
	}
	return driver.Fixable("make it readable, or name another file with "+
		"--config or WARDEN_CONFIG",
		"cannot read config file %s (%s): %s", path, source, osReason(err))
}

// osReason is the operating system's reason for a failed file operation,
// without the operation and the path that *fs.PathError puts before it.
func osReason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// configLine says which config file a command read and how it was chosen.
// Every command that reads configuration prints it, so that the working
// directory's file is never used without a word.
func configLine(cfg *Config) string {
	if cfg.File == "" {
		return "config: none (no --config, no WARDEN_CONFIG, no " +
			"./warden.toml): defaults apply"
	}
	return "config: " + cfg.File + " (" + cfg.FileSource + ")"
}

// LoadConfig reads the one config file (configFile) and applies the
// environment variables over it. It reads no other file and merges none.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Output: OutputConfig{Color: "auto"},
	}

	path, source, err := configFile()
	if err != nil {
		return nil, err
	}
	var data []byte
	if path != "" {
		if data, err = readConfigFile(path, source); err != nil {
			return nil, err
		}
	}
	if path != "" {
		if err := decodeConfig(path, data, cfg); err != nil {
			return nil, err
		}
		cfg.File, cfg.FileSource = path, source
		cfg.Runtime.CLISource = path
	}

	// Environment variable overrides
	if cli := os.Getenv(cliFromEnv); cli != "" {
		cfg.Runtime.CLI, cfg.Runtime.CLISource = cli, cliFromEnv
	}
	if d := os.Getenv("WARDEN_DRIVER"); d != "" {
		cfg.Runtime.Driver = d
	}
	if img := os.Getenv("WARDEN_IMAGE"); img != "" {
		cfg.Runtime.RelayImage = img
	}
	if caCerts := os.Getenv("WARDEN_UPSTREAM_CA_CERTS"); caCerts != "" {
		paths := strings.Split(caCerts, ":")
		cfg.Relay.UpstreamCACerts = append(cfg.Relay.UpstreamCACerts, paths...)
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
// that are made as the file loads ([vm] build_link, [vm] dns).
func decodeConfig(path string, data []byte, cfg *Config) error {
	md, err := toml.Decode(string(data), cfg)
	if err != nil {
		return driver.Fixable("fix the TOML syntax in "+path, "parsing %s: %v", path, err)
	}
	if md.IsDefined("vm", "build_link") {
		if _, err := checkBuildLink(cfg.VM.BuildLink); err != nil {
			return driver.Fixable(
				fmt.Sprintf("set [vm] build_link in %s to a valid /30, e.g. "+
					"build_link = %q, or remove it to use the default",
					path, exampleBuildLink),
				"[vm] build_link in %s: %v", path, err)
		}
		cfg.VM.BuildLinkSource = filepath.Base(path)
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
		cfg.VM.DNSSource = filepath.Base(path)
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

// containerRuntimes are the runtimes the container driver supports, in the
// order detection prefers them. A runtime named by a flag, a variable or a
// config file is one of these or is refused (resolveRuntime).
var containerRuntimes = []string{"finch", "docker", "podman"}

// supportedRuntime returns the member of containerRuntimes that v equals.
// The comparison is exact, and what is returned is the list's own string,
// never v: no text from a flag, a variable or a config file reaches exec.
func supportedRuntime(v string) (string, bool) {
	for _, name := range containerRuntimes {
		if v == name {
			return name, true
		}
	}
	return "", false
}

// runtimeNames lists containerRuntimes in prose, the last after conj:
// "finch, docker or podman".
func runtimeNames(conj string) string {
	last := len(containerRuntimes) - 1
	return strings.Join(containerRuntimes[:last], ", ") +
		" " + conj + " " + containerRuntimes[last]
}

// unsupportedRuntime is the error for a runtime in effect that is not one of
// containerRuntimes. It names the level that set the value and gives the fix
// for that level. The value is printed once, quoted: it may come from a file
// someone else wrote, and %q keeps control characters out of the terminal.
// Nothing else in the message or the fix is taken from the value.
func unsupportedRuntime(rt RuntimeConfig) error {
	var setting, set, orNot string
	switch rt.CLISource {
	case cliFromFlag:
		setting, set, orNot = cliFromFlag, "pass --runtime ", ", or leave it out"
	case cliFromEnv:
		setting, set, orNot = cliFromEnv, "set WARDEN_CTR_CLI to ", ", or unset it"
	default:
		setting, set = "[runtime] cli in "+rt.CLISource, "set it to "
		orNot = ", or remove it; --runtime with one of those replaces it " +
			"for one command"
	}
	what := "the container driver supports " + runtimeNames("and")
	fix := set + runtimeNames("or") + orNot
	// TRANSITIONAL(runtime-former-values): a value that ran before this
	// check existed gets a message that says what to change.
	if formerWhat, formerFix, ok := formerRuntimeValue(rt.CLI); ok {
		what, fix = formerWhat, set+formerFix
	}
	return driver.Fixable(fix, "%s is %q; %s", setting, rt.CLI, what)
}

// DetectRuntime probes for a working container runtime, in
// containerRuntimes' order.
func DetectRuntime() (string, error) {
	var installed []string
	for _, name := range containerRuntimes {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		installed = append(installed, name)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := exec.CommandContext(ctx, name, "info").Run()
		cancel()
		if err == nil {
			return name, nil
		}
	}
	if len(installed) == 0 {
		return "", driver.Fixable(
			"install Finch (brew install --cask finch && finch vm init), "+
				"Docker or Podman, or use a VM driver (--driver qemu or vz)",
			"no container runtime found (tried finch, docker, podman)")
	}
	// Installed but not answering: almost always its VM or daemon is down.
	return "", driver.Fixable(runtimeStartHint(installed[0]),
		"container runtime %s is installed but not running (`%s info` failed)",
		strings.Join(installed, ", "), installed[0])
}

// runtimeStartHint is the command that starts a stopped runtime.
func runtimeStartHint(name string) string {
	switch name {
	case "finch":
		return "start its VM: finch vm start (or finch vm init the first time)"
	case "podman":
		return "start its machine: podman machine start " +
			"(or podman machine init the first time)"
	default:
		return "start Docker (Docker Desktop, or the docker service)"
	}
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
// any driver. The container driver always builds a Dockerfile. VM drivers
// build whichever the context provides, with the precedence the drivers apply
// (driver.StageBuildScript): --script, then a Dockerfile (Linux guests), then
// build.sh / build.ps1 in the context. A path may name the context directory
// or a file in it (a Dockerfile, or a build script).
func resolveBuildInput(
	path, driverName, guestOS, script string,
) (dockerfile, contextDir, buildScript string, err error) {
	if driverName == "" || driverName == "container" {
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

func validateDriver(d string) error {
	switch d {
	case "", "container", "qemu", "vz", "hyperv", "firecracker":
		return nil
	default:
		return driver.Fixable("",
			"unknown driver %q (valid: container, qemu, vz, hyperv, firecracker)", d)
	}
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

func validateFlagsForDriver(d string) error {
	isVM := isVMDriver(d)
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
	if flagNetwork != "" && d != "hyperv" {
		fix := ""
		if d == "firecracker" {
			fix = "drop --network: firecracker builds use the slots " +
				"'warden setup --driver firecracker' made"
		}
		return driver.Fixable(fix, "--network is only used by --driver hyperv")
	}
	return nil
}

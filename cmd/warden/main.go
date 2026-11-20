package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

var version = "dev"

var (
	flagConfig  string
	flagDriver  string
	flagVerbose bool
	flagColor   string

	// Build/shell flags: one set, accepted by every driver; each driver
	// rejects only what it genuinely cannot use (validateFlagsForDriver).
	flagCapture    string
	flagOutput     string
	flagNoCompress bool
	flagScript     string
	flagImage      string
	flagGuestOS    string
	flagTimeout    string
	flagQuiet      bool
	flagCPUs       int
	flagMemoryMB   int
	flagKeepVMs    bool
	flagTempDir    string
	flagBuildLink  string
)

var rootCmd = &cobra.Command{
	Use:           "warden",
	Version:       version,
	Short:         "Build software with a verifiable network ledger",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var buildCmd = &cobra.Command{
	Use:   "build [path]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Run a build with network auditing",
	Long: `Run a build with full network auditing, on any driver.

The path argument can be:
  - A directory (default: current directory) holding a Dockerfile, or for
    VM drivers a build.sh / build.ps1
  - A path to a specific Dockerfile or build script (context = its directory)

All network traffic during the build is recorded to a cryptographically
signed ledger. The output directory is printed on completion.`,
	Example: `  warden build
  warden build ./my-project
  warden build ./my-project/Dockerfile.prod
  warden build --driver qemu --guest-os windows --cpus 8 --memory 16384 ./win-project`,
	RunE: runBuild,
}

var shellCmd = &cobra.Command{
	Use:   "shell [path]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Open an interactive shell in the build environment",
	Long: `Open an interactive shell in a network-audited build environment.

Useful for debugging builds or exploring what network requests a build makes.
All traffic is recorded to the ledger just as in a normal build. Drivers that
cannot attach an interactive session say so.`,
	Example: `  warden shell
  warden shell ./my-project`,
	RunE: runShell,
}

func init() {
	driver.WardenVersion = version
	// The parse must agree with the scan that selected the driver.
	rootCmd.PersistentPreRunE = func(*cobra.Command, []string) error { return checkScan() }
	rootCmd.SetUsageTemplate(usageTemplate)
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "",
		"config file to read, the only one that is "+
			"(default: WARDEN_CONFIG, then ./warden.toml if it exists)")
	rootCmd.PersistentFlags().StringVar(&flagDriver, "driver", "",
		"driver to run: container (default), qemu, vz, hyperv or firecracker, "+
			"or the name or path of another driver binary")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false,
		"verbose output")
	rootCmd.PersistentFlags().StringVar(&flagColor, "color", "",
		"color output (auto, always, never)")

	for _, c := range []*cobra.Command{buildCmd, shellCmd} {
		f := c.Flags()
		f.StringVar(&flagCapture, "capture", "",
			"capture payloads (none, headers, bodies, all)")
		f.StringVarP(&flagOutput, "output", "o", "",
			"output directory for build results (default: warden-output)")
		f.BoolVar(&flagNoCompress, "no-compress", false,
			"disable zstd compression of the ledger and CA certificate")
		f.StringVar(&flagTimeout, "timeout", "",
			"maximum build duration (e.g., 10m, 1h)")
		f.StringVar(&flagScript, "script", "",
			"build script to run (VM drivers)")
		f.StringVar(&flagImage, "image", "",
			"base disk image for the build VM (VM drivers)")
		f.StringVar(&flagGuestOS, "guest-os", "",
			"guest OS for VM drivers: linux (default) or windows")
		f.IntVar(&flagCPUs, "cpus", 0,
			"build VM vCPUs (VM drivers; default per driver)")
		f.IntVar(&flagMemoryMB, "memory", 0,
			"build VM memory in MiB (VM drivers; default per driver)")
		f.BoolVar(&flagKeepVMs, "keep-vms", false,
			"leave VMs, disks and the work dir in place for inspection (VM drivers)")
		f.StringVar(&flagTempDir, "tmpdir", "",
			"directory to create each build's private work dir in "+
				"(default: the OS temp dir)")
		addBuildLinkFlag(c)
	}
	buildCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false,
		"do not echo the guest build console (still captured to build-output.log)")

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(shellCmd)
}

func main() {
	rootCmd.SetFlagErrorFunc(flagError)
	fixableArgs(rootCmd)
	if err := execute(os.Args[1:]); err != nil {
		var es exitStatusError
		if errors.As(err, &es) {
			os.Exit(int(es))
		}
		// cobra reports an unknown subcommand as a plain error.
		if strings.HasPrefix(err.Error(), "unknown command ") {
			err = driver.Fixable("see 'warden --help'", "%v", err)
		}
		var fe *driver.FixableError
		if errors.As(err, &fe) {
			log.ErrorWithFix(err, fe.Fix)
			os.Exit(1)
		}
		var de *driverError
		if errors.As(err, &de) {
			de.report()
			os.Exit(1)
		}
		logError(err)
		os.Exit(1)
	}
}

// fixableArgs makes every command's argument-count errors FixableErrors, so
// a mistyped invocation is answered with its help pointer, not a bug link.
func fixableArgs(c *cobra.Command) {
	if args := c.Args; args != nil {
		c.Args = func(cmd *cobra.Command, a []string) error {
			if err := args(cmd, a); err != nil {
				help := fmt.Sprintf("see '%s --help'", cmd.CommandPath())
				return driver.Fixable(help, "%v", err)
			}
			return nil
		}
	}
	for _, sub := range c.Commands() {
		fixableArgs(sub)
	}
}

// resolveConfig loads the config file and applies the flags over it. The
// driver it selects is found later (selectDriver), after the caller has said
// which file it read: the file can be a repository's.
func resolveConfig() (*Config, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	// A background image prepare this run starts reads the same file.
	driver.ConfigFile = cfg.File

	if flagDriver != "" {
		cfg.Runtime.Driver, cfg.Runtime.DriverSource = flagDriver, sourceFlag
	}
	if flagVerbose {
		cfg.Output.Verbose = true
	}
	if flagColor != "" {
		cfg.Output.Color = flagColor
	}
	setColorMode(cfg.Output.Color)
	return cfg, nil
}

// announcedConfig is resolveConfig for the commands that say on stderr which
// config file they read, before any other line. doctor says it in its
// report instead. A load that fails prints its error, which names the file.
func announcedConfig() (*Config, error) {
	cfg, err := resolveConfig()
	if cfg != nil {
		log.Info(configLine(cfg))
	}
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolveRuntime returns the container runtime to run: the one the
// container driver's settings name when it is a supported runtime, the
// detected one when none does. Another value is refused here, before
// anything is looked up, run or created. Only the commands that run the
// container runtime call this, so a run that does not use the setting does
// not validate it (UX-08).
func resolveRuntime(set driver.Settings) (string, error) {
	v, _ := set.String("runtime")
	if v == "" {
		return DetectRuntime()
	}
	if name, ok := supportedRuntime(v); ok {
		return name, nil
	}
	return "", unsupportedRuntime(v, set.Source("runtime"))
}

// validateGuestOS refuses a --guest-os the driver does not take: the bundled
// drivers with today's messages, another driver by its describe's list.
func validateGuestOS(guestOS, driverName string, caps capabilities) error {
	if guestOS != "" && !contains(bundledDrivers, driverName) {
		if !contains(caps.GuestOS, guestOS) {
			return driver.Fixable("", "invalid --guest-os %q (driver %q takes %s)",
				guestOS, driverName, strings.Join(caps.GuestOS, ", "))
		}
		return nil
	}
	switch guestOS {
	case "", "linux", "windows":
	default:
		return driver.Fixable("",
			"invalid --guest-os %q (want \"linux\" or \"windows\")", guestOS)
	}
	if guestOS == "windows" && driverName != "qemu" && driverName != "hyperv" {
		return driver.Fixable("",
			"--guest-os windows is currently only supported by "+
				"--driver qemu or --driver hyperv")
	}
	return nil
}

// newRequest resolves config and flags into the BuildRequest every driver
// takes. Precedence everywhere: flag, then the environment variable, then
// the config file, then the driver's default.
func newRequest(cfg *Config, sd *selectedDriver, args []string) (*driver.BuildRequest,
	error) {
	isVM := sd.caps().VM
	if err := checkDriverFlags(sd); err != nil {
		return nil, err
	}

	capture, err := resolveCapture(cfg)
	if err != nil {
		return nil, err
	}
	timeout, err := resolveTimeout()
	if err != nil {
		return nil, err
	}
	tempDir, err := resolveTempDir(cfg)
	if err != nil {
		return nil, err
	}

	var link buildlink.Link
	var dns []netip.Addr
	if isVM {
		if link, _, err = resolveBuildLink(cfg); err != nil {
			return nil, err
		}
		dns = cfg.VM.DNSAddrs
	}

	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	dockerfile, contextDir, script, err := resolveBuildInput(path, isVM, flagGuestOS,
		flagScript)
	if err != nil {
		return nil, err
	}

	// Read here, before the output directory, the work directory or the
	// relay exists; the claim sends what was read.
	upstreamCAs, err := resolveUpstreamCAs(cfg, log.Info)
	if err != nil {
		return nil, err
	}

	return &driver.BuildRequest{
		Script:           script,
		ContextDir:       contextDir,
		Image:            flagImage,
		GuestOS:          flagGuestOS,
		Containerfile:    dockerfile,
		OutputDir:        resolveOutputDir(cfg),
		CaptureMode:      capture,
		Compress:         resolveCompress(cfg),
		Quiet:            flagQuiet,
		Timeout:          timeout,
		CPUs:             vmFlagOr(flagCPUs, cfg.VM.CPUs, isVM),
		MemoryMB:         vmFlagOr(flagMemoryMB, cfg.VM.MemoryMB, isVM),
		KeepVMs:          flagKeepVMs,
		BuildLink:        link,
		DNS:              dns,
		TempDir:          tempDir,
		UpstreamCACerts:  upstreamCAs,
		UpstreamSystemCA: resolveSystemCA(cfg),
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	}, nil
}

// checkDriverFlags refuses warden's flags of a build that the driver cannot
// use; the driver's own it checks itself, in its probe.
func checkDriverFlags(sd *selectedDriver) error {
	d, caps := sd.name(), sd.caps()
	if err := validateFlagsForDriver(caps.VM); err != nil {
		return err
	}
	return validateGuestOS(flagGuestOS, d, caps)
}

// resolveCapture applies the flag, then warden.toml, then validates the mode.
func resolveCapture(cfg *Config) (string, error) {
	capture := flagCapture
	if capture == "" {
		capture = cfg.Build.Capture
	}
	if err := validateCapture(capture); err != nil {
		return "", err
	}
	return capture, nil
}

// resolveOutputDir applies the flag, then warden.toml, then the default.
func resolveOutputDir(cfg *Config) string {
	if flagOutput != "" {
		return flagOutput
	}
	if cfg.Build.OutputDir != "" {
		return cfg.Build.OutputDir
	}
	return "warden-output"
}

// resolveCompress is on by default; an explicit compress=false in config wins
// over the (negated) --no-compress flag's default.
func resolveCompress(cfg *Config) bool {
	if cfg.Build.Compress != nil && !*cfg.Build.Compress {
		return false
	}
	return !flagNoCompress
}

// resolveSystemCA defaults to trusting the system CA bundle unless config opts
// out.
func resolveSystemCA(cfg *Config) bool {
	if cfg.Relay.SystemCABundle != nil {
		return *cfg.Relay.SystemCABundle
	}
	return true
}

// resolveTimeout parses the --timeout flag; empty means no timeout.
func resolveTimeout() (time.Duration, error) {
	if flagTimeout == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(flagTimeout)
	if err != nil {
		return 0, driver.Fixable(
			"give a Go duration such as 30s, 10m or 1h",
			"invalid --timeout: %v", err)
	}
	return timeout, nil
}

// resolveTempDir applies the flag, then warden.toml, and makes it absolute.
func resolveTempDir(cfg *Config) (string, error) {
	tempDir := flagTempDir
	if tempDir == "" {
		tempDir = cfg.Build.TempDir
	}
	if tempDir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(tempDir)
	if err != nil {
		return "", driver.Fixable("", "invalid --tmpdir: %v", err)
	}
	return abs, nil
}

// addBuildLinkFlag registers --build-link on a command (build, shell, doctor).
func addBuildLinkFlag(c *cobra.Command) {
	c.Flags().StringVar(&flagBuildLink, "build-link", "",
		"advanced: the VM drivers' relay-to-build link, a /30 "+
			"(default: [vm] build_link, then "+buildlink.DefaultCIDR+")")
}

// resolveBuildLink returns the build link and where it came from: the flag,
// then [vm] build_link (already checked at load), then the default. The flag
// is checked here, before any driver work.
func resolveBuildLink(cfg *Config) (buildlink.Link, string, error) {
	if flagBuildLink != "" {
		link, err := checkBuildLink(flagBuildLink)
		if err != nil {
			return buildlink.Link{}, "", driver.Fixable(
				fmt.Sprintf("pass --build-link a valid /30, e.g. --build-link %s",
					exampleBuildLink),
				"--build-link: %v", err)
		}
		return link, "--build-link", nil
	}
	if cfg.VM.BuildLink != "" {
		link, err := checkBuildLink(cfg.VM.BuildLink)
		if err != nil {
			return buildlink.Link{}, "", driver.Fixable("",
				"[vm] build_link in %s: %v", cfg.VM.BuildLinkSource, err)
		}
		return link, cfg.VM.BuildLinkSource, nil
	}
	return buildlink.Default(), "default", nil
}

// vmFlagOr returns flag when set or when the driver is not a VM driver;
// otherwise the config default (VM-only flags default per driver).
func vmFlagOr(flag, conf int, isVM bool) int {
	if flag != 0 || !isVM {
		return flag
	}
	return conf
}

// prepared is a build or shell that prepare readied: its configuration and
// request, its driver, and the build's ID and hold on its lock and record.
type prepared struct {
	cfg  *Config
	req  *driver.BuildRequest
	sd   *selectedDriver
	id   string
	live *liveBuild
	// done removes the stage directory and then the build's lock and record,
	// or, with keep, leaves the record and lock for warden clean.
	done func(keep bool)
}

// prepare resolves everything a build or shell needs and probes the driver's
// run, then makes the build ID, takes the build's lock and writes its
// record, and creates the output directory with driver.json and stages the
// build script, in that order (DRV-33, DRV-45, DRV-56). A shell needs a
// driver with capabilities.shell.
func prepare(args []string, mode string) (*prepared, error) {
	cfg, err := announcedConfig()
	if err != nil {
		return nil, err
	}
	sd, err := selectDriver(cfg)
	if err != nil {
		return nil, err
	}
	if mode == driver.ModeShell && !sd.caps().Shell {
		return nil, driver.Fixable(
			"use warden build, or --driver container for an interactive shell",
			"warden shell is not supported by --driver %s", sd.name())
	}
	req, err := newRequest(cfg, sd, args)
	if err != nil {
		return nil, err
	}
	p := &prepared{cfg: cfg, req: req, sd: sd}
	if sd.bin != nil {
		out, _ := filepath.Abs(req.OutputDir)
		if err := checkUTF8Paths(map[string]string{"context": req.ContextDir,
			"output": out, "temporary": req.TempDir}); err != nil {
			return nil, err
		}
	}
	if err := probeRun(p); err != nil {
		return nil, err
	}
	dj, err := newDriverJSON(sd)
	if err != nil {
		return nil, err
	}
	p.id = driver.RandAlphaNum(8)
	if p.live, err = startBuildRecord(newRecord(p)); err != nil {
		return nil, err
	}
	unstage, err := stageBuild(req, sd.caps(), p.id, dj)
	if err != nil {
		_ = p.live.end()
		return nil, err
	}
	p.done = func(keep bool) {
		unstage()
		if keep {
			p.live.keep()
			return
		}
		if err := p.live.end(); err != nil {
			log.Info(fmt.Sprintf("warning: removing the build's lock and record: %v",
				err))
		}
	}
	return p, nil
}

// probeRun probes the driver's run before anything is created (DRV-33).
func probeRun(p *prepared) error {
	probe := newStart(p.cfg, p.req, p.sd.caps(), "", "", "")
	probe.Probe = true
	if p.sd.bin != nil {
		probe.Contract = p.sd.bin.contract
		return p.sd.bin.probeRun(probe)
	}
	// The probe is given a copy, so that what it resolves (a VM driver's
	// resolvers) is resolved again by the run.
	probeReq := *p.req
	_, err := callLinkedRun(context.Background(), p.sd.linked, p.sd.name(), p.cfg,
		&probeReq, probe, nil)
	return err
}

// newRecord is the record of the build p prepares (DRV-59): a driver binary's
// path and digest, or for a linked driver warden's own path.
func newRecord(p *prepared) buildRecord {
	rec := buildRecord{
		Driver: p.sd.name(), ID: p.id, Path: p.sd.found.Path, Linked: p.sd.linked != nil,
		Contract: contractVersion, TempDir: p.req.TempDir,
		StageDir: stageDir(p.req.TempDir, p.id), ConfigFile: absConfigFile(p.cfg),
		Args: driverFlags.argsFor("run"),
	}
	if p.sd.bin != nil {
		rec.Contract = p.sd.bin.contract
		rec.SHA256, _ = fileSHA256(p.sd.found.Path)
	}
	return rec
}

// absConfigFile is the absolute path of the config file warden read, or ""
// when it read none.
func absConfigFile(cfg *Config) string {
	if cfg.File == "" {
		return ""
	}
	if abs, err := filepath.Abs(cfg.File); err == nil {
		return abs
	}
	return cfg.File
}

func runBuild(_ *cobra.Command, args []string) error {
	p, err := prepare(args, driver.ModeBuild)
	if err != nil {
		return err
	}
	out, err := runSequence(p, driver.ModeBuild)
	p.done(out.leftovers)
	if err != nil {
		return err
	}
	return reportBuild(out, p.req)
}

func runShell(_ *cobra.Command, args []string) error {
	p, err := prepare(args, driver.ModeShell)
	if err != nil {
		return err
	}
	out, err := runSequence(p, driver.ModeShell)
	p.done(out.leftovers)
	if err != nil {
		return err
	}
	return reportShell(out)
}

// exitStatusError ends warden with that exit code, the outcome already
// reported.
type exitStatusError int

func (e exitStatusError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// Exit codes for build outcomes the user caused rather than warden: a
// --timeout expiring (as timeout(1) reports it) and an interrupt (128+SIGINT).
const (
	exitTimedOut    = 124
	exitInterrupted = 130
)

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/container"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

var version = "dev"

var (
	flagRuntime string
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
	flagNetwork    string
	flagKeepVMs    bool
	flagTempDir    string
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
	rootCmd.PersistentFlags().StringVar(&flagRuntime, "runtime", "",
		"container runtime (finch, docker, podman)")
	rootCmd.PersistentFlags().StringVar(&flagDriver, "driver", "",
		"orchestration driver (container, qemu, vz, hyperv)")
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
		f.StringVar(&flagNetwork, "network", "",
			"pre-provisioned network to reuse (hyperv: the durable switch from `warden setup`)")
		f.BoolVar(&flagKeepVMs, "keep-vms", false,
			"leave VMs, disks and the work dir in place for inspection (VM drivers)")
		f.StringVar(&flagTempDir, "tmpdir", "",
			"directory to create each build's private work dir in (default: the OS temp dir)")
	}
	buildCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false,
		"do not echo the guest build console (still captured to build-output.log)")

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(shellCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var es exitStatus
		if errors.As(err, &es) {
			os.Exit(int(es))
		}
		logError(err)
		os.Exit(1)
	}
}

func resolveConfig() (*Config, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	if flagRuntime != "" {
		cfg.Runtime.CLI = flagRuntime
	}
	if flagDriver != "" {
		cfg.Runtime.Driver = flagDriver
	}
	if flagVerbose {
		cfg.Output.Verbose = true
	}
	if flagColor != "" {
		cfg.Output.Color = flagColor
	}
	setColorMode(cfg.Output.Color)
	return cfg, validateDriver(cfg.Runtime.Driver)
}

func resolveRuntime(cfg *Config) (string, error) {
	if cfg.Runtime.CLI != "" {
		return cfg.Runtime.CLI, nil
	}
	return DetectRuntime()
}

func validateGuestOS(guestOS, driverName string) error {
	switch guestOS {
	case "", "linux", "windows":
	default:
		return fmt.Errorf(
			"invalid --guest-os %q (want \"linux\" or \"windows\")", guestOS)
	}
	if guestOS == "windows" && driverName != "qemu" && driverName != "hyperv" {
		return fmt.Errorf(
			"--guest-os windows is currently only supported by --driver qemu or --driver hyperv")
	}
	return nil
}

// newDriver constructs the configured driver.
func newDriver(cfg *Config) (driver.Driver, error) {
	switch cfg.Runtime.Driver {
	case "vz":
		d := vz.New()
		d.Verbose = cfg.Output.Verbose
		return d, nil
	case "qemu":
		d := qemu.New()
		d.Verbose = cfg.Output.Verbose
		return d, nil
	case "hyperv":
		d := hyperv.New()
		d.Verbose = cfg.Output.Verbose
		return d, nil
	default:
		runtime, err := resolveRuntime(cfg)
		if err != nil {
			return nil, err
		}
		return &container.Driver{
			Runtime:    runtime,
			Verbose:    cfg.Output.Verbose,
			Version:    version,
			Extensions: driver.DefaultExtensions(),
		}, nil
	}
}

// newRequest resolves config and flags into the BuildRequest every driver
// takes. Precedence everywhere: flag, then warden.toml / user config, then
// the driver's default.
func newRequest(cfg *Config, args []string) (*driver.BuildRequest, error) {
	d := cfg.Runtime.Driver
	if err := validateFlagsForDriver(d); err != nil {
		return nil, err
	}
	if err := validateGuestOS(flagGuestOS, d); err != nil {
		return nil, err
	}

	capture := flagCapture
	if capture == "" {
		capture = cfg.Build.Capture
	}
	if err := validateCapture(capture); err != nil {
		return nil, err
	}
	outputDir := flagOutput
	if outputDir == "" {
		outputDir = cfg.Build.OutputDir
	}
	if outputDir == "" {
		outputDir = "warden-output"
	}
	compress := !flagNoCompress
	if cfg.Build.Compress != nil && !*cfg.Build.Compress {
		compress = false
	}
	systemCA := true
	if cfg.Relay.SystemCABundle != nil {
		systemCA = *cfg.Relay.SystemCABundle
	}
	var timeout time.Duration
	if flagTimeout != "" {
		var err error
		if timeout, err = time.ParseDuration(flagTimeout); err != nil {
			return nil, fmt.Errorf("invalid --timeout: %w", err)
		}
	}
	isVM := d == "qemu" || d == "vz" || d == "hyperv"
	vmOr := func(flag, conf int) int {
		if flag != 0 || !isVM {
			return flag
		}
		return conf
	}
	network := flagNetwork
	if network == "" && d == "hyperv" {
		network = cfg.VM.Network
	}
	tempDir := flagTempDir
	if tempDir == "" {
		tempDir = cfg.Build.TempDir
	}
	if tempDir != "" {
		abs, err := filepath.Abs(tempDir)
		if err != nil {
			return nil, fmt.Errorf("invalid --tmpdir: %w", err)
		}
		tempDir = abs
	}

	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	dockerfile, contextDir, script, err := resolveBuildInput(path, d, flagGuestOS, flagScript)
	if err != nil {
		return nil, err
	}

	return &driver.BuildRequest{
		Script:           script,
		ContextDir:       contextDir,
		Image:            flagImage,
		GuestOS:          flagGuestOS,
		Containerfile:    dockerfile,
		OutputDir:        outputDir,
		CaptureMode:      capture,
		Compress:         compress,
		Quiet:            flagQuiet,
		Timeout:          timeout,
		CPUs:             vmOr(flagCPUs, cfg.VM.CPUs),
		MemoryMB:         vmOr(flagMemoryMB, cfg.VM.MemoryMB),
		Network:          network,
		KeepVMs:          flagKeepVMs,
		TempDir:          tempDir,
		RelayImage:       cfg.Runtime.RelayImage,
		UpstreamCACerts:  cfg.Relay.UpstreamCACerts,
		UpstreamSystemCA: systemCA,
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	}, nil
}

func buildContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		select {
		case <-sig:
			fmt.Fprintf(os.Stderr,
				"\nInterrupted, cleaning up... "+
					"(press ctrl+c again to force)\n")
			cancel()
			signal.Stop(sig)
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// prepare resolves everything a build or shell needs.
func prepare(args []string) (driver.Driver, *driver.BuildRequest, error) {
	cfg, err := resolveConfig()
	if err != nil {
		return nil, nil, err
	}
	req, err := newRequest(cfg, args)
	if err != nil {
		return nil, nil, err
	}
	d, err := newDriver(cfg)
	if err != nil {
		return nil, nil, err
	}
	return d, req, nil
}

func runBuild(_ *cobra.Command, args []string) error {
	d, req, err := prepare(args)
	if err != nil {
		return err
	}
	defer d.Close()
	ctx, stop := buildContext()
	defer stop()

	start := time.Now()
	if _, err := d.StartBuild(ctx, req); err != nil {
		return buildFailed(ctx, err, req, time.Since(start))
	}
	fmt.Fprintf(os.Stderr, "warden: build SUCCEEDED (output: %s)\n", req.OutputDir)
	return nil
}

func runShell(_ *cobra.Command, args []string) error {
	d, req, err := prepare(args)
	if err != nil {
		return err
	}
	defer d.Close()
	ctx, stop := buildContext()
	defer stop()

	if err := d.Exec(ctx, req); err != nil {
		if err == driver.ErrExecNotSupported {
			return fmt.Errorf("warden shell is not supported by --driver %s", d.Name())
		}
		// Leaving the shell with a non-zero status is the user's own
		// result: pass it through, quietly.
		var ee *driver.ExitError
		if errors.As(err, &ee) {
			return exitStatus(ee.Code)
		}
		return err
	}
	return nil
}

// exitStatus ends warden with that exit code, the outcome already reported.
type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// Exit codes for build outcomes the user caused rather than warden: a
// --timeout expiring (as timeout(1) reports it) and an interrupt (128+SIGINT).
const (
	exitTimedOut    = 124
	exitInterrupted = 130
)

// buildFailed reports a failed build. Outcomes of the build itself (its
// script's non-zero exit, --timeout, Ctrl-C) are reported plainly and warden
// exits with a matching code; anything else is a warden or environment
// error, reported with the issue link.
func buildFailed(ctx context.Context, err error, req *driver.BuildRequest, elapsed time.Duration) error {
	var ee *driver.ExitError
	switch {
	case errors.As(err, &ee):
		msg := fmt.Sprintf("warden: build FAILED (exit code %d)", ee.Code)
		if ee.Detail != "" {
			msg += ": " + ee.Detail
		}
		fmt.Fprintln(os.Stderr, msg)
		return exitStatus(ee.Code)
	case req.Timeout > 0 && elapsed >= req.Timeout && errors.Is(err, context.DeadlineExceeded):
		fmt.Fprintf(os.Stderr, "warden: build FAILED: timed out after %s (--timeout)\n", req.Timeout)
		return exitStatus(exitTimedOut)
	case ctx.Err() != nil:
		fmt.Fprintln(os.Stderr, "warden: build interrupted")
		return exitStatus(exitInterrupted)
	}
	fmt.Fprintln(os.Stderr, "warden: build FAILED")
	return err
}

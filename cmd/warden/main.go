package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"warden/driver"
	"warden/driver/container"
	"warden/driver/qemu"
	"warden/driver/vz"
)

var version = "dev"

var (
	flagRuntime    string
	flagDriver     string
	flagVerbose    bool
	flagColor      string
	flagCapture    string
	flagOutput     string
	flagNoCompress bool
	flagScript     string
	flagImage      string
	flagTimeout    string
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
	Long: `Run a containerized build with full network auditing.

The path argument can be:
  - A directory containing a Dockerfile (default: current directory)
  - A path to a specific Dockerfile (context = its parent directory)

All network traffic during the build is recorded to a cryptographically
signed ledger. The ledger directory is printed on completion.`,
	Example: `  warden build
  warden build ./my-project
  warden build ./my-project/Dockerfile.prod`,
	RunE: runBuild,
}

var shellCmd = &cobra.Command{
	Use:   "shell [path]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Open an interactive shell in the build environment",
	Long: `Open an interactive shell in a network-audited build container.

Useful for debugging builds or exploring what network requests a build makes.
All traffic is recorded to the ledger just as in a normal build.`,
	Example: `  warden shell
  warden shell ./my-project`,
	RunE: runShell,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagRuntime, "runtime", "",
		"container runtime (finch, docker, podman)")
	rootCmd.PersistentFlags().StringVar(&flagDriver, "driver", "",
		"orchestration driver (container, qemu, vz)")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false,
		"verbose output")
	rootCmd.PersistentFlags().StringVar(&flagColor, "color", "",
		"color output (auto, always, never)")

	buildCmd.Flags().StringVar(&flagCapture, "capture", "",
		"capture payloads to disk (none, headers, bodies, all)")
	buildCmd.Flags().StringVarP(&flagOutput, "output", "o", "",
		"output directory for build results (default: warden-output)")
	buildCmd.Flags().BoolVar(&flagNoCompress, "no-compress", false,
		"disable zstd compression of ledger and payloads")
	buildCmd.Flags().StringVar(&flagScript, "script", "",
		"build script to run (vm drivers)")
	buildCmd.Flags().StringVar(&flagImage, "image", "",
		"disk image for build VM (qemu driver)")
	buildCmd.Flags().StringVar(&flagTimeout, "timeout", "",
		"maximum build duration (e.g., 10m, 1h)")
	shellCmd.Flags().StringVar(&flagCapture, "capture", "",
		"capture payloads to disk (none, headers, bodies, all)")
	shellCmd.Flags().StringVarP(&flagOutput, "output", "o", "",
		"output directory for build results (default: warden-output)")
	shellCmd.Flags().BoolVar(&flagNoCompress, "no-compress", false,
		"disable zstd compression of ledger and payloads")

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(shellCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		logError(err)
		os.Exit(1)
	}
}

func resolveConfig() (*Config, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	// CLI flags override config
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

	return cfg, nil
}

func resolveRuntime(cfg *Config) (string, error) {
	setColorMode(cfg.Output.Color)

	runtime := cfg.Runtime.CLI
	if runtime == "" {
		detected, err := DetectRuntime()
		if err != nil {
			return "", err
		}
		runtime = detected
	}
	return runtime, nil
}

func defaultExtensions() []driver.Extension {
	return driver.DefaultExtensions()
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	if err := validateDriver(cfg.Runtime.Driver); err != nil {
		return err
	}

	path := ""
	if len(args) > 0 {
		path = args[0]
	}

	capture := flagCapture
	if capture == "" {
		capture = cfg.Build.Capture
	}
	if err := validateCapture(capture); err != nil {
		return err
	}
	outputDir := flagOutput
	if outputDir == "" {
		outputDir = cfg.Build.OutputDir
	}
	compress := !flagNoCompress
	if cfg.Build.Compress != nil && !*cfg.Build.Compress {
		compress = false
	}

	if err := validateFlagsForDriver(cfg.Runtime.Driver); err != nil {
		return err
	}

	// Dispatch to VM drivers when requested
	switch cfg.Runtime.Driver {
	case "vz":
		dockerfile, contextDir, err := ResolvePath(path)
		if err != nil {
			return err
		}
		d := vz.New()
		defer d.Close()
		_, buildErr := d.StartBuild(context.Background(), &driver.BuildRequest{
			ContextDir:    contextDir,
			Containerfile: dockerfile,
			Script:        flagScript,
			Image:         flagImage,
			CaptureMode:   capture,
			OutputDir:     outputDir,
			Compress:      compress,
			Stdin:         os.Stdin,
			Stdout:        os.Stdout,
			Stderr:        os.Stderr,
		})
		return buildErr

	case "qemu":
		dockerfile, contextDir, err := ResolvePath(path)
		if err != nil {
			return err
		}
		d := qemu.New()
		d.Verbose = cfg.Output.Verbose
		defer d.Close()
		var timeout time.Duration
		if flagTimeout != "" {
			var err error
			timeout, err = time.ParseDuration(flagTimeout)
			if err != nil {
				return fmt.Errorf("invalid --timeout: %w", err)
			}
		}
		_, buildErr := d.StartBuild(context.Background(), &driver.BuildRequest{
			ContextDir:    contextDir,
			Containerfile: dockerfile,
			Script:        flagScript,
			Image:         flagImage,
			CaptureMode:   capture,
			OutputDir:     outputDir,
			Compress:      compress,
			Timeout:       timeout,
			Stdin:         os.Stdin,
			Stdout:        os.Stdout,
			Stderr:        os.Stderr,
		})
		return buildErr
	}

	// Default: container driver
	runtime, err := resolveRuntime(cfg)
	if err != nil {
		return err
	}
	dockerfile, contextDir, err := ResolvePath(path)
	if err != nil {
		return err
	}
	d := &container.Driver{
		Runtime:    runtime,
		Verbose:    cfg.Output.Verbose,
		Version:    version,
		Extensions: defaultExtensions(),
	}
	defer d.Close()
	_, buildErr := d.StartBuild(context.Background(), &driver.BuildRequest{
		ContextDir:    contextDir,
		Containerfile: dockerfile,
		CaptureMode:   capture,
		OutputDir:     outputDir,
		Compress:      compress,
		RelayImage:    cfg.Runtime.RelayImage,
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
	})
	return buildErr
}

func runShell(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	path := ""
	if len(args) > 0 {
		path = args[0]
	}

	capture := flagCapture
	if capture == "" {
		capture = cfg.Build.Capture
	}
	outputDir := flagOutput
	if outputDir == "" {
		outputDir = cfg.Build.OutputDir
	}
	compress := !flagNoCompress
	if cfg.Build.Compress != nil && !*cfg.Build.Compress {
		compress = false
	}

	// Dispatch to vz driver when requested
	if cfg.Runtime.Driver == "vz" {
		dockerfile, contextDir, err := ResolvePath(path)
		if err != nil {
			return err
		}
		d := vz.New()
		defer d.Close()
		return d.Exec(context.Background(), &driver.BuildRequest{
			ContextDir:    contextDir,
			Containerfile: dockerfile,
			CaptureMode:   capture,
			OutputDir:     outputDir,
			Compress:      compress,
			Stdin:         os.Stdin,
			Stdout:        os.Stdout,
			Stderr:        os.Stderr,
		})
	}

	// Default: container driver
	runtime, err := resolveRuntime(cfg)
	if err != nil {
		return err
	}
	dockerfile, contextDir, err := ResolvePath(path)
	if err != nil {
		return err
	}
	d := &container.Driver{
		Runtime:    runtime,
		Verbose:    cfg.Output.Verbose,
		Version:    version,
		Extensions: defaultExtensions(),
	}
	defer d.Close()
	return d.Exec(context.Background(), &driver.BuildRequest{
		ContextDir:    contextDir,
		Containerfile: dockerfile,
		CaptureMode:   capture,
		OutputDir:     outputDir,
		Compress:      compress,
		RelayImage:    cfg.Runtime.RelayImage,
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
	})
}


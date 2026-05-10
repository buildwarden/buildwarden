package main

import (
	"os"

	"github.com/lesiw/ctrctl"
	"github.com/spf13/cobra"
)

var version = "dev"

var (
	flagRuntime    string
	flagVerbose    bool
	flagColor      string
	flagCapture    string
	flagOutput     string
	flagNoCompress bool
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
	if flagVerbose {
		cfg.Output.Verbose = true
	}
	if flagColor != "" {
		cfg.Output.Color = flagColor
	}

	return cfg, nil
}

func setupRuntime(cfg *Config) error {
	setColorMode(cfg.Output.Color)

	runtime := cfg.Runtime.CLI
	if runtime == "" {
		detected, err := DetectRuntime()
		if err != nil {
			return err
		}
		runtime = detected
	}
	ctrctl.Cli = []string{runtime}
	if cfg.Output.Verbose {
		ctrctl.Verbose = true
	}
	return nil
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	if err := setupRuntime(cfg); err != nil {
		return err
	}

	path := ""
	if len(args) > 0 {
		path = args[0]
	}

	dockerfile, contextDir, err := ResolvePath(path)
	if err != nil {
		return err
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

	env := NewCtrEnv()
	config := &BuildConfig{
		Context:       contextDir,
		Containerfile: dockerfile,
		Capture:       capture,
		OutputDir:     outputDir,
		Compress:      compress,
		RelayImage:    cfg.Runtime.RelayImage,
	}
	return env.Build(config)
}

func runShell(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	if err := setupRuntime(cfg); err != nil {
		return err
	}

	path := ""
	if len(args) > 0 {
		path = args[0]
	}

	dockerfile, contextDir, err := ResolvePath(path)
	if err != nil {
		return err
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

	env := NewCtrEnv()
	config := &BuildConfig{
		Context:       contextDir,
		Containerfile: dockerfile,
		Capture:       capture,
		OutputDir:     outputDir,
		Compress:      compress,
		RelayImage:    cfg.Runtime.RelayImage,
	}
	return env.Shell(config)
}

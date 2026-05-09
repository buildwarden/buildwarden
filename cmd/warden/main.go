package main

import (
	"fmt"
	"os"

	"warden/internal/orchestrator"

	"github.com/lesiw/ctrctl"
	"github.com/spf13/cobra"
)

var version = "dev"

var (
	flagRuntime string
	flagVerbose bool
	flagColor   string
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

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(shellCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func resolveConfig() (*orchestrator.Config, error) {
	cfg, err := orchestrator.LoadConfig()
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

func setupRuntime(cfg *orchestrator.Config) error {
	runtime := cfg.Runtime.CLI
	if runtime == "" {
		detected, err := orchestrator.DetectRuntime()
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

	dockerfile, contextDir, err := orchestrator.ResolvePath(path)
	if err != nil {
		return err
	}

	env := orchestrator.NewCtrEnv()
	config := &orchestrator.BuildConfig{
		Context:       contextDir,
		Containerfile: dockerfile,
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

	dockerfile, contextDir, err := orchestrator.ResolvePath(path)
	if err != nil {
		return err
	}

	env := orchestrator.NewCtrEnv()
	config := &orchestrator.BuildConfig{
		Context:       contextDir,
		Containerfile: dockerfile,
	}
	return env.Shell(config)
}

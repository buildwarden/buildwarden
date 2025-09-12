package main

import (
	"fmt"
	"os"
	"path/filepath"
	warden "warden/pkg"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "warden",
	Version:       "0.0.1",
	SilenceUsage:  true,
	SilenceErrors: true,
}
var buildCmd = &cobra.Command{
	Use:   "build PATH",
	Args:  cobra.ExactArgs(1),
	Short: "Build a project",
	Long:  "Build a project. The contents of PATH will be used as the build context",
	RunE:  build,
}
var shellCmd = &cobra.Command{
	Use:   "shell PATH",
	Args:  cobra.ExactArgs(1),
	Short: "Run a shell in a project",
	Long:  "Run a shell in a project. The contents of PATH will be used as the context",
	RunE:  shell,
}

func init() {
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(shellCmd)

	buildCmd.PersistentFlags().StringP("file", "f", "Dockerfile",
		"Path to the containerfile to be built.")
	shellCmd.PersistentFlags().StringP("file", "f", "Dockerfile",
		"Path to the containerfile to be built.")
}

func main() {
	os.Exit(run())
}

func run() int {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	return 0
}

func build(cmd *cobra.Command, args []string) error {
	buildEnv := warden.NewCtrEnv()
	contextFullPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("error resolving path: %w", err)
	}

	containerfile, err := cmd.Flags().GetString("file")
	if err != nil {
		return fmt.Errorf(`error reading cli flag "file": %w`, err)
	}

	config := &warden.BuildConfig{
		Context:       contextFullPath,
		Containerfile: containerfile,
	}
	if err := buildEnv.Build(config); err != nil {
		return err
	}

	return nil
}

func shell(cmd *cobra.Command, args []string) error {
	buildEnv := warden.NewCtrEnv()
	contextFullPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("error resolving path: %w", err)
	}

	containerfile, err := cmd.Flags().GetString("file")
	if err != nil {
		return fmt.Errorf(`error reading cli flag "file": %w`, err)
	}

	config := &warden.BuildConfig{
		Context:       contextFullPath,
		Containerfile: containerfile,
	}
	if err := buildEnv.Shell(config); err != nil {
		return err
	}

	return nil
}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

// Host commands, the same shape for every driver:
//
//	warden doctor --driver <d>   read-only preflight; non-zero exit when blocked
//	warden setup  --driver <d>   one-time, idempotent host preparation
//
// Driver-specific machinery with no analog elsewhere (the Hyper-V privileged
// helper service) stays under `warden hyperv`.

var (
	flagSetupNATPrefix string
	flagSetupHostIP    string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Preflight the host for the selected driver",
	Long: `Read-only preflight for the selected driver (--driver or warden.toml):

  container  a container runtime (finch, docker, podman) is available
  qemu       qemu-system-* and qemu-img are on PATH; relay VM assets exist
  vz         Apple Silicon macOS; warden signed with the virtualization entitlement
  hyperv     hypervisor and management stack present; which privilege tier this
             process holds and which operations run, delegate, or are blocked

Exits non-zero when a required capability is unmet, so it is usable as a CI
or agent gate.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := resolveConfig()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		switch drv := cfg.Runtime.Driver; drv {
		case "hyperv":
			if blocked := hyperv.Doctor(out, hyperv.Detect(hypervNetwork(cfg))); blocked {
				os.Exit(1)
			}
			return nil
		case "qemu":
			err = qemu.Preflight()
		case "vz":
			err = vz.Preflight()
		default:
			var rt string
			if rt, err = resolveRuntime(cfg); err == nil {
				fmt.Fprintf(out, "container runtime: %s\n", rt)
			}
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "ok")
		return nil
	},
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Prepare the host for the selected driver (one time, idempotent)",
	Long: `One-time, idempotent host preparation for the selected driver.

  hyperv   create the durable network builds reuse (a Private build switch,
           named by --network, plus an Internal NAT switch, host IP and NAT
           rule for relay egress). Run from an elevated terminal; afterwards
           builds run as a Hyper-V Administrators member with no elevation.
           Existing resources are verified, not recreated.

Other drivers need no host setup; their base images are prepared with
'warden image prepare'.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := resolveConfig()
		if err != nil {
			return err
		}
		if cfg.Runtime.Driver != "hyperv" {
			fmt.Fprintf(cmd.OutOrStdout(),
				"--driver %s needs no host setup; prepare a base image with 'warden image prepare'\n",
				driverName(cfg))
			return nil
		}
		return hyperv.RunSetup(cmd.Context(), cmd.OutOrStdout(), hyperv.SetupOptions{
			SwitchName: hypervNetwork(cfg),
			NATPrefix:  flagSetupNATPrefix,
			HostIP:     flagSetupHostIP,
			Verbose:    cfg.Output.Verbose,
		})
	},
}

var hypervCmd = &cobra.Command{
	Use:   "hyperv",
	Short: "Hyper-V privileged helper service (Windows host)",
}

var hypervServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the privileged Hyper-V helper service (LocalSystem)",
	Long: `The privileged helper is a LocalSystem Windows service that performs ONLY
host network standup (New-VMSwitch / New-NetNat / New-NetIPAddress) on behalf of
non-elevated builds, over a named pipe ACL'd to Administrators and Hyper-V
Administrators. With it installed, 'warden build --driver hyperv' runs entirely
without elevation and stands up a fresh isolated network per build (no shared
durable switch). VM lifecycle always runs in-process; only network standup is
delegated.`,
}

func init() {
	for _, c := range []*cobra.Command{doctorCmd, setupCmd} {
		c.Flags().StringVar(&flagNetwork, "network", "",
			"hyperv: durable build switch name (default: [vm] network, then warden-dev)")
	}
	setupCmd.Flags().StringVar(&flagSetupNATPrefix, "nat-prefix", "",
		"hyperv: NAT CIDR for relay egress (default 192.168.240.0/20)")
	setupCmd.Flags().StringVar(&flagSetupHostIP, "host-ip", "",
		"hyperv: host gateway IP on the NAT switch (default 192.168.240.1)")

	hypervServiceCmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install and start the privileged helper service (run once, elevated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := hyperv.InstallService(); err != nil {
				return fmt.Errorf("hyperv service install: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "privileged Hyper-V helper installed and started.")
			return nil
		},
	})
	hypervServiceCmd.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the privileged helper service (elevated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := hyperv.UninstallService(); err != nil {
				return fmt.Errorf("hyperv service uninstall: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "privileged Hyper-V helper removed.")
			return nil
		},
	})
	hypervServiceCmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run the privileged helper (invoked by the SCM; or in a console to smoke-test)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := hyperv.RunService(ctx); err != nil {
				return fmt.Errorf("hyperv service run: %w", err)
			}
			return nil
		},
	})
	hypervCmd.AddCommand(hypervServiceCmd)

	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(hypervCmd)
}

// hypervNetwork is the durable switch a Hyper-V build reuses: --network, then
// [vm] network, then the default.
func hypervNetwork(cfg *Config) string {
	switch {
	case flagNetwork != "":
		return flagNetwork
	case cfg.VM.Network != "":
		return cfg.VM.Network
	}
	return hyperv.DefaultDevSwitch
}

func driverName(cfg *Config) string {
	if cfg.Runtime.Driver == "" {
		return "container"
	}
	return cfg.Runtime.Driver
}

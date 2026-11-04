package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/firecracker"
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
	flagSetupSlots     int
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
  firecracker  /dev/kvm, the firecracker binary and boot assets, network slots
             owned by this user and their sysctls, an off-host resolver,
             reflink on the cache, orphaned VMMs; run as root it also checks
             the bridge table, WARDEN-FC rules and FORWARD policy

Every VM driver also reports its build link (the relay-to-build /30, from
--build-link, [vm] build_link or the default) and where it came from, whether
a host interface overlaps it, and the state of each launcher asset's
link-channel stamp (current, missing or stale, with the one-time rebuild).

Exits non-zero when a required capability is unmet, so it is usable as a CI
or agent gate.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := resolveConfig()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		drv := cfg.Runtime.Driver
		if flagBuildLink != "" && !isVMDriver(drv) {
			return driver.Fixable("", "--build-link requires a VM driver "+
				"(--driver qemu, vz, hyperv or firecracker)")
		}
		var link buildlink.Link
		linkBlocked := false
		if isVMDriver(drv) {
			if link, linkBlocked, err = doctorBuildLink(out, cfg); err != nil {
				return err
			}
		}
		switch drv {
		case "hyperv":
			detected := hyperv.Detect(hypervNetwork(cfg))
			if blocked := hyperv.Doctor(out, detected); blocked || linkBlocked {
				os.Exit(1)
			}
			return nil
		case "firecracker":
			if blocked := firecracker.Doctor(out, link); blocked || linkBlocked {
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
		if linkBlocked {
			os.Exit(1)
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
  firecracker  as root (sudo), create --slots network slots (default 16, at
           most 256) owned by the invoking user: a bridge and three TAPs
           each, their sysctls, the warden_fc bridge table and the WARDEN-FC
           iptables chains. Refuses, printing the fix, on a host whose filter
           FORWARD policy is ACCEPT or whose firewalld or nftables would drop
           the relays' traffic. Runtime state: re-run after every reboot.

Other drivers need no host setup; their base images are prepared with
'warden image prepare'.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := resolveConfig()
		if err != nil {
			return err
		}
		if cfg.Runtime.Driver == "firecracker" {
			return firecracker.RunSetup(cmd.OutOrStdout(),
				firecracker.SetupOptions{Slots: flagSetupSlots})
		}
		if flagSetupSlots != 0 {
			return driver.Fixable("", "--slots is only used by --driver firecracker")
		}
		if cfg.Runtime.Driver != "hyperv" {
			fmt.Fprintf(cmd.OutOrStdout(),
				"--driver %s needs no host setup; "+
					"prepare a base image with 'warden image prepare'\n",
				driverName(cfg))
			return nil
		}
		link, _, err := resolveBuildLink(cfg)
		if err != nil {
			return err
		}
		return hyperv.RunSetup(cmd.Context(), cmd.OutOrStdout(), hyperv.SetupOptions{
			BuildLink:  link,
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
			"hyperv: durable build switch name "+
				"(default: [vm] network, then warden-dev)")
	}
	addBuildLinkFlag(doctorCmd)
	setupCmd.Flags().StringVar(&flagSetupNATPrefix, "nat-prefix", "",
		"hyperv: NAT CIDR for relay egress (default 192.168.240.0/20)")
	setupCmd.Flags().StringVar(&flagSetupHostIP, "host-ip", "",
		"hyperv: host gateway IP on the NAT switch (default 192.168.240.1)")
	setupCmd.Flags().IntVar(&flagSetupSlots, "slots", 0,
		"firecracker: network slots to create, 1-256 (default 16)")

	hypervServiceCmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install and start the privileged helper service (run once, elevated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServiceOp(cmd.OutOrStdout(), "install",
				"privileged Hyper-V helper installed and started.",
				hyperv.InstallService)
		},
	})
	hypervServiceCmd.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the privileged helper service (elevated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServiceOp(cmd.OutOrStdout(), "uninstall",
				"privileged Hyper-V helper removed.",
				hyperv.UninstallService)
		},
	})
	hypervServiceCmd.AddCommand(&cobra.Command{
		Use: "run",
		Short: "Run the privileged helper " +
			"(invoked by the SCM; or in a console to smoke-test)",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return runServiceOp(cmd.OutOrStdout(), "run", "",
				func() error { return hyperv.RunService(ctx) })
		},
	})
	hypervCmd.AddCommand(hypervServiceCmd)

	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(hypervCmd)
}

// doctorBuildLink reports a VM driver's build link and where it came from,
// whether a host interface overlaps it, and each launcher stamp's state.
// blocked is true when a host interface holds a link address, which every
// build would refuse.
func doctorBuildLink(out io.Writer, cfg *Config) (buildlink.Link, bool, error) {
	link, source, err := resolveBuildLink(cfg)
	if err != nil {
		return buildlink.Link{}, false, err
	}
	fmt.Fprintf(out, "build link %s (%s)\n", link, source)
	blocked := false
	if host, err := driver.HostPrefixes(); err != nil {
		fmt.Fprintf(out, "build link: cannot list host interfaces: %v\n", err)
	} else {
		refuse, warn := link.HostOverlap(host)
		for _, p := range refuse {
			fmt.Fprintf(out,
				"build link: BLOCKED: host interface address %s is inside "+
					"the link; %s\n", p, driver.LinkFix)
			blocked = true
		}
		for _, p := range warn {
			fmt.Fprintf(out, "build link: warning: host network %s contains the link "+
				"(only the link's four addresses are shadowed)\n", p)
		}
		if len(refuse)+len(warn) == 0 {
			fmt.Fprintln(out, "build link: no host interface overlaps it")
		}
	}
	for _, st := range launcherStamps(cfg.Runtime.Driver) {
		fmt.Fprintln(out, st.Report())
	}
	return link, blocked, nil
}

// launcherStamps lists the driver's launcher assets that carry a
// link-channel stamp.
func launcherStamps(drv string) []driver.LauncherStamp {
	switch drv {
	case "qemu":
		return qemu.LauncherStamps()
	case "firecracker":
		return firecracker.LauncherStamps()
	case "hyperv":
		return hyperv.LauncherStamps()
	case "vz":
		return vz.LauncherStamps()
	}
	return nil
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

// runServiceOp runs a privileged-helper lifecycle operation and reports it. The
// operation is taken as a func value, not called inline, so the always-failing
// non-Windows stubs are not folded into an always-true error check
// (staticcheck SA4023); the platform split stays in the hyperv package.
func runServiceOp(out io.Writer, label, okMsg string, op func() error) error {
	if err := op(); err != nil {
		return fmt.Errorf("hyperv service %s: %w", label, err)
	}
	if okMsg != "" {
		fmt.Fprintln(out, okMsg)
	}
	return nil
}

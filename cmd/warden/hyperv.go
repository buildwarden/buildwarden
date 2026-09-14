package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver/hyperv"
)

var (
	flagHyperVSwitch    string
	flagHyperVNATPrefix string
	flagHyperVHostIP    string
)

var hypervCmd = &cobra.Command{
	Use:   "hyperv",
	Short: "Hyper-V driver setup and diagnostics (Windows host)",
	Long: `Setup and diagnostics for the Hyper-V host driver.

The Hyper-V driver is the only driver that needs host-level OS privilege.
Use 'warden hyperv doctor' to preflight exactly which capabilities this
process has and which still need authorization before running a build.`,
}

var hypervDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Preflight the Hyper-V driver's privilege and host capabilities",
	Long: `Read-only preflight for the Hyper-V driver.

Reports whether the hypervisor is present, whether the Hyper-V management
stack is installed, and which privilege tier the current process holds
(full Administrator for host-network standup vs. the Hyper-V Administrators
group for VM lifecycle). It then resolves, per operation, whether each will
run directly, delegate to the privileged service, or is blocked, and prints
the precise next action.

The checks reflect the CURRENT process's token. The gateway spawns warden
non-elevated, so an elevated verdict only appears when warden is launched
from an elevated terminal (a different token).

Exits non-zero when a required capability is unmet, so it is usable as a
CI or agent gate.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sw := flagHyperVSwitch
		if sw == "" {
			sw = os.Getenv("WARDEN_HYPERV_SWITCH")
		}
		if sw == "" {
			sw = hyperv.DefaultDevSwitch
		}
		caps := hyperv.Detect(sw)
		if blocked := hyperv.Doctor(cmd.OutOrStdout(), caps); blocked {
			// A not-ready result is an expected gate outcome, not a bug. The
			// report above already carries the remediation, so control the
			// exit code directly rather than returning an error (which the
			// CLI would decorate with a "file a bug" footer).
			os.Exit(1)
		}
		return nil
	},
}

var hypervSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create the durable dev network for the Hyper-V driver (run once, elevated)",
	Long: `Create (idempotently) the durable network the Hyper-V driver reuses:

  - a Private build vSwitch (the build VM's sole NIC; no host path), and
  - an Internal NAT vSwitch + host IP + NAT rule for the RELAY VM's controlled
    upstream egress only (the build VM never attaches to it).

Run this ONCE from a terminal launched as Administrator. Afterwards, routine
'warden build --driver hyperv --switch <name>' runs under the Hyper-V
Administrators group with no elevation, because per-build network standup is
skipped in favour of the reused switch.

Safe to re-run: existing resources are verified, not recreated, and a
pre-existing NAT on the same prefix is refused rather than clobbered.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sw := flagHyperVSwitch
		if sw == "" {
			sw = os.Getenv("WARDEN_HYPERV_SWITCH")
		}
		err := hyperv.RunSetup(cmd.Context(), cmd.OutOrStdout(), hyperv.SetupOptions{
			SwitchName: sw,
			NATPrefix:  flagHyperVNATPrefix,
			HostIP:     flagHyperVHostIP,
			Verbose:    flagVerbose,
		})
		if err != nil {
			// These are environmental/actionable conditions (not elevated, no
			// Hyper-V, NAT collision), not warden bugs, so print the message
			// plainly and exit rather than returning an error the CLI would
			// decorate with a "file a bug" footer.
			fmt.Fprintf(cmd.ErrOrStderr(), "hyperv setup: %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	hypervCmd.PersistentFlags().StringVar(&flagHyperVSwitch, "switch", "",
		"durable/dev vSwitch name (default: $WARDEN_HYPERV_SWITCH, then 'warden-dev')")
	hypervSetupCmd.Flags().StringVar(&flagHyperVNATPrefix, "nat-prefix", "",
		"NAT CIDR for relay egress (default 192.168.240.0/20)")
	hypervSetupCmd.Flags().StringVar(&flagHyperVHostIP, "host-ip", "",
		"host gateway IP on the NAT switch (default 192.168.240.1)")
	hypervCmd.AddCommand(hypervDoctorCmd)
	hypervCmd.AddCommand(hypervSetupCmd)
	rootCmd.AddCommand(hypervCmd)
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/resolvers"
	"github.com/buildwarden/buildwarden/driver/vz"
)

// Host commands, the same shape for every driver:
//
//	warden doctor --driver <d>   read-only preflight; non-zero exit when blocked
//	warden setup  --driver <d>   one-time, idempotent host preparation
//
// Driver-specific machinery with no analog elsewhere (the Hyper-V privileged
// helper service) stays under `warden hyperv`.

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Preflight the host for the selected driver",
	Long: `Read-only preflight for the selected driver (--driver, WARDEN_DRIVER or
the config file). The report's first line says which config file was read
and how it was chosen (--config, WARDEN_CONFIG, or ./warden.toml found in
the working directory).

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
A missing or stale stamp fails doctor on the hyperv relay boot VHDX and the
vz prepared image, which every build uses; on the qemu and firecracker
direct-boot initramfs it is reported only. Each VM driver also prints the
relay's upstream resolvers and their source ([vm] dns, or the host's), or
the refusal a build would get, which fails doctor.

With upstream CA paths configured (WARDEN_UPSTREAM_CA_CERTS, else [relay]
upstream_ca_certs), every driver's doctor reads them as a build would and
prints the same lines: each block or file that is skipped, with its reason,
and each path's count of certificates to send. A path a build would refuse
(it does not exist, cannot be read or gives no certificate) is BLOCKED and
fails doctor. With the container driver, a relay image that is not shaped
like an image reference fails doctor with the error a build would get.

Exits non-zero when a required capability is unmet, so it is usable as a CI
or agent gate.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := resolveConfig()
		out := cmd.OutOrStdout()
		if cfg != nil {
			fmt.Fprintln(out, configLine(cfg))
		}
		if err != nil {
			return err
		}
		sd, err := selectDriver(cfg)
		if err != nil {
			return err
		}
		drv := sd.name()
		if flagBuildLink != "" && !sd.caps().VM {
			return driver.Fixable("", "--build-link requires a VM driver "+
				"(--driver qemu, vz, hyperv or firecracker)")
		}
		blocked, err := doctorUpstreamCAs(out, cfg)
		if err != nil {
			return err
		}
		if sd.bin != nil {
			return doctorBinary(cfg, sd, blocked)
		}
		set, err := linkedSettings(cfg, sd, "doctor")
		if err != nil {
			return err
		}
		var link buildlink.Link
		var vmDNS []netip.Addr
		if isVMDriver(drv) {
			var linkBlocked bool
			if link, linkBlocked, err = doctorBuildLink(out, cfg); err != nil {
				return err
			}
			blocked = blocked || linkBlocked
			vmDNS = cfg.VM.DNSAddrs
		}
		switch drv {
		case "hyperv":
			detected := hyperv.Detect(hypervNetwork(set))
			if hyperv.Doctor(out, detected, link, vmDNS) || blocked {
				os.Exit(1)
			}
			return nil
		case "firecracker":
			if firecracker.Doctor(out, link, vmDNS) || blocked {
				os.Exit(1)
			}
			return nil
		case "qemu":
			if err = qemu.Preflight(); err == nil &&
				doctorResolvers(out, qemu.Resolvers, link, vmDNS) {
				blocked = true
			}
		case "vz":
			if err = vz.Preflight(); err == nil &&
				doctorResolvers(out, vz.Resolvers, link, vmDNS) {
				blocked = true
			}
		default:
			// The check a build makes, so that doctor does not pass a relay
			// image the build refuses.
			img, _ := set.String("relay-image")
			if err = checkRelayImage(img, set.Source("relay-image")); err != nil {
				return err
			}
			var rt string
			if rt, err = resolveRuntime(set); err == nil {
				fmt.Fprintf(out, "container runtime: %s\n", rt)
			}
		}
		if err != nil {
			return err
		}
		if blocked {
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

  hyperv   create the durable network builds reuse: --slots Private build
           switches (default 4, at most 64) named <network>-0 to
           <network>-<N-1> (--network, default warden-dev), one per
           concurrent build, plus an Internal NAT switch, host IP and NAT
           rule for relay egress, which every build uses. Slot switches
           above N with nothing attached are removed; the single <network>
           Private switch of earlier setups is retired when nothing is
           attached to it. Either refuses, naming the VMs, when one is in
           use. Run from an elevated terminal; afterwards builds run as a
           Hyper-V Administrators member with no elevation. Existing
           resources are verified, not recreated.
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
		cfg, err := announcedConfig()
		if err != nil {
			return err
		}
		sd, err := selectDriver(cfg)
		if err != nil {
			return err
		}
		if sd.bin != nil {
			return setupBinary(cfg, sd)
		}
		var set driver.Settings
		if sd.lists("setup") {
			if set, err = linkedSettings(cfg, sd, "setup"); err != nil {
				return err
			}
		}
		slots, _ := settingInt(set, "slots")
		if cfg.Runtime.Driver == "firecracker" {
			return firecracker.RunSetup(cmd.OutOrStdout(),
				firecracker.SetupOptions{Slots: slots})
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
		natPrefix, _ := set.String("nat-prefix")
		hostIP, _ := set.String("host-ip")
		return hyperv.RunSetup(cmd.Context(), cmd.OutOrStdout(), hyperv.SetupOptions{
			BuildLink:  link,
			SwitchName: hypervNetwork(set),
			NATPrefix:  natPrefix,
			HostIP:     hostIP,
			Slots:      slots,
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
	addBuildLinkFlag(doctorCmd)

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
// blocked is true when a host interface holds a link address, or an asset
// every build needs has a missing or stale stamp: either refuses every build.
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
		if st.Blocks() {
			blocked = true
		}
	}
	return link, blocked, nil
}

// doctorResolvers prints the relay's upstream resolvers and their source, as
// resolve (a driver's Resolvers) chooses them from link and vmDNS ([vm] dns),
// or the refusal a build would get, and reports whether it refused.
func doctorResolvers(out io.Writer,
	resolve func(buildlink.Link, []netip.Addr) ([]netip.Addr, string, error),
	link buildlink.Link, vmDNS []netip.Addr) (blocked bool) {
	list, source, err := resolve(link, vmDNS)
	if err == nil {
		fmt.Fprintf(out, "relay resolvers: %s (from %s)\n", resolvers.Join(list), source)
		return false
	}
	from := ""
	if source != "" {
		from = " (from " + source + ")"
	}
	fmt.Fprintf(out, "relay resolvers%s: BLOCKED: %v\n", from, err)
	var fe *driver.FixableError
	if errors.As(err, &fe) && fe.Fix != "" {
		fmt.Fprintf(out, "relay resolvers: to fix: %s\n", fe.Fix)
	}
	return true
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

// hypervNetwork is the durable switch a Hyper-V build reuses: hyperv's
// network setting (--network, [driver.hyperv] network or its old name),
// else the default.
func hypervNetwork(set driver.Settings) string {
	if v, _ := set.String("network"); v != "" {
		return v
	}
	return hyperv.DefaultDevSwitch
}

// settingInt is an integer setting of set, which may be nil for a command
// the driver does not list.
func settingInt(set driver.Settings, name string) (int, bool) {
	if set == nil {
		return 0, false
	}
	return set.Int(name)
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

// hostRequest is the request of doctor and setup for a driver binary:
// {contract, vm, config_file, args, verbose}.
func hostRequest(cfg *Config, sd *selectedDriver, command string) (map[string]any,
	error) {
	req := withConfig(map[string]any{"contract": sd.bin.contract,
		"args":    driverFlags.argsFor(command),
		"verbose": cfg.Output.Verbose}, absConfigFile(cfg))
	if sd.caps().VM {
		link, source, err := resolveBuildLink(cfg)
		if err != nil {
			return nil, err
		}
		vm := map[string]any{"build_link": link.String(), "build_link_source": source}
		if source == "default" {
			vm["build_link"] = ""
		}
		req["vm"] = vm
	}
	return req, nil
}

// doctorBinary runs a driver binary's doctor after warden's own part of the
// report; blocked is whether warden's part blocks a build. It exits 1 when
// either does (UX-09).
func doctorBinary(cfg *Config, sd *selectedDriver, blocked bool) error {
	if !sd.bin.desc.lists("doctor") {
		fmt.Println("ok")
		if blocked {
			os.Exit(1)
		}
		return nil
	}
	req, err := hostRequest(cfg, sd, "doctor")
	if err != nil {
		return err
	}
	res, err := sd.bin.call("doctor", req, callOpts{})
	if err != nil {
		return err
	}
	var driverBlocked bool
	_ = json.Unmarshal(res["blocked"], &driverBlocked)
	if blocked || driverBlocked {
		os.Exit(1)
	}
	return nil
}

// setupBinary runs a driver binary's setup.
func setupBinary(cfg *Config, sd *selectedDriver) error {
	if !sd.bin.desc.lists("setup") {
		fmt.Printf("--driver %s needs no host setup\n", sd.name())
		return nil
	}
	req, err := hostRequest(cfg, sd, "setup")
	if err != nil {
		return err
	}
	_, err = sd.bin.call("setup", req, callOpts{})
	return err
}

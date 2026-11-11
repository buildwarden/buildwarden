package hyperv

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// Default addressing for the durable dev NAT network (relay egress only). The
// build VM never attaches to this; it lives on the Private build switch.
const (
	DefaultNATPrefix = "192.168.240.0/20"
	DefaultHostIP    = "192.168.240.1"
)

// SetupOptions configures `warden setup --driver hyperv`.
type SetupOptions struct {
	// SwitchName names the durable network: its NAT is <SwitchName>-nat and
	// its build switches <SwitchName>-0 and up. Empty -> warden-dev.
	SwitchName string
	// NATPrefix / HostIP address the relay's Internal NAT switch. Empty -> defaults.
	NATPrefix string
	HostIP    string
	// BuildLink is the build link builds will use ([vm] build_link or the
	// default); the NAT prefix must not overlap it. Zero is the default.
	BuildLink buildlink.Link
	// Slots is how many durable build switches setup makes, 1 to MaxSlots.
	// Zero is DefaultSlots.
	Slots   int
	Verbose bool
}

const (
	// MaxSlots is the most durable build switches setup makes.
	MaxSlots = 64
	// DefaultSlots is how many `warden setup --driver hyperv` makes.
	DefaultSlots = 4
)

// ValidateSlotCount checks a --slots value.
func ValidateSlotCount(n int) error {
	if n < 1 || n > MaxSlots {
		return driver.Fixable(fmt.Sprintf("pass --slots between 1 and %d", MaxSlots),
			"--slots %d is out of range (1-%d)", n, MaxSlots)
	}
	return nil
}

func (o *SetupOptions) applyDefaults() {
	if o.SwitchName == "" {
		o.SwitchName = DefaultDevSwitch
	}
	if o.NATPrefix == "" {
		o.NATPrefix = DefaultNATPrefix
	}
	if o.HostIP == "" {
		o.HostIP = DefaultHostIP
	}
	if o.Slots == 0 {
		o.Slots = DefaultSlots
	}
}

// RunSetup idempotently creates the durable dev network: an Internal NAT
// switch + host IP + NAT rule for relay egress, and the Private build
// switches <network>-0 to <network>-<Slots-1>, one per concurrent durable
// build. It trims unattached slots above Slots and retires the single shared
// <network> switch earlier setups made, and refuses to touch an attached one.
// It is the one-time, elevated step that lets non-elevated builds then run
// against the durable network.
//
// It preflights privilege itself and returns a clear, actionable error rather
// than surfacing a raw PowerShell failure when run without elevation.
func RunSetup(ctx context.Context, w io.Writer, opts SetupOptions) error {
	opts.applyDefaults()
	if err := ValidateSlotCount(opts.Slots); err != nil {
		return err
	}
	if err := checkNATPrefix(opts.NATPrefix, opts.BuildLink.OrDefault()); err != nil {
		return err
	}

	caps := Detect(opts.SwitchName)
	if !caps.Supported {
		return driver.Fixable(
			"", "hyperv setup requires a Windows host (current OS: %s)", caps.Platform)
	}
	if !caps.HypervisorPresent || !caps.HyperVFeature {
		return driver.Fixable(
			"enable it from an elevated PowerShell: "+
				"Enable-WindowsOptionalFeature -Online -FeatureName "+
				"Microsoft-Hyper-V -All, then reboot",
			"Hyper-V is not available on this host")
	}
	if caps.Resolve(OpNetworkStandup) == OutcomeBlocked {
		return driver.Fixable(
			"re-run `warden setup --driver hyperv` from a terminal "+
				"launched with Run as administrator",
			"network setup needs full Administrator rights, and this "+
				"process is not elevated")
	}

	p := newLocalProvisioner(opts.Verbose)
	res, err := p.EnsureNetwork(ctx, NetworkSpec{
		NATName:   opts.SwitchName + "-nat",
		NATPrefix: opts.NATPrefix,
		HostIP:    opts.HostIP,
	})
	if err != nil {
		return err
	}
	if want := netip.MustParsePrefix(opts.NATPrefix).Masked(); res.NATPrefix != want {
		return driver.Fixable(
			"remove it from an elevated shell (Remove-NetNat -Name "+res.NATName+
				"), then re-run setup to make it on "+want.String(),
			"the NAT %s already exists on %s, not --nat-prefix %s",
			res.NATName, res.NATPrefix, want)
	}
	slots, err := runSetupSlots(opts.SwitchName, opts.Slots)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "Durable dev network ready.")
	fmt.Fprintf(w, "  NAT switch (Internal) .... %s  (%s, host %s)\n",
		res.NATName, res.NATPrefix, res.HostIP)
	fmt.Fprintf(w, "  build switches (Private)  %s\n", strings.Join(slots.Slots, ", "))
	for _, sw := range slots.Removed {
		fmt.Fprintf(w, "  removed %s (above --slots %d, nothing attached)\n", sw,
			opts.Slots)
	}
	if slots.Retired != "" {
		fmt.Fprintf(w, "  retired %s (the shared build switch earlier setups made; "+
			"each durable build now takes a switch of its own)\n", slots.Retired)
	}
	if err := slots.err(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\nThis durable network is for NON-elevated builds (Hyper-V "+
		"Administrators), up to %d at a time:\n", opts.Slots)
	fmt.Fprintf(w, "  warden build --driver hyperv --network %s\n", opts.SwitchName)
	fmt.Fprintf(w, "  (or set network = \"%s\" under [vm] in warden.toml)\n", opts.SwitchName)
	fmt.Fprintln(w,
		"Elevated builds stand up a fresh build switch per build and use this NAT.")
	return nil
}

// checkNATPrefix refuses a NAT prefix that overlaps the build link, before
// setup changes anything: the relay's upstream and its build segment would
// share addresses.
func checkNATPrefix(natPrefix string, link buildlink.Link) error {
	p, err := netip.ParsePrefix(natPrefix)
	if err != nil {
		return driver.Fixable("pass --nat-prefix an IPv4 CIDR such as "+DefaultNATPrefix,
			"invalid NAT prefix %q: %v", natPrefix, err)
	}
	if p.Overlaps(link.Net) {
		return driver.Fixable(
			"pass another --nat-prefix (default "+DefaultNATPrefix+"), or set "+
				"[vm] build_link in warden.toml to a /30 outside it",
			"--nat-prefix %s overlaps the build link %s", natPrefix, link)
	}
	return nil
}

// cidrPrefixLen returns the prefix length of an IPv4 CIDR (e.g. 20 for /20).
func cidrPrefixLen(cidr string) (int, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, fmt.Errorf("invalid NAT prefix %q: %w", cidr, err)
	}
	ones, _ := ipnet.Mask.Size()
	return ones, nil
}

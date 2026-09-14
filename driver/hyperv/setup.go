package hyperv

import (
	"context"
	"fmt"
	"io"
	"net"
)

// Default addressing for the durable dev NAT network (relay egress only). The
// build VM never attaches to this; it lives on the Private build switch.
const (
	DefaultNATPrefix = "192.168.240.0/20"
	DefaultHostIP    = "192.168.240.1"
)

// SetupOptions configures `warden hyperv setup`.
type SetupOptions struct {
	// SwitchName is the durable build (Private) switch name. Empty -> warden-dev.
	SwitchName string
	// NATPrefix / HostIP address the relay's Internal NAT switch. Empty -> defaults.
	NATPrefix string
	HostIP    string
	Verbose   bool
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
}

// RunSetup idempotently creates the durable dev network (a Private build switch
// plus an Internal NAT switch + host IP + NAT rule for relay egress) and prints
// how to reuse it. It is the one-time, elevated step that lets the non-elevated
// gateway then iterate on builds against the reused switch.
//
// It preflights privilege itself and returns a clear, actionable error rather
// than surfacing a raw PowerShell failure when run without elevation.
func RunSetup(ctx context.Context, w io.Writer, opts SetupOptions) error {
	opts.applyDefaults()

	caps := Detect(opts.SwitchName)
	if !caps.Supported {
		return fmt.Errorf("hyperv setup requires a Windows host (current OS: %s)", caps.Platform)
	}
	if !caps.HypervisorPresent || !caps.HyperVFeature {
		return fmt.Errorf("Hyper-V is not available here; enable the Hyper-V feature and reboot")
	}
	if caps.Resolve(OpNetworkStandup) == OutcomeBlocked {
		return fmt.Errorf("network standup needs full Administrator: re-run `warden hyperv setup` from a terminal launched as Administrator")
	}

	p := newLocalProvisioner(opts.Verbose)
	res, err := p.EnsureNetwork(ctx, NetworkSpec{
		SwitchName: opts.SwitchName,
		SwitchType: "Private",
		NATPrefix:  opts.NATPrefix,
		HostIP:     opts.HostIP,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "Durable dev network ready.")
	fmt.Fprintf(w, "  build switch (Private) ... %s\n", res.BuildSwitch)
	fmt.Fprintf(w, "  NAT switch (Internal) .... %s  (%s, host %s)\n", res.NATName, opts.NATPrefix, opts.HostIP)
	fmt.Fprintln(w, "\nThis durable network is for NON-elevated builds (Hyper-V Administrators):")
	fmt.Fprintf(w, "  warden build --driver hyperv --switch %s\n", opts.SwitchName)
	fmt.Fprintf(w, "  (or set WARDEN_HYPERV_SWITCH=%s)\n", opts.SwitchName)
	fmt.Fprintln(w, "Elevated builds ignore it and stand up a fresh isolated network per build.")
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

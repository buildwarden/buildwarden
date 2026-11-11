package hyperv

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// The relay VM's upstream NIC sits on the NAT switch with a static address
// (New-NetNat provides routing but no DHCP), allocated per build in the NAT's
// prefix (allocate.go) and handed to the relay in its seed with the prefix,
// the host's address as gateway and a per-build claim token. The orchestrator
// claims the relay at that address with the token and serves the collector
// back over the same host-initiated connection, so the host needs no listener
// and no inbound firewall rule. The address is host-only: the build VM is on
// the Private switch and never reaches it.
const defaultReadyTimeout = 90 * time.Second

// RelayHostConfig configures StartRelay.
type RelayHostConfig struct {
	BuildID     string // used to name the VM and its resources
	OutputDir   string // where the collector lands this build's outputs
	BootVHDX    string // static, read-only UKI boot disk
	OverlayVHDX string // per-build differencing overlay path (created off BootVHDX)

	// NATSwitch is the Internal NAT switch the relay's upstream NIC attaches
	// to; NATPrefix and HostIP are that NAT's prefix and the host's address in
	// it, read back from the host: the relay's address is allocated in the
	// prefix and its gateway is HostIP.
	NATSwitch string
	NATPrefix netip.Prefix
	HostIP    netip.Addr
	// BuildSwitch is the ephemeral strategy's per-build Private switch, and
	// Slots the durable strategy's build switches, of which StartRelay takes
	// the lowest free one. Exactly one is set.
	BuildSwitch string
	Slots       []string

	// SeedDir is where the relay seed is staged, under the build's work
	// directory; the ISO is SeedDir + ".iso". MakeSeedISO packs it; nil is
	// the platform's IMAPI2 builder.
	SeedDir     string
	MakeSeedISO func(seedDir, iso string) error

	// Request carries the build's relay settings (capture mode, upstream CAs,
	// the build link and the relay's resolvers, DNS).
	Request *driver.BuildRequest
	// Environment is the ledger's environment record; nil records none.
	Environment *relaycfg.Environment
	// ContextDir and BuildScriptPath are the inputs the collector serves.
	ContextDir      string
	BuildScriptPath string
	// GuestOS picks the bootstrap config ("windows" or "linux").
	GuestOS string

	// KeepVM leaves the relay VM and its overlay for inspection (--keep-vms);
	// the VM's Notes and adapter keep holding its address and switch until
	// it is removed.
	KeepVM bool

	ClaimPort    int // default relaycfg.DefaultPort
	ReadyTimeout time.Duration
	OutputTee    io.Writer
}

// RelayHost is a running, claimed relay VM and the build's session.
type RelayHost struct {
	VMName  string
	Session *driver.Session
	// RelayIP is the relay's address on the NAT, where it was claimed.
	RelayIP netip.Addr
	// BuildSwitch is the Private switch the relay's build NIC is on, which the
	// build VM attaches to.
	BuildSwitch string

	prov    Provisioner
	overlay string
	keep    bool
}

// validateRelayHostConfig rejects a RelayHostConfig missing a required field.
func validateRelayHostConfig(cfg RelayHostConfig) error {
	switch {
	case cfg.BuildID == "":
		return fmt.Errorf("StartRelay: empty build ID")
	case cfg.OutputDir == "":
		return fmt.Errorf("StartRelay: empty output dir")
	case cfg.BootVHDX == "":
		return fmt.Errorf("StartRelay: empty boot VHDX")
	case cfg.OverlayVHDX == "":
		return fmt.Errorf("StartRelay: empty overlay VHDX")
	case (cfg.BuildSwitch == "") == (len(cfg.Slots) == 0):
		return fmt.Errorf("StartRelay: want exactly one of a build switch and slots")
	case cfg.NATSwitch == "":
		return fmt.Errorf("StartRelay: empty NAT switch")
	case !cfg.NATPrefix.IsValid() || !cfg.HostIP.IsValid():
		return fmt.Errorf("StartRelay: no NAT prefix and host IP")
	case cfg.SeedDir == "":
		return fmt.Errorf("StartRelay: empty relay seed dir")
	}
	return nil
}

// StartRelay prepares the build's collector, allocates the relay's address
// and build switch, creates the relay VM with its seed, boots it, and claims
// it with a fresh token. On any failure everything it created is torn down.
func StartRelay(ctx context.Context, prov Provisioner, cfg RelayHostConfig) (*RelayHost, error) {
	if cfg.ClaimPort == 0 {
		cfg.ClaimPort = relaycfg.DefaultPort
	}
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
	}
	if cfg.Request == nil {
		cfg.Request = &driver.BuildRequest{UpstreamSystemCA: true}
	}
	if cfg.MakeSeedISO == nil {
		cfg.MakeSeedISO = makeRelaySeedISO
	}
	if err := validateRelayHostConfig(cfg); err != nil {
		return nil, err
	}
	token, err := driver.RandomToken()
	if err != nil {
		return nil, fmt.Errorf("StartRelay: claim token: %w", err)
	}

	sess, err := driver.StartSession(driver.SessionConfig{
		OutputDir:   cfg.OutputDir,
		ContextDir:  cfg.ContextDir,
		BuildScript: cfg.BuildScriptPath,
		GuestOS:     cfg.GuestOS,
		OutputTee:   cfg.OutputTee,
	})
	if err != nil {
		return nil, fmt.Errorf("StartRelay: %w", err)
	}

	h := &RelayHost{
		VMName:  relayVMPrefix + cfg.BuildID,
		Session: sess,
		prov:    prov,
		overlay: cfg.OverlayVHDX,
		keep:    cfg.KeepVM,
	}
	if err := h.create(ctx, cfg, token); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: %w", err)
	}
	if err := prov.StartVM(ctx, h.VMName); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: %w", err)
	}

	claimCtx, cancel := context.WithTimeout(ctx, cfg.ReadyTimeout)
	defer cancel()
	claimAddr := net.JoinHostPort(h.RelayIP.String(), strconv.Itoa(cfg.ClaimPort))
	if _, err := sess.ClaimRelay(claimCtx, claimAddr, token, cfg.Request, cfg.Environment,
		cfg.Request.Link().Claim(), RefreshRelayBootFix); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: relay VM %q at %s: %w", h.VMName, h.RelayIP, err)
	}
	return h, nil
}

// create allocates the relay's address and build switch, stages and packs
// its seed, and creates the relay VM with the seed DVD, the Notes that record
// the allocation and the build NIC on the allocated switch, all under the
// host lock: the next build's allocation reads this VM's Notes and adapter.
func (h *RelayHost) create(ctx context.Context, cfg RelayHostConfig, token string) error {
	unlock, err := driver.HostLock(networkLock)
	if err != nil {
		return err
	}
	defer unlock()
	alloc, err := allocateRelay(ctx, h.prov, cfg.NATPrefix, cfg.HostIP, cfg.Slots,
		cfg.BuildSwitch)
	if err != nil {
		return err
	}
	seed := RelaySeed{IP: alloc.IP, PrefixLen: cfg.NATPrefix.Bits(), Gateway: cfg.HostIP,
		Token: token}
	if err := seed.Stage(cfg.SeedDir); err != nil {
		return err
	}
	iso := cfg.SeedDir + ".iso"
	if err := cfg.MakeSeedISO(cfg.SeedDir, iso); err != nil {
		return fmt.Errorf("relay seed ISO: %w", err)
	}
	if _, err := h.prov.CreateRelayVM(ctx, RelayVMSpec{
		Name:        h.VMName,
		BootVHDX:    cfg.BootVHDX,
		OverlayVHDX: cfg.OverlayVHDX,
		BuildSwitch: alloc.BuildSwitch,
		NATSwitch:   cfg.NATSwitch,
		COMPipePath: `\\.\pipe\` + h.VMName,
		SeedISO:     iso,
		Notes:       relayNotes(alloc.IP, alloc.BuildSwitch),
	}); err != nil {
		return err
	}
	h.RelayIP, h.BuildSwitch = alloc.IP, alloc.BuildSwitch
	return nil
}

// Close stops the collector, removes the relay VM and deletes its overlay,
// or with KeepVM leaves both.
func (h *RelayHost) Close() error {
	h.Session.Close()
	if h.keep {
		fmt.Fprintf(os.Stderr, "warden: --keep-vms: leaving relay VM %q (overlay %q, "+
			"holding %s on %s) for inspection\n", h.VMName, h.overlay, h.RelayIP,
			h.BuildSwitch)
		return nil
	}
	if h.prov != nil && h.VMName != "" {
		_ = h.prov.RemoveVM(context.Background(), h.VMName)
	}
	if h.overlay != "" {
		_ = os.Remove(h.overlay)
	}
	return nil
}

package hyperv

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// The relay VM's upstream NIC sits on the NAT switch with a static address
// (New-NetNat provides routing but no DHCP). The orchestrator claims the relay
// at that address and serves the collector back over the same host-initiated
// connection, so the host needs no listener and no inbound firewall rule. The
// address is host-only: the build VM is on the Private switch and never
// reaches it.
const (
	defaultRelayNATIP   = "192.168.240.2"
	defaultReadyTimeout = 90 * time.Second
)

// RelayHostConfig configures StartRelay.
type RelayHostConfig struct {
	BuildID     string // used to name the VM and its resources
	OutputDir   string // where the collector lands this build's outputs
	BootVHDX    string // static, read-only UKI boot disk
	OverlayVHDX string // per-build differencing overlay path (created off BootVHDX)
	BuildSwitch string // Private build switch
	NATSwitch   string // Internal NAT switch

	// Request carries the build's relay settings (capture mode, upstream CAs).
	Request *driver.BuildRequest
	// Environment is the ledger's environment record; nil records none.
	Environment *relaycfg.Environment
	// ContextDir and BuildScriptPath are the inputs the collector serves.
	ContextDir      string
	BuildScriptPath string
	// GuestOS picks the bootstrap config ("windows" or "linux").
	GuestOS string

	RelayIP      string // relay VM's static NAT address (default 192.168.240.2)
	ClaimPort    int    // default relaycfg.DefaultPort
	ReadyTimeout time.Duration
	OutputTee    io.Writer
}

// RelayHost is a running, claimed relay VM and the build's session.
type RelayHost struct {
	VMName  string
	Session *driver.Session

	prov    Provisioner
	overlay string
}

// StartRelay prepares the build's collector, boots the relay VM, and claims
// it. On any failure everything it created is torn down.
func StartRelay(ctx context.Context, prov Provisioner, cfg RelayHostConfig) (*RelayHost, error) {
	if cfg.RelayIP == "" {
		cfg.RelayIP = defaultRelayNATIP
	}
	if cfg.ClaimPort == 0 {
		cfg.ClaimPort = relaycfg.DefaultPort
	}
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
	}
	if cfg.Request == nil {
		cfg.Request = &driver.BuildRequest{UpstreamSystemCA: true}
	}
	switch {
	case cfg.BuildID == "":
		return nil, fmt.Errorf("StartRelay: empty build ID")
	case cfg.OutputDir == "":
		return nil, fmt.Errorf("StartRelay: empty output dir")
	case cfg.BootVHDX == "":
		return nil, fmt.Errorf("StartRelay: empty boot VHDX")
	case cfg.OverlayVHDX == "":
		return nil, fmt.Errorf("StartRelay: empty overlay VHDX")
	case cfg.BuildSwitch == "":
		return nil, fmt.Errorf("StartRelay: empty build switch")
	case cfg.NATSwitch == "":
		return nil, fmt.Errorf("StartRelay: empty NAT switch")
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
		VMName:  "warden-relay-" + cfg.BuildID,
		Session: sess,
		prov:    prov,
		overlay: cfg.OverlayVHDX,
	}
	if _, err := prov.CreateRelayVM(ctx, RelayVMSpec{
		Name:        h.VMName,
		BootVHDX:    cfg.BootVHDX,
		OverlayVHDX: cfg.OverlayVHDX,
		BuildSwitch: cfg.BuildSwitch,
		NATSwitch:   cfg.NATSwitch,
		COMPipePath: `\\.\pipe\` + h.VMName,
	}); err != nil {
		sess.Close()
		return nil, fmt.Errorf("StartRelay: %w", err)
	}
	if err := prov.StartVM(ctx, h.VMName); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: %w", err)
	}

	// The relay VM's command line is baked into its boot disk, so there is no
	// per-instance claim token here; the claim endpoint is only reachable on
	// the host-only NAT switch, and only until this claim lands.
	claimCtx, cancel := context.WithTimeout(ctx, cfg.ReadyTimeout)
	defer cancel()
	claimAddr := net.JoinHostPort(cfg.RelayIP, strconv.Itoa(cfg.ClaimPort))
	if _, err := sess.ClaimRelay(
		claimCtx, claimAddr, "", cfg.Request, cfg.Environment); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: relay VM %q: %w", h.VMName, err)
	}
	return h, nil
}

// Close stops the collector, removes the relay VM and deletes its overlay.
func (h *RelayHost) Close() error {
	h.Session.Close()
	if h.prov != nil && h.VMName != "" {
		_ = h.prov.RemoveVM(context.Background(), h.VMName)
	}
	if h.overlay != "" {
		_ = os.Remove(h.overlay)
	}
	return nil
}

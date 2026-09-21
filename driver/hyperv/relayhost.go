package hyperv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/buildwarden/buildwarden/collector"
	"github.com/buildwarden/buildwarden/relaycfg"
)

// Defaults for the relay host. The config port MUST match the relay init's
// RELAY_CONFIG_PORT default, since init fetches http://<gateway>:8299/config.
const (
	defaultNATHostIP     = "192.168.240.1"
	defaultConfigPort    = 8299
	defaultCollectorPort = 8390
	defaultReadyTimeout  = 90 * time.Second
)

// RelayHostConfig configures one build's relay host.
type RelayHostConfig struct {
	BuildID     string // used to name the VM and its resources
	OutputDir   string // where the collector lands this build's outputs
	BootVHDX    string // static, read-only UKI boot disk
	OverlayVHDX string // per-build differencing overlay path (created off BootVHDX)
	BuildSwitch string // Private build switch
	NATSwitch   string // Internal NAT switch

	// NATHostIP is the host's gateway address on the NAT switch, which the relay
	// VM reaches for both config and the collector. Default 192.168.240.1.
	NATHostIP string
	// ConfigPort is where the config responder listens. Default 8299 (must match
	// the relay init). CollectorPort is where the collector listens; its value
	// is embedded in the sink URL handed to the relay. Default 8390.
	ConfigPort    int
	CollectorPort int
	// ReadyTimeout bounds the wait for the relay's ready signal. Default 90s.
	ReadyTimeout time.Duration
}

// RelayHost is a running relay host: the collector + config responder on the
// host, and the relay VM they serve. The relay is up and streaming-ready by the
// time StartRelay returns. Call Close to tear it all down.
type RelayHost struct {
	VMName    string
	Token     string
	SinkURL   string
	Collector *collector.Collector

	collectorSrv *http.Server
	configSrv    *http.Server
	prov         Provisioner
	overlay      string
}

func mintToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// StartRelay wires the relay host end to end: it mints a per-build token, starts
// the collector and the config responder (both bound to the NAT gateway so only
// the relay VM on the isolated switch can reach them), creates and starts the
// relay VM, and blocks until the relay signals ready (it POSTs /v1/ready to the
// collector once it has fetched its config and come up). On any failure it tears
// down whatever it already started. The caller owns the returned RelayHost and
// must Close it when the build is done.
func StartRelay(ctx context.Context, prov Provisioner, cfg RelayHostConfig) (*RelayHost, error) {
	if cfg.NATHostIP == "" {
		cfg.NATHostIP = defaultNATHostIP
	}
	if cfg.ConfigPort == 0 {
		cfg.ConfigPort = defaultConfigPort
	}
	if cfg.CollectorPort == 0 {
		cfg.CollectorPort = defaultCollectorPort
	}
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
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

	token, err := mintToken()
	if err != nil {
		return nil, fmt.Errorf("StartRelay: minting token: %w", err)
	}
	sinkURL := "http://" + net.JoinHostPort(cfg.NATHostIP, strconv.Itoa(cfg.CollectorPort))

	// Collector: lands outputs; OnReady closes ready exactly once.
	ready := make(chan struct{})
	var once sync.Once
	coll, err := collector.New(collector.Config{
		OutputDir: cfg.OutputDir,
		Token:     token,
		OnReady:   func() { once.Do(func() { close(ready) }) },
	})
	if err != nil {
		return nil, fmt.Errorf("StartRelay: collector: %w", err)
	}

	// Config responder: serves {sinkURL, token} once, over the NAT link.
	resp, err := relaycfg.New(sinkURL, token)
	if err != nil {
		return nil, fmt.Errorf("StartRelay: config responder: %w", err)
	}

	collectorSrv, err := listenAndServe(cfg.NATHostIP, cfg.CollectorPort, coll.Handler())
	if err != nil {
		return nil, fmt.Errorf("StartRelay: collector listen: %w", err)
	}
	configSrv, err := listenAndServe(cfg.NATHostIP, cfg.ConfigPort, resp.Handler())
	if err != nil {
		_ = collectorSrv.Close()
		return nil, fmt.Errorf("StartRelay: config responder listen: %w", err)
	}

	h := &RelayHost{
		VMName:       "warden-relay-" + cfg.BuildID,
		Token:        token,
		SinkURL:      sinkURL,
		Collector:    coll,
		collectorSrv: collectorSrv,
		configSrv:    configSrv,
		prov:         prov,
		overlay:      cfg.OverlayVHDX,
	}

	// Create + start the relay VM. The responder is already serving, so the
	// relay's boot-time config fetch succeeds on the first attempt.
	if _, err := prov.CreateRelayVM(ctx, RelayVMSpec{
		Name:        h.VMName,
		BootVHDX:    cfg.BootVHDX,
		OverlayVHDX: cfg.OverlayVHDX,
		BuildSwitch: cfg.BuildSwitch,
		NATSwitch:   cfg.NATSwitch,
		COMPipePath: `\\.\pipe\` + h.VMName,
	}); err != nil {
		h.closeServers()
		return nil, fmt.Errorf("StartRelay: %w", err)
	}
	if err := prov.StartVM(ctx, h.VMName); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: %w", err)
	}

	select {
	case <-ready:
		return h, nil
	case <-time.After(cfg.ReadyTimeout):
		_ = h.Close()
		return nil, fmt.Errorf("StartRelay: relay VM %q did not signal ready within %s", h.VMName, cfg.ReadyTimeout)
	case <-ctx.Done():
		_ = h.Close()
		return nil, ctx.Err()
	}
}

// listenAndServe binds an HTTP server to host:port and serves h in the
// background, returning the server so the caller can Close it.
func listenAndServe(host string, port int, handler http.Handler) (*http.Server, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}

// closeServers shuts down the host-side HTTP servers without touching the VM.
func (h *RelayHost) closeServers() {
	if h.configSrv != nil {
		_ = h.configSrv.Close()
	}
	if h.collectorSrv != nil {
		_ = h.collectorSrv.Close()
	}
}

// Close tears the relay host down: stops the host servers, removes the relay VM,
// and deletes the per-build differencing overlay. Best-effort and idempotent.
func (h *RelayHost) Close() error {
	h.closeServers()
	if h.prov != nil && h.VMName != "" {
		_ = h.prov.RemoveVM(context.Background(), h.VMName)
	}
	if h.overlay != "" {
		_ = os.Remove(h.overlay)
	}
	return nil
}

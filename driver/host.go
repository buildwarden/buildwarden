package driver

import (
	"context"

	"github.com/buildwarden/buildwarden/relaycfg"
)

// The build dialogue as a driver's Go code sees it
// (specs/driver-contract/v1.md, The build): warden sends Start, the driver
// starts its relay and reports it with Host.ClaimRelay, starts the
// environment once warden has claimed the relay and reports it with
// Host.Started, and tears down when Host.Wait returns. A driver linked into
// warden is given warden's Host; a driver binary built on the kit is given
// one over its standard input and output.

// Start is the start message: everything one build of a driver needs, and
// nothing about the collector, which is warden's. Its fields are the
// specification's.
type Start struct {
	Contract int `json:"contract"`
	// BuildID names everything the driver creates for the build, so that
	// reap can find it again.
	BuildID string `json:"build_id,omitempty"`
	// Mode is ModeBuild or ModeShell.
	Mode string `json:"mode"`
	// ClaimToken goes on the relay's boot channel, as one of its pre-claim
	// values (RELAY-08). It is the only secret a driver is given.
	ClaimToken string `json:"claim_token,omitempty"`
	// GuestOS is the guest asked for, or the first the driver accepts.
	GuestOS string `json:"guest_os"`
	// Image is the image the user named, or empty.
	Image string `json:"image"`
	// BaseImage is the FROM image of the build's Containerfile, for a driver
	// that translates one; the driver resolves it.
	BaseImage string `json:"base_image"`
	// BuildScript is the staged build script's absolute path, which the
	// collector serves; the driver may read it to describe the environment.
	BuildScript string `json:"build_script,omitempty"`
	ContextDir  string `json:"context_dir"`
	// TempDir is where work directories are made; empty is the system's.
	TempDir string `json:"tmpdir"`
	// DiagnosticsDir is the build's output directory, where the driver may
	// write relay.log and console.log and nothing else.
	DiagnosticsDir string `json:"diagnostics_dir,omitempty"`
	// StateFile is the driver's to write for this build; attach and reap are
	// given the same path.
	StateFile string `json:"state_file,omitempty"`
	// Keep leaves what the driver's capabilities.keep names in place.
	Keep    bool `json:"keep"`
	Verbose bool `json:"verbose"`
	// VM is the shared VM settings, sent to a driver with capabilities.vm.
	VM *StartVM `json:"vm,omitempty"`
	// ConfigFile and Args are the driver's own configuration: the one config
	// file warden read, and the driver's flags as --name=value or --name.
	ConfigFile string   `json:"config_file,omitempty"`
	Args       []string `json:"args"`
	// Probe asks for the checks the driver makes before any work, and
	// nothing else.
	Probe bool `json:"probe,omitempty"`
}

// The modes of a build.
const (
	ModeBuild = "build"
	ModeShell = "shell"
)

// StartVM is the shared VM settings warden reads and owns. Zero or empty is
// not set, and the driver applies its default.
type StartVM struct {
	CPUs     int `json:"cpus"`
	MemoryMB int `json:"memory_mb"`
	// BuildLink is the relay-to-build link's /30, and BuildLinkSource where
	// it was set (--build-link, the config file, or default).
	BuildLink       string   `json:"build_link"`
	BuildLinkSource string   `json:"build_link_source"`
	DNS             []string `json:"dns"`
}

// RelayStarted is what a driver reports once its relay is up and
// unclaimed: where warden claims it, and the claim fields that are the
// driver's to give.
type RelayStarted struct {
	Claim       ClaimEndpoint `json:"claim"`
	ClaimValues ClaimValues   `json:"claim_values"`
	// GuestOS is the guest the environment really runs, which selects the
	// bootstrap config the collector serves.
	GuestOS string `json:"guest_os"`
	// OldRelayFix is the fix warden shows if the relay refuses a claim
	// field as unknown.
	OldRelayFix string `json:"old_relay_fix,omitempty"`
}

// ClaimEndpoint is where the relay accepts its claim, reachable from warden's
// process.
type ClaimEndpoint struct {
	// Network is ClaimTCP, the one value of version 1.
	Network string `json:"network"`
	Address string `json:"address"`
}

// ClaimTCP is the claim endpoint's network in version 1.
const ClaimTCP = "tcp"

// ClaimValues are copied into the claim unchanged; their shape is the claim
// protocol's.
type ClaimValues struct {
	Environment *relaycfg.Environment `json:"environment,omitempty"`
	BuildLink   *relaycfg.BuildLink   `json:"build_link,omitempty"`
	UpstreamDNS []string              `json:"upstream_dns,omitempty"`
}

// ExitedPart names what stopped on its own: the relay or the build
// environment.
type ExitedPart string

// The parts of a build that may exit on their own.
const (
	ExitedRelay ExitedPart = "relay"
	ExitedBuild ExitedPart = "build"
)

// The reasons warden gives in stop.
const (
	StopCompleted   = "completed"
	StopFailed      = "failed"
	StopInterrupted = "interrupted"
	StopTimeout     = "timeout"
	StopShellExited = "shell-exited"
)

// Host is warden's side of one build.
type Host interface {
	// ClaimRelay reports the relay as started and returns once warden has
	// claimed it and it is ready (relay-started, then claimed), or with the
	// reason the claim failed. The driver starts the environment only after
	// it returned nil.
	ClaimRelay(ctx context.Context, r RelayStarted) error
	// Started reports that the build environment is running.
	Started() error
	// Exited reports that the relay or the environment stopped on its own.
	Exited(which ExitedPart, detail string)
	// Wait returns when warden asks the driver to stop, with the reason, or
	// when ctx ends.
	Wait(ctx context.Context) string
}

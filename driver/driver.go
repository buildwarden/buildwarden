package driver

import (
	"context"
	"errors"
	"io"
	"time"
)

// Driver is the interface all orchestration backends implement.
type Driver interface {
	// Name returns a short identifier (e.g., "container", "vz", "qemu").
	Name() string

	// StartBuild provisions the environment, starts the relay, runs the
	// build script, and collects outputs.
	StartBuild(ctx context.Context, req *BuildRequest) (*BuildResult, error)

	// Exec provisions the environment and relay but opens an interactive
	// session instead of running a script. Drivers that don't support
	// interactive mode return ErrExecNotSupported.
	Exec(ctx context.Context, req *BuildRequest) error

	// Close releases any resources held by the driver.
	Close() error
}

var ErrExecNotSupported = errors.New("driver does not support interactive exec")

// BuildRequest encapsulates everything a driver needs to run a build.
type BuildRequest struct {
	// Script is the path to the build script to execute.
	Script string
	// ContextDir is the absolute path to the build context on the host.
	ContextDir string
	// Image identifies the base environment (OCI ref, IPSW URL, disk path).
	Image string
	// GuestOS selects the guest operating system family for VM drivers
	// ("linux" (default) or "windows"). Ignored by the container driver.
	GuestOS string
	// Containerfile is the path to the original Dockerfile/Containerfile.
	Containerfile string
	// OutputDir is the final output directory for the build.
	OutputDir string
	// CaptureMode controls payload capture ("none","headers","bodies","all").
	CaptureMode string
	// Compress enables zstd compression of output artifacts.
	Compress bool
	// Quiet suppresses forwarding of the guest build console to warden's stdout.
	// Output is still captured to build-output.log either way; this only governs
	// whether it is echoed live to the user.
	Quiet bool
	// Timeout is the maximum build duration. Zero means no limit.
	Timeout time.Duration
	// CPUs and MemoryMB size the build VM (VM drivers). Zero uses the
	// driver's default.
	CPUs     int
	MemoryMB int
	// Network names a pre-provisioned network to reuse instead of creating
	// one per build (hyperv: the durable switch from `warden setup`).
	Network string
	// KeepVMs leaves the build's VMs, disks and work directory in place for
	// inspection instead of tearing them down (VM drivers).
	KeepVMs bool
	// TempDir is the directory each build's private work directory is
	// created in (see NewWorkDir). Empty means the OS default (os.TempDir,
	// which honours TMPDIR / TMP). For VM-backed container runtimes it must
	// be a path the runtime's VM can see.
	TempDir string
	// RelayImage specifies the relay container image (container driver only).
	RelayImage string
	// UpstreamCACerts are host paths to additional CA certificate files/dirs
	// trusted by the relay for upstream TLS connections.
	UpstreamCACerts []string
	// UpstreamSystemCA controls whether the system cert pool is included.
	UpstreamSystemCA bool
	// Env is extra environment variables for the build.
	Env map[string]string
	// Stdin/Stdout/Stderr for build output.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// BuildResult contains outputs from a completed build.
type BuildResult struct {
	// OutputDir is the path containing the ledger, artifacts, and logs.
	OutputDir string
	// RelayLogs contains the relay's log output.
	RelayLogs []byte
}

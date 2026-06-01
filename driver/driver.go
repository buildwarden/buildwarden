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
	// Containerfile is the path to the original Dockerfile/Containerfile.
	Containerfile string
	// WardenDir is the prepared .warden directory path on host.
	WardenDir string
	// LedgerDir is where the relay writes its output.
	LedgerDir string
	// OutputDir is the final output directory for the build.
	OutputDir string
	// CaptureMode controls payload capture ("none","headers","bodies","all").
	CaptureMode string
	// Compress enables zstd compression of output artifacts.
	Compress bool
	// Timeout is the maximum build duration. Zero means no limit.
	Timeout time.Duration
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

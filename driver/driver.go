package driver

import (
	"io"
	"net/netip"
	"time"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

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
	// warden empties it for a driver whose guest cannot run one (vz).
	Containerfile string
	// BuildScript is the absolute path of the build script warden staged
	// (driver.StageBuildScript), which the collector serves. A driver reads
	// it to describe the environment and never stages one of its own.
	BuildScript string
	// BaseImage is the FROM image of Containerfile, as written there, also
	// when the build script is the user's (--script); empty when there is no
	// Containerfile, it cannot be parsed, or its FROM is scratch. The driver
	// resolves it to what it boots or runs.
	BaseImage string
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
	// ImageRefreshAfter is the age at which a prepared base image is
	// re-prepared in the background, and ImageMinGrace the least evaluation
	// grace a build may start with (VM drivers with evaluation images; zero
	// uses the driver's defaults).
	ImageRefreshAfter time.Duration
	ImageMinGrace     time.Duration
	// TempDir is the directory each build's private work directory is
	// created in (see NewWorkDir). Empty means the OS default (os.TempDir,
	// which honours TMPDIR / TMP). For VM-backed container runtimes it must
	// be a path the runtime's VM can see.
	TempDir string
	// RelayImage specifies the relay container image (container driver only).
	RelayImage string
	// UpstreamCACerts is the PEM certificates the relay trusts for upstream
	// TLS besides its own roots: the ones LoadUpstreamCAs read from the
	// configured paths before anything was created, re-encoded, and nothing
	// else. The claim carries them as they are; no file is read again.
	UpstreamCACerts []byte
	// UpstreamSystemCA controls whether the system cert pool is included.
	UpstreamSystemCA bool
	// BuildLink is the VM drivers' relay-to-build link ([vm] build_link,
	// --build-link). The zero value is the default (Link resolves it); the
	// container driver ignores it.
	BuildLink buildlink.Link
	// DNS is the relay's upstream resolvers, in order. cmd/warden sets it
	// from [vm] dns for the VM drivers; each VM driver replaces it with the
	// list it resolved ([vm] dns after its check, or the host's after its
	// selection) before claiming the relay, which takes it as upstream_dns.
	// Empty sends no field (the container driver and cmd/collector).
	DNS []netip.Addr
	// Env is extra environment variables for the build.
	Env map[string]string
	// Stdin/Stdout/Stderr for build output.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Link is the request's build link, the default when none was resolved.
func (r *BuildRequest) Link() buildlink.Link { return r.BuildLink.OrDefault() }

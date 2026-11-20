package main

import (
	"context"
	"io"
	"os"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/container"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

// The drivers linked into warden, until each becomes a binary (K2; the
// container driver in K1). This is the one file besides their doctor, setup,
// image and clean code that names them. A linked driver takes part in the
// same build sequence as a driver binary (buildseq.go), with the messages as
// Go calls.

// capabilities is what a driver's describe reports under capabilities
// (specs/driver-contract/v1.md, describe), as warden uses it.
type capabilities struct {
	// VM: it boots a build machine, takes start.vm and the VM flags.
	VM bool
	// Shell: run takes mode shell, and attach is implemented.
	Shell bool
	// Keep is what --keep-vms leaves: running, stopped or none.
	Keep string
	// Exited: it reports a relay or environment that stopped on its own.
	Exited bool
	// Containerfile is translate or ignore.
	Containerfile string
	// GuestOS are the --guest-os values it accepts; the first is the default.
	GuestOS []string
}

// The values of capabilities.containerfile and capabilities.keep.
const (
	containerfileTranslate = "translate"
	containerfileIgnore    = "ignore"
	keepRunning            = "running"
	keepStopped            = "stopped"
	keepNone               = "none"
)

// linkedDriver is a driver linked into warden: its capabilities, and the
// functions the build sequence calls. cfg and req are warden's configuration
// and request for the build, which carry what a linked driver reads beside
// the start message until it reads its own settings through the kit (K1,
// Step 6): its flags and keys, and the streams it prints to.
type linkedDriver struct {
	Capabilities capabilities
	// Commands are the contract's commands warden serves for the driver, as
	// its describe lists them; ImageOS the image prepare's guests, the first
	// the default, for a driver that lists image-prepare.
	Commands []string
	ImageOS  []string
	// Settings are the driver's flags and keys, its package's declarations.
	Settings []drivercmd.Setting
	// Preflight is what a probe of run calls: the checks the driver makes
	// before anything is created. It creates nothing. Nil makes no checks.
	Preflight func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
		start *driver.Start) error
	// Run is the driver's run: it starts the relay, reports it through
	// host, starts the environment once host has claimed it, and tears down
	// when host.Wait returns.
	Run func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
		start *driver.Start, host driver.Host) (kept []string, err error)
	// Attach enters the held environment of build buildID, which a Run in
	// mode shell reported started, on the request's terminal, and returns
	// the shell's exit code; an error is the attach's own failure. Nil for a
	// driver without capabilities.shell.
	Attach func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
		buildID string) (code int, err error)
}

// The commands warden serves for the linked drivers: the container driver's,
// and the VM drivers', whose doctor, setup, image and clean code stays in
// warden in K1.
var (
	containerCommands = []string{drivercmd.CmdRun, drivercmd.CmdReap, drivercmd.CmdDoctor,
		drivercmd.CmdClean, drivercmd.CmdAttach}
	vmCommands = []string{drivercmd.CmdRun, drivercmd.CmdReap, drivercmd.CmdDoctor,
		drivercmd.CmdSetup, drivercmd.CmdImagePrepare, drivercmd.CmdImageList,
		drivercmd.CmdImageVerify, drivercmd.CmdClean}
)

// description is the describe of the linked driver called name, built by
// the kit from its declarations as a driver binary's is.
func (l *linkedDriver) description(name string) (*drivercmd.Description, error) {
	c := l.Capabilities
	caps := drivercmd.Capabilities{VM: c.VM, Shell: c.Shell, Keep: c.Keep,
		Exited: c.Exited, Containerfile: c.Containerfile, GuestOS: c.GuestOS,
		ImageOS: l.ImageOS}
	if len(l.ImageOS) > 0 {
		caps.DefaultImageOS = l.ImageOS[0]
	}
	d := &drivercmd.Driver{Describe: drivercmd.Describe{Name: name, Version: version,
		Vendor: "buildwarden", BugURL: bugURL, Capabilities: caps,
		Settings: l.Settings, Commands: l.Commands}}
	return d.Description()
}

// linkedDrivers are the drivers linked into warden, by name.
var linkedDrivers = map[string]*linkedDriver{
	"container": {Capabilities: capabilities{Shell: true, Keep: keepNone, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux"}},
		Commands: containerCommands, Settings: container.Settings,
		Preflight: containerPreflight, Run: containerRun, Attach: containerAttach},
	"qemu": {Capabilities: capabilities{VM: true, Keep: keepStopped,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux", "windows"}},
		Commands: vmCommands, ImageOS: []string{"linux", "windows"},
		Settings: qemu.Settings,
		Preflight: func(_ context.Context, _ *Config, req *driver.BuildRequest,
			_ *driver.Start) error {
			return qemu.New().Preflight(req)
		},
		Run: func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
			start *driver.Start, host driver.Host) ([]string, error) {
			d := qemu.New()
			d.Verbose = cfg.Output.Verbose
			return d.Run(ctx, req, start, host)
		}},
	"vz": {Capabilities: capabilities{VM: true, Keep: keepStopped,
		Containerfile: containerfileIgnore, GuestOS: []string{"linux"}},
		Commands: vmCommands, ImageOS: []string{"macos"}, Settings: vz.Settings,
		Preflight: func(_ context.Context, _ *Config, req *driver.BuildRequest,
			_ *driver.Start) error {
			return vz.New().Preflight(req)
		},
		Run: func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
			start *driver.Start, host driver.Host) ([]string, error) {
			d := vz.New()
			d.Verbose = cfg.Output.Verbose
			return d.Run(ctx, req, start, host)
		}},
	"hyperv": {Capabilities: capabilities{VM: true, Keep: keepRunning, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux", "windows"}},
		Commands: vmCommands, ImageOS: []string{"windows"}, Settings: hyperv.Settings,
		Preflight: func(_ context.Context, _ *Config, req *driver.BuildRequest,
			_ *driver.Start) error {
			return hyperv.New().Preflight(req)
		},
		Run: func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
			start *driver.Start, host driver.Host) ([]string, error) {
			d := hyperv.New()
			d.Verbose = cfg.Output.Verbose
			return d.Run(ctx, req, start, host)
		}},
	"firecracker": {Capabilities: capabilities{VM: true, Keep: keepRunning, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux"}},
		Commands: vmCommands, ImageOS: []string{"linux"}, Settings: firecracker.Settings,
		Preflight: func(_ context.Context, _ *Config, req *driver.BuildRequest,
			_ *driver.Start) error {
			return firecracker.New().Preflight(req)
		},
		Run: func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
			start *driver.Start, host driver.Host) ([]string, error) {
			d := firecracker.New()
			d.Verbose = cfg.Output.Verbose
			return d.Run(ctx, req, start, host)
		}},
}

// containerPreflight is the container driver's probe: it resolves the
// container runtime (the configured one, else detection) and refuses one
// that is not supported or does not answer, before anything is created.
func containerPreflight(_ context.Context, cfg *Config, _ *driver.BuildRequest,
	_ *driver.Start) error {
	_, err := resolveRuntime(cfg)
	return err
}

// containerRun is the container driver's run. It resolves the runtime once
// more, as a build of a driver binary does after its probe.
func containerRun(ctx context.Context, cfg *Config, req *driver.BuildRequest,
	start *driver.Start, host driver.Host) ([]string, error) {
	d, err := containerDriver(cfg)
	if err != nil {
		return nil, err
	}
	return d.Run(ctx, req, start, host)
}

// containerAttach is the container driver's attach, on the runtime it
// resolves, with the request's streams, else the process's own.
func containerAttach(_ context.Context, cfg *Config, req *driver.BuildRequest,
	buildID string) (int, error) {
	d, err := containerDriver(cfg)
	if err != nil {
		return 0, err
	}
	var stdin io.Reader = os.Stdin
	var stdout, stderr io.Writer = os.Stdout, os.Stderr
	if req.Stdin != nil {
		stdin = req.Stdin
	}
	if req.Stdout != nil {
		stdout = req.Stdout
	}
	if req.Stderr != nil {
		stderr = req.Stderr
	}
	return d.Attach(buildID, stdin, stdout, stderr)
}

// containerDriver is the container driver for cfg, on the runtime it
// resolves.
func containerDriver(cfg *Config) (*container.Driver, error) {
	runtime, err := resolveRuntime(cfg)
	if err != nil {
		return nil, err
	}
	return &container.Driver{Runtime: runtime, Verbose: cfg.Output.Verbose,
		Version: version}, nil
}

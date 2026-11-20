package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/drivercmd"
	"github.com/buildwarden/buildwarden/driver/firecracker"
	"github.com/buildwarden/buildwarden/driver/hyperv"
	"github.com/buildwarden/buildwarden/driver/qemu"
	"github.com/buildwarden/buildwarden/driver/vz"
)

// The four VM drivers, linked into warden until each becomes a binary (K2).
// This is the one file besides their doctor, setup, image and clean code
// that names them. A linked driver takes part in the
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

// vmCommands are the commands warden serves for the linked VM drivers, whose
// doctor, setup, image and clean code stays in warden in K1.
var vmCommands = []string{drivercmd.CmdRun, drivercmd.CmdReap, drivercmd.CmdDoctor,
	drivercmd.CmdSetup, drivercmd.CmdImagePrepare, drivercmd.CmdImageList,
	drivercmd.CmdImageVerify, drivercmd.CmdClean}

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

// linkedSettings resolves the selected linked driver's settings for command
// with the kit, from this invocation's driver flags and config file, as a
// driver binary resolves them from its request, and prints their old-name
// warnings: warden calls it for a command it does not probe first, and for
// clean where a driver binary's clean is probed (DRV-C11).
func linkedSettings(cfg *Config, sd *selectedDriver, command string) (driver.Settings,
	error) {
	set, err := drivercmd.Resolve(sd.name(), sd.linked.Settings, command,
		driverFlags.argsFor(command), absConfigFile(cfg), os.Environ())
	if err != nil {
		return nil, err
	}
	for _, w := range set.Warnings() {
		fmt.Fprintln(os.Stderr, w)
	}
	return set, nil
}

// kit is the linked driver called name as the kit runs a driver: its run is
// called through drivercmd.Call, in warden's process, which resolves its
// settings from the start message and prints their warnings in the probe.
// The settings a linked driver's code still takes in the build request are
// put there before its Preflight and Run are called.
func (l *linkedDriver) kit(name string, cfg *Config,
	req *driver.BuildRequest) *drivercmd.Driver {
	c := l.Capabilities
	return &drivercmd.Driver{
		Describe: drivercmd.Describe{Name: name, Settings: l.Settings,
			Commands: l.Commands, Capabilities: drivercmd.Capabilities{VM: c.VM,
				Shell: c.Shell, Keep: c.Keep, Exited: c.Exited,
				Containerfile: c.Containerfile, GuestOS: c.GuestOS,
				ImageOS: l.ImageOS}},
		Preflight: func(ctx context.Context, start *driver.Start) error {
			settingsToRequest(start.Settings, req)
			if l.Preflight == nil {
				return nil
			}
			return l.Preflight(ctx, cfg, req, start)
		},
		Run: func(ctx context.Context, start *driver.Start, host driver.Host) ([]string,
			error) {
			settingsToRequest(start.Settings, req)
			// A background image prepare the run starts reads start's file.
			driver.ConfigFile = start.ConfigFile
			return l.Run(ctx, cfg, req, start, host)
		},
	}
}

// settingsToRequest puts the linked drivers' settings that their code takes
// in the build request there: the ages of qemu and hyperv, and hyperv's
// network. A setting the driver does not declare is not set, and leaves the
// field at its default.
func settingsToRequest(set driver.Settings, req *driver.BuildRequest) {
	if v, ok := set.Age("image-refresh-after"); ok {
		req.ImageRefreshAfter = v
	}
	if v, ok := set.Age("image-min-grace"); ok {
		req.ImageMinGrace = v
	}
	if v, ok := set.String("network"); ok {
		req.Network = v
	}
}

// callLinkedRun runs the linked driver's run through the kit: a probe when
// start.Probe is set, with host nil, else the build with warden's host.
func callLinkedRun(ctx context.Context, l *linkedDriver, name string, cfg *Config,
	req *driver.BuildRequest, start *driver.Start, host driver.Host) ([]string, error) {
	res, err := drivercmd.Call(ctx, l.kit(name, cfg, req), drivercmd.CmdRun,
		&drivercmd.Request{Start: start, Environ: os.Environ()}, host)
	if err != nil {
		return nil, err
	}
	return res.Kept, nil
}

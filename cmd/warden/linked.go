package main

import (
	"context"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/container"
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
	// Preflight is what a probe of run calls: the checks the driver makes
	// before anything is created. It creates nothing. Nil makes no checks.
	Preflight func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
		start *driver.Start) error
	// Run is the driver's run: it starts the relay, reports it through
	// host, starts the environment once host has claimed it, and tears down
	// when host.Wait returns. Nil while the driver still builds through
	// driver.Driver's StartBuild.
	Run func(ctx context.Context, cfg *Config, req *driver.BuildRequest,
		start *driver.Start, host driver.Host) (kept []string, err error)
}

// linkedDrivers are the drivers linked into warden, by name.
var linkedDrivers = map[string]*linkedDriver{
	"container": {Capabilities: capabilities{Shell: true, Keep: keepNone, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux"}},
		Preflight: containerPreflight, Run: containerRun},
	"qemu": {Capabilities: capabilities{VM: true, Keep: keepStopped,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux", "windows"}},
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

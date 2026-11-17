package main

import (
	"context"

	"github.com/buildwarden/buildwarden/driver"
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
// functions the build sequence calls. req is warden's request for the build,
// which carries what a linked driver reads beside the start message until it
// reads its own settings through the kit (K1, Step 6): its flags and keys,
// and the streams it prints to.
type linkedDriver struct {
	Capabilities capabilities
	// Preflight is what a probe of run calls: the checks the driver makes
	// before anything is created. It creates nothing. Nil makes no checks.
	Preflight func(ctx context.Context, req *driver.BuildRequest, start *driver.Start) error
	// Run is the driver's run: it starts the relay, reports it through
	// host, starts the environment once host has claimed it, and tears down
	// when host.Wait returns. Nil while the driver still builds through
	// driver.Driver's StartBuild.
	Run func(ctx context.Context, req *driver.BuildRequest, start *driver.Start,
		host driver.Host) (kept []string, err error)
}

// linkedDrivers are the drivers linked into warden, by name.
var linkedDrivers = map[string]*linkedDriver{
	"container": {Capabilities: capabilities{Shell: true, Keep: keepNone, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux"}}},
	"qemu": {Capabilities: capabilities{VM: true, Keep: keepStopped,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux", "windows"}}},
	"vz": {Capabilities: capabilities{VM: true, Keep: keepStopped,
		Containerfile: containerfileIgnore, GuestOS: []string{"linux"}}},
	"hyperv": {Capabilities: capabilities{VM: true, Keep: keepRunning, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux", "windows"}}},
	"firecracker": {Capabilities: capabilities{VM: true, Keep: keepRunning, Exited: true,
		Containerfile: containerfileTranslate, GuestOS: []string{"linux"}}},
}

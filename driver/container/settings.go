package container

import "github.com/buildwarden/buildwarden/driver/drivercmd"

// Settings are the container driver's flags and keys
// (specs/driver-contract/v1.md, Configuration): the runtime and the relay
// image, each under its key in [driver.container] and its old name in
// [runtime], read until 1.0, with the two variables that set them.
var Settings = []drivercmd.Setting{
	{Name: "runtime", Type: drivercmd.String, Flag: "runtime",
		Key: "driver.container.cli", Aliases: []string{"runtime.cli"},
		Env: "WARDEN_CTR_CLI", Commands: drivercmd.EveryCommand,
		Usage: "container runtime (finch, docker, podman)"},
	{Name: "relay-image", Type: drivercmd.String,
		Key:     "driver.container.relay_image",
		Aliases: []string{"runtime.relay_image"}, Env: "WARDEN_IMAGE",
		Commands: []string{drivercmd.CmdRun},
		Usage:    "relay image reference, or dev to build it from source"},
}

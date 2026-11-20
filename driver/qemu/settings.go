package qemu

import "github.com/buildwarden/buildwarden/driver/drivercmd"

// Settings are the qemu driver's flags and keys
// (specs/driver-contract/v1.md, Configuration): the two common keys, read in
// [driver.qemu] and then in [driver], each with its old name in [vm] read
// until 1.0, and image prepare's flags for Windows media.
var Settings = []drivercmd.Setting{
	{Name: "image-refresh-after", Type: drivercmd.Age,
		Common: "image_refresh_after", Aliases: []string{"vm.image_refresh_after"},
		Commands: []string{drivercmd.CmdRun},
		Usage:    "age at which a prepared Windows layer is refreshed in the background"},
	{Name: "image-min-grace", Type: drivercmd.Age,
		Common: "image_min_grace", Aliases: []string{"vm.image_min_grace"},
		Commands: []string{drivercmd.CmdRun},
		Usage:    "least evaluation grace a prepared Windows layer must still have"},
	{Name: "force", Type: drivercmd.Bool, Flag: "force",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "re-acquire even if a valid cached copy exists"},
	{Name: "edition", Type: drivercmd.String, Flag: "edition",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "Windows edition / install.wim image name (windows)"},
	{Name: "virtio", Type: drivercmd.String, Flag: "virtio",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "virtio-win driver ISO path or URL (windows)"},
	{Name: "arch", Type: drivercmd.String, Flag: "arch",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "guest arch: arm64 or amd64 (default host arch)"},
	{Name: "keep-vms", Type: drivercmd.Bool, Flag: "keep-vms",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "leave the prepare VM and its disk for inspection"},
}

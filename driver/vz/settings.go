package vz

import "github.com/buildwarden/buildwarden/driver/drivercmd"

// Settings are the vz driver's flags (specs/driver-contract/v1.md,
// Configuration): image prepare's --update. It reads no key.
var Settings = []drivercmd.Setting{
	{Name: "update", Type: drivercmd.Bool, Flag: "update",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage: "move to the newest supported OS release: restore it, or refresh " +
			"it when already prepared"},
}

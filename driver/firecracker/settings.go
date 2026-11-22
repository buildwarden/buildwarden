package firecracker

import (
	"fmt"

	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// Settings are the firecracker driver's flags (specs/driver-contract/v1.md,
// Configuration): setup's --slots. It reads no key.
var Settings = []drivercmd.Setting{
	{Name: "slots", Type: drivercmd.Int, Flag: "slots",
		Commands: []string{drivercmd.CmdSetup},
		Usage: fmt.Sprintf("network slots to create, 1-%d (default %d)",
			MaxSlots, DefaultSlots)},
}

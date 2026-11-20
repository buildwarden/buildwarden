package hyperv

import (
	"fmt"

	"github.com/buildwarden/buildwarden/driver/drivercmd"
)

// Settings are the hyperv driver's flags and keys
// (specs/driver-contract/v1.md, Configuration): the two common keys, read in
// [driver.hyperv] and then in [driver]; the durable switch and the Windows
// base image in [driver.hyperv]; each key with its old name in [vm], read
// until 1.0; and the flags of setup and image prepare.
var Settings = []drivercmd.Setting{
	{Name: "image-refresh-after", Type: drivercmd.Age,
		Common: "image_refresh_after", Aliases: []string{"vm.image_refresh_after"},
		Commands: []string{drivercmd.CmdRun},
		Usage:    "age at which a prepared Windows image is refreshed in the background"},
	{Name: "image-min-grace", Type: drivercmd.Age,
		Common: "image_min_grace", Aliases: []string{"vm.image_min_grace"},
		Commands: []string{drivercmd.CmdRun},
		Usage:    "least evaluation grace a prepared Windows image must still have"},
	{Name: "network", Type: drivercmd.String, Flag: "network",
		Key: "driver.hyperv.network", Aliases: []string{"vm.network"},
		Commands: []string{drivercmd.CmdRun, drivercmd.CmdDoctor, drivercmd.CmdSetup},
		Usage: "durable build switch name " +
			"(default: [driver.hyperv] network, then warden-dev)"},
	{Name: "windows-image", Type: drivercmd.String,
		Key: "driver.hyperv.windows_image", Aliases: []string{"vm.windows_image"},
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage: "Windows base image, a URL or local path, for image prepare " +
			"with no source"},
	{Name: "windows-image-update", Type: drivercmd.String,
		Key:      "driver.hyperv.windows_image_update",
		Aliases:  []string{"vm.windows_image_update"},
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "auto (pick up new media at that URL) or reuse (keep the current base)"},
	{Name: "nat-prefix", Type: drivercmd.String, Flag: "nat-prefix",
		Commands: []string{drivercmd.CmdSetup},
		Usage: "NAT CIDR for relay egress, used by every build (default " +
			DefaultNATPrefix + ")"},
	{Name: "host-ip", Type: drivercmd.String, Flag: "host-ip",
		Commands: []string{drivercmd.CmdSetup},
		Usage: "host gateway IP on the NAT switch, used by every build (default " +
			DefaultHostIP + ")"},
	{Name: "slots", Type: drivercmd.Int, Flag: "slots",
		Commands: []string{drivercmd.CmdSetup},
		Usage: fmt.Sprintf("build switches to create, 1-%d (default %d)", MaxSlots,
			DefaultSlots)},
	{Name: "sha256", Type: drivercmd.String, Flag: "sha256",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "expected SHA256; verified and pinned"},
	{Name: "force", Type: drivercmd.Bool, Flag: "force",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "re-acquire even if a valid cached copy exists"},
	{Name: "name", Type: drivercmd.String, Flag: "name",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "registry name of the image (default windows-server-2025)"},
	{Name: "keep-vms", Type: drivercmd.Bool, Flag: "keep-vms",
		Commands: []string{drivercmd.CmdImagePrepare},
		Usage:    "leave the prepare VM and its disk for inspection"},
}

// Package relaysource names the relay a driver runs for a build: its release
// form and the roles of its files. It is a kit library and not part of the
// driver contract: a driver from outside finds the project's relay however
// it likes, and warden never sees one.
//
// The form and role names are provisional. They are this repository's own
// until seam 8, the relay release forms, is written, and may change with it.
package relaysource

// RelayForm names a relay release form (seam 8, provisional).
type RelayForm string

// The relay release forms.
const (
	// RelayHostBinary is a relay executable run on the host in host mode.
	RelayHostBinary RelayForm = "host-binary"
	// RelayOCIImage is a container image run in container mode.
	RelayOCIImage RelayForm = "oci-image"
	// RelayAppliance is a Linux relay VM on the kernel network path: a
	// kernel and an initramfs, or one boot disk.
	RelayAppliance RelayForm = "appliance"
	// RelayUnikernel is a single bootable unikernel image. No driver runs
	// one yet; the name is reserved for M6.
	RelayUnikernel RelayForm = "unikernel"
)

// RelayRole names a file's part in a form (provisional).
type RelayRole string

// The roles of a relay's files.
const (
	// RelayBinary is the host-binary's executable; or, in an appliance, a
	// relay to append to the initramfs at /sbin/relay.
	RelayBinary RelayRole = "binary"
	// RelayKernel is an appliance's or a unikernel's kernel.
	RelayKernel RelayRole = "kernel"
	// RelayInitramfs is an appliance's initramfs.
	RelayInitramfs RelayRole = "initramfs"
	// RelayDisk is an appliance as one boot disk.
	RelayDisk RelayRole = "disk"
)

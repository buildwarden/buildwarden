// Package relayvm is the relay VM lifecycle shared by the VM drivers that boot
// the Alpine relay initramfs by direct kernel boot (qemu, firecracker): finding
// the boot assets and composing the per-build relay initrd.
package relayvm

import (
	"os"
	"path/filepath"

	"github.com/buildwarden/buildwarden/driver"
)

// Asset is one VM boot file, found either beside the warden binary under
// Release or in the source tree's tools output at Source (slash-separated,
// relative to the module root).
type Asset struct {
	Release string
	Source  string
}

// QEMURelay is the qemu relay VM's kernel and base initramfs.
var QEMURelay = []Asset{
	{Release: "relay-vm-vmlinuz", Source: "tools/relay-vm/output/vmlinuz"},
	{Release: "relay-vm-initramfs.cpio.gz", Source: "tools/relay-vm/output/initramfs.cpio.gz"},
}

// Firecracker is the Firecracker driver's boot assets, in order: the pinned
// x86_64 guest kernel both VMs boot, the x86_64 relay base initramfs, and the
// direct-boot base initramfs (launcher only).
var Firecracker = []Asset{
	{Release: "firecracker-vmlinux", Source: "tools/firecracker-vm/output/vmlinux"},
	{Release: "firecracker-relay-initramfs.cpio.gz",
		Source: "tools/relay-vm/output/x86_64/initramfs.cpio.gz"},
	{Release: "firecracker-build-initramfs.cpio.gz",
		Source: "tools/firecracker-vm/output/initramfs.cpio.gz"},
}

// Find returns the path of every asset, in order, all from beside the warden
// binary or all from the source tree, so one build never mixes the two. ok is
// false when neither place has them all.
func Find(assets []Asset) (paths []string, ok bool) {
	var dirs []func(Asset) string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		dirs = append(dirs, func(a Asset) string { return filepath.Join(dir, a.Release) })
	}
	if root, err := driver.ModuleRoot(); err == nil {
		dirs = append(dirs, func(a Asset) string {
			return filepath.Join(root, filepath.FromSlash(a.Source))
		})
	}
	for _, at := range dirs {
		if paths, ok := findIn(assets, at); ok {
			return paths, true
		}
	}
	return nil, false
}

func findIn(assets []Asset, at func(Asset) string) ([]string, bool) {
	paths := make([]string, len(assets))
	for i, a := range assets {
		paths[i] = at(a)
		if _, err := os.Stat(paths[i]); err != nil {
			return nil, false
		}
	}
	return paths, true
}

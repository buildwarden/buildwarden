//go:build linux

package firecracker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
	"github.com/buildwarden/buildwarden/driver/buildlink"
	"github.com/buildwarden/buildwarden/driver/linuxguest"
	"github.com/buildwarden/buildwarden/driver/relayvm"
)

// delivery is a build VM ready to boot: how it boots, what its record names,
// and the per-build rootfs directory teardown removes.
type delivery struct {
	boot      buildBoot
	record    buildRecord
	rootfsDir string
}

// deliver puts warden-io and its launcher in front of the build VM, per build
// (BOOT-02, BOOT-03): on the disk path, a reflink copy of the raw image and
// the qemu Linux-guest CIDATA seed ending in `reboot -f`; on direct boot,
// warden-io appended to the launcher-only initramfs. Nothing else crosses;
// trust, the script and the context come from the relay.
func deliver(a assets, image, workDir, buildID, script string,
	link buildlink.Link) (*delivery, error) {
	d := &delivery{
		boot:   buildBoot{Kernel: a.Kernel},
		record: buildRecord{Kernel: a.Kernel, Script: script},
	}
	if image != "" {
		if err := d.diskPath(image, workDir, buildID, link); err != nil {
			d.remove()
			return nil, err
		}
	} else if err := d.directBoot(a, workDir); err != nil {
		return nil, err
	} else {
		d.boot.LinkArgs = link.BootArgs()
	}
	cmdline := filepath.Join(workDir, seedKernelCmdline)
	args := buildConfig(0, d.boot, 1, 1).effectiveArgs()
	if err := os.WriteFile(cmdline, []byte(args+"\n"), 0o600); err != nil {
		d.remove()
		return nil, err
	}
	d.record.Cmdline = cmdline
	return d, nil
}

func (d *delivery) diskPath(image, workDir, buildID string, link buildlink.Link) error {
	raw, err := EnsureRaw(image)
	if err != nil {
		return err
	}
	partUUID, err := rootPartUUID(raw)
	if err != nil {
		return err
	}
	d.rootfsDir = filepath.Join(WorkRoot(), buildID)
	if err := os.MkdirAll(d.rootfsDir, 0o700); err != nil {
		return err
	}
	rootfs := filepath.Join(d.rootfsDir, "rootfs.raw")
	if out, err := exec.Command("cp", "--reflink=auto", raw,
		rootfs).CombinedOutput(); err != nil {
		return fmt.Errorf("copying the build rootfs: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	seedDir, err := linuxguest.Seed(workDir, "amd64", linuxguest.Reboot, link)
	if err != nil {
		return fmt.Errorf("generating cloud-init: %w", err)
	}
	seedISO := filepath.Join(workDir, "seed.iso")
	if err := linuxguest.GenerateSeedISO(seedDir, seedISO, "CIDATA"); err != nil {
		return fmt.Errorf("generating seed ISO: %w", err)
	}
	d.boot.Rootfs, d.boot.PartUUID, d.boot.Seed = rootfs, partUUID, seedISO
	d.record.Raw, d.record.SeedDir = raw, seedDir
	return nil
}

func (d *delivery) directBoot(a assets, workDir string) error {
	agent, err := driver.GuestBinary("warden-io", "linux", "amd64")
	if err != nil {
		return err
	}
	initrd := filepath.Join(workDir, "build-initrd")
	if err := relayvm.AppendFileCPIO(a.BuildInitramfs, initrd, agent,
		"bin/warden-io"); err != nil {
		return fmt.Errorf("composing the build initrd: %w", err)
	}
	d.boot.Initrd = initrd
	d.record.Initrd, d.record.Agent = a.BuildInitramfs, agent
	return nil
}

// remove deletes the per-build rootfs copy (ORCH-04).
func (d *delivery) remove() {
	if d.rootfsDir != "" {
		_ = os.RemoveAll(d.rootfsDir)
	}
}

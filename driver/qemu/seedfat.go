package qemu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// generateWindowsSeedFAT builds a raw FAT16 filesystem image (superfloppy, no
// partition table) from a directory, with the given volume label. Windows
// reads FAT16 natively via usb-storage with exact long filenames and assigns
// it a drive letter, unlike our custom/level-1 ISO9660 seed which Windows
// CDFS reports as FileSystemType=Unknown (no drive letter -> the startup
// task's `& ($v.DriveLetter + ":\warden-run.ps1")` resolves to nothing).
//
// macOS uses hdiutil (create FAT superfloppy -> attach -> copy -> detach ->
// convert to raw); Linux uses mtools (mformat/mcopy) when present.
func generateWindowsSeedFAT(dir, outPath, label string) error {
	if _, err := exec.LookPath("hdiutil"); err == nil {
		return generateWindowsSeedFATmacOS(dir, outPath, label)
	}
	if _, err := exec.LookPath("mformat"); err == nil {
		return generateWindowsSeedFATmtools(dir, outPath, label)
	}
	return fmt.Errorf(
		"no FAT tool available (need hdiutil on macOS or mtools on Linux)")
}

func generateWindowsSeedFATmacOS(dir, outPath, label string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	work := outPath + ".build"
	dmg := work + ".dmg"
	_ = os.Remove(dmg)
	defer os.Remove(dmg)

	// 16 MiB is ample for warden-io.exe (~5 MiB) + warden-run.ps1.
	// MBRSPUD = MBR partition table with one FAT16 partition. Windows mounts
	// the partition on a usb-storage disk and assigns a drive letter; a
	// -layout NONE superfloppy (no partition table) is NOT mounted on a
	// fixed/usb disk (Windows sees a raw uninitialized disk, no volume).
	create := exec.Command("hdiutil", "create",
		"-megabytes", "16", "-fs", "MS-DOS FAT16",
		"-volname", label, "-layout", "MBRSPUD", "-ov", dmg)
	if out, err := create.CombinedOutput(); err != nil {
		return fmt.Errorf("hdiutil create FAT: %s: %w", string(out), err)
	}

	attach := exec.Command("hdiutil", "attach", dmg, "-nobrowse")
	out, err := attach.Output()
	if err != nil {
		return fmt.Errorf("hdiutil attach: %w", err)
	}
	dev, mount := parseHdiutilAttach(string(out))
	if mount == "" {
		return fmt.Errorf("could not find FAT mountpoint in: %s", string(out))
	}
	detached := false
	detach := func() {
		if detached {
			return
		}
		_ = exec.Command("hdiutil", "detach", dev).Run()
		detached = true
	}
	defer detach()

	// Copy files with plain byte reads/writes (no cp, so no AppleDouble
	// ._ sidecars), then strip any macOS metadata the mount created.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(
			filepath.Join(mount, e.Name()), data, 0644); err != nil {
			return fmt.Errorf("copying %s to FAT seed: %w", e.Name(), err)
		}
	}
	stripMacOSCruft(mount)
	detach()

	// Convert the (UDIF) image to a raw disk image qemu attaches as format=raw.
	conv := exec.Command("hdiutil", "convert", dmg,
		"-format", "UDTO", "-ov", "-o", outPath)
	if out, err := conv.CombinedOutput(); err != nil {
		return fmt.Errorf("hdiutil convert to raw: %s: %w", string(out), err)
	}
	// hdiutil convert -format UDTO appends .cdr; move it into place.
	if err := os.Rename(outPath+".cdr", outPath); err != nil {
		return fmt.Errorf("finalizing raw seed: %w", err)
	}
	return nil
}

// parseHdiutilAttach extracts the whole-disk device node and the FAT
// mountpoint from `hdiutil attach` output. With an MBR layout the output has
// two device lines (whole disk /dev/diskN and partition /dev/diskNsM); we
// return the whole-disk node so detach releases the entire image, and the
// mountpoint from the partition line (which may contain spaces).
func parseHdiutilAttach(out string) (dev, mount string) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "/dev/") {
			continue
		}
		if dev == "" {
			dev = fields[0] // first device line == whole disk
		}
		if i := strings.Index(line, "/Volumes/"); i >= 0 && mount == "" {
			mount = strings.TrimSpace(line[i:])
		}
	}
	return dev, mount
}

// stripMacOSCruft removes AppleDouble sidecars and metadata directories that a
// mounted macOS volume accretes, so the seed contains only the intended files.
func stripMacOSCruft(mount string) {
	for _, d := range []string{
		".fseventsd", ".Spotlight-V100", ".Trashes", ".TemporaryItems",
	} {
		_ = os.RemoveAll(filepath.Join(mount, d))
	}
	if matches, err := filepath.Glob(filepath.Join(mount, "._*")); err == nil {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}
}

// generateWindowsSeedFATmtools builds the FAT seed with mtools on a Linux
// host. NOTE: mformat writes a bare (superfloppy) FAT with no partition table,
// which Windows will NOT mount on a usb-storage disk (it needs an MBR
// partition, as the macOS path produces via MBRSPUD). This path is a
// placeholder for a future Linux qemu host and needs an mpartition/MBR step
// before it is Windows-usable; the current target (macOS) uses hdiutil above.
func generateWindowsSeedFATmtools(dir, outPath, label string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// 16 MiB raw image.
	const size = 16 * 1024 * 1024
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}
	f.Close()

	mformat := exec.Command("mformat", "-i", outPath, "-F", "-v", label, "::")
	if out, err := mformat.CombinedOutput(); err != nil {
		return fmt.Errorf("mformat: %s: %w", string(out), err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(dir, e.Name())
		mcopy := exec.Command("mcopy", "-i", outPath, src, "::"+e.Name())
		if out, err := mcopy.CombinedOutput(); err != nil {
			return fmt.Errorf("mcopy %s: %s: %w", e.Name(), string(out), err)
		}
	}
	return nil
}

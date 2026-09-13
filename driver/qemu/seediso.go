package qemu

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// generateSeedISO creates an ISO9660 image from a directory with the given
// volume label ("CIDATA" for cloud-init NoCloud detection; "WARDEN" for the
// Windows provisioning seed).
// Uses platform tools (hdiutil on macOS, mkisofs/genisoimage on Linux)
// with a pure-Go fallback.
func generateSeedISO(dir, outPath, volumeID string) error {
	if err := generateSeedISOExternal(dir, outPath, volumeID); err == nil {
		return nil
	}
	return generateSeedISOBuiltin(dir, outPath, volumeID)
}

func generateSeedISOExternal(dir, outPath, volumeID string) error {
	// macOS: hdiutil
	if _, err := exec.LookPath("hdiutil"); err == nil {
		cmd := exec.Command("hdiutil", "makehybrid",
			"-o", outPath,
			"-joliet", "-iso",
			"-default-volume-name", volumeID,
			dir)
		return cmd.Run()
	}
	// Linux: mkisofs or genisoimage
	for _, tool := range []string{"mkisofs", "genisoimage"} {
		if _, err := exec.LookPath(tool); err == nil {
			cmd := exec.Command(tool,
				"-output", outPath,
				"-volid", volumeID,
				"-joliet", "-rock",
				dir)
			return cmd.Run()
		}
	}
	return fmt.Errorf("no ISO tool found")
}

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
	create := exec.Command("hdiutil", "create",
		"-megabytes", "16", "-fs", "MS-DOS FAT16",
		"-volname", label, "-layout", "NONE", "-ov", dmg)
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

// parseHdiutilAttach extracts the device node and mountpoint from
// `hdiutil attach` output (columns: /dev/diskN <type> <mountpoint>).
func parseHdiutilAttach(out string) (dev, mount string) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "/dev/") {
			continue
		}
		if dev == "" {
			dev = fields[0]
		}
		if i := strings.Index(line, "/Volumes/"); i >= 0 {
			dev = fields[0]
			mount = strings.TrimSpace(line[i:])
			return dev, mount
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

func generateSeedISOBuiltin(dir, outPath, volumeID string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var files []fileEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		files = append(files, fileEntry{name: e.Name(), data: data})
	}

	iso := buildISO9660(files, volumeID)
	return os.WriteFile(outPath, iso, 0644)
}

func buildISO9660(files []fileEntry, volumeID string) []byte {
	const sectorSize = 2048

	// ISO9660 layout:
	// Sectors 0-15: system area (zeroed)
	// Sector 16: primary volume descriptor
	// Sector 17: volume descriptor set terminator
	// Sector 18: root directory (. and .. + file entries)
	// Sector 19+: file data

	now := time.Now()
	rootDirSector := uint32(18)
	dataSector := uint32(19)

	// Calculate data sector offsets for each file
	type fileLoc struct {
		sector uint32
		size   uint32
	}
	locs := make([]fileLoc, len(files))
	cur := dataSector
	for i, f := range files {
		locs[i] = fileLoc{sector: cur, size: uint32(len(f.data))}
		sectors := (uint32(len(f.data)) + sectorSize - 1) / sectorSize
		if sectors == 0 {
			sectors = 1
		}
		cur += sectors
	}
	totalSectors := cur

	buf := make([]byte, int(totalSectors)*sectorSize)

	// --- Primary Volume Descriptor (sector 16) ---
	pvd := buf[16*sectorSize : 17*sectorSize]
	pvd[0] = 1    // type: primary
	copy(pvd[1:6], "CD001")
	pvd[6] = 1    // version
	padRight(pvd[8:40], " ", 32)        // system id
	padRight(pvd[40:72], volumeID, 32)  // volume id
	putBothEndian32(pvd[80:88], totalSectors)
	putBothEndian16(pvd[120:124], 1)    // volume set size
	putBothEndian16(pvd[124:128], 1)    // volume sequence number
	putBothEndian16(pvd[128:132], sectorSize)
	// path table size (simplified: 10 bytes for root)
	putBothEndian32(pvd[132:140], 10)
	binary.LittleEndian.PutUint32(pvd[140:144], rootDirSector) // L path table
	binary.BigEndian.PutUint32(pvd[148:152], rootDirSector)    // M path table
	// Root directory record (34 bytes)
	writeDirectoryRecord(pvd[156:190], rootDirSector, sectorSize, 0x02, now)
	// Volume dates
	writeDecDateTime(pvd[813:830], now)
	writeDecDateTime(pvd[830:847], now)
	pvd[881] = 1 // file structure version

	// --- Volume Descriptor Set Terminator (sector 17) ---
	term := buf[17*sectorSize : 18*sectorSize]
	term[0] = 255
	copy(term[1:6], "CD001")
	term[6] = 1

	// --- Root Directory (sector 18) ---
	dirBuf := buf[rootDirSector*sectorSize : (rootDirSector+1)*sectorSize]
	offset := 0

	// "." entry
	offset += putDirEntry(dirBuf[offset:], rootDirSector,
		sectorSize, 0x02, "\x00", now)
	// ".." entry
	offset += putDirEntry(dirBuf[offset:], rootDirSector,
		sectorSize, 0x02, "\x01", now)

	// File entries
	for i, f := range files {
		isoName := toISOName(f.name)
		offset += putDirEntry(dirBuf[offset:], locs[i].sector,
			int(locs[i].size), 0x00, isoName, now)
	}

	// --- File data ---
	for i, f := range files {
		start := int(locs[i].sector) * sectorSize
		copy(buf[start:], f.data)
	}

	return buf
}

func putDirEntry(
	buf []byte, sector uint32, size int, flags byte, name string, t time.Time,
) int {
	nameLen := len(name)
	recLen := 33 + nameLen
	if recLen%2 != 0 {
		recLen++
	}

	buf[0] = byte(recLen) // length of directory record
	buf[1] = 0            // extended attribute length
	putBothEndian32(buf[2:10], sector)
	putBothEndian32(buf[10:18], uint32(size))
	writeRecDateTime(buf[18:25], t)
	buf[25] = flags
	putBothEndian16(buf[28:32], 1) // volume sequence number
	buf[32] = byte(nameLen)
	copy(buf[33:33+nameLen], name)
	return recLen
}

func writeDirectoryRecord(
	buf []byte, sector uint32, size int, flags byte, t time.Time,
) {
	buf[0] = 34 // length
	putBothEndian32(buf[2:10], sector)
	putBothEndian32(buf[10:18], uint32(size))
	writeRecDateTime(buf[18:25], t)
	buf[25] = flags
	putBothEndian16(buf[28:32], 1)
	buf[32] = 1 // name length
	buf[33] = 0 // name: root
}

func writeRecDateTime(buf []byte, t time.Time) {
	buf[0] = byte(t.Year() - 1900)
	buf[1] = byte(t.Month())
	buf[2] = byte(t.Day())
	buf[3] = byte(t.Hour())
	buf[4] = byte(t.Minute())
	buf[5] = byte(t.Second())
	_, offset := t.Zone()
	buf[6] = byte(offset / (15 * 60))
}

func writeDecDateTime(buf []byte, t time.Time) {
	s := t.Format("2006010215040500")
	copy(buf, s)
	_, offset := t.Zone()
	buf[16] = byte(offset / (15 * 60))
}

func putBothEndian16(buf []byte, v uint16) {
	binary.LittleEndian.PutUint16(buf[0:2], v)
	binary.BigEndian.PutUint16(buf[2:4], v)
}

func putBothEndian32(buf []byte, v uint32) {
	binary.LittleEndian.PutUint32(buf[0:4], v)
	binary.BigEndian.PutUint32(buf[4:8], v)
}

func padRight(buf []byte, s string, width int) {
	copy(buf, s)
	for i := len(s); i < width; i++ {
		buf[i] = ' '
	}
}

func toISOName(name string) string {
	// ISO 9660 level 1: uppercase, 8.3, terminated with ;1
	var b bytes.Buffer
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - 32)
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	b.WriteString(";1")
	return b.String()
}

type fileEntry struct {
	name string
	data []byte
}

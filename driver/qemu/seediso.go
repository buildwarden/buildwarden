package qemu

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// generateSeedISO creates an ISO9660 image from a directory, labeled
// "CIDATA" for cloud-init NoCloud detection.
// Uses platform tools (hdiutil on macOS, mkisofs/genisoimage on Linux)
// with a pure-Go fallback.
func generateSeedISO(dir, outPath string) error {
	if err := generateSeedISOExternal(dir, outPath); err == nil {
		return nil
	}
	return generateSeedISOBuiltin(dir, outPath)
}

func generateSeedISOExternal(dir, outPath string) error {
	// macOS: hdiutil
	if _, err := exec.LookPath("hdiutil"); err == nil {
		cmd := exec.Command("hdiutil", "makehybrid",
			"-o", outPath,
			"-joliet", "-iso",
			"-default-volume-name", "CIDATA",
			dir)
		return cmd.Run()
	}
	// Linux: mkisofs or genisoimage
	for _, tool := range []string{"mkisofs", "genisoimage"} {
		if _, err := exec.LookPath(tool); err == nil {
			cmd := exec.Command(tool,
				"-output", outPath,
				"-volid", "CIDATA",
				"-joliet", "-rock",
				dir)
			return cmd.Run()
		}
	}
	return fmt.Errorf("no ISO tool found")
}

func generateSeedISOBuiltin(dir, outPath string) error {
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

	iso := buildISO9660(files, "CIDATA")
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

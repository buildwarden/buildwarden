package firecracker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strings"

	"github.com/buildwarden/buildwarden/driver"
)

// The GPT reader finds the build rootfs's root partition so the pinned kernel
// can boot it with root=PARTUUID (Decision 7). It reads the primary header and
// partition array of the raw image on the host, never mounting it, and treats
// both as untrusted input: every offset and count is bounds-checked against
// the file and both CRCs must match.

const (
	gptSector     = 512
	gptMaxEntries = 1024
	gptMaxEntSize = 4096
)

// Partition type GUIDs (Discoverable Partitions Specification).
const (
	gptTypeRootX86_64 = "4f68bce3-e8cd-4db1-96e7-fbcaf984b709"
	gptTypeLinuxData  = "0fc63daf-8483-4772-8e79-3d69d8477de4"
)

// gptPartition is one used entry of a partition array.
type gptPartition struct {
	Index    int // 1-based, as the kernel numbers partitions
	TypeGUID string
	PartUUID string
}

// rootPartUUID returns the PARTUUID of image's root partition: the one
// partition with the x86-64 root type, else the one Linux-filesystem
// partition. Anything else is a fixable error.
func rootPartUUID(image string) (string, error) {
	f, err := os.Open(image)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	parts, err := readGPT(f, fi.Size())
	if err != nil {
		return "", driver.Fixable(rootFix, "%s: %v", image, err)
	}
	p, err := pickRoot(parts)
	if err != nil {
		return "", driver.Fixable(rootFix, "%s: %v", image, err)
	}
	return p.PartUUID, nil
}

const rootFix = "use a GPT cloud image with one root (or one Linux filesystem) " +
	"partition, such as ubuntu:24.04, or pass another with --image"

func pickRoot(parts []gptPartition) (gptPartition, error) {
	for _, typ := range []string{gptTypeRootX86_64, gptTypeLinuxData} {
		var match []gptPartition
		for _, p := range parts {
			if p.TypeGUID == typ {
				match = append(match, p)
			}
		}
		switch len(match) {
		case 1:
			return match[0], nil
		case 0:
			continue
		default:
			return gptPartition{}, fmt.Errorf("%d partitions of type %s; "+
				"cannot tell which is root", len(match), typ)
		}
	}
	return gptPartition{}, errors.New("no x86-64 root or Linux filesystem partition")
}

// readGPTHeader reads and checks the primary GPT header at LBA 1.
func readGPTHeader(r io.ReaderAt, size int64) ([]byte, error) {
	hdr := make([]byte, gptSector)
	if size < 2*gptSector {
		return nil, errors.New("too small to hold a GPT")
	}
	if _, err := r.ReadAt(hdr, gptSector); err != nil {
		return nil, fmt.Errorf("reading the GPT header: %w", err)
	}
	if string(hdr[0:8]) != "EFI PART" {
		return nil, errors.New("no GPT header (not a GPT disk image)")
	}
	hdrSize := binary.LittleEndian.Uint32(hdr[12:16])
	if hdrSize < 92 || hdrSize > gptSector {
		return nil, fmt.Errorf("GPT header size %d is invalid", hdrSize)
	}
	want := binary.LittleEndian.Uint32(hdr[16:20])
	check := append([]byte(nil), hdr[:hdrSize]...)
	copy(check[16:20], []byte{0, 0, 0, 0})
	if crc32.ChecksumIEEE(check) != want {
		return nil, errors.New("GPT header checksum mismatch")
	}
	return hdr, nil
}

// readGPT parses the primary GPT of an image of the given size.
func readGPT(r io.ReaderAt, size int64) ([]gptPartition, error) {
	hdr, err := readGPTHeader(r, size)
	if err != nil {
		return nil, err
	}
	entLBA := binary.LittleEndian.Uint64(hdr[72:80])
	n := binary.LittleEndian.Uint32(hdr[80:84])
	entSize := binary.LittleEndian.Uint32(hdr[84:88])
	entCRC := binary.LittleEndian.Uint32(hdr[88:92])
	if n > gptMaxEntries || entSize < 128 || entSize > gptMaxEntSize || entSize%8 != 0 {
		return nil, fmt.Errorf("GPT partition array (%d entries of %d bytes) is invalid",
			n, entSize)
	}
	total := int64(n) * int64(entSize)
	if entLBA < 2 || entLBA > uint64(size/gptSector) ||
		int64(entLBA)*gptSector+total > size {
		return nil, errors.New("GPT partition array lies outside the image")
	}
	arr := make([]byte, total)
	if _, err := r.ReadAt(arr, int64(entLBA)*gptSector); err != nil {
		return nil, fmt.Errorf("reading the GPT partition array: %w", err)
	}
	if crc32.ChecksumIEEE(arr) != entCRC {
		return nil, errors.New("GPT partition array checksum mismatch")
	}
	var parts []gptPartition
	for i := uint32(0); i < n; i++ {
		e := arr[int64(i)*int64(entSize):]
		typ := guidString(e[0:16])
		if typ == "00000000-0000-0000-0000-000000000000" {
			continue
		}
		parts = append(parts, gptPartition{
			Index: int(i) + 1, TypeGUID: typ, PartUUID: guidString(e[16:32]),
		})
	}
	return parts, nil
}

// guidString formats an on-disk GUID: the first three fields are
// little-endian, the last two big-endian.
func guidString(b []byte) string {
	return strings.ToLower(fmt.Sprintf("%08x-%04x-%04x-%x-%x",
		binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint16(b[4:6]),
		binary.LittleEndian.Uint16(b[6:8]), b[8:10], b[10:16]))
}

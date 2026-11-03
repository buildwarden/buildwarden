package firecracker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// Type GUIDs the fixtures use besides the two rootPartUUID looks for.
const (
	gptTypeBIOSBoot  = "21686148-6449-6e6f-744e-656564454649"
	gptTypeESP       = "c12a7328-f81f-11d2-ba4b-00a0c93ec93b"
	gptTypeXBOOTLDR  = "bc13c2ff-59e6-4262-a352-b275fd6f7172"
	partUUIDLinux    = "5a1c7b2d-9e41-4f0a-8b3e-1d2c3b4a5f60"
	partUUIDRootType = "0b9b8a7c-6d5e-4f3a-a2b1-c0d9e8f7a6b5"
)

// guidBytes is guidString's inverse.
func guidBytes(s string) []byte {
	h := strings.ReplaceAll(s, "-", "")
	raw := make([]byte, 16)
	for i := range raw {
		var v byte
		for _, c := range h[2*i : 2*i+2] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= byte(c - '0')
			default:
				v |= byte(c-'a') + 10
			}
		}
		raw[i] = v
	}
	out := make([]byte, 16)
	binary.LittleEndian.PutUint32(out[0:4], binary.BigEndian.Uint32(raw[0:4]))
	binary.LittleEndian.PutUint16(out[4:6], binary.BigEndian.Uint16(raw[4:6]))
	binary.LittleEndian.PutUint16(out[6:8], binary.BigEndian.Uint16(raw[6:8]))
	copy(out[8:], raw[8:])
	return out
}

type fixturePart struct{ typ, uuid string }

// gptImage builds a small disk image whose primary GPT holds parts (128
// entries of 128 bytes at LBA 2, as sgdisk writes them).
func gptImage(parts []fixturePart) []byte {
	const entries, entSize = 128, 128
	img := make([]byte, 64*gptSector)
	arr := img[2*gptSector : 2*gptSector+entries*entSize]
	for i, p := range parts {
		e := arr[i*entSize:]
		copy(e[0:16], guidBytes(p.typ))
		copy(e[16:32], guidBytes(p.uuid))
		binary.LittleEndian.PutUint64(e[32:40], uint64(40+i))
		binary.LittleEndian.PutUint64(e[40:48], uint64(40+i))
	}
	h := img[gptSector : 2*gptSector]
	copy(h[0:8], "EFI PART")
	binary.LittleEndian.PutUint32(h[8:12], 0x00010000)
	binary.LittleEndian.PutUint32(h[12:16], 92)
	binary.LittleEndian.PutUint64(h[24:32], 1)
	binary.LittleEndian.PutUint64(h[72:80], 2)
	binary.LittleEndian.PutUint32(h[80:84], entries)
	binary.LittleEndian.PutUint32(h[84:88], entSize)
	binary.LittleEndian.PutUint32(h[88:92], crc32.ChecksumIEEE(arr))
	binary.LittleEndian.PutUint32(h[16:20], crc32.ChecksumIEEE(h[:92]))
	return img
}

// ubuntuLayout is noble-server-cloudimg-amd64's: root (1) next to BIOS boot
// (14), ESP (15) and XBOOTLDR (16), with no x86-64 root type GUID.
var ubuntuLayout = []fixturePart{
	{gptTypeLinuxData, partUUIDLinux},
	{gptTypeBIOSBoot, "11111111-2222-3333-4444-555555555555"},
	{gptTypeESP, "66666666-7777-8888-9999-aaaaaaaaaaaa"},
	{gptTypeXBOOTLDR, "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"},
}

func TestRootPartUUID(t *testing.T) {
	for name, tt := range map[string]struct {
		parts []fixturePart
		want  string
	}{
		"ubuntu layout: single Linux filesystem": {ubuntuLayout, partUUIDLinux},
		"x86-64 root type wins": {append([]fixturePart{
			{gptTypeRootX86_64,
				partUUIDRootType}}, ubuntuLayout...), partUUIDRootType},
		"no root": {[]fixturePart{{gptTypeESP, partUUIDLinux}}, ""},
		"two Linux parts": {[]fixturePart{{gptTypeLinuxData, partUUIDLinux},
			{gptTypeLinuxData, partUUIDRootType}}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "disk.raw")
			if err := os.WriteFile(path, gptImage(tt.parts), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := rootPartUUID(path)
			if tt.want == "" {
				var fe *driver.FixableError
				if !errors.As(err, &fe) {
					t.Fatalf("rootPartUUID = %q, %v; want a fixable error",
						got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("rootPartUUID = %q, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestReadGPTRejectsMalformed(t *testing.T) {
	good := gptImage(ubuntuLayout)
	for name, mut := range map[string]func([]byte) []byte{
		"truncated":      func(b []byte) []byte { return b[:gptSector+100] },
		"no signature":   func(b []byte) []byte { b[gptSector] = 'X'; return b },
		"header crc":     func(b []byte) []byte { b[gptSector+24] ^= 1; return b },
		"entries crc":    func(b []byte) []byte { b[2*gptSector+40] ^= 1; return b },
		"array past end": func(b []byte) []byte { return b[:3*gptSector] },
		"huge entry count": func(b []byte) []byte {
			h := b[gptSector:]
			binary.LittleEndian.PutUint32(h[80:84], 1<<20)
			binary.LittleEndian.PutUint32(h[16:20], 0)
			binary.LittleEndian.PutUint32(h[16:20], crc32.ChecksumIEEE(h[:92]))
			return b
		},
	} {
		t.Run(name, func(t *testing.T) {
			img := mut(append([]byte(nil), good...))
			if _, err := readGPT(bytes.NewReader(img), int64(len(img))); err == nil {
				t.Fatal("malformed GPT accepted")
			}
		})
	}
	parts, err := readGPT(bytes.NewReader(good), int64(len(good)))
	if err != nil || len(parts) != 4 || parts[0].Index != 1 ||
		parts[0].PartUUID != partUUIDLinux {
		t.Fatalf("good GPT: %+v, %v", parts, err)
	}
}

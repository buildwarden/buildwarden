package container

import (
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// compressedMagic identifies already-compressed formats by their
// first bytes.
var compressedMagic = [][]byte{
	{0x1f, 0x8b},             // gzip
	{0x28, 0xb5, 0x2f, 0xfd}, // zstd
	{0xfd, 0x37, 0x7a, 0x58}, // xz
	{0x50, 0x4b, 0x03, 0x04}, // zip/jar/whl
	{0x42, 0x5a, 0x68},       // bzip2
}

func isCompressedFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	n, _ := f.Read(buf)
	for _, magic := range compressedMagic {
		if n >= len(magic) {
			match := true
			for i, b := range magic {
				if buf[i] != b {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func compressFile(src, dst string) {
	if isCompressedFile(src) {
		uncompDst := strings.TrimSuffix(dst, ".zst")
		os.Rename(src, uncompDst) //nolint:errcheck
		return
	}

	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}

	enc, err := zstd.NewWriter(out)
	if err != nil {
		out.Close()
		os.Remove(dst) //nolint:errcheck
		return
	}
	io.Copy(enc, in) //nolint:errcheck
	enc.Close()
	out.Close()
}


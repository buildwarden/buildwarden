package container

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsCompressedFile_Gzip(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.gz")
	if err := os.WriteFile(f, []byte{0x1f, 0x8b, 0x08, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isCompressedFile(f) {
		t.Error("expected gzip file to be detected as compressed")
	}
}

func TestIsCompressedFile_Zstd(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.zst")
	if err := os.WriteFile(
		f, []byte{0x28, 0xb5, 0x2f, 0xfd, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isCompressedFile(f) {
		t.Error("expected zstd file to be detected as compressed")
	}
}

func TestIsCompressedFile_Xz(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.xz")
	if err := os.WriteFile(
		f, []byte{0xfd, 0x37, 0x7a, 0x58, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isCompressedFile(f) {
		t.Error("expected xz file to be detected as compressed")
	}
}

func TestIsCompressedFile_Zip(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(
		f, []byte{0x50, 0x4b, 0x03, 0x04, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isCompressedFile(f) {
		t.Error("expected zip file to be detected as compressed")
	}
}

func TestIsCompressedFile_Bzip2(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.bz2")
	if err := os.WriteFile(
		f, []byte{0x42, 0x5a, 0x68, 0x39, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isCompressedFile(f) {
		t.Error("expected bzip2 file to be detected as compressed")
	}
}

func TestIsCompressedFile_PlainText(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(
		f, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if isCompressedFile(f) {
		t.Error("expected plain text file to not be detected as compressed")
	}
}

func TestIsCompressedFile_NonExistent(t *testing.T) {
	if isCompressedFile("/nonexistent/file") {
		t.Error("expected false for nonexistent file")
	}
}

func TestIsCompressedFile_Empty(t *testing.T) {
	f := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(f, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if isCompressedFile(f) {
		t.Error("expected false for empty file")
	}
}

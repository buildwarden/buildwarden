package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// --- isCopyDirective ---

func TestIsCopyDirective_Space(t *testing.T) {
	if !isCopyDirective("COPY foo bar") {
		t.Error("expected true for 'COPY foo bar'")
	}
}

func TestIsCopyDirective_Tab(t *testing.T) {
	if !isCopyDirective("COPY\tfoo") {
		t.Error("expected true for 'COPY\\tfoo'")
	}
}

func TestIsCopyDirective_LeadingWhitespace(t *testing.T) {
	if !isCopyDirective("  COPY foo bar") {
		t.Error("expected true for '  COPY foo bar'")
	}
}

func TestIsCopyDirective_RunDirective(t *testing.T) {
	if isCopyDirective("RUN something") {
		t.Error("expected false for 'RUN something'")
	}
}

func TestIsCopyDirective_COPYING(t *testing.T) {
	if isCopyDirective("COPYING") {
		t.Error("expected false for 'COPYING' (no space/tab after COPY)")
	}
}

func TestIsCopyDirective_Empty(t *testing.T) {
	if isCopyDirective("") {
		t.Error("expected false for empty string")
	}
}

// --- parseCopyFlags ---

func TestParseCopyFlags_NoFlags(t *testing.T) {
	chown, chmod, positional := parseCopyFlags("foo bar")
	if chown != "" {
		t.Errorf("chown = %q, want empty", chown)
	}
	if chmod != "" {
		t.Errorf("chmod = %q, want empty", chmod)
	}
	if len(positional) != 2 || positional[0] != "foo" || positional[1] != "bar" {
		t.Errorf("positional = %v, want [foo bar]", positional)
	}
}

func TestParseCopyFlags_Chown(t *testing.T) {
	chown, chmod, positional := parseCopyFlags("--chown=1000:1000 foo bar")
	if chown != "1000:1000" {
		t.Errorf("chown = %q, want 1000:1000", chown)
	}
	if chmod != "" {
		t.Errorf("chmod = %q, want empty", chmod)
	}
	if len(positional) != 2 || positional[0] != "foo" || positional[1] != "bar" {
		t.Errorf("positional = %v, want [foo bar]", positional)
	}
}

func TestParseCopyFlags_ChmodAndChown(t *testing.T) {
	chown, chmod, positional := parseCopyFlags(
		"--chmod=755 --chown=root:root src/ /dest/")
	if chown != "root:root" {
		t.Errorf("chown = %q, want root:root", chown)
	}
	if chmod != "755" {
		t.Errorf("chmod = %q, want 755", chmod)
	}
	if len(positional) != 2 ||
		positional[0] != "src/" || positional[1] != "/dest/" {
		t.Errorf("positional = %v, want [src/ /dest/]", positional)
	}
}

func TestParseCopyFlags_SimpleArgs(t *testing.T) {
	chown, chmod, positional := parseCopyFlags("src dest")
	if chown != "" {
		t.Errorf("chown = %q, want empty", chown)
	}
	if chmod != "" {
		t.Errorf("chmod = %q, want empty", chmod)
	}
	if len(positional) != 2 || positional[0] != "src" || positional[1] != "dest" {
		t.Errorf("positional = %v, want [src dest]", positional)
	}
}

// --- buildFetchRun ---

func TestBuildFetchRun_SingleFileToFile(t *testing.T) {
	got := buildFetchRun([]string{"main.go"}, "/app/main.go")
	want := `RUN mkdir -p /app/ && curl -fsSL -o /app/main.go "http://cwd/main.go"`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildFetchRun_SingleFileToFileNested(t *testing.T) {
	// Single file to a non-dir dest with no directory component.
	got := buildFetchRun([]string{"app.bin"}, "app.bin")
	want := `RUN curl -fsSL -o app.bin "http://cwd/app.bin"`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildFetchRun_SingleFileToDirDest(t *testing.T) {
	got := buildFetchRun([]string{"main.go"}, "/app/")
	// Trailing / means directory dest, should use xargs pipeline.
	if !strings.HasPrefix(got, "RUN mkdir -p /app/") {
		t.Errorf("expected mkdir prefix, got:\n%s", got)
	}
	if !strings.Contains(got, "xargs") {
		t.Errorf("expected xargs pipeline for dir dest, got:\n%s", got)
	}
	if !strings.Contains(got, "'main.go'") {
		t.Errorf("expected file name in printf args, got:\n%s", got)
	}
}

func TestBuildFetchRun_MultipleFiles(t *testing.T) {
	got := buildFetchRun(
		[]string{"a.go", "b.go", "c.go"}, "/src/")
	if !strings.HasPrefix(got, "RUN mkdir -p /src/") {
		t.Errorf("expected mkdir prefix, got:\n%s", got)
	}
	if !strings.Contains(got, "xargs -P8") {
		t.Errorf("expected parallel xargs, got:\n%s", got)
	}
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(got, "'"+f+"'") {
			t.Errorf("missing file %q in output:\n%s", f, got)
		}
	}
}

func TestBuildFetchRun_MultipleFilesToNonDirDest(t *testing.T) {
	// Multiple files to a non-dir dest: trailing / is appended internally.
	got := buildFetchRun([]string{"a.go", "b.go"}, "/dest")
	if !strings.Contains(got, "xargs") {
		t.Errorf("expected xargs pipeline for multi-file, got:\n%s", got)
	}
	// The dest should get a trailing / in the curl commands.
	if !strings.Contains(got, "/dest/") {
		t.Errorf("expected /dest/ with trailing slash, got:\n%s", got)
	}
}

// --- isCompressedFile ---

func TestIsCompressedFile_Gzip(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.gz")
	// gzip magic: 1f 8b
	if err := os.WriteFile(f, []byte{0x1f, 0x8b, 0x08, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isCompressedFile(f) {
		t.Error("expected gzip file to be detected as compressed")
	}
}

func TestIsCompressedFile_Zstd(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.zst")
	// zstd magic: 28 b5 2f fd
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
	// xz magic: fd 37 7a 58
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
	// zip magic: 50 4b 03 04
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
	// bzip2 magic: 42 5a 68
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

// --- randanum ---

func TestRandanum_Length(t *testing.T) {
	for _, n := range []int{0, 1, 8, 32, 100} {
		got := randanum(n)
		if len(got) != n {
			t.Errorf("randanum(%d) length = %d, want %d", n, len(got), n)
		}
	}
}

func TestRandanum_Charset(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9]*$`)
	// Run several times to get good coverage of the character space.
	for i := 0; i < 20; i++ {
		got := randanum(64)
		if !valid.MatchString(got) {
			t.Errorf("randanum(64) = %q, contains invalid chars", got)
		}
	}
}

func TestRandanum_Uniqueness(t *testing.T) {
	// Two calls should (almost certainly) produce different values.
	a := randanum(16)
	b := randanum(16)
	if a == b {
		t.Errorf("two randanum(16) calls produced identical values: %q", a)
	}
}

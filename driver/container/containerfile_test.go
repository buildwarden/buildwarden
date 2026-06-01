package container

import (
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
	want := `RUN mkdir -p /app/ && warden-io fetch main.go -o /app/main.go`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildFetchRun_SingleFileToFileNested(t *testing.T) {
	got := buildFetchRun([]string{"app.bin"}, "app.bin")
	want := `RUN warden-io fetch app.bin -o app.bin`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildFetchRun_SingleFileToDirDest(t *testing.T) {
	got := buildFetchRun([]string{"main.go"}, "/app/")
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
	got := buildFetchRun([]string{"a.go", "b.go"}, "/dest")
	if !strings.Contains(got, "xargs") {
		t.Errorf("expected xargs pipeline for multi-file, got:\n%s", got)
	}
	if !strings.Contains(got, "/dest/") {
		t.Errorf("expected /dest/ with trailing slash, got:\n%s", got)
	}
}

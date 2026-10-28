package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestScriptDestPathPreservesBaseName(t *testing.T) {
	cases := []struct {
		script   string
		wantBase string
	}{
		{"build.sh", "build.sh"},
		{"build.ps1", "build.ps1"},
		{"build.bat", "build.bat"},
		{"nested/dir/custom-build.ps1", "custom-build.ps1"},
	}
	for _, c := range cases {
		got := scriptDestPath(c.script)
		if filepath.Base(got) != c.wantBase {
			t.Errorf("scriptDestPath(%q) base = %q, want %q",
				c.script, filepath.Base(got), c.wantBase)
		}
		if filepath.Dir(got) != filepath.Clean(os.TempDir()) {
			t.Errorf("scriptDestPath(%q) dir = %q, want temp dir %q",
				c.script, filepath.Dir(got), os.TempDir())
		}
	}
}

func TestDefaultScriptNameForPlatform(t *testing.T) {
	got := defaultScriptName()
	if got == "" {
		t.Fatal("defaultScriptName() is empty")
	}
	want := "build.sh"
	if runtime.GOOS == "windows" {
		want = "build.ps1"
	}
	if got != want {
		t.Errorf("defaultScriptName() = %q, want %q on %s",
			got, want, runtime.GOOS)
	}
	if !strings.HasPrefix(got, "build.") {
		t.Errorf("defaultScriptName() = %q, want a build.* name", got)
	}
}

func TestParseInitArgs(t *testing.T) {
	a := parseInitArgs([]string{
		"--gateway=10.0.0.1", "--ip", "10.0.0.2/30", "--script=b.ps1",
		"--workdir", "/work", "--hold", "--unknown=x",
	})
	want := initArgs{
		gateway: "10.0.0.1", selfIP: "10.0.0.2/30", script: "b.ps1",
		workdir: "/work", hold: true,
	}
	if a != want {
		t.Errorf("parseInitArgs = %+v, want %+v", a, want)
	}
}

func TestShellEnvScript(t *testing.T) {
	got := shellEnvScript(map[string]string{"B": "it's", "A": "1"}, "/work")
	want := "export A='1'\nexport B='it'\\''s'\ncd '/work'\n"
	if got != want {
		t.Errorf("shellEnvScript = %q, want %q", got, want)
	}
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/buildwarden/buildwarden/driver"
)

// The four levels that can name the container runtime, lowest first.
var runtimeLevels = []string{"user", "project", "env", "flag"}

// tomlString writes v as a TOML basic string.
func tomlString(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch {
		case r == '"' || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func cliFile(v string) string { return "[runtime]\ncli = " + tomlString(v) + "\n" }

// userConfigFile is the path of the user's config file under the test's
// home, as the error names it.
func userConfigFile(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".config", "warden", "config.toml")
}

// nameRuntime runs the test from fresh config directories with the runtime
// named v at one level and nowhere else. It returns what the error calls
// that level. The flag's value is a copy, so that it shares no memory with a
// string constant of the test binary.
func nameRuntime(t *testing.T, level, v string) string {
	t.Helper()
	switch level {
	case "user":
		inConfigFiles(t, cliFile(v), "")
		return "[runtime] cli in " + userConfigFile(t)
	case "project":
		inConfigFiles(t, "", cliFile(v))
		return "[runtime] cli in warden.toml"
	case "env":
		inConfigFiles(t, "", "")
		t.Setenv("WARDEN_CTR_CLI", v)
		return "WARDEN_CTR_CLI"
	case "flag":
		inConfigFiles(t, "", "")
		flagRuntime = strings.Clone(v)
		return "--runtime"
	}
	t.Fatalf("unknown level %q", level)
	return ""
}

// resolvedRuntime is what a command that uses the container runtime gets:
// the configuration as resolveConfig leaves it, through resolveRuntime.
func resolvedRuntime(t *testing.T) (string, error) {
	t.Helper()
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	return resolveRuntime(cfg)
}

// wantRuntimeRefused checks that err is the refusal of v as named at setting:
// a FixableError, which main exits 1 on (UX-09), whose message starts with
// the setting and the quoted value.
func wantRuntimeRefused(t *testing.T, err error, setting, v string) *driver.FixableError {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError refusing %q", err, v)
	}
	var es exitStatusError
	if errors.As(err, &es) {
		t.Errorf("the refusal carries exit status %d, want main's exit 1", int(es))
	}
	if want := setting + " is " + strconv.Quote(v) + "; "; !strings.HasPrefix(fe.Msg, want) {
		t.Errorf("message %q does not start with %q", fe.Msg, want)
	}
	if fe.Fix == "" {
		t.Error("the refusal has no fix")
	}
	return fe
}

func TestSupportedRuntimeFromEverySource(t *testing.T) {
	for i, name := range []string{"finch", "docker", "podman"} {
		for _, level := range runtimeLevels {
			t.Run(name+" from "+level, func(t *testing.T) {
				nameRuntime(t, level, name)
				got, err := resolvedRuntime(t)
				if err != nil || got != name {
					t.Fatalf("resolveRuntime = %q, %v; want %q", got, err, name)
				}
				// The string that is run is the list's own, not an equal one
				// that a flag, a variable or a file supplied.
				if unsafe.StringData(got) != unsafe.StringData(containerRuntimes[i]) {
					t.Errorf("resolveRuntime returned a string equal to %q "+
						"that is not containerRuntimes' member", name)
				}
			})
		}
	}
}

func TestUnsupportedRuntimeRefusedFromEverySource(t *testing.T) {
	for _, v := range []string{
		"./docker", "tools/docker", "/usr/bin/docker", `..\x.exe`,
		`C:\x\docker.exe`, "C:docker", "docker.exe", "Docker", " docker",
		"docker ", "nerdctl", "sh", "sudo docker", " ",
	} {
		for _, level := range runtimeLevels {
			t.Run(strconv.Quote(v)+" from "+level, func(t *testing.T) {
				setting := nameRuntime(t, level, v)
				got, err := resolvedRuntime(t)
				if got != "" {
					t.Errorf("resolveRuntime returned %q with its refusal", got)
				}
				wantRuntimeRefused(t, err, setting, v)
			})
		}
	}
}

// Detection and the check share one list, in the order detection has always
// probed.
func TestDetectRuntimeProbesTheSupportedList(t *testing.T) {
	want := []string{"finch", "docker", "podman"}
	if !slices.Equal(containerRuntimes, want) {
		t.Fatalf("containerRuntimes = %v, want %v", containerRuntimes, want)
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	bin, probed := t.TempDir(), filepath.Join(t.TempDir(), "probed")
	for _, name := range want {
		script := fmt.Sprintf("#!/bin/sh\necho %s \"$1\" >> '%s'\nexit 1\n", name, probed)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	if rt, err := DetectRuntime(); err == nil {
		t.Fatalf("DetectRuntime = %q with no runtime answering", rt)
	}
	got, err := os.ReadFile(probed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "finch info\ndocker info\npodman info\n" {
		t.Errorf("detection ran %q, want info of finch, docker, podman in order", got)
	}
}

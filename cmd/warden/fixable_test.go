package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/buildwarden/buildwarden/driver"
)

func TestErrorWithFixHasRemedyAndNoIssueLink(t *testing.T) {
	color.NoColor = true
	var buf bytes.Buffer
	l := &Logger{out: &buf}
	err := fmt.Errorf("hyperv build: %w",
		driver.Fixable("run it as administrator", "not elevated"))
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatal("a wrapped FixableError is not found by errors.As")
	}
	l.ErrorWithFix(err, fe.Fix)
	out := buf.String()
	for _, want := range []string{"hyperv build: not elevated", "To fix: run it as administrator"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	if strings.Contains(out, "issues/new") {
		t.Errorf("a fixable error printed the issue link: %q", out)
	}
}

// fakeRuntime puts an executable named name on a PATH of its own; its
// `info` subcommand exits with code.
func fakeRuntime(t *testing.T, name string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", code)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestDetectRuntimeExplainsTheFix(t *testing.T) {
	t.Run("none installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := DetectRuntime()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "install") {
			t.Fatalf("DetectRuntime = %v, want a FixableError with an install hint", err)
		}
	})
	t.Run("installed but stopped", func(t *testing.T) {
		fakeRuntime(t, "finch", 1)
		_, err := DetectRuntime()
		var fe *driver.FixableError
		if !errors.As(err, &fe) || !strings.Contains(fe.Fix, "finch vm start") {
			t.Fatalf("DetectRuntime = %v, want a FixableError saying to start finch's VM", err)
		}
	})
	t.Run("running", func(t *testing.T) {
		fakeRuntime(t, "podman", 0)
		if rt, err := DetectRuntime(); err != nil || rt != "podman" {
			t.Fatalf("DetectRuntime = %q, %v; want podman", rt, err)
		}
	})
}

func TestInputErrorsAreFixable(t *testing.T) {
	for name, err := range map[string]error{
		"driver":  validateDriver("kvm"),
		"capture": validateCapture("everything"),
		"guestOS": validateGuestOS("plan9", "qemu"),
	} {
		var fe *driver.FixableError
		if !errors.As(err, &fe) {
			t.Errorf("%s: %v is not a FixableError", name, err)
		}
	}
}

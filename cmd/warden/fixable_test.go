package main

import (
	"bytes"
	"errors"
	"fmt"
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
	wants := []string{"hyperv build: not elevated", "To fix: run it as administrator"}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	if strings.Contains(out, "issues/new") {
		t.Errorf("a fixable error printed the issue link: %q", out)
	}
}

func TestInputErrorsAreFixable(t *testing.T) {
	for name, err := range map[string]error{
		"driver":  checkDriverName("kvm-x"),
		"capture": validateCapture("everything"),
		"guestOS": validateGuestOS("plan9", "qemu", linkedDrivers["qemu"].Capabilities),
	} {
		var fe *driver.FixableError
		if !errors.As(err, &fe) {
			t.Errorf("%s: %v is not a FixableError", name, err)
		}
	}
}

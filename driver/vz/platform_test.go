package vz

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// A build without the bindings must never pass the platform check, and each
// refusal must name its fix (UX-04).
func TestPlatformError(t *testing.T) {
	const (
		noCgo    = "this warden was built without cgo, so it has no vz driver (released binaries are built this way)"
		noCgoFix = "build warden from source on this Mac (xcode-select --install, then make build), or use --driver container or --driver qemu"
	)
	for _, tc := range []struct {
		goos, goarch string
		bindings     bool
		wantMsg      string
		wantFix      string
	}{
		{"linux", "amd64", false, "vz driver requires macOS on Apple Silicon (got linux/amd64)", "use --driver qemu or --driver container on this host"},
		{"linux", "arm64", false, "vz driver requires macOS on Apple Silicon (got linux/arm64)", "use --driver qemu or --driver container on this host"},
		{"windows", "amd64", false, "vz driver requires macOS on Apple Silicon (got windows/amd64)", "use --driver qemu or --driver container on this host"},
		{"darwin", "amd64", false, "vz driver requires macOS on Apple Silicon (got darwin/amd64)", "use --driver qemu or --driver container on this host"},
		{"darwin", "arm64", false, noCgo, noCgoFix},
		{"darwin", "arm64", true, "", ""},
	} {
		t.Run(fmt.Sprintf("%s/%s bindings=%v", tc.goos, tc.goarch, tc.bindings), func(t *testing.T) {
			err := platformError(tc.goos, tc.goarch, tc.bindings)
			if tc.wantMsg == "" {
				if err != nil {
					t.Fatalf("platformError = %v, want nil", err)
				}
				return
			}
			var fe *driver.FixableError
			if !errors.As(err, &fe) {
				t.Fatalf("platformError = %v, want a FixableError", err)
			}
			if fe.Msg != tc.wantMsg || fe.Fix != tc.wantFix {
				t.Errorf("platformError = %q / fix %q, want %q / fix %q", fe.Msg, fe.Fix, tc.wantMsg, tc.wantFix)
			}
		})
	}
}

// No combination of host and build without the bindings passes the check:
// the stub's VM functions return checkPlatform's error, so a nil here would
// hand a build a nil VM and no error.
func TestPlatformErrorWithoutBindingsNeverNil(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows", "freebsd"} {
		for _, goarch := range []string{"amd64", "arm64", "386"} {
			err := platformError(goos, goarch, false)
			var fe *driver.FixableError
			if !errors.As(err, &fe) || fe.Fix == "" {
				t.Errorf("platformError(%s, %s, false) = %v, want a FixableError with a fix", goos, goarch, err)
			}
		}
	}
}

// hasBindings must agree with the build it is compiled into. The oracle is
// the CGO_ENABLED setting the go command records in the test binary, not a
// build tag, which would only repeat the constraint under test.
func TestHasBindingsMatchesBuild(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("no build info in this binary")
	}
	cgo := ""
	for _, s := range info.Settings {
		if s.Key == "CGO_ENABLED" {
			cgo = s.Value
		}
	}
	if cgo == "" {
		t.Skip("build info records no CGO_ENABLED setting")
	}
	want := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && cgo == "1"
	if hasBindings != want {
		t.Errorf("hasBindings = %v on %s/%s with CGO_ENABLED=%s, want %v",
			hasBindings, runtime.GOOS, runtime.GOARCH, cgo, want)
	}
}

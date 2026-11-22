//go:build !linux

package firecracker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func TestStubNeedsLinux(t *testing.T) {
	_, err := New().Run(context.Background(), &driver.BuildRequest{}, &driver.Start{}, nil)
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(err.Error(), "Linux host") {
		t.Fatalf("Run = %v, want the needs-Linux fixable error", err)
	}
	if err := New().Preflight(&driver.BuildRequest{}); !errors.As(err, &fe) {
		t.Fatalf("Preflight = %v, want the needs-Linux fixable error", err)
	}
}

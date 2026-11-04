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
	_, err := New().StartBuild(context.Background(), &driver.BuildRequest{})
	var fe *driver.FixableError
	if !errors.As(err, &fe) || !strings.Contains(err.Error(), "Linux host") {
		t.Fatalf("StartBuild = %v, want the needs-Linux fixable error", err)
	}
}

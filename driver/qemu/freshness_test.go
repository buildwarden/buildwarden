package qemu

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

// A due Windows layer refresh reports a lock error or a failed start with
// the error and the command that refreshes by hand, and a started one with
// the existing line; the build goes on in every case.
func TestEnsureFreshWindowsLayerReports(t *testing.T) {
	const ga = "amd64"
	layer := PreparedLayer{
		PreparedAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		GraceEnd:   time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}
	req := &driver.BuildRequest{ImageRefreshAfter: time.Minute}
	freeLock := func(string) (func(), bool, error) { return func() {}, true, nil }
	notStarted := func(cause string) string {
		return "[warden] prepared Windows layer is 2h old, and its background " +
			"refresh did not start: " + cause + "; run `" + prepareFix(ga) +
			"` to refresh it\n"
	}
	tests := []struct {
		name     string
		lock     func(string) (func(), bool, error)
		startErr error
		want     string
		started  bool
	}{
		{
			name:     "start fails",
			lock:     freeLock,
			startErr: errors.New("exec format error"),
			want:     notStarted("exec format error"),
			started:  true,
		},
		{
			name: "lock fails",
			lock: func(string) (func(), bool, error) {
				return nil, false, errors.New("lock dir unwritable")
			},
			want: notStarted("lock dir unwritable"),
		},
		{
			name: "starts",
			lock: freeLock,
			want: "[warden] prepared Windows layer is 2h old; refreshing it in the " +
				"background (log: ",
			started: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hermeticCache(t)
			var args []string
			origStart, origLock := startDetached, tryHostLock
			t.Cleanup(func() { startDetached, tryHostLock = origStart, origLock })
			tryHostLock = tt.lock
			startDetached = func(_ string, a ...string) error {
				args = a
				return tt.startErr
			}

			var out bytes.Buffer
			if err := ensureFreshWindowsLayer(layer, ga, req, &out); err != nil {
				t.Fatalf("ensureFreshWindowsLayer: %v", err)
			}
			if !strings.HasPrefix(out.String(), tt.want) {
				t.Errorf("output = %q, want it to start %q", out.String(), tt.want)
			}
			want := []string{"image", "prepare", "--driver", "qemu", "--os", "windows",
				"--arch", ga}
			if tt.started && !slices.Equal(args, want) {
				t.Errorf("args = %q, want %q", args, want)
			}
			if !tt.started && args != nil {
				t.Errorf("started %q after a lock error", args)
			}
		})
	}
}

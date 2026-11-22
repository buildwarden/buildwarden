package drivercmd

import (
	"context"
	"io"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

// DRV-49, DRV-50: a stop, or the end of the input, that comes before
// claimed is answered with stopped, also by a Run that returns ClaimRelay's
// error after tearing down, as the container driver does.
func TestRunStopBeforeClaim(t *testing.T) {
	const stop = `{"type":"stop","reason":"interrupted","detail":""}` + "\n"
	relay := driver.RelayStarted{GuestOS: "linux",
		Claim: driver.ClaimEndpoint{Network: "tcp", Address: "127.0.0.1:1"}}
	for _, tc := range []struct {
		name string
		end  func(in io.WriteCloser)
	}{
		{"stop", func(in io.WriteCloser) { _, _ = io.WriteString(in, stop) }},
		{"end of input", func(in io.WriteCloser) { _ = in.Close() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tornDown := false
			d := kitDriver(t)
			d.Run = func(ctx context.Context, _ *driver.Start,
				host driver.Host) ([]string, error) {
				defer func() { tornDown = true }()
				return nil, host.ClaimRelay(ctx, relay)
			}
			in, out, done := runPipe(t, d)
			_, _ = io.WriteString(in, startLine)
			if got := readType(t, out); got != "relay-started" {
				t.Fatalf("first message %s", got)
			}
			tc.end(in)
			if got := readType(t, out); got != "stopped" {
				t.Errorf("last message %s, want stopped", got)
			}
			if code := <-done; code != 0 || !tornDown {
				t.Errorf("exit %d, torn down %v", code, tornDown)
			}
		})
	}
}

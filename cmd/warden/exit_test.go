package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/buildwarden/buildwarden/driver"
)

func TestBuildFailedExitCodes(t *testing.T) {
	live := context.Background()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	timed := &driver.BuildRequest{Timeout: 30 * time.Second}

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		err     error
		req     *driver.BuildRequest
		elapsed time.Duration
		want    error // exitStatus, or nil for "a warden error, unchanged"
	}{
		{"build script exit code passes through", live,
			fmt.Errorf("runBuild: %w", &driver.ExitError{Code: 7}), &driver.BuildRequest{}, 0, exitStatus(7)},
		{"--timeout is 124", live,
			fmt.Errorf("waiting: %w", context.DeadlineExceeded), timed, 31 * time.Second, exitStatus(exitTimedOut)},
		{"an internal deadline before --timeout is a warden error", live,
			fmt.Errorf("claim: %w", context.DeadlineExceeded), timed, 10 * time.Second, nil},
		{"interrupt is 130", cancelled, context.Canceled, &driver.BuildRequest{}, 0, exitStatus(exitInterrupted)},
		{"anything else is a warden error", live, errors.New("boom"), &driver.BuildRequest{}, 0, nil},
	} {
		got := buildFailed(tc.ctx, tc.err, tc.req, tc.elapsed)
		var es exitStatus
		isStatus := errors.As(got, &es)
		switch {
		case tc.want == nil && isStatus:
			t.Errorf("%s: got exit status %d, want the original error", tc.name, es)
		case tc.want == nil && got != tc.err:
			t.Errorf("%s: got %v, want the original error %v", tc.name, got, tc.err)
		case tc.want != nil && (!isStatus || es != tc.want):
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

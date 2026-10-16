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
		want    error // exitStatusError, or nil for "a warden error, unchanged"
	}{
		{
			name: "build script exit code passes through",
			ctx:  live,
			err:  fmt.Errorf("runBuild: %w", &driver.ExitError{Code: 7}),
			req:  &driver.BuildRequest{},
			want: exitStatusError(7),
		},
		{
			name:    "--timeout is 124",
			ctx:     live,
			err:     fmt.Errorf("waiting: %w", context.DeadlineExceeded),
			req:     timed,
			elapsed: 31 * time.Second,
			want:    exitStatusError(exitTimedOut),
		},
		{
			name:    "an internal deadline before --timeout is a warden error",
			ctx:     live,
			err:     fmt.Errorf("claim: %w", context.DeadlineExceeded),
			req:     timed,
			elapsed: 10 * time.Second,
			want:    nil,
		},
		{
			name: "interrupt is 130",
			ctx:  cancelled,
			err:  context.Canceled,
			req:  &driver.BuildRequest{},
			want: exitStatusError(exitInterrupted),
		},
		{
			name: "anything else is a warden error",
			ctx:  live,
			err:  errors.New("boom"),
			req:  &driver.BuildRequest{},
			want: nil,
		},
	} {
		got := buildFailed(tc.ctx, tc.err, tc.req, tc.elapsed)
		var es exitStatusError
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

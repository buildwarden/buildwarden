package vz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Signal protocol between the build VM and the host orchestrator.
//
// The build VM signals liveness via HTTP to the relay:
//   - GET http://artifacts/heartbeat → relay writes signal/heartbeat
//   - GET http://artifacts/exit?code=N → relay writes signal/exit_code
//
// The host orchestrator watches signal/:
//   - If signal/exit_code exists → build done
//   - If signal/heartbeat hasn't been updated within 3 intervals → dead
//   - exit_code "0" → success; otherwise → failure

const (
	heartbeatInterval = 2 * time.Second
	heartbeatTimeout  = 3 * heartbeatInterval // 6 seconds of silence = dead
)

// BuildStatus represents the current state of the build as observed
// from the host via the shared volume.
type BuildStatus int

const (
	BuildRunning      BuildStatus = iota
	BuildCompleted                // exit_code file exists
	BuildUnresponsive             // heartbeat stale beyond timeout
	BuildNotStarted               // no heartbeat, no exit_code
)

// WaitForBuild monitors the shared volume's signal directory and returns
// when the build completes, becomes unresponsive, or the context is cancelled.
func WaitForBuild(ctx context.Context, signalDir string, isTTY bool) (exitCode int, err error) {
	heartbeatPath := filepath.Join(signalDir, "heartbeat")
	exitCodePath := filepath.Join(signalDir, "exit_code")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()

		case <-ticker.C:
			status, code := checkBuildStatus(heartbeatPath, exitCodePath)

			switch status {
			case BuildCompleted:
				return code, nil

			case BuildUnresponsive:
				return -1, unresponsiveErr(isTTY)

			case BuildRunning, BuildNotStarted:
				continue
			}
		}
	}
}

func checkBuildStatus(heartbeatPath, exitCodePath string) (BuildStatus, int) {
	// Check if build completed (exit_code exists, heartbeat removed)
	if data, err := os.ReadFile(exitCodePath); err == nil {
		code, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return BuildCompleted, code
	}

	// Check heartbeat
	info, err := os.Stat(heartbeatPath)
	if err != nil {
		// No heartbeat and no exit_code — not started yet
		return BuildNotStarted, 0
	}

	// Heartbeat exists — check freshness
	age := time.Since(info.ModTime())
	if age > heartbeatTimeout {
		return BuildUnresponsive, 0
	}

	return BuildRunning, 0
}

func unresponsiveErr(isTTY bool) error {
	msg := fmt.Sprintf(
		"build VM unresponsive (no heartbeat for %v)",
		heartbeatTimeout)
	if isTTY {
		msg += "; use --shell to diagnose or ctrl-c to clean up"
	}
	return fmt.Errorf("%s", msg)
}

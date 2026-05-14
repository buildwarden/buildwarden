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

// Signal protocol between the build VM agent and the host orchestrator.
//
// The agent runs a watcher script that:
//  1. Forks the build process
//  2. While the process is alive, writes signal/heartbeat every interval
//     containing the current Unix timestamp
//  3. When the process exits, removes signal/heartbeat and writes
//     signal/exit_code containing the numeric exit code
//
// The orchestrator watches:
//   - If signal/heartbeat is removed AND signal/exit_code exists → build done
//   - If signal/heartbeat hasn't been updated within 3 intervals → unresponsive
//   - If signal/exit_code is "0" → success; otherwise → failure

const (
	heartbeatInterval = 2 * time.Second
	heartbeatTimeout  = 3 * heartbeatInterval // 6 seconds of silence = dead
)

// WatcherScript generates the shell script that runs inside the build VM.
// It forks the actual build command, monitors it, and maintains the
// heartbeat file on the shared volume.
func WatcherScript(buildCmd string) string {
	return fmt.Sprintf(`#!/bin/sh
SIGNAL_DIR="/shared/signal"
HEARTBEAT="$SIGNAL_DIR/heartbeat"
EXIT_CODE="$SIGNAL_DIR/exit_code"

# Clean state
rm -f "$HEARTBEAT" "$EXIT_CODE"

# Fork the build process
%s &
BUILD_PID=$!

# Heartbeat loop: write timestamp while build is alive
while kill -0 "$BUILD_PID" 2>/dev/null; do
    date +%%s > "$HEARTBEAT"
    sleep 2
done

# Build process exited — capture exit code
wait "$BUILD_PID"
CODE=$?

# Remove heartbeat (signals completion to orchestrator)
rm -f "$HEARTBEAT"

# Write exit code
echo "$CODE" > "$EXIT_CODE"
`, buildCmd)
}

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
				if isTTY {
					return -1, fmt.Errorf(
						"build VM unresponsive (no heartbeat for %v); "+
							"use --shell to diagnose or ctrl-c to clean up",
						heartbeatTimeout)
				}
				return -1, fmt.Errorf(
					"build VM unresponsive (no heartbeat for %v)",
					heartbeatTimeout)

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

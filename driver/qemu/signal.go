package qemu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	heartbeatInterval = 2 * time.Second
	heartbeatTimeout  = 3 * heartbeatInterval
)

// safeReadFile reads a file only if it is a regular file (not a symlink,
// FIFO, or device). Prevents the build VM from tricking the host into
// reading arbitrary files via symlink attacks on the signal directory.
func safeReadFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	return os.ReadFile(path)
}

func watcherScript(buildCmd string) string {
	return fmt.Sprintf(`#!/bin/sh
SIGNAL_DIR="/signal"
HEARTBEAT="$SIGNAL_DIR/heartbeat"
EXIT_CODE="$SIGNAL_DIR/exit_code"
BUILD_LOG="$SIGNAL_DIR/build.log"

export PATH="/agent:$PATH"

rm -f "$HEARTBEAT" "$EXIT_CODE" "$BUILD_LOG"

%s > "$BUILD_LOG" 2>&1 &
BUILD_PID=$!

while kill -0 "$BUILD_PID" 2>/dev/null; do
    date +%%s > "$HEARTBEAT"
    sleep 2
done

wait "$BUILD_PID"
CODE=$?

rm -f "$HEARTBEAT"
echo "$CODE" > "$EXIT_CODE"
poweroff -f 2>/dev/null || true
`, buildCmd)
}

func waitForBuild(ctx context.Context, signalDir string, isTTY bool) (int, error) {
	heartbeatPath := filepath.Join(signalDir, "heartbeat")
	exitCodePath := filepath.Join(signalDir, "exit_code")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			// Check completion (safe read: reject symlinks/FIFOs)
			if data, err := safeReadFile(exitCodePath); err == nil {
				code, _ := strconv.Atoi(strings.TrimSpace(string(data)))
				return code, nil
			}

			// Check heartbeat (Lstat: don't follow symlinks)
			info, err := os.Lstat(heartbeatPath)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}

			if time.Since(info.ModTime()) > heartbeatTimeout {
				msg := fmt.Sprintf(
					"build VM unresponsive (no heartbeat for %v)",
					heartbeatTimeout)
				if isTTY {
					msg += "; use --shell to diagnose"
				}
				return -1, fmt.Errorf("%s", msg)
			}
		}
	}
}

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

func watcherScript(buildCmd string) string {
	return fmt.Sprintf(`#!/bin/sh
SIGNAL_DIR="/shared/signal"
HEARTBEAT="$SIGNAL_DIR/heartbeat"
EXIT_CODE="$SIGNAL_DIR/exit_code"

rm -f "$HEARTBEAT" "$EXIT_CODE"

%s &
BUILD_PID=$!

while kill -0 "$BUILD_PID" 2>/dev/null; do
    date +%%s > "$HEARTBEAT"
    sleep 2
done

wait "$BUILD_PID"
CODE=$?

rm -f "$HEARTBEAT"
echo "$CODE" > "$EXIT_CODE"
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
			// Check completion
			if data, err := os.ReadFile(exitCodePath); err == nil {
				code, _ := strconv.Atoi(strings.TrimSpace(string(data)))
				return code, nil
			}

			// Check heartbeat
			info, err := os.Stat(heartbeatPath)
			if err != nil {
				continue // not started yet
			}

			if time.Since(info.ModTime()) > heartbeatTimeout {
				if isTTY {
					return -1, fmt.Errorf(
						"build VM unresponsive (no heartbeat for %v); "+
							"use --shell to diagnose or ctrl-c to clean up",
						heartbeatTimeout)
				}
				return -1, fmt.Errorf(
					"build VM unresponsive (no heartbeat for %v)", heartbeatTimeout)
			}
		}
	}
}

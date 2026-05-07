#!/bin/bash
# watchdog-build.sh — Runs warden build with a stall-detection watchdog.
# Kills the build if the log file hasn't been updated in STALL_TIMEOUT seconds.
# Also enforces an overall MAX_TIMEOUT.

set -o pipefail

STALL_TIMEOUT=${STALL_TIMEOUT:-600}   # 10 minutes without log output = stall
MAX_TIMEOUT=${MAX_TIMEOUT:-14400}      # 4 hours max total runtime
LOG_FILE="warden-tf-run.log"

echo "=== BuildWarden TF Build with Watchdog ==="
echo "  Stall timeout: ${STALL_TIMEOUT}s"
echo "  Max timeout:   ${MAX_TIMEOUT}s"
echo "  Log file:      ${LOG_FILE}"
echo "  Started:       $(date)"
echo ""

# Start the build in background, tee to log
WARDEN_CTR_CLI=finch ./warden build -f Dockerfile.tfexample-aarch64 ./buildctx 2>&1 | tee "$LOG_FILE" &
BUILD_PID=$!

START_TIME=$(date +%s)

# Watchdog loop
while kill -0 "$BUILD_PID" 2>/dev/null; do
    sleep 30

    NOW=$(date +%s)
    ELAPSED=$((NOW - START_TIME))

    # Check max timeout
    if [ "$ELAPSED" -ge "$MAX_TIMEOUT" ]; then
        echo ""
        echo "!!! MAX TIMEOUT (${MAX_TIMEOUT}s) exceeded. Killing build."
        kill "$BUILD_PID" 2>/dev/null
        wait "$BUILD_PID" 2>/dev/null
        # Clean up finch containers
        finch ps -a -q | xargs -r finch rm -f 2>/dev/null
        finch network ls --format '{{.Name}}' | grep warden | xargs -r -I{} finch network rm {} 2>/dev/null
        exit 124
    fi

    # Check stall (log file not modified)
    if [ -f "$LOG_FILE" ]; then
        LAST_MOD=$(stat -f %m "$LOG_FILE" 2>/dev/null || stat -c %Y "$LOG_FILE" 2>/dev/null)
        STALL_DURATION=$((NOW - LAST_MOD))
        if [ "$STALL_DURATION" -ge "$STALL_TIMEOUT" ]; then
            echo ""
            echo "!!! STALL DETECTED: No log output for ${STALL_DURATION}s (threshold: ${STALL_TIMEOUT}s)"
            echo "    Last 5 lines of log:"
            tail -5 "$LOG_FILE"
            echo ""
            echo "    Killing build."
            kill "$BUILD_PID" 2>/dev/null
            wait "$BUILD_PID" 2>/dev/null
            # Clean up finch containers
            finch ps -a -q | xargs -r finch rm -f 2>/dev/null
            finch network ls --format '{{.Name}}' | grep warden | xargs -r -I{} finch network rm {} 2>/dev/null
            exit 125
        fi
    fi
done

# Build finished naturally
wait "$BUILD_PID"
EXIT_CODE=$?
ELAPSED=$(( $(date +%s) - START_TIME ))

echo ""
echo "=== Build completed ==="
echo "  Exit code: $EXIT_CODE"
echo "  Duration:  ${ELAPSED}s ($(( ELAPSED / 60 ))m)"
echo "  Finished:  $(date)"

exit $EXIT_CODE

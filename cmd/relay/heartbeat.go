package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// signalDir is the directory where the relay writes heartbeat/exit signals.
// Visible to the host via the 9p shared volume.
var signalDir string

// lastActivity is updated every time the relay processes a request.
// The heartbeat writer uses this to auto-heartbeat during active traffic.
var lastActivity atomic.Int64

func SetSignalDir(dir string) { signalDir = dir }

// TouchActivity records that the relay is actively processing traffic.
// Called from request handlers.
func TouchActivity() {
	lastActivity.Store(time.Now().Unix())
}

// RunHeartbeat writes a heartbeat file at regular intervals as long as
// the relay is active (processing requests or receiving explicit beats).
// The host-side driver watches this file to determine build liveness.
func RunHeartbeat() {
	if signalDir == "" {
		return
	}

	hbPath := filepath.Join(signalDir, "heartbeat")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ts := lastActivity.Load()
		if ts == 0 {
			continue
		}
		// Only write if activity occurred within the last interval
		if time.Since(time.Unix(ts, 0)) < 5*time.Second {
			_ = os.WriteFile(hbPath, []byte(fmt.Sprintf("%d\n", ts)), 0644)
		}
	}
}

// WriteExitCode writes the build's exit code to the signal directory.
// Called when the relay receives an exit signal from the build VM.
func WriteExitCode(code int) {
	if signalDir == "" {
		return
	}
	ecPath := filepath.Join(signalDir, "exit_code")
	_ = os.WriteFile(ecPath, []byte(fmt.Sprintf("%d\n", code)), 0644)
	// Remove heartbeat to signal completion
	_ = os.Remove(filepath.Join(signalDir, "heartbeat"))
}

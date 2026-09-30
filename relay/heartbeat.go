package relay

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func (r *Relay) runHeartbeat() {
	if r.cfg.SignalDir == "" {
		return
	}

	hbPath := filepath.Join(r.cfg.SignalDir, "heartbeat")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ts := r.lastActivity.Load()
		if ts == 0 {
			continue
		}
		if time.Since(time.Unix(ts, 0)) < 5*time.Second {
			_ = os.WriteFile(hbPath, []byte(fmt.Sprintf("%d\n", ts)), 0644)
		}
	}
}

func (r *Relay) writeExitCode(code int) {
	// File signal for the shared-dir drivers (qemu/vz poll SignalDir for the
	// exit_code file). Skipped on a diskless relay (SignalDir empty).
	if r.cfg.SignalDir != "" {
		ecPath := filepath.Join(r.cfg.SignalDir, "exit_code")
		_ = os.WriteFile(ecPath, []byte(fmt.Sprintf("%d\n", code)), 0644)
		_ = os.Remove(filepath.Join(r.cfg.SignalDir, "heartbeat"))
	}
	// Sink signal for the diskless drivers (Hyper-V httpSink -> collector
	// /v1/complete). localSink.PostComplete is a no-op, so calling this for every
	// driver is safe and keeps completion delivery in one place. Bounded inside
	// postSignal (15s), so a stuck collector can't hang the exit path.
	//
	// Flush the ledger BEFORE signaling completion: the orchestrator tears down
	// the relay VM the moment it sees /v1/complete, so the ledger (and, for the
	// httpSink, its PUT to the collector) must already be done. finishAndFlush is
	// idempotent, so a later Wait/Stop won't double-finish.
	r.finishAndFlush()
	if err := r.PostComplete(code, "", ""); err != nil {
		log.Printf("relay: PostComplete(%d) failed: %v", code, err)
	}
}

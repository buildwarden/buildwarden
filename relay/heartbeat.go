package relay

import (
	"fmt"
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
	if r.cfg.SignalDir == "" {
		return
	}
	ecPath := filepath.Join(r.cfg.SignalDir, "exit_code")
	_ = os.WriteFile(ecPath, []byte(fmt.Sprintf("%d\n", code)), 0644)
	_ = os.Remove(filepath.Join(r.cfg.SignalDir, "heartbeat"))
}

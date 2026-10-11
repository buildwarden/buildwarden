package relay

import (
	"log"
	"time"
)

// Liveness: the build guest pings http://artifacts/heartbeat, which marks
// activity; while activity is recent the relay forwards a heartbeat to the
// collector every interval, and the orchestrator treats a stale heartbeat as
// a hung guest.
const heartbeatInterval = 2 * time.Second

func (r *Relay) runHeartbeat() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for range ticker.C {
		ts := r.lastActivity.Load()
		if ts == 0 || time.Since(time.Unix(ts, 0)) >= 5*time.Second {
			continue
		}
		if err := r.sink.postHeartbeat(); err != nil {
			log.Printf("relay: heartbeat: %v", err)
		}
	}
}

// complete finalizes the build: the ledger is flushed to the collector BEFORE
// completion is signalled, because the orchestrator tears the relay down the
// moment it sees /v1/complete.
func (r *Relay) complete(code int) {
	r.finishAndFlush()
	if err := r.sink.postComplete(code, "", ""); err != nil {
		log.Printf("relay: posting completion (%d): %v", code, err)
	}
}

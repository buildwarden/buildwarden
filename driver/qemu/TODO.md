# QEMU Driver — Complete

All items implemented and validated.

## Must-have — Done

- [x] COPY with globs/directories
- [x] Context listing endpoint
- [x] Error surfacing
- [x] Ubuntu/Debian cloud image validation

## Should-have — Done

- [x] Build timeout (--timeout flag)
- [x] Relay binary staleness detection
- [x] Cleanup on failure (signal handling)
- [x] Multi-file COPY destination semantics

## Nice-to-have — Done

- [x] Progress output (preparing/starting/running status)
- [x] `warden clean` (removes cached images + binaries, reports size)
- [x] Image integrity verification (SHA256 sidecar, re-downloads on mismatch)
- [x] Parallel relay+build binary compilation

## Future: Relay DoS Resilience

The relay is vulnerable to availability attacks from the build VM (not
escape, but can prevent builds from completing). Consider:

- [ ] Per-source connection limit (max concurrent from build VM)
- [ ] Request timeout (kill slow/stalled upstream fetches after N seconds)
- [ ] DNS query rate limit (mitigates DNS tunneling bandwidth)
- [ ] HTTP request body size limit (prevent memory exhaustion)
- [ ] Signal dir size quota (prevent host tmpdir exhaustion via 9p writes)
- [ ] Watchdog: relay auto-restarts if it crashes (supervisor in init)

These are availability hardening, not isolation fixes. The security
boundary (no unrecorded egress, no host access) is intact without them.

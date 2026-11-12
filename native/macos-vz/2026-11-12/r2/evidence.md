# hyperv-relay-per-build-channel f6-validate-macos-vz-r2 — host:macos-vz/20261112T213545Z (attended, Jeff)

Re-run after Implement r4 (consolidated at 5ac5532). macbook, macOS
27.2.1 host, vz active image macos-0181f7e0b146845c (27.2.1
26B131). Branch 5ac5532 and main 6b2ab14, each `make build sign` in its own
.scratch worktree. Since the r1 pass on 59d7440 the vz/relay path changed only
in driver/hostutil.go and cmd/warden/config.go; all cases re-run. Same
build.sh (`curl https://example.com/`) and forwarder (../dnsfwd.py, 127.0.0.1:53
to 1.1.1.1, run under Jeff's sudo) as r1.

- **main baseline:** vz build SUCCEEDED, fetch 200, ledger verifies (11
  signatures, all channels closed).
- **Case 1 PASS:** build SUCCEEDED, ledger verifies (11). Relay `upstream
  resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1 (claim)`; doctor `relay resolvers:
  8.8.8.8, 8.8.4.4, 1.1.1.1 (from /etc/resolv.conf)`.
- **Case 2 PASS:** `[vm] dns = ["9.9.9.9"]`: doctor and relay list only
  9.9.9.9, fetch 200, ledger verifies, relay.log names none of the Mac's
  resolvers. `["192.0.2.53"]`: relay lists only 192.0.2.53, 51 `did not
  answer` timeouts, `curl: (6) Could not resolve host`, build FAILED (exit 6),
  no Mac resolver in the log (no fallback), ledger verifies (7).
- **Case 3 PASS:** `[vm] dns = ["127.0.0.1"]`: doctor `relay resolvers:
  127.0.0.1 (from [vm] dns)`, relay `upstream resolvers: 127.0.0.1 (claim)`,
  build SUCCEEDED (fetch 200), ledger verifies (11); the forwarder logged 43
  queries during the build, 9 for example.com (dnsfwd.log).

Follow-ups filed: none.

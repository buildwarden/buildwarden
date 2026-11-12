# hyperv-relay-per-build-channel f6-validate-macos-vz-r3 — host:macos-vz/20261112T222605Z (attended, Jeff)

Re-run after Implement r5 (consolidated at 2080a28; r5 changed only
driver/hyperv scripts and docs, nothing on the vz or relay path). macbook,
macOS 27.2.1 host, vz active image macos-0181f7e0b146845c
(27.2.1). Branch 2080a28 and main 6b2ab14, `make build sign` in .scratch
worktrees. Same build.sh (`curl https://example.com/`) and forwarder
(../dnsfwd.py, 127.0.0.1:53 to 1.1.1.1, under Jeff's sudo) as r1/r2.

- **main baseline:** vz build SUCCEEDED, fetch 200, ledger verifies (11).
- **Case 1 PASS:** build SUCCEEDED, ledger verifies (11); relay `upstream
  resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1 (claim)`; doctor `relay resolvers:
  8.8.8.8, 8.8.4.4, 1.1.1.1 (from /etc/resolv.conf)`.
- **Case 2 PASS:** `["9.9.9.9"]` used alone, fetch 200, ledger verifies;
  `["192.0.2.53"]` alone, 43 `did not answer` timeouts, `curl: (6) Could not
  resolve host`, build FAILED; no Mac resolver in either log (no fallback).
- **Case 3 PASS:** `["127.0.0.1"]`: doctor and relay list only 127.0.0.1,
  build SUCCEEDED (fetch 200), ledger verifies (11); the forwarder logged 40
  queries during the build, 3 for example.com (dnsfwd.log).

Follow-ups filed: none.

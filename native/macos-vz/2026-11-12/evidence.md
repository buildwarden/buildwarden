# hyperv-relay-per-build-channel f6-validate-macos-vz — host:macos-vz/20261112T184819Z (attended, Jeff)

macbook, macOS 27.2.1 host, vz active image
macos-0181f7e0b146845c (macOS 27.2.1 26B131). Branch
feat/hyperv-relay-per-build-channel 59d7440 and main 1173c5f, each `make
build sign` in its own .scratch worktree. Mac resolvers (/etc/resolv.conf):
8.8.8.8, 8.8.4.4, 1.1.1.1. Script build.sh: `curl https://example.com/`.
Each log has the build output and the relay's relay.log.

- **main baseline:** vz build SUCCEEDED, fetch 200, ledger verifies (11
  signatures, all channels closed).
- **Case 1 PASS:** vz build SUCCEEDED, fetch 200, ledger verifies (11
  signatures). Host relay: `upstream resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1
  (claim)`; `doctor --driver vz`: `relay resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1
  (from /etc/resolv.conf)`.
- **Case 2 PASS:** `[vm] dns = ["9.9.9.9"]`: doctor `relay resolvers: 9.9.9.9
  (from [vm] dns)`, relay `upstream resolvers: 9.9.9.9 (claim)`, build
  SUCCEEDED (fetch 200), ledger verifies; relay.log names none of the Mac's
  resolvers. `[vm] dns = ["192.0.2.53"]`: relay lists only 192.0.2.53, 43
  `upstream resolver 192.0.2.53:53 did not answer … i/o timeout` lines,
  guest `curl: (6) Could not resolve host: example.com`, build FAILED (exit
  6); no mention of 8.8.8.8, 8.8.4.4 or 1.1.1.1 (no fallback); ledger
  verifies (7 signatures).
- **Case 3 PASS:** a logging forwarder (dnsfwd.py: 127.0.0.1:53 only, UDP and
  TCP, to 1.1.1.1; run under Jeff's sudo, since binding port 53 needs root
  here). `[vm] dns = ["127.0.0.1"]`: doctor `relay resolvers: 127.0.0.1 (from
  [vm] dns)`, relay `upstream resolvers: 127.0.0.1 (claim)`, build SUCCEEDED
  (fetch 200), ledger verifies (11 signatures). dnsfwd.log shows the build's
  queries arriving during the build (example.com x3, plus the guest's Apple
  service names and the relay's own reverse/_dns.resolver.arpa probes).

Follow-ups filed: none.

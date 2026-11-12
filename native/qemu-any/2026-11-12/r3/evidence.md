# hyperv-relay-per-build-channel f6-validate-qemu-any-r3 — host:qemu-any/20261112T222605Z (attended)

Re-run after Implement r5 (consolidated at 2080a28; r5 changed only
driver/hyperv scripts and docs, nothing under driver/qemu, tools/relay-vm,
tools/build-vm, cmd or relay). macbook, macOS 27.2.1, QEMU emulator version 11.1.1.
Branch 2080a28 and main 6b2ab14 in .scratch worktrees, each with `make build
sign` and both initramfs scripts (relay initramfs rebuilt from the branch).
Same build.sh (HTTP APKINDEX fetch) and alpine_3_21.qcow2 as r1/r2. Claim
tokens redacted.

- **main baseline:** direct-boot and alpine disk builds SUCCEEDED, ledgers
  verify (11 signatures).
- **Case 1 PASS:** direct-boot and disk builds SUCCEEDED, ledgers verify (11
  each); relay `upstream resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1 (claim)`,
  upstream eth1 by DHCP; doctor `relay resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1
  (from /etc/resolv.conf)`.
- **Case 2 PASS:** `["9.9.9.9"]` used alone, build SUCCEEDED, ledger
  verifies; `["192.0.2.53"]` alone, `did not answer` timeout, `wget: bad
  address`, build FAILED; no Mac resolver in either log (no fallback).
- **Case 3 PASS:** `["127.0.0.1"]`: doctor BLOCKED, build refused before any
  VM ("…which the relay cannot use (loopback)", with its fix); no QEMU boot.

Follow-ups filed: none.

# hyperv-relay-per-build-channel f6-validate-qemu-any-r2 — host:qemu-any/20261112T214209Z (attended)

Re-run after Implement r4 (consolidated at 5ac5532). macbook, macOS
27.2.1, QEMU emulator version 11.1.1. Branch 5ac5532 and main 6b2ab14, each in its
own .scratch worktree with `make build sign`, `tools/relay-vm/build-initramfs.sh`
and `tools/build-vm/build-initramfs.sh` (relay initramfs rebuilt from the
branch). Since the r1 pass on 59d7440 the qemu/relay path changed only in
driver/hostutil.go and cmd/warden (config, clean); all cases re-run. Same
build.sh (HTTP APKINDEX fetch) and alpine_3_21.qcow2 as r1. Claim tokens
redacted.

- **main baseline:** direct-boot and alpine disk builds SUCCEEDED, ledgers
  verify (11 signatures); relay on QEMU's forwarder (`init: dns=nameserver
  10.0.2.3`).
- **Case 1 PASS:** direct-boot and alpine disk builds SUCCEEDED, ledgers
  verify (11 each); relay upstream `nat-if=eth1 inet 10.0.2.15/24` by DHCP
  (`warden.net=dhcp`), `upstream resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1
  (claim)`; doctor `relay resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1 (from
  /etc/resolv.conf)`.
- **Case 2 PASS:** `[vm] dns = ["9.9.9.9"]`: doctor and relay list only
  9.9.9.9, build SUCCEEDED, ledger verifies, no Mac resolver in the log.
  `["192.0.2.53"]`: relay lists only 192.0.2.53, `did not answer` timeouts,
  guest `wget: bad address`, build FAILED, no Mac resolver in the log (no
  fallback), ledger verifies (7).
- **Case 3 PASS:** `[vm] dns = ["127.0.0.1"]`: doctor BLOCKED and the build
  refused before any VM ("[vm] dns lists 127.0.0.1, which the relay cannot use
  (loopback)", "To fix: change or remove that entry in [vm] dns"); no QEMU
  boot, no output directory.

Follow-ups filed: none.

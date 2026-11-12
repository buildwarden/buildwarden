# hyperv-relay-per-build-channel f6-validate-qemu-any — host:qemu-any/20261112T184026Z (attended)

macbook, macOS 27.2.1, QEMU emulator version 11.1.1, Go go1.26.5.
Branch feat/hyperv-relay-per-build-channel 59d7440, main 1173c5f, each in its
own .scratch worktree: `make build sign`, `tools/relay-vm/build-initramfs.sh`
and `tools/build-vm/build-initramfs.sh` from that tree (relay initramfs
rebuilt from the branch's tools/relay-vm/init). Mac resolvers
(/etc/resolv.conf): 8.8.8.8, 8.8.4.4, 1.1.1.1. Script build.sh fetches
http://dl-cdn.alpinelinux.org/alpine/v3.21/main/aarch64/APKINDEX.tar.gz (HTTP:
the direct-boot guest's busybox wget cannot do TLS through the relay, on main
and the branch alike); cloud image = cached alpine_3_21.qcow2. Claim tokens
redacted in the logs.

- **main baseline:** direct-boot and alpine disk builds SUCCEEDED, ledgers
  verify (11 signatures, all channels closed); the relay used QEMU's DNS
  forwarder (`init: dns=nameserver 10.0.2.3`).
- **Case 1 PASS:** direct-boot and alpine disk builds SUCCEEDED, ledgers
  verify (11 signatures each). Relay console: `warden.net=dhcp`, `nat-if=eth1
  inet 10.0.2.15/24` (udhcpc lease; init stops without one), `relay: upstream
  resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1 (claim)`; `doctor --driver qemu`:
  `relay resolvers: 8.8.8.8, 8.8.4.4, 1.1.1.1 (from /etc/resolv.conf)`.
- **Case 2 PASS:** `[vm] dns = ["9.9.9.9"]`: doctor `relay resolvers: 9.9.9.9
  (from [vm] dns)`; relay `upstream resolvers: 9.9.9.9 (claim)`; build
  SUCCEEDED, ledger verifies. `[vm] dns = ["192.0.2.53"]` (no server): relay
  lists only 192.0.2.53, `upstream resolver 192.0.2.53:53 did not answer …
  i/o timeout`, guest `wget: bad address 'dl-cdn.alpinelinux.org'`, build
  FAILED; no 8.8.8.8/8.8.4.4/1.1.1.1 anywhere in the log (no fallback);
  ledger verifies (7 signatures).
- **Case 3 PASS:** `[vm] dns = ["127.0.0.1"]`: doctor BLOCKED, build refused
  before any VM ("[vm] dns lists 127.0.0.1, which the relay cannot use
  (loopback)", "To fix: change or remove that entry in [vm] dns"); no QEMU
  started, no output directory.

Follow-ups filed: none.

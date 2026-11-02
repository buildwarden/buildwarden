# f6-validate-linux-kvm, first attempt (2026-11-02, unattended)

host:linux-kvm/20261102T231002Z on windows-desktop's WSL2 Ubuntu 22.04,
feat/firecracker-driver at 96ca21a (f3-implement done, f4-review in
progress). The step was claimed, released with the reason below, and now
needs the human step `f6-provision-linux-kvm-dns`. The spike's own evidence
from earlier today is `README.md`, `spike-runs.log` and `ledgers.log`.

## Setup

- `dist/warden` and `dist/warden-io` cross-compiled on the Windows host
  (GOOS=linux GOARCH=amd64 CGO_ENABLED=0) from a detached worktree of the
  branch; WSL's own go is 1.18 and cannot build the module.
- Boot assets: the branch's `tools/firecracker-vm/build-initramfs.sh` and
  `ARCH=x86_64 NO_MODULES=1 tools/relay-vm/build-initramfs.sh` ran in WSL
  (busybox-static 1.37.0-r14 pin verified). The kernel is the spike's
  `vmlinux` (Linux 6.1.191, sha256 169495b7…), since `build-kernel.sh`
  changed only its config-hash pin since the spike built it.
- Host: kernel 6.18.54.1-microsoft-standard-WSL2-warden+, jeff in kvm,
  Firecracker v1.17.0 at ~/.local/bin, passwordless sudo, FORWARD policy DROP
  (warden-fc-posture.service active), uplink eth0 MTU 1280, resolver
  10.255.255.254 only.

## What ran

| Check | Result | Log |
|---|---|---|
| `warden doctor` as jeff, before setup | FAIL on slots (expected) and upstream DNS; everything else ok | `f6-doctor.log` |
| `sudo warden setup --driver firecracker --slots 4` | ok: 16 interfaces created, bridge table and WARDEN-FC chains loaded, ip_forward=1 | `f6-doctor.log` |
| `sudo warden doctor` after setup | all ok (slots, sysctls, rules, posture, orphans) except upstream DNS | `f6-doctor.log` |
| setup re-run (idempotent) | ok: 0 created, 16 verified | `f6-setup-rerun.log` |
| setup with FORWARD policy ACCEPT (distro case) | refuses with the fix; policy restored to DROP | `f6-forward-accept.log` |
| loaded rules | WARDEN-FC-IN/FWD/OUT, WARDEN-FC6, WARDEN-FC-RAW, WARDEN-FC-NAT and bridge table warden_fc as designed (two-way MSS clamp, RELATED, OUTPUT 8301 by skuid) | `f6-rules-nft.log` |
| `warden image prepare --driver firecracker` (local Ubuntu 24.04 cloud image) | ok, raw in 4.5 s | `f6-image-prepare.log` |
| direct-boot build | FAILED in preflight: upstream DNS (host has only the local resolver 10.255.255.254) | `f6-direct-build-preflight.log` |
| Ubuntu build, isolation, two slots, kill cases, local-user relay access, teardown, ledger | not run: same preflight | |
| firewalld distro case | not run: firewalld is not installed | |
| local-resolver distro case | demonstrated by the preflight refusal above | `f6-direct-build-preflight.log` |

## Why it stopped

Decision 5 excludes loopback and host-local resolvers, and WSL's default
resolver is its DNS-tunnelling address on `lo`. The fix the doctor prints is a
WSL configuration change (`/etc/wsl.conf` plus `/etc/resolv.conf`, or
`.wslconfig`), outside the checkout and the documented caches, which the
workflow-native Never list keeps an unattended session from making. The spike
avoided this by passing the Windows host's resolvers to its harness; the
driver reads `/etc/resolv.conf` and has no override.

## Cleanup

`warden clean --driver firecracker`, then `sudo sh tools/firecracker-vm/slots.sh down`
removed the four slots; the worktree and scratch output were deleted. Kept in
the documented cache: `~/.cache/warden/firecracker/images/noble-server-cloudimg-amd64-04e8f2461a16.raw`
(3.5 GB) for the next run.

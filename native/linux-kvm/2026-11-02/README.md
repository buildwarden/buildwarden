# firecracker-driver Phase 0 spike (f2-spike-linux-kvm)

Attended run, 2026-11-02, worker `host:linux-kvm/20261102T191851Z`, on
windows-desktop's WSL2 Ubuntu 22.04. Branch `feat/firecracker-driver` at
d31aa30. The findings are in the design's Rollback notes; this is the
evidence.

## Host

- Kernel `6.18.54.1-microsoft-standard-WSL2-warden+`: Microsoft's
  `linux-msft-wsl-6.18.54.1` with the running config plus `NF_TABLES_BRIDGE`
  (and `NFT_BRIDGE_META`, `NFT_BRIDGE_REJECT`, `NF_CONNTRACK_BRIDGE`), built
  in a Podman container from `%LOCALAPPDATA%\warden\wsl-kernel\build.sh` and
  set in `%USERPROFILE%\.wslconfig`. The stock WSL kernel has no bridge-family
  nf_tables, which Decision 4 needs.
- `/dev/kvm` as jeff (kvm group), `kvm_intel nested=Y`.
- Firecracker v1.17.0 x86_64 from the official release, sha256
  `06094a11…d558`, at `~/.local/bin/firecracker`. No jailer (Decision 11).
- Passwordless sudo for jeff (Jeff, 2026-11-02: WSL is used only for
  BuildWarden testing); slot setup is `sudo tools/firecracker-vm/slots.sh up`.
- `warden-fc-posture.service` (Jeff) sets the filter FORWARD policy to DROP at
  every WSL boot; WSL keeps no iptables state across restarts.
- Uplink `eth0` MTU 1280. Only resolver `10.255.255.254` (WSL DNS tunnelling,
  on `lo`); the spike passed the Windows host's `8.8.8.8,8.8.4.4`.

## Runs

`spike-runs.log` has the key lines of each run and `ledgers.log` each output's
`warden inspect` summary.

| Run | Path | Result |
|---|---|---|
| run1 | direct boot, `RUN echo hello` | pass: claim 1.05 s, build 0.47 s, 5 signatures valid |
| run-iso | direct boot, isolation | pass after the launcher's PATH fix: direct TCP and NTP fail, UDP send seen on no uplink packet, HTTP recorded, 9 signatures valid |
| run-ubuntu | Ubuntu 24.04 raw + CIDATA | boot, root `/dev/vda1`, kernel 6.1.191; HTTPS 502 (MTU) |
| run-ubuntu2 | Ubuntu, several fetches | HTTP 200 up to 255 KB; HTTPS 502 (MTU) before the MSS fix |
| run-tls2 | Ubuntu, HTTPS with captures | 200 after two-way MSS clamping and RELATED ICMP |
| run-big | Ubuntu, 7.4 MB HTTPS | pass: sha256 matches the published value, 13 signatures valid |
| fc-mem | run-big at 256/192/128 MiB relay | pass at every size, no OOM |

The capture that found the MTU problem: the relay's 1388-byte ClientHello
segment appeared on `wfc0u` and never on `eth0`, retransmitted until the
server closed (server SYN-ACK MSS 1400; uplink MTU 1280).

## Cleanup

Slot 0 removed (`slots.sh down`) at the end. Kept for f6 validation and
Implement: the built kernel and initramfs under `~/bw` (WSL), the Ubuntu raw
image `~/fc-spike/images/noble.raw` (3.5 GB sparse), the seed, and the WSL
kernel.

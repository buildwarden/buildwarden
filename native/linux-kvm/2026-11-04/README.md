# linux-kvm 2026-11-04: build-link-subnet f6-validate-linux-kvm

`host:linux-kvm/20261105T051002Z`, unattended, on windows-desktop's WSL2
Ubuntu 22.04 (kernel 6.18.54.1-microsoft-standard-WSL2-warden+, Firecracker
v1.17.0, resolvers 8.8.8.8 and 8.8.4.4, FORWARD policy DROP from
warden-fc-posture.service). Branch `feat/build-link-subnet` at **a68ac2c**;
main at 4c8f9dc for the baseline. The Validation plan's cases for linux-kvm
all pass except the in-guest flood-and-forge case, which was not run (below).

## Setup

- Binaries cross-compiled on the Windows host (`GOOS=linux GOARCH=amd64
  CGO_ENABLED=0`): `warden`, `warden-io-linux-amd64` and
  `warden-relay-linux-amd64` from the branch into `~/fcval/bin`, and from main
  into `~/fcval-main/bin` (`scripts/stage.sh`).
- **Pre-change assets** (built 2026-11-02 from feat/firecracker-driver 824abd7,
  kept under `~/bw`), staged beside both binaries under their release names:
  `firecracker-vmlinux` (6.1.191, sha256 169495b7…), the relay initramfs
  (sha256 b5b657e0…, its init adds `10.0.0.1/30` to the build interface) and
  the direct-boot launcher (sha256 ebaf4177…, no link-channel stamp).
- **New assets** built natively in WSL from the branch's scripts
  (`scripts/build-assets.sh`, `build-assets.log`): the launcher initramfs
  (sha256 ca0b66e1…) with its `link-channel` stamp `WARDEN_LINK_CHANNEL=1`,
  and the x86_64 relay initramfs (sha256 c7cf36e7…, `ARCH=x86_64
  NO_MODULES=1`). The kernel is unchanged on the branch
  (`tools/firecracker-vm/kernel.config` untouched), so the spike's vmlinux
  was reused.
- Image: the cached prepared ubuntu:24.04 raw
  (`noble-server-cloudimg-amd64-04e8f2461a16.raw`).
- `sudo warden setup --driver firecracker --slots 2` at the start of each
  phase: WSL stops the idle distro between scripts and the slots go with it
  (`logs/01-setup.log`, `09-setup.log`, `29-setup.log`; the drivers guide's
  "re-run setup after a reboot").

Each phase is a script under `scripts/`; `phase1.log`, `phase2.log` and
`phase3.log` are their consoles, and `logs/` the per-command logs (the build
logs are trimmed to their warden, init, relay and result lines).

## Results

| Case | Result | Log |
|---|---|---|
| main 4c8f9dc direct-boot build on the old assets (baseline) | SUCCEEDED; relay init `build-if=eth0 10.0.0.1/30`, guest `inet 10.0.0.2/30`, `default via 10.0.0.1` | `02-main-direct.log` |
| (1) branch doctor on the old assets | `firecracker direct-boot launcher initramfs: link-channel stamp missing; rebuild it once: tools/firecracker-vm/build-initramfs.sh`, reported only (exit 0, the stamp is not required) | `03-doctor-old.log` |
| (1) branch direct-boot build on the old launcher | refused before any VM: `predates the configurable build link and must be rebuilt once (no link-channel stamp at …firecracker-build-initramfs.cpio.gz.link-channel)`, fix `tools/firecracker-vm/build-initramfs.sh`; no firecracker process | `04-old-launcher-direct.log` |
| (1) branch ubuntu disk build on the OLD relay initramfs | SUCCEEDED. Relay console: old init `build-if=eth0 10.0.0.1/30`, then `relay: build-if eth0 100.127.255.253/30 from the claim; DHCP offers 100.127.255.254` (the init's address flushed); guest `100.127.255.254/30`, `default via 100.127.255.253`, HTTPS fetch 16337 bytes | `05-oldrelay-ubuntu.log` |
| (1, a14 d) new launcher, OLD relay initramfs, direct boot | SUCCEEDED; relay `DHCP OFFER/ACK 100.127.255.254`; guest console `init: link 100.127.255.254/30 via 100.127.255.253 from dhcp`, equal to the command line `warden.link_ip=100.127.255.254/30 warden.link_gw=100.127.255.253` | `07-oldrelay-direct.log` |
| (2) default link, new assets: doctor | `build link 100.127.255.252/30 (default)`, `launcher initramfs: link-channel stamp current`, all ok | `10-doctor-default.log` |
| (2) default link: direct-boot build | SUCCEEDED; `init: link 100.127.255.254/30 via 100.127.255.253 from dhcp`; `inspect --verify-disks`: vm-disk-chain/v1 verified, 11 signatures valid, all channels closed | `11-def-direct.log`, `13-inspect-out-def-direct.log` |
| (2) default link: ubuntu disk build | SUCCEEDED; guest `100.127.255.254/30`; verified, 11 signatures valid, all channels closed | `12-def-ubuntu.log`, `13-inspect-out-def-ubuntu.log` |
| (3) `--build-link 100.126.255.252/30`: doctor | `build link 100.126.255.252/30 (--build-link)`, stamp current, no rebuild asked | `14-doctor-alt.log` |
| (3) configured link: direct-boot and ubuntu builds, no rebuild | both SUCCEEDED; relay `build-if eth0 100.126.255.253/30 from the claim; DHCP offers 100.126.255.254`; direct boot `init: link 100.126.255.254/30 via 100.126.255.253 from dhcp`; guest at 100.126.255.254 on both; both ledgers verified, 11 signatures, all channels closed | `15-alt-direct.log`, `16-alt-ubuntu.log`, `17-inspect-*.log` |
| (3) `[vm] build_link` in `./warden.toml` | doctor `build link 100.126.255.252/30 (warden.toml)`; direct-boot build SUCCEEDED at 100.126.255.254 from dhcp; `--build-link 100.125.255.252/30` wins over the key | `18-doctor-toml.log`, `19-toml-direct.log`, `phase2.log` |
| a bad `[vm] build_link` fails at load | `100.126.255.253/30 is not a network address; did you mean 100.126.255.252/30?`, exit 1 | `20-doctor-badtoml.log` |
| (4) `--build-link 100.64.88.0/30` | refused at startup: `overlaps the firecracker driver's upstream range (100.64.88.0/22)` | `21-overlap.log` |
| (4) `--build-link 172.19.128.44/30` (holds eth0's 172.19.128.47) | doctor `BLOCKED: host interface address 172.19.128.47/20 is inside the link`; build refused before any VM with the `[vm] build_link` fix | `22-hostoverlap.log` |
| host resolver inside the configured link (only `100.126.255.253`) | doctor FAIL `only resolvers it cannot reach: 100.126.255.253`, fix names a resolver outside the link or another `[vm] build_link`; build refuses in preflight, no VM | `23-doctor-inlink.log`, `24-build-inlink.log` |
| in-link resolver plus 8.8.8.8 | `ok upstream DNS for the relay: 8.8.8.8` (the in-link one excluded) | `25-doctor-inlink-mixed.log` |
| resolver inside the default link only | FAIL, same fix for 100.127.255.252/30 | `26-doctor-inlink-default.log` |
| resolver `10.0.0.2` (the old link) | now passes: `ok upstream DNS for the relay: 10.0.0.2` | `27-doctor-oldlink.log` |
| (a14 d) DISCOVER from the host on the slot's upstream TAP `wfc0u` during a build | no BOOTREPLY in 8 s | `32-probe-upstream.log` |
| control: DISCOVER from the host on the relay-side TAP `wfc0r` | OFFER yiaddr=100.127.255.254 server-id=100.127.255.253 router=100.127.255.253 mask=255.255.255.252; the relay logged `DHCP OFFER 100.127.255.254 to 76:90:a8:97:dc:39` (the probe's MAC): the responder is live on the build interface only | `33-probe-relayside.log`, `31-sec-build.log` |
| (TEST-08) build fetching the link's relay address `http://100.127.255.253/` through the proxy | relay `upstream error: blocked: connection to 100.127.255.253 (100.127.255.253) denied`, guest got 502; `http://100.127.255.254/` (its own address) connection refused; the next fetch (pypi, 16337 bytes) succeeded; 15 signatures valid, all channels closed | `31-sec-build.log`, `35-inspect-sec.log` |
| guest-sourced egress (TCP 1.1.1.1:22, UDP 1.1.1.1:9) | TCP blocked; the UDP send is accepted locally and nothing guest-sourced reached the upstream TAP: the host's `WARDEN-FC-FWD` DROP counters for `wfc0u` stayed 0 and only the relay's own `100.64.88.2` traffic was forwarded | `30-counters-before.log`, `34-counters-after.log` |

The resolver cases ran warden as jeff in a private mount namespace with a
temporary resolv.conf bind-mounted over `/etc/resolv.conf` (as the 2026-11-02
lan-resolver run did); the host file was unchanged throughout and no bind
mount was left.

## Not run

- **Flood-and-forge (TEST-08, a14 d):** a build script flooding DISCOVER and
  REQUEST with random MACs and sending a forged BOOTREPLY and RELEASE. The
  unattended session could not produce the packet-forging guest script (its
  tooling withheld it as attack tooling). The responder's behaviour it would
  exercise is shown by the two probes above (one fixed offer, bound to the
  build interface) and by tier 0 (`cmd/relay/dhcp_test.go`, fuzzing). Needs an
  attended run, or security-suite's Arm A corpus (Consolidation handed it the
  Security note's list).
- **A claim whose build_link overlaps the relay's upstream interface:**
  unreachable from the CLI, which refuses `100.64.88.0/30` first; covered at
  tier 0 (`cmd/relay/vmlink_test.go`).

## Cleanup

`warden clean --driver firecracker` (cache kept, 7.8 GB), `slots.sh down`
(0 wfc interfaces, FORWARD DROP), no firecracker processes, work dir empty.
`~/fcval`, `~/fcval-main` and `~/bwb` removed at the end of the session; the
prepared raw image stays in `~/.cache/warden/firecracker/images`.

# build-link-subnet f6-validate-qemu-any — host:qemu-any/20261105T124003Z

Unattended, on the macbook (qemu-any is homed here in hosts.yaml). macOS 27.0.1
(26A434), arm64. QEMU 11.1.1 (Homebrew), Go 1.26.5. Branch feat/build-link-subnet
at a68ac2c (contains main 4c8f9dc). Binaries from `make -C .scratch/build-link-subnet build`.
Relay initramfs and the direct-boot launcher rebuilt from the branch with
`make relay-vm` and `tools/build-vm/build-initramfs.sh` (link-channel stamp
WARDEN_LINK_CHANNEL=1). main baselines built with the main checkout's binaries.

Hand boots and verbose build consoles are in this directory:
direct-default-verbose.log, direct-old-relay-verbose.log,
alpine-old-relay-verbose.log, handboot-keys.log, handboot-nokeys.log.

## (1) Upgrade path (pre-change assets in place)

- Old (unstamped) direct-boot initramfs from main: `doctor --driver qemu`
  reports "qemu direct-boot initramfs: link-channel stamp missing; rebuild it
  once: tools/build-vm/build-initramfs.sh" (reported only, exit 0). A
  direct-boot build is refused before any VM:
  "resolving build assets: qemu direct-boot initramfs predates the configurable
  build link and must be rebuilt once (no link-channel stamp ...) To fix:
  tools/build-vm/build-initramfs.sh".
- Old relay initramfs from main, with the new binaries (alpine disk build,
  default link): SUCCEEDED. Relay console: "build-if eth0 100.127.255.253/30
  from the claim; DHCP offers 100.127.255.254" — 10.0.0.1 is gone from the
  relay's build interface, the relay addresses it from the claim. Guest at
  100.127.255.254. inspect --verify-disks: 35 signatures valid, all channels
  closed. (alpine-old-relay-verbose.log)

## (2) Default link after the one-time rebuild

- direct-boot: SUCCEEDED, guest 100.127.255.254, console "init: link
  100.127.255.254/30 via 100.127.255.253 from dhcp". inspect: 11 signatures
  valid, all channels closed. (direct-default-verbose.log)
- alpine disk build: SUCCEEDED, guest 100.127.255.254. inspect: 35 signatures
  valid, all channels closed.

## (3) Configured link 100.126.255.252/30, no rebuild

- doctor from a dir with [vm] build_link = 100.126.255.252/30 reports
  "build link 100.126.255.252/30 (warden.toml)"; with --build-link
  100.125.255.252/30 it reports "(--build-link)" — the flag wins.
- direct-boot --build-link 100.126.255.252/30: SUCCEEDED, guest
  100.126.255.254 from dhcp, relay "build-if eth0 100.126.255.253/30 from the
  claim". inspect: 11 signatures valid, all channels closed.
- alpine disk build --build-link 100.126.255.252/30: SUCCEEDED, guest
  100.126.255.254. inspect: 35 signatures valid, all channels closed. No
  rebuild, refresh or re-prepare was done between (2) and (3).

## (4) Overlap / malformed refused at startup

`doctor --driver qemu --build-link X` (build refuses identically):
- 10.0.2.0/30 — overlaps qemu's user-mode network (10.0.2.0/24)
- 100.64.88.0/30 — overlaps the firecracker driver's upstream range (100.64.88.0/22)
- 100.126.0.1/30 — not a network address; did you mean 100.126.0.0/30
- 10.0.0.28/30 — BLOCKED: host interface address 10.0.0.30/8 is inside the link
  (the host's own address; names the fix)
- 10.0.0.0/30 — warning only: host network 10.0.0.30/8 contains the link
  (exit 0)

## a14 (d) vm-mode DHCP

- Every direct-boot console shows the link "from dhcp", equal to the command
  line, on both links: default (direct-default-verbose.log, direct-old-relay
  on the old relay) and configured (100.126.255.254). The old-relay-initramfs
  direct boot took its lease too ("DHCP OFFER/ACK 100.127.255.254",
  "init: link ... from dhcp"). (direct-old-relay-verbose.log)
- Hand boot of the new direct-boot initramfs with the keys and no relay (a
  socket netdev to nowhere): "init: no DHCP answer; using warden.link_ip",
  "init: link 100.127.255.254/30 via 100.127.255.253 from cmdline", held the
  keys' address. (handboot-keys.log)
- Hand boot without keys and no relay: the init took the stop path and exited 1
  (kernel panic "Attempted to kill init! exitcode=0x00000100", = exit 1). The
  init's echoed stop line did not flush to the serial console before the
  immediate exit+panic in this bare hand boot (no warden-io to keep the
  console alive); the exit code is the evidence the no-lease/no-keys branch
  ran. (handboot-nokeys.log)

## Link block (TEST-08, non-forging part)

On both the default and configured link, an in-guest fetch of the relay's own
link address through the proxy is refused: relay "upstream error: blocked:
connection to 100.127.255.253 (100.127.255.253) denied", the guest's wget
returns rc 1; the next fetch (https://example.com/) still succeeds. Direct
egress blocked.

## Not run

- The in-guest flood-and-forge DHCP case (a14 (d), TEST-08): the unattended
  session did not write the packet-forging build script, the same as the
  windows-hyperv and linux-kvm runs. The design's Consolidation hands the
  Security note's forging cases to security-suite.

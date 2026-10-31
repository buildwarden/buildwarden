---
feature: firecracker-driver
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # firecracker is new; qemu loses its Linux-guest seed and cloud-image code to driver/linuxguest; hyperv and qemu share the relay VM init and initramfs build; container and vz declared for the cmd/warden wiring and any driver/ root edit (TEST-02 caveat)
seams: [bootstrap-model]   # the per-driver mapping in bootstrap.md gains a firecracker row; every invariant unchanged (see Bootstrap comparison, question a2)
surfaces: [cli, config]    # --driver firecracker and runtime.driver = "firecracker"; warden setup and warden doctor for the driver
security_boundary: host-env  # also env-relay; see "Security note"
alters_relationships: true   # conservative: a new driver and a new bootstrap-model mapping row (a2); no relay, executor, collector or Driver-contract change
design_review: required
native_validation:
  - {host: linux-kvm, blocking: true, reason: "driver/firecracker runs only on a Linux host with /dev/kvm; no CI runner validates it today (question a8)"}
  - {host: qemu-any, blocking: false, reason: "regression: Linux-guest seed and cloud-image code move from driver/qemu to driver/linuxguest; relay VM init gains virtio_mmio"}
  - {host: windows-hyperv, blocking: false, reason: "regression: relay VM init and x86_64 relay initramfs build shared with the Firecracker relay VM"}
  - {host: macos-vz, blocking: false, reason: "regression only: cmd/warden driver wiring, and driver/ root if implementation touches it"}
tier2_gaps:
  - {host: qemu-any, gap: "Linux guest build after the driver/linuxguest extraction"}
  - {host: windows-hyperv, gap: "relay VM boot after the shared init change"}
  - {host: macos-vz, gap: "VZ build after the cmd/warden wiring change"}
  - {host: linux-kvm, gap: "arm64 Firecracker hosts: no arm64 Linux KVM machine; x86_64 only in this feature (question a9)"}
rules: [ORCH-01, ORCH-02, ORCH-03, ORCH-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-02, RELAY-03, RELAY-04, RELAY-07, BOOT-01, BOOT-02, BOOT-03, BOOT-04, BOOT-05, BOOT-06, BOOT-07, EXEC-01, UX-01, UX-04, UX-07, TEST-02, TEST-03, TEST-08, SEC-01, FLOW-02, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-15, FLOW-19]
concepts: [firecracker, microvm, kvm, two-vm-topology, tap-networking, host-bridge, network-slots, host-setup, direct-kernel-boot, cloud-init, raw-rootfs, image-cache, jailer, warm-pool, snapshot-resume, vsock, mmds]
related: [firecracker-spike-plan.md, firecracker-driver-plan.md, unified-bootstrap.md, relay-leaf-crl.md, ledger-environment-types.md, hyperv-windows-prepare.md, hyperv-driver-plan.md, vm-driver-release-plan.md, platform-compatibility.md]
---

# Firecracker driver

## Problem

BuildWarden has no driver for Linux CI fleets that run many builds at once on
one KVM host. QEMU works there but is heavy per VM (a full device model, about
1 to 2 s boot, tens of MB overhead) and carries a large attack surface; the
container driver shares the host kernel. Firecracker microVMs boot in about
125 ms with a few MB of overhead and a minimal virtio-mmio device model, which
is what fleet operators running hundreds of concurrent builds want.

Two plans exist and disagree with each other and with the code that has landed
since they were written:

- [`firecracker-driver-plan.md`](firecracker-driver-plan.md) (older) moves
  signaling to vsock, injects metadata over MMDS, reads the ledger from the
  relay's rootfs, and snapshots a warm relay pool after its CA exists, so every
  restored relay would share one CA key (RELAY-03).
- [`firecracker-spike-plan.md`](firecracker-spike-plan.md) (Jeff's latest)
  keeps the relay unchanged in `vm` mode and the topology of the qemu driver,
  but still mounts rootfs images to read signals and the ledger, injects the CA
  by writing into the build rootfs, and passes build material over MMDS.

Since then the relay became diskless and streams every output to the host
collector over a tunnel the host opens at claim time (RELAY-07), and the
bootstrap model fixed what may cross into a build environment (BOOT-01 to
BOOT-06, [`unified-bootstrap`](unified-bootstrap.md)). This design keeps the
spike plan's intent (a new `driver/firecracker`, the relay unchanged, the
qemu topology, the phased spike-first rollout) and brings it into line with the
current model. It supersedes both plans, and both share the feature slug until
this design resolves it (FLOW-12).

## Prior art

- **Bootstrap model** ([`bootstrap.md`](../development/bootstrap.md),
  [`unified-bootstrap`](unified-bootstrap.md)): isolate, deliver `warden-io`,
  start, then only watch. Firecracker follows it exactly; the qemu Linux-guest
  path (cloud-init `CIDATA` seed per build, `runcmd` launcher) is reused rather
  than the plans' MMDS and rootfs writes.
- **Session and collector** (`driver/session.go`, `collector/`, the
  [overview](../book/src/concepts/overview.md)): `StartSession`, `ClaimRelay`,
  `Wait`. Readiness is the relay's `POST /v1/ready`; heartbeat and exit arrive
  over the tunnel. This retires both plans' signaling designs (files on a
  mounted rootfs, vsock listeners).
- **qemu driver** (`driver/qemu`): the relay VM boots the shared Alpine
  initramfs (`tools/relay-vm`) with the current relay binary appended as a cpio
  (`relayInitrd`), takes a per-instance claim token on the kernel command line,
  and is claimed on port 8301. Firecracker reuses all of it; only the network
  plumbing and the VMM differ.
- **Hyper-V driver** ([`hyperv-driver-plan`](hyperv-driver-plan.md),
  `driver/hyperv`): a one-time `warden setup` creates the durable network
  (private switch, NAT) so builds run without elevation, and the host claims
  the relay at its static NAT address. The relay init already accepts
  `warden.net=static` and `warden.nat_*` for this. Firecracker copies the shape:
  `warden setup --driver firecracker` creates durable network slots, and builds
  run unprivileged.
- **Relay leaf CRL** ([`relay-leaf-crl`](relay-leaf-crl.md)): the CA and leaf
  handling is per relay instance; nothing here changes it. It is why the relay
  must never be cloned after key generation (Decision 9).
- **Environment records** ([`ledger-environment-types`](ledger-environment-types.md)):
  `vm-disk-chain/v1` already takes a free-form `driver` string, `raw` disks and
  `kernel` / `initrd` layers, and `prepared` sources with hashed inputs, so the
  Firecracker build environment is described with no schema change.
- **Guest image preparation** ([`hyperv-windows-prepare`](hyperv-windows-prepare.md)):
  the image cache, layer-hash cache and `prepared` provenance pattern is reused
  for the raw images converted from cloud images.
- **Release plan** ([`vm-driver-release-plan`](vm-driver-release-plan.md),
  Phase 3) and [`platform-compatibility`](platform-compatibility.md): Firecracker
  is the Linux-fleet driver, KVM-only, same-architecture only. Its "reuse the
  relay VM initramfs" note holds; its "MMDS for metadata" and "vsock for
  signaling" notes are superseded here.
- **Shared Windows guest code** (`driver/winguest`): precedent for pulling a
  guest family's seed code out of one driver into a shared package; Linux-guest
  code moves to `driver/linuxguest` the same way (Decision 7).

Found in passing, outside this design: the qemu direct-boot build initramfs
(`tools/build-vm/build-initramfs.sh`) bakes `warden-io` in at asset-build time,
which BOOT-02 and BOOT-06 do not allow and the deviation register does not
list. Firecracker's direct-boot path appends `warden-io` per build instead
(Decision 6); the qemu path is left for its own fix.

## Decisions

1. **Relay unchanged, `vm` mode, same initramfs** (spike 4.1; RELAY-04). The
   relay VM is the shared Alpine initramfs with the relay appended at
   `/sbin/relay`, booted by direct kernel boot, diskless, claimed on 8301. The
   relay binary and `relay/` are not modified. The init script gains only
   `virtio_mmio` in its module list (a no-op where absent). *Rejected:* a
   Firecracker-specific relay init or rootfs image (the spike's
   `build-rootfs.sh`), which would fork the trust boundary.

2. **Signaling is the collector; no vsock, no rootfs mounts** (spike 4.2, 7.3;
   old Q3). Readiness is the claim's `/v1/ready`; heartbeat, output and exit
   come over the tunnel; the ledger streams to the collector as it is written
   (RELAY-07). The host never mounts a guest or relay image. The VMM process
   exiting is the "environment stopped" signal (Decision 8). *Rejected:*
   file signals read from a mounted rootfs (breaks RELAY-07 and BOOT-03, and
   mounting an untrusted filesystem on the host is a host-env risk); vsock
   signaling (a second guest-to-host channel outside the relay, and a CID
   allocator to maintain, for nothing the collector lacks). So no vsock device
   is configured and no CID allocator is built.

3. **No MMDS** (spike 4.6, 7.7; old Q6). Neither VM gets an MMDS device. The
   build script, CA and context come from the relay (BOOT-04); network config is
   static in the seed (BOOT-03 allows it). *Rejected:* MMDS v1 or v2 for build
   material: an out-of-band input path not recorded by the relay (BOOT-02,
   BOOT-04), and the token question disappears with it.

4. **Network: durable per-host slots created once by `warden setup`** (spike
   4.7, 7.8; old Q1; question a3). `warden setup --driver firecracker` (root,
   once, idempotent) creates N slots. Slot *i* is:
   - `wfc<i>b`, a bridge with no address (IPv4 or IPv6), joining two TAPs,
     `wfc<i>r` (relay build NIC) and `wfc<i>g` (build VM NIC), both owned by the
     warden user, with IPv6 disabled on all three;
   - `wfc<i>u`, a TAP owned by the warden user for the relay's upstream, with
     host address `100.64.88.(4i+1)/30`; the relay gets `.(4i+2)`;
   - host rules in a `WARDEN-FC` chain: MASQUERADE from `100.64.88.0/22` to the
     default route; FORWARD accept `wfc+u` to the uplink and established
     replies; FORWARD accept bridged frames only within the same `wfc<i>b`
     (needed when `br_netfilter` is loaded, as Docker hosts do); drop any
     forward between slots; INPUT from `wfc+u` accept established only, so a
     relay cannot open connections to the host.
   Every slot reuses the relay init's fixed build link `10.0.0.1/30` to
   `10.0.0.2/30`, because each bridge is its own L2 segment with no host route.
   A build takes a free slot under `driver.TryHostLock("firecracker-slot-<i>")`
   and holds it until teardown; the lock dies with the process. Builds then need
   only `/dev/kvm` access (the `kvm` group) and the TAPs' ownership; warden
   never runs as root for a build. The upstream range is
   `100.64.88.0/22` (256 slots), next to the container driver's
   `100.64.87.0/24`. *Rejected:* per-build TAP and iptables creation (needs
   `CAP_NET_ADMIN` on every build, so warden as root); slirp4netns for the
   upstream (extra process and latency, and still a TAP for the build link);
   operator-provided bridges (no isolation guarantees warden can check).

5. **Claim over the upstream TAP** (no relay change). The host claims at
   `100.64.88.(4i+2):8301` with a per-build token on the kernel command line,
   exactly as Hyper-V claims at the relay's static NAT address. Kernel command
   line: `console=ttyS0 reboot=k panic=1 pci=off warden.net=static
   warden.nat_ip=… warden.nat_prefix=30 warden.nat_gw=… warden.dns=…
   warden.claim_token=…`. Upstream DNS is the host's non-loopback resolvers
   (from `/run/systemd/resolve/resolv.conf` when systemd-resolved fronts
   `127.0.0.53`); with none, the build fails early with a fixable error
   (UX-04).

6. **Executor delivery: the qemu Linux-guest paths, made Firecracker-shaped**
   (spike 4.10; BOOT-02, BOOT-03, BOOT-06).
   - *Disk path* (a cloud image): the per-build cloud-init NoCloud `CIDATA`
     ISO, the same seed qemu builds, attached as a second, read-only virtio-blk
     drive (Firecracker has no CD-ROM; cloud-init finds the seed by label). The
     launcher is cloud-init `runcmd`, ending in `reboot -f` instead of
     `poweroff -f`, because Firecracker exits on a guest reset, not a poweroff.
   - *Direct-boot path* (no resolvable image): the build-vm initramfs with
     `warden-io` appended per build as a cpio at `/bin/warden-io`, which
     overrides any copy baked in. Its init execs `warden-io initialize`.
   Trust, the build script and context come from the relay. *Rejected:* writing
   the CA or script into the build rootfs (spike 4.10; BOOT-02, BOOT-05), MMDS
   (Decision 3).

7. **Build rootfs: cloud image converted once to raw, copied per build** (spike
   4.4, 4.5, 7.6; old Q5; question a6). Firecracker needs raw block devices.
   The cloud image (the qemu catalog, via `driver/linuxguest`) is converted
   with `qemu-img convert -O raw` once, into the image cache, and recorded in
   the layer-hash cache as a `prepared` layer whose input is the downloaded
   image. Each build copies it into its work dir with `cp --reflink=auto`
   (instant on btrfs and reflink XFS, a full copy elsewhere) and boots it
   read-write. The cloud image's own kernel is not used: Firecracker boots the
   pinned guest kernel (Decision 10) with `root=PARTUUID=…` of the image's root
   partition. The qemu Linux-guest seed, ISO writer, cpio append and
   cloud-image resolution move to a new `driver/linuxguest` package that both
   drivers import, behavior-preserving for qemu (ORCH-01). *Rejected:*
   converting per build (slow, and a fresh hash per build); device-mapper thin
   snapshots now (root per build or a privileged helper; a follow-up for
   fleets); overlayfs inside the guest (setup code in the image, BOOT-06).
   Default catalog unchanged from qemu (spike 7.2); custom images use
   `--image <path>` as qemu does.

8. **Process model: `firecracker --no-api --config-file`** (old "API socket"
   risk). Each VM is one `firecracker` process started from a JSON config in
   the per-build work dir (0700), with no API socket at all, Firecracker's
   default seccomp filters on, serial console to the driver (shown with
   `--verbose`, as qemu does). The driver watches both processes: if the build
   VM's process exits before the collector reports completion, the build fails
   at once with the console tail, instead of waiting out the heartbeat timeout.
   Resource defaults match qemu (spike 7.4): relay 2 vCPUs and 512 MiB
   (the spike may show the initramfs relay needs less; lowering it is an
   implementation detail, recorded in Rollback notes); build `--cpus` /
   `--memory`, default 4 and 4096 MiB.

9. **No warm pool, no snapshots in this feature** (spike Phase 4; old Q7;
   RELAY-03). A snapshot of a relay after its keys exist clones the CA and
   ledger-signing keys into every restored VM, which RELAY-03 forbids. Even a
   snapshot of an *unclaimed* relay (keys are generated at claim, in
   `relay.Start`) clones the guest kernel's CSPRNG state, so two restored relays
   could derive the same keys unless the VMM signals a VM generation change and
   the guest reseeds before claim. The warm pool is retired from this feature;
   a later design may take it up under three conditions recorded here: snapshot
   only before the claim; require VMGenID (or an equivalent forced reseed
   from fresh host entropy) on restore; and a test that two restored relays
   produce different CA and signing keys. Old Q7 (snapshot lifetime) moves to
   that design.

10. **Guest kernel: one pinned kernel, built from Firecracker's guest config**
    (spike 4.9, 7.1; old Q4; question a5). `tools/firecracker-vm/build-kernel.sh`
    builds a pinned 6.1 LTS kernel from Firecracker's published microVM guest
    config, plus the options the relay and build guests need built in
    (virtio-mmio with command-line devices, virtio-blk, virtio-net, ext4,
    iso9660, netfilter and NAT for the relay). Output is an uncompressed
    `vmlinux` (x86_64), used by both VMs. The release ships it next to the
    relay VM assets; the driver finds it beside the warden binary, then in
    `tools/firecracker-vm/output`, like `resolveRelayAssets`. Not configurable.
    The relay initramfs is the existing x86_64 build (shared with Hyper-V).

11. **No jailer in this feature** (spike 4.3, 7.5; old Q2; question a4). The
    jailer needs root to chroot and drop to a per-VM UID, which would undo
    Decision 4's unprivileged builds. Firecracker runs as the invoking user with
    its seccomp filters; the per-build work dir and the slot TAPs are the only
    host resources it can open. Jailer support (UID range, cgroups, network
    namespaces per slot) is a follow-up design for multi-tenant fleets.

12. **CLI and config are additive** (spike 9.2; ORCH-03, UX-01, UX-07). New:
    the `firecracker` value for `--driver` and `runtime.driver`;
    `warden setup --driver firecracker` with `--slots N` (default 16); `warden
    doctor --driver firecracker`. The Firecracker binary is taken from `PATH`
    and checked against a pinned minimum version; no binary-path flag or key.
    `warden shell` returns `ErrExecNotSupported`, as qemu does. `warden clean`
    reaps stale Firecracker work dirs.

13. **Linux-only behind build tags** (GLOBAL-07). `driver/firecracker` has
    `*_linux.go` files and a stub for other hosts whose `StartBuild` returns a
    fixable "needs a Linux host with KVM" error, so `cmd/warden` builds for
    every target. Same-architecture only; x86_64 in this feature (question a9).

How the plans' items end up:

| Plan item | Outcome |
|---|---|
| Old Q1 relay networking; spike 4.7, 7.8 subnet | Folded into Decision 4 (setup slots, `100.64.88.0/22`); question a3 |
| Old Q2 jailer ownership; spike 4.3, 7.5 | Folded into Decision 11 (no jailer now); question a4 |
| Old Q3 vsock CID allocation; spike 4.8 | Retired: no vsock (Decision 2) |
| Old Q4 kernel and rootfs provisioning; spike 4.9, 7.1 | Folded into Decisions 1 and 10; question a5 |
| Old Q5 rootfs mutability; spike 4.4, 4.5, 7.6 | Folded into Decision 7; question a6 |
| Old Q6 MMDS version; spike 4.6, 7.7 | Retired: no MMDS (Decision 3) |
| Old Q7 snapshot lifecycle; spike Phase 4 | Retired from this feature with its RELAY-03 conditions (Decision 9) |
| Spike 4.2, 7.3 signaling phases | Retired: the collector (Decision 2) |
| Spike 7.2 cloud image catalog | Folded into Decision 7 (qemu catalog unchanged) |
| Spike 7.4 resource defaults | Folded into Decision 8 |
| Spike 4.10 extension adaptation | Retired: extensions became the executor baseline (BOOT-05) |

## Questions

### Blocking

None. Every open point has a recommended default the design proceeds on.

### Assumed

Each is "proceeding with the recommendation; confirm" in the design review
(FLOW-19). They are also in the queue file.

- **a1 — Supersede both plans.** Proceeding with: archive
  `firecracker-driver-plan.md` and `firecracker-spike-plan.md` as superseded by
  this design, front-matter only (bodies untouched), each with `feature` set to
  its file stem so only this design carries `firecracker-driver`, and drop the
  `design_tolerate` entry in `tools/tier1/rules.yaml` that covered the shared
  slug. Options: archive both (recommended); keep the spike plan `draft` as an
  appendix of this design; delete both (loses the archive record FLOW-12 keeps).
- **a2 — Treat the new mapping row as a bootstrap-model change.** Proceeding
  with: declare the `bootstrap-model` seam, fill the Bootstrap comparison, and
  record `alters_relationships: true`, though no invariant changes. Options:
  as stated (recommended, conservative under BOOT-07 and FLOW-02); treat the row
  as "correcting the mapping to match the code", with no seam and
  `alters_relationships: false` (the design review would then be judged without
  the comparison).
- **a3 — Network slots made once by `warden setup`.** Proceeding with Decision 4:
  a root-once `warden setup --driver firecracker [--slots N]` (default 16) makes
  durable bridges, TAPs and `WARDEN-FC` rules; builds run unprivileged.
  Options: setup slots (recommended); per-build TAP and iptables creation with
  warden run as root or with `CAP_NET_ADMIN`; slirp4netns for the upstream.
- **a4 — No jailer in this feature.** Proceeding with Decision 11. Options: no
  jailer, follow-up design (recommended); optional `--jailer` (requires root
  builds); jailer by default.
- **a5 — One pinned 6.1 LTS guest kernel, built by script, shipped as a release
  asset.** Proceeding with Decision 10. Options: 6.1 from Firecracker's config
  (recommended); 5.10 or 6.6; download Firecracker's CI-published kernels by
  pinned hash instead of building; make the kernel configurable.
- **a6 — Raw image converted once into the cache, `cp --reflink=auto` per
  build.** Proceeding with Decision 7. Options: as stated (recommended);
  convert per build; device-mapper thin snapshots now.
- **a7 — Relay upstream reaches what qemu's relay reaches, minus the host and
  other slots.** Proceeding with: the `WARDEN-FC` rules block the relay from
  opening connections to the host and to other slots (both required by the
  Security note), but do not block link-local or private ranges (for example a
  cloud host's `169.254.169.254`), keeping parity with the qemu and container
  drivers, whose relays can reach them today. Options: parity (recommended),
  with a security-suite follow-up across drivers; block `169.254.0.0/16` for
  Firecracker only (diverges from ORCH-01 parity); block link-local on every
  driver (a separate cross-driver design).
- **a8 — Native host `linux-kvm` served by `windows-desktop` under WSL2.**
  Proceeding with: add `linux-kvm` to `windows-desktop`'s hosts in
  `hosts.yaml` (WSL2 with nested virtualization exposes `/dev/kvm`; the
  machine is always on), and promote to a CI job on a KVM-capable hosted Linux
  runner when one is confirmed. Options: windows-desktop under WSL2
  (recommended); a GitHub-hosted runner with `/dev/kvm` as a CI tier-2 job
  (then not native validation, FLOW-09); a new dedicated Linux machine. Until
  `hosts.yaml` lists `linux-kvm`, `wf validate` reports the
  `f6-validate-linux-kvm` step as served by no machine.
- **a9 — x86_64 hosts only in this feature.** Proceeding with x86_64 (the
  common CI fleet host); arm64 Firecracker (a PE `Image` kernel, the existing
  aarch64 relay initramfs) is a tracked gap until an arm64 Linux KVM host
  exists. Options: x86_64 only (recommended); both now (no validation host for
  arm64).

## Design

### Topology

```
HOST (warden, unprivileged; slot i taken under a host lock)
  collector <==== tunnel (host dials 100.64.88.(4i+2):8301) ====+
                                                                |
  wfc<i>u  100.64.88.(4i+1)/30  --MASQUERADE--> uplink          |
     |                                                          |
  +--+------------------------- Relay VM (firecracker) ---------+--+
  |  eth1  100.64.88.(4i+2)/30   (upstream, static, from cmdline)  |
  |  eth0  10.0.0.1/30           (build link)                      |
  |  Alpine initramfs + /sbin/relay (vm mode), diskless             |
  +--+--------------------------------------------------------------+
     |
  wfc<i>r ==== wfc<i>b (bridge, no address, IPv6 off) ==== wfc<i>g
                                                              |
  +-----------------------------------------------------------+--+
  |  Build VM (firecracker)   eth0 10.0.0.2/30 -> 10.0.0.1 only   |
  |  disk path:   rootfs.raw (rw copy) + CIDATA seed (ro)         |
  |  direct boot: build initramfs + warden-io cpio (per build)    |
  +---------------------------------------------------------------+
```

The build VM's only NIC is on a bridge whose only other port is the relay's
build NIC. The bridge, its TAPs and the host have no address on that segment,
so nothing from the build VM can reach the host or the uplink except through
the relay's proxy, DNS and control plane.

### StartBuild

```
StartBuild(req):                                   # driver/firecracker, linux only
    preflight: /dev/kvm rw, firecracker >= pinned, kernel + relay assets,
               qemu-img (disk path), at least one setup slot     # UX-04
    script := StageBuildScript; RecordInputs                      # ORCH-02
    resolve build boot: disk path (cached raw, else convert) or direct boot
    env := describe as vm-disk-chain/v1, driver "firecracker"     # no schema change
    session := StartSession(...)
    slot := first free slot under TryHostLock("firecracker-slot-<i>")
    relay initrd := relay-vm initramfs + /sbin/relay cpio         # linuxguest
    start relay VM: firecracker --no-api --config-file relay.json
        drives: none; NICs: eth0=wfc<i>r, eth1=wfc<i>u; boot args per Decision 5
    session.ClaimRelay(100.64.88.(4i+2):8301, token)              # tunnel + /v1/ready
    deliver: disk path   -> cp --reflink=auto raw -> work/rootfs.raw;
                            CIDATA ISO (linuxguest) -> work/seed.iso
             direct boot -> build initramfs + warden-io cpio -> work/build-initrd
    start build VM: firecracker --no-api --config-file build.json
        NIC: eth0=wfc<i>g only; drives: rootfs (rw) + seed (ro), or none
    wait: session.Wait, or build VM process exits first -> fail with console tail
    teardown: stop both VMs, remove work dir, release slot lock   # ORCH-04
    FinalizeOutput
```

The output directory is identical to the other drivers' (RELAY-07); nothing is
read back from any VM image.

### Host setup and doctor

```
warden setup --driver firecracker [--slots N]:   # root, idempotent
    check: Linux, /dev/kvm, the invoking (sudo) user's kvm group membership
    for i in 0..N-1: create or verify wfc<i>b, wfc<i>r, wfc<i>g, wfc<i>u
                     (TAPs owned by the user; IPv6 off; no address but wfc<i>u's)
    net.ipv4.ip_forward=1; create or verify the WARDEN-FC chain and its jumps
    print what was created and what was verified

warden doctor --driver firecracker:              # read-only
    each preflight item above, plus: slots present and owned by this user,
    WARDEN-FC rules present, a reachable non-loopback upstream resolver
```

Setup state does not survive a reboot (TAPs and iptables rules are runtime
state); `warden doctor` says to re-run setup, and the guide shows a systemd
unit that runs it at boot. Removing the slots is documented as the matching
`ip link del` and chain flush; it is not a warden command (ORCH-04 treats the
network as durable).

### Packages

- `driver/firecracker`: `driver_linux.go` (StartBuild, teardown), `vm_linux.go`
  (config JSON, process start and watch), `slots_linux.go` (slot naming,
  locking, setup and doctor), `images_linux.go` (raw conversion and cache),
  `driver_other.go` (stub), tests.
- `driver/linuxguest` (moved from `driver/qemu`, behavior-preserving): the
  NoCloud seed, the ISO writer, the cpio append, cloud-image resolution and
  download. The cloud-init `runcmd`'s last line becomes a parameter
  (`poweroff -f` for qemu, `reboot -f` for Firecracker).
- `tools/firecracker-vm/build-kernel.sh`: the pinned kernel. `tools/relay-vm/init`:
  `virtio_mmio` added to the module list.
- `cmd/warden`: driver dispatch, config validation, `setup`, `doctor`, `clean`.

### Overview change (FLOW-15)

`overview.md` gains `firecracker` in the driver list and a Firecracker line in
each step of "VM drivers": isolate (a TAP on a per-slot host bridge whose only
other port is the relay), deliver (the qemu Linux seed as a read-only drive,
or `warden-io` appended to the build initramfs), start (cloud-init `runcmd`, or
the initramfs init).

## Cross-driver impact

- **firecracker** (new): everything above.
- **qemu**: its Linux-guest seed, ISO writer, cpio append and cloud-image code
  move to `driver/linuxguest` with no behavior change (the `runcmd` tail stays
  `poweroff -f`). Its relay VM boots the same init, which now also tries
  `virtio_mmio` (absent from its initramfs, so skipped). Covered by tier-0 unit
  tests that move with the code and the tracked `qemu-any` gap.
- **hyperv**: shares `tools/relay-vm/init` and the x86_64 relay initramfs build;
  the one-line module-list change is skipped on Hyper-V. Tracked
  `windows-hyperv` gap.
- **container**: no code change. Declared because `cmd/warden` changes (tier-1
  integration CI covers it) and because implementation may touch the `driver/`
  root, which every driver imports (TEST-02 caveat). If it does not, the
  declaration is still harmless.
- **vz**: no code change; declared for the same `cmd/warden` and `driver/` root
  reasons; tracked `macos-vz` gap.
- `relay/`, `cmd/relay/`, `collector/`, `ledger/`, `relaycfg/`, `cmd/warden-io/`,
  the `driver.Driver` interface and `BuildRequest` / `BuildResult` do not
  change (spike section 9).

## Bootstrap comparison

### Current model

The diagram in [`bootstrap.md`](../development/bootstrap.md), unchanged:
isolate, then deliver the executor per build, then start it; everything else
over the relay. Per-driver mapping rows: qemu Linux, qemu and hyperv Windows,
vz macOS, container. Deviation register: D2 (vz).

### Proposed model

The same diagram. The per-driver mapping gains one row; the deviation register
gains nothing.

| Driver / guest | (1) Isolation | (2) Executor delivery | (3) Start trigger |
|---|---|---|---|
| firecracker / Linux | Topology: the build VM's only NIC is a TAP on a per-slot host bridge whose only other port is the relay VM; the bridge and host have no address on it | cloud-init `CIDATA` ISO per build as a read-only virtio-blk drive (disk path), or `warden-io` appended per build to the build initramfs (direct boot) | cloud-init `runcmd` (disk path); the initramfs init (direct boot) |

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | Topological for VM drivers; holder plus sidecar for container | Unchanged; Firecracker is topological: the bridge exists (setup) and the relay is claimed before the build VM process starts | firecracker: by construction; others: none |
| BOOT-02 executor is the only out-of-band input | Seed disk, `CIDATA` ISO, tar copy; vz baked (D2) | Unchanged; Firecracker delivers only `warden-io` and the cloud-init launcher, per build | firecracker: no MMDS, no rootfs writes; others: none |
| BOOT-03 one start, then hands off | Boot mechanism or entrypoint starts `initialize`; host only watches | Unchanged; the host only watches the collector and the VMM processes | firecracker: process exit is lifecycle state, not a guest step; others: none |
| BOOT-04 inputs come from the relay | CA, script, context fetched by the executor | Unchanged | none |
| BOOT-05 setup is the executor's domain | Baseline plus `/bootstrap` policy | Unchanged; static `network-config` in the seed only | none |
| BOOT-06 images carry hooks, not content | Generic hooks only | Unchanged; the converted raw image is the cloud image's content plus nothing; the direct-boot `warden-io` is per build | firecracker: none; the qemu direct-boot gap noted in Prior art is not changed here |

### Gains

A fifth driver that is in the model from its first commit, with no deviation
entry, and with no new input path into the build environment: the record of
what the build consumed is exactly as complete as on qemu.

### Costs

None to the invariants. The security cost is on the host side (bridges, TAPs,
NAT rules made by a root setup step), covered in the Security note, not in
what crosses into the build environment. The record gains nothing and loses
nothing.

## Security note

Boundaries: **host-env** (the host OS and the build environment) and
**env-relay** (the relay VM and the build VM). The relay binary does not change,
so GLOBAL-06's "any change to the relay" applies only through the shared relay
VM init (one module name added).

- **New host network state, made by root once.** `warden setup` runs as root and
  leaves bridges, TAPs, `ip_forward=1` and a `WARDEN-FC` chain. Builds afterwards
  need no privilege beyond `/dev/kvm`, which is less than the plans' per-build
  `CAP_NET_ADMIN`. Setup only creates or verifies named `wfc*` devices and its
  own chain; it never flushes or reorders other rules. `ip_forward=1` is a
  host-wide change; the chain's default for `wfc*` traffic is drop, and
  forwarding is allowed only from `wfc+u` to the uplink.
- **The build VM cannot reach the host.** The bridge, both of its TAPs and the
  host have no IPv4 address on the build segment, and IPv6 is disabled on all
  three so no link-local address appears (an IPv6 link-local on the bridge
  would otherwise be a direct path from the build VM to host services). There is
  no route between a bridge and anything else. With `br_netfilter` loaded,
  bridged frames are accepted only within the same bridge.
- **The relay cannot open connections to the host or to other slots.** INPUT
  from `wfc+u` is established-only (the host dials the claim and tunnel), and
  forwarding between slots is dropped, so one build's relay cannot reach
  another relay's proxy ports and write into another build's ledger. What the
  relay can reach upstream is otherwise what qemu's relay reaches (question a7).
- **Slot exclusivity.** A slot is held by a host file lock for the whole build;
  a crashed build releases it, and the next build starts fresh VMs on it. The
  TAPs are owned by the warden user, so other unprivileged users cannot attach
  a VMM to them.
- **The claim port** is reachable from the host's network namespace at the
  relay's upstream address, as on Hyper-V (and on qemu's loopback forward); the
  per-build claim token guards it, and the relay stops listening after one
  claim.
- **VMM surface.** No API socket (`--no-api`), no vsock, no MMDS, no shared
  filesystem; two virtio-blk drives at most and one NIC for the build VM.
  Firecracker's default seccomp filters stay on. Without the jailer, a VMM
  escape lands as the invoking user, not root (question a4).
- **No untrusted filesystem is mounted on the host.** The plans read signals and
  ledgers by mounting VM images; this design never mounts a guest or relay image
  on the host, so a malicious build cannot attack the host's filesystem drivers.
- **Keys stay per relay (RELAY-03).** No snapshot or warm pool (Decision 9); a
  future pool must meet the three conditions there.
- **Image conversion** runs `qemu-img convert` on a downloaded cloud image whose
  hash is verified first, as qemu does today; the raw output is a cache entry
  with recorded provenance.
- **TEST-08 should exercise** (SEC-01): from the build VM, reach the host over
  IPv4 and IPv6 link-local on the bridge, a raw socket or a spoofed source on
  the bridge, and the uplink directly (all must fail); from the build VM through
  the relay, reach the host's own addresses and another slot's relay (must
  fail); attach a second VMM to a held slot's TAP as another user (must fail);
  two concurrent builds on two slots produce distinct CA fingerprints and
  signing keys and ledgers with no cross-entries; a build VM that resets or
  dies early fails the build promptly with no partial ledger accepted as
  complete.

## Validation plan

- **Tier 0** (every host, CI): Firecracker config JSON generation (golden
  files), kernel command-line construction, slot naming and address
  arithmetic, the setup command list (a fake runner, no root), raw-cache key
  and provenance, preflight error messages, the non-Linux stub; the moved
  `driver/linuxguest` tests, plus a test that qemu's seed is byte-identical
  before and after the move (TEST-04 golden fixtures).
- **Tier 1** (CI integration): unchanged container suite, run because
  `cmd/warden` changes.
- **Tier 2, blocking: `linux-kvm`** (FLOW-09; the change is that driver's tree):
  `warden setup --driver firecracker`; `warden doctor`; the direct-boot spike
  build; an Ubuntu 24.04 cloud-image build with a COPY and an HTTPS fetch;
  exclusive trust; egress to an un-proxied address blocked; two concurrent
  builds on two slots; a build VM killed mid-build fails promptly; teardown
  leaves no work dir and frees the slot; ledger verifies. Runs on
  `windows-desktop` under WSL2 if a8 is confirmed.
- **Tier 2, tracked:** `qemu-any` (a Linux guest build after the extraction),
  `windows-hyperv` (relay VM boot after the init change), `macos-vz` (a build
  after the `cmd/warden` change); arm64 Firecracker (no host). On the
  native-gap backlog and in `tier2_gaps` (TEST-03).
- **TEST-08**: the release that ships this trips GLOBAL-06; the security suite
  runs the cases in the Security note.

## Implementation plan

1. Phase 0 spike, before anything else lands: on a Linux KVM host, boot the
   relay VM from the shared x86_64 initramfs with the pinned kernel under
   Firecracker, claim it over a hand-made slot, and run a direct-boot build of
   `FROM alpine` / `RUN echo hello` to a verified ledger. Confirm the relay
   memory default and that Alpine's modules plus `virtio_mmio` suffice; record
   any finding in Rollback notes and return to Design if a decision fails
   (FLOW-06, RELAY-04, BOOT-01).
2. `tools/firecracker-vm/build-kernel.sh` and its pinned config; release
   packaging next to the relay VM assets; asset lookup beside the binary then in
   the tools output (GLOBAL-07, UX-04).
3. `tools/relay-vm/init`: add `virtio_mmio` to the module list; nothing else
   changes (RELAY-04, GLOBAL-06).
4. Extract `driver/linuxguest` from `driver/qemu` (seed, ISO writer, cpio append,
   cloud-image resolution and download; `runcmd` tail as a parameter), with the
   byte-identical seed test; qemu behavior unchanged (ORCH-01, TEST-04).
5. `driver/firecracker` slots: naming, address arithmetic, `TryHostLock`
   acquisition, and the setup and doctor command lists behind a runner
   interface, with tier-0 tests (ORCH-04, GLOBAL-06, UX-04).
6. `driver/firecracker` images: raw conversion into the cache, layer-hash
   record as `prepared` with the cloud image as input, per-build reflink copy
   (ORCH-04, BOOT-06).
7. `driver/firecracker` VMs: config JSON, `--no-api` process start, console
   handling, process watch, teardown on every error path (ORCH-04, BOOT-03).
8. `driver/firecracker` StartBuild: session, slot, relay boot and claim, both
   delivery paths, build VM boot, wait with the early-exit failure,
   `vm-disk-chain/v1` environment with driver `firecracker` (BOOT-02, BOOT-04,
   RELAY-07, ORCH-02, GLOBAL-08).
9. Non-Linux stub and build tags; `make lint` for linux, windows and darwin
   (GLOBAL-07).
10. `cmd/warden`: `firecracker` in driver validation and dispatch, `setup
    --driver firecracker --slots`, `doctor`, `clean`; `Exec` returns
    `ErrExecNotSupported` (ORCH-01, ORCH-03, UX-07).
11. Reference docs in the same change: `guide/drivers.md`, `guide/cli.md`,
    `guide/configuration.md`, the architecture page; `bootstrap.md`'s
    per-driver mapping row (proposed in the Bootstrap comparison, landed with
    the design review's acceptance); `overview.md` VM drivers and driver list
    (UX-01, FLOW-15, BOOT-07).
12. Tier-0 tests listed in the Validation plan; seeded fixtures, no KVM needed
    (TEST-02, TEST-04).
13. Native validation on `linux-kvm` (blocking) and the tracked gaps on the
    backlog (FLOW-09, TEST-03).
14. Security-suite cases from the Security note added to the corpus for the
    release (TEST-08, SEC-01).

## Design review

<!-- No review yet. design_review: required (FLOW-19): alters_relationships,
the bootstrap-model seam, security_boundary host-env, blocking linux-kvm
validation and nine assumed questions. The review must explicitly accept the
Bootstrap comparison (BOOT-07). -->

## Consolidation

<!-- Filled in at Publish (FLOW-10, FLOW-15). -->

## Rollback notes

<!-- None yet. The Phase 0 spike records its findings here. -->

## Archive summary

<!-- Filled in at Publish (FLOW-12). -->

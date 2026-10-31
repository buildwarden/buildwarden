---
feature: firecracker-driver
status: active
drivers: [container, qemu, vz, hyperv, firecracker]  # firecracker is new; qemu loses its Linux-guest seed, image catalog and relay-initrd helpers to driver/linuxguest and driver/relayvm, and its relay initramfs build gains an arch parameter; container, vz and hyperv declared for the cmd/warden wiring and the new shared driver/ packages (TEST-02 caveat)
seams: [bootstrap-model]   # the per-driver mapping in bootstrap.md gains a firecracker row; every invariant unchanged (see Bootstrap comparison, question a2)
surfaces: [cli, config]    # --driver firecracker and runtime.driver = "firecracker"; warden setup, doctor, clean and image for the driver
security_boundary: host-env  # also env-relay (host-local access to the relay is new here); see "Security note"
alters_relationships: true   # conservative: a new driver and a new bootstrap-model mapping row (a2); no relay, executor, collector or Driver-contract change
design_review: required
native_validation:
  - {host: linux-kvm, blocking: true, reason: "driver/firecracker runs only on a Linux host with /dev/kvm; cloud sessions have none, and no CI job validates it yet (question a8). The Phase 0 spike also runs here (queue step f2-spike-linux-kvm)"}
  - {host: qemu-any, blocking: false, reason: "regression: Linux-guest seed, image catalog and relay-initrd helpers move to driver/linuxguest and driver/relayvm; tools/relay-vm/build-initramfs.sh gains an arch parameter (default unchanged)"}
  - {host: windows-hyperv, blocking: false, reason: "regression only: cmd/warden driver wiring and the shared driver/ packages; Hyper-V's relay boot disk and init are not touched"}
  - {host: macos-vz, blocking: false, reason: "regression only: cmd/warden driver wiring and the shared driver/ packages"}
tier2_gaps:
  - {host: qemu-any, gap: "Linux guest build after the driver/linuxguest and driver/relayvm extraction and the relay initramfs arch parameter"}
  - {host: windows-hyperv, gap: "Hyper-V build after the cmd/warden wiring change"}
  - {host: macos-vz, gap: "VZ build after the cmd/warden wiring change"}
  - {host: linux-kvm, gap: "arm64 Firecracker hosts: no arm64 Linux KVM machine; x86_64 only in this feature (question a9)"}
rules: [ORCH-01, ORCH-02, ORCH-03, ORCH-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-02, RELAY-03, RELAY-04, RELAY-07, BOOT-01, BOOT-02, BOOT-03, BOOT-04, BOOT-05, BOOT-06, BOOT-07, EXEC-01, UX-01, UX-04, UX-07, TEST-02, TEST-03, TEST-04, TEST-08, SEC-01, FLOW-02, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19]
concepts: [firecracker, microvm, kvm, two-vm-topology, tap-networking, host-bridge, network-slots, host-setup, direct-kernel-boot, cloud-init, raw-rootfs, gpt, image-cache, jailer, landlock, warm-pool, snapshot-resume, vsock, mmds, conntrack-zones, guest-kernel-config]
related: [firecracker-spike-plan.md, firecracker-driver-plan.md, unified-bootstrap.md, relay-leaf-crl.md, ledger-environment-types.md, hyperv-windows-prepare.md, hyperv-driver-plan.md, vm-driver-release-plan.md, platform-compatibility.md, design-front-matter-retrofit.md]
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
current model. It supersedes both plans; question a1 settles how the archive
records that (FLOW-12).

## Prior art

- **Bootstrap model** ([`bootstrap.md`](../development/bootstrap.md),
  [`unified-bootstrap`](unified-bootstrap.md)): isolate, deliver `warden-io`,
  start, then only watch. Firecracker follows it exactly; the qemu Linux-guest
  path (cloud-init `CIDATA` seed per build, `runcmd` launcher) is reused rather
  than the plans' MMDS and rootfs writes.
- **Session and collector** (`driver/session.go`, `collector/`, the
  [overview](../book/src/concepts/overview.md)): `StartSession`, `ClaimRelay`,
  `Wait`, and `Collector.Status()` for a completion that races tunnel teardown.
  Readiness is the relay's `POST /v1/ready`; heartbeat and exit arrive over the
  tunnel. This retires both plans' signaling designs (files on a mounted
  rootfs, vsock listeners).
- **qemu driver** (`driver/qemu`): the relay VM boots the Alpine relay
  initramfs (`tools/relay-vm/build-initramfs.sh`, **aarch64 only today**: it
  hard-codes `ALPINE_ARCH=aarch64`, and `hosts.yaml` records that the qemu
  driver runs only on aarch64 hosts) with the current relay binary appended as
  a cpio (`relayInitrd`, `appendFileCPIO`, assets found by
  `resolveRelayAssets`), takes a per-instance claim token on the kernel command
  line, and is claimed on port 8301. Its environment record is built by
  `describeBuild` into `<cache>/images/layers.json`. Firecracker reuses the
  init, the cpio composition and the claim; it needs **new x86_64 assets**
  (Decision 10) because none exist in the form a virtio-mmio relay can boot.
- **Hyper-V driver** ([`hyperv-driver-plan`](hyperv-driver-plan.md),
  `driver/hyperv`): a one-time `warden setup` creates the durable network
  (private switch, NAT) so builds run without elevation, and the host claims
  the relay at its static NAT address. The relay init already accepts
  `warden.net=static` and `warden.nat_*` for this. Firecracker copies the shape:
  `warden setup --driver firecracker` creates durable network slots, and builds
  run unprivileged. Hyper-V's x86_64 relay boot disk
  (`tools/relay-vm/hyperv/build.sh`) is **not** reusable: it is an offline
  build from a cache fetched by PowerShell, bakes `/sbin/relay` in, carries
  VMBus modules, and emits a UKI. It stays separate (Decision 10).
- **qemu direct-boot build initramfs** (`tools/build-vm`): arm64 only, labelled
  for test and integration use, bakes `warden-io` in, and its init writes
  `/etc/resolv.conf` and loads 9p. Firecracker does not reuse it (Decision 6).
- **Relay leaf CRL** ([`relay-leaf-crl`](relay-leaf-crl.md)): the CA and leaf
  handling is per relay instance; nothing here changes it. It is why the relay
  must never be cloned after key generation (Decision 9).
- **Environment records** ([`ledger-environment-types`](ledger-environment-types.md)):
  `vm-disk-chain/v1` already takes a free-form `driver` string, `raw` disks,
  `kernel` / `initrd` layers, `prepared` sources with hashed inputs, and seed
  files, so the Firecracker build environment is described with no schema
  change (Decision 14 gives the exact layers).
- **Guest image preparation** ([`hyperv-windows-prepare`](hyperv-windows-prepare.md)):
  the image cache, layer-hash cache and `prepared` provenance pattern is reused
  for the raw images converted from cloud images.
- **Release plan** ([`vm-driver-release-plan`](vm-driver-release-plan.md),
  Phase 3 and "pre-built kernel+initramfs in GitHub Releases") and
  [`platform-compatibility`](platform-compatibility.md): Firecracker is the
  Linux-fleet driver, KVM-only, same-architecture only. Release packaging of VM
  assets belongs to that plan; today no VM asset ships in a release
  (`release.yml` and goreleaser ship none). Its "MMDS for metadata" and "vsock
  for signaling" notes are superseded here.
- **Shared Windows guest code** (`driver/winguest`, listed in tier-1's
  `drivers.shared` and `bootstrap.surface`): precedent for pulling a guest
  family's seed code out of one driver into a shared package and registering
  it with the checks; Linux-guest code moves to `driver/linuxguest` the same
  way (Decision 7).
- **Front-matter retrofit** ([`design-front-matter-retrofit`](design-front-matter-retrofit.md)):
  its a6 gave `firecracker-driver-plan.md` the queue slug, its a2 set it
  `draft`, and its a7 left it to this design to resolve, "for example to
  `archived`" (question a1).

Found in passing, outside this design (each a follow-up for intake, listed at
the end of the Implementation plan): `tools/build-vm` bakes `warden-io` in at
asset-build time and its init writes DNS, which BOOT-02, BOOT-03 and BOOT-06 do
not allow and the deviation register does not list; the relay VM init never
checks iptables' exit status and enables forwarding before its rules exist, so a
kernel missing a netfilter feature fails open on every driver; and qemu's image
cache trusts the first download of a cloud image.

## Decisions

1. **Relay unchanged, `vm` mode, the shared init** (spike 4.1; RELAY-04). The
   relay VM runs `tools/relay-vm/init` and the relay at `/sbin/relay`, appended
   as a cpio to a new x86_64 relay initramfs (Decision 10), booted by direct
   kernel boot, diskless, claimed on 8301. The relay binary, `relay/` and
   `tools/relay-vm/init` are **not** modified. Under Firecracker the init's
   `insmod` loop is a no-op: the x86_64 Firecracker initramfs carries no `.ko`
   files, and every feature the loop would load is built into the pinned kernel
   (Decision 10). *Rejected:* a Firecracker-specific relay init or rootfs image
   (the spike's `build-rootfs.sh`), which would fork the trust boundary; adding
   `virtio_mmio` to the init's module list (an earlier draft of this design),
   which does nothing because no Alpine module loads into the pinned kernel.

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
   4.7, 7.8; old Q1; questions a3, a10, a11). `warden setup --driver
   firecracker [--slots N]` (root, once, idempotent; N from 1 to 256, default
   16) creates N slots. Slot *i* (0 ≤ *i* < 256) is:
   - `wfc<i>b`, a bridge with no address, joining two TAPs, `wfc<i>r` (relay
     build NIC) and `wfc<i>g` (build VM NIC), both owned by the warden user;
   - `wfc<i>u`, a TAP owned by the warden user for the relay's upstream, with
     the host address and the relay address taken as offsets into
     `100.64.88.0/22`: host = `100.64.88.0 + 4i + 1`, relay = `100.64.88.0 +
     4i + 2`, each a /30, carried into the third octet (slot 63: `.88.253` and
     `.88.254`; slot 64: `.89.1` and `.89.2`; slot 255: `.91.253` and
     `.91.254`);
   - IPv6 disabled on all four interfaces; `rp_filter=1` on `wfc<i>b` and
     `wfc<i>u`; `arp_ignore=8` on `wfc<i>b` (the host answers no ARP there);
   - a bridge-family nft table `warden_fc` whose input hook drops every frame
     arriving on a `wfc*` bridge port (no frame from a build segment reaches
     the host stack) and whose forward hook accepts frames only between
     `wfc<i>r` and `wfc<i>g` of the same slot;
   - host iptables in a `WARDEN-FC` chain (jumps inserted first in INPUT,
     FORWARD and OUTPUT) and its ip6tables twin:
     - INPUT: `-i wfc+b` drop; from `wfc<i>u` accept ESTABLISHED only, and
       only from the slot's relay address; drop the rest from `wfc+`;
     - FORWARD: from `wfc<i>u` with source the slot's relay address to the
       uplink accept, and replies (ESTABLISHED) back; any other source from
       `wfc<i>u` drop (anti-spoof, so un-NATed build traffic can never leave
       even if the relay's own rules are missing); within the same `wfc<i>b`
       accept (needed when `br_netfilter` is loaded); every other `wfc+`
       forward drop;
     - OUTPUT (host to relays): to `100.64.88.0/22` accept ESTABLISHED; accept
       NEW `tcp/8301` only from the slot owner's UID (`-m owner --uid-owner`);
       drop everything else, so no other local user reaches the relay's
       `:80`, `:443`, `:53` or `:8300`, and the warden user reaches only the
       claim port (a10);
     - ip6tables: drop all INPUT, FORWARD and OUTPUT on `wfc+`;
     - raw PREROUTING: frames on `wfc<i>b` get conntrack zone *i*+1, so the
       slots' identical `10.0.0.1`/`10.0.0.2` flows never share conntrack
       entries when `br_netfilter` is loaded;
   - MASQUERADE in nat POSTROUTING from `100.64.88.0/22` to the default route.

   Every slot reuses the relay init's fixed build link `10.0.0.1/30` to
   `10.0.0.2/30`, because each bridge is its own L2 segment with no host route
   and its own conntrack zone. A build takes a free slot under
   `driver.TryHostLock("firecracker-slot-<i>")` and holds it until teardown
   (Decision 8 covers VMMs that outlive a crashed warden). Builds then need
   only `/dev/kvm` access (the `kvm` group) and the TAPs' ownership; warden
   never runs as root for a build. The upstream range sits next to the
   container driver's `100.64.87.0/24`.

   Host posture checks, in setup (which refuses) and in doctor and the build
   preflight (which report, fixable, UX-04): a filter FORWARD policy of ACCEPT
   (enabling `ip_forward` would then route between the host's other
   interfaces; setup refuses and prints the fix, a11); firewalld running or an
   nft-only ruleset whose own base chains drop `wfc*` forwarding (setup refuses
   and prints the `firewall-cmd` lines that put `wfc+u` in the `trusted` zone,
   which is safe because a drop in `WARDEN-FC` stays final in any netfilter
   hook, a11); `nft`, `iptables` (legacy or nft) and `ip6tables` present.
   *Rejected:* per-build TAP and iptables creation (needs `CAP_NET_ADMIN` on
   every build, so warden as root); slirp4netns for the upstream (extra process
   and latency, and still a TAP for the build link); operator-provided bridges
   (no isolation guarantees warden can check); setup changing firewalld or the
   FORWARD policy itself (a host posture change the operator should make).

5. **Claim over the upstream TAP** (no relay change). The host claims at the
   slot's relay address, port 8301, with a per-build token on the kernel
   command line, exactly as Hyper-V claims at the relay's static NAT address.
   Kernel command line: `console=ttyS0 reboot=k panic=1 pci=off
   warden.net=static warden.nat_ip=… warden.nat_prefix=30 warden.nat_gw=…
   warden.dns=… warden.claim_token=…`. Upstream DNS is the host's resolvers
   (from `/run/systemd/resolve/resolv.conf` when systemd-resolved fronts
   `127.0.0.53`), excluding any loopback address and any address assigned to
   the host itself (a local dnsmasq, libvirt or unbound on a LAN address),
   which INPUT would drop as NEW traffic. With none left, the preflight fails
   with a fixable error naming the local resolver and how to point warden at
   an off-host one (UX-04); doctor reports the same.

6. **Executor delivery: the qemu Linux-guest seed, and a minimal direct-boot
   launcher** (spike 4.10; BOOT-02, BOOT-03, BOOT-06).
   - *Disk path* (a cloud image): the per-build cloud-init NoCloud `CIDATA`
     ISO, the same seed qemu builds, attached as a second, read-only virtio-blk
     drive (Firecracker has no CD-ROM; cloud-init finds the seed by label). The
     launcher is cloud-init `runcmd`, ending in `reboot -f` instead of
     `poweroff -f`, because Firecracker exits on a guest reset, not a poweroff.
   - *Direct-boot path* (no resolvable image): a new x86_64 base initramfs
     (`tools/firecracker-vm/build-initramfs.sh`: static busybox and a launcher,
     no `warden-io`, no modules) with `warden-io` appended per build as a cpio
     at `/bin/warden-io`. The launcher mounts `/dev`, `/proc` and `/sys`, sets
     the static `10.0.0.2/30` via `10.0.0.1` on its only NIC, and execs
     `warden-io initialize --gateway=10.0.0.1`. It writes no DNS (that is
     `initialize`'s, BOOT-03) and nothing else.
   Trust, the build script and context come from the relay. *Rejected:* writing
   the CA or script into the build rootfs (spike 4.10; BOOT-02, BOOT-05), MMDS
   (Decision 3); reusing `tools/build-vm` (arm64, `warden-io` baked in, DNS in
   the init); dropping direct boot (it is the spike's fastest end-to-end path
   and the fallback for builds with no `FROM` image).

7. **Build rootfs: cloud image converted once to raw, copied per build on the
   cache's filesystem** (spike 4.4, 4.5, 7.6; old Q5; question a6). Firecracker
   needs raw block devices. The cloud image comes from the qemu catalog via
   `driver/linuxguest` (download and cache unchanged). Conversion:
   `qemu-img info --output=json` first, refusing any image with a
   `backing-filename` or a format other than `qcow2` or `raw`; then
   `qemu-img convert -f <format> -O raw` into
   `<cache>/firecracker/images/<name>.raw`, recorded in
   `<cache>/firecracker/images/layers.json` as a `prepared` layer whose input
   is the downloaded image's hash. Each build copies the raw image with
   `cp --reflink=auto` into a per-build directory
   `<cache>/firecracker/work/<build-id>/`, on the cache's filesystem (so reflink
   works where the filesystem supports it, and a `tmpfs` temp dir never holds a
   multi-GB rootfs); the rest of the work dir stays under `driver.NewWorkDir`.
   The per-build copy is removed in teardown (ORCH-04) and reaped by `warden
   clean`; doctor warns when the cache's filesystem lacks reflink (each build
   then copies the whole image). The image's own kernel is not used:
   Firecracker boots the pinned kernel (Decision 10) with `root=PARTUUID=<id>`,
   where `<id>` comes from a small Go GPT reader in `driver/firecracker` that
   reads the raw image's primary GPT header and entries on the host (no mount,
   no loop device) and picks the partition with the x86-64 root type GUID, else
   the single Linux-filesystem partition, else fails with a fixable error.
   *Rejected:* converting per build (slow, and a fresh hash per build);
   device-mapper thin snapshots now (root per build or a privileged helper; a
   follow-up for fleets); overlayfs inside the guest (setup code in the image,
   BOOT-06); the per-build copy in the temp dir (a cross-filesystem full copy,
   in RAM on `tmpfs`). Default catalog unchanged from qemu (spike 7.2); custom
   images use `--image <path>` as qemu does and go through the same
   `qemu-img info` check.

8. **Process model: `firecracker --no-api --config-file`** (old "API socket"
   risk). Each VM is one `firecracker` process started from a JSON config in
   the per-build work dir (0700), with no API socket at all and Firecracker's
   default seccomp filters on. Its stdin is `/dev/null`: the serial console is
   output only, tee'd to the driver (shown with `--verbose`, as qemu does), so
   no host bytes reach either guest through ttyS0. Each process starts with
   `Setpgid` and `Pdeathsig: SIGKILL` (Linux only; the starting goroutine holds
   `runtime.LockOSThread` for the child's lifetime, since `Pdeathsig` follows
   the creating thread), and its pid and start time are written to
   `<lockdir>/firecracker-slot-<i>.pids`. On acquiring a slot, the driver kills
   any VMM still recorded for that slot whose `/proc/<pid>` start time and
   `exe` match (a VMM that outlived a SIGKILLed or OOM-killed warden and still
   holds the slot's TAPs); `warden doctor` reports such orphans and `warden
   clean` reaps them. The relay VM gets Firecracker's entropy (virtio-rng)
   device.

   **Exit ordering.** On a successful build the build VM's process exits
   (`reboot -f` on the disk path; on direct boot PID 1 exits and `panic=1`
   resets the VM). The driver runs `session.Wait` and watches both processes:
   when the build VM's process exits, it waits up to a bounded grace
   (`completionGrace`, 10 s) for the session's completion, checking
   `sess.Collector.Status()`, and only then cancels `Wait` and fails the build
   with the console tail; a completion that arrives in the grace wins. The
   relay VM's process exiting before completion fails the build at once.
   This lives in `driver/firecracker` and needs no `driver/` root change.

   Resource defaults match qemu (spike 7.4): relay 2 vCPUs and 512 MiB (the
   spike may show the initramfs relay needs less; lowering it is an
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

10. **Boot assets: one pinned kernel and two new x86_64 initramfs builds**
    (spike 4.9, 7.1; old Q4; question a5).
    - *Kernel:* `tools/firecracker-vm/build-kernel.sh` builds a pinned 6.1 LTS
      kernel from Firecracker's published microVM guest config plus a checked-in
      fragment of options that must be built in (`=y`; nothing is a module, and
      the kernel loads no modules): virtio and devices (`VIRTIO_MMIO`,
      `VIRTIO_MMIO_CMDLINE_DEVICES`, `VIRTIO_NET`, `VIRTIO_BLK`,
      `HW_RANDOM_VIRTIO`, `SERIAL_8250_CONSOLE`); the relay init's networking
      (`PACKET`, `NETFILTER`, legacy `IP_NF_IPTABLES`, `IP_NF_FILTER`,
      `IP_NF_NAT`, `NF_CONNTRACK`, `NF_NAT`, `NF_DEFRAG_IPV4`,
      `NETFILTER_XT_TARGET_REDIRECT`, `NETFILTER_XT_TARGET_MASQUERADE`,
      `NETFILTER_XT_MATCH_CONNTRACK`, `NETFILTER_XT_MATCH_STATE`,
      `NETFILTER_XT_MATCH_TCPUDP` where the version has it, `CRC32C`);
      filesystems (`EXT4_FS`, `ISO9660_FS` with Joliet, `VFAT_FS`,
      `NLS_CODEPAGE_437`, `NLS_ISO8859_1`, `TMPFS`, `TMPFS_POSIX_ACL`,
      `DEVTMPFS`, `DEVTMPFS_MOUNT`, `AUTOFS_FS`); partitions
      (`PARTITION_ADVANCED`, `EFI_PARTITION`, `MSDOS_PARTITION`); systemd's
      minimums for the cloud images (`CGROUPS` and cgroup v2 controllers,
      `CGROUP_BPF`, `BPF_SYSCALL`, `SECCOMP`, `SECCOMP_FILTER`, `FHANDLE`,
      `INOTIFY_USER`, `SIGNALFD`, `TIMERFD`, `EPOLL`, `UNIX`, `NET_NS`,
      `USER_NS`, `SYSFS`, `PROC_FS`); entropy (`RANDOM_TRUST_CPU` alongside the
      virtio-rng device). A tier-0 test parses the pinned config and fails if
      any listed option is not `=y`. Output is an uncompressed `vmlinux`
      (x86_64), used by both VMs. Building is required rather than downloading
      Firecracker's CI kernels, which do not promise legacy iptables NAT, VFAT,
      the NLS codepages or GPT.
    - *Relay initramfs:* `tools/relay-vm/build-initramfs.sh` takes an
      architecture (`ARCH=aarch64` default, byte-identical output for qemu;
      `ARCH=x86_64` for Firecracker) and a no-modules mode, which the Firecracker
      build uses, so the init's `insmod` loop finds nothing to load. Same Alpine
      minirootfs, iptables and the unchanged init; nothing baked in (the relay
      is appended per build, as on qemu). Hyper-V's build
      (`tools/relay-vm/hyperv/build.sh`) stays separate: it targets VMBus,
      bakes the relay into a UKI and builds offline from a Windows-fetched
      cache; merging the two is not this feature's to do.
    - *Direct-boot initramfs:* `tools/firecracker-vm/build-initramfs.sh`
      (Decision 6).
    - *Shipping:* source-tree built, as qemu's relay assets are today. The
      driver finds them beside the warden binary, then in the tools output
      directories, through shared lookup code in `driver/relayvm` (Decision
      13); the preflight's fixable error names the script to run. Release
      packaging of VM assets is recorded as a gap for
      [`vm-driver-release-plan`](vm-driver-release-plan.md), which owns
      "pre-built kernel+initramfs in GitHub Releases". Not configurable.

11. **No jailer, no extra confinement in this feature** (spike 4.3, 7.5; old
    Q2; question a4). The jailer needs root to chroot and drop to a per-VM UID,
    which would undo Decision 4's unprivileged builds. Firecracker runs as the
    invoking user with its default seccomp filters; nothing else confines it.
    The blast radius of a VMM escape is therefore the warden user's full
    authority (Security note). Confinement (the jailer with a UID range and
    per-slot network namespaces, or unprivileged Landlock applied by a warden
    re-exec helper before `exec firecracker`, plus a cgroup scope) is a
    follow-up design for multi-tenant fleets.

12. **CLI and config are additive, and every existing VM surface learns the
    driver** (spike 9.2; ORCH-01, ORCH-03, ORCH-04, UX-01, UX-07). New:
    - the `firecracker` value for `--driver` and `runtime.driver`
      (`validateDriver` and its error text);
    - `warden setup --driver firecracker [--slots N]`; `warden doctor --driver
      firecracker` (preflight, slots, rules, sysctls, posture, resolver,
      reflink, orphaned VMMs);
    - `warden image prepare --driver firecracker [source]` (download if needed
      and convert to raw, so the first build does not pay for it), `warden image
      list` listing the raw cache (it globs only `*.qcow2` today), `warden
      image verify` covering `<cache>/firecracker/images/layers.json`
      (registered in `layerCaches`; its fixable hint names firecracker).
    Changed: `validateFlagsForDriver` and `main.go`'s `isVM` include
    firecracker, so `--image`, `--script`, `--cpus`, `--memory` and
    `--keep-vms` work and the "requires a VM driver" hint names it;
    `validateGuestOS` rejects `--guest-os windows` and macOS for firecracker;
    `--network` is rejected for firecracker (slots are fixed by setup);
    `--keep-vms` leaves both VMs running and the slot held until the user stops
    them, as qemu leaves its VMs, and prints the pids; `warden clean` no longer
    calls `resolveRuntime` first when the driver is firecracker (it must reap
    on a host with no Docker or Podman) and reaps stale Firecracker work dirs,
    per-build rootfs copies and orphaned VMMs; `warden shell` returns
    `ErrExecNotSupported`, as qemu does. The Firecracker binary is taken from
    `PATH` and checked against a pinned minimum version (one with the entropy
    device; the spike fixes the exact version); no binary-path flag or key.
    Removing cached raw images is `warden clean` / `warden image` territory as
    for qemu (ORCH-04).

13. **Shared relay-VM lifecycle in `driver/relayvm`; Linux guest code in
    `driver/linuxguest`** (ORCH-01). Relay asset resolution
    (`resolveRelayAssets`), relay initrd composition (`relayInitrd`) and the
    cpio append (`appendFileCPIO`) are relay lifecycle, not guest code: they move
    from `driver/qemu` to a new `driver/relayvm` that qemu and firecracker both
    import, extended to look up the kernel and the direct-boot initramfs.
    `driver/linuxguest` takes only the Linux guest family's code: the NoCloud
    seed, the ISO writer, and cloud-image resolution and download. Both moves
    are behavior-preserving for qemu.

14. **Environment record: exact layers** (ledger-environment-types; BOOT-02,
    RELAY-02). `driver/firecracker`'s `describeBuild` mirrors qemu's, into
    `vm-disk-chain/v1` with driver `firecracker`, guest OS `linux`, arch
    `x86_64`:
    - *Disk path:* layers `[kernel: vmlinux, disk: <name>.raw (format raw,
      source prepared {inputs: the cloud image's sha256, prepared by warden
      <version>})]`; seed: the `CIDATA` files, the build script, and a
      `kernel-cmdline` file holding the build VM's command line (so the record
      names the kernel and how it was told to boot; the build VM's line carries
      no secret).
    - *Direct boot:* layers `[kernel: vmlinux, initrd: the base direct-boot
      initramfs]`; seed: `warden-io` (the appended file), the build script and
      `kernel-cmdline`.
    The per-build rootfs copy is the writable child, not a layer. That departs
    from the convention that the child is "created empty from the last disk"
    (qemu's empty qcow2 overlay): Firecracker has no overlay format, so the
    child starts as a byte copy of the raw layer, whose hash is the layer's.
    Golden tier-0 tests cover both paths.

15. **Linux-only behind build tags** (GLOBAL-07). `driver/firecracker` has
    `*_linux.go` files and a stub for other hosts whose `StartBuild` returns a
    fixable "needs a Linux host with KVM" error, so `cmd/warden` builds for
    every target. Same-architecture only; x86_64 in this feature (question a9).

How the plans' items end up:

| Plan item | Outcome |
|---|---|
| Old Q1 relay networking; spike 4.7, 7.8 subnet | Folded into Decision 4 (setup slots, `100.64.88.0/22`); questions a3, a10, a11 |
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

None. Every open point has a recommended default the design proceeds on. Note
that a8 gates Implement in practice: the Phase 0 spike is the queue step
`f2-spike-linux-kvm` (`host:linux-kvm`), phase 2 cannot finish until it is
done, and no machine can claim it until `hosts.yaml` serves `linux-kvm`.

### Assumed

Each is "proceeding with the recommendation; confirm" in the design review
(FLOW-19). They are also in the queue file.

- **a1 — How the archive records the two superseded plans.** This overrides two
  answers Jeff gave in the front-matter retrofit: its a2 ("`draft` when nothing
  it describes is built", which set `firecracker-driver-plan` to `draft`, and
  rejected an `archived` status with a comment because a search on `status`
  alone would read the document as complete) and its a6 (`feature` is the queue
  slug where one exists, which gave the old plan `firecracker-driver`). Its a7
  anticipated this design amending the old plan "for example to `archived`".
  Trade-off: `archived` keeps unbuilt plans out of searches for live designs,
  but a `status`-only search reads them as delivered; `draft` keeps the
  retrofit's meaning but leaves two unbuilt documents that look like competing
  live designs for one feature; the shared slug fails tier-1's
  feature-equals-file-stem check unless tolerated. Options: (a) both
  `archived`, a `# Superseded by firecracker-driver.md (never built)` comment
  on `status`, `feature` set to each file stem, the `design_tolerate` entry
  dropped (as now on the branch); (b) both stay `draft` with a superseded
  comment, the old plan keeps `feature: firecracker-driver`, the spike plan
  takes it too, and `design_tolerate` lists both for `feature`; (c) add a
  `superseded` status to `TEMPLATE.md` and to wf's front-matter validator, a
  process item (FLOW-14), and use (a) until it lands. Recommended: (a), because
  the retrofit's a7 named it, because "nothing it describes is built" stops
  being the useful signal once a successor design exists (the successor is
  where the live status is), and because only (a) passes tier-1 without a
  tolerance; the `status` comment and `related` both point at this design, and
  (c) is the clean fix if a status-only search matters.
- **a2 — Treat the new mapping row as a bootstrap-model change.** Proceeding
  with: declare the `bootstrap-model` seam, fill the Bootstrap comparison, and
  record `alters_relationships: true`, though no invariant changes. Options:
  as stated (recommended, conservative under BOOT-07 and FLOW-02); treat the row
  as "correcting the mapping to match the code", with no seam and
  `alters_relationships: false` (the design review would then be judged without
  the comparison).
- **a3 — Network slots made once by `warden setup`.** Proceeding with Decision 4:
  a root-once `warden setup --driver firecracker [--slots N]` (default 16, at
  most 256) makes durable bridges, TAPs, sysctls, the `warden_fc` bridge table
  and the `WARDEN-FC` rules; builds run unprivileged. Options: setup slots
  (recommended); per-build TAP and iptables creation with warden run as root or
  with `CAP_NET_ADMIN`; slirp4netns for the upstream.
- **a4 — No jailer and no extra confinement in this feature.** Proceeding with
  Decision 11 and an accurate blast radius in the Security note. Options: no
  confinement now, follow-up design (recommended: either route needs its own
  design, the jailer conflicts with unprivileged builds, and Landlock needs a
  re-exec helper and a policy that tracks Firecracker's file use); unprivileged
  Landlock via a warden re-exec helper now; optional `--jailer` (requires root
  builds); jailer by default.
- **a5 — One pinned 6.1 LTS guest kernel, built by script, source-tree built
  like qemu's assets.** Proceeding with Decision 10. Options: 6.1 from
  Firecracker's config plus the required-options fragment (recommended); 5.10
  or 6.6; download Firecracker's CI-published kernels by pinned hash (cheaper,
  but they lack options Decision 10 requires); make the kernel configurable.
  Release packaging is a gap recorded for `vm-driver-release-plan`, not this
  feature.
- **a6 — Raw image converted once into the cache, `cp --reflink=auto` per
  build onto the cache's filesystem.** Proceeding with Decision 7. Options: as
  stated (recommended); convert per build; device-mapper thin snapshots now;
  require `--temp-dir` to share the cache's filesystem instead.
- **a7 — Relay upstream reaches what qemu's relay reaches, minus the host and
  other slots.** Proceeding with: the `WARDEN-FC` rules block the relay from
  opening connections to the host and to other slots (both required by the
  Security note), but do not block link-local or private ranges (for example a
  cloud host's `169.254.169.254`), keeping parity with the qemu and container
  drivers, whose relays can reach them today. Options: parity (recommended),
  with a security-suite follow-up across drivers; block `169.254.0.0/16` for
  Firecracker only (diverges from ORCH-01 parity); block link-local on every
  driver (a separate cross-driver design).
- **a8 — Where `linux-kvm` runs.** Proceeding with: `windows-desktop` serves
  `linux-kvm` from WSL2 (add it to that machine's hosts in `hosts.yaml`) for
  the Phase 0 spike and the blocking validation, and Implement adds a CI tier-2
  job on GitHub's `ubuntu-latest` (which exposes `/dev/kvm` once the standard
  udev rule grants it) that runs the same checks; once that job has passed on
  this branch, the design review or Publish may reclassify `linux-kvm` as
  covered by CI (TEST-02 prefers CI). WSL2 prerequisites, checked at the start
  of every native run: `nestedVirtualization=true` in `.wslconfig` on a CPU
  that supports it; `/dev/kvm` present and the native user in the `kvm` group;
  a WSL kernel with `bridge`, `tun`, `br_netfilter`, legacy iptables NAT,
  `nf_tables` bridge family and conntrack zones; and `sudo warden setup --driver
  firecracker` re-run first, because the WSL VM shuts down when idle and loses
  the slots even though the machine stays on. The native session itself runs on
  the Windows side, so the workflow-native procedure enters WSL for this host.
  Options: windows-desktop under WSL2 plus the CI job (recommended); the CI job
  alone (the spike would then iterate through CI runs, slow for boot debugging,
  and it is not native validation under FLOW-09 until it is proven); a new
  dedicated Linux machine.
- **a9 — x86_64 hosts only in this feature.** Proceeding with x86_64 (the
  common CI fleet host); arm64 Firecracker (a PE `Image` kernel, the existing
  aarch64 relay initramfs) is a tracked gap until an arm64 Linux KVM host
  exists. Options: x86_64 only (recommended); both now (no validation host for
  arm64).
- **a10 — Close host-local access to the relay with a host OUTPUT filter, not
  a relay init change.** In `vm` mode the relay serves `:80`, `:443`, `:53`
  and the unauthenticated control plane `:8300` on every interface, and the
  relay init filters INPUT only on its build link, so without a host rule any
  local user could drive a running build's control plane or proxy through its
  relay into its ledger. Proceeding with: the `WARDEN-FC` OUTPUT rules in
  Decision 4 (only the slot owner's UID, only NEW `tcp/8301`). Options: host
  OUTPUT filter (recommended: no relay change, and the relay is reached only
  through `wfc<i>u`); relay init accepts only `8301` and ESTABLISHED on its
  upstream (fixes every driver, Hyper-V included, but is a relay VM change
  under GLOBAL-06 and belongs in its own cross-driver item, listed as a
  follow-up); both.
- **a11 — Setup refuses on host firewall states it cannot make safe.**
  Proceeding with Decision 4's posture checks: setup refuses when the filter
  FORWARD policy is ACCEPT or when firewalld or an nft-only ruleset would drop
  slot forwarding, and prints the commands to fix it. Options: refuse with the
  fix (recommended: the operator owns host posture); warn and continue; change
  the FORWARD policy or firewalld zones automatically.

## Design

### Topology

```
HOST (warden, unprivileged; slot i taken under a host lock)
  collector <== tunnel (host dials relay(i):8301; OUTPUT allows only this) ==+
                                                                            |
  wfc<i>u  host(i)/30  --MASQUERADE (source relay(i) only)--> uplink        |
     |                                                                      |
  +--+------------------------------ Relay VM (firecracker) ----------------+--+
  |  eth1  relay(i)/30            (upstream, static, from cmdline)            |
  |  eth0  10.0.0.1/30            (build link)                                |
  |  x86_64 Alpine initramfs (no modules) + /sbin/relay cpio (vm mode)        |
  |  entropy device; diskless                                                 |
  +--+-------------------------------------------------------------------------+
     |
  wfc<i>r ==== wfc<i>b (bridge: no address, IPv6 off, nft input drop, ==== wfc<i>g
                         conntrack zone i+1)                                  |
  +---------------------------------------------------------------------------+--+
  |  Build VM (firecracker)   eth0 10.0.0.2/30 -> 10.0.0.1 only                  |
  |  disk path:   rootfs.raw (rw copy on the cache fs) + CIDATA seed (ro)        |
  |  direct boot: base initramfs (launcher only) + warden-io cpio (per build)    |
  +------------------------------------------------------------------------------+

host(i)  = 100.64.88.0 + 4i + 1      relay(i) = 100.64.88.0 + 4i + 2     (i < 256)
```

The build VM's only NIC is on a bridge whose only other port is the relay's
build NIC. The bridge has no address, the host answers no ARP on it, and the
bridge table drops every frame headed for the host stack, so nothing from the
build VM reaches the host or the uplink except through the relay's proxy, DNS
and control plane. The relay's upstream can send only from its own address,
only out of the uplink, and the host reaches the relay only on its claim port.

### StartBuild

```
StartBuild(req):                                   # driver/firecracker, linux only
    preflight: /dev/kvm rw, firecracker >= pinned, kernel + relay + direct-boot
               assets (relayvm), qemu-img (disk path), a setup slot owned by
               this user, an off-host upstream resolver            # UX-04
    script := StageBuildScript; RecordInputs                      # ORCH-02
    resolve build boot: disk path (cached raw, else qemu-img info check +
                        convert) or direct boot
    session := StartSession(...)
    slot := first free slot under TryHostLock("firecracker-slot-<i>")
            kill stale VMMs recorded in the slot's pid file
    relay initrd := relayvm.RelayInitrd(x86_64 relay initramfs, relay binary)
    start relay VM: firecracker --no-api --config-file relay.json
        stdin /dev/null; Setpgid; Pdeathsig SIGKILL; pid recorded
        drives: none; NICs: eth0=wfc<i>r, eth1=wfc<i>u; entropy device;
        boot args per Decision 5
    session.ClaimRelay(relay(i):8301, token)                      # tunnel + /v1/ready
    deliver: disk path   -> cp --reflink=auto raw -> <cache>/firecracker/work/<id>/rootfs.raw;
                            PARTUUID from the GPT reader;
                            CIDATA ISO (linuxguest) -> work/seed.iso
             direct boot -> base initramfs + warden-io cpio (relayvm) -> work/build-initrd
    env := describeBuild (Decision 14)                            # vm-disk-chain/v1
    start build VM: firecracker --no-api --config-file build.json
        same process attributes; NIC: eth0=wfc<i>g only;
        drives: rootfs (rw) + seed (ro), or none
    wait: session.Wait, until
          - completion                                  -> result
          - build VM exits -> wait <= completionGrace for Collector.Status()
                              -> result, else fail with console tail
          - relay VM exits                              -> fail with console tail
    teardown: stop both VMs, remove work dir and rootfs copy, clear the pid
              file, release slot lock                     # ORCH-04 (skipped VMs with --keep-vms)
    FinalizeOutput
```

The output directory is identical to the other drivers' (RELAY-07); nothing is
read back from any VM image.

### Host setup and doctor

```
warden setup --driver firecracker [--slots N]:   # root, idempotent, 1 <= N <= 256
    check: Linux, /dev/kvm, the invoking (sudo) user's kvm group membership,
           nft + iptables + ip6tables present
    refuse (fixable, printed commands): filter FORWARD policy ACCEPT;
           firewalld running or nft-only rules that drop wfc* forwarding
    for i in 0..N-1: create or verify wfc<i>b, wfc<i>r, wfc<i>g, wfc<i>u
                     (TAPs owned by the user; IPv6 off on all four;
                      rp_filter=1 on b and u; arp_ignore=8 on b;
                      address only on wfc<i>u)
    create or verify: bridge table warden_fc (input drop, same-slot forward only);
                      WARDEN-FC (INPUT, FORWARD, OUTPUT, anti-spoof, raw zones,
                      MASQUERADE) and its ip6tables drops; jumps first in each chain
    net.ipv4.ip_forward=1
    print what was created and what was verified

warden doctor --driver firecracker:              # read-only
    each preflight item above, plus: slots present and owned by this user;
    sysctls, bridge table and WARDEN-FC rules present as setup makes them;
    FORWARD policy and firewalld state; a non-loopback, off-host upstream
    resolver; reflink on the cache filesystem (warn); orphaned VMMs (report)
```

Setup state does not survive a reboot (TAPs, sysctls and rules are runtime
state); `warden doctor` says to re-run setup, and the guide shows a systemd
unit that runs it at boot. Removing the slots is documented as the matching
`ip link del`, `nft delete table bridge warden_fc` and chain flush; it is not a
warden command (ORCH-04 treats the network as durable).

### Packages

- `driver/firecracker`: `driver_linux.go` (StartBuild, wait and exit ordering,
  teardown), `deliver_linux.go` (both delivery paths, kernel command line,
  `describeBuild`), `vm_linux.go` (config JSON, process start with its
  attributes, pid files, watch), `slots_linux.go` (slot naming, address
  arithmetic, locking, stale-VMM reaping, setup and doctor command lists behind
  a runner), `images_linux.go` (`qemu-img` checks, raw conversion and cache,
  per-build copy), `gpt.go` (the GPT reader, portable for tier-0 tests),
  `driver_other.go` (stub), tests.
- `driver/relayvm` (new, moved from `driver/qemu`): relay and direct-boot asset
  resolution, relay initrd composition, cpio append.
- `driver/linuxguest` (new, moved from `driver/qemu`, behavior-preserving): the
  NoCloud seed, the ISO writer, cloud-image resolution and download. The
  cloud-init `runcmd`'s last line becomes a parameter (`poweroff -f` for qemu,
  `reboot -f` for Firecracker).
- `tools/firecracker-vm/`: `build-kernel.sh` and its config fragment;
  `build-initramfs.sh` and its launcher (direct boot).
  `tools/relay-vm/build-initramfs.sh`: `ARCH` and no-modules parameters.
  `tools/relay-vm/init`: unchanged.
- `cmd/warden`: everything in Decision 12.
- `tools/tier1/rules.yaml`: `drivers.shared` gains `driver/linuxguest/` and
  `driver/relayvm/`; `drivers.all` gains `firecracker` once the driver exists;
  `bootstrap.surface` replaces `driver/qemu/cloudinit.go` with
  `driver/linuxguest/` and adds `driver/firecracker/slots_linux.go`,
  `driver/firecracker/deliver_linux.go`, `driver/firecracker/vm_linux.go` and
  `tools/firecracker-vm/` (isolation, delivery and start).

### Proposed standards change (FLOW-14)

`docs/development/bootstrap.md` changes in two places, proposed here for the
design review to accept explicitly (BOOT-07); Implement applies them only after
that acceptance, in the same diff as the code they describe, or they move to a
process item if the review prefers:

1. The per-driver mapping gains the firecracker row in the Bootstrap
   comparison below.
2. BOOT-07's bootstrap-surface list replaces `driver/qemu/cloudinit.go` with
   `driver/linuxguest/` and adds the `driver/firecracker` isolation, delivery
   and start files and `tools/firecracker-vm/`, matching `rules.yaml`.

### Overview change (FLOW-15)

`overview.md` gains `firecracker` in the driver list and a Firecracker line in
each step of "VM drivers": isolate (a TAP on a per-slot host bridge whose only
other port is the relay), deliver (the qemu Linux seed as a read-only drive,
or `warden-io` appended to the direct-boot initramfs), start (cloud-init
`runcmd`, or the launcher's exec of `initialize`).

## Cross-driver impact

- **firecracker** (new): everything above.
- **qemu**: its Linux-guest seed, ISO writer and cloud-image code move to
  `driver/linuxguest`, and its relay asset lookup, relay initrd and cpio append
  move to `driver/relayvm`, with no behavior change (the `runcmd` tail stays
  `poweroff -f`). `tools/relay-vm/build-initramfs.sh` gains `ARCH` (default
  `aarch64`) and a no-modules mode; qemu's output is byte-identical. Covered by
  tier-0 tests that move with the code, a byte-identical seed test, and the
  tracked `qemu-any` gap.
- **hyperv**: no code change; its relay boot disk build and the relay init are
  not touched. Declared for the `cmd/warden` wiring and the new shared
  `driver/` packages (TEST-02 caveat); tracked `windows-hyperv` gap.
- **container**: no code change. Declared for the same reasons; tier-1
  integration CI covers it.
- **vz**: no code change; declared for the same reasons; tracked `macos-vz`
  gap.
- `relay/`, `cmd/relay/`, `tools/relay-vm/init`, `collector/`, `ledger/`,
  `relaycfg/`, `cmd/warden-io/`, the `driver.Driver` interface and
  `BuildRequest` / `BuildResult` do not change (spike section 9).

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
| firecracker / Linux | Topology: the build VM's only NIC is a TAP on a per-slot host bridge whose only other port is the relay VM; the bridge has no address and drops every frame bound for the host | cloud-init `CIDATA` ISO per build as a read-only virtio-blk drive (disk path), or `warden-io` appended per build to a launcher-only initramfs (direct boot) | cloud-init `runcmd` (disk path); the launcher's exec of `warden-io initialize` (direct boot) |

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | Topological for VM drivers; holder plus sidecar for container | Unchanged; Firecracker is topological: the bridge and its drops exist (setup) and the relay is claimed before the build VM process starts | firecracker: by construction; others: none |
| BOOT-02 executor is the only out-of-band input | Seed disk, `CIDATA` ISO, tar copy; vz baked (D2) | Unchanged; Firecracker delivers only `warden-io` and the cloud-init launcher, per build; VMM stdin is `/dev/null` | firecracker: no MMDS, no rootfs writes, no console input; others: none |
| BOOT-03 one start, then hands off | Boot mechanism or entrypoint starts `initialize`; host only watches | Unchanged; the direct-boot launcher only sets the static address and execs `initialize`, which owns DNS; the host only watches the collector and the VMM processes | firecracker: process exit is lifecycle state, and a completion seen within the grace takes precedence over it; others: none |
| BOOT-04 inputs come from the relay | CA, script, context fetched by the executor | Unchanged | none |
| BOOT-05 setup is the executor's domain | Baseline plus `/bootstrap` policy | Unchanged; static `network-config` in the seed only | none |
| BOOT-06 images carry hooks, not content | Generic hooks only | Unchanged; the converted raw image is the cloud image's content plus nothing; the direct-boot base initramfs holds a generic launcher and no `warden-io` | firecracker: none; qemu's `tools/build-vm` deviation is not inherited, and is filed as a follow-up |

### Gains

A fifth driver that is in the model from its first commit, with no deviation
entry, and with no new input path into the build environment: the record names
the kernel, the disk or initrd, the seed and the kernel command line, so what
the build consumed is at least as completely recorded as on qemu.

### Costs

None to the invariants. The security cost is on the host side (bridges, TAPs,
sysctls, NAT and filter rules made by a root setup step, and host-local
reachability of the relay that the filter closes), covered in the Security
note, not in what crosses into the build environment. The record gains the
kernel command line and loses nothing.

## Security note

Boundaries: **host-env** (the host OS and the build environment) and
**env-relay** (the relay VM and the build VM; new here, host-local processes
and the relay VM). The relay binary and the relay VM init do not change, so
GLOBAL-06's "any change to the relay" does not apply; the relay's environment
does change (a new kernel, a new initramfs build, a new network path to the
host).

- **New host network state, made by root once.** `warden setup` runs as root and
  leaves bridges, TAPs, per-interface sysctls, the `warden_fc` bridge table,
  `ip_forward=1` and the `WARDEN-FC` chains. Builds afterwards need no
  privilege beyond `/dev/kvm`, which is less than the plans' per-build
  `CAP_NET_ADMIN`. Setup only creates or verifies named `wfc*` devices and its
  own table and chains; it never flushes or reorders other rules.
  `ip_forward=1` is host-wide: on a host whose FORWARD policy is ACCEPT it would
  make the host route between all its interfaces, so setup refuses there and
  doctor reports it (a11). Under firewalld or an nft-only ruleset setup refuses
  and prints the zone change; WARDEN-FC drops stay final whatever another
  table accepts.
- **The build VM cannot reach the host.** Isolation does not rest on the bridge
  having no address: the bridge table drops every frame from a `wfc*` port
  bound for the host stack, the host answers no ARP there (`arp_ignore=8`),
  `rp_filter=1` rejects packets with no route back, INPUT drops `wfc+b`, and
  IPv6 is off on the bridge and both TAPs with ip6tables dropping `wfc+` as a
  backstop. Each layer alone blocks the path, so a host with `rp_filter=0`
  (Kubernetes and some Docker hosts) stays closed. With `br_netfilter` loaded,
  bridged frames are accepted only within the same bridge, in their own
  conntrack zone.
- **The relay cannot open connections to the host or to other slots, or send
  un-NATed traffic.** INPUT from `wfc<i>u` is ESTABLISHED-only and only from
  the slot's relay address; forwarding from `wfc<i>u` requires that source, so
  build traffic that escaped the relay's own NAT (for example on a kernel
  missing a netfilter feature) is dropped, not routed; forwarding between slots
  is dropped; IPv6 is off on `wfc<i>u` with ip6tables drops behind it. What the
  relay can reach upstream is otherwise what qemu's relay reaches (a7).
- **Host-local processes cannot reach the relay** (a10). The relay serves
  `:80`, `:443`, `:53` and an unauthenticated control plane (`:8300`:
  `/v1/complete` ends the build with any exit code, `/v1/output` injects
  outputs) on every interface. The OUTPUT rules allow only NEW `tcp/8301` from
  the slot owner's UID plus ESTABLISHED, so other local users reach nothing on
  a relay. Processes running as the warden user can reach the claim port only;
  the per-build token guards it and the relay stops listening after one claim.
  Root is out of scope (it owns the host).
- **The relay's own rules on a new kernel.** The relay init applies its
  iptables rules without checking their exit status, after enabling forwarding.
  The pinned kernel builds in every feature those rules use (Decision 10), a
  tier-0 test fails if any option is not `=y`, and the spike and tier 2 check
  the rules' effect (un-proxied egress blocked, proxied traffic in the ledger).
  The host's anti-spoof rule is the backstop if a rule is still missing. The
  init's fail-open behavior on every driver is filed as a follow-up.
- **Slot exclusivity and orphans.** A slot is held by a host file lock for the
  whole build. VMMs die with warden (`Pdeathsig`), and a VMM that survives
  anyway is killed when the next build takes the slot, using the slot's pid
  file checked against `/proc` start time and `exe`; doctor reports orphans.
  The TAPs are owned by the warden user, so other unprivileged users cannot
  attach a VMM to them.
- **VMM surface and blast radius.** No API socket (`--no-api`), no vsock, no
  MMDS, no shared filesystem, stdin `/dev/null`; two virtio-blk drives at most
  and one NIC for the build VM; an entropy device on the relay. Firecracker's
  default seccomp filters stay on. Nothing else confines the VMM (a4): an
  escape from the build VM into its VMM process holds the warden user's full
  authority, including the user's credentials and files, other concurrent
  builds' work dirs, output dirs and ledgers, the lock dir, and every slot's
  TAPs. That is the same as qemu today; a jailer or Landlock design is the
  follow-up.
- **No untrusted filesystem is mounted on the host.** The plans read signals and
  ledgers by mounting VM images; this design never mounts a guest or relay image
  on the host. The GPT reader parses two sectors and the partition array of the
  raw image with bounds checks, as untrusted input (the image is the user's, not
  the build's, but may be custom).
- **Keys stay per relay (RELAY-03).** No snapshot or warm pool (Decision 9); a
  future pool must meet the three conditions there. The relay VM gets the
  entropy device, the kernel trusts the CPU's RNG, and Go's `crypto/rand` uses
  `getrandom`, which blocks until the CRNG is ready; the spike checks the
  console shows `crng init done` before the relay reports ready.
- **Image provenance and conversion.** qemu's image cache trusts the first
  download of a cloud image and records its hash beside it (the hash is then
  checked on reuse and recorded in the environment); custom `--image` paths are
  hashed, not checked against anything. Firecracker inherits that. The
  conversion refuses images with a backing file or an unexpected format and
  passes `-f` explicitly, so a crafted qcow2 cannot flatten a host file (say
  `~/.ssh/id_rsa`) into the build's rootfs. The raw output is a cache entry with
  recorded provenance.

TEST-08 cases (SEC-01). "Ledger oracle" means a canary host the test controls
is contacted with a fresh token, and the case passes only if the attempt fails
or the token appears in the ledger.

| Boundary | Class | Attacker position | Case | Expected |
|---|---|---|---|---|
| build-env ↔ host | 1 | build VM | ARP for and connect to host addresses (LAN IP, docker0, `host(i)`) over the bridge, on a host with `rp_filter=0` and `arp_ignore=0` | no ARP reply, no connection |
| build-env ↔ host | 1 | build VM | IPv6 link-local to the host on the bridge | fails |
| build-env ↔ host | 1 | build VM | through the relay's proxy, reach `host(i)`, the host's LAN IP and another slot's relay | refused or no route |
| build-env ↔ host | 1 | build VM | write to ttyS0 expecting host input, and host stdin bytes reaching the guest | VMM stdin is `/dev/null`; nothing read |
| build-env ↔ host | 1 | build VM (VMM escape simulated) | list what the VMM process can open: work dir, other builds' dirs, lock dir, other slots' TAPs | matches the blast radius stated above (documents it; tightens when confinement lands) |
| build-env ↔ host | 1 | other local user | attach a VMM to a held slot's TAPs | EPERM or EBUSY |
| build-env ↔ host | 1 | operator | `kill -9` warden mid-build, then start a build on that slot | the next build kills the orphans and succeeds; doctor reported them in between |
| build-env ↔ host | 1 | custom image | qcow2 with a backing file pointing at a host path | conversion refused; no host bytes in the rootfs |
| build-env ↔ relay | 1 | other local user | connect to `relay(i)` `:8300`, `:80`, `:443`, `:53`, `:8301` on a running slot | all fail |
| build-env ↔ relay | 1 | warden user (another process) | connect to `relay(i)` `:8300`, `:80`, `:443`, `:53` | all fail; only `:8301` is reachable and it is already claimed |
| build-env ↔ relay | 1 | build VM | two concurrent builds on two slots | distinct CA fingerprints and signing keys; ledgers with no cross-entries |
| build-env ↔ relay | 1 | relay (compromised) | open a connection to the host over IPv4 or IPv6 on `wfc<i>u` | dropped |
| build-env ↔ relay | 2 | build VM | concurrent builds with `br_netfilter` loaded and identical 5-tuples in two slots | no cross-slot conntrack match; each ledger complete |
| build-env ↔ beyond | 1 | build VM | direct egress to the canary bypassing the proxy: TCP to a non-proxied port, UDP, raw socket, alternate DNS server, IPv6, ICMP | fails; ledger oracle |
| build-env ↔ beyond | 2 | relay (rules missing, simulated by booting a kernel built without `NETFILTER_XT_TARGET_MASQUERADE` in a test-only run) | build VM sends to the canary with source `10.0.0.2` | dropped by the host anti-spoof rule; ledger oracle |
| build-env ↔ beyond | 2 | relay (compromised) | send with a spoofed source out of `wfc<i>u` | dropped |
| build-env ↔ beyond | 2 | build VM | a build VM that resets or dies early | the build fails promptly; no partial ledger accepted as complete |
| build-env ↔ beyond | 2 | build VM | HTTPS to the canary through the proxy | recorded in the ledger (control case for the oracle) |

The "other local user" and "operator" positions are new to the suite, whose
matrix assumes a build-environment attacker; they map to the build-env ↔ host
and build-env ↔ relay Class 1 cells (reaching other builds, driving the control
plane, tampering ledger writes) and are noted for the security-suite item.

## Validation plan

- **Tier 0** (every host, CI): Firecracker config JSON generation (golden
  files, including stdin and entropy settings); the `exec.Cmd` attributes
  (stdin `/dev/null`, `Setpgid`, `Pdeathsig`); kernel command-line
  construction; slot naming and address arithmetic (slots 0, 63, 64 and 255,
  and `--slots` above 256 rejected); the setup and doctor command lists (a fake
  runner, no root) including the bridge table, sysctls, OUTPUT, anti-spoof,
  ip6tables and raw-zone rules, and the FORWARD-policy and firewalld refusals;
  resolver selection (loopback and host-local addresses excluded); the GPT
  reader on fixture images (root type GUID, single Linux partition, no match,
  truncated or malformed tables); the `qemu-img info` gate (backing file and
  unknown format refused); raw-cache key and provenance; `describeBuild` golden
  descriptors for both paths; the exit-ordering race with a fake process and a
  fake collector (completion just before, just after and never after the VMM
  exits); stale-VMM detection from a pid file; the pinned kernel config's
  required `=y` options; preflight error messages; the non-Linux stub; the
  moved `driver/linuxguest` and `driver/relayvm` tests, plus a test that qemu's
  seed and relay initrd are byte-identical before and after the move (TEST-04
  golden fixtures).
- **Tier 1** (CI integration): unchanged container suite, run because
  `cmd/warden` changes.
- **Tier 2, blocking: `linux-kvm`** (FLOW-09; the change is that driver's tree),
  after re-running `warden setup --driver firecracker`: `warden doctor`; the
  direct-boot build; an Ubuntu 24.04 cloud-image build with a COPY and an HTTPS
  fetch; exclusive trust; egress to an un-proxied address blocked (the relay's
  rules in effect); two concurrent builds on two slots with `br_netfilter`
  loaded; a build VM killed mid-build fails promptly; warden killed with
  `kill -9` mid-build, then the next build on that slot succeeds; another local
  user cannot reach a running relay; teardown leaves no work dir or rootfs copy
  and frees the slot; ledger verifies. Distro cases on the checklist: a
  firewalld host (setup refuses with the zone fix, then works), a host with a
  local resolver on a LAN address (preflight refuses), a host with FORWARD
  policy ACCEPT (setup refuses). Runs on `windows-desktop` under WSL2 if a8 is
  confirmed, and in the CI job once it exists.
- **Tier 2, tracked:** `qemu-any` (a Linux guest build after the extraction and
  the arch parameter), `windows-hyperv` and `macos-vz` (a build after the
  `cmd/warden` change); arm64 Firecracker (no host). On the native-gap backlog
  and in `tier2_gaps` (TEST-03).
- **TEST-08**: the release that ships this trips GLOBAL-06; the security suite
  runs the cases in the Security note's table.

## Implementation plan

1. **Phase 0 spike** (queue step `f2-spike-linux-kvm`, `host:linux-kvm`, in
   phase 2 so Implement cannot start before it; it needs a8 confirmed and
   `hosts.yaml` serving `linux-kvm`). On the host, on this branch:
   a. `tools/firecracker-vm/build-kernel.sh` and its required-options fragment
      (Decision 10), with the tier-0 config test;
   b. `tools/relay-vm/build-initramfs.sh`'s `ARCH` and no-modules parameters,
      aarch64 output unchanged (checked by hash);
   c. `tools/firecracker-vm/build-initramfs.sh` and its launcher (Decision 6);
   d. a throwaway `tools/firecracker-vm/spike.sh` that makes one slot by hand
      with Decision 4's rules and boots both VMs from hand-written configs.
   Then: claim the relay; run a direct-boot `FROM alpine` / `RUN echo hello`
   to a verified ledger; boot the Ubuntu 24.04 raw image (converted by hand)
   with the `CIDATA` seed on virtio-blk and `root=PARTUUID=…` to a verified
   ledger; check from the build VM that an un-proxied TCP connection and a UDP
   send to an outside address fail while an HTTPS fetch is recorded; check the
   relay console shows `crng init done` before the relay is ready; settle the
   relay memory default and the pinned Firecracker version. Record every
   finding in Rollback notes; a decision that fails returns the feature to
   Design (FLOW-06, RELAY-04, BOOT-01).
2. **Extract `driver/relayvm` and `driver/linuxguest`** from `driver/qemu`
   (Decision 13; `runcmd` tail as a parameter), with byte-identical seed and
   relay initrd tests; qemu behavior unchanged. In the same commit,
   `tools/tier1/rules.yaml`: `drivers.shared` gains `driver/linuxguest/` and
   `driver/relayvm/`; `bootstrap.surface` replaces `driver/qemu/cloudinit.go`
   with `driver/linuxguest/`; and, once the design review has accepted it, the
   matching BOOT-07 surface list in `bootstrap.md` (Proposed standards change)
   (ORCH-01, TEST-02, TEST-04, BOOT-07).
3. **`driver/firecracker` slots**: naming, address arithmetic, `TryHostLock`
   acquisition, pid files and stale-VMM reaping, the setup and doctor command
   lists (interfaces, sysctls, bridge table, `WARDEN-FC` with OUTPUT,
   anti-spoof, ip6tables and raw zones, posture refusals, resolver selection)
   behind a runner interface, with tier-0 tests (ORCH-04, GLOBAL-06, UX-04).
4. **`driver/firecracker` images**: the GPT reader with fixtures, the
   `qemu-img info` gate and `-f` conversion into
   `<cache>/firecracker/images`, `layers.json` as `prepared` with the cloud
   image as input, the per-build reflink copy under
   `<cache>/firecracker/work/<id>` (ORCH-04, BOOT-06).
5. **`driver/firecracker` VMs**: config JSON (entropy device on the relay),
   `--no-api` process start with stdin `/dev/null`, `Setpgid`, `Pdeathsig` and
   a locked OS thread, pid files, console handling, process watch, teardown on
   every error path (ORCH-04, BOOT-02, BOOT-03).
6. **`driver/firecracker` StartBuild**: session, slot, relay boot and claim,
   both delivery paths, build VM boot, wait with the exit-ordering grace,
   `describeBuild` per Decision 14 (BOOT-02, BOOT-04, RELAY-07, ORCH-02,
   GLOBAL-08).
7. **Non-Linux stub and build tags**; `make lint` for linux, windows and darwin
   (GLOBAL-07).
8. **`cmd/warden`**: every item in Decision 12 (driver validation and
   dispatch, flag and guest-OS validation, `--network` rejection,
   `--keep-vms`, `setup`, `doctor`, `clean` without a container runtime,
   `image prepare`, `list` and `verify` with `layerCaches`, `Exec`
   unsupported); `tools/tier1/rules.yaml` `drivers.all` gains `firecracker`
   and `bootstrap.surface` the `driver/firecracker` isolation, delivery and
   start files and `tools/firecracker-vm/` (ORCH-01, ORCH-03, ORCH-04, UX-07,
   BOOT-07).
9. **CI tier-2 job** (a8): a workflow on `ubuntu-latest` that grants `/dev/kvm`
   with the standard udev rule, builds the assets (cached by their pinned
   inputs), runs `warden setup` and the tier-2 checklist's builds; not required
   for merge until it has passed on this branch (TEST-02).
10. **Reference docs in the same change**: `guide/drivers.md`, `guide/cli.md`,
    `guide/configuration.md`, `concepts/architecture.md`,
    `concepts/network-isolation.md` (a Firecracker topological-isolation
    section with the bridge table and `WARDEN-FC` rules),
    `contributing/structure.md` (`driver/firecracker`, `driver/linuxguest`,
    `driver/relayvm`, `tools/firecracker-vm`),
    `getting-started/installation.md` (the `firecracker` binary, the `kvm`
    group, the asset scripts, the setup-at-boot unit); `overview.md` VM drivers
    and driver list; `bootstrap.md`'s mapping row once the design review has
    accepted it (UX-01, FLOW-15, BOOT-07, FLOW-14).
11. **Tier-0 tests** listed in the Validation plan; seeded fixtures, no KVM
    needed (TEST-02, TEST-04).
12. **Native validation** on `linux-kvm` (blocking) and the tracked gaps on the
    backlog (FLOW-09, TEST-03).
13. **Security-suite cases** from the Security note's table added to the corpus
    for the release (TEST-08, SEC-01).

Follow-ups for intake (workflow-intake; not this feature's work):

- Relay VM init fails closed: check every iptables command, enable forwarding
  only after the rules exist, print the rule set to the console, and accept
  only `8301` and ESTABLISHED on the upstream interface (every driver; a10's
  second option).
- `tools/build-vm`: stop baking `warden-io` and writing DNS in the init, or
  register the deviation (BOOT-02, BOOT-03, BOOT-06).
- Release packaging of the VM assets (kernel, relay and direct-boot
  initramfs), in `vm-driver-release-plan`.
- VMM confinement: jailer or unprivileged Landlock plus cgroup scope (a4).
- Link-local and metadata-address blocking across drivers (a7).
- Pinned hashes for the default cloud images instead of trust on first
  download (qemu and firecracker).

## Design review

- **r1, 2026-10-31: approved** (Jeff, attended session, FLOW-19).
  Approved at `2df5953`. a1 to a11 confirmed as recommended; confirming a2
  accepts the Bootstrap comparison (BOOT-07). Decisions taken at the review:
  - **a8, `linux-kvm` on `windows-desktop` under WSL2, is proposed and
    experimental.** It may need a real Linux root host. If the spike or the
    blocking validation is blocked by the host itself, the native session
    records the evidence and adds a human escalation step: only Jeff can
    provision or procure dedicated Linux KVM hardware, and that step surfaces
    in QUEUE.md's Needs Jeff and in /workflow-human. Recorded on the spike's
    queue step and in `hosts.yaml`.
  - **a9, x86_64 only:** confirmed for availability of validation, not because
    arm64 is expected to behave the same. arm64 Firecracker differs (PE
    `Image` kernel, GIC, CPU templates, the aarch64 relay initramfs, an arm64
    cloud image) and stays a tracked gap with no support claimed until an
    arm64 Linux KVM host exists.
  - **The Proposed standards change to `bootstrap.md`** moves to its own
    process item (FLOW-14) rather than this feature's diff.

## Consolidation

<!-- Filled in at Publish (FLOW-10, FLOW-15). -->

## Rollback notes

<!-- None yet. The Phase 0 spike records its findings here. -->

## Archive summary

<!-- Filled in at Publish (FLOW-12). -->

---
feature: build-link-subnet
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # every VM driver: the shared relay init (qemu, hyperv, firecracker), driver/linuxguest (qemu, firecracker), driver/winguest (qemu, hyperv), driver/vz, driver/firecracker, driver/relayvm. container is declared conservatively (FLOW-02): driver.BuildRequest gains the BuildLink field and cmd/warden refuses --build-link for it, but its code and behaviour are unchanged and it does not import driver/buildlink
seams: []                # the executor's `initialize --gateway --ip` contract, the claim, collector and guest API are unchanged; the relay VM's kernel command line gains keys, but it is the orchestrator's boot configuration, not a relay seam (question a7)
surfaces: [cli, config, output]  # [vm] build_link and --build-link (advanced, Decision 5); doctor's build-link lines; fixable stale-asset, mismatch and overlap refusals; firecracker's resolver refusal names the configured link (UX-01, UX-04, ORCH-03)
security_boundary: env-relay  # the link is the only path between the build environment and the relay; its posture is meant to be unchanged (Security note)
alters_relationships: false   # no component's role or interaction changes; BuildRequest gains an additive input like CPUs or Network (question a7)
design_review: required
native_validation:
  - {host: linux-kvm, blocking: true, reason: "reaches driver/firecracker (boot args carry the link, resolver exclusion follows the configured link), tools/firecracker-vm/init and the x86_64 relay init. CI tier 2 (firecracker.yml) covers a fresh hosted runner on the default link, now without its DNS workaround so the VPC resolver 10.0.0.2 is the test; the native run covers the upgrade path (pre-change assets refused, then rebuilt) and a non-default link with no rebuild"}
  - {host: qemu-any, blocking: true, reason: "reaches driver/qemu (relay and direct-boot command lines carry the link, asset record check), tools/build-vm/init, the aarch64 relay init and the driver/linuxguest seed on qemu: a direct-boot and a cloud-image Linux build on the default and on a non-default link, and the stale-asset refusal"}
  - {host: macos-vz, blocking: true, reason: "reaches driver/vz (network_unix.go, the warden-boot script baked at prepare from the configured link, the image record check) and relay/ host mode (subnet block): an image prepared before the change is refused; a re-prepared one builds; a changed build_link is refused until re-prepared with it; the host-mode relay refuses dials into the configured link"}
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv (relay-boot VHDX record checked against the configured link, NAT-prefix overlap refusal), refresh-cache.sh --link, the relay boot VHDX rebuilt from the new init, and driver/winguest on a Windows guest"}
  - {host: macos-qemu, blocking: false, reason: "Windows guest on qemu: driver/qemu's changes are guest-agnostic (the relay command line and the asset record check) and validated by qemu-any, and the Windows-specific piece (driver/winguest) by windows-hyperv. qemu Windows builds already fail on main (backlog qemu-windows-guest-package), so a blocking step could not pass for reasons outside this feature (question a5)"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows guest build on the default and a non-default link (backlog build-link-subnet-qemu-windows), after qemu-windows-guest-package is fixed"}
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, ORCH-01, ORCH-03, BOOT-03, BOOT-06, BOOT-07, EXEC-04, RELAY-04, RELAY-05, UX-01, UX-03, UX-04, UX-05, UX-06, UX-07, UX-08, TEST-01, TEST-02, TEST-03, TEST-08, FLOW-02, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-15, FLOW-19]
concepts: [build-link, configurable-build-link, advanced-config, two-vm-topology, network-isolation, address-plan, rfc6598, upstream-dns, resolver-selection, relay-vm-init, kernel-command-line, boot-assets, stale-assets, image-prepare, socketpair-topology, config-validation]
related: [firecracker-driver.md, unified-bootstrap.md, hyperv-windows-prepare.md, vm-driver-release-plan.md, platform-compatibility.md]
---

# Move every VM driver's relay-to-build link off 10.0.0.0/30

## Problem

Every VM driver joins the build VM to the relay over one private link with
fixed addresses: relay `10.0.0.1/30`, build VM `10.0.0.2/30`. Inside the relay
VM that link is a connected route, so any upstream address inside
`10.0.0.0/30` is sent to the build VM instead of upstream.

CI tier 2's first run of the Firecracker driver (PR #90, 2026-11-02) hit this.
The hosted runner's only resolver was its VPC resolver `10.0.0.2` (an AWS VPC
on `10.0.0.0/16` puts its resolver at base + 2), so the relay sent every DNS
query to the build VM. PR #90 added a stopgap: Firecracker's preflight and
doctor refuse a host resolver inside the link, and the CI job gives
systemd-resolved public upstreams. The clash is not specific to Firecracker or
to DNS:

- `10.0.0.0/16` and `10.0.0.0/24` are common VPC and LAN plans, and many home
  routers (Xfinity, for one) are `10.0.0.1` as both gateway and resolver.
- A build can fetch from a private mirror on any RFC 1918 address. On qemu
  and Hyper-V, where the relay's upstream is NAT through the host, a mirror at
  `10.0.0.1` or `10.0.0.2` is unreachable, and the build fails in a confusing
  way.
- The refusal makes the user reconfigure their host's DNS to work around an
  address BuildWarden chose.

This feature moves the link for every VM driver to a default range no common
upstream uses, lets the user move it again when their network collides with
the default anyway (design review r1), defines the default once, and keeps
assets that bake a link from failing silently.

## Prior art

- [`firecracker-driver.md`](firecracker-driver.md), Decision 4 (slots: the
  upstream `/30`s in `100.64.88.0/22`, beside the container driver's
  `100.64.87.0/24`; every slot reuses the one build link, kept apart by a
  conntrack zone per bridge) and Decision 5 (resolver selection: loopback,
  host-local and, since PR #90, build-link resolvers are excluded; none left
  is a fixable refusal). Its Rollback notes ("CI tier 2") queued this feature.
  Reused: the RFC 6598 address plan, the resolver exclusion (retargeted at the
  configured link, not removed) and its fix text. The relay's command-line
  keys (`warden.net`, `warden.nat_*`, `warden.dns`, `warden.claim_token`) are
  the per-build channel this design extends. Decision 4's conntrack zones
  still matter, since every slot keeps reusing the same two addresses.
- [`unified-bootstrap.md`](unified-bootstrap.md) and
  [`bootstrap.md`](../development/bootstrap.md): the executor gets the relay's
  address from `initialize --gateway` (and `--ip` where the guest has no
  cloud-init). That contract (EXEC-04, the `executor-initialize` seam) is
  unchanged; only the value changes. BOOT-03 allows static addressing applied
  by the platform's own boot configuration in the launcher, which covers a
  direct-boot init reading its address from the kernel command line. The
  per-driver mapping and the deviation register are unchanged. vz's D2
  (executor and boot script baked at prepare) is why a vz image must be
  re-prepared once, and again after a change of the setting (Decision 3).
  BOOT-06 is why the Windows and Linux guests need no re-prepare: their
  address comes from the per-build seed.
- [`hyperv-windows-prepare.md`](hyperv-windows-prepare.md): `warden-run.ps1`
  goes on the per-build `WARDEN` seed, and the prepared image carries only the
  startup task, so the Windows guest's address moves with no image change.
- `driver/subnet.go`: the container driver's `100.64.87.0/24` (RFC 6598,
  "unlikely to collide with user networks"). This design follows the same
  reasoning for the VM link's default.
- `cmd/warden/config.go` and [`configuration.md`](../book/src/guide/configuration.md):
  the `[vm]` table holds the VM-driver defaults whose flag always wins
  (`cpus`, `memory_mb`, `network`), and `[vm] network` / `--network` is the
  precedent for a key with a matching flag on `build`, `shell`, `doctor` and
  `setup`. `warden setup --nat-prefix` / `--host-ip` are the precedent for an
  address flag's naming and help text. The setting follows both (Decision 5).
- GLOBAL-08: the address is orchestrator topology, not part of a versioned
  relay or executor seam. RELAY-05: no ledger, schema or environment-record
  format names the address (checked below), so no spec version changes.

## Decisions

1. **The default link is `100.127.255.252/30`: relay `100.127.255.253`, build
   VM `100.127.255.254`** (question a1, confirmed in r1). This is the last
   `/30` of RFC 6598 shared address space (`100.64.0.0/10`).

   What decides the choice is which upstream addresses the relay must still
   reach: host resolvers (Firecracker passes them on the kernel command line),
   the collector and claim endpoints (Firecracker's `100.64.88.0/22`, Hyper-V's
   NAT `192.168.240.0/20`, qemu's user-mode `10.0.2.0/24`), and whatever a
   build fetches, private mirrors included. How each host OS treats the range
   matters only inside the guests and the relay VM, because no host holds an
   address on the link. vz's link is a socketpair into the host relay's gVisor
   netstack. qemu uses a socket netdev between the two VMs. Hyper-V uses a
   private switch with no host vNIC. Firecracker uses a bridge with no
   address.

   | Candidate | Upstream clash | Guest OS behaviour | Verdict |
   |---|---|---|---|
   | `10.0.0.0/30` (today) | Common: AWS VPCs on `10.0.0.0/16` (resolver `10.0.0.2`), home routers at `10.0.0.1` | Fine everywhere | Rejected: the bug |
   | Another RFC 1918 `/30` (e.g. `172.31.255.252/30`, `192.168.255.252/30`) | Every RFC 1918 block is someone's LAN, VPC or VPN (AWS default VPC is `172.31.0.0/16`) | Fine | Rejected: moves the clash, doesn't remove it |
   | **RFC 6598, `100.127.255.252/30`** | Reserved for carrier NAT between the CPE and the carrier, so not a LAN, VPC or resolver range. Known uses sit elsewhere: Tailscale MagicDNS `100.100.100.100`, Alibaba metadata `100.100.100.200`, ChromeOS `100.115.92.0/23`, EKS custom networking `100.64.0.0/16`, BuildWarden's own `100.64.87.0/24` and `100.64.88.0/22`. Tailscale node addresses are spread over the whole `/10`, so a tailnet peer at one of these two addresses is possible but about one in a million per peer | Ordinary unicast on Linux, Windows and macOS; Go's `IsPrivate` is false, so nothing in the relay special-cases it | **Chosen as the default** |
   | RFC 6598 next to BuildWarden's block (e.g. `100.64.86.0/30`) | As above, but inside `100.64.0.0/16`, which EKS custom networking and low-first CGNAT pools use | Fine | Rejected: the top of the `/10` is the least allocated |
   | Link-local `169.254.x.0/30` (away from `169.254.169.0/24`) | No routed address can be link-local, but on-link cloud services are, and the relay must reach some of them through host NAT: AWS DNS `169.254.169.253`, Kubernetes NodeLocal DNSCache `169.254.20.10`, ECS/EKS agents `169.254.170.x` | macOS configd treats a 169.254 IPv4 address as self-assigned and may not make the service primary. With only a link-local address, macOS and Windows may ARP for every destination instead of using the router (RFC 3927 §2.6.2). Windows labels it an autoconfiguration address and the network unidentified. The host-mode relay's SSRF filter already blocks link-local destinations | Rejected, and refused as a configured value (Decision 5) |
   | Benchmark `198.18.0.0/15`, TEST-NETs, class E `240.0.0.0/4` | Clash/Surge-style fake-IP DNS hands out `198.18.x`. TEST-NETs appear in labs and documentation | Windows and macOS refuse to assign class E | Rejected as the default; class E refused as a configured value |
   | Keep `10.0.0.0/30` and policy-route the relay's own sockets (marked dials take an upstream-only table) | None: overlap stops mattering | Unchanged | Rejected for now: a relay change in every dialer (proxy, DNS forward, collector), RELAY-04 parity across three modes, and nothing to gain on vz. Kept as the follow-up if configurability proves not enough |

   No range is guaranteed free, so the guards stay (Decision 4) and the link
   is configurable (Decision 5).

2. **One default, `driver/buildlink/link.env`; a configured link reaches each
   consumer per build where the boot path has a per-build channel, and is
   baked only where it has none** (question a2). The file has four
   `KEY=value` lines (`WARDEN_LINK_NET`, `WARDEN_LINK_PREFIX`,
   `WARDEN_LINK_RELAY`, `WARDEN_LINK_GUEST`; r1's text said three) and is the
   only place the default addresses are written. A configured value is only
   the network (`100.126.0.0/30`, say): the relay is always the network's
   first host and the build VM its second, so the two can never disagree.

   - Go: package `driver/buildlink` (a leaf: `embed`, `net/netip`; nothing
     from `driver`) embeds the file and exposes a `Link` value (`Net`,
     `Relay`, `Guest`, `GuestCIDR()`, `Mask()`), `Default()`, `Parse(cidr)`
     (Decision 5's checks), `Link.Env()` (the four-key rendering, also the
     record format), `Link.BootArgs()` and `ReadRecord(path)`. The default is
     validated at init. `driver.BuildRequest` gains `BuildLink buildlink.Link`;
     the zero value means the default, as `CPUs: 0` means the driver's
     default. `cmd/warden` resolves it once (Decision 5) and passes it to
     builds, `doctor`, `setup` and `image prepare`.
   - Per build, with no rebuild for a new value:

     | Consumer | Channel |
     |---|---|
     | relay init on qemu and Firecracker | kernel command line: `warden.link_relay=`, `warden.link_guest=`, `warden.link_prefix=` beside the existing `warden.*` keys |
     | build-VM direct-boot init (qemu `tools/build-vm`, `tools/firecracker-vm`) | the same three keys on its own command line |
     | Linux cloud-init seed (`driver/linuxguest`, qemu and Firecracker disk path) | rendered per build, as today |
     | Windows `WARDEN` seed (`driver/winguest`, qemu and Hyper-V) | `warden-run.ps1` rendered per build, as today |
     | vz host relay and its netstack | `--subnet` and `network_unix.go`, per build, as today |

   - Baked, because the boot path has no per-build channel:

     | Consumer | Why there is no channel | Baked by |
     |---|---|---|
     | Hyper-V relay boot VHDX | The UKI's `.cmdline` is fixed at build (`relayhost.go`: "the relay VM's command line is baked into its boot disk"), the relay VM has no data disk and its init mounts nothing | `tools/relay-vm/hyperv/refresh-cache.sh --link <cidr>` (default: `link.env`), which renders `/etc/warden/link.env` into the initramfs through a small `go run` helper over `buildlink`, so the shell never parses or derives addresses |
     | vz prepared image (`warden-boot`) | D2: the boot script is baked at prepare, and the guest has no shared directory at runtime | `warden image prepare --driver vz`, rendering from the resolved link |

   - Shell: each `build-initramfs.sh` copies `link.env` (the default) to
     `/etc/warden/link.env` in the image. The relay and direct-boot inits
     source it, then take any `warden.link_*` keys from `/proc/cmdline` over
     it. The inits only accept dotted-quad addresses and prefix `30`, and
     never evaluate the command line, as with today's keys. A value that
     fails the check is logged on the console and the init keeps the sourced
     default, so a mismatch shows up as a clear console line and not a
     silent timeout. The qemu and Firecracker drivers always pass the keys,
     so the baked copy only matters on Hyper-V and when an init is booted by
     hand.
   - C (`cmd/warden-vmnet`): it has no callers. `tools/vz-image-prep` uses its
     own `vz-netfwd` (`netfwd.go`), and nothing in the tree runs
     `warden-vmnet` (`structure.md`: "for future use"). `--subnet` becomes
     required, so the C file holds no address, and no Go caller needs to pass
     one.
   - `tools/vz-image-prep` (`//go:build ignore`, run with `go run` in the
     module) takes its prep-time link from `buildlink.Default()`, and
     `netfwd`'s default `--subnet` becomes it. Prep-time networking is a
     host-local forwarder used once per image. It does not follow the
     setting: `netfwd` already takes `--subnet` for a host that needs another
     value.
   - A tier-0 test fails if a default address, or the old `10.0.0.1`,
     `10.0.0.2` or `10.0.0.0/30`, appears anywhere in the tree outside an
     allowlist kept in the test: `driver/buildlink/`, the new default in
     reference docs (`docs/book/**`, `tools/*/README.md`), the old link only
     under `docs/design/`, and the named refusal and parsing tests. Matches
     are on whole addresses, so `10.0.0.20` or `110.0.0.1` do not trip it.
     (r1's text allowed only `docs/design/`, which contradicted UX-01.)

   *Rejected:* baking the link into every initramfs and rebuilding per value
   (needs a Linux toolchain for a setting change, when qemu and Firecracker
   already pass a per-build command line). Delivering the link in the relay
   claim (`relaycfg`), with the relay binary addressing its build interface
   after the claim: one channel for every VM relay, but a versioned-seam
   change (GLOBAL-08), and it moves interface setup from the init to the
   relay, which is a responsibility change (FLOW-02). Neither the vz guest
   nor the Windows guest would benefit, since they take their address from
   their own seed or image. Kept as the follow-up if Hyper-V's rebake proves
   a burden. A per-build seed disk for the Hyper-V relay VM (its init mounts
   nothing today, and this adds a filesystem module and a mount to the trust
   boundary). Hyper-V KVP (a guest daemon in the relay VM for one value). A Go
   constant plus a drift test over the shell copies.

3. **Every link-carrying boot asset carries a link record, checked before
   anything boots** (question a3; UX-04, BOOT-06). The record is the asset's
   baked `link.env`, written beside it by the tool that builds it. What the
   check compares depends on the asset's channel:

   | Asset | Driver | Record | Check | Fix named by the refusal |
   |---|---|---|---|---|
   | relay initramfs (aarch64, x86_64) | qemu, firecracker | `link.env` beside the output (source tree), or the release file name plus `.link.env` (e.g. `relay-vm-initramfs.cpio.gz.link.env`) | **present**: the asset understands `warden.link_*`; its value is not compared, since the driver passes the link | rebuild with `tools/relay-vm/build-initramfs.sh` (or upgrade the release assets) |
   | build-VM direct-boot initramfs | qemu (`tools/build-vm`), firecracker (`tools/firecracker-vm`) | same | present | that tool's `build-initramfs.sh` |
   | relay boot VHDX (UKI) | hyperv | `relay-boot.link.env` beside `relay-boot.vhdx` in the hyperv cache (kept paired with `.prev`), or `<name>.link.env` beside a `WARDEN_RELAY_BOOT_VHDX` override | **equal to the resolved link** | `tools/relay-vm/hyperv/refresh-cache.sh --link <the resolved link>` |
   | prepared macOS image (`warden-boot`) | vz | `link.env` in the prepared image directory, written by both prepare paths | **equal to the resolved link** | `warden image prepare --driver vz`, plus `--build-link <cidr>` when the link came from a flag |

   A missing record means an asset built before this change: it would boot on
   `10.0.0.0/30` and surface as `warden-io` timing out on `relay not ready`.
   A different record on a baked asset means the setting changed since the
   asset was built. Either is a `driver.Fixable` naming the command, raised
   before any VM boots, and `warden doctor` reports the same per driver. The
   relay and build assets are checked together, so a build never pairs a new
   init with an old one. `refresh-cache.sh`'s source stamp gains
   `driver/buildlink` and the link value, so `--check` reports a disk built
   for another link as stale. No release workflow ships the initramfs today,
   so `release.yml` does not change.

   *Rejected:* comparing every record's value with the resolved link (forces
   a qemu or Firecracker rebuild for a value the command line already
   carries). Documenting the rebuild only (a silent timeout for everyone who
   upgrades). A transition window where the relay listens on both addresses.
   Caching one VHDX or vz image per link value (a setting meant for the rare
   collision does not need a cache).

4. **Guards stay, and follow the resolved link** (question a4).
   - Firecracker's resolver exclusion (PR #90) checks the resolved link
     instead of a constant. Its refusal (no resolver left) names both fixes:
     give the host a resolver outside the link, or set `[vm] build_link` to a
     `/30` that holds none of them. Firecracker is still the only driver whose
     relay is handed host resolvers. qemu's relay uses user-mode DNS
     `10.0.2.3`, Hyper-V's is baked at refresh, and vz resolves on the host.
   - `firecracker.yml` drops its "Off-host DNS for the relay" step, so the
     hosted runner's own VPC resolver `10.0.0.2` is used. That turns the
     original failure into the CI test for this fix. The job's asset cache
     key gains `driver/buildlink/link.env` (an input of every cached
     initramfs), and its `pull_request.paths` gains `driver/buildlink/**`, so
     a change to the link runs tier 2 (TEST-02's conservative scoping). r1's
     text also added `tools/build-vm/init` to the key; it is dropped, since
     the job never builds that tool.
   - Hyper-V setup refuses a `--nat-prefix` that overlaps the resolved link,
     before changing anything (UX-04), as the relay would route that NAT into
     the build VM.
   - The host-mode relay (vz) refuses upstream dials into its whole
     `--subnet`, not only the gateway (question a8). Before, the guest address
     `10.0.0.2` was covered by the SSRF filter's RFC 1918 block. An RFC 6598
     address is not, and on a host with Tailscale it could be a real peer.
     This changes `relay/`, not only `cmd/relay`: the filter is
     `Relay.isBlockedIP` in `relay/proxy.go`, fed by `relay.Config`
     (`relay/relay.go`), which gains a blocked prefix. The `relay` package
     does not import `buildlink`; `cmd/relay/mode_host.go` passes the parsed
     `--subnet`, so a configured link is blocked with no extra wiring.
   - Every VM driver refuses, at build start and in doctor, a resolved link
     that holds an address of a host interface (Decision 5). No host ever
     holds a link address by design, so one there is a real collision.

5. **The link is configurable as an advanced option: `[vm] build_link` in
   `warden.toml`, and `--build-link` on the commands it affects, defaulting
   to `100.127.255.252/30`** (questions a6, confirmed reversed in r1; a9;
   a10; ORCH-03, UX-07).
   - **Where.** `[vm] build_link = "100.127.255.252/30"`, beside the other
     VM-driver defaults, in `~/.config/warden/config.toml` or `./warden.toml`
     (project over user), and `--build-link <cidr>` over both (UX-07,
     ORCH-03). The flag is registered on `build`, `shell`, `doctor`, `setup`
     and `image prepare`, as `--network` is on the first four. `build` and
     `shell` refuse it for `--driver container`, like the other VM flags;
     `image prepare` takes it only with `--driver vz` and `setup` only with
     `--driver hyperv`, refusing it elsewhere as `--network` is refused
     (UX-05). Long form only (UX-06). No environment variable: UX-07 keeps
     driver knobs to flags and keys. `configuration.md` lists the key in the
     Full Reference marked advanced, and gains an **Advanced options**
     section explaining when to change it, what to re-run on vz and Hyper-V,
     and the checks below. `cli.md` lists the flag under each command.
   - **Validation at load and startup** (UX-08). `buildlink.Parse` accepts an
     IPv4 CIDR with prefix exactly `/30` whose address is the network address
     (`100.126.0.1/30` is refused with "did you mean 100.126.0.0/30"). It
     refuses `0.0.0.0/8`, loopback `127.0.0.0/8`, link-local
     `169.254.0.0/16`, multicast `224.0.0.0/4` and class E `240.0.0.0/4`.
     `cmd/warden` then checks the link is disjoint from every range
     BuildWarden itself uses: the container base `100.64.87.0/24`,
     Firecracker's upstream `100.64.88.0/22`, Hyper-V's default NAT
     `192.168.240.0/20` and qemu's user-mode `10.0.2.0/24`, read from those
     packages' exported constants. `buildlink` takes the list as an argument
     and imports none of them. A config value is checked when the file loads
     and a flag when it is resolved, both before any driver work. The error
     is fixable and names the key or flag and a valid example.
   - **Upstream overlap at build start and in doctor** (UX-04: a few
     `netip` comparisons, negligible on a successful run). Each VM driver
     refuses a link that holds an address of any host interface
     (`net.InterfaceAddrs`). Firecracker also checks its host resolvers
     (Decision 4), and Hyper-V checks the NAT prefix at setup. A host network
     that merely contains the link (a `/10` CGNAT WAN, say) is a doctor
     warning, not a refusal: the link is shadowed only for destinations
     inside its four addresses. Destinations a build will fetch cannot be
     known in advance. That residual case is what the setting is for, and
     the docs say so.
   - **Doctor.** Every VM driver's doctor prints the resolved link and where
     it came from (`default`, `warden.toml`, `config.toml` or `--build-link`),
     the host-overlap result, and each asset record's state (Decision 3).
     `warden doctor`'s help text in `cmd/warden/host.go` says so.
   - **No record of the value in the ledger or environment record.** The link
     is orchestrator topology (GLOBAL-08), and the build's view of it is its
     own address. The vz `warden-boot` script's layer digest still differs
     with the link, as any baked change does.

   *Rejected:* a fixed link (r1). A new `[advanced]` table (the `[vm]` table
   already holds the VM-driver knobs, and the docs can mark the key advanced
   without a second table for one key). Key only, no flag (UX-07 wants every
   level). An environment variable (UX-07). Accepting any prefix length (the
   relay and guest need exactly two hosts, and a wider link only widens what
   it can shadow).

## Questions

### Blocking

None. Every open question below has a recommended answer the design proceeds
on.

### Assumed

- **a1. The default range.** Confirmed in r1: `100.127.255.252/30` (relay
  `.253`, build VM `.254`), with the link configurable (a6).
- **a2. Where the default is defined and how a configured link reaches each
  consumer.** Proceeding with one embedded `driver/buildlink/link.env` for
  the default. Per build: `warden.link_*` kernel command-line keys for the
  qemu and Firecracker relay and direct-boot inits, and the seeds and vz
  `--subnet` as today. Baked: the Hyper-V relay VHDX (`refresh-cache.sh
  --link`) and the vz image (`warden image prepare`). Plus the tier-0
  stray-literal test; confirm. Options: (a) as stated; (b) bake everywhere
  and rebuild every initramfs per value; (c) deliver the link in the relay
  claim and let the relay address its build interface (a `relay-claim` seam
  change, and the relay takes over interface setup from the init; it would
  remove Hyper-V's rebake). Recommended: (a).
- **a3. Link records and what they are checked against.** Proceeding with a
  record beside every link-carrying asset, checked before boot and in doctor:
  present for per-build-channel assets (qemu and Firecracker initramfs), and
  equal to the resolved link for baked assets (Hyper-V VHDX, vz image), each
  refusal naming its rebuild, refresh or re-prepare command with the link;
  confirm. Options: (a) as stated; (b) compare every record with the
  resolved link (a rebuild per value even where the command line carries
  it); (c) document the rebuild only. Recommended: (a). Users upgrading
  rebuild their relay and build initramfs once, re-run `refresh-cache.sh`,
  and re-prepare vz images once. After a change of the setting, only
  Hyper-V and vz need a step.
- **a4. Guards.** Proceeding with Firecracker's resolver exclusion following
  the resolved link (its fix names both remedies), dropping the CI DNS
  workaround so CI exercises the VPC-resolver case, the Hyper-V
  `--nat-prefix` overlap refusal and the host-interface overlap refusal on
  every VM driver; confirm. Options: (a) as stated; (b) remove the
  Firecracker exclusion, now that the link can be moved; (c) keep the CI
  workaround too. Recommended: (a).
- **a5. macos-qemu tracked, not blocking.** Proceeding with macos-qemu
  (Windows guest on qemu) as a tracked gap: driver/qemu's changes are
  guest-agnostic and validated on qemu-any, `driver/winguest` on
  windows-hyperv, and qemu Windows builds already fail on main (backlog
  `qemu-windows-guest-package`); confirm. Options: (a) tracked; (b)
  blocking, with the criterion "gets no further than on main"; (c) blocking,
  waiting on the qemu Windows fix. Recommended: (a).
- **a6. A configuration knob.** Confirmed reversed in r1: the link is
  configurable in the advanced options, defaulting to `100.127.255.252/30`.
  Where and how is a9; validation is a10.
- **a7. No seam, no relationship change.** Proceeding with `seams: []` and
  `alters_relationships: false`. `initialize --gateway/--ip` keeps its
  contract (EXEC-04). The claim, collector and guest API don't change. The
  relay VM's command line gains three keys, but it is the orchestrator's boot
  configuration of its own relay VM, not a relay seam a third party consumes
  (GLOBAL-08 lists the claim, collector, control plane and guest protocols).
  `driver.BuildRequest` gains an additive input, as `CPUs` and `Network`
  did, and the Driver and Session interfaces don't change. No ledger, schema
  or environment-record format names the address (RELAY-05). No BOOT
  invariant, mapping row or deviation changes (BOOT-07). Baking the
  configured link into a vz image is the existing D2 bake, not a new
  deviation; confirm. Options: (a) as stated; (b) conservatively declare
  `executor-initialize` and `alters_relationships: true` because every
  driver's guest address moves and the driver contract's input grows.
  Recommended: (a).
- **a8. Host-mode relay blocks its own subnet.** Proceeding with `relay/`
  host mode blocking upstream dials into the whole `--subnet` (the resolved
  link), not only the gateway. This changes `relay/proxy.go` and
  `relay/relay.go` (a blocked prefix in `relay.Config`) as well as
  `cmd/relay/mode_host.go`. Container and VM modes leave it unset; confirm.
  Options: (a) block the subnet; (b) leave the relay alone. Recommended: (a).
- **a9. Where the setting lives.** Proceeding with `[vm] build_link` in
  `warden.toml` / `config.toml` and `--build-link` on `build`, `shell`,
  `doctor`, `setup` (hyperv) and `image prepare` (vz), flag over project
  over user, long form only, no environment variable, documented as an
  advanced option in `configuration.md` and per command in `cli.md`; confirm.
  Options: (a) as stated; (b) the key only, no flag; (c) a new `[advanced]`
  table; (d) (a) plus a `WARDEN_BUILD_LINK` environment variable.
  Recommended: (a).
- **a10. Validating the setting.** Proceeding with checks at load and
  startup (IPv4, exactly `/30`, network-aligned; not `0/8`, loopback,
  link-local, multicast or class E; disjoint from the container base,
  Firecracker's upstream, Hyper-V's default NAT and qemu's user-mode range),
  and at build start and in doctor (no host interface address inside the
  link; Firecracker resolvers; Hyper-V NAT prefix at setup). A host network
  that only contains the link is a doctor warning; confirm. Options: (a) as
  stated; (b) syntax and prefix only; (c) (a), and also refuse when a host
  network contains the link. Recommended: (a).

## Design

### The definition

```
driver/buildlink/
  link.env      WARDEN_LINK_NET=100.127.255.252/30
                WARDEN_LINK_PREFIX=30
                WARDEN_LINK_RELAY=100.127.255.253
                WARDEN_LINK_GUEST=100.127.255.254
  buildlink.go  //go:embed link.env
                type Link {Net netip.Prefix; Relay, Guest netip.Addr}
                Default() Link                 the embedded file, validated at init
                Parse(cidr) (Link, error)      Decision 5's syntactic checks
                (Link) Disjoint(named []Range) error
                (Link) HostOverlap(addrs []netip.Prefix) (refuse, warn []netip.Prefix)
                (Link) GuestCIDR(), Mask(), Env() []byte, BootArgs() string
                ReadRecord(path) (Link, error)  missing -> ErrNoRecord
tools/relay-vm/linkenv.go   //go:build ignore; `go run ... <cidr>` prints Parse(cidr).Env()
```

`buildlink` imports nothing from `driver`, so each caller wraps its errors in
`driver.Fixable` with its own fix text, because only the caller knows the key,
flag or rebuild command. `HostOverlap` takes the addresses as an argument, so
its tests need no host network.

### Resolving the link (`cmd/warden`)

```
resolveBuildLink(cfg, cmd):
    s := flag --build-link if set, else cfg.VM.BuildLink, else ""
    if s == "": return buildlink.Default(), source "default"
    link, err := buildlink.Parse(s)            -> Fixable naming the key or flag
    err = link.Disjoint(container base, firecracker upstream,
                        hyperv DefaultNATPrefix, qemu user-mode)
    return link, source
```

`LoadConfig` runs the same parse and checks on `[vm] build_link` as the file
loads, so a bad value fails before any command does work (UX-08). With no key
and no flag, the default resolves with no config file (UX-07).

### Per driver

```
qemu / firecracker StartBuild (after asset Find):
    for each initramfs: ReadRecord(beside it)  missing -> Fixable(rebuild cmd)
    link.HostOverlap(host addrs)               refuse -> Fixable(set build_link)
    firecracker: resolvers minus those in link; none left -> Fixable(both fixes)
    relay cmdline += link.BootArgs(); direct-boot build cmdline += link.BootArgs()
    linuxguest seed rendered from link
hyperv StartBuild:
    ReadRecord(relay-boot.link.env) != link   -> Fixable(refresh-cache.sh --link L)
    link.HostOverlap(host addrs)
    winguest seed rendered from link
hyperv setup:  NAT prefix overlaps link       -> Fixable(another --nat-prefix)
vz StartBuild:
    ReadRecord(<image>/link.env) != link      -> Fixable(warden image prepare --driver vz [--build-link L])
    link.HostOverlap(host addrs)
    netstack relay/build/gateway from link; relay --subnet=link.Net
vz image prepare (both paths): warden-boot rendered from link; write <image>/link.env
doctor (each VM driver): "build link ... <net> (<source>)", overlap result,
    each record's state; a host network containing the link is a warning
```

### Every site that changes

| Site | Today | After |
|---|---|---|
| `driver/buildlink/` (new) | — | `link.env`, `buildlink.go`, tests |
| `driver/driver.go` `BuildRequest` | — | `BuildLink buildlink.Link` (zero = default) |
| `cmd/warden/config.go`, `main.go`, `host.go`, `image.go` | — | `[vm] build_link`, `--build-link` on build, shell, doctor, setup, image prepare; `resolveBuildLink`; `validateFlagsForDriver` refuses it for container; doctor help text |
| `tools/relay-vm/init` (qemu, hyperv, firecracker) | `ip addr add 10.0.0.1/30` on eth0, header comment | sources `/etc/warden/link.env`, takes `warden.link_*` from the command line over it; header lists the keys |
| `tools/relay-vm/build-initramfs.sh` | — | copies `link.env` into the image and writes the output record |
| `tools/relay-vm/linkenv.go` (new) | — | renders `link.env` for a CIDR through `buildlink` |
| `tools/relay-vm/hyperv/` (`build.sh`, `build-boot-vhdx*.{sh,ps1}`, `refresh-cache.sh`) | bakes the init | `refresh-cache.sh --link <cidr>`; `build.sh` bakes the rendered `link.env`; the record travels with the VHDX as `relay-boot.link.env` (and `.prev`); the source stamp adds `driver/buildlink` and the link |
| `tools/build-vm/init` + `build-initramfs.sh` (qemu direct boot) | `10.0.0.2/30`, `via 10.0.0.1`, `nameserver 10.0.0.1`, `--gateway=10.0.0.1` | sourced default, command line over it; record written |
| `tools/firecracker-vm/init` + `build-initramfs.sh` | `10.0.0.2/30`, `via 10.0.0.1`, `--gateway=10.0.0.1` | same |
| `driver/qemu/vm.go` | relay `-append` without link; direct-boot `-append` console only | both append `BootArgs()` |
| `driver/firecracker/vm.go` | `relayArgs`, `buildConfig` boot args | both append `BootArgs()` |
| `driver/linuxguest/seed.go` + `testdata/seed/{network-config,user-data}` | literal addresses in network-config v2 and `runcmd` | rendered from the request's link; goldens for the default |
| `driver/winguest/winguest.go` (`BuildGatewayIP`, `BuildGuestCIDR`), `run.go` | string constants | derived from the request's link (default kept under the old names for callers) |
| `driver/vz/network_unix.go` | `net.IPv4(10,0,0,1)` etc. | from the request's link |
| `driver/vz/image.go` (`warden-boot` `GW`/`SELF`/`MASK`, comment), `ReprepareImage` | literals baked at prepare | rendered from the resolved link at prepare; both prepare paths write the record |
| `driver/vz` StartBuild, `Preflight` | — | record equality, host overlap |
| `driver/relayvm/assets.go` (`QEMURelay`, `Firecracker`), `driver/qemu` `resolveBuildAssets` | kernel + initramfs | plus each initramfs's record, from the same location `Find` chose |
| `driver/firecracker/posture.go` (`buildLink`), preflight and doctor fix text, `slots.go` comment | `10.0.0.0/30` | the request's link; fix text names both remedies |
| `driver/hyperv` (`buildassets.go`, `setup.go`, `doctor.go`, `buildhost.go` comment) | — | VHDX record equality; `--nat-prefix` overlap refusal; doctor lines |
| `relay/proxy.go`, `relay/relay.go` | blocks the gateway only | a blocked prefix in `relay.Config` that `isBlockedIP` checks (a8) |
| `cmd/relay/mode_host.go` | passes the gateway | passes the parsed `--subnet` as the blocked prefix |
| `cmd/relay/main.go` `--subnet` default `10.100.0.0/30` | not the build link (vz always passes it) | unchanged; the comment says vz passes the resolved link |
| `cmd/warden-vmnet/main.c` | default `--subnet=10.0.0.0/30` | `--subnet` required (no callers) |
| `tools/vz-image-prep/{personalize,uv-build-test,netfwd}.go` | literals for the prep-time link | `buildlink.Default()` |
| `cmd/warden-io` tests and `platform_windows.go` comments | `10.0.0.2/30` as parsing examples | other example addresses; behaviour unchanged |
| `.github/workflows/firecracker.yml` | DNS workaround step; cache key; paths | step removed; key adds `driver/buildlink/link.env`; paths add `driver/buildlink/**` |
| Reference docs (UX-01): `guide/configuration.md` (Full Reference key, Advanced options section), `guide/cli.md` (`--build-link` per command; doctor's per-driver link and record checks; `setup --nat-prefix` overlap refusal), `guide/drivers.md` (Upstream DNS; upgrade note per VM driver; changing the link on vz and Hyper-V), `concepts/network-isolation.md` (slot table, host-mode SSRF bullet), `contributing/structure.md` ("fixed 10.0.0.0/30"), `tools/win-image-prep/README.md`, and `cmd/warden/host.go`'s doctor help | old link, fixed | default link, the setting, the checks and the rebuild notes |
| `concepts/overview.md` pseudocode (FLOW-15) | — | each VM StartBuild resolves the link, checks records and host overlap before booting; the relay block notes host mode refuses dials into its subnet |

Unchanged, and why: `driver/subnet.go` (container, `100.64.87.0/24`);
Firecracker's upstream `100.64.88.0/22`; Hyper-V's NAT default
`192.168.240.0/20`; qemu's user-mode `10.0.2.0/24` (all must stay disjoint
from the link, which the load-time check enforces); `relay/` except a8's
blocked prefix (VM-mode relays bind by interface, and REDIRECT and FORWARD
DROP match on interfaces); `relaycfg/`, `specs/`, `ledger/` (no address in any
format; see below); Firecracker's slots, TAPs and host rules (none matches a
link address).

### Compatibility (RELAY-05, UX-03)

No ledger entry, metadata schema, environment record or spec names the link's
address. The relay's DNS answers for its reserved names (`artifacts`, `cwd`)
carry its own address, but they go only to the guest and are not recorded.
Environment records list layer digests (initramfs, the vz `warden-boot`
script). Those digests change value because the files change, which is what
the records exist to show. No format version changes. Old ledgers verify as
before.

The key and flag are additive (UX-03). The upgrade cost is the baked-asset
rebuild in Decision 3, which the refusals name.

## Cross-driver impact

- **qemu.** The relay and direct-boot command lines carry the link, so a
  changed setting needs no rebuild. Linux guests: the per-build cloud-init
  seed follows the link. The aarch64 relay initramfs and the direct-boot
  build initramfs are rebuilt once to gain the link-aware init and its
  record. Windows guests: the per-build `warden-run.ps1` follows the link,
  and the prepared image is unchanged (BOOT-06).
- **firecracker.** As qemu for the relay and direct-boot initramfs, and the
  disk path's seed moves through `driver/linuxguest`. The resolver exclusion
  follows the resolved link. Slots, TAPs and host rules don't change, since no
  host rule matches the link's address. Every slot still reuses the same two
  addresses, kept apart by conntrack zones (firecracker-driver Decision 4).
- **hyperv.** The relay boot VHDX is rebuilt from the new init, for the
  resolved link. A changed setting is refused until `refresh-cache.sh --link`
  rebuilds it. The Windows guest's static address comes from the per-build
  seed. New: the record refusal and the NAT-prefix overlap refusal in setup.
- **vz.** The host relay takes the resolved `--subnet`, and the netstack
  serves its gateway. The prepared image's `warden-boot` script is rendered
  from the resolved link at prepare, so vz images are re-prepared once (D2)
  and again after a change of the setting; the record refusal says so. The
  host-mode relay blocks dials into its subnet.
- **container.** Behaviour unchanged. It never had the link (its relay and
  build share a `/29` from `100.64.87.0/24`), does not import
  `driver/buildlink` (ORCH-01: no cross-contamination), and ignores
  `BuildRequest.BuildLink`. `--build-link` with `--driver container` is
  refused like the other VM flags, and `[vm] build_link` is ignored, like
  `[vm] cpus`.

## Security note

**Boundary.** env-relay (GLOBAL-06): the build link is the only path between
the build environment and the relay. One relay change (a8) also touches the
relay's upstream filter in host mode.

**Posture.** Meant to be unchanged. Isolation stays topological: one NIC on
a private segment (socketpair, socket netdev, private switch, address-less
bridge), and in the relay VM, REDIRECT of 80/443/53 on the build interface,
MASQUERADE on the upstream interface and FORWARD DROP. None of those rules
matches on the link's address, so moving it, by default or by setting,
changes no rule. What the move or the setting could break, and what guards
it:

- *Who chooses the link.* The value comes from the host's config or the
  command line, never from the build. It reaches the relay VM on its own
  kernel command line, which the build cannot read, and the build VM on its
  own command line or seed, which carries only addresses. The inits accept
  only dotted quads and prefix 30 and never evaluate the command line.
- *The link must stay disjoint from every upstream range BuildWarden
  routes.* If it overlapped Firecracker's `100.64.88.0/22`, a relay's own
  upstream could be shadowed. A configured value that overlaps any of
  BuildWarden's ranges is refused at load (Decision 5), and a tier-0 test
  asserts the default is disjoint. Even then, the host anti-spoof rule (drop
  anything from `wfc<i>u` not sourced from the slot's relay) and MASQUERADE
  limited to `-s 100.64.88.0/22` keep build-sourced packets from leaving.
- *A link that shadows something the build should reach.* A user can choose
  a range their network uses. That is a reachability failure, not an escape:
  packets for those four addresses go to the build VM or the relay, both
  inside the boundary. Host interface addresses inside the link are refused,
  and the docs say what the setting shadows.
- *The host-mode relay's SSRF filter.* `10.0.0.2` was blocked as RFC 1918. An
  RFC 6598 guest address is not, so a build asking the vz host relay to dial
  `100.127.255.254` would go out the host's default route, which with
  Tailscale could be a real peer. a8 blocks the whole configured subnet,
  whatever range the user picks. Dials to other `100.64.0.0/10` addresses
  keep today's behaviour: Tailscale peers were reachable through the vz host
  relay before this change and still are, and every such fetch is recorded.
- *A resolver inside the link.* Still excluded on Firecracker (Decision 4),
  against the resolved link.
- *Stale or mismatched assets.* Refused before boot, so a mismatched pair
  never runs half configured.

**TEST-08 should exercise:** a build-VM packet sourced from the guest address
(default and a configured one), sent to an off-host canary, is dropped. On
Firecracker that is the existing anti-spoof case with its source updated. A
build that resolves a name to the link's addresses through the vz host relay
is refused, for a configured link too. A host resolver inside the link is
refused on Firecracker. A stale relay initramfs, a VHDX or vz image built for
another link, and a configured link overlapping a BuildWarden range or a host
interface, are each refused before any VM boots.

## Validation plan

- **Tier 0** (TEST-01: pure Go, table-driven, no host network).
  `driver/buildlink`: the embedded file parses and validates, and a malformed
  copy is rejected. `Parse`: the default; `/29` and `/31`; a misaligned
  `100.126.0.1/30`; IPv6; `0.0.0.0/30`, `127.0.0.0/30`, `169.254.1.0/30`,
  `224.0.0.0/30` and `240.0.0.0/30` refused. `Disjoint`: the default against
  the container base, Firecracker upstream, Hyper-V NAT default and qemu
  user-mode ranges, read from the packages' own constants in an external test
  package; one overlapping value per range refused. `HostOverlap`: an address
  inside refuses, a containing network warns, an unrelated one passes.
  `ReadRecord`: missing, different and equal. `BootArgs` and `Env` render.
  The stray-literal scan with its allowlist. `cmd/warden`: `resolveBuildLink`
  precedence (flag over project over user over default, defaults resolving
  with no config file, UX-07), a bad `[vm] build_link` failing at load
  (UX-08), and `--build-link` refused for container, for `image prepare` off
  vz and for `setup` off hyperv. Renders for the default and a configured
  link: seed goldens (`driver/linuxguest`), `winguest.WardenRunPS1`, the vz
  `warden-boot` script, and the qemu and Firecracker relay and direct-boot
  command lines. Record checks for each asset set on temporary directories,
  each refusal a `driver.Fixable` whose fix names the command with the link,
  and the record taken from the same location as its asset. Firecracker
  resolver selection: a resolver inside the resolved link is excluded, and
  one at `10.0.0.2` now passes. The Hyper-V NAT-prefix overlap refusal. The
  host-mode relay's `isBlockedIP`: every address in the subnet blocked,
  another `100.64.0.0/10` address not. All free of platform build tags, so
  they run everywhere (GLOBAL-07).
- **Lint.** `make lint` for each platform touched (FLOW-17): darwin for
  `driver/vz` and `cmd/relay/mode_host.go`, windows for `driver/hyperv`,
  `cmd/warden-io` and `driver/vz/network_windows.go`, linux for firecracker.
  (r1's text called this tier 1; it is not.)
- **Tier 1** (TEST-02): the `integration` workflow on the branch, since
  `relay/` and `cmd/` change.
- **Tier 2, CI.** `firecracker.yml` without the DNS workaround: the hosted
  runner's VPC resolver `10.0.0.2` is used directly on the default link, and
  the Ubuntu disk build and the direct-boot build succeed.
- **Tier 2, native (FLOW-09).** Blocking: linux-kvm, qemu-any, macos-vz,
  windows-hyperv, each with four cases. (1) With the pre-change assets still
  in place, the build and doctor refuse with the fix. (2) After the named
  rebuild, re-prepare or refresh, a build on the default link succeeds and
  its ledger verifies, and the guest's address is `100.127.255.254` (console
  or `warden-run.log`). (3) With `[vm] build_link = "100.126.255.252/30"`,
  doctor reports it. On linux-kvm and qemu-any a build succeeds with no
  rebuild. On macos-vz and windows-hyperv the build first refuses, naming
  `warden image prepare --driver vz` or `refresh-cache.sh --link
  100.126.255.252/30`, and succeeds after it. The guest's address is
  `100.126.255.254`. (4) A `--build-link` overlapping a BuildWarden range is
  refused at startup. Plus per host: linux-kvm a direct-boot and a disk
  build, and a host resolver inside the configured link excluded with the
  fix; qemu-any a direct-boot and a cloud-image Linux build; macos-vz the
  subnet block on the configured link; windows-hyperv a Windows guest build
  and the `--nat-prefix` overlap refusal. Tracked: macos-qemu (a Windows
  guest on qemu, after `qemu-windows-guest-package`), backlog
  `build-link-subnet-qemu-windows`. Not run: container-any and macos-finch,
  since the container driver's behaviour is untouched.
- **Queue steps.** The four `f6-validate-<host>` steps are not in the queue
  during Design. The only steps that exist now finish with Design, so a
  `host:` step needing them would be offered to the native machines while
  Plan and the design review are still open, and `wf validate` refuses a need
  on a step not yet seeded. Plan (`f2-plan-r2`) adds them, each needing
  `f2-design-review-r2` (seeded with it in phase 2). Implement adds
  `f3-implement` to their needs, and Publish makes `f6-open-pr` need all
  four.
- **Security (TEST-08).** The Security note's cases are covered at tier 0
  and in native validation. Firecracker's anti-spoof rule matches no link
  address, so its test needs only its source updated. The Arm A corpus does
  not exist yet (security-suite is in Design), so Consolidation hands the
  Security note's list to it.

## Implementation plan

Plan r2 (phase 2, `f2-plan-r2`, 2026-11-04), written for the r2 design; the
r1 plan for the fixed link is at `fe166f3`. Each task names the rules it must
satisfy and the open assumed questions it rests on, so a change to an
assumption at the design review changes the tasks that cite it (a1 and a6 are
confirmed). Tasks 12 to 23, 29 and 31 rework code that already exists
(FLOW-06). Tasks 33 and 34 are the reference-doc and `overview.md` updates the
change owes (UX-01, FLOW-15).

**Plan notes for the design review** (FLOW-19). Planning read every site in
the site table and found the design workable as written. Where the design is
silent or its text and the code disagree, the plan takes the reading below; the
review confirms it or sends the design back:

- *Seed goldens and the stray-literal allowlist* (Decision 2, UX-01). The
  site table keeps `driver/linuxguest/testdata/seed/` as goldens for the
  default link, which hold `100.127.255.253`/`.254`, but Decision 2's
  allowlist names only `driver/buildlink/`, reference docs, `docs/design/`
  and named tests. The plan allowlists that testdata directory as fixtures of
  the named render tests (task 6). Other render tests compare with
  `buildlink.Default()`, not literals.
- *The ranges `Disjoint` reads* (Decision 5, ORCH-01). Only Hyper-V's
  `DefaultNATPrefix` is an exported CIDR today: the container base is
  `driver.WardenBaseNet = "100.64.87"` (an octet prefix), Firecracker's
  `upstreamNet` is unexported, and qemu has no constant for its user-mode
  `10.0.2.0/24` (qemu's built-in default). Task 2 exports one CIDR constant
  per package, leaving existing names in place.
- *Doctor functions take the link* (a7, FLOW-02). `qemu.Preflight()`,
  `vz.Preflight()`, `firecracker.Doctor(out)` and `hyperv.Doctor(w, c)` are
  package functions called from `cmd/warden/host.go`, not `Driver` or
  `Session` methods, so giving them the resolved link keeps a7's "interfaces
  unchanged".
- *Host addresses* (Decision 4, ORCH-01). `HostOverlap` takes the addresses as
  an argument; the four VM drivers read them through one small
  `driver.HostPrefixes()` (over `net.InterfaceAddrs`) so `buildlink` stays a
  leaf of `embed` and `net/netip`. The container driver does not call it.
- *A durable Hyper-V NAT and a later change of the setting* (Decision 4,
  UX-04). Setup refuses a `--nat-prefix` overlapping the resolved link, but a
  durable switch made by an earlier `warden setup` with a custom
  `--nat-prefix` is not re-checked when `build_link` changes afterwards; the
  design names the check at setup only, and the plan follows it. The review
  decides whether hyperv's build start and doctor should also check the
  durable NAT's prefix.
- *Firecracker's anti-spoof test* (TEST-08). The Security note says the
  existing anti-spoof case needs "its source updated", but the tier-0 rule
  test (`slots_test.go`) names no link address, so it does not change; the
  guest-sourced canary case runs natively on linux-kvm (task 38).

### The definition

1. Add `driver/buildlink`: `link.env` with the four keys of the Design
   section, and `buildlink.go` embedding it, with `Link` (`Net`, `Relay`,
   `Guest`), `Default()` (validated at init: four keys, prefix 30, relay and
   guest the first and second hosts of `Net`), `Parse(cidr)` (IPv4, exactly
   `/30`, network-aligned with a "did you mean" fix, not `0/8`, loopback,
   link-local, multicast or class E), `Disjoint(named ranges)`,
   `HostOverlap(addrs)` (refuse: an address inside the link; warn: a network
   containing it), `GuestCIDR()`, `Mask()`, `Env()`, `BootArgs()` (the three
   `warden.link_*` keys) and `ReadRecord(path)` (`ErrNoRecord` when missing).
   Standard library only, nothing from `driver`, so errors are plain and each
   caller wraps them. (GLOBAL-08, ORCH-01, ORCH-03, UX-08, GLOBAL-07; a2, a10)
2. Export one CIDR constant for each range the link must avoid: the container
   base `100.64.87.0/24` beside `driver.WardenBaseNet`, Firecracker's
   upstream `100.64.88.0/22`, and qemu's user-mode `10.0.2.0/24` (commented as
   qemu's built-in default); Hyper-V's `DefaultNATPrefix` is already exported.
   Existing names and behaviour are unchanged. (ORCH-01, GLOBAL-08; a10)
3. `tools/relay-vm/linkenv.go` (`//go:build ignore`, run with `go run` in the
   module): prints `buildlink.Parse(cidr).Env()` for its argument, or exits
   non-zero with `Parse`'s error, so no shell script parses or derives an
   address. (GLOBAL-08, UX-08; a2)
4. Tier-0 tests for `driver/buildlink` (TEST-02 tier 0, TEST-01: pure Go,
   table-driven, no host network): the embedded file parses and validates and
   a malformed copy is rejected; `Parse` accepts the default and refuses
   `/29`, `/31`, misaligned `100.126.0.1/30` (fix names `100.126.0.0/30`),
   IPv6, `0.0.0.0/30`, `127.0.0.0/30`, `169.254.1.0/30`, `224.0.0.0/30` and
   `240.0.0.0/30`; `HostOverlap` refuses an address inside, warns on a
   containing network, passes an unrelated one; `ReadRecord` on missing,
   different and equal files; `Env` and `BootArgs` render for the default and
   a configured link. (TEST-02, TEST-01, GLOBAL-07; a3, a10)
5. Tier-0 `Disjoint` test in an external test package, reading task 2's
   constants and `hyperv.DefaultNATPrefix` rather than restating them: the
   default is disjoint from all four, and one overlapping value per range is
   refused. (TEST-02 tier 0, TEST-01, GLOBAL-06; a10)
6. Tier-0 stray-literal test: walks the module tree from the root (no `git`,
   TEST-01), skipping `.git`, `.dev`, `dist`, gitignored build outputs and
   binary files, and fails on a whole-address match of `100.127.255.252`,
   `.253`, `.254`, `10.0.0.0/30`, `10.0.0.1` or `10.0.0.2` outside the
   allowlist kept in the test: `driver/buildlink/`, the new default in
   `docs/book/**` and `tools/*/README.md`, the old link only under
   `docs/design/`, `driver/linuxguest/testdata/seed/` (plan note), and the
   named refusal and parsing tests (Firecracker's `10.0.0.2` resolver case).
   `10.0.0.20` and `110.0.0.1` do not match. (TEST-02 tier 0, TEST-01,
   GLOBAL-08, UX-01; a2)

### The setting

7. `driver.BuildRequest` gains `BuildLink buildlink.Link`, the zero value
   meaning the default (a helper returns `Default()` for it). The container
   driver ignores it and does not import `buildlink`. (ORCH-01, GLOBAL-08,
   UX-03; a7)
8. `cmd/warden/config.go`: `[vm] build_link` in `VMConfig`; `LoadConfig` runs
   `buildlink.Parse` and `Disjoint` (task 2's ranges) on it as the file loads,
   with a `driver.Fixable` naming the key, the file and a valid example.
   (UX-08, UX-07, ORCH-03, UX-04; a9, a10)
9. `--build-link <cidr>` (long form only) on `build` and `shell`
   (`main.go`), `doctor` and `setup` (`host.go`) and `image prepare`
   (`image.go`), help text marking it advanced; `resolveBuildLink` (flag over
   project over user over default, returning the source label `default`,
   `warden.toml`, `config.toml` or `--build-link`), flags checked before any
   driver work. `validateFlagsForDriver` refuses it for `container` on
   `build`, `shell` and `doctor`; `setup` refuses it off `hyperv` and
   `image prepare` off `vz`, as `--network` is refused. The resolved link goes
   into `BuildRequest`, doctor, Hyper-V setup and vz prepare. (UX-05, UX-06,
   UX-07, ORCH-03, UX-04, UX-08; a9, a10)
10. Tier-0 tests in `cmd/warden`: `resolveBuildLink` precedence and source
    labels, the default resolving with no config file, a bad
    `[vm] build_link` failing at load, an overlapping `--build-link` refused
    at startup, and the flag refused for container, for `setup` off hyperv
    and for `image prepare` off vz. (TEST-02 tier 0, TEST-01, UX-07, UX-08;
    a9, a10)
11. `warden doctor` for every VM driver prints `build link <net> (<source>)`
    and the host-overlap result (task 28's check: an address inside blocks; a
    containing network is a warning), and passes the link to
    `qemu.Preflight`, `vz.Preflight`, `firecracker.Doctor` and
    `hyperv.Doctor`, which report their records (tasks 24 to 26) and, on
    Firecracker, the resolvers (task 29). (UX-04, ORCH-01; a3, a10)

### Shell inits and baked assets (rework)

12. `tools/relay-vm/init`: sources `/etc/warden/link.env`, then takes
    `warden.link_relay=`, `warden.link_guest=` and `warden.link_prefix=` from
    `/proc/cmdline` over it, accepting only dotted quads and prefix `30` (a
    bad value is logged with `say` and the sourced default kept; the command
    line is never evaluated), and addresses the build interface from them.
    The header comment lists the keys; the log line names the address.
    `build-initramfs.sh` copies `driver/buildlink/link.env` to
    `/etc/warden/link.env` in the image and writes it beside the output as the
    record (`output/link.env`, `output/x86_64/link.env`). (BOOT-03, BOOT-06,
    GLOBAL-06, GLOBAL-08; a2, a3)
13. `tools/build-vm/init` and `build-initramfs.sh` (qemu direct boot): the
    same sourcing and keys for its address, default route, resolver and
    `initialize --gateway`; record `output/link.env`. (BOOT-03, EXEC-04,
    BOOT-06; a2, a3)
14. `tools/firecracker-vm/init` and `build-initramfs.sh`: as task 13. (BOOT-03,
    EXEC-04, BOOT-06; a2, a3)
15. Hyper-V relay boot disk: `refresh-cache.sh` parses `--check`, `--force`
    and `--link <cidr>` (default: `link.env`), renders the link through task
    3's helper (a bad value stops it before building), and hands the file to
    `build.sh`, which bakes it into the UKI's initramfs at
    `/etc/warden/link.env`; `build-boot-vhdx.sh` and
    `build-boot-vhdx-native.ps1` carry it to the output; the script installs
    `relay-boot.link.env` beside `relay-boot.vhdx`, moving the pair to `.prev`
    together. The source stamp adds `driver/buildlink` and the link value, so
    `--check` reports a disk built for another link as stale with a fix naming
    `--link`. (BOOT-06, UX-04, GLOBAL-08; a2, a3)

### Per-build channels (rework)

16. `driver/qemu/vm.go`: the relay `-append` and the direct-boot build
    `-append` add `BootArgs()` of the request's link. Tier-0 test of both
    command lines for the default and a configured link. (BOOT-03, ORCH-01,
    TEST-02 tier 0; a2)
17. `driver/firecracker/vm.go`: `relayArgs` and the direct-boot `buildConfig`
    boot args add `BootArgs()`. Tier-0 test as task 16. (BOOT-03, ORCH-01,
    TEST-02 tier 0; a2)
18. `driver/linuxguest/seed.go`: network-config v2 address, gateway and
    nameserver, and the `runcmd` `initialize --gateway`, rendered from the
    request's link (qemu and the Firecracker disk path); goldens in
    `testdata/seed/` for the default, plus a tier-0 render for a configured
    link. The `initialize` contract is unchanged. (BOOT-06, EXEC-04, TEST-02
    tier 0; a2, a7)
19. `driver/winguest`: `WardenRunPS1` renders `--gateway` and `--ip` from the
    request's link (qemu and Hyper-V); `BuildGatewayIP` and `BuildGuestCIDR`
    stay as the default's values for existing callers. Tier-0 render for the
    default and a configured link. (BOOT-06, EXEC-04, TEST-02 tier 0; a2, a7)
20. `driver/vz/network_unix.go`: relay, build and gateway addresses and the
    mask from the request's link; the host relay's `--subnet` is `link.Net`.
    (ORCH-01, GLOBAL-07; a2)
21. `driver/vz/image.go`: the `warden-boot` script's `GW`, `SELF` and `MASK`
    (and the `firstBoot` comment) are rendered from the resolved link at
    prepare, and both prepare paths (`warden image prepare`'s first prepare
    and `ReprepareImage`) write `link.env` into the prepared image directory
    after installing the script. Tier-0 render test (`image.go` has no build
    tag). (BOOT-07, BOOT-06, TEST-02 tier 0; a2, a3, a7)
22. `tools/vz-image-prep/{personalize,uv-build-test,netfwd}.go` take the
    prep-time link from `buildlink.Default()` and render their shell snippets
    from it; `netfwd`'s default `--subnet` is it. `cmd/warden-vmnet/main.c`:
    `--subnet` becomes required (no callers), so the file holds no address.
    (GLOBAL-08, GLOBAL-07; a2)
23. `cmd/warden-io`: the `initialize_test.go` and `platform_windows_test.go`
    examples and the `platform_windows.go` comments use other addresses;
    parsing behaviour is unchanged. (EXEC-04, TEST-02 tier 0; a7)

### Records and refusals

24. `driver/relayvm`: each `Asset` resolves its record from the same location
    `Find` chose (release: file name plus `.link.env`; source tree: `link.env`
    in the output directory). qemu (`resolveRelayAssets`,
    `resolveBuildAssets`) and Firecracker require a record beside every
    initramfs, checking relay and build assets together before anything
    boots; a missing one is a `driver.Fixable` naming the rebuild
    (`tools/relay-vm/build-initramfs.sh`, `tools/build-vm/` or
    `tools/firecracker-vm/build-initramfs.sh`, or upgrading the release
    assets). The value is not compared. (UX-04, BOOT-06, ORCH-01; a3)
25. `driver/hyperv` (`buildassets.go`): `resolveRelayBootVHDX` reads
    `relay-boot.link.env` beside the cached disk, or `<name>.link.env` beside
    a `WARDEN_RELAY_BOOT_VHDX` override; missing or different from the
    resolved link is a `driver.Fixable` naming
    `tools/relay-vm/hyperv/refresh-cache.sh --link <link>`. (UX-04, ORCH-01;
    a3)
26. `driver/vz` StartBuild and `Preflight`: the prepared image's `link.env`
    must equal the resolved link; otherwise a `driver.Fixable` naming
    `warden image prepare --driver vz`, plus `--build-link <link>` when the
    link came from the flag. With no prepared image, Preflight reports as
    today. (UX-04, BOOT-07; a3)
27. Tier-0 tests for tasks 24 to 26 on temporary directories: missing,
    different and equal records for each asset set, each refusal a
    `driver.Fixable` whose fix names the command with the link, and the record
    taken from the same location as its asset; kept free of platform build
    tags. (TEST-02 tier 0, TEST-01, GLOBAL-07, UX-04; a3)

### Guards

28. Host-interface overlap: `driver.HostPrefixes()` reads
    `net.InterfaceAddrs`; qemu, Firecracker, Hyper-V and vz StartBuild call
    `link.HostOverlap` on it before any VM boots and refuse an address inside
    the link with a `driver.Fixable` naming `[vm] build_link` /
    `--build-link`. (UX-04, GLOBAL-06, ORCH-01; a4, a10)
29. `driver/firecracker/posture.go` (rework): the `buildLink` variable becomes
    the request's link in resolver selection for StartBuild and doctor; the
    no-resolver refusal names both fixes (a resolver outside the link, or
    another `[vm] build_link`); the `slots.go` comment follows. Tier-0: a
    resolver inside a configured link is excluded with that fix, one at
    `10.0.0.2` now passes; `slots_test.go`'s `uplinkFromRoute` example moves
    off the old link. (UX-04, ORCH-01, TEST-02 tier 0; a4)
30. `driver/hyperv` setup: `SetupOptions` gains the link and `RunSetup`
    refuses a `--nat-prefix` overlapping it before changing anything, with a
    `driver.Fixable` naming another prefix; the `buildhost.go` comment
    follows. Tier-0 test. (UX-04, GLOBAL-06, TEST-02 tier 0; a4)
31. Host-mode relay (rework): `relay.Config` gains a blocked prefix that
    `Relay.isBlockedIP` checks (the gateway check folds into it);
    `cmd/relay/mode_host.go` passes the parsed `--subnet`; the
    `cmd/relay/main.go` `--subnet` comment says vz passes the resolved link,
    its default unchanged. `relay` does not import `buildlink`; container and
    VM modes leave the prefix unset. Tier-0: every address of the subnet
    blocked, for the default and a configured link, and another
    `100.64.0.0/10` address not. (GLOBAL-06, RELAY-04, GLOBAL-08, TEST-02
    tier 0; a8)

### CI

32. `.github/workflows/firecracker.yml`: remove the "Off-host DNS for the
    relay" step and its comment; the asset cache key adds
    `driver/buildlink/link.env`; `pull_request.paths` adds
    `driver/buildlink/**`. (TEST-02 tier 2; a4)

### Docs

33. Reference docs in the same change (UX-01): `guide/configuration.md` (the
    key in the Full Reference marked advanced, and an Advanced options section:
    when to change it, what to re-run on vz and Hyper-V, the checks and what
    the link shadows); `guide/cli.md` (`--build-link` under `build`, `shell`,
    `doctor`, `setup` and `image prepare`; doctor's link line, record and
    overlap checks; `setup --nat-prefix`'s overlap refusal);
    `guide/drivers.md` (Upstream DNS, an upgrade note per VM driver, changing
    the link on vz and Hyper-V); `concepts/network-isolation.md` (slot table,
    host-mode SSRF bullet); `contributing/structure.md`;
    `tools/win-image-prep/README.md`; and `warden doctor`'s help text in
    `cmd/warden/host.go`. (UX-01, UX-04, UX-07; a9, a10)
34. `concepts/overview.md` pseudocode: each VM driver's StartBuild resolves the
    link and checks records and host overlap before booting; the relay block
    notes host mode refuses dials into its subnet. (FLOW-15)

### Verification

35. `make test`, and `make lint` for linux, darwin (`driver/vz`,
    `cmd/relay/mode_host.go`) and windows (`driver/hyperv`, `cmd/warden-io`,
    `driver/vz/network_windows.go`); push the branch at each green checkpoint.
    (TEST-01, FLOW-17, GLOBAL-07)
36. Tier 1: the `integration` workflow on the branch, since `relay/` and
    `cmd/` change; record the result. (TEST-02 tier 1)
37. Tier 2, CI: `firecracker.yml` passes on the PR with the runner's own VPC
    resolver; the disk and direct-boot builds succeed. (TEST-02 tier 2; a4)
38. Tier 2, native: `f6-validate-linux-kvm`, `-qemu-any`, `-macos-vz` and
    `-windows-hyperv` (each needing `f2-design-review-r2`, then
    `f3-implement`; `f6-open-pr` needs all four) run the Validation plan's
    four cases on the default and on `100.126.255.252/30`, plus each host's
    cases, including the guest-sourced canary drop on linux-kvm. macos-qemu
    stays tracked: Publish adds `build-link-subnet-qemu-windows` to the
    native-gaps backlog. (FLOW-09, TEST-03, TEST-08; a5)
39. Security (TEST-08): the Security note's cases are covered at tier 0 (tasks
    5, 10, 27 to 31) and natively (task 38). The Arm A corpus does not exist
    yet (security-suite is in Design), so Consolidation hands it the Security
    note's list. (GLOBAL-06, TEST-08)

**Site-table coverage.** `driver/buildlink/`: 1, 4 to 6. `BuildRequest`: 7.
`cmd/warden`: 8 to 11, 33. Relay init and `build-initramfs.sh`: 12.
`linkenv.go`: 3. `tools/relay-vm/hyperv/`: 15. `tools/build-vm`: 13.
`tools/firecracker-vm`: 14. qemu `vm.go`: 16. Firecracker `vm.go`: 17.
`linuxguest`: 18. `winguest`: 19. vz network: 20; image: 21; StartBuild and
Preflight: 26, 28. `relayvm` and qemu assets: 24. Firecracker posture: 29.
`driver/hyperv`: 25, 28, 30. `relay/` and `cmd/relay`: 31. `warden-vmnet` and
`vz-image-prep`: 22. `cmd/warden-io`: 23. `firecracker.yml`: 32. Reference
docs: 33. `overview.md`: 34.

## Design review

- **r1, 2026-11-03: changes requested** (Jeff, attended session, FLOW-19).
  Read at `fe166f3`. Sent back to Design (FLOW-06).
  - **a1: the default is confirmed: `100.127.255.252/30`.**
  - **a6 is reversed: the link must be manually configurable.** Put the
    setting in the advanced configuration options. It covers a collision,
    or some other limitation nobody knows about yet, on a user's network.
    Fixed-only is rejected.
  - **a2, a3, a4, a5, a7 and a8 are not confirmed.** Re-evaluate each one
    with a configurable link in mind. In particular:
    - **a2:** `link.env` becomes the default, not the only definition.
      Decide how a configured value reaches the baked assets: the relay and
      build initramfs, the Hyper-V UKI (its command line is baked in), and
      the vz `warden-boot` script (baked at prepare). The choices are
      passing the value at boot or rebuilding per value.
    - **a3:** the record check compares against the configured value. The
      refusal's fix has to cover a deliberate change of the setting.
    - **a4 and a8:** the guards (Firecracker resolver exclusion, Hyper-V
      `--nat-prefix` overlap, host-mode subnet block) follow the configured
      link.
    - **a7:** the setting is a new config surface (UX-01; surfaces gain
      config). Re-check the seams and `alters_relationships`.
    - **Validation of the setting:** a /30, and disjoint from BuildWarden's
      own ranges. Add tier-0 coverage for it, and check ORCH-03.
  - **Carry forward into the revised design:** the plan's reconciliation
    notes (a8 touches `relay/`; `warden-vmnet` has no callers; the allowlist
    for reference docs; `guide/cli.md` and the doctor help; the
    `firecracker.yml` paths; tier labels; record names; the Hyper-V stamp).
    Also two text fixes: Decision 2 has four keys, not three, and
    `tools/build-vm/init` is dropped from the firecracker cache key, since
    that job never builds it.
- **r1 finding, addressed in Design (f1-design-r2, 2026-11-04).** What each
  point changed:
  - *a6 reversed:* Decision 5 is now the setting: `[vm] build_link` and
    `--build-link`, advanced, default `100.127.255.252/30`. Where it lives is
    new question a9. Surfaces gain `cli` and `config`.
  - *a2:* per-build delivery wherever a boot path has a per-build channel
    (new `warden.link_*` command-line keys for the qemu and Firecracker
    relay and direct-boot inits; the seeds and vz `--subnet` as before).
    Rebake only where there is none: the Hyper-V VHDX through
    `refresh-cache.sh --link`, and the vz image at prepare. The claim-borne
    alternative is recorded and rejected (Decision 2).
  - *a3:* records are compared with the resolved link for baked assets, and
    only required to be present where the command line carries the link.
    Every fix names its command with the link (Decision 3).
  - *a4, a8:* every guard takes the resolved link. A host-interface overlap
    refusal is added for every VM driver (Decision 4).
  - *a7:* re-checked. Still no seam and `alters_relationships: false`, with
    the reasons stated in a7: the command-line keys are not a relay seam,
    and `BuildRequest` gains an additive input. `drivers` now declares
    container conservatively.
  - *a5:* its premise is corrected ("driver/qemu does not change" is no
    longer true), and the conclusion is unchanged.
  - *Validation:* Decision 5 and new question a10. Tier-0 cases are in the
    Validation plan. ORCH-03 and UX-07 precedence is tested.
  - *Plan notes and text fixes:* folded in as listed under Implementation
    plan. The old plan is marked superseded.
  - *Native validation:* same blocking hosts. Each now also runs a
    non-default link. The `f6-validate` steps return in Plan (Validation
    plan, Queue steps).

- **r2, 2026-11-04: changes requested** (Jeff, workflow-human, FLOW-19).
  Read at `ef8c014`, with the design published as a review page. Sent back
  to Design (FLOW-06). Before answering, Jeff paused the review to commission
  an overview of today's network setup: what is baked and what is
  configurable on each driver (the Claude Docs page "BuildWarden network
  configuration: what is baked, what is configurable"). Rebuilds and cache
  invalidation are the concern.
  - **a2: answered (c), deliver the link in the relay claim.** The relay
    configures its own build interface from the claim, so no relay-side
    asset bakes the link, and Hyper-V's rebake goes away. This changes the
    `relay-claim` seam.
  - **a3: link records only where something still bakes the link.** Design
    re-decides which assets still need a record once the claim carries the
    link. The candidates are the guest side: vz `warden-boot` and the
    direct-boot inits.
  - **a7: superseded by a2.** Re-decide `seams` (`relay-claim` at least) and
    `alters_relationships`. The claim gains the link and the relay takes over
    interface setup from the init.
  - **Confirmed:** a4 (a) as designed; a5 (a) tracked; a8 (a) block the
    subnet, which Design may apply in every mode now that the relay knows
    the link; a9 (a) `[vm] build_link` plus `--build-link`, with the command
    list trimmed to what still needs it; a10 (a) as designed.
  - **For Design r3:**
    - Weigh the overview's open questions on per-build channels versus
      baking, a general staleness stamp, Hyper-V's fixed relay address and
      missing claim token, and the link allocation model.
    - Keep the Bootstrap comparison in mind if interface setup moves from
      the init into the relay (BOOT-07).

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None yet.

## Archive summary

Filled in at Publish (FLOW-12).

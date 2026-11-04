---
feature: build-link-subnet
status: draft
drivers: [container, qemu, vz, hyperv, firecracker]  # every VM driver: the claim carries the link (driver/session.go), driver/linuxguest (qemu, firecracker), driver/winguest (qemu, hyperv), driver/vz, driver/firecracker, driver/relayvm, the relay VM init and the direct-boot inits. container is declared conservatively (FLOW-02): relaycfg/, relay/ and cmd/relay change under it, but its driver sends no build_link and its behaviour is unchanged (question a17)
seams: [relay-claim, relay-guest-api, bootstrap-model]  # relay-claim: the claim gains build_link (question a11); relay-guest-api, conservatively: the relay's address on the build segment now comes from the claim, and the host-mode DHCP lease becomes the vz guest's address (protocol unchanged); bootstrap-model: BOOT-03's launcher-addressing wording gains a DHCP lease served by the relay (Bootstrap comparison, question a14). executor-initialize is unchanged: --gateway/--ip keep their contract, only the values change (question a19)
surfaces: [cli, config, output]  # [vm] build_link and --build-link on build, shell and doctor (advanced, Decision 8); doctor's build-link and stamp lines; fixable stale-launcher, overlap and old-relay refusals; firecracker's resolver refusal names the configured link (UX-01, UX-04, ORCH-03)
security_boundary: env-relay  # the link is the only path between the build environment and the relay, and the relay now configures it from the claim (Security note)
alters_relationships: true   # a versioned seam changes (relay-claim), the bootstrap model's BOOT-03 wording changes, and the relay takes over its build interface's addressing from the relay VM init (question a19; FLOW-05 slot)
design_review: required
native_validation:
  - {host: linux-kvm, blocking: true, reason: "reaches driver/firecracker (claim carries the link, direct-boot command line, launcher stamp, resolver exclusion against the resolved link), the vm-mode relay's new build-interface setup on the x86_64 relay VM, and tools/firecracker-vm/init. CI tier 2 (firecracker.yml) covers a fresh hosted runner on the default link without its DNS workaround; the native run covers the upgrade path (an old direct-boot initramfs refused; an old relay initramfs still works) and a non-default link with no rebuild"}
  - {host: qemu-any, blocking: true, reason: "reaches driver/qemu (claim carries the link, direct-boot command line, launcher stamp), the vm-mode relay's build-interface setup on the aarch64 relay VM, tools/build-vm/init and the driver/linuxguest seed on qemu: a direct-boot and a cloud-image Linux build on the default and a non-default link, an old direct-boot initramfs refused, an old relay initramfs still working"}
  - {host: macos-vz, blocking: true, reason: "reaches driver/vz (claim carries the link, no --subnet, warden-boot by DHCP from the host relay, the image stamp) and cmd/relay host mode (netstack built at claim, DHCP lease live, subnet block): an image prepared before the change refused, re-prepared once, then builds on the default and a non-default link with no further re-prepare"}
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv (claim carries the link, relay-boot stamp, NAT-prefix overlap refusal), the relay boot VHDX refreshed once for the new relay binary and init, the vm-mode relay's build-interface setup on hv_netvsc, and driver/winguest on a Windows guest on the default and a non-default link"}
  - {host: macos-qemu, blocking: false, reason: "Windows guest on qemu: driver/qemu's changes are guest-agnostic (the claim and the launcher stamp) and validated by qemu-any, and the Windows-specific piece (driver/winguest) by windows-hyperv. qemu Windows builds already fail on main (backlog qemu-windows-guest-package), so a blocking step could not pass for reasons outside this feature (question a5)"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows guest build on the default and a non-default link (backlog build-link-subnet-qemu-windows), after qemu-windows-guest-package is fixed"}
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, ORCH-01, ORCH-03, BOOT-01, BOOT-02, BOOT-03, BOOT-05, BOOT-06, BOOT-07, EXEC-04, RELAY-04, RELAY-05, UX-01, UX-03, UX-04, UX-05, UX-06, UX-07, UX-08, TEST-01, TEST-02, TEST-03, TEST-08, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19]
concepts: [build-link, configurable-build-link, advanced-config, relay-claim, claim-protocol, relay-interface-setup, two-vm-topology, network-isolation, address-plan, rfc6598, upstream-dns, resolver-selection, relay-vm-init, kernel-command-line, dhcp, launcher-stamp, stale-assets, image-prepare, socketpair-topology, config-validation, ssrf-filter]
related: [firecracker-driver.md, unified-bootstrap.md, hyperv-windows-prepare.md, vm-driver-release-plan.md, windows-driver-plan.md, platform-compatibility.md]
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

The link is also written at least eight times with no shared definition, and
three of those copies are baked into boot assets (the relay VM init, the two
direct-boot inits) or a prepared image (vz `warden-boot`) that nothing checks
for staleness ("BuildWarden network configuration: what is baked, what is
configurable", the overview Jeff commissioned for design review r2).

This feature moves the link for every VM driver to a default range no common
upstream uses and lets the user move it again when their network collides
with the default anyway (design review r1). After design review r2 the link
reaches every consumer per build: the relay takes it from the claim and
configures its own build interface, and each guest takes it from its seed,
its kernel command line or the relay's DHCP lease. No asset bakes a link
value, so a change of the setting never needs a rebuild, a refresh or a
re-prepare.

## Prior art

- [`firecracker-driver.md`](firecracker-driver.md), Decision 4 (slots: the
  upstream `/30`s in `100.64.88.0/22`, beside the container driver's
  `100.64.87.0/24`; every slot reuses the one build link, kept apart by a
  conntrack zone per bridge) and Decision 5 (resolver selection: loopback,
  host-local and, since PR #90, build-link resolvers are excluded; none left
  is a fixable refusal). Its Rollback notes ("CI tier 2") queued this feature.
  Reused: the RFC 6598 address plan, the resolver exclusion (retargeted at the
  resolved link), its fix text, and the direct-boot kernel command line as a
  per-build channel. Decision 4's conntrack zones still matter, since every
  slot keeps reusing the same two addresses.
- The claim protocol (`relaycfg/`, GLOBAL-08's `relay-claim` seam;
  [`architecture.md`](../book/src/concepts/architecture.md), "Claim
  protocol"). Its package doc already states the ordering this design relies
  on: "the build guest is not started until the claim has completed". Its
  decoder uses `DisallowUnknownFields`, so a relay that predates a new field
  refuses the claim instead of ignoring it, and its reply is written only
  after the relay's start callback returns. Both shape the compatibility
  story (Decision 3).
- [`unified-bootstrap.md`](unified-bootstrap.md) and
  [`bootstrap.md`](../development/bootstrap.md): the executor gets the relay's
  address from `initialize --gateway` (and `--ip` where the guest has no
  cloud-init). That contract (EXEC-04, the `executor-initialize` seam) is
  unchanged; only the value changes. BOOT-03 allows static addressing applied
  by the platform's own boot configuration in the launcher. vz's D2
  (executor and boot script baked at prepare) is why a vz image must be
  re-prepared once for this change. BOOT-06 is why the Windows and Linux
  seeded guests need nothing: their address comes from the per-build seed.
- [`windows-driver-plan.md`](windows-driver-plan.md), `configureNetwork`:
  "prefer DHCP served by the relay (matches the topological model; the
  guest's sole NIC faces the relay)". The host-mode relay already has that
  responder (`cmd/relay/ingress_dhcp.go`, offering the guest address with the
  relay as router and DNS); it is unused today because the vz guest is
  static. `tools/vz-image-prep/personalize.go` already runs a DHCP
  `warden-boot` (`ipconfig set <if> DHCP`) during prep. Decision 5 reuses
  both.
- [`hyperv-windows-prepare.md`](hyperv-windows-prepare.md): `warden-run.ps1`
  goes on the per-build `WARDEN` seed, and the prepared image carries only the
  startup task, so the Windows guest's address moves with no image change.
- `driver/subnet.go`: the container driver's `100.64.87.0/24` (RFC 6598,
  "unlikely to collide with user networks"), allocated per build. This design
  follows the same reasoning for the VM link's default and keeps the
  container's allocation as it is (Decision 4).
- `cmd/warden/config.go` and [`configuration.md`](../book/src/guide/configuration.md):
  the `[vm]` table holds the VM-driver defaults whose flag always wins
  (`cpus`, `memory_mb`, `network`); `[vm] network` / `--network` is the
  precedent for a key with a matching flag on `build`, `shell` and `doctor`.
- [`vm-driver-release-plan.md`](vm-driver-release-plan.md), "Escape vectors":
  DHCP starvation and spoofing, and ARP spoofing of the relay, are already on
  the TEST-08 list; making the host-mode DHCP lease live makes them current
  for vz (Security note).
- The network overview (r2): the verified per-driver topology, every baked
  asset and its staleness behaviour, and five open questions. Decision 10
  answers or defers each one.

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
   | Link-local `169.254.x.0/30` (away from `169.254.169.0/24`) | No routed address can be link-local, but on-link cloud services are, and the relay must reach some of them through host NAT: AWS DNS `169.254.169.253`, Kubernetes NodeLocal DNSCache `169.254.20.10`, ECS/EKS agents `169.254.170.x` | macOS configd treats a 169.254 IPv4 address as self-assigned and may not make the service primary. With only a link-local address, macOS and Windows may ARP for every destination instead of using the router (RFC 3927 §2.6.2). Windows labels it an autoconfiguration address and the network unidentified. The host-mode relay's SSRF filter already blocks link-local destinations | Rejected, and refused as a configured value (Decision 8) |
   | Benchmark `198.18.0.0/15`, TEST-NETs, class E `240.0.0.0/4` | Clash/Surge-style fake-IP DNS hands out `198.18.x`. TEST-NETs appear in labs and documentation | Windows and macOS refuse to assign class E | Rejected as the default; class E refused as a configured value |
   | Keep `10.0.0.0/30` and policy-route the relay's own sockets (marked dials take an upstream-only table) | None: overlap stops mattering | Unchanged | Rejected for now: a relay change in every dialer (proxy, DNS forward, collector), RELAY-04 parity across three modes, and nothing to gain on vz. Kept as the follow-up if configurability proves not enough |

   No range is guaranteed free, so the guards stay (Decision 7) and the link
   is configurable (Decision 8).

2. **One definition, in Go; the link reaches every consumer per build, and
   no asset bakes it** (question a2, answered (c) in r2; questions a12 to
   a14). `driver/buildlink` holds the default as a Go constant and is the
   only place it is written. r2's embedded `link.env` and its shell copies
   go: no shell script needs the link any more, because the relay VM init no
   longer addresses the build interface and the direct-boot inits take the
   link from their command line. A configured value is only the network
   (`100.126.0.0/30`, say): the relay is always its first host and the build
   VM its second, so the two can never disagree.

   | Consumer | Channel, per build | Decision |
   |---|---|---|
   | relay, vm mode (qemu, firecracker, hyperv) | the claim's `build_link`; the relay addresses its build interface before it replies | 3, 4 |
   | relay, host mode (vz) | the claim's `build_link`; the relay builds its netstack from it | 3, 4 |
   | Linux cloud-init seed (`driver/linuxguest`, qemu and Firecracker disk path) | rendered per build, as today | 5 |
   | Windows `WARDEN` seed (`driver/winguest`, qemu and Hyper-V) | `warden-run.ps1` rendered per build, as today | 5 |
   | direct-boot inits (qemu `tools/build-vm`, `tools/firecracker-vm`) | two keys on the build VM's kernel command line, which both drivers already write per build | 5 |
   | vz guest (`warden-boot`) | a DHCP lease from the host relay, which answers from the claim | 5 |

   Nothing in the table is baked. What remains baked is the *launcher* that
   reads a channel: the two direct-boot inits and vz `warden-boot` today bake
   `10.0.0.x` and must be replaced once by a launcher that reads its channel.
   Decision 6 covers that one-time upgrade. A tier-0 test fails if a default
   address, or the old `10.0.0.1`, `10.0.0.2` or `10.0.0.0/30`, appears in the
   tree outside an allowlist kept in the test: `driver/buildlink/`, the new
   default in reference docs (`docs/book/**`, `tools/*/README.md`), the old
   link only under `docs/design/`, the seed goldens
   (`driver/linuxguest/testdata/seed/`) and the named refusal and parsing
   tests. Matches are on whole addresses, so `10.0.0.20` or `110.0.0.1` do not
   trip it.

   *Rejected:* r2's design (per-build kernel command-line keys for the qemu and
   Firecracker relay inits, a rebake of the Hyper-V relay VHDX per value
   through `refresh-cache.sh --link`, the vz image baked at prepare per value,
   and a link record compared on each). Jeff answered a2 (c) in r2: the
   rebake and its cache invalidation were the concern. Baking into every
   initramfs and rebuilding per value (r1's alternative). A per-build seed
   disk or Hyper-V KVP for the relay VM (each adds a component to the relay
   VM's trust boundary for a value the claim can carry).

3. **The claim carries the link: `build_link {prefix, relay, guest}` on
   `/v1/claim`, optional on the wire, required by a vm-mode relay with an
   unaddressed build interface** (question a11; GLOBAL-08, RELAY-05).

   ```json
   "build_link": {"prefix": "100.127.255.252/30",
                  "relay":  "100.127.255.253",
                  "guest":  "100.127.255.254"}
   ```

   `relaycfg.Config` gains `BuildLink *BuildLink` (`json:"build_link,omitempty"`)
   and `relaycfg` validates it the same way for every claimant and mode:
   IPv4; `prefix` is its network address with a length from 8 to 30; `relay`
   and `guest` are distinct hosts inside it, neither its network nor its
   broadcast address; none is unspecified, loopback, link-local, multicast or
   class E. The relay's rule is deliberately looser than warden's (exactly
   `/30`, disjoint from BuildWarden's ranges, Decision 8): the relay is
   independently usable (GLOBAL-08), and a third-party orchestrator may use a
   wider segment. A claim that fails validation is refused with 400 and does
   not consume the relay, as a malformed claim is refused today.

   What each mode does with it:

   | Mode | With `build_link` | Without |
   |---|---|---|
   | vm | flushes the build interface's IPv4 addresses, adds `relay/len`, sets the relay's self address to `relay` and blocks upstream dials into `prefix`, all before it replies (Decision 4) | keeps the address the init (or a third-party image) already put on the build interface and uses it as self address and blocked prefix; refuses the claim if the interface has none |
   | host | builds the netstack, ARP, DHCP and listeners on `relay`/`guest`/`prefix` (Decision 4) | falls back to `--subnet`, as today |
   | container | refuses the claim: the runtime assigns the relay's address (`--ip`), and the relay holds no capability to change it | as today |

   **Versioning.** The field is added to `/v1`, not to a new `/v2/claim`.
   Pre-1.0 the seam may move (GLOBAL-08), and the change is additive on the
   wire: an orchestrator that sends no `build_link` gets today's behaviour
   from every mode whose interface is already addressed. The contract
   document is the `relaycfg` package doc and architecture.md's "Claim
   protocol" section; both list the field, its validation, the table above
   and the ordering rule ("a relay replies to a claim only after its build
   segment is configured"), which is what GLOBAL-08's automated check asks
   for. No ledger, metadata schema, environment record or spec names the
   link (RELAY-05), so no `specs/` version changes; the relay does not record
   `build_link` in the ledger, as it records no other topology.

   **Compatibility.**

   | Orchestrator | Relay | Outcome |
   |---|---|---|
   | new | new | the link from the claim |
   | new | old (a Hyper-V relay VHDX built before this change; a relay binary pinned beside an old warden) | the old decoder refuses the unknown field: `ErrRejected`, "unknown field \"build_link\"". `driver.Session.ClaimRelay` turns that into a `driver.Fixable` with the driver's fix (Hyper-V: `refresh-cache.sh`), before the build VM boots. On Hyper-V the launcher stamp (Decision 6) refuses earlier, before the relay VM boots; this mapping is the backstop |
   | old | new, vm mode, old relay VM init (assigns `10.0.0.1/30`) | no field: the relay keeps the init's address; behaves as today |
   | old | new, vm mode, new init (a refreshed Hyper-V VHDX with an older warden) | the claim is refused: "build_link is required: the build interface has no address" |
   | old | new, host mode | `--subnet`, as today |
   | any | container | warden's container driver sends no field; as today |

   On qemu, Firecracker and vz the relay binary is version-matched to warden
   per build (`driver.GuestBinary`), so only Hyper-V, whose VHDX bakes the
   relay binary, can meet a mismatch in practice. `cmd/collector`, the
   reference third-party claimant, gains `-build-link <cidr>` (relay the first
   host, guest the second) for vm-mode relays whose init leaves the interface
   unaddressed.

   *Rejected:* a `/v2/claim` path with `/v1` kept (a second handler for one
   optional field, pre-1.0). A `version` field in the claim (the decoder's
   refusal of unknown fields already makes skew loud). Starting a
   `specs/relay-claim/v1.md` contract document now (worth doing before 1.0
   for third parties, but it documents the whole protocol, not this field; a
   follow-up, Decision 10).

4. **The relay owns its build interface's address; the init keeps the rest
   of the relay VM's network setup** (questions a12, a13, a17; RELAY-04).

   - **vm mode** (qemu, Firecracker, Hyper-V). What stays in
     `tools/relay-vm/init`: module loading, picking the build interface (the
     first non-`lo` entry of `/sys/class/net`) and the upstream interface,
     `ip link set <build-if> up`, the whole upstream setup (DHCP or static
     NAT address, default route, resolvers), IPv6 off, `ip_forward`, and
     every iptables rule. None of those rules names an address: REDIRECT of
     80/443/53 and the INPUT accept of 8300 and drops of 8301 and ICMP match
     `-i <build-if>`, MASQUERADE matches `-o <upstream-if>`, FORWARD is policy
     DROP. So they are in place before the relay starts and need nothing from
     the claim. What moves to the relay: the build interface's address. The
     init stops running `ip addr add 10.0.0.1/30` and passes
     `-build-if <name>` to `/sbin/relay -mode vm`. On a valid claim, inside
     the start callback and before replying, the relay refuses a link that
     overlaps an address or route on any other interface (the upstream NAT,
     say), flushes the build interface's IPv4 addresses, adds `relay/len`
     through netlink (`golang.org/x/sys/unix`, already a dependency; a
     `linux`-tagged file with a stub elsewhere, GLOBAL-07), and starts the
     relay with that address as its self address. Today vm mode detects its
     self address as the first non-loopback IPv4 address, which only works
     because the init assigned the build interface first. Without
     `-build-if`, the relay picks the build interface by the init's own rule,
     so a relay VM booted from an older base initramfs works: its init still
     adds `10.0.0.1/30`, and the relay flushes it. The qemu and Firecracker
     relay initramfs therefore need no rebuild; only Hyper-V's VHDX, which
     bakes the relay binary, needs a refresh (Decision 6).
   - **host mode** (vz). The relay builds its gVisor netstack, ARP answers,
     DHCP responder, channel listeners and DNS socket from the claim's
     `build_link` instead of from `--subnet` at startup. Nothing reaches the
     netstack before the claim, because the build VM boots after it, so
     moving its construction into the claim step changes no observable order.
     The vz driver stops passing `--subnet`; the flag stays as the fallback
     for an orchestrator that sends no link, with its default unchanged.
   - **container mode.** Out of scope (question a17). The container driver
     already allocates a `/29` per build from `100.64.87.0/24` under a lock,
     so it never had the collision this feature fixes. The relay's address is
     assigned by the container runtime (`--ip`) on a bridge the runtime owns,
     and the relay container cannot change it. Its relay image is pulled by
     tag with no version pin to warden, so a new claim field would add a
     version-skew failure there for no gain. A container-mode relay refuses a
     claim that carries `build_link`. The cross-driver SSRF question (container
     and vm modes have no upstream filter) is deferred (Decision 10).
   - **Ordering.** The relay configures the link inside the claim's start
     callback, and `relaycfg.Serve` writes the reply only after the callback
     returns. So when `driver.Session.ClaimRelay` returns, the build segment is
     addressed. Every VM driver already boots the build VM only after
     `ClaimRelay` returns: qemu `startAndClaimRelay` then `startBuildVM`;
     Firecracker `startRelay` then `startBuild`; Hyper-V `StartRelay` (which
     claims) then `CreateVM` and `StartVM` for the build VM; vz
     `setupNetworkAndRelay` (which claims) then `bootBuildVM`. No driver warms a
     relay or boots the build VM in parallel. A tier-0 test per driver pins
     this order where the driver's orchestration is unit-testable (Hyper-V's
     `runBuild` over its `Provisioner`; the others through their run
     structs), and the `relaycfg` contract states it. Isolation does not
     depend on the order (BOOT-01): a build VM booted early would still have
     only the one NIC to the relay, just no relay address to reach.

   *Rejected:* keeping the address in the init, from a kernel command-line key
   (r2; a2 was answered (c)). Exec'ing busybox `ip` from the relay (it ties
   the relay binary to the initramfs's tools; netlink is a few dozen lines of
   pure Go). Keeping `--subnet` as host mode's channel and leaving the claim
   field to vm mode only (two channels for one value, and the relay contract
   would differ by mode; question a13 option (b)).

5. **Each guest takes the link per build: seeds as today, the direct-boot
   inits from their kernel command line, and the vz guest by DHCP from the
   host relay** (question a14; BOOT-03, BOOT-06).

   - **Seeded guests** (Linux cloud-init on qemu and Firecracker's disk path;
     Windows on qemu and Hyper-V): the seed is rendered per build from the
     resolved link, so these need nothing new beyond the render.
   - **Direct-boot inits** (`tools/build-vm/init`, `tools/firecracker-vm/init`):
     the drivers add `warden.link_ip=<guest>/<len>` and
     `warden.link_gw=<relay>` to the build VM's command line, which both
     already write per build (qemu `-append`, Firecracker `boot_args`). The
     inits parse only those keys, accept only dotted quads and a numeric
     length, and never evaluate the command line, as the relay init does with
     its keys. They carry no default: with the keys missing or malformed the
     init says so on the console and stops, so a mistake is a clear console
     line and not a silent timeout. On Firecracker the command line is already
     recorded as the seed file `kernel-cmdline`, so the record shows the link.
   - **vz** (`warden-boot`, baked at prepare, D2): it stops writing the
     address. It runs `ipconfig set <if> DHCP`, waits for the lease (up to the
     60 s it already waits for an interface), reads the address, mask and
     router from it (`ipconfig getifaddr`, `ipconfig getoption <if>
     subnet_mask` and `router`), and goes on as today: the scutil IPv4 and DNS
     state and `exec warden-io initialize --gateway=<router> --ip=<address>`.
     The host relay's DHCP responder answers from the claim's link, offering
     the guest address with the relay as router and DNS server and an
     infinite lease, so a long build never renews. The vz image then carries
     no address at all.

   Neither a seed nor a DHCP lease is a new input into the build environment:
   both carry only addresses, as the seed's `network-config` already does
   (BOOT-02). BOOT-03 allows "static network addressing applied by the
   platform's own boot configuration" in the launcher. A lease from the relay
   is addressing by the platform's own DHCP client, but it is not static, so
   this is proposed as a BOOT-03 wording change with a Bootstrap comparison
   (BOOT-07) rather than assumed.

   *Rejected:* baking the resolved link into the vz image at prepare, with a
   record compared on each build and a re-prepare per value (r2's design; a3
   asks for records only where baking remains, and a guest-side channel
   removes this one). Baking only the default into the vz image and leaving
   vz out of the setting (the link shadows nothing upstream on vz, whose relay
   dials from the host, but the setting would then mean different things per
   driver, and a8's block would still need the link). A per-build seed disk
   for vz (it would also retire D2, so it is its own feature; Decision 10).
   DHCP for the direct-boot inits too (the relay VM would need a DHCP
   responder on its build interface: a new guest-facing listener in vm mode
   for a value the command line already carries).

6. **No link records. A one-time launcher stamp where a baked launcher must
   be replaced, checked before boot and in doctor** (questions a3, a15;
   UX-04). Since nothing bakes a link value, no asset needs a record compared
   with the setting. What can still be stale is a launcher built before this
   change, which bakes `10.0.0.x` and ignores the new channel. That shows up
   as `warden-io` timing out on `relay not ready`. So each launcher-carrying
   asset gets a stamp, `WARDEN_LINK_CHANNEL=1`, written beside it by the tool
   that builds it. The driver compares the stamp with
   `buildlink.Channel`. A missing or different stamp is a `driver.Fixable`
   naming the rebuild, raised before any VM boots, and `warden doctor`
   reports the same per driver. A later change to how a launcher reads the
   link bumps the constant.

   | Asset | Driver | Stamp | Rebuilt by (once per upgrade, never per link value) |
   |---|---|---|---|
   | direct-boot build initramfs | qemu (`tools/build-vm`) | `link-channel` in its output directory, or the release file name plus `.link-channel` | `tools/build-vm/build-initramfs.sh` |
   | direct-boot launcher initramfs | firecracker (`tools/firecracker-vm`) | same | `tools/firecracker-vm/build-initramfs.sh` |
   | relay boot VHDX (bakes the relay binary and init) | hyperv | `relay-boot.link-channel` beside `relay-boot.vhdx` (moved to `.prev` with it), or `<name>.link-channel` beside a `WARDEN_RELAY_BOOT_VHDX` override | `tools/relay-vm/hyperv/refresh-cache.sh` |
   | prepared macOS image (`warden-boot`) | vz | `link-channel` in the prepared image directory, written by both prepare paths | `sudo warden image prepare --driver vz`, which re-prepares an existing image in place (`ReprepareImage`; no IPSW restore) |
   | relay initramfs (aarch64, x86_64) | qemu, firecracker | none | none: an old init's `10.0.0.1` is flushed by the new relay (Decision 4) |

   The upgrade cost, per driver: qemu rebuilds its direct-boot initramfs
   once (only users of direct boot), Firecracker its launcher initramfs once,
   Hyper-V runs `refresh-cache.sh` once (it must anyway, since its VHDX
   bakes the relay binary, and its existing source stamp already reports the
   disk stale), and vz re-prepares once. After that, changing
   `[vm] build_link` needs nothing on any driver.

   *Rejected:* no stamp, documenting the rebuild only (a silent timeout for
   everyone who upgrades). A link record compared with the setting (r2; there
   is no baked value to compare). A general source-hash staleness check over
   every baked asset (the overview's second open question; deferred,
   Decision 10, because it is a policy for every asset, not this one value).

7. **Guards follow the resolved link, and the relay blocks it in every mode
   that has one** (questions a4, a8, confirmed in r2; a16).
   - Firecracker's resolver exclusion (PR #90) checks the resolved link
     instead of a constant: the relay VM still has a connected route for the
     link, so a resolver inside it would be shadowed. Its refusal (no
     resolver left) names both fixes: give the host a resolver outside the
     link, or set `[vm] build_link` to a `/30` that holds none of them.
   - `firecracker.yml` drops its "Off-host DNS for the relay" step, so the
     hosted runner's own VPC resolver `10.0.0.2` is used. That turns the
     original failure into the CI test for this fix. Its `pull_request.paths`
     gains `driver/buildlink/**` (TEST-02's conservative scoping). Its asset
     cache key is unchanged: no initramfs contains the link any more, and the
     key already covers both inits, so the changed inits rebuild once.
   - Hyper-V setup refuses a `--nat-prefix` that overlaps the resolved link
     (`[vm] build_link` or the default), before changing anything (UX-04).
     Per-build networking ignores `--nat-prefix` today (it always uses
     `192.168.240.0/20`, which the load-time check keeps disjoint), so this
     matters once the Hyper-V follow-up honours it (Decision 9). That also
     settles plan r2's note on re-checking a durable NAT at build start: not
     needed while builds use the default prefix.
   - **The relay refuses upstream dials into the link in vm and host modes**
     (a8 confirmed "block the subnet"; a16 extends it to vm mode now that
     the relay knows the link there too). `relay.Config` gains the build
     link's prefix. The relay's upstream dialer refuses any destination inside
     it, checked on the address actually dialed, in every mode where it is
     set and whether or not the SSRF filter is on; the host-mode SSRF filter
     keeps its other blocks, and its gateway-only `BlockedSelfIP` folds into
     the prefix. `detectUpstreamDNS` skips a nameserver inside the prefix as
     it already skips its own address. On vz this closes what an RFC 6598
     guest address opened (`10.0.0.2` was blocked as RFC 1918;
     `100.127.255.254` would go out the host's default route, which with
     Tailscale could be a real peer). In vm mode it stops a build from
     having the relay fetch from the build VM's own address and record the
     result as an upstream input. `relay` does not import `buildlink`; the
     prefix comes from the claim (or, without one, from the interface or
     `--subnet`). Container mode leaves it unset (a17).
   - Every VM driver refuses, at build start and in doctor, a resolved link
     that holds an address of a host interface (Decision 8). No host ever
     holds a link address by design, so one there is a real collision.

8. **The link is configurable as an advanced option: `[vm] build_link` in
   `warden.toml`, and `--build-link` on `build`, `shell` and `doctor`,
   defaulting to `100.127.255.252/30`** (questions a6, reversed in r1; a9 and
   a10, confirmed in r2; a20).
   - **Where.** `[vm] build_link = "100.127.255.252/30"`, beside the other
     VM-driver defaults, in `~/.config/warden/config.toml` or `./warden.toml`
     (project over user), and `--build-link <cidr>` over both (UX-07,
     ORCH-03). Long form only (UX-06), no environment variable (UX-07).
     `build` and `shell` refuse the flag for `--driver container`, like the
     other VM flags, and `doctor` refuses it there too (UX-05).
   - **The command list, trimmed** (a9: "Design trims the command list once
     the claim carries the link"; a20). r2 also put the flag on
     `setup` (Hyper-V) and `image prepare` (vz), because the Hyper-V VHDX and
     the vz image baked the link. Neither bakes it now. `image prepare` drops
     out entirely. `setup --driver hyperv` keeps only its `--nat-prefix`
     overlap refusal, checked against `[vm] build_link` or the default; a
     one-off flag there would check a value no later build is bound to.
   - **Validation at load and startup** (UX-08), as confirmed in a10.
     `buildlink.Parse` accepts an IPv4 CIDR with prefix exactly `/30` whose
     address is the network address (`100.126.0.1/30` is refused with "did
     you mean 100.126.0.0/30"). It refuses `0.0.0.0/8`, loopback
     `127.0.0.0/8`, link-local `169.254.0.0/16`, multicast `224.0.0.0/4` and
     class E `240.0.0.0/4`. `cmd/warden` then checks the link is disjoint
     from every range BuildWarden itself uses: the container base
     `100.64.87.0/24`, Firecracker's upstream `100.64.88.0/22`, Hyper-V's
     default NAT `192.168.240.0/20` and qemu's user-mode `10.0.2.0/24`, read
     from those packages' exported constants. `buildlink` takes the list as an
     argument and imports none of them. A config value is checked when the
     file loads and a flag when it is resolved, both before any driver work.
     The error is fixable and names the key or flag and a valid example.
   - **Upstream overlap at build start and in doctor** (UX-04). Each VM
     driver refuses a link that holds an address of any host interface.
     Firecracker also checks its host resolvers (Decision 7), and Hyper-V
     checks the NAT prefix at setup. A host network that merely contains the
     link (a `/10` CGNAT WAN, say) is a doctor warning, not a refusal: the link
     is shadowed only for destinations inside its four addresses.
   - **Doctor.** Every VM driver's doctor prints the resolved link and where
     it came from (`default`, `warden.toml`, `config.toml` or `--build-link`),
     the host-overlap result and each launcher stamp's state (Decision 6).
   - **No record of the value in the ledger or environment record**, beyond
     Firecracker's existing record of the direct-boot command line. The link
     is orchestrator topology (GLOBAL-08), and the build's view of it is its
     own address.

   *Rejected:* a fixed link (r1). A new `[advanced]` table. Key only, no flag.
   An environment variable (UX-07). Accepting any prefix length (the relay
   and guest need exactly two hosts, and a wider link only widens what it can
   shadow). Keeping `--build-link` on `setup` and `image prepare` (r2's
   reason, the bake, is gone).

9. **Hyper-V's fixed relay address, missing claim token, ignored
   `--nat-prefix` and fixed upstream DNS are deferred to one follow-up**
   (question a18). The overview found that every Hyper-V relay takes
   `192.168.240.2`, the claim has no token, `--nat-prefix` is accepted at
   setup but ignored per build, and the relay's upstream DNS is `8.8.8.8`. All
   four have one cause: the relay VM's UKI command line is fixed, so Hyper-V
   has no per-build channel into the relay VM *before* the claim, and the
   claim cannot carry what is needed to reach and authenticate it. This
   feature removes the link from that list, since the claim carries it, but
   it does not create a pre-claim channel. The follow-up (a new intake item,
   `hyperv-relay-per-build-channel`) designs one channel and fixes the four
   together: a per-build relay address on the NAT, a claim token,
   `--nat-prefix` and `--host-ip` honoured per build (with the durable NAT's
   prefix re-checked against the link), and resolvers from the host. It also
   takes the overview's finding that the durable strategy reuses one Private
   switch (`warden-dev`) across builds, so concurrent durable builds share a
   segment. Here, Hyper-V gets only what the link needs: the claim field, the
   VHDX stamp, and the `--nat-prefix` overlap refusal (Decision 7). The
   Security note weighs the token.

10. **The overview's open questions, each answered or deferred.**

    | Open question (overview) | Here |
    |---|---|
    | A per-build channel (kernel command line, seed or claim) as the rule for every relay and guest value? | For the build link, yes: the claim for the relay, and the seed, command line or DHCP lease for each guest (Decisions 2 to 5). As a rule for every value (upstream addressing, DNS, the token), deferred: Hyper-V is the driver without a pre-claim channel (Decision 9), and the rule itself would be a standards change. Proposed as an optional BOOT-06 extension below, for the review to take or leave |
    | One general staleness check for every baked asset? | Not here. This feature adds only the launcher stamp its own change needs (Decision 6), written so a later general check can absorb it. The general check (a source digest per baked asset, read by build and doctor, including the Hyper-V source stamp that only `refresh-cache.sh` reads today) is a follow-up item, `baked-asset-staleness` |
    | Hyper-V: fixed `.2`, no token, `--nat-prefix` ignored, DNS `8.8.8.8`? | Deferred together to `hyperv-relay-per-build-channel` (Decision 9) |
    | Container model (range plus allocation) or Firecracker's (fixed per slot) for the link? | Neither: one link per isolated segment, configurable. Every VM build has its own L2 segment (a socket netdev, a per-slot bridge, a per-build switch), so the same two addresses can repeat without clashing, and an allocator would buy nothing. The exception is Hyper-V's durable shared switch, which is a segment problem, not an addressing one (Decision 9). The container driver keeps its allocation (Decision 4) |
    | SSRF and MSS clamp consistent across drivers? | The build-link block is now the same in vm and host modes (Decision 7). A shared upstream filter for vm and container modes (RFC 1918, metadata, link-local), and an MSS clamp outside Firecracker, are deferred: both change what builds can reach upstream on three drivers, which is a security-suite question (security-suite is in Design) |

    Two more, found while answering these: a `specs/relay-claim/v1.md`
    contract document for third-party orchestrators (Decision 3), and a vz
    per-build seed disk that would retire D2 (Decision 5). Both are recorded
    as follow-ups, not designed here.

## Questions

### Blocking

None. Every open question below has a recommended answer the design proceeds
on.

### Assumed

Confirmed in earlier reviews, recorded for the history:

- **a1. The default range.** Confirmed in r1: `100.127.255.252/30` (relay
  `.253`, build VM `.254`), with the link configurable (a6).
- **a2. How the link reaches each consumer.** Answered (c) in r2: deliver the
  link in the relay claim; the relay configures its own build interface, so
  no relay-side asset bakes the link and Hyper-V's rebake goes away
  (Decisions 2 to 4).
- **a3. Link records.** Answered in r2: records only where something still
  bakes the link. Nothing does after a14, so there are no link records; a15
  is the one-time launcher stamp that takes their place for the upgrade
  (Decision 6).
- **a4. Guards.** Confirmed (a) in r2: Firecracker's resolver exclusion
  follows the resolved link, the CI DNS workaround goes, the Hyper-V
  `--nat-prefix` overlap refusal and the host-interface overlap refusal on
  every VM driver (Decision 7).
- **a5. macos-qemu tracked, not blocking.** Confirmed (a) in r2: backlog
  `build-link-subnet-qemu-windows`.
- **a6. A configuration knob.** Reversed in r1: the link is configurable in
  the advanced options.
- **a7. Seams and relationships.** Superseded in r2 by a2 (c); re-decided as
  a19.
- **a8. The host-mode relay blocks its subnet.** Confirmed (a) in r2, "block
  the subnet", with leave to apply it in every mode; the extension is a16.
- **a9. Where the setting lives.** Confirmed (a) in r2: `[vm] build_link`
  plus `--build-link`, with the command list trimmed once the claim carries
  the link; the trimmed list is a20.
- **a10. Validating the setting.** Confirmed (a) in r2 (Decision 8).

New in r3, each "proceeding with the recommendation; confirm":

- **a11. The claim schema, versioning and compatibility** (Decision 3).
  Proceeding with an optional `build_link {prefix, relay, guest}` on
  `/v1/claim`, validated in `relaycfg` (IPv4, length 8 to 30, two distinct
  hosts inside it), refused by a container-mode relay, required by a vm-mode
  relay whose build interface has no address, with `--subnet` as host mode's
  fallback. An old relay refuses the field (its decoder rejects unknown
  fields), which warden maps to a fixable error; the contract document is
  the `relaycfg` package doc and architecture.md's "Claim protocol".
  Options: (a) as stated; (b) a `/v2/claim` path with `/v1` kept; (c) a
  version field in the claim; (d) (a), and start `specs/relay-claim/v1.md`
  now. Recommended: (a).
- **a12. vm mode: what stays in the init and what moves to the relay**
  (Decision 4). Proceeding with the init keeping interface selection,
  link-up, upstream setup, IPv6 off, `ip_forward` and every iptables rule
  (all interface-keyed), and passing `-build-if`. The relay, on a valid
  claim and before replying, refuses a link overlapping its other
  interfaces, flushes the build interface's IPv4 addresses, adds
  `relay/len` through netlink, and uses it as its self address. Without
  `-build-if` it applies the init's rule, so old qemu and Firecracker relay
  initramfs need no rebuild. Options: (a) as stated; (b) the relay execs
  busybox `ip` instead of netlink; (c) the relay adds its address without
  flushing (an old init's `10.0.0.1/30` would stay, and with it the original
  shadow). Recommended: (a).
- **a13. host mode: the claim, not `--subnet`** (Decision 4). Proceeding with
  the host-mode relay building its netstack, ARP, DHCP and listeners from the
  claim's `build_link`, the vz driver no longer passing `--subnet`, and
  `--subnet` kept as the fallback. Options: (a) as stated; (b) keep
  `--subnet` as host mode's only channel and send no `build_link` on vz; (c)
  both, the relay refusing a claim whose link differs from `--subnet`.
  Recommended: (a).
- **a14. The guest side** (Decision 5; Bootstrap comparison). Proceeding with
  seeds rendered per build as today, `warden.link_ip` / `warden.link_gw` on
  the direct-boot kernel command line (no default in the inits), and vz
  `warden-boot` taking a DHCP lease from the host relay's existing responder
  (infinite lease), so no guest asset bakes an address; BOOT-03's wording
  gains a DHCP lease served by the relay. Options: (a) as stated; (b) vz
  bakes the resolved link at prepare with a record and a re-prepare per
  value (r2); (c) vz bakes only the default and ignores the setting; (d) (a),
  plus DHCP for the direct-boot inits from a new vm-mode responder.
  Recommended: (a).
- **a15. The launcher stamp** (Decision 6). Proceeding with
  `WARDEN_LINK_CHANNEL=1` beside the qemu and Firecracker direct-boot
  initramfs, the Hyper-V relay boot VHDX and the vz prepared image, compared
  with `buildlink.Channel` before any VM boots and in doctor, each refusal
  naming its one-time rebuild, refresh or re-prepare; none on the qemu and
  Firecracker relay initramfs. Options: (a) as stated; (b) no stamp, a
  documented one-time rebuild; (c) a general source-digest staleness check
  over every baked asset instead. Recommended: (a), with (c) as the
  follow-up `baked-asset-staleness`.
- **a16. The link block in vm mode too** (Decision 7). Proceeding with the
  relay refusing upstream dials (checked on the dialed address) and
  upstream resolvers inside the build link in vm and host modes, whether or
  not the SSRF filter is on; container mode unchanged. Options: (a) vm and
  host modes; (b) host mode only (r2's a8); (c) every mode, the container
  driver sending its `/29` for the block alone. Recommended: (a).
- **a17. The container driver stays out** (Decision 4). Proceeding with the
  container driver keeping its per-build `/29` allocation, sending no
  `build_link`, and a container-mode relay refusing one: the runtime owns the
  relay's address, the driver never had the collision, and its relay image
  has no version pin to warden. Options: (a) out of scope; (b) send its `/29`
  in the claim for the a16 block only. Recommended: (a).
- **a18. Hyper-V's fixed relay address, token, `--nat-prefix` and DNS**
  (Decision 9). Proceeding with deferring all four, with the durable shared
  switch, to a follow-up item `hyperv-relay-per-build-channel`, queued at
  intake after this review. This feature adds only the claim field, the VHDX
  stamp and the `--nat-prefix` overlap refusal. Options: (a) defer to the
  follow-up; (b) add a claim token now through a per-build channel into the
  relay VM (a seed disk or KVP), which would also fix the address and
  `--nat-prefix`; (c) defer with no follow-up. Recommended: (a).
- **a19. Seams and relationships** (supersedes a7). Proceeding with
  `seams: [relay-claim, relay-guest-api, bootstrap-model]`. `relay-claim`:
  the new field. `relay-guest-api`, conservatively: the relay's address on
  the build segment comes from the claim, and the host-mode DHCP lease
  becomes the vz guest's address, though no endpoint's protocol changes.
  `bootstrap-model`: BOOT-03's wording (a14). `executor-initialize` is
  unchanged: `--gateway` and `--ip` keep their meaning and only their values
  change. `alters_relationships: true`: a versioned seam and the bootstrap
  model change, and the relay takes over its build interface's addressing
  from the relay VM init (FLOW-02). The FLOW-05 slot is free today, since
  no other in-progress feature declares `true`. Options: (a) as stated; (b)
  `[relay-claim]` only, if a14 (b) or (c) is chosen (no BOOT change and no
  live DHCP lease); (c) (a) plus `executor-initialize`, conservatively.
  Recommended: (a).
- **a20. The trimmed command list** (Decision 8). Proceeding with
  `--build-link` on `build`, `shell` and `doctor` only. `image prepare` drops
  it, since the vz image bakes no link. `setup --driver hyperv` checks
  `--nat-prefix` against `[vm] build_link` or the default, with no flag of its
  own. Options: (a) as stated; (b) keep it on `setup` for the overlap check;
  (c) the key only, no flag anywhere. Recommended: (a).

## Design

### The definition (`driver/buildlink`)

```
driver/buildlink/buildlink.go
  const DefaultCIDR = "100.127.255.252/30"
  const Channel     = 1                     launcher stamp value (Decision 6)
  type Link {Net netip.Prefix; Relay, Guest netip.Addr}
  Default() Link                            DefaultCIDR, validated at init
  Parse(cidr) (Link, error)                 Decision 8's syntactic checks
  (Link) Disjoint(named []Range) error
  (Link) HostOverlap(addrs []netip.Prefix) (refuse, warn []netip.Prefix)
  (Link) Claim() *relaycfg.BuildLink        the claim field
  (Link) BootArgs() string                  "warden.link_ip=<guest>/30 warden.link_gw=<relay>"
  (Link) GuestCIDR(), Mask()
  ReadStamp(path) (int, error)              missing -> ErrNoStamp
```

`buildlink` imports `relaycfg` (a leaf) and nothing from `driver`, so each
caller wraps its errors in `driver.Fixable` with its own fix text. The
`relay` package imports neither.

### The claim (`relaycfg`)

```
type BuildLink struct {Prefix, Relay, Guest string}   json: prefix, relay, guest
Config.BuildLink *BuildLink                            json: build_link, omitempty
(BuildLink) Parse() (netip.Prefix, relay, guest netip.Addr, error)   Decision 3's rules

handleClaim:
    decode (DisallowUnknownFields, as today)
    if cfg.BuildLink != nil and Parse fails: release; 400 "invalid claim: build_link: ..."
    start(cfg, transport)          the mode configures the link here (below)
    reply Identity                 only now: the segment is configured
```

### The relay (`cmd/relay`, `relay`)

```
relay -mode vm [-build-if NAME]:
    accept ONE claim on :8301; in its start step:
        if = NAME, else the first non-lo entry of /sys/class/net
        if claim.build_link:
            refuse if prefix overlaps an address or route on another interface
            flush IPv4 on if; add relay/len (netlink)
            self, block := relay, prefix
        else if if has an IPv4 address A/len:  self, block := A, A/len
        else: refuse "build_link is required: the build interface has no address"
        relay.Start(Config{SelfIP: self, BuildLink: block, ...})

relay -mode host --fd 3 [--subnet CIDR]:
    accept ONE claim on --claim-addr; in its start step:
        link := claim.build_link, else --subnet (gateway .1, guest .2)
        netstack, ARP (relay only), DHCP (offer guest, router and DNS relay,
            infinite lease), listeners and udp/53 on relay
        relay.Start(Config{SelfIP: relay, BuildLink: prefix, SSRF: true, ...})

relay -mode container:
    a claim with build_link is refused; otherwise as today

relay.Start:
    upstream dialer: refuse a dialed address inside BuildLink (every mode it is set);
                     host mode's SSRF blocks as today, BlockedSelfIP folded into BuildLink
    detectUpstreamDNS: skip a nameserver inside BuildLink (and selfIP, as today)
```

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
every VM driver StartBuild, before any VM boots:
    link.HostOverlap(driver.HostPrefixes())    refuse -> Fixable(set build_link)
    launcher stamps (Decision 6)               missing/different -> Fixable(rebuild once)
    ... boot the relay; sess.ClaimRelay(..., link.Claim())
        old relay ("unknown field build_link") -> Fixable(the driver's relay fix)
    ... only then boot the build VM, its seed or command line from link

qemu:        direct-boot -append += link.BootArgs(); linuxguest / winguest seeds from link
firecracker: resolvers minus those in link; none left -> Fixable(both fixes)
             direct-boot boot_args += link.BootArgs(); linuxguest seed from link
hyperv:      relay-boot.link-channel stamp; winguest seed from link
hyperv setup: --nat-prefix overlaps the resolved link -> Fixable(another --nat-prefix)
vz:          no --subnet; <image>/link-channel stamp; the guest takes its lease
vz image prepare (both paths): DHCP warden-boot; write <image>/link-channel
container:   ClaimRelay(..., nil); unchanged
doctor (each VM driver): "build link <net> (<source>)", overlap result, each stamp's state
```

### Every site that changes

| Site | Today | After |
|---|---|---|
| `driver/buildlink/` (new) | — | the definition above, tests |
| `relaycfg/relaycfg.go` | no link | `BuildLink`, `Config.BuildLink`, validation before start (release on invalid), package doc: the field, the per-mode table, the ordering rule |
| `relay/relay.go`, `relay/proxy.go` | SSRF dialer in host mode only; `BlockedSelfIP` | `Config.BuildLink` (a prefix) refused by the upstream dialer in every mode it is set; `BlockedSelfIP` folds into it; `detectUpstreamDNS` skips nameservers inside it |
| `cmd/relay/main.go` | vm and container modes share `serveClaim(relay.Config{})`; `--subnet` default `10.100.0.0/30` | a per-mode start step that reads the claim's link; `-build-if` (vm); container refuses `build_link`; `--subnet` documented as host mode's fallback, default unchanged |
| `cmd/relay/buildif_linux.go` (new), `buildif_other.go` | — | netlink flush and add, overlap check against other interfaces; stub elsewhere (GLOBAL-07) |
| `cmd/relay/mode_host.go`, `ingress_fd.go`, `ingress_dhcp.go` | netstack built at startup from `--subnet`; lease 3600 s | built in the claim step from the link; infinite lease; ARP and DHCP answer the claim's addresses |
| `tools/relay-vm/init` | `ip addr add 10.0.0.1/30` on the build interface; header comment | link up only; `-build-if "$PRIVATE_IF"`; header says the relay addresses the interface from the claim |
| `driver/session.go` `ClaimRelay` | builds the claim from the request | takes the build link to send (nil for none) and maps an old relay's unknown-field refusal to a `driver.Fixable` with the caller's fix |
| `driver/driver.go` `BuildRequest` | — | `BuildLink buildlink.Link` (zero = default) |
| `cmd/collector/main.go` | — | `-build-link <cidr>` |
| `driver/qemu` (`driver.go`, `vm.go`) | claim without link; direct-boot `-append` console only | claim carries the link; direct-boot `-append` adds `BootArgs()`; stamp check on the direct-boot initramfs |
| `driver/firecracker` (`driver_linux.go`, `vm.go`, `posture.go`, preflight, doctor, `slots.go` comment) | `buildLink` constant `10.0.0.0/30`; claim without link | the request's link in resolver selection and fix text; claim carries it; `buildConfig` boot args add `BootArgs()`; stamp check |
| `driver/relayvm/assets.go` | kernel + initramfs | plus the direct-boot initramfs stamp, from the same location `Find` chose |
| `tools/build-vm/init` + `build-initramfs.sh` | `10.0.0.2/30`, `via 10.0.0.1`, `nameserver 10.0.0.1`, `--gateway=10.0.0.1` | from `warden.link_ip` / `warden.link_gw`, no default; the script writes the stamp |
| `tools/firecracker-vm/init` + `build-initramfs.sh` | `10.0.0.2/30`, `via 10.0.0.1`, `--gateway=10.0.0.1` | same |
| `driver/linuxguest/seed.go` + `testdata/seed/` | literal addresses in network-config v2 and `runcmd` | rendered from the request's link; goldens for the default |
| `driver/winguest/winguest.go` (`BuildGatewayIP`, `BuildGuestCIDR`), `run.go` | string constants | derived from the request's link (the default kept under the old names for callers) |
| `driver/hyperv` (`relayhost.go`, `buildassets.go`, `setup.go`, `doctor.go`, `buildhost.go` comment) | claim without link; VHDX existence only | claim carries the link; VHDX stamp; `--nat-prefix` overlap refusal against the key; doctor lines |
| `tools/relay-vm/hyperv/refresh-cache.sh` (and `build-boot-vhdx*.{sh,ps1}` where they move the output) | installs `relay-boot.vhdx`, keeps `.prev` | also installs `relay-boot.link-channel`, moved to `.prev` with the disk; no `--link` (r2's) |
| `driver/vz` (`network_unix.go`, `driver.go`, `image.go`, `Preflight`) | literal link; relay `--subnet=10.0.0.0/30`; `warden-boot` with `GW`/`SELF`/`MASK` | link from the request; claim carries it, no `--subnet`; DHCP `warden-boot`; both prepare paths write the stamp; stamp check |
| `cmd/warden/config.go`, `main.go`, `host.go` | — | `[vm] build_link`; `--build-link` on build, shell, doctor; `resolveBuildLink`; container refusal; hyperv setup reads the key; doctor help text |
| `cmd/warden-vmnet/main.c` | default `--subnet=10.0.0.0/30` | `--subnet` required (no callers) |
| `tools/vz-image-prep/{personalize,uv-build-test,netfwd}.go` | literals for the prep-time link | `buildlink.Default()`; prep-time networking does not follow the setting (`netfwd` takes `--subnet`) |
| `cmd/warden-io` tests and `platform_windows.go` comments | `10.0.0.2/30` as parsing examples | other example addresses; behaviour unchanged |
| `.github/workflows/firecracker.yml` | DNS workaround step; paths | step removed; paths add `driver/buildlink/**` |
| `.github/workflows/integration.yml` | paths lack `relaycfg/**` | paths add `relaycfg/**` and `driver/buildlink/**`, since the container mode's claim handling changes (TEST-02) |
| Reference docs (UX-01): `guide/configuration.md` (Full Reference key, Advanced options section), `guide/cli.md` (`--build-link` on build, shell, doctor; doctor's link and stamp lines; `setup --nat-prefix` overlap refusal), `guide/drivers.md` (Upstream DNS; the one-time upgrade step per VM driver), `concepts/architecture.md` (Claim protocol: `build_link`, the ordering rule; the contract document, GLOBAL-08), `concepts/network-isolation.md` (slot table, the link block in vm and host modes), `contributing/structure.md` ("fixed 10.0.0.0/30"; `buildif_linux.go`), `tools/win-image-prep/README.md`, and `cmd/warden/host.go`'s doctor help | old link, fixed | default link, the setting, the claim field, the checks and the upgrade step |
| `concepts/overview.md` pseudocode (FLOW-15) | — | `ClaimRelay` pushes the build link; the relay block configures its build segment in the claim and blocks dials into it; each VM StartBuild resolves the link, checks stamps and host overlap before booting; vz's guest takes a DHCP lease |

Unchanged, and why: `driver/subnet.go` and the container driver (question
a17); Firecracker's upstream `100.64.88.0/22`, slots, TAPs and host rules (no
rule matches a link address); Hyper-V's NAT default `192.168.240.0/20` and its
per-build provisioning (Decision 9); qemu's user-mode `10.0.2.0/24` (all
three stay disjoint from the link, which the load-time check enforces); the
relay VM's iptables rules (interface-keyed); `specs/` and `ledger/` (no
address in any format); the qemu and Firecracker relay initramfs contents,
apart from the init text (old ones keep working); the `initialize` contract.

### Compatibility (RELAY-05, UX-03)

No ledger entry, metadata schema, environment record or spec names the link's
address. The relay's DNS answers for its reserved names (`artifacts`, `cwd`)
carry its own address, but they go only to the guest and are not recorded.
Environment records list layer digests (the direct-boot initramfs, the vz
`warden-boot` script) and, on Firecracker, the direct-boot command line.
Those change value once because the files change, which is what the records
exist to show; after that, the vz script's digest no longer varies with the
link. No format version changes. Old ledgers verify as before.

The claim's compatibility is Decision 3's table. The key and flag are
additive (UX-03). The upgrade cost is Decision 6's one-time step per driver,
which the stamps name.

## Cross-driver impact

- **qemu.** The claim carries the link and the relay addresses its build
  interface, so the relay initramfs needs no rebuild (an old one's
  `10.0.0.1/30` is flushed). Linux guests: the per-build cloud-init seed
  follows the link. Direct-boot builds: the command line carries it, and the
  direct-boot initramfs is rebuilt once to gain the reading init and its
  stamp. Windows guests: the per-build `warden-run.ps1` follows the link, and
  the prepared image is unchanged (BOOT-06).
- **firecracker.** As qemu: the relay initramfs needs no rebuild (CI
  rebuilds it anyway, since its cache key covers the init), the launcher
  initramfs is rebuilt once, and the disk path's seed moves through
  `driver/linuxguest`. The resolver exclusion follows the resolved link.
  Slots, TAPs and host rules don't change. Every slot still reuses the same
  two addresses, kept apart by conntrack zones (firecracker-driver
  Decision 4).
- **hyperv.** The relay boot VHDX is refreshed once with
  `refresh-cache.sh`, for the new relay binary and init (the stamp refuses
  an old disk); after that a changed setting needs nothing. The claim
  carries the link, the relay addresses `hv_netvsc`'s build NIC, and the
  Windows guest's static address comes from the per-build seed. New: the
  `--nat-prefix` overlap refusal in setup. Not changed here: the fixed relay
  address, the missing token, the ignored `--nat-prefix` and the fixed DNS
  (Decision 9).
- **vz.** The claim carries the link; the host relay builds its netstack from
  it and serves the guest a DHCP lease. The prepared image's `warden-boot`
  takes that lease, so a vz image is re-prepared once (D2, in place) and
  never again for the link. The host-mode relay blocks dials into the link.
- **container.** Behaviour unchanged. Its driver keeps its per-build `/29`,
  sends no `build_link` and does not import `driver/buildlink` (ORCH-01); a
  container-mode relay refuses the field. It is declared because
  `relaycfg/`, `relay/` and `cmd/relay` change under it (FLOW-02): tier 1
  covers its claim path. `--build-link` with `--driver container` is refused
  like the other VM flags, and `[vm] build_link` is ignored, like
  `[vm] cpus`.

## Bootstrap comparison

Required because the guest side changes BOOT-03's wording (a14) and the relay
takes over a step the relay VM init did before the guest boots (a12). Sign-off
must accept this section explicitly (BOOT-07).

### Current model

The diagram in [`bootstrap.md`](../development/bootstrap.md), unchanged on
main: (1) isolate, the sole network path is the relay, which exists before
any guest code runs; (2) deliver the executor per build; (3) start it, and
everything else over the relay. In step (1) today, the VM drivers' relay VM
init addresses the build link at boot, from a literal; vz's host relay takes
it from `--subnet`. Each launcher applies a static address: cloud-init
`network-config` and `warden-run.ps1` from the per-build seed, and the
direct-boot inits and vz `warden-boot` from literals baked into them.
Per-driver mapping rows: qemu Linux, firecracker Linux, qemu and hyperv
Windows, vz macOS, container. Deviation register: D2 (vz).

### Proposed model

The same diagram and mapping rows; the deviation register gains nothing. Two
things change in how steps (1) and (3) are carried out:

```
  (1) isolate ------------------------------>  sole network path: the relay
      claim the relay with the build link;     (topology unchanged; the relay
      the relay addresses the segment and      addresses its side before it
      replies; only then boot the guest        answers the claim)

  (3) start -------------------------------->  launcher: address from the seed,
                                               the kernel command line, or a
                                               DHCP lease served by the relay;
                                               then `warden-io initialize`
```

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | Topological for VM drivers; the relay VM init addresses the link at boot | Unchanged. The topology is the same; the relay's address on the segment now exists from the claim, and every VM driver already boots the guest only after the claim returns. Isolation does not depend on the address: a guest booted early has the same single NIC to the relay, only nothing to reach | none to isolation; ordering is pinned by tier-0 tests |
| BOOT-02 the executor is the only out-of-band input | Seeds, `CIDATA`, tar copy, the direct-boot initramfs; vz baked (D2) | Unchanged. The direct-boot command line gains two address keys, which, like a seed's `network-config`, are addressing, not material; the vz lease carries only addresses | qemu and firecracker direct boot: two keys; vz: a lease |
| BOOT-03 one start, then hands off | "Static network addressing applied by the platform's own boot configuration (cloud-init `network-config`, a fixed address on a single NIC) is allowed in the launcher" | Addressing applied by the platform's own boot configuration, static or from a DHCP lease served by the relay, is allowed in the launcher; trust, DNS and every other setup step still belong to `initialize`. The host still performs no step inside the environment | vz: `warden-boot` takes a lease instead of a literal; others unchanged |
| BOOT-04 build inputs come from the relay | CA, script and context fetched by the executor | Unchanged | none |
| BOOT-05 setup is the executor's domain | Baseline plus `/bootstrap` policy | Unchanged. The lease's DNS option names the relay, as `warden-boot`'s own DNS lines already do; `initialize` still owns DNS | none |
| BOOT-06 images carry hooks, not content | Generic hooks; but the vz image and both direct-boot initramfs carry the link literals, a value the orchestrator chooses | Closer: no image or initramfs carries an address. vz still carries `warden-io` and its launcher (D2) | vz, qemu and firecracker direct boot: no address baked; a one-time rebuild to get there |

### Gains

No boot asset or prepared image bakes the link, so changing it never
rebuilds, refreshes or re-prepares anything, and Hyper-V's per-value rebake
(r2) is gone. The relay learns its build segment from one place in every
mode that has one, which is what lets the build-link block apply in vm mode
too. The vz image moves toward BOOT-06. What the build consumed is recorded
as before: the Firecracker command line gains the two keys, and the vz
script's digest stops varying with the link.

### Costs

The relay gains a privileged step in vm mode: it changes its own interface's
address from claim data. That is bounded (one interface the init names, IPv4
addresses only, no routes or rules, refused when it overlaps another
interface), but it is new relay code on the security boundary (Security
note). vz now depends on the host-mode DHCP responder, which goes from unused
to live: its parsing is exposed to the guest, and DHCP and ARP spoofing join
the vz TEST-08 cases. On macOS, a DHCP lease is a platform behaviour this
design has not yet observed against this responder (personalize's DHCP phase
ran against vmnet NAT, not the netstack), so macos-vz validation is blocking.
The record loses nothing.

### Proposed standards change (FLOW-14)

Not edited here. For the design review to accept explicitly, and then to
land in this feature's diff or as its own process item if the review
prefers (as `bootstrap-firecracker-row` did for firecracker-driver):

1. `bootstrap.md`, BOOT-03: replace "Static network addressing applied by the
   platform's own boot configuration (cloud-init `network-config`, a fixed
   address on a single NIC) is allowed in the launcher" with "Network
   addressing applied by the platform's own boot configuration (cloud-init
   `network-config`, a fixed address on a single NIC, or a DHCP lease served
   by the relay) is allowed in the launcher".
2. Optional, for the review to take or leave (the overview's first open
   question): BOOT-06 gains "A boot asset or image carries no address or
   other per-build network value; those reach the relay by the claim and the
   guest by its seed, command line or the relay's DHCP". It holds after this
   feature for the build link, but not yet for Hyper-V's relay VM (Decision
   9), so taking it now would add a deviation-register entry for Hyper-V.
   Recommended: leave it for `hyperv-relay-per-build-channel`.

## Security note

**Boundary.** env-relay (GLOBAL-06): the build link is the only path between
the build environment and the relay, and this is a change to the relay
itself: it now configures that link from the claim (vm and host modes),
refuses upstream dials into it (vm and host modes), and serves the vz guest
a DHCP lease (host mode). RELAY-04: vm and host modes change; container mode
changes only in refusing the new field.

**Posture.** Meant to be unchanged or tighter. Isolation stays topological: one
NIC on a private segment (socketpair, socket netdev, private switch,
address-less bridge), and in the relay VM, REDIRECT of 80/443/53 on the build
interface, MASQUERADE on the upstream interface and FORWARD DROP. None of
those rules matches on the link's address, and the init still installs them
before the relay starts, so moving the address into the relay changes no
rule and opens no window: before the claim the build VM does not exist, and
the claim port is dropped on the build interface.

**Who can send the claim, and what it now controls.** The claim always
controlled everything about a build (the collector, so every input the build
receives). It now also sets the relay's build-segment address and the
blocked prefix. Who can reach the claim port before the orchestrator's claim
lands:

| Driver | Claim endpoint | Reachable by | Token |
|---|---|---|---|
| qemu | relay `:8301` via user-mode `hostfwd` on a free `127.0.0.1` port | local processes | per build, on the relay's kernel command line, which is in qemu's argv and so readable by local users |
| firecracker | the slot's relay address on `wfc<i>u` | only the slot owner's uid (host OUTPUT rule) | per build, in the relay's boot args |
| vz | `127.0.0.1:<free>` | local processes | per build, in the relay's environment |
| container | published `127.0.0.1:<free>` | local processes | per build, in the container's environment |
| hyperv | `192.168.240.2:8301` on the Internal NAT switch, whose host vNIC is on the host | local processes, and other VMs on that switch (other builds' relay VMs, never a build VM) | **none**: the UKI's command line is fixed |

Bounds on what a claim can do with `build_link`: `relaycfg` validates it
(IPv4, length 8 to 30, two distinct hosts inside); the relay changes only the
build interface the init named, only its IPv4 addresses, and adds no route
or rule; it refuses a link that overlaps an address or route on any other
interface, so a claim cannot shadow the relay's upstream or its NAT gateway.
The orchestrator boots the build VM only after its own claim succeeded, so a
claim that someone else won leaves the build VM unbooted.

**Hyper-V's missing token, weighed.** Without a token, a local process that
reaches `192.168.240.2:8301` first can claim the relay. Today and after this
change, the outcome is the same: the orchestrator's own claim (or tunnel) is
refused with 409, `StartRelay` fails, and the build VM is never created. So
the race is a denial of service, not a way to feed a build. Carrying the link
in the claim does not widen it: the winner could set the relay's build
address, but no build VM ever attaches to that relay. The same holds for two
concurrent Hyper-V builds whose relays share `.2`: one orchestrator may claim
the other's relay, and both builds fail. A token needs a value in the relay
VM before the claim, which the fixed UKI cannot take, so adding one means
designing a pre-claim channel (a seed disk or KVP), and that channel is what
also fixes `.2` and `--nat-prefix`. Recommended: defer the token to
`hyperv-relay-per-build-channel` (a18), which is queued with this risk named.

**What else the change could break, and what guards it:**

- *A stale address keeping the old shadow.* A relay VM booted from an older
  init still adds `10.0.0.1/30`; the relay flushes the build interface
  before adding the link, so the old connected route goes too.
- *The link overlapping a BuildWarden range or a host network.* Refused at
  load (BuildWarden ranges), at build start and in doctor (host interface
  addresses), and by the relay itself (its other interfaces). Even then,
  Firecracker's anti-spoof rule and `MASQUERADE` limited to
  `-s 100.64.88.0/22` keep build-sourced packets from leaving.
- *A link that shadows something the build should reach.* A reachability
  failure, not an escape: packets for those four addresses stay inside the
  boundary. The docs say what the setting shadows.
- *Upstream dials into the link.* Refused in vm and host modes on the dialed
  address (a16), whatever range the user picks, so a build cannot have the
  relay fetch from the build VM's own address and record it as upstream, and
  on vz cannot reach a tailnet peer that happens to sit on the link. Dials to
  other `100.64.0.0/10` addresses keep today's behaviour, and every such fetch
  is recorded.
- *The vz DHCP responder, now live.* Its only peer is the guest's NIC on the
  socketpair. It makes one fixed offer per build from the claim, so a guest
  cannot obtain another address or move the relay's; a guest that changes its
  own address afterwards could do so today as root. Malformed packets reach
  its parser, so it gets fuzz tests.
- *The direct-boot command-line keys.* The inits accept only dotted quads and
  a numeric length and never evaluate the command line, as with the relay
  init's keys. The build cannot read or change the relay VM's command line.
- *Stale launchers.* Refused before boot by the stamp, so a mismatched pair
  never runs half configured.

**TEST-08 should exercise:** a claim whose `build_link` overlaps the relay's
upstream interface is refused and leaves the relay unclaimed; a second claim
after a valid one is refused, as today; a build that resolves a name to the
link's addresses is refused by the relay on qemu, Firecracker, Hyper-V and
vz, for the default and a configured link; a build-VM packet sourced from the
guest address and sent to an off-host canary is dropped (Firecracker's
anti-spoof case); on vz, a guest sending forged DHCP DISCOVER and REQUEST
floods and ARP replies claiming the relay's address gets no other address and
does not disturb the relay (the existing escape-vector list in
`vm-driver-release-plan.md`); a host resolver inside the link is refused on
Firecracker; a stale launcher and a configured link overlapping a
BuildWarden range or a host interface are each refused before any VM boots.

## Validation plan

- **Tier 0** (TEST-01: pure Go, table-driven, no host network).
  `driver/buildlink`: the default validates; `Parse` accepts it and refuses
  `/29`, `/31`, misaligned `100.126.0.1/30` (fix names `100.126.0.0/30`),
  IPv6, `0.0.0.0/30`, `127.0.0.0/30`, `169.254.1.0/30`, `224.0.0.0/30` and
  `240.0.0.0/30`; `Disjoint` against the four ranges read from their
  packages' constants in an external test package; `HostOverlap` refuse,
  warn and pass; `ReadStamp` missing, different and equal; `Claim()` and
  `BootArgs()` render. The stray-literal scan with its allowlist.
  `relaycfg`: `build_link` round-trips; each validation failure is a 400 that
  does not consume the relay; a claim without it decodes as today; an old
  decoder (a copy of today's `Config`) refuses it with the "unknown field"
  message that `ClaimRelay` maps. `cmd/relay`: the per-mode start step (vm
  with and without the field, with an addressed and an unaddressed
  interface, overlap with another interface refused; host from the claim
  and from `--subnet`; container refusal) over an interface stub, so the
  netlink file is the only untested part at tier 0; the DHCP reply built from
  the claim's addresses with an infinite lease; fuzzing of the DHCP parser.
  `relay`: the dialer refuses every address in the prefix with SSRF off and on,
  and passes another `100.64.0.0/10` address; `detectUpstreamDNS` skips a
  nameserver in the prefix. `cmd/warden`: `resolveBuildLink` precedence (flag
  over project over user over default, the default with no config file,
  UX-07), a bad `[vm] build_link` failing at load (UX-08), `--build-link`
  refused for container; hyperv setup's overlap refusal against the key.
  Renders for the default and a configured link: seed goldens
  (`driver/linuxguest`), `winguest.WardenRunPS1`, the vz DHCP `warden-boot`
  (no address in it), and the qemu and Firecracker direct-boot command lines.
  Each driver's claim carries the link (nil for container). Stamp checks for
  each asset on temporary directories, each refusal a `driver.Fixable` whose
  fix names the one-time command, and the stamp taken from the same location
  as its asset. The order test per driver: no build VM boots before
  `ClaimRelay` returns. Firecracker resolver selection: a resolver inside the
  resolved link excluded, one at `10.0.0.2` now passing. All free of
  platform build tags except the netlink file's stub pair (GLOBAL-07).
- **Lint.** `make lint` for each platform touched (FLOW-17): linux for
  `cmd/relay/buildif_linux.go` and firecracker, darwin for `driver/vz` and
  `cmd/relay/mode_host.go`, windows for `driver/hyperv`, `cmd/warden-io`,
  `cmd/relay` (its stub) and `driver/vz/network_windows.go`.
- **Tier 1** (TEST-02): the `integration` workflow on the branch, since
  `relay/`, `relaycfg/` and `cmd/` change; it covers container mode's claim
  path end to end.
- **Tier 2, CI.** `firecracker.yml` without the DNS workaround: the hosted
  runner's VPC resolver `10.0.0.2` is used directly on the default link, the
  rebuilt relay and launcher initramfs boot, and the Ubuntu disk build and the
  direct-boot build succeed.
- **Tier 2, native (FLOW-09).** Blocking: linux-kvm, qemu-any, macos-vz,
  windows-hyperv, each with four cases. (1) Upgrade path: with the pre-change
  assets in place, the build and doctor refuse with the one-time command
  (direct-boot initramfs on linux-kvm and qemu-any, the image on macos-vz,
  the VHDX on windows-hyperv), and on linux-kvm and qemu-any a build on an
  old relay initramfs succeeds with `10.0.0.1` gone from the relay's build
  interface (console). (2) After the one-time command, a build on the
  default link succeeds, its ledger verifies, and the guest's address is
  `100.127.255.254` (console, `warden-run.log` or `warden-boot.log`). (3)
  With `[vm] build_link = "100.126.255.252/30"`, doctor reports it and a
  build succeeds on every host with no rebuild, refresh or re-prepare, the
  guest at `100.126.255.254`. (4) A `--build-link` overlapping a BuildWarden
  range is refused at startup. Plus per host: linux-kvm a direct-boot and a
  disk build, a host resolver inside the configured link excluded with the
  fix, the guest-sourced canary drop, and a build fetching the link's relay
  address refused; qemu-any a direct-boot and a cloud-image Linux build;
  macos-vz the lease taken within the boot window (time from boot to
  `initialize` against main), a build longer than an hour with no renewal,
  and the link block on the configured link; windows-hyperv a Windows guest
  build, an old VHDX refused by the stamp, and with the stamp forged, the
  claim's "unknown field" refusal mapped to the same fix, plus the
  `--nat-prefix` overlap refusal. Tracked: macos-qemu (a Windows guest on
  qemu, after `qemu-windows-guest-package`), backlog
  `build-link-subnet-qemu-windows`. Not run natively: container-any and
  macos-finch, since the container driver's behaviour is untouched and tier
  1 covers its claim path.
- **Queue steps.** The four `f6-validate-<host>` steps are not in the queue
  during Design. `wf back` at the r2 review removed them with
  `f2-design-review-r2`. Re-adding them now could only make them need
  phase-1 steps, which finish with Design, so the native machines would be
  offered them while Plan and design review r3 are still open; and `wf
  validate` refuses a need on `f2-design-review-r3` before Plan seeds it. As
  in r1 and r2, Plan (`f2-plan-r3`) re-adds them, each needing
  `f2-design-review-r3`; Implement adds `f3-implement` to their needs, and
  Publish makes `f6-open-pr` need all four.
- **Security (TEST-08).** The Security note's cases are covered at tier 0
  and in native validation. The Arm A corpus does not exist yet
  (security-suite is in Design), so Consolidation hands the Security note's
  list to it.

## Implementation plan

Plan r3 (phase 2, `f2-plan-r3`, 2026-11-04, attended), written for design r3
at `c19e8f0` after reading the code each site-table row names (branch
`c19e8f0`; `relay/relay.go` also on main `1f61b0c`, where only
`recordServed` differs). The r2 plan (39 tasks, design r2) is at `ef8c014`
and the r1 plan at `fe166f3`; both are superseded. Each task names the rules it
must satisfy and the open assumed questions it rests on (a11 to a20), so a
change to an answer at design review r3 changes the tasks that cite it; tasks
citing only confirmed questions (a1 to a10) or none rest on settled ground.
Tasks 8, 10, 13, 15, 24, 25, 30, 31 and 34 to 41 rework code that already
exists (FLOW-06). Tasks 44 and 45 are the reference-doc and `overview.md`
updates the change owes (UX-01, FLOW-15). Testing tasks name their tier
(TEST-02) and keep tier 0 pure Go, table-driven and free of host networking
(TEST-01).

**Budget** (FLOW-13). 52 tasks, all in one `f3-implement` step. Steps fit:
7 spent, and the rest of the item is about 18 to 25 claims (Implement over
three or four leases, review, guide, merge-main, consolidate, open-pr, four
native steps with one re-run each, close), well inside 60. Runs do not fit 5:
3 are spent, and an honest estimate of the rest is 3 for Implement (about 50
tasks over the relay, four drivers, three inits and the docs), 1 for review
and guide, 1 for Publish, 3 for native validation (the macbook and the
Windows desktop each batch their hosts, and macos-vz's first DHCP lease and
the experimental WSL2 `linux-kvm` make one re-run likely), and 1 to close:
about 12 runs. Recommended at the design review: raise `budget.estimate.runs`
to 12, before the runaway check (factor 2, so 10 runs) escalates mid-Implement.

**Plan notes for the design review** (FLOW-19). Planning found the design
workable as written. Where its text is silent or does not match the code, the
plan takes the reading below; the review confirms it or sends the design back:

- *"Overlaps an address or route on any other interface" (Decision 4, the relay
  pseudocode; GLOBAL-06, RELAY-04; a12).* Read literally this refuses every
  claim, because the relay VM's default route on its upstream interface
  (`0.0.0.0/0`, on qemu, Firecracker and Hyper-V alike) overlaps every link.
  The plan's reading (task 11): refuse when another interface holds an address
  inside the link, or has a route whose prefix lies inside the link or equals
  it; a route that strictly contains the link (the default route, a `/10`
  CGNAT network) is not a refusal, since the link's more specific connected
  route shadows only its own four addresses, as warden's host-side
  `HostOverlap` treats a containing network as a warning (Decision 8). The
  review should fix the design's wording to match, or choose a stricter rule.
- *Relay-side refusals leave the relay unclaimed (Decision 3, Security note's
  TEST-08 list; GLOBAL-08, UX-04; a11, a12, a17).* The design wants the
  overlap refusal, "build_link is required" and the container-mode refusal to
  leave the relay unclaimed, but those checks run inside the start callback,
  and `relaycfg.Serve` today answers a start error with 500, consumes the claim
  and returns, so the relay process exits. The plan adds one mechanism to
  `relaycfg` (task 6): a start callback may return an error wrapping
  `relaycfg.ErrRefused`, which `handleClaim` answers with 400 and releases, as
  it does a malformed claim. This is a small addition to the relay-claim seam
  beyond the field itself, stated in the package doc with the field.
- *Order tests for qemu and vz (Decision 4, "through their run structs";
  BOOT-01; a12).* Firecracker has a `run` struct and Hyper-V a `Provisioner`;
  qemu (`StartBuild` calling `startAndClaimRelay` then `startBuildVM`) and vz
  (`setupNetworkAndRelay` then `bootBuildVM`) have neither. Task 32 adds the
  smallest seam that makes the order testable (the relay-and-claim and the
  build-boot steps as function fields on the driver, defaulting to today's
  methods), with no behaviour change.
- *A stamp value written by shell, compared in Go (Decision 6; UX-04,
  TEST-01; a15).* The three build scripts write `WARDEN_LINK_CHANNEL=1` while
  warden compares `buildlink.Channel`. A later bump could miss a script, and
  the Firecracker CI cache key (which hashes the scripts, not Go) would then
  keep serving an old stamp. Task 28 adds a tier-0 test that reads each
  script and asserts it writes the constant's value; it goes beyond the
  design's list of tests, with no behaviour of its own.
- *The stray-literal scan and Go-form literals (Decision 2; GLOBAL-08,
  UX-01; a2 confirmed).* Whole dotted-quad matching misses
  `net.IPv4(10, 0, 0, 1)` (`driver/vz/network_unix.go`) and
  `net.IPv4(10, 0, 0, 0)` (`driver/firecracker/posture.go`). Both sites change
  in tasks 25 and 31 anyway; task 42 also matches the `IPv4(10, 0, 0, <n>)`
  form for the old link so it cannot return. The new default is matched in
  dotted form only.
- *Host-mode tests carry the `unix` tag (GLOBAL-07, TEST-01; a13).* The
  design says tier 0 is free of build tags except the netlink pair, but
  `cmd/relay`'s host mode (`mode_host.go`, `ingress_fd.go`,
  `ingress_dhcp.go`) is `unix`-tagged with a Windows stub, as is its existing
  `ingress_fd_test.go`. The host-mode start-step and DHCP tests (task 14)
  keep that tag; the vm-mode start step is untagged.
- *Carried from plan r2 (ORCH-01, GLOBAL-08, TEST-08).* Only Hyper-V's
  `DefaultNATPrefix` is an exported CIDR today, so task 2 exports the other
  three; `qemu.Preflight()`, `vz.Preflight()`, `firecracker.Doctor(out)` and
  `hyperv.Doctor(w, c)` are package functions, so passing them the link
  (task 23) changes no `Driver` method; one `driver.HostPrefixes()` keeps
  `buildlink` a leaf (task 17); `slots_test.go` names no link address, and the
  guest-sourced canary case runs natively (task 50).
- *Callers of `ClaimRelay` (GLOBAL-08, ORCH-01; a11, a17).* Six, not five:
  `cmd/collector` also claims through `driver.Session`. Task 15 updates all
  six; task 19 is the collector's `-build-link`.
- *The BOOT-03 wording change (BOOT-07, FLOW-14; a14).* The design leaves the
  choice to the review: this feature's diff or its own process item. Task 46
  is written for the first and is dropped if the review picks the second.

### The definition

1. Add `driver/buildlink` (`buildlink.go`): `DefaultCIDR =
   "100.127.255.252/30"`, `Channel = 1`, `Link {Net netip.Prefix; Relay,
   Guest netip.Addr}`, `Default()` (validated at package init), `Parse(cidr)`
   (IPv4, exactly `/30`, the network address with a "did you mean" fix; not
   `0.0.0.0/8`, loopback, link-local, multicast or class E), `Disjoint(named
   ranges)`, `HostOverlap(addrs)` (refuse: an address inside the link; warn: a
   network containing it), `Claim() *relaycfg.BuildLink`, `BootArgs()`
   (`warden.link_ip=<guest>/30 warden.link_gw=<relay>`), `GuestCIDR()`,
   `Mask()` and `ReadStamp(path)` (`ErrNoStamp` when missing; parses
   `WARDEN_LINK_CHANNEL=<n>`). Imports `relaycfg` and the standard library
   only, nothing from `driver`; errors are plain for callers to wrap.
   (GLOBAL-08, ORCH-01, ORCH-03, UX-08, GLOBAL-07; a11, a15)
2. Export one CIDR constant per range the link must avoid, beside the existing
   names: the container base `100.64.87.0/24` next to `driver.WardenBaseNet`,
   Firecracker's upstream `100.64.88.0/22` next to `upstreamNet`, and qemu's
   user-mode `10.0.2.0/24` (commented as qemu's built-in default); Hyper-V's
   `DefaultNATPrefix` is already exported. No behaviour changes. (ORCH-01,
   GLOBAL-08)
3. Tier-0 tests for `driver/buildlink` (TEST-02 tier 0): the default
   validates; `Parse` accepts it and refuses `/29`, `/31`, misaligned
   `100.126.0.1/30` (the fix names `100.126.0.0/30`), IPv6, `0.0.0.0/30`,
   `127.0.0.0/30`, `169.254.1.0/30`, `224.0.0.0/30` and `240.0.0.0/30`;
   `HostOverlap` refuses, warns and passes; `ReadStamp` missing, malformed,
   different and equal; `Claim()` and `BootArgs()` render for the default and
   `100.126.255.252/30`. (TEST-02, TEST-01, UX-08; a11, a15)
4. Tier-0 `Disjoint` test in an external test package that reads task 2's
   constants and `hyperv.DefaultNATPrefix` rather than restating them: the
   default is disjoint from all four, and one overlapping value per range is
   refused with the range named. (TEST-02 tier 0, TEST-01, ORCH-01)

### The claim (`relaycfg`)

5. `relaycfg.BuildLink {Prefix, Relay, Guest string}` (`prefix`, `relay`,
   `guest`) and `Config.BuildLink *BuildLink` (`build_link,omitempty`), with
   `(BuildLink) Parse()` applying Decision 3's rules: IPv4; `prefix` is its own
   network address, length 8 to 30; `relay` and `guest` distinct, inside it,
   neither its network nor broadcast address; none unspecified, loopback,
   link-local, multicast or class E. (GLOBAL-08, RELAY-05, UX-03, GLOBAL-06;
   a11)
6. `relaycfg.handleClaim`: after decoding (`DisallowUnknownFields` kept), an
   invalid `build_link` releases the claim and answers 400 `invalid claim:
   build_link: ...`; a start error wrapping the new `ErrRefused` releases and
   answers 400 instead of 500 (plan note). The package doc gains the field, its
   validation, Decision 3's per-mode table, `ErrRefused`, and the ordering
   rule "a relay replies to a claim only after its build segment is
   configured", which is the contract document GLOBAL-08 asks for. (GLOBAL-08,
   RELAY-04, RELAY-05, UX-04; a11, a12)
7. Tier-0 `relaycfg` tests (TEST-02 tier 0): `build_link` round-trips; each
   validation failure is a 400 that leaves the relay claimable (a valid claim
   then succeeds); an `ErrRefused` start error does the same; a claim without
   the field decodes and starts as today (**new relay, old warden**); a copy of
   today's `Config` decoded with `DisallowUnknownFields` refuses a claim that
   carries the field with `unknown field "build_link"` (**old relay, new
   warden**: the exact text task 15 maps); the reply is written only after the
   start callback returns (a callback that blocks on a channel holds the
   reply). (TEST-02, TEST-01, GLOBAL-08, UX-03; a11)

### The relay (`relay`, `cmd/relay`)

8. `relay` (rework): `Config.BuildLink netip.Prefix` replaces `BlockedSelfIP`.
   When it is set, the upstream transport's dialer refuses every dialed
   address inside it, checked on each resolved address before dialing, in
   every mode and whether or not SSRF is on; in host mode `isBlockedIP`'s
   other SSRF blocks stay. `detectUpstreamDNS` skips a nameserver inside the
   prefix as it skips `selfIP`, with the resolv.conf parsing split into a pure
   function so it is testable. `relay` imports neither `buildlink` nor
   `driver`. (GLOBAL-06, RELAY-04, GLOBAL-08; a16)
9. Tier-0 `relay` tests (TEST-02 tier 0): with the default and a configured
   prefix, each of its four addresses is refused with SSRF off and on, another
   `100.64.0.0/10` address is passed by the link check, and a zero prefix
   blocks nothing extra (container mode); the resolv.conf parser skips a
   nameserver inside the prefix. (TEST-02, TEST-01, TEST-08; a16, a17)
10. `cmd/relay/main.go` (rework): `serveClaim` takes a per-mode start step,
    `func(relaycfg.Config) (relay.Config, cleanup func(), error)`, run inside
    the claim callback before `relay.Start`; vm gains `-build-if NAME`;
    container refuses a claim carrying `build_link` with `ErrRefused` and
    otherwise behaves as today; `--subnet`'s help says it is host mode's
    fallback, default unchanged; the mode comment lists qemu, Firecracker and
    Hyper-V for vm. (RELAY-04, GLOBAL-08, UX-04; a12, a13, a17)
11. vm-mode start step (`cmd/relay/vmlink.go`, untagged), over a small
    `buildIface` interface (name, IPv4 addresses, flush, add, and the
    addresses and routes of every other interface): pick `-build-if` or the
    first non-`lo` entry of `/sys/class/net`; with a link, refuse with
    `ErrRefused` when another interface holds an address inside it or a route
    inside or equal to it (plan note), then flush the build interface's IPv4
    addresses, add `relay/len`, and set `SelfIP` and `BuildLink` from the
    claim; without one, use the interface's existing `A/len` for both; with
    neither, refuse "build_link is required: the build interface has no
    address". (RELAY-04, GLOBAL-06, BOOT-01, GLOBAL-07; a12)
12. `cmd/relay/buildif_linux.go`: the `buildIface` implementation over
    `golang.org/x/sys/unix` netlink (`RTM_GETADDR`, `RTM_DELADDR`,
    `RTM_NEWADDR`, `RTM_GETROUTE`), IPv4 only, no route or rule changes;
    `buildif_other.go` returns "vm mode needs Linux". (GLOBAL-07, GLOBAL-06,
    RELAY-04; a12)
13. Host mode (rework: `mode_host.go`, `ingress_fd.go`, `ingress_dhcp.go`):
    the FD ingress, gVisor netstack, ARP, DHCP responder, channel listeners and
    udp/53 are built in the start step from the claim's `build_link`, else
    from `--subnet` (gateway first host, guest second); ARP answers the relay
    address; DHCP offers the guest address with the mask from the prefix,
    router and DNS the relay, and an infinite lease (`0xffffffff`); the relay
    starts with `SelfIP` the relay address, `BuildLink` the prefix and SSRF on;
    the ingress closes in the step's cleanup. `mode_host_windows.go` keeps its
    refusal. (RELAY-04, GLOBAL-06, GLOBAL-07; a13, a14)
14. Tier-0 `cmd/relay` tests (TEST-02 tier 0): the vm step over a stub
    `buildIface`, with and without the field, on an addressed and an
    unaddressed interface, an old init's `10.0.0.1/30` flushed and replaced,
    an address or contained route on the upstream interface refused while a
    default route is not, and each refusal leaving the relay claimable through
    `relaycfg.Serve`; the host step from the claim and from `--subnet`; the
    container refusal; the DHCP OFFER and ACK built from the claim's addresses
    with an infinite lease; `FuzzHandleDHCP` over the responder's parser with a
    seed corpus, run as an ordinary test in `make test`. Host-mode tests keep
    the `unix` tag (plan note); only the netlink file is untested at tier 0.
    (TEST-02, TEST-01, TEST-08, GLOBAL-07; a12, a13, a14, a17)

### The orchestrator side of the claim

15. `driver/session.go` `ClaimRelay` (rework): takes the link to send
    (`*relaycfg.BuildLink`, nil for none) and the caller's old-relay fix text;
    sets `Config.BuildLink`; maps an `ErrRejected` whose message holds
    `unknown field "build_link"` to a `driver.Fixable` with that fix, returned
    before the caller boots the build VM. All six callers updated (qemu,
    Firecracker, Hyper-V, vz, container with nil, `cmd/collector`). (GLOBAL-08,
    UX-04, UX-03, ORCH-01; a11, a17)
16. `driver.BuildRequest` gains `BuildLink buildlink.Link`, the zero value
    meaning the default through one helper; the container driver ignores it
    and does not import `buildlink`. (ORCH-01, GLOBAL-08, UX-03)
17. `driver.HostPrefixes()` over `net.InterfaceAddrs`, the one host read that
    `HostOverlap` is given, so `buildlink` stays a leaf. (ORCH-01, GLOBAL-07)
18. Tier-0 session tests (TEST-02 tier 0): against an `httptest` relay that
    decodes with today's `Config`, `ClaimRelay` with a link returns a
    `driver.Fixable` carrying the caller's fix (**old relay, new warden**);
    against `relaycfg.Serve` with task 11's vm step over the stub, a claim
    with no link succeeds on an addressed interface and is refused with
    "build_link is required" on an unaddressed one (**new relay, old warden**,
    both rows of Decision 3's table); nil sends no field. (TEST-02, TEST-01,
    UX-03, RELAY-05; a11)
19. `cmd/collector -build-link <cidr>`: the relay the first host and the guest
    the second, validated by `relaycfg.BuildLink.Parse`, refused with the flag
    named; tier-0 test of the parse. (GLOBAL-08, RELAY-05, UX-04, TEST-02 tier
    0; a11)

### The setting (`cmd/warden`)

20. `config.go`: `[vm] build_link` in `VMConfig`; `LoadConfig` runs
    `buildlink.Parse` and `Disjoint` over task 2's ranges as the file loads,
    and a failure is a `driver.Fixable` naming the key, the file and a valid
    example. (UX-08, UX-07, ORCH-03, UX-04)
21. `--build-link <cidr>` (long form only) on `build` and `shell` (`main.go`)
    and `doctor` (`host.go`), its help marking it advanced; `resolveBuildLink`
    (flag over project over user over the default, with the source label
    `default`, `warden.toml`, `config.toml` or `--build-link`), checked before
    any driver work; `validateFlagsForDriver` refuses it for `container` on
    all three; the result goes into `BuildRequest` and doctor; `setup --driver
    hyperv` reads `[vm] build_link` or the default and has no flag. (UX-05,
    UX-06, UX-07, ORCH-03, UX-04, UX-08; a20)
22. Tier-0 `cmd/warden` tests (TEST-02 tier 0): precedence and source labels;
    the default with no config file; a bad `[vm] build_link` failing at load;
    an overlapping `--build-link` refused at startup; the flag refused for
    container on build, shell and doctor; no `--build-link` on `setup` or
    `image prepare`. (TEST-02, TEST-01, UX-07, UX-08, UX-05; a20)
23. Doctor: for each VM driver, `build link <net> (<source>)`, the
    host-overlap result (task 29: refusal or warning) and each launcher
    stamp's state (task 27); `qemu.Preflight`, `vz.Preflight`,
    `firecracker.Doctor` and `hyperv.Doctor` take the link; `host.go`'s doctor
    help text follows. (UX-04, ORCH-01, UX-01; a15)

### Per driver

24. qemu (rework: `driver.go`, `vm.go`): the claim carries the request's link
    (old-relay fix: rebuild warden, since the relay binary is appended per
    build); the direct-boot `-append` adds `BootArgs()`; the seeds render from
    the link (tasks 37, 38); the overlap and stamp checks (tasks 27, 29) run
    before the relay VM starts. Tier-0 test of the direct-boot command line for
    the default and a configured link. (ORCH-01, BOOT-03, UX-04, TEST-02 tier
    0; a11, a14)
25. Firecracker (rework: `driver_linux.go`, `vm.go`, `posture.go`, preflight,
    doctor, the `slots.go` comment): the `buildLink` variable goes and resolver
    selection takes the request's link in StartBuild and doctor; the
    no-resolver refusal names both fixes (a host resolver outside the link, or
    another `[vm] build_link`); `buildConfig`'s direct-boot `boot_args` add
    `BootArgs()`; the claim carries the link. Tier-0: a resolver inside a
    configured link excluded with that fix, one at `10.0.0.2` now passing
    (`posture_test.go`), the boot args for both links. (ORCH-01, UX-04,
    BOOT-03, TEST-02 tier 0; a4, a11, a14)
26. `driver/relayvm/assets.go`: a stamp path for an asset from the same place
    `Find` chose (release: the file name plus `.link-channel`; source tree:
    `link-channel` in the asset's output directory), used for Firecracker's
    launcher initramfs and by qemu's `resolveBuildAssets` for the direct-boot
    initramfs; the relay initramfs get none. (UX-04, ORCH-01, BOOT-06; a15)
27. Launcher-stamp checks, before any VM boots, each a `driver.Fixable` naming
    the one-time command: qemu's direct-boot initramfs (only when direct boot
    is used; `tools/build-vm/build-initramfs.sh`), Firecracker's launcher
    initramfs (`tools/firecracker-vm/build-initramfs.sh`), Hyper-V's
    `relay-boot.link-channel` beside the cached disk or `<name>.link-channel`
    beside a `WARDEN_RELAY_BOOT_VHDX` override
    (`tools/relay-vm/hyperv/refresh-cache.sh`), and vz's `<image>/link-channel`
    (`sudo warden image prepare --driver vz`); each driver's doctor reports
    the same. (UX-04, BOOT-06, ORCH-01; a15)
28. Tier-0 stamp tests on temporary directories (TEST-02 tier 0): missing,
    different and equal for each asset; each refusal a `driver.Fixable` naming
    its command; the stamp read from the same location as its asset (release
    beside the binary versus source tree); and each of the three build scripts
    writes `WARDEN_LINK_CHANNEL=` with `buildlink.Channel`'s value (plan
    note). No platform build tags. (TEST-02, TEST-01, GLOBAL-07, UX-04; a15)
29. Host-interface overlap: qemu, Firecracker, Hyper-V and vz StartBuild call
    `link.HostOverlap(driver.HostPrefixes())` before any VM boots and refuse
    an address inside the link with a `driver.Fixable` naming `[vm]
    build_link` and `--build-link`; a containing network is a doctor warning
    only. (UX-04, GLOBAL-06, ORCH-01; a4)
30. Hyper-V (rework: `relayhost.go`, `buildassets.go`, `setup.go`,
    `doctor.go`, the `buildhost.go` comment): the claim carries the link
    (old-relay fix: `refresh-cache.sh`); the VHDX stamp (task 27); the
    `winguest` seed from the link; `SetupOptions` gains the link and
    `RunSetup` refuses a `--nat-prefix` overlapping it before changing
    anything, naming another prefix; doctor lines. The fixed relay address,
    missing token, ignored `--nat-prefix` and fixed DNS stay as they are.
    Tier-0: the setup refusal; `runBuild` over a fake `Provisioner` creates and
    starts no build VM before `StartRelay`'s claim returns. (ORCH-01, UX-04,
    GLOBAL-06, BOOT-01, TEST-02 tier 0; a11, a15, a18, a20)
31. vz (rework: `network_unix.go`, `network_windows.go`, `driver.go`,
    `image.go`, `Preflight`): the virtual network's addresses from the
    request's link; the relay starts without `--subnet` and the claim carries
    the link (old-relay fix: rebuild warden); the `warden-boot` script runs
    `ipconfig set <if> DHCP`, waits for the lease within its 60 s interface
    window, reads the address, mask and router (`ipconfig getifaddr`,
    `getoption <if> subnet_mask` and `router`), writes the scutil IPv4 and
    DNS state and execs `warden-io initialize --gateway=<router> --ip=<address>`,
    with no address in the script; both prepare paths (first prepare and
    `ReprepareImage`) write `link-channel` after installing it; the stamp
    check (task 27). Tier-0: the script render (no link address, DHCP used).
    (ORCH-01, BOOT-03, BOOT-06, BOOT-07, UX-04, GLOBAL-07, TEST-02 tier 0;
    a13, a14, a15)
32. Order tests (TEST-02 tier 0): no build VM boots before `ClaimRelay`
    returns, on qemu and vz through function-field seams defaulting to today's
    methods (plan note) and on Firecracker through its `run` struct; Hyper-V's
    is in task 30. (BOOT-01, TEST-02, TEST-01; a12)
33. Tier-0 per-driver claim test (TEST-02 tier 0): the claim qemu,
    Firecracker, Hyper-V and vz send carries the request's link for the
    default and a configured value; the container driver's carries none.
    (ORCH-01, GLOBAL-08, TEST-02; a11, a17)

### Guests and boot assets (rework)

34. `tools/build-vm/init` and `build-initramfs.sh` (qemu direct boot): take
    `warden.link_ip=<a.b.c.d>/<n>` and `warden.link_gw=<a.b.c.d>` from
    `/proc/cmdline`, accepting only dotted quads and a numeric length and never
    evaluating the line; with either missing or malformed, say so on the
    console and stop. They drive the address, default route, resolver and
    `initialize --gateway`. The script writes `output/link-channel`
    (`WARDEN_LINK_CHANNEL=1`). (BOOT-03, BOOT-02, EXEC-04, BOOT-06; a14, a15)
35. `tools/firecracker-vm/init` and `build-initramfs.sh`: as task 34.
    (BOOT-03, BOOT-02, EXEC-04, BOOT-06; a14, a15)
36. `tools/relay-vm/init`: drops `ip addr add 10.0.0.1/30`, keeps
    `ip link set "$PRIVATE_IF" up` and every interface-keyed rule, starts
    `/sbin/relay -mode vm -build-if "$PRIVATE_IF"`; the header comment and the
    `say` line say the relay addresses the interface from the claim. (RELAY-04,
    BOOT-01, GLOBAL-06; a12)
37. `driver/linuxguest/seed.go`: network-config v2 address, gateway and
    nameserver, and `runcmd`'s `initialize --gateway`, from the request's link;
    `testdata/seed/` goldens for the default; a tier-0 render for a configured
    link. (BOOT-06, EXEC-04, TEST-02 tier 0)
38. `driver/winguest` (`winguest.go`, `run.go`): `WardenRunPS1` renders
    `--gateway` and `--ip` from the request's link; `BuildGatewayIP` and
    `BuildGuestCIDR` become the default's values for existing callers; a
    tier-0 render for both links. (BOOT-06, EXEC-04, TEST-02 tier 0)
39. `tools/relay-vm/hyperv/refresh-cache.sh` (and `build-boot-vhdx.sh` /
    `build-boot-vhdx-native.ps1` where they move the output): also install
    `relay-boot.link-channel`, moved to `.prev` with the disk; no `--link`.
    (BOOT-06, UX-04; a15)
40. `tools/vz-image-prep/{personalize,uv-build-test,netfwd}.go` take the
    prep-time link from `buildlink.Default()`, and `netfwd`'s `--subnet`
    defaults to it; prep-time networking does not follow the setting.
    `cmd/warden-vmnet/main.c`: `--subnet` becomes required (no callers).
    (GLOBAL-08, GLOBAL-07)
41. `cmd/warden-io` (`initialize_test.go`, `platform_windows_test.go`,
    `platform_windows.go` comments): other example addresses; behaviour and
    the `initialize` contract unchanged. (EXEC-04, TEST-02 tier 0; a19)

### Sweep, CI, docs and validation

42. Tier-0 stray-literal test (TEST-02 tier 0): walks the module tree from its
    root without `git`, skipping `.git`, `.dev`, `dist`, build outputs and
    binary files; fails on a whole-address `100.127.255.252`, `.253` or
    `.254`, or the old `10.0.0.0/30`, `10.0.0.1` or `10.0.0.2` (also in
    `IPv4(10, 0, 0, <n>)` form, plan note), outside the allowlist in the test:
    `driver/buildlink/`, the new default in `docs/book/**` and
    `tools/*/README.md`, the old link only under `docs/design/`,
    `driver/linuxguest/testdata/seed/`, and the named refusal and parsing
    tests (Firecracker's `10.0.0.2` resolver case, task 11's flush case).
    `10.0.0.20` and `110.0.0.1` do not match. Lands after tasks 24 to 41.
    (TEST-02, TEST-01, GLOBAL-08, UX-01)
43. CI: `firecracker.yml` drops the "Off-host DNS for the relay" step and its
    comment and adds `driver/buildlink/**` to `pull_request.paths`, its cache
    key unchanged; `integration.yml` adds `relaycfg/**` and
    `driver/buildlink/**` to its paths. (TEST-02; a4)
44. Reference docs (UX-01), as the site table lists: `guide/configuration.md`
    (the Full Reference key and an Advanced options section: default, what the
    link shadows, validation, precedence), `guide/cli.md` (`--build-link` on
    build, shell and doctor; doctor's link and stamp lines; `setup
    --nat-prefix`'s overlap refusal), `guide/drivers.md` (Upstream DNS; the
    one-time upgrade step per VM driver), `concepts/architecture.md` (Claim
    protocol: `build_link`, validation, the per-mode table, `ErrRefused`, the
    ordering rule), `concepts/network-isolation.md` (the slot table; the link
    block in vm and host modes), `contributing/structure.md` ("fixed
    10.0.0.0/30"; `driver/buildlink`, `buildif_linux.go`),
    `tools/win-image-prep/README.md`, and the `tools/relay-vm`,
    `tools/build-vm` and `tools/firecracker-vm` script headers. (UX-01, UX-03,
    GLOBAL-08, ORCH-03; a11, a15, a20)
45. `concepts/overview.md` pseudocode (FLOW-15): `ClaimRelay` pushes the build
    link; the relay block configures its build segment inside the claim
    (vm: netlink from the claim; host: the netstack and DHCP from it;
    container: refuses it) and blocks dials into it; each VM driver's
    StartBuild resolves the link and checks stamps and host overlap before
    booting; the direct-boot inits read their command line; vz's guest takes a
    DHCP lease. (FLOW-15, UX-01; a12, a13, a14)
46. If design review r3 lands Proposed standards change 1 in this feature's
    diff: `docs/development/bootstrap.md` BOOT-03 takes the design's
    replacement wording, and change 2 stays out unless the review takes it.
    Dropped if the review routes it to its own process item. (BOOT-07,
    FLOW-14, BOOT-03; a14)
47. Lint and the gate: `make test`, and `make lint` for linux
    (`buildif_linux.go`, Firecracker), darwin (`driver/vz`,
    `cmd/relay/mode_host.go`) and windows (`driver/hyperv`, `cmd/warden-io`,
    `cmd/relay`'s stub, `driver/vz/network_windows.go`). (FLOW-17, GLOBAL-07,
    TEST-01)
48. Tier 1 (TEST-02 tier 1): the `integration` workflow on the branch, covering
    container mode's claim path end to end with the new `relaycfg` and
    `cmd/relay`. (TEST-02, RELAY-04; a17)
49. Tier 2 in CI (TEST-02 tier 2): `firecracker.yml` on the branch with the
    DNS workaround gone, the hosted runner's VPC resolver `10.0.0.2` used
    directly on the default link, the rebuilt relay and launcher initramfs
    booted, the Ubuntu disk and the direct-boot builds green. (TEST-02,
    FLOW-11; a4, a12, a14)
50. Native validation (FLOW-09, TEST-02 tier 2), the queue's
    `f6-validate-linux-kvm`, `-qemu-any`, `-macos-vz` and `-windows-hyperv`,
    each with the Validation plan's four cases on the default link and
    `100.126.255.252/30` (pre-change assets refused with the one-time command
    and, on linux-kvm and qemu-any, an old relay initramfs still building with
    `10.0.0.1` gone from the relay's build interface; a default-link build
    whose ledger verifies; the configured link reported by doctor and built
    with no rebuild; an overlapping `--build-link` refused) plus the per-host
    cases there and the Security note's TEST-08 cases each host can reach.
    (FLOW-09, TEST-02, TEST-03, TEST-08; a12, a13, a14, a15, a16)
51. Tracked native gap (FLOW-09, TEST-03): the `build-link-subnet-qemu-windows`
    backlog entry for macos-qemu (a Windows guest on qemu on both links, after
    `qemu-windows-guest-package`), recorded by Publish with the item's
    `tier2_gaps`. (FLOW-09, TEST-03)
52. Follow-ups, recorded at Consolidation (FLOW-10): intake items
    `hyperv-relay-per-build-channel` (Decision 9, with Hyper-V's missing token
    named as a risk), `baked-asset-staleness` (Decision 10), a
    `specs/relay-claim/v1.md` contract document and a vz per-build seed disk;
    and the Security note's TEST-08 list handed to security-suite. (FLOW-10,
    FLOW-12, TEST-08; a15, a18)

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

- **r2 finding, addressed in Design (f1-design-r3, 2026-11-04, attended).**
  Based on the network overview (read at main `353e8a7`) and the relay,
  claim and driver code on main (`1f61b0c`). What each point changed:
  - *a2 (c), the link in the claim:* the claim gains `build_link {prefix,
    relay, guest}` on `/v1` (Decision 3, new question a11). In vm mode the
    relay addresses its build interface from it before replying; the init
    keeps every interface-keyed rule and the upstream setup (Decision 4,
    a12). Host mode builds its netstack from it instead of `--subnet` (a13).
    Container stays out (a17). The Hyper-V rebake, `refresh-cache.sh --link`,
    `link.env` and the shell copies are gone; the default is a Go constant.
  - *a3, records only where baking remains:* nothing bakes a link value
    after the guest side moved to per-build channels (Decision 5, a14): the
    seeds as before, two kernel command-line keys for the direct-boot inits,
    and a DHCP lease from the host relay's existing responder for vz. So
    there are no link records. A one-time launcher stamp replaces them for
    the upgrade (Decision 6, a15); the qemu and Firecracker relay initramfs
    need neither a stamp nor a rebuild, because the new relay flushes an old
    init's `10.0.0.1/30`.
  - *a7 superseded:* `seams: [relay-claim, relay-guest-api,
    bootstrap-model]` and `alters_relationships: true` (a19). The FLOW-05
    slot is free: no other in-progress feature declares `true`, and `wf
    next` enforces it at the end of Design.
  - *a8 in every mode:* the relay blocks upstream dials and resolvers inside
    the link in vm and host modes, independent of SSRF (Decision 7, a16).
  - *a9 trimmed:* `--build-link` on `build`, `shell` and `doctor` only;
    Hyper-V setup checks `--nat-prefix` against the key (Decision 8, a20).
  - *Ordering:* every VM driver already boots the build VM only after
    `ClaimRelay` returns, and `relaycfg` replies only after the start
    callback, in which the relay configures the link (Decision 4). Tier-0
    order tests pin it.
  - *Bootstrap comparison:* added. BOOT-03's wording gains a DHCP lease
    served by the relay (a proposed standards change); every other
    invariant is unchanged, and BOOT-06 moves closer for vz and the
    direct-boot inits.
  - *Security note:* rewritten for the claim change: who can send the claim
    per driver, the bounds on what `build_link` can do, and Hyper-V's
    missing token weighed (a denial of service today and after; deferred).
  - *The overview's open questions:* each answered or deferred (Decision
    10). Hyper-V's fixed `.2`, missing token, ignored `--nat-prefix` and
    `8.8.8.8` go to a follow-up, `hyperv-relay-per-build-channel` (Decision
    9, a18). A general staleness check goes to `baked-asset-staleness`.
  - *Native validation:* the same four blocking hosts and macos-qemu
    tracked; the cases now cover the one-time upgrade step and a
    non-default link with no rebuild on every host. The `f6-validate` steps
    return in Plan, needing `f2-design-review-r3`.
  - *Plan:* r2's plan is marked superseded; `f2-plan-r3` rewrites it.
  - *FLOW-06:* this is the second return to Design. A third would escalate to
    a `human` step.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None yet.

## Archive summary

Filled in at Publish (FLOW-12).

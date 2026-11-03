---
feature: build-link-subnet
status: draft
drivers: [qemu, vz, hyperv, firecracker]  # every VM driver: the shared relay init (qemu, hyperv, firecracker), driver/linuxguest (qemu, firecracker), driver/winguest (qemu, hyperv), driver/vz, driver/firecracker; container does not import the new package and is untouched
seams: []                # the executor's `initialize --gateway --ip` contract, the claim, collector and guest API are unchanged; only the values the orchestrator passes move (question a7)
surfaces: [output]       # new fixable stale-asset refusals and doctor lines; firecracker's resolver refusal names the new link; reference docs follow (UX-01, UX-04)
security_boundary: env-relay  # the link is the only path between the build environment and the relay; its posture is meant to be unchanged (Security note)
alters_relationships: false   # no component's role or interaction changes (question a7)
design_review: required
native_validation:
  - {host: linux-kvm, blocking: true, reason: "reaches driver/firecracker (resolver exclusion), tools/firecracker-vm/init and the x86_64 relay init. CI tier 2 (firecracker.yml) covers a fresh hosted runner, now without its DNS workaround so the VPC resolver 10.0.0.2 is the test; the native run covers the upgrade path: an existing slot host with stale pre-change boot assets refused with the fix, then rebuilt, then a direct-boot and a disk build"}
  - {host: qemu-any, blocking: true, reason: "reaches tools/build-vm/init (qemu-only direct boot), the aarch64 relay init and the driver/linuxguest seed on qemu: a Linux guest build, direct boot and cloud image, and the stale-asset refusal"}
  - {host: macos-vz, blocking: true, reason: "reaches driver/vz (network_unix.go, the warden-boot script baked at prepare): a re-prepared image builds; an image prepared before the change is refused with the re-prepare fix; the host-mode relay refuses dials into the link"}
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv (stale relay-boot VHDX refusal, NAT prefix overlap refusal), the relay boot VHDX rebuilt from the new init, and driver/winguest on a Windows guest"}
  - {host: macos-qemu, blocking: false, reason: "Windows guest on qemu: driver/qemu itself does not change, and the shared pieces it uses (driver/winguest, the relay init) are validated by windows-hyperv and qemu-any. qemu Windows builds already fail on main (backlog qemu-windows-guest-package), so a blocking step could not pass for reasons outside this feature (question a5)"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows guest build on the new link (backlog build-link-subnet-qemu-windows), after qemu-windows-guest-package is fixed"}
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, ORCH-01, ORCH-03, BOOT-06, BOOT-07, EXEC-04, RELAY-04, RELAY-05, UX-01, UX-04, TEST-02, TEST-03, TEST-08, FLOW-02, FLOW-07, FLOW-09, FLOW-12, FLOW-15, FLOW-19]
concepts: [build-link, two-vm-topology, network-isolation, address-plan, rfc6598, upstream-dns, resolver-selection, relay-vm-init, boot-assets, stale-assets, image-prepare, socketpair-topology]
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

This feature moves the link for every VM driver to a range no common upstream
uses, defines it once, and keeps assets that bake the old address from failing
silently.

## Prior art

- [`firecracker-driver.md`](firecracker-driver.md), Decision 4 (slots: the
  upstream `/30`s in `100.64.88.0/22`, beside the container driver's
  `100.64.87.0/24`; every slot reuses the fixed build link, kept apart by a
  conntrack zone per bridge) and Decision 5 (resolver selection: loopback,
  host-local and, since PR #90, build-link resolvers are excluded; none left
  is a fixable refusal). Its Rollback notes ("CI tier 2") queued this feature.
  Reused: the RFC 6598 address plan, the resolver exclusion (retargeted, not
  removed) and its fix text. Decision 4's conntrack zones still matter, since
  every slot keeps reusing the same two addresses.
- [`unified-bootstrap.md`](unified-bootstrap.md) and
  [`bootstrap.md`](../development/bootstrap.md): the executor gets the relay's
  address from `initialize --gateway` (and `--ip` where the guest has no
  cloud-init). That contract (EXEC-04, the `executor-initialize` seam) is
  unchanged; only the value changes. The per-driver mapping and the deviation
  register are unchanged too. vz's D2 (executor and boot script baked at
  prepare) is why a vz image must be re-prepared once (question a3). BOOT-06
  is why the Windows and Linux guests need no re-prepare: their address comes
  from the per-build seed.
- [`hyperv-windows-prepare.md`](hyperv-windows-prepare.md): `warden-run.ps1`
  goes on the per-build `WARDEN` seed, and the prepared image carries only the
  startup task, so the Windows guest's address moves with no image change.
- `driver/subnet.go`: the container driver's `100.64.87.0/24` (RFC 6598,
  "unlikely to collide with user networks"). This design follows the same
  reasoning for the VM link.
- GLOBAL-08: the address is orchestrator topology, not part of a versioned
  relay or executor seam. RELAY-05: no ledger, schema or environment-record
  format names the address (checked below), so no spec version changes.

## Decisions

1. **The link moves to `100.127.255.252/30`: relay `100.127.255.253`, build VM
   `100.127.255.254`** (question a1). This is the last `/30` of RFC 6598
   shared address space (`100.64.0.0/10`).

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
   | **RFC 6598, `100.127.255.252/30`** | Reserved for carrier NAT between the CPE and the carrier, so not a LAN, VPC or resolver range. Known uses sit elsewhere: Tailscale MagicDNS `100.100.100.100`, Alibaba metadata `100.100.100.200`, ChromeOS `100.115.92.0/23`, EKS custom networking `100.64.0.0/16`, BuildWarden's own `100.64.87.0/24` and `100.64.88.0/22`. Tailscale node addresses are spread over the whole `/10`, so a tailnet peer at one of these two addresses is possible but about one in a million per peer | Ordinary unicast on Linux, Windows and macOS; Go's `IsPrivate` is false, so nothing in the relay special-cases it | **Chosen** |
   | RFC 6598 next to BuildWarden's block (e.g. `100.64.86.0/30`) | As above, but inside `100.64.0.0/16`, which EKS custom networking and low-first CGNAT pools use | Fine | Rejected: the top of the `/10` is the least allocated |
   | Link-local `169.254.x.0/30` (away from `169.254.169.0/24`) | No routed address can be link-local, but on-link cloud services are, and the relay must reach some of them through host NAT: AWS DNS `169.254.169.253`, Kubernetes NodeLocal DNSCache `169.254.20.10`, ECS/EKS agents `169.254.170.x` | macOS configd treats a 169.254 IPv4 address as self-assigned and may not make the service primary. With only a link-local address, macOS and Windows may ARP for every destination instead of using the router (RFC 3927 §2.6.2). Windows labels it an autoconfiguration address and the network unidentified. The host-mode relay's SSRF filter already blocks link-local destinations | Rejected: real guest risk on the two OSes CI can't test |
   | Benchmark `198.18.0.0/15`, TEST-NETs, class E `240.0.0.0/4` | Clash/Surge-style fake-IP DNS hands out `198.18.x`. TEST-NETs appear in labs and documentation | Windows and macOS refuse to assign class E | Rejected |
   | Keep `10.0.0.0/30` and policy-route the relay's own sockets (marked dials take an upstream-only table) | None: overlap stops mattering | Unchanged | Rejected for now: a relay change in every dialer (proxy, DNS forward, collector), RELAY-04 parity across three modes, and nothing to gain on vz. Kept as the follow-up if a real RFC 6598 clash is ever reported |

   No range is guaranteed free, so the guards stay (Decision 4). A tier-0 test
   asserts the link is disjoint from every range BuildWarden itself uses or
   defaults to: the container base, Firecracker's upstream, Hyper-V's default
   NAT and qemu's user-mode network.

2. **One definition: `driver/buildlink/link.env`, embedded in Go and copied
   into every baked asset** (question a2). The file is three `KEY=value`
   lines (`WARDEN_LINK_NET`, `WARDEN_LINK_RELAY`, `WARDEN_LINK_GUEST`, plus
   `WARDEN_LINK_PREFIX`). It is the only place the addresses are written.
   - Go: package `driver/buildlink` (a leaf: `embed`, `net/netip`) embeds the
     file and exposes `Net`, `Relay`, `Guest`, `GuestCIDR()` and `Mask()`,
     validated at init (both hosts inside `Net`, prefix 30). Every Go
     consumer imports it. The container driver does not, so it stays
     untouched.
   - Shell: each `build-initramfs.sh` copies the file to `/etc/warden/link.env`
     in the image, and the inits source it (`. /etc/warden/link.env`) instead
     of writing addresses.
   - C (`cmd/warden-vmnet`, used only by `tools/vz-image-prep`): `--subnet`
     becomes required, so the C file holds no default, and the Go callers
     pass `buildlink.Net`.
   - A tier-0 test fails if either address, or the old `10.0.0.1`,
     `10.0.0.2` or `10.0.0.0/30`, appears anywhere in the tree outside the
     definition, `docs/design/` and the tests that check the refusal (an
     allowlist in the test).

   *Rejected:* a Go constant plus a drift test over the shell copies (two
   definitions kept equal by a test). Passing the link on the kernel command
   line (Hyper-V's relay command line is baked into its UKI, so the image
   would still need its own copy). Generating the init scripts from Go
   (heavy for three short scripts).

3. **Assets that bake the link record it, and a stale one is refused with
   the fix** (question a3; UX-04, BOOT-06). Per-build seeds pick up the new
   link with no user action: the Linux cloud-init `CIDATA` seed, the Windows
   `WARDEN` seed with `warden-run.ps1`, and the vz relay's `--subnet`.
   Built or prepared assets do not, and a mismatch would surface as
   `warden-io` timing out on `relay not ready`. These assets bake the link:

   | Asset | Driver | Record | Fix named by the refusal |
   |---|---|---|---|
   | relay initramfs (aarch64, x86_64) | qemu, firecracker | `link.env` beside the output (source tree) or `<release-name>.link.env` beside the binary | rebuild with `tools/relay-vm/build-initramfs.sh` (or upgrade the release assets) |
   | build-VM direct-boot initramfs | qemu (`tools/build-vm`), firecracker (`tools/firecracker-vm`) | same | that tool's `build-initramfs.sh` |
   | relay boot VHDX (UKI) | hyperv | `relay-boot.link.env` beside `relay-boot.vhdx` in the hyperv cache | `tools/relay-vm/hyperv/refresh-cache.sh` |
   | prepared macOS image (`warden-boot` script) | vz | `link.env` in the prepared image dir, written by prepare | `warden image prepare --driver vz` (re-prepare, D2) |

   The driver compares the record's bytes with the embedded definition before
   booting anything. A missing or different record is a fixable error naming
   the command. `warden doctor` reports the same per driver. The relay and
   build assets are checked together, so a build never pairs a new init with
   an old one. The vz boot script is still generated in Go at prepare, now
   from `buildlink`.

   *Rejected:* documenting the rebuild only (a silent timeout for everyone
   who upgrades). A transition window where the relay listens on both
   addresses (dead weight, and the build VM side is still split). Reading the
   link from the vz per-build share so vz needs no re-prepare (it still needs
   one re-prepare to learn to read it, and D2 is its own item).

4. **Guards stay, retargeted at the new link** (question a4).
   - Firecracker's resolver exclusion (PR #90) keeps its check against
     `buildlink.Net`, and its fix text names the new link. Keeping it costs
     nothing, and it is the only defence against the residual RFC 6598 case
     (a tailnet or CGNAT resolver). Firecracker is still the only driver
     whose relay is handed host resolvers. qemu's relay uses user-mode DNS
     `10.0.2.3`, Hyper-V's is baked at refresh, and vz resolves on the host.
   - `firecracker.yml` drops its "Off-host DNS for the relay" step, so the
     hosted runner's own VPC resolver `10.0.0.2` is used. That turns the
     original failure into the CI test for this fix. The job's asset cache
     key gains `driver/buildlink/link.env` and `tools/build-vm/init`.
   - Hyper-V setup refuses a `--nat-prefix` that overlaps the link (UX-04),
     as the relay would route that NAT into the build VM.
   - The host-mode relay (vz) refuses upstream dials into its own `--subnet`,
     not only the gateway (question a8). Before, the guest address `10.0.0.2`
     was covered by the SSRF filter's RFC 1918 block. An RFC 6598 address is
     not, and on a host with Tailscale it could be a real peer.

5. **No configuration knob** (question a6; ORCH-03). The link is a fixed
   constant like the container base and Firecracker's upstream range. A
   setting would mean a baked-asset record per value and a new surface. It
   can be added later, as an additive change, if a clash is reported.

## Questions

### Blocking

None. Every question below has a recommended answer the design proceeds on.

### Assumed

- **a1. The range.** Proceeding with `100.127.255.252/30` (relay `.253`, build
  VM `.254`), the top `/30` of RFC 6598; confirm. Options: (a) RFC 6598 top,
  `100.127.255.252/30`; (b) RFC 6598 next to BuildWarden's block,
  `100.64.86.0/30`; (c) link-local `169.254.87.0/30`; (d) keep `10.0.0.0/30`
  and policy-route the relay's upstream sockets. Recommended: (a). See
  Decision 1's table.
- **a2. Where the address is defined.** Proceeding with one embedded file,
  `driver/buildlink/link.env`, read by Go through `embed` and copied into
  every initramfs for the inits to source, with a tier-0 test against stray
  literals; confirm. Options: (a) embedded `link.env`; (b) a Go constant plus
  a drift test over the shell copies; (c) the kernel command line (not
  possible for Hyper-V's baked UKI). Recommended: (a).
- **a3. Assets baked with the old link.** Proceeding with a link record
  beside each baked asset and a fixable refusal (build and doctor) naming the
  rebuild or re-prepare command; confirm. Options: (a) record and refuse;
  (b) document the rebuild only; (c) the relay answers on both addresses for
  a transition. Recommended: (a). Users upgrading must rebuild or re-fetch
  their relay and build initramfs, re-run `refresh-cache.sh` on Hyper-V, and
  re-prepare vz images once.
- **a4. PR #90's refusal.** Proceeding with keeping Firecracker's resolver
  exclusion, retargeted at the new link, and dropping the CI DNS workaround
  so CI exercises the VPC-resolver case; confirm. Options: (a) keep and
  retarget, drop the workaround; (b) remove the exclusion, since the new
  range is out of common use; (c) keep both the exclusion and the
  workaround. Recommended: (a).
- **a5. macos-qemu tracked, not blocking.** Proceeding with macos-qemu
  (Windows guest on qemu) as a tracked gap: `driver/qemu` doesn't change, its
  shared pieces are validated on windows-hyperv and qemu-any, and qemu
  Windows builds already fail on main (backlog
  `qemu-windows-guest-package`); confirm. Options: (a) tracked; (b) blocking,
  with the criterion "gets no further than on main"; (c) blocking, waiting on
  the qemu Windows fix. Recommended: (a).
- **a6. No configuration knob.** Proceeding with a fixed link, no
  `warden.toml` key or flag; confirm. Options: (a) fixed; (b) a config key
  with a per-value asset record. Recommended: (a).
- **a7. No seam, no relationship change.** Proceeding with `seams: []` and
  `alters_relationships: false`. `initialize --gateway/--ip` keeps its
  contract (EXEC-04). The claim, collector and guest API don't change. No
  ledger, schema or environment-record format names the address (RELAY-05),
  only layer digests change in value. No BOOT invariant, mapping row or
  deviation changes (BOOT-07); confirm. Options: (a) as stated; (b)
  conservatively declare `executor-initialize` and `alters_relationships:
  true` because every driver's guest address moves. Recommended: (a).
- **a8. Host-mode relay blocks its own subnet.** Proceeding with
  `cmd/relay` host mode blocking upstream dials into the whole `--subnet`,
  not only the gateway, because an RFC 6598 guest address is no longer
  covered by the SSRF filter's private-range block; confirm. Options: (a)
  block the subnet (a small relay change, covered by the Security note);
  (b) leave the relay alone. Recommended: (a).

## Design

### The definition

```
driver/buildlink/
  link.env      WARDEN_LINK_NET=100.127.255.252/30
                WARDEN_LINK_PREFIX=30
                WARDEN_LINK_RELAY=100.127.255.253
                WARDEN_LINK_GUEST=100.127.255.254
  buildlink.go  //go:embed link.env; Net, Relay, Guest, GuestCIDR(), Mask(),
                Record() []byte (the file's bytes), CheckRecord(path) error
```

`CheckRecord` reads an asset's record and returns an error when it is
missing or differs. Package `driver/buildlink` imports nothing from `driver`,
so each driver wraps that error in `driver.Fixable` with its own fix text,
because only the driver knows the rebuild command.

### Every site that changes

| Site | Today | After |
|---|---|---|
| `tools/relay-vm/init` (qemu, hyperv, firecracker) | `ip addr add 10.0.0.1/30` on eth0, header comment | sources `/etc/warden/link.env`; `$WARDEN_LINK_RELAY/$WARDEN_LINK_PREFIX` |
| `tools/relay-vm/build-initramfs.sh` | — | copies `link.env` into the image and writes the output record |
| `tools/relay-vm/hyperv/` (`build.sh`, `build-boot-vhdx*.{sh,ps1}`, `refresh-cache.sh`) | bakes the init | rebuilt VHDX, writes `relay-boot.link.env` beside it |
| `tools/build-vm/init` + `build-initramfs.sh` (qemu direct boot) | `10.0.0.2/30`, `via 10.0.0.1`, `nameserver 10.0.0.1`, `--gateway=10.0.0.1` | sourced values; record written |
| `tools/firecracker-vm/init` + `build-initramfs.sh` | `10.0.0.2/30`, `via 10.0.0.1`, `--gateway=10.0.0.1` | sourced values; record written |
| `driver/linuxguest/seed.go` + `testdata/seed/{network-config,user-data}` | literal addresses in network-config v2 and `runcmd` | rendered from `buildlink`; goldens updated |
| `driver/winguest/winguest.go` (`BuildGatewayIP`, `BuildGuestCIDR`), `run.go` | string constants | derived from `buildlink` (names kept for callers) |
| `driver/vz/network_unix.go` | `net.IPv4(10,0,0,1)` etc. | from `buildlink` |
| `driver/vz/image.go` (`warden-boot` `GW`/`SELF`/`MASK`, comment) | literals baked at prepare | rendered from `buildlink` at prepare; prepare writes the record |
| `driver/vz` StartBuild, doctor | — | checks the prepared image's record |
| `driver/relayvm/assets.go` (`QEMURelay`, `Firecracker`), `driver/qemu` `resolveBuildAssets` | kernel + initramfs | plus each asset set's record, checked by `Find` callers |
| `driver/firecracker/posture.go` (`buildLink`), preflight and doctor fix text, `slots.go` comment | `10.0.0.0/30` | `buildlink.Net`; text names the new link |
| `driver/hyperv` (`buildassets.go`, setup, `buildhost.go` comment) | — | VHDX record check; `--nat-prefix` overlap refusal |
| `cmd/relay/mode_host.go` | blocks the gateway only | blocks dials into the whole `--subnet` (a8) |
| `cmd/relay/main.go` `--subnet` default `10.100.0.0/30` | not the build link (vz always passes it) | unchanged; the comment says vz passes `buildlink.Net` |
| `cmd/warden-vmnet/main.c` | default `--subnet=10.0.0.0/30` | `--subnet` required |
| `tools/vz-image-prep/{personalize,uv-build-test,netfwd}.go` | literals for the prep-time link | from `buildlink` (they run under `go run` in the module) |
| `cmd/warden-io` tests and `platform_windows.go` comments | `10.0.0.2/30` as parsing examples | updated examples; behaviour unchanged |
| `.github/workflows/firecracker.yml` | DNS workaround step; cache key | step removed; key adds `link.env`, `tools/build-vm/init` |
| Docs: `concepts/network-isolation.md` (slot table), `guide/drivers.md` (Upstream DNS; upgrade note per driver), `contributing/structure.md` ("fixed 10.0.0.0/30"), `tools/win-image-prep/README.md`, `concepts/overview.md` (stale-asset check in each VM StartBuild) | old link | new link and the rebuild/re-prepare notes (UX-01, FLOW-15) |

Unchanged, and why: `driver/subnet.go` (container, `100.64.87.0/24`);
Firecracker's upstream `100.64.88.0/22` (it must stay disjoint from the link,
which the tier-0 test checks); Hyper-V's NAT default `192.168.240.0/20`; qemu's
user-mode `10.0.2.0/24`; `relay/` (VM-mode relays bind by interface, and
REDIRECT and FORWARD DROP match on interfaces); `specs/`, `ledger/`,
`relaycfg/` (no address in any format; see below).

### Compatibility (RELAY-05)

No ledger entry, metadata schema, environment record or spec names the link's
address. The relay's DNS answers for its reserved names (`artifacts`, `cwd`)
carry its own address, but they go only to the guest and are not recorded.
Environment records list layer digests (initramfs, the vz `warden-boot`
script). Those digests change value because the files change, which is what
the records exist to show. No format version changes. Old ledgers verify as
before.

The upgrade cost is the baked-asset rebuild in Decision 3, which the refusal
names.

## Cross-driver impact

- **qemu.** Linux guests: per-build cloud-init seed and the direct-boot build
  initramfs move to the new link. The aarch64 relay initramfs is rebuilt.
  Windows guests: the per-build `warden-run.ps1` moves, and the prepared image
  is unchanged (BOOT-06). The stale-asset check covers both initramfs sets.
- **firecracker.** The relay and build initramfs are rebuilt, the
  disk-path seed moves through `driver/linuxguest`, and the resolver
  exclusion is retargeted. Slots, TAPs and host rules don't change, since no
  host rule matches the link's address. Every slot still reuses the same two
  addresses, kept apart by conntrack zones (firecracker-driver Decision 4).
- **hyperv.** The relay boot VHDX is rebuilt from the new init. The Windows
  guest's static address comes from the per-build seed. New: the stale-VHDX
  refusal and the NAT-prefix overlap refusal in setup.
- **vz.** The host relay takes the new `--subnet`, and the netstack serves the
  new gateway. The prepared image's `warden-boot` script is regenerated, so
  vz images are re-prepared once (D2), and the stale-image refusal says so.
  The host-mode relay blocks dials into its subnet.
- **container.** Unaffected. It never had the link (its relay and build share
  a `/29` from `100.64.87.0/24`) and doesn't import `driver/buildlink`
  (ORCH-01: no cross-contamination).

## Security note

**Boundary.** env-relay (GLOBAL-06): the build link is the only path between
the build environment and the relay. One relay change (a8) also touches the
relay's upstream filter in host mode.

**Posture.** Meant to be unchanged. Isolation stays topological: one NIC on
a private segment (socketpair, socket netdev, private switch, address-less
bridge), and in the relay VM, REDIRECT of 80/443/53 on the build interface,
MASQUERADE on the upstream interface and FORWARD DROP. None of those rules
matches on the link's address, so moving it changes no rule. What the move
could break, and what guards it:

- *The link must stay disjoint from every upstream range BuildWarden
  routes.* If it overlapped Firecracker's `100.64.88.0/22`, a relay's own
  upstream could be shadowed. The host anti-spoof rule (drop anything from
  `wfc<i>u` not sourced from the slot's relay) and MASQUERADE limited to
  `-s 100.64.88.0/22` still keep build-sourced packets from leaving, but a
  tier-0 test now asserts disjointness.
- *The host-mode relay's SSRF filter.* `10.0.0.2` was blocked as RFC 1918. An
  RFC 6598 guest address is not, so a build asking the vz host relay to dial
  `100.127.255.254` would go out the host's default route, which with
  Tailscale could be a real peer. a8 blocks the whole subnet. Dials to other
  `100.64.0.0/10` addresses keep today's behaviour: Tailscale peers were
  reachable through the vz host relay before this change and still are, and
  every such fetch is recorded.
- *A resolver inside the link.* Still excluded on Firecracker (Decision 4).
- *Stale assets.* Refused before boot, so a mismatched pair never runs half
  configured.

**TEST-08 should exercise:** a build-VM packet sourced from the new guest
address, sent to an off-host canary, is dropped. On Firecracker that is the
existing anti-spoof case with its source updated. A build that resolves a
name to the link's addresses through the vz host relay is refused. A host
resolver inside the link is refused on Firecracker. A stale relay initramfs,
VHDX or vz image is refused before any VM boots.

## Validation plan

- **Tier 0.** `driver/buildlink`: parse and validate `link.env`; disjointness
  from the container base, Firecracker upstream, Hyper-V NAT default and qemu
  user-mode ranges; no stray literal (old or new) outside the allowlist.
  `CheckRecord`: missing, different and equal records. Seed goldens
  (`driver/linuxguest`), `winguest.WardenRunPS1`, the vz `warden-boot`
  render, Firecracker resolver selection (a resolver at `100.127.255.254` is
  excluded and one at `10.0.0.2` now passes), the Hyper-V NAT-prefix overlap
  refusal, and the host-mode relay's subnet block (`isBlockedIP`).
- **Tier 1.** `make lint` per platform touched (darwin for `driver/vz` and
  `cmd/relay/mode_host.go`, windows for `driver/hyperv` and `cmd/warden-io`,
  linux for firecracker).
- **Tier 2, CI.** `firecracker.yml` without the DNS workaround: the hosted
  runner's VPC resolver `10.0.0.2` is used directly, and the Ubuntu disk
  build and the direct-boot build succeed.
- **Tier 2, native (FLOW-09).** Blocking: linux-kvm, qemu-any, macos-vz,
  windows-hyperv, each with the same three cases. (1) With the pre-change
  assets still in place, the build and doctor refuse with the fix. (2) After
  the named rebuild, re-prepare or refresh, a build succeeds and its ledger
  verifies. (3) The guest's address is the new one (console or
  `warden-run.log`). Plus per host: linux-kvm a direct-boot and a disk
  build; qemu-any a direct-boot and a cloud-image Linux build; macos-vz the
  subnet block; windows-hyperv a Windows guest build and the
  `--nat-prefix` overlap refusal. Tracked: macos-qemu (a Windows guest on
  qemu, after `qemu-windows-guest-package`), backlog
  `build-link-subnet-qemu-windows`. Not run: container-any and macos-finch,
  since the container driver is untouched.

## Implementation plan

Drafted here; Plan (phase 2) refines it after the design review.

1. Add `driver/buildlink` (`link.env`, embed, accessors, `CheckRecord`) with
   its tier-0 tests: validation, disjointness, and the stray-literal scan
   (GLOBAL-08, GLOBAL-07, TEST-02).
2. Move the Go consumers to it: `driver/linuxguest` seed and goldens,
   `driver/winguest`, `driver/vz` network and `warden-boot` render,
   `driver/firecracker` posture, preflight and doctor text, and
   `tools/vz-image-prep` (ORCH-01, UX-04).
3. Shell inits and image builds: `tools/relay-vm/init`, `tools/build-vm/init`
   and `tools/firecracker-vm/init` source `/etc/warden/link.env`. Each
   `build-initramfs.sh` copies it in and writes the record, and the Hyper-V
   boot-disk build and `refresh-cache.sh` write `relay-boot.link.env`
   (BOOT-06).
4. Stale-asset refusal: the record check in `driver/relayvm` asset lookup,
   qemu's build assets, Hyper-V's VHDX resolution and vz's prepared image
   lookup and prepare, with doctor lines for each (UX-04, ORCH-01).
5. Guards: Hyper-V `--nat-prefix` overlap refusal; host-mode relay blocks its
   `--subnet` (a8, GLOBAL-06, RELAY-04); `cmd/warden-vmnet` `--subnet`
   required.
6. CI: drop the DNS workaround in `firecracker.yml`, and extend its asset
   cache key (TEST-02).
7. Docs in the same change: `network-isolation.md`, `drivers.md` (Upstream
   DNS, per-driver upgrade note), `structure.md`, `win-image-prep/README.md`,
   and `overview.md` pseudocode for the stale-asset check (UX-01, FLOW-15).
8. Native validation per the Validation plan: queue steps
   `f6-validate-{linux-kvm,qemu-any,macos-vz,windows-hyperv}`, and the
   macos-qemu backlog gap (FLOW-09, TEST-03).

## Design review

None yet (`design_review: required`).

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None yet.

## Archive summary

Filled in at Publish (FLOW-12).

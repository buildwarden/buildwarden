---
feature: hyperv-relay-per-build-channel
status: draft
drivers: [qemu, firecracker, hyperv, vz]  # hyperv: the relay seed, the claim token, the per-build relay address, the NAT read back from the host, the relay's resolvers ([vm] dns or the host's), the durable strategy's per-build switches made by setup, the Windows guest on the relay's lease, the relay-boot stamp. qemu: tools/relay-vm/init is shared by every vm-mode relay and loses its baked network defaults (Decision 2), its Windows guests run the same warden-run.ps1 and also drop --ip (Decision 8), and a build with [vm] dns set is refused (Decision 6). firecracker: the shared init (as qemu), and its resolver selection moves to a shared package that also honours [vm] dns (Decision 6). vz, conservatively (FLOW-02): only the [vm] dns refusal, made in cmd/warden; nothing in driver/vz changes. container is unaffected: it ignores [vm] as it does today, and its relay never boots tools/relay-vm/init
seams: [bootstrap-model]  # BOOT-06 gains the per-build network-value invariant, and BOOT-07's surface list grows (Rule changes; Bootstrap comparison). relay-claim is unchanged: the claim token is already part of the protocol (relaycfg, Bearer on /v1/claim and the tunnel); Hyper-V only starts sending one. executor-initialize is unchanged: --gateway and the optional --ip keep their contract (overview.md: initialize --gateway=<relay> [--ip=<static>]); on Windows warden stops passing the optional --ip (Decision 8), a mode the contract already defines. relay-guest-api is unchanged: the relay's DHCP responder already serves the lease the Windows guest now keeps
surfaces: [cli, config, output]  # cli: `warden setup --driver hyperv` gains --slots (the durable strategy's per-build switches) and --nat-prefix/--host-ip now take effect on every build (UX-01); config: a new optional [vm] dns key (Decision 6, UX-01, UX-07, UX-08); output: doctor's NAT, relay-resolver and durable-slot lines, the new fixable refusals (UX-04)
security_boundary: env-relay  # the relay VM gains a per-build input (the seed), its claim gains a token, durable builds stop sharing one build segment (a build VM could reach another build's relay), and the relay's upstream resolvers become the user's choice or the host's. Also the host's claim path to the relay (the token); Security note
alters_relationships: true   # the bootstrap model's BOOT-06 changes (FLOW-02), the Hyper-V relay VM's upstream configuration moves from defaults baked in its init to a per-build seed the orchestrator writes, and the Windows guest's address moves from the seed's --ip to the relay's lease; FLOW-05 slot
design_review: required      # FLOW-19: alters_relationships, a seam, a security boundary, blocking native validation, open assumed questions (a10-a13) and rule changes; r2 changes the impact fields (drivers, surfaces) since the approval of the rule changes at 0df0a64
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv end to end (relay seed ISO on the relay VM, the relay initramfs's new storage modules, claim token, per-build relay address on the NAT, the NAT prefix and host IP read back from the host, the relay's resolvers from [vm] dns or the host, the durable strategy's setup-made per-build switches, the relay-boot stamp bump) and the Windows guest on the relay's lease with no --ip, behaviour only a Hyper-V host exercises"}
  - {host: qemu-any, blocking: false, reason: "regression only: tools/relay-vm/init loses its 192.168.240.x and 8.8.8.8 fallbacks, which a qemu relay (warden.net=dhcp from user-mode NAT, resolver from its lease) never takes; a rebuilt aarch64 relay initramfs booting a Linux build is the check (backlog hyperv-relay-per-build-channel-qemu-relay)"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows guest runs the same warden-run.ps1, which no longer passes --ip, so it keeps the relay's lease (Decision 8); qemu Windows builds already fail on main (backlog qemu-windows-guest-package), and windows-hyperv validates the same script and lease (backlog hyperv-relay-per-build-channel-qemu-windows)"}
tier2_gaps:
  - {host: qemu-any, gap: "a qemu Linux build on a relay initramfs rebuilt from the branch's tools/relay-vm/init (backlog hyperv-relay-per-build-channel-qemu-relay)"}
  - {host: macos-qemu, gap: "a qemu Windows guest ends on the relay's lease (DHCP enabled, PrefixOrigin Dhcp, the link's guest address) with no --ip in warden-run.log (backlog hyperv-relay-per-build-channel-qemu-windows), after qemu-windows-guest-package is fixed"}
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, ORCH-01, ORCH-03, BOOT-01, BOOT-02, BOOT-03, BOOT-05, BOOT-06, BOOT-07, EXEC-04, RELAY-03, RELAY-04, UX-01, UX-03, UX-04, UX-07, UX-08, UX-09, TEST-01, TEST-02, TEST-03, TEST-08, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19]
rule_changes:
  - {rule: BOOT-06, change: amend, paths: [docs/development/bootstrap.md]}
  - {rule: BOOT-07, change: amend, paths: [docs/development/bootstrap.md, tools/tier1/rules.yaml]}
concepts: [relay-seed, per-build-channel, claim-token, relay-claim, relay-vm-init, relay-address-allocation, nat-prefix, host-ip, upstream-dns, resolver-selection, vm-dns-config, durable-switch, network-slots, host-setup, two-vm-topology, network-isolation, launcher-stamp, stale-assets, windows-guest-network, dhcp, explicit-over-implicit]
related: [build-link-subnet.md, firecracker-driver.md, hyperv-driver-plan.md, hyperv-windows-prepare.md, windows-driver-plan.md, unified-bootstrap.md, vm-driver-release-plan.md]
---

# Give the Hyper-V relay VM a per-build channel before the claim

## Problem

Every other VM driver hands its relay VM per-build values before the claim:
qemu and Firecracker on the relay's kernel command line (`warden.claim_token`,
and for Firecracker `warden.nat_ip`, `warden.dns`), vz in the host relay's
environment. Hyper-V's relay VM boots a UKI whose command line is baked into
the boot VHDX (`console=ttyS0 console=tty0`, `tools/relay-vm/hyperv/build.sh`),
and Gen2 firmware gives the host no way to pass load options per boot. So
`tools/relay-vm/init` falls back to defaults it bakes, and
[`build-link-subnet`](build-link-subnet.md) Decision 9 (approved at its design
review r3, a18 (a)) deferred the consequences here, together:

1. **Every relay takes `192.168.240.2/20` via `192.168.240.1`**
   (`NAT_IP`/`NAT_PREFIX`/`NAT_GW` in the init, `defaultRelayNATIP` in
   `driver/hyperv/relayhost.go`). Two concurrent Hyper-V builds share the NAT
   switch (a durable one, or the one `EnsureNetwork` reuses), so their relays
   hold the same address: one orchestrator can claim the other's relay, and
   both builds fail.
2. **The claim carries no token** (`StartRelay` passes `""`). A local
   process that reaches `192.168.240.2:8301` first, or another build's
   orchestrator, wins the claim. Today that is a denial of service: the
   orchestrator's own claim gets 409 and the build VM is never created
   ([`build-link-subnet`](build-link-subnet.md), Security note).
3. **`--nat-prefix` and `--host-ip` are accepted at setup and ignored per
   build.** `warden setup --driver hyperv` builds the durable NAT on whatever
   the user passes, but the relay still addresses itself in
   `192.168.240.0/20`, so a durable network on another prefix has a relay
   that reaches nothing. Ephemeral builds always ask `EnsureNetwork` for the
   default prefix. build-link-subnet Decision 7 added the overlap refusal at
   setup and left the per-build re-check for when the prefix is honoured.
4. **The relay's upstream DNS is a fixed `8.8.8.8`** (the init's fallback
   when `resolv.conf` is empty, which on Hyper-V's static upstream it always
   is). Networks that block public DNS, or only resolve internal names on
   their own resolvers, fail every build's name lookups.
5. **The durable strategy reuses one Private switch for every build**
   (`warden-dev`, `provisionBuildNetwork`'s `NetReuseDurable`). Concurrent
   durable builds share one L2 segment: build VM A can reach relay B and
   build VM B, both relays answer DHCP on the same link addresses
   (`100.127.255.253`), and A's traffic can land in B's ledger. That breaks
   BOOT-01's "sole network path is *the* relay" for each build, and the
   ledger's record of what a build consumed.

Also weighed here, from build-link-subnet's native validation on
windows-desktop (`native/windows-hyperv/2026-11-04/README.md`, finding 1, and
PR #98's archive summary): a Windows guest now takes the vm-mode relay's DHCP
lease before `warden-io initialize --ip` runs, so it ends DHCP-configured
(`PrefixOrigin Dhcp`, the relay's infinite lease) where main's ended
`Manual`. The address and route are the same either way. Whether
`initialize` should still make the NIC static was left for this item.

All five have one cause: no per-build channel into the relay VM before the
claim. This feature adds one and fixes them together.

## Prior art

- [`build-link-subnet.md`](build-link-subnet.md): Decision 9 (this item's
  scope), Decision 3 (the claim carries the link; a relay that predates a
  claim field must fail loudly with a `driver.Fixable`), Decision 4 (the
  relay owns its build interface's address; the init keeps the rest of the
  relay VM's network setup, and no init iptables rule names an address),
  Decision 6 (the one-time `link-channel` launcher stamp beside the relay
  boot VHDX, checked before boot and in doctor), Decision 7 (the
  `--nat-prefix` overlap refusal at setup, the per-build re-check deferred
  here), Decision 10 (the overview's open questions: "a per-build channel as
  the rule for every relay and guest value" deferred, with BOOT-06's optional
  extension left for this item), its Bootstrap comparison item 2 (the
  extension's wording), and its Security note (the claim-endpoint table, in
  which Hyper-V is the only row without a token). Its Decision 2 rejected "a
  per-build seed disk or Hyper-V KVP for the relay VM (each adds a component
  to the relay VM's trust boundary for a value the claim can carry)". That
  reason does not hold here: the values below are the ones needed to *reach
  and authenticate* the claim, so the claim cannot carry them.
  Reused: the stamp, the claim's token path, the per-build-channel principle
  ("no asset bakes a value").
- [`firecracker-driver.md`](firecracker-driver.md), Decision 5 (resolver
  selection): the relay takes the host's resolvers, excluding loopback,
  host-local and (since build-link-subnet) build-link addresses, and none left
  is a fixable refusal; Decision 4 (a per-slot upstream address and a
  per-build claim token, both on the relay's command line), and its durable
  per-host network slots made once by a root `warden setup --driver
  firecracker [--slots N]`, so builds run unprivileged. Reused: the
  selection rule, moved to a shared package so both drivers run one copy
  (FLOW-15), the token's handling, and the slot model and `--slots` flag for
  the Hyper-V durable strategy's per-build switches (Decision 7).
- Jeff's design principles (2026-11-08, after this design's review r1;
  queued as the process item `design-principles`): *least surprise* (the
  default is a sane, obvious choice), *explicit over implicit* (a provided
  value is used exclusively, never as a fallback unless explicitly named as
  one), *minimal magic* (predictable behaviour over resilience against
  failures). Round 2 applies them to Decisions 6 to 8 and checks every other
  fallback in the design against them (Decision 11).
- `cmd/warden/config.go` and `configuration.md`'s `[vm]` table: the
  precedent for a `[vm]` key (`build_link`: validated at load, applied by
  every VM driver, ignored by the container driver; `network` and
  `image_refresh_after`: labelled with the one driver that reads them), which
  `[vm] dns` follows (Decision 6).
- [`hyperv-driver-plan.md`](hyperv-driver-plan.md) (active): the privilege
  model (VM operations at the Hyper-V Administrators tier; `New-VMSwitch`,
  `New-NetNat` and `New-NetIPAddress` need full Administrator), the two-NIC
  relay VM, the COM1 named pipe, the network strategies, and Open Question 1
  (hvsock rejected for the build link in favour of a vSwitch). Its "Option 4,
  KVP" weighed KVP for guest-to-host signalling, not host-to-guest
  configuration.
- [`windows-driver-plan.md`](windows-driver-plan.md): the seed disk as the
  robust way to get per-build material into a VM; `configureNetwork` "prefer
  DHCP served by the relay".
- [`hyperv-windows-prepare.md`](hyperv-windows-prepare.md): the per-build
  `WARDEN` seed is a DVD ISO built by the in-box IMAPI2 COM API
  (`makeSeedISOWindows`), at the Hyper-V Administrators tier, attached with
  `Add-VMDvdDrive`. This feature builds the relay's seed the same way.
- [`unified-bootstrap.md`](unified-bootstrap.md) and
  [`bootstrap.md`](../development/bootstrap.md): BOOT-02 binds what reaches
  the *build environment* out of band; the relay VM is not the build
  environment, so a relay seed is not a second input to it. BOOT-06 is the
  rule this feature extends. BOOT-07 governs the change.
- `driver/subnet.go` (`AllocateNetwork` under `driver.HostLock`) and
  `driver/hyperv/reap.go` (`ListVMs` by name prefix): the host-wide lock and
  the VM-name convention the relay-address allocator reuses.
- `relaycfg` (`Serve(ln, token, ...)`, `Claim`, `OpenTunnel`) and
  `cmd/relay` (`WARDEN_CLAIM_TOKEN`): a vm-mode relay already enforces a
  token when its init passes one. No relay code changes for the token.

## Decisions

Round 2 (after design review r1, 2026-11-08) rewrites Decision 6 (a3: an
optional `[vm] dns` key, used exclusively when set), Decision 7 (a4 (c): a
per-build Private switch for each durable build) and Decision 8 (a6 (c): no
`--ip` on Windows, the relay's lease alone), and adds Decision 11 (no runtime
fallback anywhere in the design). Decisions 1 to 5, 9 and 10 stand as r1
confirmed them; Decision 1's SMBIOS wording is clarified, not changed.

1. **The channel is a per-build relay seed: a small ISO, built on the host
   per build and attached to the relay VM as a DVD, which the init reads
   before it configures anything** (question a1).

   The host stages `<workDir>/relay-seed/warden-relay.conf` and packs it
   with `makeSeedISOWindows` (IMAPI2, volume label `WARDENRELAY`), and
   `buildRelayVMScript` adds `Add-VMDvdDrive -Path <iso>` while the VM is
   off. The relay initramfs gains the modules to read it (Hyper-V storage,
   SCSI CD-ROM and ISO 9660, whichever the `linux-virt` kernel builds as
   modules; `build.sh` already skips a module the kernel has built in). The
   init, on Hyper-V only, waits up to 10 s for the CD device, mounts it
   read-only, copies the one file to tmpfs, unmounts it, and parses it
   (Decision 2).

   | Option | Per VM, before boot | New guest code | Host privilege | Verdict |
   |---|---|---|---|---|
   | **Seed ISO on a DVD** | yes | 3-4 kernel modules, a mount, the existing key parser | Hyper-V Administrators (as the build VM's seed) | **Chosen**: the pattern every seeded guest already uses; IMAPI2 already builds the Windows seed |
   | Hyper-V KVP (Data Exchange) host-to-guest items | only after the VM runs, so the init must wait on a channel that the network setup itself waits on | `hv_utils` plus a reader for the kernel's KVP message protocol (or the C `hv_kvp_daemon`), new parsing code inside the trust boundary | WMI `AddKvpItems`, Hyper-V Administrators | Rejected: more new code in the relay VM, and an ordering dependency |
   | SMBIOS strings (`BIOSSerialNumber`, `ChassisAssetTag` via `Msvm_VirtualSystemSettingData`) | yes | none (`/sys/class/dmi/id`) | WMI `ModifySystemSettings` | Rejected for now: no cmdlet, undocumented length limits on Gen2, so it needs a spike. Named as the *contingency* if the DVD path fails native validation: a return to Design that replaces the DVD (Rollback), never a runtime fallback; no build ever tries one channel and then the other (Decision 11) |
   | COM1 named pipe, host writes a config line | after boot | a serial reader | the pipe | Rejected: one client per pipe (the console tooling holds it), data written with no reader is lost, and the token would cross the console channel |
   | Claim over hvsock (AF_HYPERV / AF_VSOCK) instead of the NAT | n/a | `hv_sock`, a vsock listener in `cmd/relay` | a per-host registry entry for the service GUID (Administrator), `go-winio` | Rejected: changes the claim transport (relay-claim seam), a new dependency, and an elevated step on every host; it would also not give the relay its upstream address or resolvers |
   | Address from the NIC's static MAC | yes | none | Hyper-V Administrators | Rejected: 24 usable bits carry an address, not a token or resolvers |

2. **The seed carries exactly the relay's pre-claim network values, in the
   key syntax the relay's kernel command line already uses, and the init
   loses every baked network default** (question a7; BOOT-06 as amended).

   `warden-relay.conf` is one `warden.<key>=<value>` per line, the keys the
   init already parses: `warden.net=static`, `warden.nat_ip`,
   `warden.nat_prefix`, `warden.nat_gw`, `warden.dns` (comma-separated) and
   `warden.claim_token`, plus `warden.seed=1` (the format version). Nothing
   else goes in it: not the build link (the claim carries it), not the
   collector, not the build's inputs. The init reads its keys from one
   function that takes words from `/proc/cmdline` and, on Hyper-V, from the
   seed file, with the same rules as today (only known keys, never
   evaluated) and new value checks applied to both sources: `nat_ip`,
   `nat_gw` and each `dns` entry dotted quads, `nat_prefix` 1 to 30,
   `claim_token` `[0-9a-f]{64}` (what `driver.RandomToken` emits), `net` one
   of `dhcp` and `static`. A malformed value, a missing seed on Hyper-V, or a
   static upstream without `nat_ip`/`nat_prefix`/`nat_gw` stops the init
   with a console line on COM1 and starts no relay, so the orchestrator's
   claim times out with the relay's serial log as the evidence.

   The init's `NAT_IP=192.168.240.2`, `NAT_PREFIX=20`, `NAT_GW=192.168.240.1`,
   the "Hyper-V means static" default, the "DHCP failed, use the static
   defaults" fallback and the `${UPSTREAM_DNS:-8.8.8.8}` fallback all go.
   None of them is reached by qemu (DHCP from user-mode NAT, whose lease
   carries the resolver) or Firecracker (every key on its command line); a
   qemu relay whose DHCP fails now stops with a console line instead of
   addressing itself in Hyper-V's NAT prefix, which never worked on qemu.
   After this, no relay boot asset carries a network value.

   *Rejected:* a JSON or YAML seed (the init is busybox `sh`; the key syntax
   already has a parser and a validation story). Keeping the defaults as a
   fallback for an orchestrator that sends no seed (a third-party
   orchestrator gets loud failure instead; see Decision 9 for warden's own
   skew, and the BOOT-06 amendment for why no default stands in for a
   channel).

3. **Every Hyper-V build claims its relay with a fresh token** (question a1;
   Security note). `StartRelay` takes `driver.RandomToken()` (32 random
   bytes, hex), writes it to the seed as `warden.claim_token`, and passes it
   to `Session.ClaimRelay`, which already sends it as `Bearer` on
   `/v1/claim` and on the tunnel. The init already exports it to
   `/sbin/relay` as `WARDEN_CLAIM_TOKEN`, and `relaycfg.Serve` already
   refuses a wrong token with 401 without consuming the relay. So no relay or
   protocol code changes; Hyper-V joins the table in which every other driver
   has a per-build token. The token lives only in the seed ISO and its
   staged file under the build's work directory (removed with it), and in the
   orchestrator's memory. It is never logged, never in the environment
   record, and never on the relay's console (the init's progress lines name
   keys, not the token's value).

4. **Each relay gets its own address on the NAT, allocated per build under a
   host lock, recorded where every warden process can see it** (question
   a2). The allocator, in `driver/hyperv`, takes `driver.HostLock("hyperv-network")`,
   lists the host's VMs named `warden-relay-*` with their Notes, collects the
   addresses recorded there (`warden-relay-ip=<addr>`), and picks the lowest
   host address of the NAT prefix that is not the network address, the
   broadcast address, the host IP, or recorded. `CreateRelayVM` writes the
   address into the new VM's Notes (`Set-VM -Notes`, Hyper-V Administrators)
   before the lock is released, so the next allocation sees it whether the
   VM is running, stopped, kept by `--keep-vms`, or left by a crash. The
   address is freed when the VM is removed. A prefix with no free address is
   a fixable refusal naming the VMs that hold them. The relay's address, the
   prefix length and the host IP go into the seed; `StartRelay` claims at
   that address.

   *Rejected:* lease files in the lock directory (they outlive a removed VM
   and miss a `--keep-vms` VM after its process exits). A per-address file
   lock held for the build's lifetime (released when the process exits,
   while a kept VM still holds the address). A random address with no record
   (nothing detects an IP conflict on the NAT switch; the host vNIC would
   just reach whichever relay answered ARP last).

5. **The NAT a build uses is read back from the host, so `--nat-prefix` and
   `--host-ip` take effect on every build, and the prefix is checked against
   the build link at build start** (question a5; UX-04, UX-08).

   - **What a build uses.** Durable strategy: the dev switch's NAT,
     `<switch>-nat`. Ephemeral and delegated strategies: the dev switch's
     NAT when it exists (whatever prefix setup gave it), else a warden NAT
     that owns `DefaultNATPrefix`, else a new per-build NAT on
     `DefaultNATPrefix`, as today. So a user who ran `warden setup
     --nat-prefix 10.200.0.0/24` gets that prefix on every build, elevated or
     not, and the host keeps one NAT (WinNAT supports one NAT network per
     host in practice; standing up a second on the default prefix beside the
     user's is what the setup flag was meant to avoid).
   - **How it is read.** `EnsureNetwork` and `VerifyNetwork` return the NAT's
     `InternalIPInterfaceAddressPrefix` (`Get-NetNat`) and the host vNIC's
     IPv4 address on that prefix (`Get-NetIPAddress -InterfaceAlias
     "vEthernet (<nat>)"`). The relay's prefix length and gateway come from
     those, never from a constant. A NAT whose host vNIC holds no address in
     its prefix is a fixable refusal naming `warden setup --driver hyperv`.
   - **The re-check build-link-subnet deferred.** At build start and in
     doctor, a NAT prefix that overlaps the resolved build link (`[vm]
     build_link`, `--build-link` or the default) is refused before any VM
     boots, with the fix naming both `[vm] build_link` and `warden setup
     --nat-prefix` (setup's own refusal stays). The relay's own check (it
     refuses a link inside another interface's route, build-link-subnet
     Decision 4) stays as the backstop.

   *Rejected:* persisting setup's flags to a file or a `[vm]` key that builds
   read (a second copy that drifts from the host the moment someone edits the
   NAT by hand). Adding `--nat-prefix` to `build` (the NAT is host
   infrastructure that setup owns; a per-build prefix would need a NAT per
   build, which needs elevation).

6. **The relay's upstream resolvers are the `[vm] dns` list when it is set,
   used exclusively, and otherwise the host's, chosen by Firecracker's rule,
   which moves to a shared package; when neither yields a resolver the build
   is refused** (question a3, changed at r1; question a10; UX-01, UX-04,
   UX-07, UX-08).

   - **The key.** `[vm] dns`, an optional list of IPv4 addresses
     (`dns = ["10.20.0.53", "10.20.0.54"]`), in `warden.toml` or the user
     config with the usual precedence (project over user, ORCH-03, UX-07).
     It has no flag and no environment variable (Jeff asked for a key; a
     `--dns` flag can follow if wanted). Unset, the default is the host's
     resolvers, below.
   - **Set: used exclusively.** Its entries, in order, are the relay's
     resolvers, and nothing else is: the host's resolvers are not read, not
     appended and not tried if the listed ones fail. A listed resolver that
     does not answer fails the build's lookups, as a host resolver that does
     not answer would; the relay records what it served, as always.
   - **Validated, never filtered.** At config load (UX-08), every command:
     each entry an IPv4 address, not unspecified, loopback, link-local,
     multicast or broadcast, and the list not empty (`dns = []` is refused:
     "remove `[vm] dns` to use the host's resolvers"). At build start and in
     doctor, the driver's own reachability rule (the same exclusions the
     host selection applies: an address assigned to the host, inside the NAT
     prefix on Hyper-V, inside the build link) refuses the build naming the
     entry and why. A set list is all used or the build is refused: an
     unusable entry is never dropped silently, because the user named it.
   - **Unset: the host's.** At build start the Hyper-V driver reads the IPv4
     DNS servers of the interface that holds the host's lowest-metric IPv4
     default route (`Get-NetRoute -DestinationPrefix 0.0.0.0/0`, then
     `Get-DnsClientServerAddress -InterfaceIndex <i> -AddressFamily IPv4`)
     and runs the shared selection: exclude loopback, link-local, addresses
     assigned to the host, addresses inside the NAT prefix and addresses
     inside the build link. Here filtering is the rule itself (the host's
     list is not the user's choice for the relay; it is the default's
     source), as on Firecracker today.
   - **Neither yields one: refused.** No qualifying host resolver is a
     `driver.Fixable` naming each excluded server and why, with the fixes
     "give `<interface>` an off-host DNS server" and "set `[vm] dns`" (and,
     for an excluded link address, `[vm] build_link`). No public resolver
     stands in.
   - **Where it goes.** The chosen list goes into the seed as `warden.dns`
     (Decision 2). Doctor prints it with its source (`[vm] dns`, or the
     host interface it came from), or the refusal.
   - **Which drivers read it.** Hyper-V and Firecracker, the two drivers
     whose relay takes its resolvers from a selection over the host's: the
     key replaces the selection's input in the shared step, and Firecracker
     passes it on the relay's command line as `warden.dns` as it does its
     selection today, with "set `[vm] dns`" added to its fix texts (the WSL
     case among them). qemu's relay resolves through user-mode NAT's lease
     and vz's host-mode relay through the Mac's own resolver; neither has a
     resolver selection, so a qemu or vz build with `[vm] dns` set is
     refused at startup, in `cmd/warden` beside the existing `--slots` check,
     with "`[vm] dns` is read by the hyperv and firecracker drivers; remove
     it to build with <driver>". A set value is never ignored (explicit over
     implicit). The container driver ignores `[vm]`, as it does
     `[vm] build_link`. Honouring the key on qemu (the init already prefers
     `warden.dns` over its lease) and vz (a host-mode relay resolver option)
     is a possible follow-up, not this feature.

   `selectResolvers` and `inLink` move from `driver/firecracker/posture.go`
   into a new `driver/resolvers` package with two entry points: `Select`
   (host list, link, host-local predicate, extra excluded prefixes: filters,
   reports what it excluded) and `Check` (the `[vm] dns` list, same inputs:
   refuses on the first unusable entry). Each driver keeps its own source
   (`/etc/resolv.conf` and systemd-resolved on Linux, the default-route
   interface on Windows) and its own fix text (FLOW-15: one rule, not two
   that drift).

   *Rejected:* keeping `8.8.8.8` (the bug). Falling back to public resolvers
   when no host resolver qualifies (a resolver the user never chose; r1's
   (b)). Using `[vm] dns` and then the host's resolvers if it fails, or
   merging the two lists (r1 review: a provided value is used exclusively).
   Filtering an unusable `[vm] dns` entry and using the rest (the user named
   it; refusing says so). A `--dns` flag in this feature (not asked for).
   Honouring the key silently only on Hyper-V and ignoring it elsewhere (a
   set value with no effect).

7. **Each durable build gets its own Private switch, from a pool that the
   elevated `warden setup --driver hyperv --slots N` makes once; a build
   holds its switch by attaching its relay VM to it, under the host lock of
   Decision 4, and needs no elevation** (question a4, changed at r1 to (c);
   question a11; BOOT-01).

   - **Where the elevation happens: setup.** `New-VMSwitch` is in the
     full-Administrator tier of the Hyper-V driver plan's privilege table,
     and the durable strategy exists for the build that has neither
     elevation nor the privileged service (`NetReuseDurable`: Hyper-V
     Administrators, no service). A switch created per build would need one
     of those, which that build lacks by definition, so the switches are
     made where elevation already happens for this strategy: `warden setup
     --driver hyperv`, which already creates the NAT switch, the host IP and
     the NAT. `--slots N` (1 to 64, default 4) is the flag Firecracker's
     setup already has, now accepted by `--driver hyperv` too; setup creates
     the Private switches `<network>-0` to `<network>-<N-1>` (`warden-dev-0`
     by default) idempotently, removes slot switches above `N` that no
     adapter is attached to, and refuses (naming the VMs) to remove one that
     is in use. The single shared `<network>` Private switch of earlier
     setups is no longer used: setup removes it when no adapter is attached
     and says so, and refuses with the attached VMs named otherwise.
   - **The privilege tier per build: Hyper-V Administrators.** A durable
     build only lists adapters and connects its VMs' NICs to an existing
     switch (`Get-VMNetworkAdapter -All`, `Add-VMNetworkAdapter
     -SwitchName`), the VM-lifecycle tier. Elevated and service-delegated
     builds keep their ephemeral per-build switch (`warden-<buildID>`), as
     today; nothing changes for them.
   - **What a durable build holds.** No lock for its lifetime. Under
     `driver.HostLock("hyperv-network")`, the allocator of Decision 4 also
     picks the lowest slot switch that no VM adapter is attached to, and
     `CreateRelayVM` attaches the relay's build NIC to it before the lock is
     released. The attachment is the hold, read from Hyper-V like the
     relay's Notes: it survives a crash and a `--keep-vms` run, and is freed
     when the relay VM is removed (teardown, `warden` reaping, or the user).
     The relay VM's Notes also carry `warden-relay-switch=<name>`, so doctor
     can name which build holds which slot. The build VM attaches to the
     same switch later; an attached build VM alone also marks a slot busy.
   - **All slots busy** is a `driver.Fixable` before any VM is created,
     naming each slot's holders and the fixes (wait, remove a kept VM, or
     `warden setup --driver hyperv --slots <more>`, elevated). No queueing
     and no fallback to a shared switch (Decision 11).
   - **Checks per build.** `VerifyNetwork` checks the chosen slot switch is
     Private and has nothing attached but this build's VMs once they exist,
     and the NAT as today. A host whose durable network has no slot switches
     (set up by an older warden) is refused with "run `warden setup --driver
     hyperv` (elevated) to create the build switches"; doctor says the same.
     Capability detection (`DevSwitchPresent`) probes for the slot switches
     instead of the one switch.

   | Option | Isolation | Elevation | Held per build | Verdict |
   |---|---|---|---|---|
   | **Pool of Private switches made by setup (`--slots`)** | a switch per build | once, at setup (already elevated) | the relay VM's attached NIC | **Chosen** |
   | A Private switch created per durable build | a switch per build | every build: full Administrator, which the durable strategy's caller lacks | nothing extra | Rejected: impossible for the caller this strategy serves; if Hyper-V Administrators can in fact create a Private switch, that is a later simplification, found by a spike, not assumed here |
   | Per-build creation through the privileged service | a switch per build | the service | nothing extra | Rejected: a build that reaches the service already takes the ephemeral strategy (`NetDelegateService`) |
   | A per-build access VLAN on the shared switch (r1) | VLAN filtering | none per build | the VLAN on the relay's NIC | Rejected at review r1 |
   | One durable build at a time | one build per segment | none | a host lock | Rejected: no concurrency |

8. **The Windows guest takes its address from the relay's lease alone:
   `warden-run.ps1` runs `warden-io initialize --gateway=<relay>` with no
   `--ip`, and a guest with no lease fails its build with a line that says
   so** (question a6, changed at r1 to (c); question a12; EXEC-04,
   BOOT-03).

   - **What changes.** `winguest.WardenRunPS1` drops `--ip=<guest CIDR>`
     from the command it writes into the per-build `WARDEN` seed (Hyper-V,
     `driver/hyperv/winseed.go`) and the qemu Windows seed
     (`driver/qemu/windows.go`, the same function). The seed keeps
     `--gateway=<relay>`, the build link's relay address, which the guest
     needs for its hosts entries, DNS and the relay's health and output
     endpoints. `winguest.BuildGuestCIDR` loses its last user and goes.
   - **The address.** The guest's NIC is in DHCP mode from first boot (the
     prepared image never sets a static address). The vm-mode relay is
     claimed, and its responder (`cmd/relay/dhcp*.go`, build-link-subnet
     a14 (d)) answering, before the build VM is created (Decision 3's
     order), so the NIC's first DISCOVER gets the link's guest address, the
     relay as router and DNS, and an infinite lease. The guest ends `DHCP
     Enabled: Yes`, `PrefixOrigin Dhcp`, at the link's guest address: the
     state build-link-subnet's windows-desktop run already observed, now by
     design rather than by race.
   - **`initialize` without `--ip` on Windows** is the contract's existing
     mode (`configureNetwork`'s "rely on relay-provided DHCP" branch): it
     writes the `artifacts`/`cwd` hosts entries, points every IPv4
     interface's DNS at the gateway (BOOT-03: DNS belongs to `initialize`),
     and waits for the relay's health endpoint for 60 s, as today.
   - **No lease.** Nothing stands in for it: no static address, no retry
     with `--ip`. The health wait fails, and on Windows its error line names
     the NIC's IPv4 addresses and their origin, so a missing lease reads as
     one: "relay at <gateway> not reachable within 60s: <alias> has
     169.254.x.y (WellKnown, no DHCP lease from the relay)", or the leased
     address when the lease came and the relay still did not answer.
     `warden-io` exits non-zero, `warden-run.ps1` logs the line to
     `warden-run.log` and COM1 and powers off, and the driver fails the
     build with no completion report and the console tail as the evidence,
     as it does for any guest that fails before it reaches the relay.
   - **The `executor-initialize` seam is unchanged.** `--gateway` and the
     optional `--ip` keep their contract on every platform
     (`overview.md`: `initialize --gateway=<relay> [--ip=<static>]`); only
     warden's own Windows launcher stops passing the optional flag. The
     Linux seeds (`network-config`), the direct-boot inits and vz's
     launcher are untouched. `warden-io`'s Windows `--ip` branch, now
     unused by warden, stays as it is for an orchestrator that passes it
     (question a12 weighs making it strict).
   - **The winguest seed** carries no guest address afterwards, only the
     relay's (`--gateway`), so BOOT-06 as amended holds more simply on
     Windows: the guest's address comes from the relay's lease, a channel,
     not from a value in a boot asset.

   *Rejected:* making the NIC static in `initialize` (r1's Decision 8,
   changed at review r1: two channels then configure one NIC). Leaving
   `--ip` in the script with today's silent `New-NetIPAddress
   -ErrorAction SilentlyContinue` (a provided value that may or may not be
   applied, depending on which channel answered first). A static fallback
   when no lease arrives (minimal magic: the build fails clearly instead of
   recovering on a second path). Stopping the relay from answering Windows
   guests (the responder is guest-agnostic by design).

9. **Warden and the relay boot VHDX refuse each other across this change, by
   a new value of the existing stamp** (Decision 6 of build-link-subnet;
   UX-04). The new init cannot work without a seed, and an older warden
   sends none; an older init ignores the seed and sits at `.2` without a
   token. So `refresh-cache.sh` writes `WARDEN_LINK_CHANNEL=2` into
   `relay-boot.link-channel`, and the Hyper-V driver compares that file with
   its own constant, `relayBootChannel = 2`, instead of `buildlink.Channel`.
   A new warden refuses a disk stamped `1` (the fix is `refresh-cache.sh`);
   an older warden, which expects `1`, refuses a disk stamped `2` with the
   same fix, which, run from that warden's tree, builds the matching older
   disk. Both are refused before any VM boots, and doctor reports `BLOCKED`.
   The qemu and Firecracker direct-boot initramfs and the vz image keep
   `buildlink.Channel = 1`: nothing about how they read the link changes, and
   bumping the shared constant would make their users rebuild for nothing.
   `baked-asset-staleness` (queued) later replaces both stamps with a source
   digest per asset.

   *Rejected:* a second stamp key or file (an older warden ignores what it
   does not read, accepts the new disk, and times out on the claim). Bumping
   `buildlink.Channel` (rebuilds on three drivers that did not change). A
   seedless fallback in the init (Decision 2).

10. **BOOT-06 takes the per-build network-value invariant, and BOOT-07's
    surface list catches up** (questions a7, a8; FLOW-14). After Decisions 2
    to 6, no boot asset or image carries a network value the orchestrator
    chooses per build, on any driver: the relay gets its values from its
    claim, its command line or its seed, the guest from its seed, its
    command line or the relay's lease. build-link-subnet left the
    extension's wording to this item because Hyper-V's relay was the one
    place it did not hold (its Bootstrap comparison, item 2); it now holds,
    so it lands with no deviation-register entry. BOOT-07's bootstrap
    surface gains the relay VM's init and build tools (`tools/relay-vm/`,
    which install BOOT-01's isolation rules), the qemu direct-boot launcher
    (`tools/build-vm/`, the twin of the listed `tools/firecracker-vm/`), the
    relay's DHCP responder (`cmd/relay/dhcp*.go`, a guest-facing endpoint)
    and this feature's relay seed (`driver/hyperv/relayseed*.go`).
    build-link-subnet's Consolidation proposed the middle two "for a process
    item or the next feature that edits the surface"; this feature edits
    `tools/relay-vm/` and adds the seed. The wording is in **Rule changes**.

11. **No runtime fallback anywhere in the design; contingencies are named
    and chosen by a return to Design** (question a13; Jeff's principles,
    Prior art). Every place r1 or the code had a second path is settled:

    | Where | Before | After |
    |---|---|---|
    | The relay's channel (Decision 1) | r1: "SMBIOS as the fallback" | The DVD seed only. SMBIOS is a named contingency in **Rollback**: if native validation finds the DVD path does not hold, the feature returns to Design (FLOW-06) and the review chooses it. No build tries one then the other |
    | The relay's address (Decision 2) | init: static defaults, "DHCP failed, use the static defaults" | Each driver's one channel; a missing or bad value stops the init with a console line |
    | The relay's resolvers (Decision 6) | init: `8.8.8.8` | `[vm] dns` exclusively when set, else the host's; neither is a refusal |
    | The durable segment (Decision 7) | r1: "one build at a time as the fallback if VLANs fail" | A per-build switch; all busy is a refusal |
    | The Windows guest's address (Decision 8) | the seed's `--ip` racing the lease | The lease only; none is a failed build with a line saying so |
    | The NAT a build uses (Decision 5, confirmed at r1) | the default prefix, whatever setup made | What exists, in a fixed order: the dev switch's NAT when setup made one (the user's choice, used exclusively), else the warden NAT on the default prefix. This is a choice of what the host has, decided before any VM, not a retry after a failure, so it stays as confirmed |
    | Version skew (Decision 9) | none | A refusal both ways, no compatibility mode |

    *Rejected:* keeping SMBIOS as an automatic second channel (two code
    paths in the trust boundary, and a build's behaviour would depend on
    which one worked).

12. **Out of scope.** The Linux build guest on Hyper-V (still not built).
    The relay binary baked into the VHDX, which is a staleness matter for
    `baked-asset-staleness`, not a network value. A `specs/relay-claim/v1.md`
    contract document (named in build-link-subnet, not queued). An upstream
    SSRF filter for vm mode (deferred to `security-suite` by build-link-subnet
    Decision 10). IPv6 on the NAT (the init disables IPv6).

## Questions

### Blocking

None. Every open choice has a recommendation the design proceeds on, and
each is confirmed in the design review (FLOW-07, FLOW-19).

### Assumed

**Settled at review r1 (0df0a64).** Confirmed as asked: a1 (the seed ISO;
its SMBIOS wording is clarified as a contingency in a13), a2 (relay
addresses in VM Notes under a host lock), a5 (the NAT read back from the
host), a7 (BOOT-06's extension, rule change approved), a8 (BOOT-07's surface
list, rule change approved), a9 (seams, relationships, validation). Turned
into changes, now Decisions 6 to 8: a3 (an optional `[vm] dns`, used
exclusively, refuse if none), a4 (c) (a per-build Private switch for durable
builds), a6 (c) (no `--ip` on Windows, the relay's lease alone). They are not
asked again.

**New in r2: the delta since 0df0a64, for the re-review.**

- **a10. `[vm] dns`: shape, validation and which drivers read it**
  (Decision 6). Proceeding with: an optional list of IPv4 addresses, no flag
  and no environment variable; when set, used exclusively (never merged with,
  nor followed by, the host's); syntax and address class checked at config
  load, reachability (host-local, NAT prefix, build link) at build start and
  in doctor, any unusable entry refusing the build rather than being
  dropped; read by hyperv and firecracker (both select the relay's resolvers
  from the host's today); a qemu or vz build with it set refused at startup;
  the container driver ignoring `[vm]` as today. Options: (a) as stated; (b)
  hyperv only, other VM drivers refusing it; (c) every VM driver honours it
  (qemu via `warden.dns` on its relay's command line, vz via a new host-mode
  relay resolver option), widening this feature to qemu and vz validation;
  (d) as (a), plus a `--dns` flag on build, shell and doctor. Recommended:
  (a), with (c) as a follow-up if wanted.
- **a11. The durable strategy's per-build switches** (Decision 7).
  Proceeding with: the elevation at setup, not per build: `warden setup
  --driver hyperv --slots N` (1 to 64, default 4) makes the Private switches
  `<network>-0` to `<network>-<N-1>` and retires the single shared
  `<network>` switch when nothing is attached to it; a durable build runs
  at the Hyper-V Administrators tier, takes the lowest unattached slot under
  the `hyperv-network` host lock, and holds it only by its relay VM's
  attached NIC (no lock for the build's lifetime; freed when the VM is
  removed, kept by `--keep-vms`); all slots busy, or no slots on the host,
  is a fixable refusal. Options: (a) as stated; (b) as stated with
  Firecracker's default of 16 slots; (c) create a Private switch per durable
  build, after a native spike shows Hyper-V Administrators may (otherwise
  durable builds would need elevation, which removes the strategy's
  purpose); (d) as stated, but keep the old `<network>` switch in place
  instead of removing it. Recommended: (a).
- **a12. The Windows guest on the relay's lease** (Decision 8). Proceeding
  with: `warden-run.ps1` passes `--gateway` only, on Hyper-V and qemu; the
  `executor-initialize` contract is unchanged (`--ip` stays optional on every
  platform, and its Windows branch is left as it is for other
  orchestrators); no lease is a failed build whose `warden-io` line names
  the NIC's address and origin, with no static fallback. Options: (a) as
  stated; (b) as (a), and make `--ip` on Windows strict for whoever passes it
  (disable DHCP, set the address, fail if it does not stick: r1's Decision 8
  code, unused by warden); (c) as (a), and remove `--ip` on Windows from the
  contract (an `executor-initialize` seam change with a deprecation, UX-03).
  Recommended: (a); (b) belongs with the `design-principles` item if it
  wants `--ip` itself to be explicit everywhere.
- **a13. No runtime fallbacks; contingencies named** (Decision 11).
  Proceeding with: SMBIOS (a1) is a contingency chosen by a return to
  Design if the DVD path fails native validation, never a second channel a
  build tries; r1's "one durable build at a time" contingency goes with the
  VLANs; the NAT choice (a5) is a fixed order over what the host has, not a
  fallback, and stays as confirmed. Options: (a) as stated; (b) keep SMBIOS
  as an automatic second channel when the DVD is missing; (c) drop the SMBIOS
  contingency altogether (a DVD failure in validation is then an open
  return to Design). Recommended: (a).

## Design

### A Hyper-V build, after the change

```
StartBuild
  caps := Detect(sw); strategy := caps.NetworkStrategy()
  resolveBuildDisks
    relay boot VHDX, its stamp == relayBootChannel (2)      else Fixable(refresh-cache.sh)
    CheckHostLink(link)
  provisionBuildNetwork -> nat {name, prefix, hostIP}, buildSwitch (ephemeral only)
    durable:   VerifyNetwork(sw) -> read back <sw>-nat; slot switches <sw>-<i> exist
                                                             none -> Fixable(warden setup)
    ephemeral: EnsureNetwork(per-build Private switch; NAT = <devsw>-nat if present,
               else one owning DefaultNATPrefix, else new) -> read back
  nat.prefix overlaps link?                                  -> Fixable (UX-04)
  dns := req.DNS set ? resolvers.Check(req.DNS, hostLocal, nat.prefix, link)
                                                             unusable entry -> Fixable
                     : resolvers.Select(hostDefaultRouteDNS(), hostLocal, nat.prefix, link)
                                                             none -> Fixable ([vm] dns named)
  token := RandomToken()
  with HostLock("hyperv-network"):
    relayIP := lowest free host in nat.prefix (not net, bcast, hostIP,
               nor any warden-relay-* VM's Notes)
    durable: buildSwitch := lowest <sw>-<i> with no VM adapter attached
                                                             none -> Fixable (holders, --slots)
    seed ISO <- warden.seed=1 warden.net=static warden.nat_ip=relayIP
                warden.nat_prefix=len(nat.prefix) warden.nat_gw=nat.hostIP
                warden.dns=dns warden.claim_token=token
    CreateRelayVM(..., DVD=seed ISO, build NIC on buildSwitch,
                  Notes="warden-relay-ip=relayIP warden-relay-switch=buildSwitch")
  StartVM(relay)
  ClaimRelay(relayIP:8301, token, build_link)                Bearer on claim and tunnel
  CreateVM(build on buildSwitch); StartVM(build)             unchanged order
    guest: DHCP lease from the relay; warden-run.ps1 -> initialize --gateway=<relay>
  ... wait, teardown: RemoveVM(relay) frees relayIP and the slot
```

### The relay VM init (`tools/relay-vm/init`)

```
read_keys(words):            # one parser for both sources; never evaluated
  for kv in words: case warden.{net,nat_ip,nat_prefix,nat_gw,dns,claim_token,seed}=*
  validate each value (dotted quad, 1..30, [0-9a-f]{64}, dhcp|static) or stop
read_keys $(cat /proc/cmdline)
if HYPERV:
  load storage + CD + isofs modules
  wait <= 10 s for the CD device; mount -o ro; copy warden-relay.conf to /tmp; umount
  missing or warden.seed != 1 -> say "no relay seed (WARDENRELAY) ..."; idle
  read_keys $(cat /tmp/warden-relay.conf)
build-if up (unchanged); upstream:
  static: nat_ip, nat_prefix, nat_gw all required, else say + idle
  dhcp:   udhcpc; failure -> say + idle   (no static fallback)
iptables (unchanged)
resolv.conf: warden.dns if given; else what DHCP wrote; empty -> say + idle (no 8.8.8.8)
WARDEN_CLAIM_TOKEN=$CLAIM_TOKEN /sbin/relay -mode vm -build-if $PRIVATE_IF   (unchanged)
```

### Every site that changes

| Site | Change | Decision |
|---|---|---|
| `tools/relay-vm/init` | one key reader for cmdline and seed, value checks, the Hyper-V seed mount, the baked defaults and fallbacks removed, progress lines name keys not values | 1, 2 |
| `tools/relay-vm/hyperv/build.sh` | the storage, CD-ROM and ISO 9660 modules copied; the "no block device" comment rewritten | 1 |
| `tools/relay-vm/hyperv/refresh-cache.sh` | writes `WARDEN_LINK_CHANNEL=2` | 9 |
| `driver/hyperv/relayseed.go` (new, pure) | stage `warden-relay.conf` from a `RelaySeed{IP, PrefixLen, Gateway, DNS, Token}`; validation mirrors the init's | 1, 2 |
| `driver/hyperv/winseed_windows.go` | `makeSeedISOWindows` reused for the relay seed (label `WARDENRELAY`) | 1 |
| `driver/hyperv/relayvm.go` | `RelayVMSpec` gains `SeedISO`, `Notes` and the build NIC's switch; the script adds the DVD and the Notes while the VM is off | 1, 4, 7 |
| `driver/hyperv/provision*.go` | `EnsureNetwork`/`VerifyNetwork` return the NAT's prefix and host IP and verify a slot switch; the ephemeral NAT choice prefers the dev switch's NAT; listers of `warden-relay-*` Notes and of the adapters attached to each slot switch | 4, 5, 7 |
| `driver/hyperv/setup.go`, `capability*.go` | setup makes `<network>-0..N-1` (`--slots`, default 4), trims unattached extras, retires the old `<network>` switch when unattached; `DevSwitchPresent` probes the slots | 7 |
| `driver/hyperv/relayhost.go`, `driver_windows.go` | `defaultRelayNATIP` goes; `StartRelay` takes the NAT, resolvers and link, allocates the address and (durable) the slot under the lock, writes the seed, claims with the token; `provisionBuildNetwork`'s durable case no longer names one build switch | 3, 4, 7 |
| `driver/hyperv/driver_windows.go`, `doctor.go` | NAT read back, overlap refusal, resolver check or selection before any VM; doctor prints NAT, host IP, resolvers and their source, slots and their holders, stamp | 5, 6, 7, 9 |
| `driver/hyperv/buildassets.go` | the relay-boot stamp compared with `relayBootChannel` | 9 |
| `driver/resolvers` (new), `driver/firecracker/posture.go`, `vm.go` | the shared `Select` and `Check`; Firecracker calls them with its own source and fix text, and uses `[vm] dns` when set | 6 |
| `driver/driver.go` (`BuildRequest`) | a `DNS []netip.Addr` field, from `[vm] dns` | 6 |
| `cmd/warden/config.go`, `host.go`, `build.go` | `[vm] dns` parsed and validated at load; refused for qemu and vz at startup; `--slots` accepted for hyperv (1 to 64, default 4) and its help text; `--nat-prefix`/`--host-ip` help: used by every build | 5, 6, 7 |
| `driver/winguest/run.go`, `winguest.go` | `WardenRunPS1` passes `--gateway` only; `BuildGuestCIDR` goes | 8 |
| `cmd/warden-io/platform_windows.go`, `initialize.go` | on Windows, a failed relay wait names the NIC's IPv4 addresses and origin (no lease reads as one); the `--ip` branch unchanged | 8 |
| `docs/development/bootstrap.md`, `tools/tier1/rules.yaml` | BOOT-06 and BOOT-07 as approved | 10 |
| docs: `overview.md`, `architecture.md`, `network-isolation.md`, `guide/cli.md`, `guide/configuration.md`, `guide/drivers.md`, `tools/relay-vm` READMEs | the relay seed, the token, per-build address, the durable slots, NAT read back, `[vm] dns` and the resolvers, the Windows guest on the lease (UX-01, FLOW-15) | all |

### Compatibility

| Warden | Relay boot VHDX | Outcome |
|---|---|---|
| new | new (stamp 2) | the seed, the token, everything above |
| new | old (stamp 1, or no stamp) | refused before any VM: `refresh-cache.sh` |
| old | new (stamp 2) | refused before any VM by the old warden's stamp check (it expects 1): `refresh-cache.sh` from its own tree |
| third-party orchestrator | new, booted with no seed | the init stops with a console line; no relay starts. A third party passes the same keys in a `WARDENRELAY` ISO (documented in `tools/relay-vm/README`) |
| new | durable network from an older setup (one `<network>` switch, no slots) | durable builds refused before any VM: run `warden setup --driver hyperv` (elevated), which makes the slots and retires the old switch; elevated and service builds unaffected |
| old | durable network from a new setup (slots, no `<network>` switch) | the old warden's durable strategy finds no switch and is refused with its own "run `warden setup`" message; its setup recreates `<network>`, which the new warden's setup later retires again |

qemu and Firecracker relay initramfs are not stamped and need no rebuild:
an old init still works with the new drivers (they changed nothing they
send), and a rebuilt one behaves the same on every path they use.

## Cross-driver impact

- **hyperv.** Everything in Decisions 1 to 9. Builds need one
  `refresh-cache.sh` (the stamp says so), and a durable host one elevated
  `warden setup --driver hyperv` (for the slots). A host whose only
  resolvers are loopback or host-local addresses now gets a fixable refusal
  naming `[vm] dns` where it used to get `8.8.8.8`. A durable network set up
  on a non-default prefix starts working. Concurrent builds, durable or
  ephemeral, no longer collide. The Windows guest keeps the relay's lease.
- **qemu.** The shared relay init loses fallbacks a qemu relay never takes;
  its claim, token and command line are unchanged. A qemu Windows guest runs
  the same `warden-run.ps1`, so it too drops `--ip` and keeps the relay's
  lease (Decision 8). A build with `[vm] dns` set is refused at startup
  (Decision 6). Tracked: qemu-any (relay init), macos-qemu (Windows guest).
- **firecracker.** The shared relay init (as qemu; Firecracker passes every
  key on its command line, so none of the removed defaults applied). Its
  resolver selection moves to `driver/resolvers` with no behaviour change
  when `[vm] dns` is unset; when it is set, the relay gets that list on its
  command line instead, checked by the same rule. CI tier 2
  (`firecracker.yml`, whose cache key hashes `tools/relay-vm`) rebuilds the
  relay initramfs and runs both builds; `[vm] dns` is covered at tier 0.
  `--slots` keeps its Firecracker meaning and default.
- **vz.** Only the startup refusal of a build with `[vm] dns` set, made in
  `cmd/warden` and covered at tier 0. Its relay runs in host mode and never
  boots the init; nothing in `driver/vz` or `cmd/relay` host mode changes.
- **container.** Unaffected: it ignores `[vm]` (as it does
  `[vm] build_link`), its relay runs in container mode with its token in the
  container's environment, and nothing in `driver/container` changes.

## Bootstrap comparison

Required because BOOT-06 gains an invariant (rule change BOOT-06). Sign-off
must accept this section explicitly (BOOT-07).

### Current model

The diagram in [`bootstrap.md`](../development/bootstrap.md), unchanged since
build-link-subnet: (1) isolate, the sole network path is the relay, which
exists before any guest code runs; (2) deliver the executor per build; (3)
start it, everything else over the relay. In step (1), every relay gets its
build link from the claim. What the relay needs to be *reached* comes per
build on qemu (command line), Firecracker (command line) and vz
(environment), and on Hyper-V from defaults baked into the relay VM's init
(`192.168.240.2/20`, gateway `.1`, `8.8.8.8`, no token). Per-driver mapping
rows: qemu Linux, firecracker Linux, qemu and hyperv Windows, vz macOS,
container. Deviation register: D2 (vz).

### Proposed model

The same diagram, mapping rows and deviation register. Step (1) on Hyper-V
gains a per-build relay seed and, on the durable strategy, a per-build
switch; step (3) on Windows takes its address from the relay's lease alone
(r2: the BOOT-01, BOOT-03 rows and this diagram change; the BOOT-06 row,
approved at 0df0a64 with its rule change, does not):

```
  (1) isolate ------------------------------>  sole network path: the relay
      Hyper-V: write the relay seed (NAT       (relay VM reads its seed, sets its
      address, resolvers, claim token),        upstream, starts the relay with the
      boot the relay VM, claim it with the     token; durable builds take their own
      token and the build link; only then      Private switch from setup's slots)
      boot the guest

  (3) start -------------------------------->  launcher: address from the seed,
                                               command line or relay's lease (on
                                               Windows, the lease alone); then
                                               `warden-io initialize --gateway`
```

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | Topological; but concurrent durable Hyper-V builds share one Private segment, so a build's network path reaches other builds' relays and guests | Topological per build on every strategy: a durable build's relay and guest are the only VMs on a Private switch, taken from setup's slots | hyperv durable: per-build switch; others: none |
| BOOT-02 the executor is the only out-of-band input | Seeds, `CIDATA`, tar copy, direct-boot initramfs; vz baked (D2) | Unchanged. The relay seed goes to the relay VM, not the build environment, and carries no build input | none |
| BOOT-03 one start, then hands off | Addressing in the launcher from the seed, a fixed address or the relay's lease; trust, DNS and the rest in `initialize` | Unchanged wording. On Windows the address comes only from the relay's DHCP lease, which BOOT-03 already names as the platform's own configuration; `initialize` gets `--gateway` alone and sets DNS, as the rule assigns it | qemu and hyperv Windows: no `--ip`; the NIC keeps the lease (`PrefixOrigin Dhcp`) |
| BOOT-04 build inputs come from the relay | CA, script and context fetched by the executor | Unchanged | none |
| BOOT-05 setup is the executor's domain | Baseline plus `/bootstrap` policy | Unchanged. The relay's resolvers are the relay's upstream, not the guest's setup; the guest's DNS is still the relay, set by `initialize` | none |
| BOOT-06 images carry hooks, not content | Generic hooks, nothing per-build, nothing tied to a warden version; but the Hyper-V relay VM's init bakes its upstream address, gateway and resolver, and every Hyper-V relay shares them | Adds: no boot asset or image, the relay VM's included, carries a network value the orchestrator chooses per build, and none bakes a default that stands in for a channel. Holds on every driver after this feature | hyperv: the relay seed; qemu, firecracker: the init's unused defaults removed |
| BOOT-07 the model changes by a signed comparison | Surface list without `tools/relay-vm/`, `tools/build-vm/`, the relay's DHCP responder | Unchanged rule; surface list extended (rule change BOOT-07) | tier 1 hint fires on those paths |

### Gains

Hyper-V's relay becomes like every other driver's: a per-build address, a
per-build token, the user's resolvers or the host's, the user's NAT prefix. The claim race
is closed: a local process or another build's orchestrator gets 401 and the
relay stays claimable. Concurrent Hyper-V builds work, and durable builds stop
sharing a segment, so each build's ledger records only what its own guest
received. The Windows guest's address has one source, the relay's lease. BOOT-06 then states, for every driver, the principle
build-link-subnet established for the link, and the tier 1 hint covers the
files where it could regress.

### Costs

The relay VM gains a block device and the kernel modules to read an ISO 9660
image (code inside the trust boundary that parses host-written data; the
build VM cannot reach the relay VM's disks or devices). The relay seed holds
a secret on the host's disk for the build's lifetime, under the build's work
directory. The Hyper-V host's DNS configuration, or `[vm] dns`, now decides
the relay's resolvers, so a host with only loopback resolvers gets a refusal
it did not get before (its fix names `[vm] dns`). A durable host needs one
more elevated `warden setup` for its slots, and concurrent durable builds are
capped at the slot count. A Windows guest that gets no lease fails its build
instead of falling back to a static address. A third-party orchestrator that boots warden's relay VHDX must
write a seed. One more `refresh-cache.sh` for every Hyper-V user. The record
loses nothing; the relay seed itself is not recorded, because it holds the
token (the relay's upstream fetches are recorded as always).

## Rule changes

### BOOT-06 (amend), `docs/development/bootstrap.md`

**Why:** build-link-subnet established that no asset bakes the build link,
and left the general statement for this item because Hyper-V's relay VM was
the one place it could not hold (its Bootstrap comparison, item 2; its
Consolidation). After Decisions 2 to 6 it holds on every driver, so the rule
can say it, and a later change that bakes an address, a resolver or a token
into a boot asset is a model change that needs a comparison.

**Current wording:**

> A static or prepared image may carry a generic hook that finds and starts the
> per-build executor (the Windows `warden-run` task, cloud-init itself). It
> carries nothing per-build and nothing tied to a warden version, so a warden
> upgrade never needs an image re-prepare to take effect.

**Proposed wording** (the first paragraph unchanged, a second added):

> A static or prepared image may carry a generic hook that finds and starts the
> per-build executor (the Windows `warden-run` task, cloud-init itself). It
> carries nothing per-build and nothing tied to a warden version, so a warden
> upgrade never needs an image re-prepare to take effect.
>
> No boot asset or image, the relay VM's included, carries a network value
> the orchestrator chooses per build: an address or prefix, a gateway, a
> resolver or a claim token. The relay gets those from its claim, its kernel
> command line or a per-build relay seed; the build environment from its
> seed, its kernel command line or the relay's DHCP lease. A boot asset may
> carry the launcher that reads such a channel, but no default that stands
> in for it.

**Severity and check:** unchanged, MUST and agent-judgment.

**Enforcement:** none beyond the agent's judgment; BOOT-06 has no tier 1
check. build-link-subnet's tier-0 stray-literal scan already refuses the old
and new link literals outside its allowlist; this feature adds a tier-0 test
that `tools/relay-vm/init` holds no IPv4 literal (Validation plan), which is
a test, not rule enforcement.

**Bootstrap comparison (BOOT-07):** the BOOT-06 row of the **Bootstrap
comparison** is the model change this wording carries; the review accepts
the two together.

### BOOT-07 (amend), `docs/development/bootstrap.md` and `tools/tier1/rules.yaml`

**Why:** the bootstrap surface is the set of paths on which tier 1 hints
"this touches the bootstrap model; does `bootstrap.md` change?". Four paths
that now carry the model are missing: `tools/relay-vm/` (the relay VM's init,
which installs BOOT-01's isolation rules and now reads the relay seed, and
its Hyper-V build tools), `tools/build-vm/` (qemu's direct-boot launcher,
listed for Firecracker as `tools/firecracker-vm/` but not for qemu),
`cmd/relay/dhcp*.go` (the relay's DHCP responder, a guest-facing endpoint
since build-link-subnet) and `driver/hyperv/relayseed*.go` (this feature's
seed). build-link-subnet's Consolidation proposed the second and third for
"the next feature that edits the surface".

**Current wording** (the automated-part paragraph's second sentence):

> The bootstrap surface is the executor's setup
> (`cmd/warden-io/initialize*.go`, `baseline.go`, `trust_*.go`, `jks.go`), the
> bootstrap config (`bootstrap/`), the relay's guest-facing endpoints, and each
> driver's isolation, executor delivery and start code
> (`driver/container/container.go`, `driver/container/images*`, `cmd/warden-pause/`,
> `driver/linuxguest/`, `driver/winguest/`, `driver/hyperv/winseed*.go`,
> `driver/vz/image.go`, and the firecracker isolation, delivery and start files
> (`driver/firecracker/slots*.go`, `setup.go`, `posture.go`, `vm*.go`,
> `deliver_linux.go`) and `tools/firecracker-vm/`).

**Proposed wording:**

> The bootstrap surface is the executor's setup
> (`cmd/warden-io/initialize*.go`, `baseline.go`, `trust_*.go`, `jks.go`), the
> bootstrap config (`bootstrap/`), the relay's guest-facing endpoints
> (`relay/controlplane.go`, `cmd/relay/dhcp*.go`), the relay VM's init and
> build tools (`tools/relay-vm/`), and each driver's isolation, executor
> delivery and start code (`driver/container/container.go`,
> `driver/container/images*`, `cmd/warden-pause/`, `driver/linuxguest/`,
> `driver/winguest/`, `driver/hyperv/winseed*.go`,
> `driver/hyperv/relayseed*.go`, `driver/vz/image.go`, `tools/build-vm/`, and
> the firecracker isolation, delivery and start files
> (`driver/firecracker/slots*.go`, `setup.go`, `posture.go`, `vm*.go`,
> `deliver_linux.go`) and `tools/firecracker-vm/`).

(`relay/controlplane.go` is already in `tools/tier1/rules.yaml`; the
amendment names it in the rule too.)

**Severity and check:** unchanged, MUST-review and automated +
agent-judgment.

**Enforcement:** `tools/tier1/rules.yaml`, `bootstrap.surface` gains
`tools/relay-vm/`, `tools/build-vm/`, `"cmd/relay/dhcp*.go"` and
`"driver/hyperv/relayseed*.go"`. The `bootstrap-surface` check then hints on
a diff touching them without touching `bootstrap.md`. It adds hints, removes
none, so tier 1 built from the merge base reports nothing the branch's no
longer does (FLOW-14's comparison passes trivially). This feature's own diff
touches `bootstrap.md`, so its hint is satisfied.

## Security note

**Boundary.** env-relay (GLOBAL-06), plus the host's claim path to the
relay. Four postures change: who can claim a Hyper-V relay (the token), what
a durable build's guest can reach on its segment (its own switch), what the
relay VM reads before its relay starts (the seed), and where the relay's
names come from (`[vm] dns` or the host's resolvers). RELAY-04: the relay binary
is unchanged; the relay VM's init changes for every vm-mode relay.

**The claim.** Updated row of build-link-subnet's claim-endpoint table:

| Driver | Claim endpoint | Reachable by | Token |
|---|---|---|---|
| hyperv | `<relayIP>:8301` on the NAT switch, `relayIP` allocated per build | local processes (through the host vNIC), and other relay VMs on that NAT switch, never a build VM (the init drops 8301 on the build interface) | **per build**, in the relay seed ISO under the build's work directory, readable by the user, administrators and vmms |

A claim or tunnel without the token gets 401 and does not consume the relay
(`relaycfg`), so the race build-link-subnet weighed as a denial of service is
closed: a local process can still connect, but not claim. The token is 256
random bits compared in constant time. It is exposed to fewer readers than
qemu's (the relay's argv, readable by every local user) and is never logged,
recorded, or written to COM1. Another relay VM on the NAT switch runs the
same trusted image and holds its own token. A host process that is not an
administrator cannot add an address on the NAT switch to impersonate a relay
and harvest a token; an administrator is outside this boundary, as on every
driver.

**The segment.** Before: concurrent durable builds share `warden-dev` at L2;
build VM A can ARP for and reach relay B (`100.127.255.253` on both), take
B's DHCP offer, send its traffic through B (into B's ledger) or reach build VM
B directly. After: each durable build's relay and guest are attached to a
Private switch that no other VM is attached to, as an ephemeral build's
per-build switch is, so the segment holds exactly one relay and one guest.
A guest cannot change which switch its adapter is on. The slot is taken
under the host lock from what Hyper-V reports attached, so two builds never
take one switch; a VM a user attaches to a busy slot by hand is outside this
boundary (Hyper-V Administrators can already reach every VM), and
`VerifyNetwork` refuses a slot that has a foreign adapter on it when the
build starts.

**The seed.** Host-written, read once by the init, from a device only the
relay VM has. The build VM cannot reach the relay VM's devices, the NAT
switch, or the host. Each value is checked against a strict pattern before
use, the file is never evaluated, and a bad value stops the init instead of
starting a misconfigured relay. The new kernel modules parse an ISO 9660
image the orchestrator built; nothing the build controls reaches them.

**Resolvers.** The relay's upstream resolvers move from `8.8.8.8` to
`[vm] dns` when set, else the host's. A build's names now resolve as the user
or the host chooses, which can include internal names; the relay records
every answer it serves, as before. `[vm] dns` is config the user controls,
like `[vm] build_link`; the build cannot set it. Loopback, link-local,
host-local and in-NAT or in-link resolvers are excluded from the host's list
and refused in `[vm] dns`, so the relay never sends DNS to the host itself or
into the build link.

**What else the change could break, and what guards it:**

- *An address conflict on the NAT.* The allocator excludes the host IP and
  every address a `warden-relay-*` VM records, under a host-wide lock; a
  full prefix is a refusal, not a guess.
- *Version skew.* The stamp value refuses a new warden on an old disk and an
  old warden on a new disk, both before any VM boots (Decision 9).
- *A NAT prefix that shadows the link.* Refused at build start and in doctor,
  and still by the relay (build-link-subnet Decision 4).
- *The Windows guest on the lease.* The guest's only NIC faces the relay,
  so the only DHCP server it can hear is the relay's responder, which
  answers with the link's guest address only; a build VM cannot reach
  another build's responder (its own switch). No lease is a failed build,
  not a guest on an unexpected address.
- *Slot exhaustion.* All slots busy is a refusal before any VM, never a
  shared switch.

**TEST-08 should exercise:** a claim and a tunnel with a missing or wrong
token are refused with 401 and the relay is still claimable by the right
token, on Hyper-V; two concurrent durable builds each succeed on their own
slot switch, and from inside build VM A, ARP and ICMP to relay B's link
address and to build VM B, and a DHCP DISCOVER, get no answer from B; a
durable build with every slot busy is refused before any VM; two concurrent
ephemeral builds get distinct relay addresses; a relay seed with a malformed
value, or no seed, starts no relay; a NAT prefix overlapping the link, a
host with no qualifying resolver and no `[vm] dns`, and a `[vm] dns` entry
that is host-local or in the link, are each refused before any VM boots;
with `[vm] dns` set, the relay uses only those resolvers. These
go to `security-suite` at Consolidation, as build-link-subnet's did.

## Validation plan

- **Tier 0** (TEST-01: pure Go, table-driven, no host network).
  `driver/hyperv`: `relayseed` renders every key and refuses a bad IP,
  prefix, gateway, resolver or token; `buildRelayVMScript` adds the DVD, the
  Notes (address and slot) and puts the build NIC on the given switch, with
  escaping; the allocator over canned VM and adapter lists (lowest free
  address, skipping network, broadcast, host IP and recorded addresses, a
  full prefix refused with the holders named; lowest unattached slot, a
  slot with only a build VM attached counted busy, all busy refused with the
  holders and `--slots` named); setup's slot script (creates `<network>-i`,
  trims unattached extras, refuses to remove an attached one, retires an
  unattached `<network>` switch); `DevSwitchPresent` and the "no slots"
  refusal; `parseEnsureNetworkOutput` with the prefix and host IP; the NAT
  choice for each strategy; the overlap refusal; `StartRelay` end to end
  over the fake `Provisioner`: the token reaches both the seed and the
  claim, the claim address is the allocated one, `[vm] dns` (when given)
  is exactly the seed's `warden.dns` with the host's never read, and
  teardown removes the VM; the stamp check against `relayBootChannel` (1
  refused, 2 accepted, missing refused, each a `driver.Fixable` naming
  `refresh-cache.sh`); the Windows default-route DNS parser on canned
  output. `driver/resolvers`: `Select`'s every exclusion and Firecracker's
  existing cases moved over unchanged; `Check` refuses (not filters) each
  unusable entry and passes a usable list through in order.
  `driver/firecracker`: `[vm] dns` set reaches the relay's command line and
  the host's resolv.conf is not read; the fix texts name `[vm] dns`.
  `cmd/warden`: `[vm] dns` at load (IPv4 only, address classes, `[]`
  refused, project over user); a qemu or vz build with it set refused at
  startup; `--slots` accepted for hyperv (1 to 64, default 4) and still
  refused for other non-firecracker drivers. `driver/winguest`:
  `WardenRunPS1` runs `initialize --gateway=<relay>` with no `--ip` (both
  default and configured links); the qemu Windows seed carries the same
  script. `cmd/warden-io` (`platform_windows.go`, under its build tag,
  rendering only): the diagnostic for a failed relay wait renders the NIC
  addresses with their origin and says "no DHCP lease" for a `WellKnown`
  169.254 address; the `--ip` branch is unchanged. `tools/relay-vm/init` as
  text: one key reader for both sources, never evaluates, the seed read only
  under `HYPERV`, no IPv4 literal anywhere in the file, the token's value
  never on a `say` line. `refresh-cache.sh` writes stamp 2.
- **Lint.** `make lint` for linux (`driver/resolvers`, firecracker,
  `cmd/warden`) and windows (`driver/hyperv`, `driver/winguest`,
  `cmd/warden-io`) (FLOW-17).
- **Tier 1** (TEST-02): the `integration` workflow on the branch, since
  `driver/` and `cmd/` change.
- **Tier 2, CI.** `firecracker.yml` rebuilds its relay initramfs from the
  changed init (its cache key hashes `tools/relay-vm`) and runs its disk and
  direct-boot builds.
- **Tier 2, native (FLOW-09).** Blocking: **windows-hyperv**, as queue step
  `f6-validate-windows-hyperv`, after consolidation. Cases:
  1. *Upgrade.* With the VHDX from main (stamp 1), doctor reports `BLOCKED`
     and a build is refused before any VM, both naming `refresh-cache.sh`;
     after it, doctor is current (stamp 2).
  2. *Default build, ephemeral (elevated).* A Windows guest build succeeds
     and its ledger verifies. The relay's console shows its upstream from
     the seed (the allocated address, gateway, the host's resolvers) and no
     token value; the claim carried a token (driver log). In the guest,
     `warden-run.log` shows `initialize --gateway=<relay>` with no `--ip`,
     and the NIC ends `DHCP Enabled: Yes`, `PrefixOrigin Dhcp`, at the
     link's guest address.
  3. *Token.* While a build's relay waits for its claim, a claim and a
     tunnel from the host with no token and with a wrong one get 401, and
     the build then succeeds.
  4. *Concurrency.* Two ephemeral builds at once both succeed, with
     different relay addresses (their VMs' Notes and consoles).
  5. *Durable, non-elevated.* After `warden setup --driver hyperv
     --nat-prefix 192.168.250.0/24 --host-ip 192.168.250.1 --slots 2`
     (default NAT removed; on a host set up by main, the old `warden-dev`
     switch is retired, and a durable build before this setup is refused
     naming it), doctor prints the prefix, host IP and two free slots; a
     Hyper-V Administrators build uses that prefix (relay console) and
     slot `warden-dev-0` (relay VM Notes) and succeeds; an elevated build
     reuses the same NAT on its own ephemeral switch.
  6. *Durable concurrency and isolation.* Two durable builds at once take
     `warden-dev-0` and `warden-dev-1` and succeed; from build VM A (a build
     script that probes), ARP and ping to relay B's and build VM B's link
     addresses and a DHCP DISCOVER get no answer from B. A third concurrent
     durable build is refused before any VM, naming both slots' holders and
     `--slots`. With `--keep-vms`, the kept build's slot stays busy (doctor
     names it) until its VMs are removed.
  7. *Refusals.* A `[vm] build_link` inside the durable NAT's prefix is
     refused at build start and in doctor; with the host's default-route
     interface given only `127.0.0.1` as DNS server and no `[vm] dns`, the
     build is refused naming it and both fixes; `[vm] dns = ["<host's NAT
     IP>"]` is refused naming the entry; a relay VM booted by hand with no
     seed prints the no-seed line and starts no relay.
  8. *Resolvers.* With the host's DNS pointed at an internal resolver that
     answers a name public DNS does not, a build resolves it (no `[vm]
     dns`). With the host's DNS public and `[vm] dns` set to that internal
     resolver, a build resolves the internal name and the relay's console
     and doctor list only the key's resolver; with `[vm] dns` set to an
     address that runs no DNS server, the build's lookups fail (no fallback
     to the host's).
  9. *No lease.* After a `--keep-vms` build, its build VM moved to an empty
     Private switch and started again (the `warden-run` task runs at every
     start) logs a failed relay wait naming the NIC's address and origin
     (a 169.254 `WellKnown` address reads "no DHCP lease"), applies no
     static address, and powers off.

  Tracked (TEST-03): **qemu-any**, a qemu Linux build on a relay initramfs
  rebuilt from the branch (backlog
  `hyperv-relay-per-build-channel-qemu-relay`); **macos-qemu**, a qemu
  Windows guest ends on the relay's lease with no `--ip` (backlog
  `hyperv-relay-per-build-channel-qemu-windows`, after
  `qemu-windows-guest-package`). linux-kvm is covered by CI tier 2. Not run
  natively: macos-vz (only the `[vm] dns` startup refusal, tier 0),
  container-any and macos-finch (unaffected).
- **Queue steps.** `f6-validate-windows-hyperv` (`host:windows-hyperv`),
  added at Design r1 and kept by `wf back`, carries these r2 cases in its
  note, with no `needs`: a phase-6 step is not offered before Publish
  (FLOW-04), and seeding Publish makes it need `f6-consolidate` and
  `f6-open-pr` need it.
- **Security (TEST-08).** The Security note's cases are covered at tier 0
  and in the native cases above; Consolidation hands the list to
  `security-suite`.

## Implementation plan

Written by Plan (`f2-plan`) from the design the review approves (FLOW-19).
The outline it starts from, in order:

1. `[vm] dns` and shared resolver selection: the key in `cmd/warden`
   (load-time validation, the qemu and vz startup refusal, `BuildRequest`),
   `driver/resolvers` (`Select`, `Check`), Firecracker moved onto it and
   honouring the key (FLOW-15, UX-07, UX-08, TEST-01, GLOBAL-07).
2. Relay init and Hyper-V build tools: the key reader, value checks, the
   seed mount, defaults removed; the modules in `build.sh`; stamp 2 in
   `refresh-cache.sh` (BOOT-06, BOOT-01, RELAY-04, TEST-01).
3. `driver/hyperv`: relay seed, NAT read back and choice, allocator (address
   and durable slot), setup's slots and `--slots`, `RelayVMSpec` additions,
   `StartRelay` with the token, overlap and resolver refusals, stamp
   constant, doctor lines (ORCH-01, UX-04, UX-08, UX-09, GLOBAL-06, TEST-01).
4. The Windows guest on the lease: `WardenRunPS1` without `--ip`, the
   `warden-io` no-lease diagnostic (EXEC-04, BOOT-03, TEST-01).
5. `cmd/warden` setup flags and help text and the reference and concept
   docs (`cli.md`, `configuration.md`, `drivers.md`), the overview's
   pseudocode (UX-01, UX-03, ORCH-03, FLOW-15).
6. The approved rule changes: BOOT-06 and BOOT-07 in `bootstrap.md`, the
   surface in `tools/tier1/rules.yaml` (FLOW-14, BOOT-07).
7. Backlog entries for the tracked gaps (TEST-03, FLOW-09).

## Design review

Required (FLOW-19): `alters_relationships: true`, the `bootstrap-model`
seam, `security_boundary: env-relay`, blocking windows-hyperv validation,
assumed questions (a1 to a9 at r1; a10 to a13 open for r2), and rule changes
BOOT-06 and BOOT-07.

- **r1, 2026-11-08: changes requested** (Jeff, workflow-human, FLOW-19).
  Read at `0df0a64`, from a review page with the assumptions and rule
  changes first. Sent back to Design (FLOW-06).
  - **Rule changes approved as worded** (FLOW-14):
    - rule change BOOT-06 approved at 0df0a64 (with the Bootstrap
      comparison's BOOT-06 row);
    - rule change BOOT-07 approved at 0df0a64.
  - **Confirmed (a):** a1 (seed ISO, SMBIOS as the fallback), a2 (VM Notes
    under a host lock), a5 (NAT read back from the host), a7, a8, a9.
  - **Changes for Design r2:**
    - **a3, DNS:** the host's resolvers by default, plus an optional
      `[vm] dns` key. When the key is set, its value is used explicitly,
      with no fallback to the host's resolvers. When neither yields a
      resolver, the build is refused. This adds a config surface (UX-01,
      the reference docs).
    - **a4, the durable switch: (c)**, a per-build Private switch for each
      durable build instead of a shared switch with per-build VLANs. It
      needs elevation; Design weighs where that elevation happens (setup
      or build) and its privilege tier.
    - **a6, the Windows NIC: (c)**, drop `--ip` on Windows and rely on the
      relay's lease alone. Design checks the `executor-initialize` seam and
      the winguest seed, and what replaces the static fallback when no
      lease arrives.
  - **Re-review:** covers only what r2 changes since `0df0a64` (FLOW-19's
    delta rule). The BOOT-06 and BOOT-07 approvals stand unless r2 changes
    those entries.
- **Design r2, 2026-11-09** (FLOW-06; the finding is r1's notes above, read
  with Jeff's 2026-11-08 design principles). What it changes since
  `0df0a64`:
  - Decision 6 (a3): `[vm] dns`, used exclusively when set, validated and
    never filtered; the host's resolvers otherwise; a refusal naming the key
    when neither yields one. Read by hyperv and firecracker; a qemu or vz
    build with it set is refused. New surface: config (UX-01). Question a10.
  - Decision 7 (a4 (c)): the durable strategy's per-build Private switches,
    made by the elevated `warden setup --driver hyperv --slots N` (default
    4); builds stay at the Hyper-V Administrators tier and hold a slot only
    by their relay VM's attached NIC, taken under the existing host lock;
    the VLAN design and its one-at-a-time contingency are gone. New
    surface: `--slots` for hyperv. Question a11.
  - Decision 8 (a6 (c)): `warden-run.ps1` drops `--ip` on Hyper-V and qemu
    Windows guests; the guest keeps the relay's lease; no lease is a failed
    build with a `warden-io` line naming it, no static fallback; the
    `executor-initialize` seam is unchanged (the optional flag is simply not
    passed). Question a12.
  - Decision 11 (new): no runtime fallback in the design; a1's SMBIOS is a
    named contingency for a return to Design. Question a13.
  - Impact fields: `drivers` adds vz (the `[vm] dns` refusal only),
    `surfaces` adds config, `rules` adds UX-03 and UX-07; the native and
    tier-2 reasons for windows-hyperv and macos-qemu are reworded; the seams,
    `alters_relationships`, `security_boundary` and the blocking host are
    unchanged.
  - **Rule changes unchanged:** the BOOT-06 and BOOT-07 entries (front-matter
    and **Rule changes** wording) are as approved at `0df0a64`, so their
    approvals stand. The Bootstrap comparison's BOOT-06 row is unchanged; its
    BOOT-01 and BOOT-03 rows and the step (1) and (3) diagram change with
    Decisions 7 and 8 and are part of the re-review (BOOT-07).
  - Updated with them: Cross-driver impact, Security note, Validation plan
    (native cases 2, 5 to 9), the `f6-validate-windows-hyperv` step's note,
    the macos-qemu backlog entry, Rollback notes.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None yet. One named contingency, never a runtime fallback (Decision 11): if
native validation finds the relay seed's DVD path does not hold on Hyper-V,
the feature returns to Design (FLOW-06), which replaces it with SMBIOS
strings (Decision 1) for the review to approve. Rolling back the feature on
a host: an older warden's `warden setup --driver hyperv` recreates the single
`<network>` switch, and its `refresh-cache.sh` rebuilds the stamp-1 relay
disk (Compatibility).

## Archive summary

Filled in at Publish (FLOW-12).

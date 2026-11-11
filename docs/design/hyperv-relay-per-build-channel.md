---
feature: hyperv-relay-per-build-channel
status: draft
drivers: [qemu, firecracker, hyperv, vz, container]  # hyperv: the relay seed (pre-claim values only), the claim token, the per-build relay address, the NAT read back from the host, the relay's resolvers ([vm] dns or the host's) sent in the claim, the durable strategy's per-build switches made by setup, the Windows guest on the relay's lease, the relay-boot stamp. qemu: tools/relay-vm/init is shared by every vm-mode relay and loses its baked network defaults and its resolver keys (Decision 2), the qemu driver now sends the relay's resolvers in the claim ([vm] dns or the macOS host's, Decision 6), and its Windows guests run the same warden-run.ps1 and also drop --ip (Decision 8). firecracker: the shared init (as qemu), its resolver selection moves to a shared package, and the resolvers leave its relay's kernel command line (warden.dns) for the claim (Decision 6). vz: driver/vz sends the host-mode relay's resolvers in the claim ([vm] dns or the Mac's), where the relay used the Mac's system resolver (Decision 6). container: driver.BuildRequest (shared by every driver) gains DNS, which the container driver leaves empty, so it sends no upstream_dns and ignores [vm] as today; its relay takes the absent-field default, every usable nameserver of its own resolv.conf in order, where it used the system resolver first and 8.8.8.8 when none (Decision 6, RELAY-04: container, vm and host all change); its relay never boots tools/relay-vm/init
seams: [bootstrap-model, relay-claim]  # bootstrap-model: BOOT-06 gains the boot-asset and boot-channel invariant, and BOOT-07's surface list grows (Rule changes; Bootstrap comparison). relay-claim (r3, a10): relaycfg.Config gains upstream_dns, the relay's upstream resolvers, used in every mode (RELAY-08, GLOBAL-08); the claim token was already part of the protocol (Bearer on /v1/claim and the tunnel), Hyper-V only starts sending one. executor-initialize is unchanged: --gateway and the optional --ip keep their contract (overview.md: initialize --gateway=<relay> [--ip=<static>]); on Windows warden stops passing the optional --ip (Decision 8), a mode the contract already defines. relay-guest-api is unchanged: the relay's DHCP responder already serves the lease the Windows guest now keeps, and the guest's DNS is still the relay
surfaces: [cli, config, output]  # cli: `warden setup --driver hyperv` gains --slots (the durable strategy's per-build switches) and --nat-prefix/--host-ip now take effect on every build (UX-01); config: a new optional [vm] dns key, read by every VM driver (Decision 6, UX-01, UX-07, UX-08); output: doctor's NAT, relay-resolver and durable-slot lines on hyperv and its relay-resolver line on the other VM drivers, the new fixable refusals (UX-04)
security_boundary: env-relay  # the relay VM gains a per-build input (the seed), its claim gains a token on Hyper-V and the upstream resolvers on every VM driver, durable builds stop sharing one build segment (a build VM could reach another build's relay), and the relay's upstream resolvers become the user's choice or the host's. Also the host's claim path to the relay (the token); Security note
alters_relationships: true   # the bootstrap model's BOOT-06 changes (FLOW-02), a versioned seam changes (relay-claim: upstream_dns), the Hyper-V relay VM's pre-claim values move from defaults baked in its init to a per-build seed the orchestrator writes, every VM relay's resolvers move from its boot channel or platform into the claim, and the Windows guest's address moves from the seed's --ip to the relay's lease; FLOW-05 slot
design_review: required      # FLOW-19: rule change BOOT-06 reworded at r3 (needs re-approval), the relay-claim seam added at r3, alters_relationships, a security boundary, blocking native validation (two hosts added at r3), and open assumed questions (a14-a16)
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv end to end (relay seed ISO on the relay VM, the relay initramfs's new storage modules, claim token, per-build relay address on the NAT, the NAT prefix and host IP read back from the host, the relay's resolvers from [vm] dns or the host sent in the claim, the durable strategy's setup-made per-build switches, the relay-boot stamp bump) and the Windows guest on the relay's lease with no --ip, behaviour only a Hyper-V host exercises"}
  - {host: qemu-any, blocking: true, reason: "r3: reaches driver/qemu, which now selects the relay's resolvers from the macOS host (or [vm] dns) and sends them in the claim, where its relay used user-mode NAT's resolver from its lease; the shared tools/relay-vm/init also loses its fallbacks and resolver keys. A qemu Linux build resolving through the claim's resolvers, on a relay initramfs rebuilt from the branch, is the check"}
  - {host: macos-vz, blocking: true, reason: "r3: reaches driver/vz, which now sends the host-mode relay's resolvers in the claim ([vm] dns or the Mac's /etc/resolv.conf), where the relay used the Mac's system resolver; only a Mac exercises the vz host-mode relay's name resolution"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows guest runs the same warden-run.ps1, which no longer passes --ip, so it keeps the relay's lease (Decision 8); qemu Windows builds already fail on main (backlog qemu-windows-guest-package), and windows-hyperv validates the same script and lease (backlog hyperv-relay-per-build-channel-qemu-windows)"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows guest ends on the relay's lease (DHCP enabled, PrefixOrigin Dhcp, the link's guest address) with no --ip in warden-run.log (backlog hyperv-relay-per-build-channel-qemu-windows), after qemu-windows-guest-package is fixed"}
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, ORCH-01, ORCH-03, BOOT-01, BOOT-02, BOOT-03, BOOT-05, BOOT-06, BOOT-07, EXEC-04, RELAY-03, RELAY-04, RELAY-08, UX-01, UX-03, UX-04, UX-07, UX-08, UX-09, TEST-01, TEST-02, TEST-03, TEST-08, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19]
rule_changes:
  - {rule: BOOT-06, change: amend, paths: [docs/development/bootstrap.md]}
  - {rule: BOOT-07, change: amend, paths: [docs/development/bootstrap.md, tools/tier1/rules.yaml]}
concepts: [relay-seed, per-build-channel, boot-channel, claim-token, relay-claim, claim-protocol, relay-configuration, relay-vm-init, relay-address-allocation, nat-prefix, host-ip, upstream-dns, resolver-selection, vm-dns-config, durable-switch, network-slots, host-setup, two-vm-topology, network-isolation, launcher-stamp, stale-assets, windows-guest-network, dhcp, explicit-over-implicit, defaults, overrides]
related: [build-link-subnet.md, design-principles.md, firecracker-driver.md, hyperv-driver-plan.md, hyperv-windows-prepare.md, windows-driver-plan.md, unified-bootstrap.md, vm-driver-release-plan.md]
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
- Jeff's design principles (2026-11-08, after this design's review r1),
  now on `main` as GLOBAL-09 (least surprise: the default is the obvious
  choice), GLOBAL-10 (explicit over implicit: a provided value is used
  exclusively, a merge or fallback only when named) and GLOBAL-11 (minimal
  magic: predictable over resilient), with the template's **Defaults and
  overrides** section, by [`design-principles`](design-principles.md)
  (`beb51ba`, #104). Round 2 applied them to Decisions 6 to 8 and checked
  every other fallback in the design against them (Decision 11); round 3
  fills the section.
- RELAY-08 (`relay.md`, from the same item; Jeff's rule at this design's r2
  re-review): the relay is configured through its claim. Only its pre-claim
  values (its own address, prefix and gateway, and the claim token) reach it
  on its boot channel (a per-build seed or the kernel command line in vm
  mode); every other relay setting is a `relaycfg.Config` field, a setting
  travels on one channel, and the default for an absent optional field is
  named in `relaycfg`'s package doc. `design-principles` left BOOT-06's
  boot-asset half to this design's r3 wording (its Decision 8) and listed the
  relay's existing configuration outside the claim for the next design that
  touches each; this one touches its upstream resolvers.
- `relay/relay.go` and `relay/dns.go`, the relay's resolvers today: an
  `UPSTREAM_DNS` environment variable, else the first usable `resolv.conf`
  nameserver (`pickNameserver`), else `8.8.8.8:53`; the DNS handler asks the
  system resolver first and forwards to that address when it gives nothing,
  and upstream dials use `net.DefaultResolver`. Three sources, two unnamed
  fallbacks (GLOBAL-11); Decision 6 replaces them with one list from the
  claim.
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

Round 3 (after the r2 re-review, 2026-11-09) applies a10's change and
RELAY-08: the relay's upstream resolvers leave the seed and Firecracker's
kernel command line and become a claim field, `upstream_dns`, sent by every
VM driver (Decision 6, rewritten). Decisions 1 and 2 lose the resolvers
from the seed and the init, Decision 9 covers the new field's skew, Decision
10 and BOOT-06's proposed paragraph are reworded around RELAY-08's boot
channel, and Decision 11's resolver row follows. Decisions 3 to 5, 7 and 8,
confirmed at r1 and r2, stand unchanged.

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
   | Claim over hvsock (AF_HYPERV / AF_VSOCK) instead of the NAT | n/a | `hv_sock`, a vsock listener in `cmd/relay` | a per-host registry entry for the service GUID (Administrator), `go-winio` | Rejected: changes the claim transport, a new dependency, and an elevated step on every host; it would also not give the relay its upstream address |
   | Address from the NIC's static MAC | yes | none | Hyper-V Administrators | Rejected: 24 usable bits carry an address, not a token or resolvers |

2. **The seed carries exactly the relay's pre-claim values, in the key
   syntax the relay's kernel command line already uses, and the init loses
   every baked network default** (question a7; BOOT-06 as amended; RELAY-08).

   `warden-relay.conf` is one `warden.<key>=<value>` per line, the keys the
   init already parses: `warden.net=static`, `warden.nat_ip`,
   `warden.nat_prefix`, `warden.nat_gw` and `warden.claim_token`, plus
   `warden.seed=1` (the format version). These are RELAY-08's boot-channel
   values: the relay's own address (the one it accepts the claim on),
   prefix and gateway, and the claim token. Nothing else goes in it (r3): not
   the relay's resolvers, not the build link, not the collector (the claim
   carries each, RELAY-08), and not the build's inputs. The init reads its
   keys from one function that takes words from `/proc/cmdline` and, on
   Hyper-V, from the seed file, with the same rules as today (only known
   keys, never evaluated) and new value checks applied to both sources:
   `nat_ip` and `nat_gw` dotted quads, `nat_prefix` 1 to 30,
   `claim_token` `[0-9a-f]{64}` (what `driver.RandomToken` emits), `net` one
   of `dhcp` and `static`. A malformed value, a missing seed on Hyper-V, or a
   static upstream without `nat_ip`/`nat_prefix`/`nat_gw` stops the init
   with a console line on COM1 and starts no relay, so the orchestrator's
   claim times out with the relay's serial log as the evidence.

   The init's `NAT_IP=192.168.240.2`, `NAT_PREFIX=20`, `NAT_GW=192.168.240.1`,
   the "Hyper-V means static" default, the "DHCP failed, use the static
   defaults" fallback and the `${UPSTREAM_DNS:-8.8.8.8}` fallback all go,
   and so do the `warden.dns` key and the init's `resolv.conf` writing (r3:
   the relay takes its resolvers from the claim, Decision 6). An init that
   still finds `warden.dns` on its command line (a third-party orchestrator
   written against main) stops with a console line naming the key and
   `upstream_dns` in the claim, rather than ignoring a value someone set
   (GLOBAL-10). None of the removed defaults is reached by qemu (DHCP from
   user-mode NAT) or Firecracker (every key on its command line); a qemu
   relay whose DHCP fails now stops with a console line instead of
   addressing itself in Hyper-V's NAT prefix, which never worked on qemu.
   After this, no relay boot asset carries a network value, and no boot
   channel carries a relay setting other than its pre-claim values.

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

6. **The relay's upstream resolvers are a claim field, `upstream_dns`,
   which every VM driver sends: the `[vm] dns` list when it is set, used
   exclusively, and otherwise the host's, chosen by Firecracker's rule moved
   to a shared package; when neither yields a resolver the build is
   refused** (question a3, changed at r1; question a10, changed at r2;
   questions a14, a15; RELAY-08, GLOBAL-08, GLOBAL-10, UX-01, UX-04, UX-07,
   UX-08).

   r2 put the resolvers in the seed and on Firecracker's command line. They
   are not needed to reach or authenticate the claim, so under RELAY-08 they
   are claim configuration: the boot channel carries only the relay's own
   address, prefix, gateway and token (Decision 2).

   - **The claim field.** `relaycfg.Config` gains `UpstreamDNS []string`
     (JSON `upstream_dns`, omitted when empty): IPv4 addresses, port 53
     implied, in the order the relay asks them. It is optional on the wire,
     like `build_link`, and validated at the claim for every mode
     (`relaycfg.ParseUpstreamDNS`): present means a non-empty list of IPv4
     addresses, none unspecified, multicast or broadcast, none inside the
     claim's build link; anything else is refused with 400 (`ErrRefused`) and
     does not consume the relay. A relay that predates the field refuses a
     claim carrying it (`unknown field "upstream_dns"`, the decoder rejects
     unknown fields), which the driver turns into a `driver.Fixable`
     (Decision 9). The relay's console and log name the resolvers it took
     and their source (`claim` or `resolv.conf`); the claim token is never
     printed.
   - **What the relay does with it, in every mode (RELAY-04: container, vm
     and host all change).** One resolver list per relay, fixed at the claim:
     `upstream_dns` when the claim carries it, used exclusively. Its DNS
     handler forwards every guest query to that list, and its own upstream
     dials (the SSRF-filtered dialer and the plain transport, and a collector
     URL that names a host) resolve only through that list, by the same
     in-order forwarder (not Go's `net.Resolver`, whose retries restart at
     the first server; `/etc/hosts` and search domains do not apply). The list is asked in order: the next entry is asked when one
     does not answer within the per-query timeout, as `resolv.conf`'s
     `nameserver` lines are; that is a fallback inside the value the user
     gave, named in **Defaults and overrides**. Gone: the `UPSTREAM_DNS`
     environment variable (a second channel), the `8.8.8.8:53` default, and
     the handler's "system resolver first, then forward" split (two paths for
     one query).
   - **The default when the field is absent** (the container driver, which
     sends none, and any third-party claimant): the relay's platform
     resolver, the usable `nameserver` lines of its own `/etc/resolv.conf`
     (`pickNameserver`'s rule as today: not the relay's own address, not
     `127.0.0.1`, not inside the build link; Docker's embedded `127.0.0.11`
     stays usable), used exclusively through the same single
     path. No usable line refuses the claim with 400 naming `upstream_dns`.
     A boot-channel value never stands in for the field: on Hyper-V and
     Firecracker (static upstream) the init writes no `resolv.conf`, so a
     claim without the field is refused there. This default is written in
     `relaycfg`'s package doc (RELAY-08).
   - **The key.** `[vm] dns`, an optional list of IPv4 addresses
     (`dns = ["10.20.0.53", "10.20.0.54"]`), in `warden.toml` or the user
     config with the usual precedence (project over user, ORCH-03, UX-07).
     It has no flag and no environment variable (Jeff asked for a key; a
     `--dns` flag can follow if wanted). Unset, the default is the host's
     resolvers, below.
   - **Set: used exclusively.** Its entries, in order, are the claim's
     `upstream_dns`, and nothing else is: the host's resolvers are not read,
     not appended and not tried if the listed ones fail. A listed resolver
     that does not answer fails the build's lookups, as a host resolver that
     does not answer would; the relay records what it served, as always.
   - **Validated, never filtered.** At config load (UX-08), every command:
     each entry an IPv4 address, not unspecified, multicast or broadcast,
     and the list not empty (`dns = []` is refused: "remove `[vm] dns` to use
     the host's resolvers"). At build start and in doctor, the driver's own
     reachability rule (below) refuses the build naming the entry and why. A
     set list is all used or the build is refused: an unusable entry is never
     dropped silently, because the user named it. (r3 moves loopback and
     link-local from the load-time check to the drivers' rules, because a vz
     relay runs on the host, where a loopback resolver is reachable.)
   - **Unset: the host's.** At build start each VM driver reads the host's
     resolvers from its own source and runs the shared selection with its
     own reachability rule; filtering is the rule itself here (the host's
     list is not the user's choice for the relay; it is the default's
     source), as on Firecracker today:

     | Driver | Where its relay runs | Host source | Excluded (from the host's list; refused in `[vm] dns`) |
     |---|---|---|---|
     | hyperv | relay VM behind WinNAT | IPv4 DNS servers of the lowest-metric IPv4 default route's interface (`Get-NetRoute -DestinationPrefix 0.0.0.0/0`, `Get-DnsClientServerAddress -InterfaceIndex <i> -AddressFamily IPv4`) | loopback, link-local, addresses assigned to the host, the NAT prefix, the build link |
     | firecracker | relay VM behind its slot's TAP NAT | `/etc/resolv.conf`, and systemd-resolved's upstreams when its stub fronts them (as today) | as today: loopback, addresses assigned to the host, the build link; link-local added (unreachable from behind the NAT) |
     | qemu | relay VM behind user-mode NAT (qemu runs on aarch64 macOS hosts) | the Mac's `/etc/resolv.conf` (the primary resolvers macOS writes there), or the Linux source on a Linux host | loopback and link-local (they would be the relay VM's own), user-mode NAT's `10.0.2.0/24`, the build link |
     | vz | host-mode relay, a process on the Mac | the Mac's `/etc/resolv.conf` | the build link (loopback is reachable from a host process) |

     No qualifying host resolver is a `driver.Fixable` naming each excluded
     server and why, with the fixes "give the host an off-host DNS server"
     (on Hyper-V, naming the interface) and "set `[vm] dns`" (and, for an
     excluded link address, `[vm] build_link`). No public resolver stands in.
   - **Where it goes.** The chosen list goes into `BuildRequest.DNS` and on
     to `Session.ClaimRelay`, which sets `upstream_dns` on every VM driver's
     claim. It is not in the Hyper-V seed and not on Firecracker's command
     line (`warden.dns` leaves `driver/firecracker/vm.go`). Doctor prints it
     on every VM driver with its source (`[vm] dns`, or the host source it
     came from), or the refusal.
   - **Which drivers read it: every VM driver** (a10 at r2). No driver
     refuses `[vm] dns` any more; r2's qemu and vz startup refusal goes. The
     container driver ignores `[vm]`, as it does `[vm] build_link`, sends no
     `upstream_dns`, and its relay keeps the absent-field default (its
     container's `resolv.conf`).

   `selectResolvers` and `inLink` move from `driver/firecracker/posture.go`
   into a new `driver/resolvers` package with two entry points: `Select`
   (host list, link, an exclusion rule: filters, reports what it excluded)
   and `Check` (the `[vm] dns` list, same inputs: refuses on the first
   unusable entry), and the two Unix host sources (Linux's and macOS's).
   Each driver keeps its own exclusion rule and fix text, and Hyper-V its
   Windows source (FLOW-15: one selection, not four that drift).

   *Rejected:* keeping `8.8.8.8` (the bug). Falling back to public resolvers
   when no host resolver qualifies (a resolver the user never chose; r1's
   (b)). Using `[vm] dns` and then the host's resolvers if it fails, or
   merging the two lists (r1 review: a provided value is used exclusively).
   Filtering an unusable `[vm] dns` entry and using the rest (the user named
   it; refusing says so). A `--dns` flag in this feature (not asked for).
   The resolvers in the seed and on Firecracker's command line (r2; RELAY-08:
   not a pre-claim value, changed at the r2 re-review). Sending them in the
   claim only when `[vm] dns` is set and leaving the relay's platform
   resolver as the default on qemu and vz (question a14 (b)). Keeping
   `UPSTREAM_DNS` or the system-resolver-first split beside the claim field
   (a setting on one channel, one path per query; question a15).

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

   The claim's new `upstream_dns` field (r3, Decision 6) needs a relay
   binary that knows it. On Hyper-V the binary is baked into the relay boot
   VHDX, so stamp 2 covers it: a disk built from this branch has both the
   seed reader and the field. qemu, Firecracker and vz append warden's own
   relay binary per build, so it always matches. Either way, a relay that
   refuses the claim with `unknown field "upstream_dns"` gets the same
   `driver.Fixable` as `build_link` does today in `Session.ClaimRelay`
   (`OldRelayFix` on the drivers that append the binary, `refresh-cache.sh`
   on Hyper-V): the check generalises from the one field name to any
   unknown claim field. The container driver sends no `upstream_dns`, so an
   older relay image keeps working with it.

   *Rejected:* a second stamp key or file (an older warden ignores what it
   does not read, accepts the new disk, and times out on the claim). Bumping
   `buildlink.Channel` (rebuilds on three drivers that did not change). A
   seedless fallback in the init (Decision 2).

10. **BOOT-06 takes the per-build network-value invariant, and BOOT-07's
    surface list catches up** (questions a7, a8; FLOW-14; RELAY-08). After
    Decisions 2 to 6, no boot asset or image carries a network value the
    orchestrator chooses per build, on any driver: the relay gets only its
    pre-claim values (its own address, prefix and gateway, and the claim
    token) from its boot channel, a per-build seed or its kernel command
    line, and everything else from its claim (RELAY-08, r3); the guest gets
    its network values from its seed, its command line or the relay's
    lease. build-link-subnet left the
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
    | The relay's resolvers (Decision 6) | init: `8.8.8.8`; relay: `UPSTREAM_DNS`, else `resolv.conf`, else `8.8.8.8`, and system resolver before forwarding | One list from the claim's `upstream_dns`: `[vm] dns` exclusively when set, else the host's; neither is a refusal. The list's own order is the one named fallback (**Defaults and overrides**) |
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

**Settled at the r2 re-review (7be917d).** Confirmed as asked: a11
(setup-made per-build Private switches, `--slots` default 4), a12 (the
Windows guest keeps the relay's lease, `--gateway` only) and a13 (no runtime
fallbacks; SMBIOS a named contingency). Turned into a change, now Decision
6: a10 (the relay's upstream DNS goes in the claim, for every VM driver, and
nothing refuses `[vm] dns`). They are not asked again; r2's text for a10 to
a13 is in the queue file's `questions` and the **Design review** history.

**New in r3: the delta since 7be917d, for the re-review.**

- **a14. What each VM driver sends when `[vm] dns` is unset, and the
  relay's absent-field default** (Decision 6). Proceeding with: every VM
  driver always sends `upstream_dns`: `[vm] dns` when set, else the host's
  resolvers by the shared selection, each driver with its own host source
  and reachability rule (the table in Decision 6: qemu and vz read the Mac's
  `/etc/resolv.conf`; vz keeps loopback, since its relay is a host process;
  loopback and link-local move from `[vm] dns`'s load-time check to these
  rules). A relay claimed without the field (the container driver, third
  parties) uses its own `resolv.conf`'s usable nameservers exclusively, and
  refuses the claim when there are none; this is the default named in
  `relaycfg`'s package doc. Options: (a) as stated; (b) qemu and vz send the
  field only when `[vm] dns` is set, and otherwise keep the resolver their
  platform gives the relay (user-mode NAT's lease on qemu, the Mac's system
  resolver for vz's host-mode relay, which follows macOS's scoped and
  split-DNS resolvers that `/etc/resolv.conf` does not list); (c) as (a),
  and the container driver also sends the host's resolvers (container mode
  then reads `[vm] dns` too). Recommended: (a), the reading of a10's "the
  relay's upstream DNS (`[vm] dns` or the host's) is delivered in the claim,
  for every VM driver"; its cost is (b)'s split-DNS point on vz, where `[vm]
  dns` is the override.
- **a15. How the relay uses the list** (Decision 6; RELAY-04, GLOBAL-11).
  Proceeding with: one list per relay, fixed at the claim, for both the
  guest's DNS queries (forwarded) and the relay's own upstream dials
  (resolved only through that list, in order), in all three modes; the entries
  asked in order, the next when one does not answer within the per-query
  timeout (the named fallback inside the user's list); `UPSTREAM_DNS`, the
  `8.8.8.8:53` default and the system-resolver-then-forward split removed;
  the field validated at the claim (IPv4, not unspecified, multicast or
  broadcast, not in the build link) with a 400 that leaves the relay
  claimable. Options: (a) as stated; (b) as (a), but only the first entry
  is used (no fallback at all; a second entry is refused); (c) as (a), and
  keep `UPSTREAM_DNS` for `cmd/relay` run by hand outside warden.
  Recommended: (a).
- **a16. Native validation for the claim field** (front-matter; FLOW-09).
  Proceeding with: qemu-any and macos-vz become blocking, as steps
  `f6-validate-qemu-any` and `f6-validate-macos-vz`, because r3 reaches
  `driver/qemu` and `driver/vz` (each now selects and sends the relay's
  resolvers) and changes how a vz host-mode relay resolves names; the
  qemu-any backlog entry `hyperv-relay-per-build-channel-qemu-relay` is
  folded into the qemu step. windows-hyperv stays blocking; macos-qemu
  (Windows guest) stays tracked. Options: (a) as stated; (b) qemu-any and
  macos-vz tracked, relying on tier 0 for the drivers' selection and the
  relay's resolver; (c) as (a), plus a linux-kvm step for Firecracker
  rather than CI tier 2. Recommended: (a).

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
                warden.claim_token=token                     pre-claim values only (RELAY-08)
    CreateRelayVM(..., DVD=seed ISO, build NIC on buildSwitch,
                  Notes="warden-relay-ip=relayIP warden-relay-switch=buildSwitch")
  StartVM(relay)
  ClaimRelay(relayIP:8301, token, {build_link, upstream_dns: dns, ...})
                                                             Bearer on claim and tunnel
  CreateVM(build on buildSwitch); StartVM(build)             unchanged order
    guest: DHCP lease from the relay; warden-run.ps1 -> initialize --gateway=<relay>
  ... wait, teardown: RemoveVM(relay) frees relayIP and the slot
```

### The relay VM init (`tools/relay-vm/init`)

```
read_keys(words):            # one parser for both sources; never evaluated
  for kv in words: case warden.{net,nat_ip,nat_prefix,nat_gw,claim_token,seed}=*
                   warden.dns=* -> say "warden.dns retired: upstream_dns in the claim"; idle
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
resolv.conf: not written by the init (static leaves it empty; DHCP's script
             writes the lease's); the relay takes upstream_dns from the claim
WARDEN_CLAIM_TOKEN=$CLAIM_TOKEN /sbin/relay -mode vm -build-if $PRIVATE_IF   (unchanged)
```

### The relay's resolvers (`relaycfg`, `relay`, every mode)

```
claim(cfg):
  cfg.UpstreamDNS present?
    ParseUpstreamDNS: non-empty, IPv4, not unspecified/multicast/broadcast,
                      not in cfg.BuildLink                   else 400, relay stays claimable
    servers := cfg.UpstreamDNS                               source "claim"
  absent:
    servers := usable nameservers of /etc/resolv.conf        source "resolv.conf"
               (not self, not 127.0.0.1, not in the link)    none -> 400 naming upstream_dns
  log "upstream resolvers: <servers> (<source>)"
  DNS handler: forward each query to servers in order        (no system-first split)
  transport and safeDialContext: A/AAAA through that forwarder (not net.DefaultResolver)
```

### Every site that changes

| Site | Change | Decision |
|---|---|---|
| `tools/relay-vm/init` | one key reader for cmdline and seed, value checks, the Hyper-V seed mount, the baked defaults and fallbacks removed, `warden.dns` retired and `resolv.conf` no longer written, progress lines name keys not values | 1, 2, 6 |
| `relaycfg/relaycfg.go` | `Config.UpstreamDNS` (`upstream_dns`), `ParseUpstreamDNS`; the package doc names the absent-field default (RELAY-08) | 6 |
| `relay/relay.go`, `relay/dns.go`, `relay/proxy.go`, `cmd/relay/mode_*.go` | one resolver list from the claim or `resolv.conf`; the DNS handler forwards to it only; dials resolve through it; `UPSTREAM_DNS`, `8.8.8.8:53` and the system-first split removed; the list logged with its source | 6 |
| `driver/session.go` | `ClaimRelay` sets `upstream_dns` from `BuildRequest.DNS`; an `unknown field` refusal of any claim field maps to the driver's old-relay `Fixable` | 6, 9 |
| `driver/qemu`, `driver/vz` | resolver selection (shared source and rule, the driver's exclusions) or `Check` of `[vm] dns` before any VM; doctor's resolver line | 6 |
| `tools/relay-vm/hyperv/build.sh` | the storage, CD-ROM and ISO 9660 modules copied; the "no block device" comment rewritten | 1 |
| `tools/relay-vm/hyperv/refresh-cache.sh` | writes `WARDEN_LINK_CHANNEL=2` | 9 |
| `driver/hyperv/relayseed.go` (new, pure) | stage `warden-relay.conf` from a `RelaySeed{IP, PrefixLen, Gateway, Token}`; validation mirrors the init's | 1, 2 |
| `driver/hyperv/winseed_windows.go` | `makeSeedISOWindows` reused for the relay seed (label `WARDENRELAY`) | 1 |
| `driver/hyperv/relayvm.go` | `RelayVMSpec` gains `SeedISO`, `Notes` and the build NIC's switch; the script adds the DVD and the Notes while the VM is off | 1, 4, 7 |
| `driver/hyperv/provision*.go` | `EnsureNetwork`/`VerifyNetwork` return the NAT's prefix and host IP and verify a slot switch; the ephemeral NAT choice prefers the dev switch's NAT; listers of `warden-relay-*` Notes and of the adapters attached to each slot switch | 4, 5, 7 |
| `driver/hyperv/setup.go`, `capability*.go` | setup makes `<network>-0..N-1` (`--slots`, default 4), trims unattached extras, retires the old `<network>` switch when unattached; `DevSwitchPresent` probes the slots | 7 |
| `driver/hyperv/relayhost.go`, `driver_windows.go` | `defaultRelayNATIP` goes; `StartRelay` takes the NAT, resolvers and link, allocates the address and (durable) the slot under the lock, writes the seed, claims with the token; `provisionBuildNetwork`'s durable case no longer names one build switch | 3, 4, 7 |
| `driver/hyperv/driver_windows.go`, `doctor.go` | NAT read back, overlap refusal, resolver check or selection before any VM; doctor prints NAT, host IP, resolvers and their source, slots and their holders, stamp | 5, 6, 7, 9 |
| `driver/hyperv/buildassets.go` | the relay-boot stamp compared with `relayBootChannel` | 9 |
| `driver/resolvers` (new), `driver/firecracker/posture.go`, `vm.go` | the shared `Select`, `Check` and Unix host sources; Firecracker calls them with its own rule and fix text, uses `[vm] dns` when set, and drops `warden.dns` from the relay's command line | 6 |
| `driver/driver.go` (`BuildRequest`) | a `DNS []netip.Addr` field, from `[vm] dns` | 6 |
| `cmd/warden/config.go`, `host.go`, `build.go` | `[vm] dns` parsed and validated at load (no driver refuses it, r3); `--slots` accepted for hyperv (1 to 64, default 4) and its help text; `--nat-prefix`/`--host-ip` help: used by every build | 5, 6, 7 |
| `driver/winguest/run.go`, `winguest.go` | `WardenRunPS1` passes `--gateway` only; `BuildGuestCIDR` goes | 8 |
| `cmd/warden-io/platform_windows.go`, `initialize.go` | on Windows, a failed relay wait names the NIC's IPv4 addresses and origin (no lease reads as one); the `--ip` branch unchanged | 8 |
| `docs/development/bootstrap.md`, `tools/tier1/rules.yaml` | BOOT-06 and BOOT-07 as approved | 10 |
| docs: `overview.md`, `architecture.md`, `network-isolation.md`, `guide/cli.md`, `guide/configuration.md`, `guide/drivers.md`, `tools/relay-vm` READMEs | the relay seed, the token, per-build address, the durable slots, NAT read back, `[vm] dns` and `upstream_dns` in the claim, the Windows guest on the lease (UX-01, FLOW-15) | all |

### Compatibility

| Warden | Relay boot VHDX | Outcome |
|---|---|---|
| new | new (stamp 2) | the seed, the token, everything above |
| new | old (stamp 1, or no stamp) | refused before any VM: `refresh-cache.sh` |
| old | new (stamp 2) | refused before any VM by the old warden's stamp check (it expects 1): `refresh-cache.sh` from its own tree |
| third-party orchestrator | new, booted with no seed | the init stops with a console line; no relay starts. A third party passes the same keys in a `WARDENRELAY` ISO (documented in `tools/relay-vm/README`) |
| third-party orchestrator | any new relay, claimed without `upstream_dns` | the relay uses its own `resolv.conf` (the named default); on Hyper-V and Firecracker's static upstream it has none, so the claim is refused with 400 naming `upstream_dns` |
| new | an old relay (any mode that receives `upstream_dns`) | the claim is refused `unknown field "upstream_dns"`; the driver's old-relay `Fixable` (Decision 9) |
| new | durable network from an older setup (one `<network>` switch, no slots) | durable builds refused before any VM: run `warden setup --driver hyperv` (elevated), which makes the slots and retires the old switch; elevated and service builds unaffected |
| old | durable network from a new setup (slots, no `<network>` switch) | the old warden's durable strategy finds no switch and is refused with its own "run `warden setup`" message; its setup recreates `<network>`, which the new warden's setup later retires again |

qemu and Firecracker relay initramfs are not stamped and need no rebuild:
an old init still works with the new drivers (they send it nothing it does
not know; Firecracker stops sending `warden.dns`, and the relay binary
appended per build ignores the `resolv.conf` an old init writes because the
claim carries `upstream_dns`), and a rebuilt one behaves the same on every
path they use.

## Defaults and overrides

Each default this design adds or changes, why it is the obvious choice
(GLOBAL-09), and what overrides it, used exclusively (GLOBAL-10). Relay
settings travel on one channel (RELAY-08).

| Setting | Default (nobody names a value) | Why it is the obvious one | Override, used exclusively | Channel to the relay |
|---|---|---|---|---|
| The relay's upstream resolvers, warden's VM drivers (Decision 6) | the host's resolvers, by the shared selection with the driver's source and reachability rule | a build resolves names as the machine it runs on does, internal names included, with no resolver the user never chose | `[vm] dns` (project over user, ORCH-03): its entries in order, nothing else; an unusable entry refuses the build | claim, `upstream_dns` |
| `upstream_dns` absent from a claim (the container driver, third-party claimants; Decision 6, a14) | the usable `nameserver` lines of the relay's own `/etc/resolv.conf`; none refuses the claim with 400 | the platform resolver of wherever the relay runs, which is what a relay with no instruction would be expected to use; written in `relaycfg`'s package doc | the claim's `upstream_dns` | (default inside the relay) |
| The relay's address on Hyper-V's NAT (Decision 4) | the lowest free host address of the NAT prefix (not network, broadcast, host IP, nor recorded by a `warden-relay-*` VM) | deterministic and readable in VM Notes; the first free address is what an operator would pick | none: the address is warden's per-build choice, not a user setting | seed, `warden.nat_ip` |
| The relay's prefix and gateway on Hyper-V (Decision 5) | read back from the NAT the build uses (`Get-NetNat`, the host vNIC's address) | they are what the host has, so they cannot drift | `warden setup --driver hyperv --nat-prefix/--host-ip`, which makes that NAT; every build then reads it back | seed, `warden.nat_prefix`, `warden.nat_gw` |
| The NAT a Hyper-V build uses (Decision 5) | the dev switch's NAT when setup made one, else a warden NAT owning `DefaultNATPrefix`, else a new one on `DefaultNATPrefix` | one NAT per host, the user's when they set one up | `warden setup --nat-prefix` (it makes the dev switch's NAT, which then comes first) | (host infrastructure) |
| The claim token on Hyper-V (Decision 3) | always: 32 random bytes per build | every other driver already has one; no reason to run without | none | seed, `warden.claim_token` |
| Durable build switches (Decision 7) | `warden setup --driver hyperv` makes 4 slot switches (`<network>-0..3`); a build takes the lowest unattached one | Firecracker's slot model; 4 covers the concurrent builds a workstation runs | `--slots N` (1 to 64) at setup | (host infrastructure) |
| The Windows guest's address (Decision 8) | the relay's DHCP lease (no `--ip` passed) | the lease the relay already serves; one source for one NIC | none in warden; `initialize --ip` stays in the contract for other orchestrators | (guest) |
| Seed format version (Decision 2) | `warden.seed=1`, required | a version the init checks, so a future format refuses loudly | none | seed |

**Merges and fallbacks, named** (GLOBAL-10, GLOBAL-11):

- **`upstream_dns`'s own order.** The relay asks the list's entries in
  order and asks the next when one does not answer within the per-query
  timeout, as `resolv.conf`'s `nameserver` lines are. Trigger: no answer
  from an entry. The user sees it in the relay's log (one line per
  timed-out server) and, for a set `[vm] dns`, it never leaves the list
  they gave. It is the only fallback in the design.
- **No merges.** `[vm] dns` is never merged with or followed by the host's
  resolvers; the claim field is never merged with or followed by
  `resolv.conf`; the seed and the command line never both supply a key
  (Hyper-V reads the seed's values; other drivers have no seed).
- **The NAT order (Decision 5)** is a choice of what the host has, made
  once before any VM, not a path that runs on failure (Decision 11).
- **Removed** (each was an unnamed fallback, GLOBAL-11): the init's static
  network defaults and "DHCP failed, use them", the init's and the relay's
  `8.8.8.8`, the relay's `UPSTREAM_DNS` and system-resolver-first split,
  r1's SMBIOS-then-DVD and one-at-a-time ideas (Decision 11), and the
  Windows guest's `--ip` racing its lease.

## Cross-driver impact

- **hyperv.** Everything in Decisions 1 to 9. Builds need one
  `refresh-cache.sh` (the stamp says so), and a durable host one elevated
  `warden setup --driver hyperv` (for the slots). A host whose only
  resolvers are loopback or host-local addresses now gets a fixable refusal
  naming `[vm] dns` where it used to get `8.8.8.8`. A durable network set up
  on a non-default prefix starts working. Concurrent builds, durable or
  ephemeral, no longer collide. The Windows guest keeps the relay's lease.
- **qemu.** The shared relay init loses fallbacks a qemu relay never takes
  and its `warden.dns` key (qemu never passed it); its claim token and
  command line are unchanged. Its claim now carries `upstream_dns`: `[vm]
  dns`, or the Mac's resolvers by the shared selection, reached through
  user-mode NAT, where the relay used the resolver from user-mode NAT's
  lease (Decision 6). A qemu Windows guest runs the same `warden-run.ps1`,
  so it too drops `--ip` and keeps the relay's lease (Decision 8). Blocking:
  qemu-any; tracked: macos-qemu (Windows guest).
- **firecracker.** The shared relay init (as qemu; Firecracker passes every
  key on its command line, so none of the removed defaults applied). Its
  resolver selection moves to `driver/resolvers` with no change in what it
  selects when `[vm] dns` is unset; the result now goes in the claim's
  `upstream_dns` instead of `warden.dns` on the relay's command line, and
  `[vm] dns`, when set, is checked by the same rule and sent the same way.
  CI tier 2 (`firecracker.yml`, whose cache key hashes `tools/relay-vm`)
  rebuilds the relay initramfs and runs both builds; `[vm] dns` is covered
  at tier 0. `--slots` keeps its Firecracker meaning and default.
- **vz.** `driver/vz` selects the host-mode relay's resolvers (the Mac's
  `/etc/resolv.conf`, or `[vm] dns`) and sends them in the claim; the relay,
  a process on the Mac, resolves through that list instead of the Mac's
  system resolver, so macOS's scoped (split-DNS) resolvers that
  `/etc/resolv.conf` does not list are no longer followed unless named in
  `[vm] dns` (question a14). Its relay never boots the init. Blocking:
  macos-vz.
- **container.** Unaffected in what it sends: it ignores `[vm]` (as it does
  `[vm] build_link`), sends no `upstream_dns`, and nothing in
  `driver/container` changes. Its relay runs in container mode, where the
  relay change applies: with the field absent it uses its container's
  `resolv.conf` exclusively, without the `8.8.8.8` it fell back to. A
  container whose `resolv.conf` names no usable server now fails its claim
  loudly instead (tier 0 and the container CI path; container-any not run
  natively).

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
(`192.168.240.2/20`, gateway `.1`, `8.8.8.8`, no token). The relay's
resolvers come from a boot channel (Firecracker's `warden.dns`), a lease
(qemu), the host's system resolver (vz), an environment variable or a
baked `8.8.8.8`. Per-driver mapping
rows: qemu Linux, firecracker Linux, qemu and hyperv Windows, vz macOS,
container. Deviation register: D2 (vz).

### Proposed model

The same diagram, mapping rows and deviation register. Step (1) on Hyper-V
gains a per-build relay seed and, on the durable strategy, a per-build
switch; step (3) on Windows takes its address from the relay's lease alone
(r2: the BOOT-01, BOOT-03 rows and this diagram change). r3: the relay's
resolvers come from the claim on every VM driver, and the boot channel
carries only the relay's pre-claim values (RELAY-08), so the diagram's step
(1) and the BOOT-06 row change with BOOT-06's reworded paragraph, for
re-approval:

```
  (1) isolate ------------------------------>  sole network path: the relay
      Hyper-V: write the relay seed (its       (relay VM reads its seed, sets its
      NAT address, prefix, gateway and the     upstream, starts the relay with the
      claim token: pre-claim values only),     token; durable builds take their own
      boot the relay VM, claim it with the     Private switch from setup's slots)
      token, the build link and the
      resolvers; only then boot the guest      every VM driver: the relay's
                                               resolvers in the claim, RELAY-08

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
| BOOT-06 images carry hooks, not content | Generic hooks, nothing per-build, nothing tied to a warden version; but the Hyper-V relay VM's init bakes its upstream address, gateway and resolver, and every Hyper-V relay shares them | Adds (reworded at r3): no boot asset or image, the relay VM's included, carries a network value the orchestrator chooses per build, and none bakes a default that stands in for a channel; the relay's boot channel carries only its pre-claim values, everything else in its claim (RELAY-08). Holds on every driver after this feature | hyperv: the relay seed, pre-claim values only; firecracker: `warden.dns` leaves the command line for the claim; qemu, firecracker: the init's unused defaults removed; vz: resolvers in the claim |
| BOOT-07 the model changes by a signed comparison | Surface list without `tools/relay-vm/`, `tools/build-vm/`, the relay's DHCP responder | Unchanged rule; surface list extended (rule change BOOT-07) | tier 1 hint fires on those paths |

### Gains

Hyper-V's relay becomes like every other driver's: a per-build address, a
per-build token, the user's resolvers or the host's, the user's NAT prefix. The claim race
is closed: a local process or another build's orchestrator gets 401 and the
relay stays claimable. Concurrent Hyper-V builds work, and durable builds stop
sharing a segment, so each build's ledger records only what its own guest
received. The Windows guest's address has one source, the relay's lease.
The relay's resolvers have one source on every driver and mode, the claim
(or, when absent, the named default), instead of four. BOOT-06 then states,
for every driver, the principle build-link-subnet established for the link,
with RELAY-08 for the relay's channel, and the tier 1 hint covers the files
where it could regress.

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
write a seed, and one that claims a relay with no `resolv.conf` (Hyper-V,
Firecracker) must send `upstream_dns`. On vz, the relay no longer follows
macOS's scoped resolvers unless `[vm] dns` names them. One more `refresh-cache.sh` for every Hyper-V user. The record
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

**r3, for re-approval.** The paragraph approved at `0df0a64` said "The relay
gets those from its claim, its kernel command line or a per-build relay
seed", which lets any of the three carry any value. The r2 re-review's rule,
now RELAY-08, limits the boot channel to the relay's pre-claim values, so
the paragraph is reworded to say which values travel there and to cite
RELAY-08 for the rest, as `design-principles` Decision 8 expects (RELAY-08
cites BOOT-06 for the boot-asset half; each holds whichever merges first).
The first paragraph, severity, check and enforcement are as approved.

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
> resolver or a claim token. The relay is configured through its claim
> (RELAY-08); only its pre-claim values, its own address, prefix and gateway
> and the claim token, come on its boot channel, a per-build relay seed or
> its kernel command line. The build environment gets its network values
> from its seed, its kernel command line or the relay's DHCP lease. A boot
> asset may carry the launcher that reads such a channel, but no default
> that stands in for it.

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
names come from (`[vm] dns` or the host's resolvers, now in the claim).
RELAY-04: the relay binary changes in all three modes (its resolver list
comes from the claim's `upstream_dns`, or its own `resolv.conf` when absent,
and `UPSTREAM_DNS` and `8.8.8.8` go); the relay VM's init changes for every
vm-mode relay.

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

**Resolvers.** The relay's upstream resolvers move from `8.8.8.8` (and on
vz the Mac's system resolver, on qemu user-mode NAT's) to the claim's
`upstream_dns`: `[vm] dns` when set, else the host's. A build's names now
resolve as the user or the host chooses, which can include internal names;
the relay records every answer it serves, as before. `[vm] dns` is config
the user controls, like `[vm] build_link`; the build cannot set it, and the
field reaches the relay only in the claim, which the host sends over its
claim path with the token (the build VM cannot reach the claim port). The
relay refuses an `upstream_dns` entry inside the build link, so it never
sends DNS into the build segment; each driver also excludes from the host's
list, and refuses in `[vm] dns`, what its relay cannot reach or would loop
on (the table in Decision 6). One list serves both the guest's queries and
the relay's own dials, so the relay cannot resolve a name for its upstream
fetch differently from the answer it served the guest. Moving the value off
Firecracker's kernel command line also takes it out of the relay's argv.

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
with `[vm] dns` set, the relay uses only those resolvers, for the guest's
queries and its own dials; a claim with an `upstream_dns` entry in the build
link, or malformed, is refused with 400 and the relay stays claimable; a
claim without the field on a relay with no usable `resolv.conf` is refused.
These go to `security-suite` at Consolidation, as build-link-subnet's did.

## Validation plan

- **Tier 0** (TEST-01: pure Go, table-driven, no host network).
  `relaycfg`: `upstream_dns` round-trips; `ParseUpstreamDNS` refuses an
  empty list, a non-IPv4, unspecified, multicast or broadcast entry and one
  inside the build link, with 400 that leaves the relay claimable; a claim
  without it is accepted. `relay`: with `upstream_dns`, the DNS handler
  forwards only to those servers in order (a fake UDP server per entry, the
  second answering when the first does not) and the dialer resolves through
  them, `resolv.conf` and `UPSTREAM_DNS` not read; without it, the
  `resolv.conf` rule (`127.0.0.11` kept, `127.0.0.1`, self and in-link
  dropped) and none usable refused; no `8.8.8.8` left in `relay/`.
  `driver`: `ClaimRelay` puts `BuildRequest.DNS` in the claim, and an
  `unknown field` refusal of `upstream_dns` or `build_link` becomes the
  old-relay `Fixable`.
  `driver/hyperv`: `relayseed` renders every key, holds no resolver, and
  refuses a bad IP, prefix, gateway or token; `buildRelayVMScript` adds the DVD, the
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
  is exactly the claim's `upstream_dns` with the host's never read and no
  resolver in the seed, and teardown removes the VM; the stamp check against `relayBootChannel` (1
  refused, 2 accepted, missing refused, each a `driver.Fixable` naming
  `refresh-cache.sh`); the Windows default-route DNS parser on canned
  output. `driver/resolvers`: `Select`'s every exclusion and Firecracker's
  existing cases moved over unchanged; `Check` refuses (not filters) each
  unusable entry and passes a usable list through in order.
  `driver/firecracker`: the relay's command line has no `warden.dns`; the
  selection (or `[vm] dns`, with the host's resolv.conf not read) reaches
  the claim; the fix texts name `[vm] dns`. `driver/qemu` and `driver/vz`:
  the macOS source parser on canned `/etc/resolv.conf`, each driver's
  exclusions (qemu: loopback, link-local, `10.0.2.0/24`, link; vz: link
  only, loopback kept), `[vm] dns` checked by the same rule and sent in the
  claim. `cmd/warden`: `[vm] dns` at load (IPv4 only, unspecified,
  multicast and broadcast refused, `[]` refused, project over user); a qemu
  or vz build with it set is not refused; `--slots` accepted for hyperv (1 to 64, default 4) and still
  refused for other non-firecracker drivers. `driver/winguest`:
  `WardenRunPS1` runs `initialize --gateway=<relay>` with no `--ip` (both
  default and configured links); the qemu Windows seed carries the same
  script. `cmd/warden-io` (`platform_windows.go`, under its build tag,
  rendering only): the diagnostic for a failed relay wait renders the NIC
  addresses with their origin and says "no DHCP lease" for a `WellKnown`
  169.254 address; the `--ip` branch is unchanged. `tools/relay-vm/init` as
  text: one key reader for both sources, never evaluates, the seed read only
  under `HYPERV`, `warden.dns` stops the init, no `resolv.conf` write, no
  IPv4 literal anywhere in the file, the token's value never on a `say`
  line. `refresh-cache.sh` writes stamp 2.
- **Lint.** `make lint` for linux (`driver/resolvers`, firecracker,
  `cmd/warden`, `relay`, `relaycfg`), darwin (`driver/vz`, `driver/qemu`)
  and windows (`driver/hyperv`, `driver/winguest`,
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
     the seed (the allocated address and gateway), its resolvers with
     source `claim` (the host's), and no token value; the seed ISO holds no
     resolver; the claim carried a token (driver log). In the guest,
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

  Blocking (r3, question a16): **qemu-any**, as queue step
  `f6-validate-qemu-any`, after consolidation, on a relay initramfs rebuilt
  from the branch (it replaces the backlog entry
  `hyperv-relay-per-build-channel-qemu-relay`). Cases:
  1. A qemu Linux build (direct boot and a cloud image) succeeds and its
     ledger verifies; the relay's console names its upstream from DHCP and
     its resolvers with source `claim`, the Mac's from `/etc/resolv.conf`
     (doctor prints the same list).
  2. With `[vm] dns` set to a public resolver other than the Mac's, the
     build resolves names and the relay lists only that resolver; set to an
     address with no DNS server, lookups fail (no fallback).
  3. With `[vm] dns = ["127.0.0.1"]`, the build is refused before any VM
     naming the entry (loopback is the relay VM itself).

  Blocking (r3, question a16): **macos-vz**, as queue step
  `f6-validate-macos-vz`, after consolidation. Cases:
  1. A vz macOS build succeeds and its ledger verifies; the host-mode
     relay's log lists its resolvers with source `claim`, the Mac's from
     `/etc/resolv.conf`, and doctor prints the same list.
  2. With `[vm] dns` set to a public resolver other than the Mac's, the
     build resolves names through it only (relay log); set to an address
     with no DNS server, lookups fail (no fallback to the Mac's).
  3. With `[vm] dns` naming a resolver on `127.0.0.1` (a local forwarder),
     the build resolves through it (loopback is reachable from the host
     relay).

  Tracked (TEST-03): **macos-qemu**, a qemu Windows guest ends on the
  relay's lease with no `--ip` (backlog
  `hyperv-relay-per-build-channel-qemu-windows`, after
  `qemu-windows-guest-package`). linux-kvm is covered by CI tier 2
  (Firecracker, now with the resolvers in the claim). Not run natively:
  container-any (container mode's absent-field default is tier 0 and the
  CI container path) and macos-finch (unaffected).
- **Queue steps.** `f6-validate-windows-hyperv` (`host:windows-hyperv`),
  added at Design r1 and kept by `wf back`, carries these cases in its note;
  r3 adds `f6-validate-qemu-any` (`host:qemu-any`) and
  `f6-validate-macos-vz` (`host:macos-vz`) with theirs. None has `needs`: a
  phase-6 step is not offered before Publish (FLOW-04), and seeding Publish
  makes each need `f6-consolidate` and `f6-open-pr` need it.
- **Security (TEST-08).** The Security note's cases are covered at tier 0
  and in the native cases above; Consolidation hands the list to
  `security-suite`.

## Implementation plan

Plan (phase 2, `f2-plan`, 2026-11-11), written for the design as approved at
re-review r3 (`cbb2cda`; the branch head `cc19f7e` adds only the review's
record), after reading the code each site-table row names. Each task names
the rules it must satisfy and the decisions and assumed questions it rests
on (a1 to a16, all confirmed). Tasks 1, 3 to 6, 8, 11, 13, 14, 16 to 18,
20 to 22, 25, 26, 28, 29, 31 to 35, 37 and 38 rework code that already
exists (FLOW-06). Tasks 40 to 42 are the reference-doc, concept-doc and
`overview.md` updates the change owes (UX-01, FLOW-15); tasks 43 and 44 are
the approved rule changes (FLOW-14). Testing tasks name their tier
(TEST-02) and keep tier 0 pure Go, table-driven and free of host networking,
with platform code behind build-tagged stubs (TEST-01, GLOBAL-07).

**Budget** (FLOW-13). 50 tasks, in `f3-implement`. Spent: 4 runs of the 4
estimated, 7 steps of 18. The rest is about 18 claims (Implement over three
or four leases, review, guide, merge-main, consolidate, open-pr, three
blocking native steps with one re-run likely on windows-hyperv, close), so
about 25 steps in all, and about 8 more runs (3 Implement, 1 review and
guide, 1 Publish, 2 native, since the Windows desktop and the macbook each
batch their hosts, 1 close), about 12 in all. Recommended to the tick or
Jeff: raise `budget.estimate` to 12 runs and 26 steps before Implement, so
the runaway check does not escalate mid-Implement.

**Plan notes** (FLOW-19). Planning found the design workable as written; no
finding needs Design. Where its text names a file that does not exist or is
silent on a mechanism, the plan takes the reading below, which Review checks
against the design:

- *File names (FLOW-15).* The site table's `cmd/warden/build.go` is
  `cmd/warden/main.go` (`buildRequest`, `resolveBuildLink`), and
  `cmd/relay/mode_*.go` is `cmd/relay/main.go` (container mode's step and the
  shared `serveClaim`/`applyClaim`), `mode_host.go` and `vmlink.go`.
- *Where the chosen list travels (RELAY-08, ORCH-01; Decision 6: "the chosen list goes into
  `BuildRequest.DNS` and on to `Session.ClaimRelay`").* `cmd/warden` fills
  `BuildRequest.DNS` from `[vm] dns` for VM drivers only, as it does
  `BuildLink`, so the container driver and `cmd/collector` send no field.
  Each VM driver replaces `req.DNS` with its resolved list (the `[vm] dns`
  list after `Check`, or the host's after `Select`) before it claims, and
  `ClaimRelay` copies `req.DNS` into `upstream_dns`. No `ClaimRelay`
  signature changes (tasks 8, 11, 13, 14, 29).
- *Where the absent-field default runs (RELAY-08, RELAY-04, UX-04; Decision 6, a14).* Its refusal must
  be a 400 that leaves the relay claimable, which only a start step can
  return (`relaycfg.ErrRefused`). So `cmd/relay`'s shared `serveClaim` path
  resolves the list for every mode, through a pure `relay` helper that
  replaces `pickNameserver`, and hands `relay.Start` a non-empty list; the
  relay package no longer reads `resolv.conf` or the environment itself
  (tasks 3, 6).
- *The init's `resolv.conf` (BOOT-06, RELAY-08; Decision 2: "the init's `resolv.conf` writing"
  goes; the pseudocode: "not written by the init (static leaves it empty;
  DHCP's script writes the lease's)").* The init's own block (the
  `warden.dns`/`8.8.8.8` writer) goes; the `udhcpc` lease script keeps
  writing the lease's servers, which is the platform resolver the
  absent-field default reads on a DHCP upstream. The tier-0 text test
  checks for no `resolv.conf` write outside that heredoc (tasks 20, 23).
- *The stamp value per asset (UX-04, ORCH-01; Decision 9).* `driver.LauncherStamp` compares
  with `buildlink.Channel` today; it gains a `Want` value (zero keeps
  `buildlink.Channel`) so Hyper-V's stamp compares with `relayBootChannel`
  while qemu's and Firecracker's keep 1. `driver/buildlink/scripts_test.go`
  stops listing `refresh-cache.sh`, and a `driver/hyperv` test asserts it
  writes `relayBootChannel` instead (tasks 22, 23, 34).
- *The privileged service and the read-back NAT (UX-04, GLOBAL-11; Decision 5).*
  `NetworkResources` gains the NAT's prefix and host IP, which a service
  installed by an older warden would not send. The plan bumps
  `serviceProtocolVersion` (1 to 2), so the existing mismatch refusal names
  `warden hyperv service install`: a refusal both ways, as Decision 9 and
  Decision 11's skew row ask, with no new mechanism (task 28).
- *What the allocator reads (ORCH-01, GLOBAL-06; Decisions 4, 7).* Two new `Provisioner`
  methods at the Hyper-V Administrators tier: the Notes of `warden-relay-*`
  VMs and the VM adapters attached to each named switch. They run in
  process (the service still delegates only network standup), and the
  existing fakes gain them (task 26).
- *The slot's "nothing attached but this build's VMs" check (BOOT-01, GLOBAL-06; Decision 7,
  Security note).* The allocator only takes a slot with nothing attached;
  `VerifyNetwork` at build start checks the NAT and that the slot switches
  exist and are Private; and before the build VM starts, the driver re-reads
  the slot's adapters and refuses unless they are exactly the relay VM's
  build NIC and the build VM's NIC. The build VM attaches to the switch
  `StartRelay` allocated (task 32).
- *The seed ISO in `StartRelay`'s tests (TEST-01).* `StartRelay` takes the
  ISO builder as a `RelayHostConfig` field defaulting to the platform's
  `makeSeedISO` (IMAPI2 on Windows, an error elsewhere), so the end-to-end
  test runs on the fake `Provisioner` on every platform (task 31).
- *The Hyper-V doctor's inputs (ORCH-01, UX-04).* `hyperv.Doctor`, `firecracker.Doctor` and
  the new qemu and vz resolver reports are package functions, so passing them
  the link and `[vm] dns` changes no `Driver` method (tasks 17, 35).
- *The qemu host source (GLOBAL-07; Decision 6).* The qemu driver is not supported on a Windows host
  (`driver/qemu/vm_windows.go`), so its source is the macOS `/etc/resolv.conf`
  on darwin and the Linux source on linux, as Decision 6's table says.
- *Backlog (TEST-03, FLOW-09).* The one tracked gap's entry
  (`hyperv-relay-per-build-channel-qemu-windows`, macos-qemu) already exists
  with Decision 8's wording, and the qemu-any entry is folded into its step,
  so task 49 adds none.
- *Native step notes (FLOW-09).* The three `f6-validate-*` notes already carry the
  Validation plan's cases; the plan leaves them unchanged (task 48).

### The claim field (`relaycfg`)

1. `relaycfg.Config.UpstreamDNS []string` (`upstream_dns,omitempty`) and
   `ParseUpstreamDNS(list []string, link *BuildLink) ([]netip.Addr, error)`:
   present means non-empty, each an IPv4 address, none unspecified,
   multicast or broadcast, none inside the claim's build link. `handleClaim`
   validates it after `build_link` and answers 400 `invalid claim:
   upstream_dns: ...`, releasing the claim. The package doc gains the field,
   its validation, the absent-field default (the usable `nameserver` lines
   of the relay's own `/etc/resolv.conf`, used exclusively, a 400 naming
   `upstream_dns` when none), the order rule (asked in order, the next on a
   per-query timeout) and the old-relay `unknown field` refusal. (RELAY-08,
   GLOBAL-08, RELAY-04, GLOBAL-10, UX-04; Decision 6, a14, a15)
2. Tier-0 `relaycfg` tests (TEST-02 tier 0): the field round-trips and is
   omitted when empty; each `ParseUpstreamDNS` refusal (empty, IPv6, a
   name, unspecified, multicast, broadcast, inside the link) is a 400 after
   which a valid claim succeeds; a claim without the field is accepted; a
   copy of today's `Config` decoded with `DisallowUnknownFields` refuses one
   carrying it with `unknown field "upstream_dns"` (the text task 8 maps).
   (TEST-02, TEST-01, GLOBAL-08, UX-03; a15)

### The relay (`relay`, `cmd/relay`)

3. `relay` (rework, `relay.go`): `Config.UpstreamDNS` becomes the fixed
   resolver list (`[]netip.Addr`) plus its source label (`claim` or
   `resolv.conf`), required non-empty by `Start`; `detectUpstreamDNS`, the
   `UPSTREAM_DNS` read and both `8.8.8.8:53` defaults go; `pickNameserver`
   becomes a pure `ResolvConfServers(conf, self, link)` returning every
   usable server in order (not self, not `127.0.0.1`, not in the link;
   `127.0.0.11` kept). (RELAY-04, RELAY-08, GLOBAL-10, GLOBAL-11; Decision 6,
   a15)
4. `relay/dns.go` (rework): the handler forwards every guest query to the
   list in order, asking the next entry when one does not answer within the
   per-query timeout and logging one line per timed-out server;
   `resolveViaSystem` and the system-first split go. (RELAY-04, GLOBAL-11,
   GLOBAL-10; a15)
5. `relay/proxy.go` (rework): one lookup (A and AAAA through the DNS
   handler's in-order forwarder; Go's `net.Resolver` retries from the first
   server, which breaks the order rule) used by `safeDialContext`, the
   plain transport's dialer and a direct `CollectorURL` that names a host;
   `net.DefaultResolver` leaves `relay/`. The build-link refusal on every
   resolved address stays. (RELAY-04, GLOBAL-06; a15)
6. `cmd/relay` (rework, `main.go`'s `serveClaim`/`applyClaim`, used by
   container, vm and host mode alike): the claim's `upstream_dns` (already
   validated) is the list with source `claim`; absent, `ResolvConfServers`
   over `/etc/resolv.conf` with source `resolv.conf`, and none usable refuses
   with `ErrRefused` naming `upstream_dns`, so the relay stays claimable. The
   relay logs `upstream resolvers: <list> (<source>)` (on COM1 in vm mode on
   Hyper-V via the init's redirect); the claim token is never printed.
   (RELAY-04, RELAY-08, RELAY-03, UX-04; a14, a15)
7. Tier-0 relay tests (TEST-02 tier 0): with a list, the DNS handler
   forwards only to those servers in order (a fake UDP server per entry on
   loopback, the second answering when the first does not) and the dialer
   resolves through them, with `UPSTREAM_DNS` set in the test environment
   and ignored; `ResolvConfServers`' table (`127.0.0.11` kept, `127.0.0.1`,
   self and in-link dropped, order kept); `cmd/relay`: the absent-field
   path refused with 400 and still claimable when nothing is usable, the
   source label in the log; a scan that `relay/` and `cmd/relay/` hold no
   `8.8.8.8` and no `UPSTREAM_DNS`. Host-mode tests keep their `unix` tag.
   (TEST-02, TEST-01, TEST-08, RELAY-04; a15)

### The claim, orchestrator side (`driver`)

8. `driver.BuildRequest.DNS []netip.Addr` (doc: `[vm] dns` from `cmd/warden`
   for VM drivers; each VM driver replaces it with the list it resolved
   before claiming; empty sends no field). `Session.ClaimRelay` (rework) sets
   `cfg.UpstreamDNS` from it, and maps a rejection containing any `unknown
   field "<name>"` to `Fixable(oldRelayFix, "the relay predates the claim
   field <name>")`, generalising today's `build_link` match. All six
   callers keep their signatures. (GLOBAL-08, RELAY-08, UX-04, ORCH-01;
   Decision 9, a14)
9. Tier-0 `driver` tests (TEST-02 tier 0, `session_link_test.go`): `req.DNS`
   reaches the claim in order; an empty one sends no `upstream_dns` (the
   container and collector path); `unknown field "upstream_dns"` and
   `unknown field "build_link"` both become the old-relay `Fixable`.
   (TEST-02, TEST-01, GLOBAL-08)

### Shared resolver selection (`driver/resolvers`)

10. New package `driver/resolvers` (untagged, pure): a `Rule` (an exclusion
    function returning the reason), `Select(host []netip.Addr, rule)`
    (kept, and excluded with reasons) and `Check(vmDNS, rule)` (the first
    unusable entry is an error naming it and why; a usable list passes
    through in order), `InLink`, and the two Unix host sources moved from
    `driver/firecracker/posture.go`: Linux (`/etc/resolv.conf`, and
    systemd-resolved's upstream file when its stub fronts them) and macOS
    (`/etc/resolv.conf`), each a pure parser plus a reader over an injected
    file read. Each driver keeps its own rule and fix text. (FLOW-15,
    GLOBAL-07, GLOBAL-10, ORCH-01; Decision 6, a3, a14)
11. Firecracker (rework, `posture.go`, `preflight.go`, `vm.go`,
    `driver_linux.go`, `doctor.go`): `selectResolvers`, `inLink` and the
    source code go to `driver/resolvers`; its rule is loopback, the host's
    own addresses, the build link and (new) link-local; with `[vm] dns` set
    it runs `Check` and reads no host file; the fix texts add "set `[vm]
    dns`"; the list goes into `req.DNS` for the claim; `relayArgs` drops
    `warden.dns`; doctor's line names the source (`[vm] dns` or the file).
    `--slots` keeps its Firecracker meaning. (ORCH-01, FLOW-15, RELAY-08,
    BOOT-06, UX-04; Decision 6, a14)
12. Tier-0 tests (TEST-02 tier 0): `driver/resolvers` carries Firecracker's
    existing selection cases unchanged, plus `Check` refusing (not
    filtering) each unusable entry and the macOS parser on canned files;
    `driver/firecracker`: the relay command line has no `warden.dns`, the
    selection or `[vm] dns` (host `resolv.conf` not read) reaches the
    claim, link-local is excluded, and the fixes name `[vm] dns`. (TEST-02,
    TEST-01, FLOW-15)
13. qemu (rework, `driver.go`): before the relay boots, resolve the list:
    `[vm] dns` through `Check`, else the host source by `GOOS` through
    `Select`, with the rule loopback, link-local, `UserNetCIDR` and the
    build link, and a `driver.Fixable` naming the excluded servers, "give
    the host an off-host DNS server" and "set `[vm] dns`" (and `[vm]
    build_link` for an in-link server); set `req.DNS`; export the same
    resolution for doctor. The relay's command line and token are
    unchanged. (ORCH-01, UX-04, RELAY-08; Decision 6, a14)
14. vz (rework, `driver.go`, selection in an untagged file): the same with
    the Mac's `/etc/resolv.conf` and the rule "the build link only"
    (loopback kept, the relay is a host process); set `req.DNS` before
    `ClaimRelay`; export the resolution for doctor. (ORCH-01, RELAY-08,
    RELAY-04; Decision 6, a14)
15. Tier-0 qemu and vz tests (TEST-02 tier 0, untagged): each driver's
    exclusions on canned lists (qemu: loopback, link-local, `10.0.2.0/24`,
    link; vz: link only, loopback kept), `[vm] dns` checked by the same rule
    and placed in `req.DNS`, and a qemu or vz request with `[vm] dns` set is
    not refused. (TEST-02, TEST-01, GLOBAL-07; a14)

### Config and CLI (`cmd/warden`)

16. `[vm] dns` (rework, `config.go`, `main.go`): `VM.DNS []string`
    (`toml:"dns"`) with its source file, validated at load for every
    command: each an IPv4 address, not unspecified, multicast or broadcast
    (loopback and link-local are left to the drivers' rules, r3); `dns = []`
    refused with "remove `[vm] dns` to use the host's resolvers"; project
    over user. `buildRequest` sets `BuildRequest.DNS` for VM drivers only.
    No flag and no environment variable. (UX-07, UX-08, ORCH-03, GLOBAL-10;
    Decision 6, a3, a10, a14)
17. `warden doctor` (rework, `host.go`): every VM driver prints the relay's
    resolvers and their source, or the refusal (exit non-zero), passing the
    link and `[vm] dns` to `hyperv.Doctor`, `firecracker.Doctor` and the new
    qemu and vz resolver reports. (UX-04, UX-09, ORCH-01; Decision 6)
18. `warden setup` (rework, `host.go`): `--slots` accepted for `--driver
    hyperv` (1 to 64, default 4) and passed as `SetupOptions.Slots`;
    Firecracker keeps 1 to 256, default 16; other drivers still refused; the
    flag's help names both; `--nat-prefix`/`--host-ip` help says every build
    uses the NAT setup makes; the `hyperv` paragraph of `setup`'s long help
    describes the slot switches and the retired `<network>` switch.
    (UX-01, UX-03, UX-04, UX-05; Decision 7, a11)
19. Tier-0 `cmd/warden` tests (TEST-02 tier 0): `[vm] dns` at load (IPv4
    only; unspecified, multicast, broadcast, a name and `[]` refused; a
    loopback entry accepted at load; project over user); `BuildRequest.DNS`
    set for a VM driver and empty for the container driver; `--slots` for
    hyperv (bounds, default) and still refused for qemu, vz and container.
    (TEST-02, TEST-01, UX-08)

### Relay VM init and Hyper-V build tools

20. `tools/relay-vm/init` (rework): one `read_keys` function for
    `/proc/cmdline` and, on Hyper-V, the seed file: only the known keys
    (`net`, `nat_ip`, `nat_prefix`, `nat_gw`, `claim_token`, `seed`), never
    evaluated, each value checked (dotted quads, prefix 1 to 30, token
    `[0-9a-f]{64}`, `dhcp|static`) or the init stops with a console line and
    idles; `warden.dns` stops it naming `upstream_dns` in the claim. On
    `HYPERV`: load the storage, CD-ROM and ISO 9660 modules, wait up to 10 s
    for the CD, mount it read-only, copy `warden-relay.conf` to `/tmp`,
    unmount; missing or `warden.seed` other than 1 says "no relay seed
    (WARDENRELAY)" and idles. The baked `NAT_*` defaults, the "Hyper-V means
    static" default, the DHCP-failed static fallback and the init's own
    `resolv.conf` block (`8.8.8.8`) go; static needs all three values; a
    DHCP failure stops. Progress lines name keys, never the token. The
    header comment documents the keys and the seed. (BOOT-06, BOOT-01,
    RELAY-04, RELAY-08, RELAY-03, GLOBAL-11; Decisions 1, 2, a1, a7)
21. `tools/relay-vm/hyperv/build.sh` (rework): copy the storage, SCSI CD-ROM
    and ISO 9660 modules the `linux-virt` kernel builds as modules
    (`hv_storvsc`, `scsi_mod`, `sr_mod`, `cdrom`, `isofs`, dependencies
    first, skipped when built in); the "no block device" comment rewritten.
    (BOOT-01, RELAY-04; Decision 1, a1)
22. `tools/relay-vm/hyperv/refresh-cache.sh` (rework): `channel=
    "WARDEN_LINK_CHANNEL=2"`, so a disk stamped 1 is rebuilt. (UX-04;
    Decision 9)
23. Tier-0 text tests for the tools (TEST-02 tier 0, reading the files):
    the init has one key reader used for both sources, no `eval`, the seed
    read only under `HYPERV`, `warden.dns` stopping it, no `resolv.conf`
    write outside the `udhcpc` heredoc, no IPv4 literal anywhere, and no
    `say` line expanding the token; `build.sh` lists the modules;
    `refresh-cache.sh` writes `relayBootChannel` (in `driver/hyperv`) and
    `driver/buildlink`'s stamp test no longer covers it. (TEST-02, TEST-01,
    BOOT-06; Decisions 2, 9)

### `driver/hyperv`

24. `relayseed.go` (new, pure): `RelaySeed{IP, PrefixLen, Gateway,
    Token}`, `Validate` mirroring the init's checks, and `Stage(dir)`
    writing `warden-relay.conf` (`warden.seed=1`, `warden.net=static`,
    `nat_ip`, `nat_prefix`, `nat_gw`, `claim_token`; no resolver, link or
    collector key), mode 0600, under the build's work directory; the ISO via
    `makeSeedISOWindows(dir, iso, "WARDENRELAY")`, with an error stub off
    Windows. (BOOT-06, RELAY-08, RELAY-03, GLOBAL-07; Decisions 1 to 3, a1)
25. `relayvm.go` (rework): `RelayVMSpec` gains `SeedISO` and `Notes`;
    `buildRelayVMScript` adds `Add-VMDvdDrive -Path` and `Set-VM -Notes`
    while the VM is off, both escaped, and keeps the build NIC on
    `BuildSwitch`. (BOOT-01, GLOBAL-06; Decisions 1, 4, 7, a1, a2, a11)
26. `Provisioner` (rework, `provision.go`, `provision_local_windows.go`,
    `provision_local_other.go`, test fakes): `RelayNotes(ctx)` (name to
    Notes of each `warden-relay-*` VM) and `SwitchAdapters(ctx, switches)`
    (switch to attached VM names), Hyper-V Administrators tier, not
    delegated to the service. (ORCH-01, GLOBAL-07; Decisions 4, 7, a2, a11)
27. `allocate.go` (new, pure core): parse `warden-relay-ip=` and
    `warden-relay-switch=` from Notes; the lowest host of the NAT prefix
    that is not the network, broadcast, host IP or a recorded address, a
    full prefix a `driver.Fixable` naming the holders; for durable builds
    the lowest slot switch with no adapter attached (a build VM alone counts
    busy), all busy a `driver.Fixable` before any VM naming each slot's
    holders and the fixes (wait, remove a kept VM, `warden setup --driver
    hyperv --slots <more>`, elevated). `StartRelay` holds
    `driver.HostLock("hyperv-network")` from the listing through
    `CreateRelayVM`. (GLOBAL-06, BOOT-01, UX-04, GLOBAL-11; Decisions 4, 7,
    a2, a11)
28. NAT read back (rework, `provision.go`, `provision_local_windows.go`,
    `service.go`): `NetworkResources` gains `NATPrefix` and `HostIP`; the
    `EnsureNetwork` script picks the dev switch's NAT (`<devsw>-nat`) when it
    exists, else a warden NAT owning `DefaultNATPrefix`, else a new one, and
    prints the NAT, its `InternalIPInterfaceAddressPrefix` and the host
    vNIC's address in it; `parseEnsureNetworkOutput` reads them;
    `VerifyNetwork` returns the durable NAT's prefix and host IP and checks
    the slot switches exist and are Private; a NAT whose host vNIC has no
    address in its prefix, or no slot switches, is a `driver.Fixable` naming
    `warden setup --driver hyperv` (elevated); `serviceProtocolVersion`
    goes to 2 (plan note). (UX-04, UX-08, ORCH-01, GLOBAL-11; Decisions 5,
    7, 11, a5)
29. `driver_windows.go` (rework, `provisionBuildNetwork` and `StartBuild`):
    return the NAT `{name, prefix, hostIP}`; the durable case names no
    build switch (the allocator does); the ephemeral case keeps its
    per-build switch and takes the NAT the script chose; before any VM, a
    NAT prefix overlapping the link is a `driver.Fixable` naming `[vm]
    build_link` and `warden setup --nat-prefix`, and the resolvers are
    resolved (`Check` of `req.DNS`, else `Select` over the Windows
    default-route source) with the rule loopback, link-local, the host's
    addresses, the NAT prefix and the link, and set into `req.DNS`.
    (UX-04, BOOT-01, ORCH-01, GLOBAL-10; Decisions 5, 6, a3, a5, a14)
30. The Windows default-route DNS source: `Get-NetRoute -DestinationPrefix
    0.0.0.0/0` (lowest metric) and `Get-DnsClientServerAddress
    -InterfaceIndex <i> -AddressFamily IPv4` in a `windows`-tagged file,
    its output parser pure and untagged; the refusal names the interface.
    (GLOBAL-07, UX-04; Decision 6, a3)
31. `relayhost.go` (rework, `StartRelay`): `defaultRelayNATIP` and the "no
    per-instance claim token" comment go; `RelayHostConfig` gains the NAT
    prefix and host IP, the slot switches (durable) or the build switch
    (ephemeral), the seed directory and the ISO builder (plan note);
    `StartRelay` takes `driver.RandomToken()`, allocates under the lock
    (task 27), stages and packs the seed, creates the relay VM with the DVD,
    the Notes (`warden-relay-ip=<ip> warden-relay-switch=<sw>`) and the build
    NIC, releases the lock, starts it, and claims `<relayIP>:8301` with the
    token; `RelayHost` exposes the build switch and the relay address. The
    token is never logged. (RELAY-03, GLOBAL-06, RELAY-08, UX-04; Decisions
    1, 3, 4, 7, a1, a2, a11)
32. `buildhost.go` (rework, `runBuild`, `validateBuildConfig`): the build VM
    attaches to `rh`'s build switch; on a durable slot, before the build VM
    starts, the slot's adapters must be exactly the relay VM's and the build
    VM's, else the build is refused (plan note); `--keep-vms` keeps both VMs,
    and so the slot and the address. (BOOT-01, GLOBAL-06; Decision 7, a11)
33. `setup.go`, `capability*.go` (rework): `SetupOptions.Slots` (default
    4); setup creates `<network>-0..N-1` Private switches idempotently,
    removes unattached ones above N, refuses to remove an attached one
    naming its VMs, and retires the old `<network>` switch when nothing is
    attached (saying so), refusing with the VMs named otherwise;
    `DevSwitchPresent` probes the slot switches, and a host with only the
    old switch gets the "run `warden setup --driver hyperv` (elevated) to
    create the build switches" reason. (UX-04, UX-03, BOOT-01; Decision 7,
    a11)
34. `buildassets.go`, `driver/hostlink.go` (rework): `relayBootChannel = 2`;
    `LauncherStamp` gains `Want` (zero keeps `buildlink.Channel`) used by
    `Check` and `Report`; Hyper-V's relay-boot stamp sets it, so a disk
    stamped 1 or unstamped is refused before any VM and `BLOCKED` in doctor,
    naming `refresh-cache.sh`. (UX-04, ORCH-01; Decision 9)
35. `doctor.go` (rework, `Doctor(w, caps, link, dns)`): the NAT name, its
    prefix and host IP read back, an overlap with the link `BLOCKED`, the
    resolvers and their source or the refusal, each slot free or its
    holders (from Notes and adapters), the no-slots message, and the stamp.
    (UX-04, UX-09; Decisions 5 to 7, 9)
36. Tier-0 `driver/hyperv` tests (TEST-02 tier 0; PowerShell renderers and
    parsers in untagged or `windows`-tagged parse tests, as today):
    `relayseed` renders every key, holds no resolver, refuses a bad IP,
    prefix, gateway or token; `buildRelayVMScript` adds the DVD and the
    Notes with escaping and puts the build NIC on the given switch; the
    allocator over canned Notes and adapter lists (lowest free address,
    skipping network, broadcast, host IP and recorded; a full prefix
    refused naming holders; lowest unattached slot; a build VM alone busy;
    all busy refused naming holders and `--slots`); setup's slot script
    (create, trim, refuse an attached one, retire an unattached old switch);
    `DevSwitchPresent` and the no-slots reason; `parseEnsureNetworkOutput`
    with prefix and host IP; the NAT choice per strategy; the overlap
    refusal; the Windows DNS parser on canned output; the slot's foreign
    adapter refusal; `StartRelay` end to end on the fake `Provisioner` (the
    token in the seed and the claim's `Bearer`, the claim at the allocated
    address, `[vm] dns` exactly the claim's `upstream_dns` with the host
    source never read and no resolver in the seed, teardown removing the
    VM); the stamp (1, missing refused, 2 accepted, each naming
    `refresh-cache.sh`). (TEST-02, TEST-01, TEST-08, UX-04)

### The Windows guest on the lease

37. `driver/winguest` (rework, `run.go`, `winguest.go`): `WardenRunPS1`
    writes `initialize --gateway=<relay>` with no `--ip` (its `Emit` line
    too); `BuildGuestCIDR` goes. The Hyper-V and qemu Windows seeds pick it
    up unchanged. (EXEC-04, BOOT-03, BOOT-06; Decision 8, a6, a12)
38. `cmd/warden-io` (rework, `initialize.go`, `platform_*.go`): a failed
    relay wait appends a per-platform diagnostic; on Windows it names each
    IPv4 address with its `PrefixOrigin` and says "no DHCP lease from the
    relay" for a `WellKnown` 169.254 address (rendering pure, the query
    behind the `windows` tag; other platforms add nothing). The Windows
    `--ip` branch is unchanged. (EXEC-04, BOOT-03, UX-04, GLOBAL-07;
    Decision 8, a12)
39. Tier-0 tests (TEST-02 tier 0): `WardenRunPS1` has no `--ip` for the
    default and a configured link; `driver/hyperv`'s and `driver/qemu`'s
    Windows seeds carry that script; `platform_windows_test.go` renders the
    diagnostic for a leased, a `WellKnown` and a missing address. (TEST-02,
    TEST-01, EXEC-04)

### Docs

40. `docs/book/src/concepts/overview.md` pseudocode: the claim's
    `upstream_dns` and the relay's resolver flow (every mode), the relay VM
    init's key reader and seed, the Hyper-V build flow (NAT read back,
    allocation and slot under the lock, the seed, the token), the shared
    resolver selection per VM driver, and the Windows launcher's
    `initialize --gateway`; `architecture.md` and `network-isolation.md`
    lose `192.168.240.2`, `8.8.8.8` and `warden.dns` for the per-build
    address, the token, the slots and the claim's resolvers. (FLOW-15,
    UX-01)
41. `docs/book/src/guide/`: `configuration.md` gains `[vm] dns` (shape,
    default the host's resolvers, used exclusively when set, the one named
    fallback being its own order, read by every VM driver, ignored by the
    container driver, validated at load and per driver); `cli.md` covers
    `setup --slots` for hyperv, `--nat-prefix`/`--host-ip` taking effect on
    every build, and doctor's new lines; `drivers.md` covers Hyper-V (one
    `refresh-cache.sh`, one elevated setup for the slots, concurrent builds,
    the claim token, resolvers), qemu and Firecracker (resolvers in the
    claim) and vz (resolvers in the claim; split DNS needs `[vm] dns`).
    (UX-01, UX-03, ORCH-03, GLOBAL-09, GLOBAL-10)
42. `tools/relay-vm/README.md` (new): the relay VM init's boot channel for
    third-party orchestrators: the keys and their checks, the `WARDENRELAY`
    seed ISO on Hyper-V, `warden.dns` retired for `upstream_dns`, and a
    static upstream needing `upstream_dns` in the claim. (UX-01, BOOT-06,
    RELAY-08; Compatibility)

### The approved rule changes

43. Rule change BOOT-06 (approved at `cbb2cda`), in
    `docs/development/bootstrap.md` only: the second paragraph exactly as
    the design's **Rule changes** words it, and the model's diagram steps
    (1) and (3) and per-driver rows as the **Bootstrap comparison**'s
    Proposed model states them. (FLOW-14, BOOT-06, BOOT-07; Decision 10, a7)
44. Rule change BOOT-07 (approved at `0df0a64`), in
    `docs/development/bootstrap.md` and `tools/tier1/rules.yaml`: the
    surface sentence as worded, and `bootstrap.surface` gaining
    `tools/relay-vm/`, `tools/build-vm/`, `"cmd/relay/dhcp*.go"` and
    `"driver/hyperv/relayseed*.go"`. (FLOW-14, BOOT-07; Decision 10, a8)

### Lint, higher tiers and validation

45. `make test`, and `make lint` for linux (`driver/resolvers`,
    Firecracker, `cmd/warden`, `relay`, `relaycfg`, `cmd/relay`), darwin
    (`driver/vz`, `driver/qemu`) and windows (`driver/hyperv`,
    `driver/winguest`, `cmd/warden-io`). (FLOW-17, GLOBAL-07, TEST-01)
46. Tier 1 (TEST-02 tier 1): the `integration` workflow green on the
    branch, which runs the container driver's relay with the absent-field
    default. (TEST-02, RELAY-04)
47. Tier 2, CI (TEST-02 tier 2): `firecracker.yml` rebuilds its relay
    initramfs from the changed init and runs its disk and direct-boot
    builds with the resolvers in the claim. (TEST-02, ORCH-01)
48. Tier 2, native (TEST-02 tier 2, FLOW-09), after consolidation, by the
    existing steps and their notes: `f6-validate-windows-hyperv` (cases 1 to
    9, tasks 20 to 38), `f6-validate-qemu-any` (cases 1 to 3, tasks 13, 20)
    and `f6-validate-macos-vz` (cases 1 to 3, task 14). (FLOW-09, TEST-02)
49. Tracked gap (TEST-03): the existing backlog entry
    `hyperv-relay-per-build-channel-qemu-windows` (macos-qemu) stands; no
    new entry. (TEST-03, FLOW-09)
50. TEST-08: the Security note's list goes to `security-suite` at
    Consolidation, with the tier-0 cases above named as its regression
    seeds. (TEST-08, GLOBAL-06)

## Design review

Required (FLOW-19): `alters_relationships: true`, the `bootstrap-model`
and (r3) `relay-claim` seams, `security_boundary: env-relay`, blocking
native validation (windows-hyperv; r3 adds qemu-any and macos-vz), assumed
questions (a1 to a9 at r1; a10 to a13 at r2; a14 to a16 open for r3), and
rule changes BOOT-06 (reworded at r3, to re-approve) and BOOT-07.

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

- **r2 re-review, 2026-11-09: changes requested** (Jeff, workflow-human,
  FLOW-19). Read at `7be917d`; it covers the delta since `0df0a64`. Sent
  back to Design (FLOW-06).
  - **a10, `[vm] dns`: changed.** Jeff asked what the key configures. It
    sets the relay's upstream resolvers; the build VM keeps using the relay
    as its DNS. That value is not needed before the claim, so:
    - **It is delivered in the claim.** The claim gains an upstream-DNS
      field, so the `relay-claim` seam joins `seams`.
    - The relay sets its resolver from the claim. The seed and Firecracker's
      command line stop carrying `warden.dns`.
    - **Every VM driver honours it, qemu and vz included.** Nothing refuses
      `[vm] dns` any more.
    - What r2 decided about the value itself stands: `[vm] dns` is used
      exclusively when set; otherwise the host's resolvers, selected by
      Firecracker's rule; refused when neither yields one.
  - **The new rule (Jeff):** "from here onwards, any configuration of the
    relay should happen in the claim interface explicitly." Only what the
    relay needs to receive a claim (its own address, prefix, gateway and the
    claim token) comes from the boot channel, the seed or the kernel command
    line.
    - **Design r3 rewords the proposed BOOT-06 paragraph to say this.** It
      currently reads "the relay gets those from its claim, its kernel
      command line or a per-build relay seed". So BOOT-06 is re-approved at
      the r3 review; its approval at 0df0a64 no longer covers the wording.
    - The same rule is also written into the design-principles item.
    - BOOT-07's entry is unaffected, and its approval at 0df0a64 stands.
  - **Confirmed (a):** a11 (setup-made per-build Private switches,
    `--slots` default 4), a12 (the Windows guest keeps the relay's lease)
    and a13 (no runtime fallbacks; SMBIOS a named contingency).

- **Design r3, 2026-11-11** (FLOW-06; the finding is the r2 re-review's
  notes above, with RELAY-08 and GLOBAL-09 to GLOBAL-11 now on `main`).
  The branch merged `origin/main` first (`beb51ba` design-principles, #104;
  `f861315` rule-running-line-match, #105). What it changes since
  `7be917d`:
  - **Decision 6 (a10's change):** the relay's upstream resolvers become a
    claim field, `relaycfg.Config.UpstreamDNS` (`upstream_dns`), sent by
    every VM driver: `[vm] dns` exclusively when set, else the host's by the
    shared selection, each driver with its own source and reachability
    table; refused when neither yields one. They leave the Hyper-V seed and
    Firecracker's command line (`warden.dns`). r2's qemu and vz startup
    refusal goes. The relay uses one list for the guest's queries and its
    own dials in all three modes; `UPSTREAM_DNS`, `8.8.8.8` and the
    system-first split go; a claim without the field uses the relay's own
    `resolv.conf`, the named default. Loopback and link-local move from
    `[vm] dns`'s load-time check to the drivers' rules (vz's host relay can
    reach loopback). Questions a14, a15.
  - **Decision 2:** the seed and the init carry only RELAY-08's pre-claim
    values (address, prefix, gateway, token); the init retires `warden.dns`
    and stops writing `resolv.conf`.
  - **Decision 9:** the claim field's skew (stamp 2 on Hyper-V, the relay
    appended per build elsewhere; an unknown-field refusal of any claim
    field becomes the old-relay `Fixable`).
  - **Decision 10, Decision 11's resolver row, and the BOOT-06 rule change**
    reworded around RELAY-08: the relay gets only its pre-claim values from
    a seed or the kernel command line. **BOOT-06 needs re-approval** at this
    review; its first paragraph, severity, check and enforcement are as
    approved at `0df0a64`. **BOOT-07's entry is unchanged** (approval at
    `0df0a64` stands).
  - **Defaults and overrides** filled (template section required since
    `beb51ba`): every default, its override, the one named fallback
    (`upstream_dns`'s order) and what was removed.
  - **Impact fields:** `seams` adds `relay-claim`; `rules` adds GLOBAL-09,
    GLOBAL-10, GLOBAL-11 and RELAY-08; `drivers` and `surfaces` comments
    reworded (every VM driver reads `[vm] dns`); qemu-any and macos-vz
    become blocking with steps `f6-validate-qemu-any` and
    `f6-validate-macos-vz` (question a16), the qemu-any backlog entry folded
    into the step, and `tier2_gaps` keeps macos-qemu only.
  - Updated with them: Prior art, Design (pseudocode, the relay's resolver
    flow, sites, Compatibility), Cross-driver impact, Bootstrap comparison
    (step (1), the BOOT-06 row, gains and costs), Security note, Validation
    plan (tier 0, native windows-hyperv case 2, the qemu-any and macos-vz
    cases), Implementation plan outline, the `f6-validate-windows-hyperv`
    note.
  - Unchanged and still approved or confirmed: Decisions 1, 3 to 5, 7 and 8,
    a1 to a9 and a11 to a13.
- **Re-review r3, 2026-11-11: approved at `cbb2cda`** (Jeff, workflow-human,
  FLOW-19; page https://claude.ai/artifact/VnT6LdGvg8wBUmVHhG3ZkS). Rule change
  BOOT-06 approved at `cbb2cda` (the r3 paragraph citing RELAY-08); rule change
  BOOT-07's approval at `0df0a64` stands. Assumptions a14 (every VM driver
  always sends `upstream_dns`; an absent field means the relay's own
  `resolv.conf`), a15 (one ordered list fixed at the claim, all modes) and a16
  (qemu-any and macos-vz blocking) confirmed as drafted, (a) each. Accepted
  with them: container mode loses its `8.8.8.8` fallback, and vz no longer
  follows macOS split DNS unless `[vm] dns` names those resolvers. What
  changed since r2: Decisions 2 and 6, the BOOT-06 paragraph, Defaults and
  overrides, the blocking hosts. No changes folded in.
- **2026-11-11, during f3-implement: approved at `71df1d1`** (Jeff, attended,
  FLOW-19 fold-in, no second review). Two changes folded in at `71df1d1`:
  `drivers:` adds `container` (task 8's shared `driver.BuildRequest.DNS` and
  the container relay's new resolver default, per Decision 6 and RELAY-04;
  tier 1's ORCH-01 drivers claim), and the relay's own lookups resolve
  through the DNS handler's in-order forwarder rather than a `net.Resolver`,
  whose retries restart at the first server (Decision 6, its pseudocode and
  plan task 5). Rule change approvals are unchanged: BOOT-06 at `cbb2cda`,
  BOOT-07 at `0df0a64`.

## Consolidation

`main` merged at `1173c5f` (merge commit `5ad9308`). The branch forked at
`beb51ba` (#104 design-principles: GLOBAL-09 to GLOBAL-11 and RELAY-08,
which this design's Defaults and overrides and its claim-only relay
configuration answer); #105 (FLOW-14 guards the CI jobs that run the rules)
was already in that base. Since the fork, five PRs landed:

- **#108 vz-image-prepare-refresh.** The one conflict, in
  `driver/vz/driver.go` `StartBuild`: #108 passes the image cache to
  `checkLink` so the stale-image fix names the active image, and this branch
  wraps `checkLink` in `checkRequest` to set the relay's resolvers (task 14).
  Resolved keeping both: `checkRequest` takes the `*ImageCache` and passes it
  on, then runs `setRelayDNS`. `overview.md`, `cli.md`, `drivers.md` and
  `installation.md` merged clean; the vz pre-boot pseudocode now has #108's
  active-image line and stamp fixes followed by this branch's resolver step,
  which is the order `checkRequest` runs. #108 does not touch the host-mode
  relay or its claim, so the macos-vz step's cases are unchanged; its native
  run builds from the active image.
- **#110 build-vm-busybox.** `tools/build-vm/build-initramfs.sh` fetches its
  own pinned busybox (and `TestDirectBootBusyboxPins`). The file is on the
  bootstrap surface this branch adds (BOOT-07, `tools/build-vm/`); it changes
  where the qemu direct-boot initramfs gets busybox, not any network value,
  so BOOT-06 as amended holds. The relay initramfs the qemu-any step rebuilds
  comes from `tools/relay-vm/`, untouched by #110.
- **#109 jks-test-skip-without-jdk**, **#106 next-skips-merge-wait** and
  **#107 wf-squash-after-base-moved**: test, CI and workflow changes with no
  overlap.

**Fold-in after review r3.** Jeff's fold-in at `71df1d1` (recorded at
`4f54198`, Design review): `drivers:` adds `container`, and the relay's own
lookups go through the DNS handler's in-order forwarder rather than a
`net.Resolver`. Both are in the code (`driver.BuildRequest.DNS`, which the
container driver leaves empty; `relay/proxy.go`'s upstream dial), and
Review S4 below is about how tightly the second is pinned.

**Versioned seams** (GLOBAL-08). `relay-claim`: changed only by this feature
since the fork (`upstream_dns`, its 400 refusals, the absent-field default in
`relaycfg`'s package doc). `bootstrap-model`: only this feature's BOOT-06 and
BOOT-07 paragraphs, landed as approved (`cbb2cda`, `0df0a64`); `wf check`
clears both and `make enforcement` keeps main's findings. `executor-initialize`
and `relay-guest-api`: unchanged.

**Terminology.** Code, guides, `overview.md` and doctor say "relay seed
(`WARDENRELAY`)" for the relay VM's pre-claim channel, "seed DVD" for the
drive it is on, "`WARDEN` seed disk" for the Windows guest's, "claim token",
"`[vm] dns`", "`upstream_dns`", "slot switch" for the durable strategy's
per-build switches, and "link-channel stamp". One fix here (Review S3):
`tools/relay-vm/hyperv/build.sh`'s closing line said a per-build kernel
command line is layered by the driver; it now says the command line carries
no per-build value, the pre-claim values come on the relay seed and the rest
in the claim.

**Review findings for the PR body** (report
`reports/hyperv-relay-per-build-channel/c828e3a562e4.md`, 0 MUST; M1 to M3
fixed in Implement). Six SHOULDs, for the human to acknowledge at merge
(FLOW-08); S3 is fixed above, the other five are open:

- *S1 (GLOBAL-09), open.* `tools/relay-vm/init` keeps `warden.net=dhcp` when
  the key is absent; no warden driver reaches it (each sends `warden.net`),
  and Defaults and overrides does not list it. Fix: require the key, or list
  it through Design.
- *S2 (GLOBAL-09), open.* `ensureSetupNAT`'s prefix-mismatch refusal compares
  the defaulted `--nat-prefix`, so re-running setup without the flag on a host
  set up on another prefix is refused naming a flag the user did not pass;
  `--host-ip` is not compared. Fix: compare only flags the user gave.
- *S3 (GLOBAL-01), fixed at Consolidation.* `build.sh`'s closing line (above).
- *S4 (TEST-06), open.* The in-order forwarder for the relay's own lookups
  is not pinned against a resolver that restarts at the first server.
- *S5 (TEST-06), open.* `serveClaim`'s refusal path (the mode step's cleanup,
  the relay left claimable) has no test through the vm- and host-mode wiring.
- *S6 (TEST-06), open.* Nothing fails if the qemu or vz `StartBuild` stops
  calling `setRelayDNS`; the qemu-any and macos-vz native steps cover it.

**Follow-ups filed** (FLOW-09, at review `525e36a6c04c`), both `follows:
hyperv-relay-per-build-channel` and on hold behind Jeff's refactor (q1):
`relay-host-subnet-claim-only` (priority 45: host mode's `--subnet` stands
in for an absent `build_link`, RELAY-08) and `relay-init-dhcp-mask-default`
(priority 46: the relay init's `${mask:-24}` for a lease without a mask,
GLOBAL-11). Both are pre-existing on `main`. Consolidation found no further
regression on `main` and files none.

**Validation and plan tasks 46 to 50.**

- *Task 50 (TEST-08), done here.* The Security note's "TEST-08 should
  exercise" list is handed to `security-suite` as a history entry in its
  queue file (workflow branch `450820d`), as build-link-subnet's was, with
  the tier-0 regression seeds: `driver/hyperv` `TestStartRelay_WrongTokenRefused`,
  `TestAllocRelayIP`, `TestAllocSlot`, `TestAllocateRelay`,
  `TestRunBuild_DurableSlot`, `TestRunBuild_SlotForeignAdapter`,
  `TestCheckSlotAdapters`, `TestRelaySeedValidate`, `TestCheckNATOverlap`,
  `TestVMDNSUsedExclusively`; `driver/relayvm` `TestRelayInitHyperVSeedAlone`;
  `driver/qemu` and `driver/vz` `TestVMDNS`; `relaycfg`
  `TestClaimUpstreamDNSRefusalsLeaveRelayClaimable`; `relay`
  `TestDNSHandlerForwardsToListInOrder`, `TestUpstreamDialResolvesThroughList`,
  `TestNoBuiltInResolver`; `cmd/relay`
  `TestAbsentUpstreamDNSRefusalLeavesRelayClaimable`.
- *Task 49 (TEST-03).* The tracked gap's backlog entry
  `hyperv-relay-per-build-channel-qemu-windows` (macos-qemu) stands; no new
  entry.
- *Task 48 (FLOW-09).* The three blocking native steps run on the branch
  after this commit, by their notes, which the merge does not change.
- *Tasks 46 and 47.* The `integration` workflow (the container relay's
  absent-field default) and `firecracker.yml` (relay initramfs rebuilt from
  the changed init, resolvers in the claim) run on the PR, at Publish.

**FLOW-15 sweep.**

- *Dead code.* `make deadcode` on linux, darwin and windows, compared with
  `main`: one entry leaves the list (`driver/hyperv` `cidrPrefixLen`, which
  `ensurenet.go` now calls) and two join it on linux and darwin only,
  `parseRelayNotes` and `parseSwitchAdapters` in
  `driver/hyperv/provision.go`: untagged parsers of the PowerShell output
  whose only caller is the Windows `Provisioner`, so reachable on windows.
  No test calls them on any platform: plan task 36 puts the `Provisioner`'s
  parsers under tier-0 parse tests, and these two have none, so it is
  reported to the tick as work the plan covers (`wf back -to 3`), not done
  here. Everything else reported predates the branch.
- *Duplication.* The four drivers' `relayResolvers` share one shape (`[vm]
  dns` through `resolvers.Check`, else the host source through
  `resolvers.Select`, else a fix) and `vmDNSRefusal` is the same in
  firecracker, hyperv and qemu (vz inlines its own, since its rule is the
  build link only). Kept: plan task 10 puts selection in `driver/resolvers`
  and has "each driver keep its own rule and fix text", and the refusals
  differ in what they name (the NAT, user-mode NAT, the Mac). The seed
  parsing and the claim path have one definition each (`relayseed.go`,
  `driver.Session.ClaimRelay`).
- *The overview.* `overview.md` matches the code after the merge: the VM
  drivers' pre-boot steps (#108's active image, then the resolver step), the
  Hyper-V setup and StartBuild flow (NAT read back, allocator, slots, seed),
  the relay's claim handling of `upstream_dns` and the relay VM init.

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

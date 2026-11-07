---
feature: hyperv-relay-per-build-channel
status: draft
drivers: [qemu, firecracker, hyperv]  # hyperv: the relay seed, the claim token, the per-build relay address, the NAT read back from the host, the host's resolvers, the durable switch's per-build VLAN, the relay-boot stamp. qemu and firecracker, conservatively (FLOW-02): tools/relay-vm/init is shared by every vm-mode relay and loses its baked network defaults (Decision 2), the resolver selection moves out of driver/firecracker into a shared package (Decision 6), and cmd/warden-io's Windows network step also runs on qemu Windows guests (Decision 8). container and vz are unaffected: their relays run in container and host mode and never boot tools/relay-vm/init
seams: [bootstrap-model]  # BOOT-06 gains the per-build network-value invariant, and BOOT-07's surface list grows (Rule changes; Bootstrap comparison). relay-claim is unchanged: the claim token is already part of the protocol (relaycfg, Bearer on /v1/claim and the tunnel); Hyper-V only starts sending one. relay-guest-api and executor-initialize are unchanged: --gateway/--ip keep their contract, and Decision 8 makes the Windows executor do what --ip already says
surfaces: [cli, output]  # cli: `warden setup --driver hyperv --nat-prefix/--host-ip` now take effect on every build (their reference text changes, UX-01); output: doctor's NAT, relay-resolver and durable-switch lines, the new fixable refusals (UX-04)
security_boundary: env-relay  # the relay VM gains a per-build input (the seed), its claim gains a token, and durable builds stop sharing one build segment (a build VM could reach another build's relay). Also the host's claim path to the relay (the token); Security note
alters_relationships: true   # the bootstrap model's BOOT-06 changes (FLOW-02), and the Hyper-V relay VM's upstream configuration moves from defaults baked in its init to a per-build seed the orchestrator writes; FLOW-05 slot
design_review: required      # FLOW-19: alters_relationships, a seam, a security boundary, blocking native validation, assumed questions and rule changes
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "reaches driver/hyperv end to end (relay seed ISO on the relay VM, the relay initramfs's new storage modules, claim token, per-build relay address on the NAT, the NAT prefix and host IP read back from the host, host resolvers, the durable switch's per-build VLAN, the relay-boot stamp bump) and cmd/warden-io's Windows network step, behaviour only a Hyper-V host exercises"}
  - {host: qemu-any, blocking: false, reason: "regression only: tools/relay-vm/init loses its 192.168.240.x and 8.8.8.8 fallbacks, which a qemu relay (warden.net=dhcp from user-mode NAT, resolver from its lease) never takes; a rebuilt aarch64 relay initramfs booting a Linux build is the check (backlog hyperv-relay-per-build-channel-qemu-relay)"}
  - {host: macos-qemu, blocking: false, reason: "the Windows executor's static-address step (Decision 8) also runs on qemu Windows guests, whose builds already fail on main (backlog qemu-windows-guest-package); windows-hyperv validates the step itself (backlog hyperv-relay-per-build-channel-qemu-windows)"}
tier2_gaps:
  - {host: qemu-any, gap: "a qemu Linux build on a relay initramfs rebuilt from the branch's tools/relay-vm/init (backlog hyperv-relay-per-build-channel-qemu-relay)"}
  - {host: macos-qemu, gap: "a qemu Windows guest ends with DHCP disabled and the seed's static address (backlog hyperv-relay-per-build-channel-qemu-windows), after qemu-windows-guest-package is fixed"}
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, ORCH-01, ORCH-03, BOOT-01, BOOT-02, BOOT-03, BOOT-05, BOOT-06, BOOT-07, EXEC-04, RELAY-03, RELAY-04, UX-01, UX-04, UX-08, UX-09, TEST-01, TEST-02, TEST-03, TEST-08, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19]
rule_changes:
  - {rule: BOOT-06, change: amend, paths: [docs/development/bootstrap.md]}
  - {rule: BOOT-07, change: amend, paths: [docs/development/bootstrap.md, tools/tier1/rules.yaml]}
concepts: [relay-seed, per-build-channel, claim-token, relay-claim, relay-vm-init, relay-address-allocation, nat-prefix, host-ip, upstream-dns, resolver-selection, durable-switch, vlan-isolation, two-vm-topology, network-isolation, launcher-stamp, stale-assets, windows-guest-network, dhcp]
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
  per-build claim token, both on the relay's command line). Reused: the
  selection rule, moved to a shared package so both drivers run one copy
  (FLOW-15), and the token's handling.
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
   | SMBIOS strings (`BIOSSerialNumber`, `ChassisAssetTag` via `Msvm_VirtualSystemSettingData`) | yes | none (`/sys/class/dmi/id`) | WMI `ModifySystemSettings` | Rejected for now: no cmdlet, undocumented length limits on Gen2, so it needs a spike; kept as the fallback if the DVD path fails native validation (Rollback) |
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

6. **The relay's upstream resolvers are the host's, chosen by Firecracker's
   rule, which moves to a shared package** (question a3). At build start the
   Hyper-V driver reads the IPv4 DNS servers of the interface that holds the
   host's lowest-metric IPv4 default route (`Get-NetRoute
   -DestinationPrefix 0.0.0.0/0`, then `Get-DnsClientServerAddress
   -InterfaceIndex <i> -AddressFamily IPv4`) and runs the shared selection:
   exclude loopback, link-local, addresses assigned to the host, addresses
   inside the NAT prefix and addresses inside the build link. None left is a
   `driver.Fixable` naming each excluded server and why, with the fix "give
   `<interface>` an off-host DNS server" (and, for an excluded link address,
   `[vm] build_link`). The chosen list goes into the seed as `warden.dns`;
   doctor prints it (or the refusal).

   `selectResolvers` and `inLink` move from `driver/firecracker/posture.go`
   into a new `driver/resolvers` package taking the link and a predicate for
   host-local addresses, with each driver keeping its own source
   (`/etc/resolv.conf` and systemd-resolved on Linux, the default-route
   interface on Windows) and its own fix text (FLOW-15: one selection rule,
   not two that drift).

   *Rejected:* keeping `8.8.8.8` (the bug). Falling back to public resolvers
   when no host resolver qualifies (silently changes which resolver a build's
   names come from, which the ledger records only as answers; Firecracker
   refuses instead, and the two drivers should agree). A new `[vm] dns` key
   (a new surface; recorded as a possible follow-up if the refusal proves
   common, for instance on hosts whose only resolver is a loopback DNS client
   such as a VPN agent's).

7. **A durable build gets its own access VLAN on the shared Private
   switch** (question a4; BOOT-01). The allocator of Decision 4 also picks a
   VLAN ID from 2 to 4094 that no adapter on the durable switch carries
   (`Get-VMNetworkAdapter -All` filtered by `SwitchName`, then
   `Get-VMNetworkAdapterVlan`), and `CreateRelayVM` and `CreateVM` set it on
   the relay's build NIC and the build VM's NIC with
   `Set-VMNetworkAdapterVlan -Access -VlanId <id>` before either VM starts.
   The relay's NIC carries the VLAN before the lock is released, so the next
   allocation sees it. Hyper-V's switch forwards a frame only between ports
   in the same access VLAN, so two durable builds on `warden-dev` share the
   switch and nothing else: no ARP, no DHCP, no traffic between them, and an
   untagged adapter (a VM a user attached by hand, or a build from an older
   warden) reaches neither. `VerifyNetwork` keeps checking the switch is
   Private. Ephemeral builds keep their per-build switch and set no VLAN.

   | Option | Isolation | Privilege per build | Concurrency | Verdict |
   |---|---|---|---|---|
   | **Per-build access VLAN on the durable switch** | per-port VLAN filtering in the vSwitch | Hyper-V Administrators (VM adapter settings) | yes | **Chosen** |
   | A per-build Private switch in the durable strategy too | a switch per build | `New-VMSwitch`, full Administrator (hyperv-driver-plan's privilege table) | yes | Rejected: the durable strategy exists for hosts where the user is not elevated |
   | One durable build at a time (a host lock held for the build) | one build per segment | none new | no | Rejected as the default; kept as the fallback if native validation finds VLAN filtering does not hold on a Private switch (Rollback) |
   | Leave it | none between concurrent builds | none | yes | Rejected: the bug |

8. **`warden-io initialize --ip` on Windows makes the NIC static: it turns
   DHCP off on the interface, then sets the address, route and DNS server,
   and logs the result** (question a6; EXEC-04). Today it runs
   `New-NetIPAddress ... -ErrorAction SilentlyContinue`, which on a NIC that
   already took the relay's lease fails silently (the address exists) and
   leaves DHCP on. It will instead run `Set-NetIPInterface -Dhcp Disabled`
   on the chosen interface, remove any other IPv4 address on it, add `--ip`
   with the default route via `--gateway` (skipping what is already
   present), set the DNS server, and write one line to stderr (so to
   `warden-run.log` and COM1) with the interface's final address, prefix
   origin and DHCP state. A final address other than `--ip` stays non-fatal
   as today, but that line now names it. The guest ends `PrefixOrigin
   Manual`, as on main before build-link-subnet, whether or not it took the
   lease first: once `initialize` runs, the guest's addressing no longer
   depends on the relay's responder, and `--ip` means what EXEC-04 says it
   means on every driver.

   *Rejected:* leaving the NIC on DHCP (harmless with an infinite lease, but
   the end state then depends on which of two channels answered first, and
   the Windows DHCP client re-requests on media events for the rest of the
   build). Dropping `--ip` from `warden-run.ps1` and relying on the lease
   alone, as vz does (loses the seed as the cross-check and fallback that the
   direct-boot inits keep, build-link-subnet Decision 5). Stopping the relay
   from answering Windows guests (the claim does not say which guest OS
   boots, and the responder is guest-agnostic by design).

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

11. **Out of scope.** The Linux build guest on Hyper-V (still not built).
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

- **a1. The pre-claim channel and the token** (Decisions 1, 3). Proceeding
  with a per-build seed ISO (IMAPI2, label `WARDENRELAY`) on a DVD attached
  to the relay VM before it starts, read once by the init on Hyper-V, holding
  the relay's NAT address, prefix, gateway, resolvers and a fresh 32-byte
  claim token that `StartRelay` then sends. Options: (a) the seed ISO; (b)
  KVP host-to-guest items; (c) SMBIOS strings set through WMI; (d) a claim
  over hvsock instead of the NAT. Recommended: (a), with (c) as the fallback
  if the DVD path fails native validation.
- **a2. Per-build relay address** (Decision 4). Proceeding with the lowest
  free host address of the NAT prefix, allocated under a host lock, the
  in-use set read from the Notes of existing `warden-relay-*` VMs, and the
  address written to the new VM's Notes before the lock is released.
  Options: (a) as stated; (b) lease files in the lock directory; (c) a
  per-address file lock held for the build. Recommended: (a).
- **a3. The relay's upstream DNS** (Decision 6). Proceeding with the host's
  resolvers from its default-route interface, filtered by Firecracker's rule
  (moved to a shared `driver/resolvers`), and a fixable refusal when none
  qualifies. Options: (a) as stated; (b) the same with a public-resolver
  fallback; (c) keep `8.8.8.8`; (d) a new `[vm] dns` key. Recommended: (a),
  with (d) as a follow-up only if the refusal proves common.
- **a4. The durable shared switch** (Decision 7). Proceeding with a
  per-build access VLAN on the durable Private switch, set on the relay's
  build NIC and the build VM's NIC before they start. Options: (a) per-build
  VLAN; (b) one durable build at a time; (c) a per-build Private switch
  (needs elevation). Recommended: (a), with (b) as the fallback if native
  validation finds VLAN filtering does not hold.
- **a5. `--nat-prefix` and `--host-ip`** (Decision 5). Proceeding with every
  build reading the NAT it uses back from the host (prefix from `Get-NetNat`,
  host IP from the NAT switch's host vNIC), ephemeral builds reusing the dev
  switch's NAT when it exists, and a NAT prefix overlapping the build link
  refused at build start and in doctor. Options: (a) as stated; (b) persist
  setup's flags and read them per build; (c) honour them on durable builds
  only. Recommended: (a).
- **a6. The Windows guest's NIC** (Decision 8; the windows-desktop finding).
  Proceeding with `warden-io initialize --ip` on Windows turning DHCP off on
  the interface and setting the static address, route and DNS, logging the
  final state, so the guest ends `Manual` whether or not it took the relay's
  lease first. Options: (a) make it static; (b) leave it DHCP-configured, as
  after build-link-subnet; (c) drop `--ip` and rely on the lease alone.
  Recommended: (a).
- **a7. BOOT-06's per-build extension** (Decision 10; rule change BOOT-06).
  Proceeding with the amendment in **Rule changes**, taken now that it holds
  on every driver, with the init's baked network defaults removed (Decision
  2). Options: (a) take it, and remove the defaults; (b) remove the defaults
  but leave BOOT-06 as it is (then `alters_relationships` and the
  `bootstrap-model` seam drop out, and the review is still required for the
  other reasons); (c) neither. Recommended: (a).
- **a8. BOOT-07's surface list** (Decision 10; rule change BOOT-07).
  Proceeding with adding `tools/relay-vm/`, `tools/build-vm/`,
  `cmd/relay/dhcp*.go` and `driver/hyperv/relayseed*.go` to the bootstrap
  surface in `bootstrap.md` and `tools/tier1/rules.yaml`. Options: (a) take
  it here; (b) leave it to a process item. Recommended: (a).
- **a9. Seams, relationships and validation** (front-matter). Proceeding
  with `seams: [bootstrap-model]`, `alters_relationships: true`,
  `security_boundary: env-relay`, windows-hyperv blocking, qemu-any and
  macos-qemu tracked. The FLOW-05 slot is free today: no other in-progress
  item declares `alters_relationships: true`. Options: (a) as stated; (b)
  add `relay-claim`, conservatively, although the wire is unchanged.
  Recommended: (a).

## Design

### A Hyper-V build, after the change

```
StartBuild
  caps := Detect(sw); strategy := caps.NetworkStrategy()
  resolveBuildDisks
    relay boot VHDX, its stamp == relayBootChannel (2)      else Fixable(refresh-cache.sh)
    CheckHostLink(link)
  provisionBuildNetwork -> buildSwitch, nat {name, prefix, hostIP}
    durable:   VerifyNetwork(sw) -> sw, read back <sw>-nat
    ephemeral: EnsureNetwork(per-build Private switch; NAT = <devsw>-nat if present,
               else one owning DefaultNATPrefix, else new) -> read back
  nat.prefix overlaps link?                                  -> Fixable (UX-04)
  dns := resolvers.Select(hostDefaultRouteDNS(), hostLocal, nat.prefix, link)
                                                             none -> Fixable
  token := RandomToken()
  with HostLock("hyperv-network"):
    relayIP := lowest free host in nat.prefix (not net, bcast, hostIP,
               nor any warden-relay-* VM's Notes)
    vlan    := durable ? free access VLAN on sw : none
    seed ISO <- warden.seed=1 warden.net=static warden.nat_ip=relayIP
                warden.nat_prefix=len(nat.prefix) warden.nat_gw=nat.hostIP
                warden.dns=dns warden.claim_token=token
    CreateRelayVM(..., DVD=seed ISO, Notes="warden-relay-ip=relayIP", vlan)
  StartVM(relay)
  ClaimRelay(relayIP:8301, token, build_link)                Bearer on claim and tunnel
  CreateVM(build, ..., vlan); StartVM(build)                 unchanged order
  ... wait, teardown: RemoveVM(relay) frees relayIP and vlan
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
| `driver/hyperv/relayvm.go` | `RelayVMSpec` gains `SeedISO`, `Notes`, `VLAN`; the script adds the DVD, the Notes and the build NIC's access VLAN while the VM is off | 1, 4, 7 |
| `driver/hyperv/provision*.go`, `VMSpec` | `CreateVM` sets the build NIC's access VLAN when given; `EnsureNetwork`/`VerifyNetwork` return the NAT's prefix and host IP; the ephemeral NAT choice prefers the dev switch's NAT; a lister of `warden-relay-*` Notes and of the VLANs on a switch | 4, 5, 7 |
| `driver/hyperv/relayhost.go` | `defaultRelayNATIP` goes; `StartRelay` takes the NAT, resolvers and link, allocates under the lock, writes the seed, claims with the token | 3, 4 |
| `driver/hyperv/driver_windows.go`, `doctor.go` | NAT read back, overlap refusal, resolver selection before any VM; doctor prints NAT, host IP, resolvers, stamp | 5, 6, 9 |
| `driver/hyperv/buildassets.go` | the relay-boot stamp compared with `relayBootChannel` | 9 |
| `driver/resolvers` (new), `driver/firecracker/posture.go` | the shared selection; Firecracker calls it with its own source and fix text | 6 |
| `cmd/warden-io/platform_windows.go` | `--ip` disables DHCP, sets the static address, logs the final state | 8 |
| `cmd/warden/host.go` | `--nat-prefix`/`--host-ip` help text: used by every build | 5 |
| `docs/development/bootstrap.md`, `tools/tier1/rules.yaml` | BOOT-06 and BOOT-07 as approved | 10 |
| docs: `overview.md`, `architecture.md`, `network-isolation.md`, `guide/cli.md`, `guide/drivers.md`, `tools/relay-vm` READMEs | the relay seed, the token, per-build address and VLAN, NAT read back, resolvers (UX-01, FLOW-15) | all |

### Compatibility

| Warden | Relay boot VHDX | Outcome |
|---|---|---|
| new | new (stamp 2) | the seed, the token, everything above |
| new | old (stamp 1, or no stamp) | refused before any VM: `refresh-cache.sh` |
| old | new (stamp 2) | refused before any VM by the old warden's stamp check (it expects 1): `refresh-cache.sh` from its own tree |
| third-party orchestrator | new, booted with no seed | the init stops with a console line; no relay starts. A third party passes the same keys in a `WARDENRELAY` ISO (documented in `tools/relay-vm/README`) |

qemu and Firecracker relay initramfs are not stamped and need no rebuild:
an old init still works with the new drivers (they changed nothing they
send), and a rebuilt one behaves the same on every path they use.

## Cross-driver impact

- **hyperv.** Everything in Decisions 1 to 9. Builds need one
  `refresh-cache.sh` (the stamp says so). A host whose only resolvers are
  loopback or host-local addresses now gets a fixable refusal where it used
  to get `8.8.8.8`. A durable network set up on a non-default prefix starts
  working. Concurrent builds, durable or ephemeral, no longer collide.
- **qemu.** The shared relay init loses fallbacks a qemu relay never takes;
  its claim, token and command line are unchanged. A qemu Windows guest
  ends with a static NIC (Decision 8). Tracked: qemu-any (relay init),
  macos-qemu (Windows guest).
- **firecracker.** The shared relay init (as qemu; Firecracker passes every
  key on its command line, so none of the removed defaults applied), and
  its resolver selection moves to `driver/resolvers` with no behaviour
  change. CI tier 2 (`firecracker.yml`, whose cache key hashes
  `tools/relay-vm`) rebuilds the relay initramfs and runs both builds.
- **vz.** Unaffected: its relay runs in host mode on the Mac and never boots
  the init; nothing in `driver/vz` or `cmd/relay` host mode changes.
- **container.** Unaffected: its relay runs in container mode with its token
  in the container's environment; nothing in `driver/container` changes.

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
gains a per-build relay seed, and step (3) on Windows ends with a static NIC:

```
  (1) isolate ------------------------------>  sole network path: the relay
      Hyper-V: write the relay seed (NAT       (relay VM reads its seed, sets its
      address, resolvers, claim token),        upstream, starts the relay with the
      boot the relay VM, claim it with the     token; durable builds get their own
      token and the build link; only then      access VLAN on the shared switch)
      boot the guest

  (3) start -------------------------------->  launcher: address from the seed,
                                               command line or relay's lease; then
                                               `warden-io initialize` (on Windows,
                                               --ip leaves the NIC static)
```

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | Topological; but concurrent durable Hyper-V builds share one Private segment, so a build's network path reaches other builds' relays and guests | Topological per build on every strategy: a durable build's relay and guest share an access VLAN no other port carries | hyperv durable: per-build VLAN; others: none |
| BOOT-02 the executor is the only out-of-band input | Seeds, `CIDATA`, tar copy, direct-boot initramfs; vz baked (D2) | Unchanged. The relay seed goes to the relay VM, not the build environment, and carries no build input | none |
| BOOT-03 one start, then hands off | Addressing in the launcher from the seed, a fixed address or the relay's lease; trust, DNS and the rest in `initialize` | Unchanged wording. On Windows, `initialize --ip` now makes the static address stick over a lease the NIC took first, which is still addressing by the platform's own configuration | qemu and hyperv Windows: `PrefixOrigin Manual` again |
| BOOT-04 build inputs come from the relay | CA, script and context fetched by the executor | Unchanged | none |
| BOOT-05 setup is the executor's domain | Baseline plus `/bootstrap` policy | Unchanged. The relay's resolvers are the relay's upstream, not the guest's setup; the guest's DNS is still the relay, set by `initialize` | none |
| BOOT-06 images carry hooks, not content | Generic hooks, nothing per-build, nothing tied to a warden version; but the Hyper-V relay VM's init bakes its upstream address, gateway and resolver, and every Hyper-V relay shares them | Adds: no boot asset or image, the relay VM's included, carries a network value the orchestrator chooses per build, and none bakes a default that stands in for a channel. Holds on every driver after this feature | hyperv: the relay seed; qemu, firecracker: the init's unused defaults removed |
| BOOT-07 the model changes by a signed comparison | Surface list without `tools/relay-vm/`, `tools/build-vm/`, the relay's DHCP responder | Unchanged rule; surface list extended (rule change BOOT-07) | tier 1 hint fires on those paths |

### Gains

Hyper-V's relay becomes like every other driver's: a per-build address, a
per-build token, the host's resolvers, the user's NAT prefix. The claim race
is closed: a local process or another build's orchestrator gets 401 and the
relay stays claimable. Concurrent Hyper-V builds work, and durable builds stop
sharing a segment, so each build's ledger records only what its own guest
received. BOOT-06 then states, for every driver, the principle
build-link-subnet established for the link, and the tier 1 hint covers the
files where it could regress.

### Costs

The relay VM gains a block device and the kernel modules to read an ISO 9660
image (code inside the trust boundary that parses host-written data; the
build VM cannot reach the relay VM's disks or devices). The relay seed holds
a secret on the host's disk for the build's lifetime, under the build's work
directory. The Hyper-V host's DNS configuration now decides the relay's
resolvers, so a host with only loopback resolvers gets a refusal it did not
get before. A third-party orchestrator that boots warden's relay VHDX must
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
relay. Three postures change: who can claim a Hyper-V relay (the token), what
a durable build's guest can reach on its segment (the VLAN), and what the
relay VM reads before its relay starts (the seed). RELAY-04: the relay binary
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
B directly. After: each durable build's two ports share an access VLAN no
other port carries, and Hyper-V's switch forwards nothing between VLANs, so
the segment holds exactly one relay and one guest, as an ephemeral build's
per-build switch does. A VLAN is set by the host on the VM's adapter; a guest
cannot change its port's VLAN. Allocation reads the VLANs every adapter on
the switch carries, under the host lock, so two builds never get one VLAN.

**The seed.** Host-written, read once by the init, from a device only the
relay VM has. The build VM cannot reach the relay VM's devices, the NAT
switch, or the host. Each value is checked against a strict pattern before
use, the file is never evaluated, and a bad value stops the init instead of
starting a misconfigured relay. The new kernel modules parse an ISO 9660
image the orchestrator built; nothing the build controls reaches them.

**Resolvers.** The relay's upstream resolvers move from `8.8.8.8` to the
host's. A build's names now resolve as the host's do, which can include
internal names; the relay records every answer it serves, as before. Loopback,
link-local, host-local and in-NAT or in-link resolvers are excluded, so the
relay never sends DNS to the host itself or into the build link.

**What else the change could break, and what guards it:**

- *An address conflict on the NAT.* The allocator excludes the host IP and
  every address a `warden-relay-*` VM records, under a host-wide lock; a
  full prefix is a refusal, not a guess.
- *Version skew.* The stamp value refuses a new warden on an old disk and an
  old warden on a new disk, both before any VM boots (Decision 9).
- *A NAT prefix that shadows the link.* Refused at build start and in doctor,
  and still by the relay (build-link-subnet Decision 4).
- *The Windows NIC change.* `initialize` disables DHCP only on the one
  interface it configures, which is the guest's only NIC (it faces the
  relay); the end state is what the seed already asked for.

**TEST-08 should exercise:** a claim and a tunnel with a missing or wrong
token are refused with 401 and the relay is still claimable by the right
token, on Hyper-V; two concurrent durable builds on one switch each succeed,
and from inside build VM A, ARP and ICMP to relay B's link address and to
build VM B, and a DHCP DISCOVER, get no answer from B; two concurrent
ephemeral builds get distinct relay addresses; a relay seed with a malformed
value, or no seed, starts no relay; a NAT prefix overlapping the link, and a
host with no qualifying resolver, are each refused before any VM boots. These
go to `security-suite` at Consolidation, as build-link-subnet's did.

## Validation plan

- **Tier 0** (TEST-01: pure Go, table-driven, no host network).
  `driver/hyperv`: `relayseed` renders every key and refuses a bad IP,
  prefix, gateway, resolver or token; `buildRelayVMScript` adds the DVD, the
  Notes and the access VLAN (and no VLAN when none is given), with escaping;
  the `CreateVM` script sets the VLAN; the allocator over canned VM lists
  (lowest free, skips network, broadcast, host IP and recorded addresses,
  full prefix refused with the holders named; VLAN skips those in use);
  `parseEnsureNetworkOutput` with the prefix and host IP; the NAT choice for
  each strategy (dev NAT present or not, a NAT owning the default prefix or
  not); the overlap refusal; `StartRelay` end to end over the fake
  `Provisioner`: the token reaches both the seed and the claim, the claim
  address is the allocated one, and teardown removes the VM; the stamp check
  against `relayBootChannel` (1 refused, 2 accepted, missing refused, each a
  `driver.Fixable` naming `refresh-cache.sh`); the Windows default-route DNS
  parser on canned output. `driver/resolvers`: every exclusion, and
  Firecracker's existing cases moved over unchanged. `cmd/warden-io`
  (`platform_windows.go`, under its build tag, rendering only): the
  PowerShell disables DHCP, removes other IPv4 addresses, adds `--ip` and the
  route, and logs the final state. `tools/relay-vm/init` as text: one key
  reader for both sources, never evaluates, the seed read only under
  `HYPERV`, no IPv4 literal anywhere in the file, the token's value never on
  a `say` line. `refresh-cache.sh` writes stamp 2.
- **Lint.** `make lint` for linux (`driver/resolvers`, firecracker) and
  windows (`driver/hyperv`, `cmd/warden-io`) (FLOW-17).
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
     token value; the claim carried a token (driver log); the guest's NIC
     ends `DHCP Enabled: No`, `PrefixOrigin Manual`, at the seed's address.
  3. *Token.* While a build's relay waits for its claim, a claim and a
     tunnel from the host with no token and with a wrong one get 401, and
     the build then succeeds.
  4. *Concurrency.* Two ephemeral builds at once both succeed, with
     different relay addresses (their VMs' Notes and consoles).
  5. *Durable, non-elevated.* After `warden setup --driver hyperv
     --nat-prefix 192.168.250.0/24 --host-ip 192.168.250.1` (and the
     default NAT removed), a Hyper-V Administrators build uses that prefix
     (relay console) and succeeds; doctor prints the prefix and host IP; an
     elevated build reuses the same NAT.
  6. *Durable concurrency.* Two durable builds at once on `warden-dev`
     succeed with different access VLANs; from build VM A (a build script
     that probes), ARP and ping to relay B's and build VM B's link addresses
     and a DHCP DISCOVER get no answer from B.
  7. *Refusals.* A `[vm] build_link` inside the durable NAT's prefix is
     refused at build start and in doctor; with the host's default-route
     interface given only `127.0.0.1` as DNS server, the build is refused
     naming it and the fix; a relay booted by hand with no seed prints the
     no-seed line and starts no relay.
  8. *Resolvers.* With the host's DNS pointed at an internal resolver that
     answers a name public DNS does not, a build resolves it.

  Tracked (TEST-03): **qemu-any**, a qemu Linux build on a relay initramfs
  rebuilt from the branch (backlog
  `hyperv-relay-per-build-channel-qemu-relay`); **macos-qemu**, a qemu
  Windows guest ends `Manual` (backlog
  `hyperv-relay-per-build-channel-qemu-windows`, after
  `qemu-windows-guest-package`). linux-kvm is covered by CI tier 2. Not run
  natively: macos-vz, container-any and macos-finch (unaffected).
- **Queue steps.** `f6-validate-windows-hyperv` (`host:windows-hyperv`) is
  added at Design with these cases in its note and no `needs`: a phase-6
  step is not offered before Publish (FLOW-04), and seeding Publish makes it
  need `f6-consolidate` and `f6-open-pr` need it.
- **Security (TEST-08).** The Security note's cases are covered at tier 0
  and in the native cases above; Consolidation hands the list to
  `security-suite`.

## Implementation plan

Written by Plan (`f2-plan`) from the design the review approves (FLOW-19).
The outline it starts from, in order:

1. Shared resolver selection: `driver/resolvers`, Firecracker moved onto it
   with no behaviour change (FLOW-15, TEST-01, GLOBAL-07).
2. Relay init and Hyper-V build tools: the key reader, value checks, the
   seed mount, defaults removed; the modules in `build.sh`; stamp 2 in
   `refresh-cache.sh` (BOOT-06, BOOT-01, RELAY-04, TEST-01).
3. `driver/hyperv`: relay seed, NAT read back and choice, allocator (address
   and VLAN), `RelayVMSpec`/`VMSpec` additions, `StartRelay` with the token,
   overlap and resolver refusals, stamp constant, doctor lines (ORCH-01,
   UX-04, UX-08, UX-09, GLOBAL-06, TEST-01).
4. `cmd/warden-io` Windows network step (EXEC-04, TEST-01).
5. `cmd/warden` setup help text and the reference and concept docs, the
   overview's pseudocode (UX-01, ORCH-03, FLOW-15).
6. The approved rule changes: BOOT-06 and BOOT-07 in `bootstrap.md`, the
   surface in `tools/tier1/rules.yaml` (FLOW-14, BOOT-07).
7. Backlog entries for the tracked gaps (TEST-03, FLOW-09).

## Design review

None yet. Required (FLOW-19): `alters_relationships: true`, the
`bootstrap-model` seam, `security_boundary: env-relay`, blocking windows-hyperv
validation, assumed questions a1 to a9, and rule changes BOOT-06 and BOOT-07.

## Consolidation

Filled in at Publish (FLOW-10, FLOW-15).

## Rollback notes

None yet. Named fallbacks if native validation finds a mechanism does not
hold on Hyper-V: SMBIOS strings for the seed (Decision 1), one durable build
at a time for the shared switch (Decision 7). Either is a return to Design
(FLOW-06).

## Archive summary

Filled in at Publish (FLOW-12).

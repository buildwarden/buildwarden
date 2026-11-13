---
feature: driver-contract
status: draft
drivers: [interface]     # driver.Driver itself changes, and every driver/<name>/ tree is reached
seams: [driver-contract] # new: the Go API between the kit and a driver becomes a supported seam (GLOBAL-08)
surfaces: [cli, config]  # no flag, subcommand or key is added, renamed or removed; declared for the
                         # differences listed under "Behaviour" (a5) and the reference-doc edits
security_boundary: env-relay  # conservative: every driver's relay start goes through the relay
                         # source. The relay artifacts and lookups are unchanged (Security note).
alters_relationships: true
design_review: required
native_validation:
  - {host: macos-vz, blocking: true, reason: "driver/vz is reached: its doctor, image prepare, list and verify, and its relay start move behind the contract"}
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv is reached: doctor, setup, image prepare, clean, the service commands, the relay boot disk lookup and --network move behind the contract"}
  - {host: qemu-any, blocking: true, reason: "driver/qemu is reached: doctor, the Linux image prepare, and the relay VM assets move behind the contract"}
  - {host: macos-finch, blocking: true, reason: "driver/container is reached: runtime detection, clean and the relay image move into the driver, and finch is the default runtime CI does not run"}
  - {host: macos-qemu, blocking: false, reason: "the qemu Windows prepare moves from cmd/warden into driver/qemu; a run needs an install ISO and hours, and the host's open backlog entry (qemu-windows-guest-package) already blocks it"}
tier2_gaps:
  - {host: macos-qemu, gap: "warden image prepare --driver qemu --os windows, then a Windows build, through the contract's PrepareImage (backlog driver-contract-qemu-windows)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10,
        GLOBAL-11, ORCH-01, ORCH-02, ORCH-03, ORCH-04, ORCH-05, RELAY-04, RELAY-07, RELAY-08,
        BOOT-02, BOOT-06, BOOT-07, UX-01, UX-03, UX-04, UX-05, UX-07, UX-08, UX-09,
        TEST-01, TEST-02, TEST-03, TEST-04, TEST-07, TEST-08, LINT-05,
        FLOW-01, FLOW-02, FLOW-05, FLOW-07, FLOW-09, FLOW-12, FLOW-14, FLOW-15, FLOW-19, FLOW-20]
rule_changes:
  - {rule: GLOBAL-08, change: amend, paths: [docs/development/global.md, tools/tier1/rules.yaml]}
  - {rule: ORCH-01, change: amend, paths: [docs/development/orchestrator.md, tools/tier1/rules.yaml]}
  - {rule: UX-01, change: amend, paths: [tools/tier1/rules.yaml, tools/tier1/rules/rules.go,
      tools/tier1/rules/rules_test.go, tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt,
      tools/tier1/cmd/tier1/main.go, tools/tier1/cmd/tier1/main_test.go]}
  - {rule: UX-03, change: amend, paths: [tools/tier1/rules.yaml, tools/tier1/rules/rules.go,
      tools/tier1/rules/rules_test.go, tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt,
      tools/tier1/cmd/tier1/main.go, tools/tier1/cmd/tier1/main_test.go]}
concepts: [driver-contract, driver-registration, driver-options, driver-owned-config, drivertest,
           relay-source, relay-release-forms, doctor, host-setup, image-prepare, image-list,
           image-verify, clean, driver-commands, build-request, kit, componentization, program]
related: [componentization.md, unified-bootstrap.md, firecracker-driver.md, build-link-subnet.md,
          tier1-static-checks.md, hyperv-relay-per-build-channel.md, hyperv-driver-plan.md,
          vm-driver-release-plan.md, platform-compatibility.md, ledger-environment-types.md,
          hyperv-windows-prepare.md, vz-image-prepare-refresh.md, hyperv-clean.md,
          guest-binary-cache-per-tree.md, design-approved-rule-changes.md, design-principles.md]
---

# Complete the driver contract in-tree, including the relay source (componentization K1)

This is a program item (FLOW-20): phase K1 of
[`componentization`](componentization.md). It designs within that design's
Decisions 4, 6, 7 and 18 and its answer a2, and does not reopen them.

It is not a move-only change in FLOW-20's sense. It adds a Go API and moves
code behind it. It is meant to change structure and not behaviour; the few
places where behaviour does change are listed under **Behaviour** and asked
as a5, and **What guards behaviour** says how the rest is held still.

## Problem

`driver.Driver` (`driver/driver.go`) has four methods: `Name`, `StartBuild`,
`Exec`, `Close`. Everything else a driver does is reached by `cmd/warden`
calling the driver's package by name. On `main` at `b903f65`, six files of
`cmd/warden` name a driver on about a hundred lines (the umbrella's "about fifty places"
counts sites, not lines):

| File | Functions that name a driver | What they do |
|---|---|---|
| `main.go` | `newDriver`; `validateGuestOS`; `newRequest` (through `isVMDriver`); `resolveNetwork`; the `--driver` and `--network` help | Construct each driver and set its `Verbose` (and the container driver's `Runtime` and `Version`); allow `--guest-os windows` for `qemu` and `hyperv` only; give `[vm] network` to `hyperv` only |
| `config.go` | `validateDriver`; `isVMDriver`; `validateFlagsForDriver`; `resolveBuildInput`; `linkRanges`; `DetectRuntime`, `runtimeStartHint` | The list of valid names; which drivers take the VM flags; `--network` for `hyperv` only, with a firecracker-specific fix; Dockerfile discovery for the container driver against script-or-Dockerfile for the rest; the build link's disjointness from `driver.ContainerBaseCIDR`, `firecracker.UpstreamCIDR` and `qemu.UserNetCIDR`; the container runtime probe |
| `host.go` | `doctorCmd`; `setupCmd`; `hypervCmd`, `hypervServiceCmd` and its three subcommands; `launcherStamps`; `hypervNetwork`; `driverName`; the `--network`, `--nat-prefix`, `--host-ip` and `--slots` flags | Doctor is a five-way switch (`hyperv.Detect` and `hyperv.Doctor`; `firecracker.Doctor`; `qemu.Preflight` and `qemu.Resolvers`; `vz.Preflight` and `vz.Resolvers`; `resolveRuntime` for the container driver). Setup calls `firecracker.RunSetup` or `hyperv.RunSetup` and prints a line for the others. `warden hyperv service install|uninstall|run` calls `hyperv.InstallService`, `UninstallService`, `RunService` |
| `image.go` | `defaultImageOS`; `runImagePrepare`; `checkImageUpdate`; `prepareVZMacos`; `prepareQemuWindows`; `prepareHyperVWindows`; `hypervImageSource`; `runImageList`; `formatVZImages`; ten `--` flags | Prepare is a switch on `driver/os` whose three largest arms (vz, qemu Windows, hyperv) are driver logic living in `cmd/warden`; list prints vz, qemu, firecracker and hyperv lines, each its own way |
| `image_verify.go` | `layerCaches`; `runImageVerify` | A map of four drivers' `LayerCache()` and a fixed order |
| `clean.go` | `runClean` | `firecracker.Clean`, `hyperv.Clean`, and otherwise the container runtime's cleanup, which is written in `cmd/warden` itself (`listResources`, `findLiveWardenProcesses`, `isOrphan`) |

`cmd/warden/main_darwin_arm64.go` pins the main thread for vz. The tests
`image_vz_test.go` and `image_hyperv_test.go` import drivers too.

Outside `cmd/warden`, shared packages name drivers as well:

- `driver.BuildRequest` is a union. `Network` is hyperv's; `RelayImage` is
  the container driver's; `ImageRefreshAfter` and `ImageMinGrace` are read by
  qemu (`ensureFreshWindowsLayer`) and hyperv (`ensureFreshBase`) only;
  `Image`, `GuestOS`, `CPUs`, `MemoryMB`, `KeepVMs`, `Script`, `BuildLink`
  and `DNS` are set for the four VM drivers only.
- `driver/relayvm/assets.go` holds `QEMURelay`, `Firecracker` and
  `QEMUBuild`, each driver's boot asset list.
- `driver/subnet.go` holds `ContainerBaseCIDR` and `AllocateNetwork`, which
  only the container driver uses.
- `driver/buildlink/disjoint_test.go` imports `firecracker`, `hyperv` and
  `qemu`; `driver/claimlink_test.go` lists `qemu`, `firecracker` and `vz`.
- Configuration (`cmd/warden/config.go`): `[runtime] cli` and
  `[runtime] relay_image` are the container driver's; `[vm] network`,
  `windows_image` and `windows_image_update` are hyperv's;
  `[vm] image_refresh_after` and `image_min_grace` are qemu's and hyperv's.
  Each is a field of `Config` that `cmd/warden` routes by driver name.

There is no relay source. Each driver finds its relay its own way (table
under **The relay source**), nothing says which release form a driver runs,
and none checks a hash. M6 ("adopt relay version X", one item per driver)
needs one place per driver where that is stated and changed.

So an integrator who wants a driver must fork `cmd/warden`, and the Rust
relay has nowhere to be adopted. GLOBAL-08 says the first on purpose:
`driver.Driver` is "not covered by this obligation". The umbrella's Decision 4
reverses that, and K1 is its first step: the contract complete in-tree.

## Prior art

- [`componentization`](componentization.md): the umbrella. Seam 9, phase K1,
  Decisions 4, 6, 7, 18 and a2 bound this design; its Consolidation records
  what #115 changed, which **Design** describes as `main` has it.
- [`firecracker-driver`](firecracker-driver.md) added the fifth driver by
  editing every file in the table above, and moved the qemu relay-VM and
  Linux-guest code to `driver/relayvm` and `driver/linuxguest`. Its Decision
  4 (durable slots made by `warden setup`) is why setup and doctor are part
  of the contract. Reused: the two kit packages; `Slot`, `SetupOptions`.
- [`hyperv-driver-plan`](hyperv-driver-plan.md): the `Provisioner` boundary
  and the privileged service, the one driver-specific command group
  (`warden hyperv service`). Reused as the model for driver-owned commands.
- [`build-link-subnet`](build-link-subnet.md) and
  [`hyperv-relay-per-build-channel`](hyperv-relay-per-build-channel.md):
  `BuildRequest.BuildLink` and `DNS`, `driver.LauncherStamp`,
  `driver/resolvers`, and RELAY-08. The contract keeps these as the kit's
  shared VM request fields and helpers; the relay source adds no relay
  setting (RELAY-08).
- [`unified-bootstrap`](unified-bootstrap.md): the bootstrap model. K1
  changes none of it; the executor delivery (`driver.GuestBinary("warden-io",
  ...)`) is untouched, and BOOT-02's wording changes in K2, not here.
- [`tier1-static-checks`](tier1-static-checks.md): `drivers-claim`,
  `surface-docs`, `surface-removed` and `seam-contract`. K1 changes the two
  surface checks' input (Rule changes) and gives `seam-contract` an entry.
- [`design-approved-rule-changes`](design-approved-rule-changes.md): the
  route for this design's four rule changes (FLOW-14).
- [`vm-driver-release-plan`](vm-driver-release-plan.md) (relay VM
  distribution), [`guest-binary-cache-per-tree`](guest-binary-cache-per-tree.md)
  (`driver.GuestBinary`'s lookup): the relay lookups the relay source wraps,
  unchanged.
- [`ledger-environment-types`](ledger-environment-types.md) (`LayerCache`,
  `image verify`), [`hyperv-windows-prepare`](hyperv-windows-prepare.md),
  [`vz-image-prepare-refresh`](vz-image-prepare-refresh.md),
  [`hyperv-clean`](hyperv-clean.md): the image and clean behaviour that moves
  behind the contract unchanged.
- [`platform-compatibility`](platform-compatibility.md): which driver runs on
  which host; `Info` does not restate it, each driver's stub does (GLOBAL-07).
- [`design-principles`](design-principles.md): GLOBAL-09 to GLOBAL-11, applied
  under **Defaults and overrides**.

Every design relied on already has front-matter (FLOW-12).

## Decisions

1. **One interface with the build and doctor, and optional interfaces for the
   rest.** `driver.Driver` keeps `StartBuild`, `Exec` and `Close`, replaces
   `Name` with `Info`, and gains `Doctor`. Setup, images, clean and the
   driver's own commands are optional interfaces (`HostSetup`, `Images`,
   `Cleaner`, `Commander`) found by type assertion. Alternatives: one wide
   interface with every operation mandatory and a `Base` to embed (an
   integrator's minimal driver would carry six no-op methods, and "this
   driver has no setup" would be a convention, not a type); only optional
   interfaces, doctor included (every driver has a doctor today, UX-04 sends
   slow checks there, and K2's dispatcher needs one question it can ask any
   driver).
2. **Drivers are linked by an explicit list, not a global registry.**
   `cmd/warden/drivers.go` holds the one list of in-tree drivers and the
   default driver's name, and is the only Go file outside `driver/<name>/`
   that names a driver. Alternatives: `driver.Register` called from each
   driver's `init`, linked by blank imports (global state and init order for
   something K2 deletes, and the blank-import list still names every driver);
   a generated import file (a build step to keep one line current); one
   binary per driver now (that is K2). K2's driver binary is the same frame
   given a list of one (a1).
3. **A driver declares its options as data, and the frame parses them.** Each
   flag and config key a driver owns is a `driver.Option` in its `Info`. The
   frame registers the union of the linked drivers' flags, resolves each
   option with the existing precedence (ORCH-03), and hands the selected
   driver a `driver.Values`. Alternatives: hand the driver a `*pflag.FlagSet`
   and the raw TOML (cobra and the TOML library become part of the public
   contract, and tier 1 cannot read the surface); keep the flags in
   `cmd/warden` and pass a map (a new driver's flag edits `cmd/warden`).
4. **`Info.VM` is the one capability switch for the shared VM fields.** The
   VM drivers take `--script`, `--image`, `--cpus`, `--memory`, `--keep-vms`,
   `--build-link`, `[vm] cpus`, `memory_mb`, `build_link` and `dns` as a
   group today, and the user-facing text says "VM drivers". Alternative: a
   capability per field (no driver differs, and every refusal message would
   change). It can be added later without breaking a driver (a9).
5. **Driver-owned configuration: existing keys stay where they are; a new
   driver's keys go in `[driver.<name>]`.** UX-03 forbids moving the seven
   existing keys without aliases, and K1 changes no key. Alternatives: move
   them all now with deprecated aliases (a user-facing change in a freeze,
   for no gain before K2); let a driver declare any table (collisions with
   the core tables) (a4).
6. **The relay source is a kit type each driver resolves its relay through;
   K1 adds no user-facing relay override.** `driver.Relay` names a release
   form (seam 8's vocabulary), a version, an origin and files with optional
   hashes; `driver/relaysource` wraps today's lookups. The two existing
   overrides (`[runtime] relay_image` with `WARDEN_IMAGE`, and
   `WARDEN_RELAY_BOOT_VHDX`) are routed through it unchanged. Alternatives:
   add `--relay` and `[driver.<name>] relay` to every driver now (it would
   fix an on-disk layout for forms seam 8 has not written, and no driver can
   boot a form it has no code for); build the pinned-download source now
   (needs seam 8's manifest and hash format) (a3).
7. **The claim stays as it is; S4 owns the claim connection.** K1 does not
   change `Session.ClaimRelay`. The relay source ends where the relay is
   started, unconfigured; how it is claimed is S4's (a2).
8. **The contract's types name no CLI library.** A driver's own commands are
   a `driver.Command` tree, not `*cobra.Command`. Alternative: expose cobra
   (drivers released separately in K3 and K4 would have to agree on a cobra
   version with the dispatcher).
9. **`drivertest` has two suites.** `Contract` runs anywhere with no
   hypervisor and is run by `make test` for every linked driver; `Lifecycle`
   runs real builds on a host that can run the driver. Isolation is not
   `drivertest`'s: that is S3's build-link suite. Alternative: one suite
   (either CI could not run it, or it would prove nothing about a build).
10. **Dropped fallback: `warden clean` for a driver with no cleaner no longer
    runs the container runtime's cleanup.** Today `--driver qemu` and
    `--driver vz` fall through to it. Keeping it would make the frame name
    the container driver (a5, GLOBAL-11).
11. **One queue item and one PR, converted driver by driver in ordered
    commits.** Alternative: an item per driver (five designs, plans and
    reviews for one mechanical change, and the frame cannot lose its adapter
    until the last lands) (a6).
12. **The kit's public, unstable packages** after K1 are `driver` (the
    contract, `Session`, the host helpers), `driver/drivertest`,
    `driver/relaysource`, `driver/buildlink`, `driver/resolvers`,
    `driver/script`, `driver/relayvm`, and the guest families
    `driver/linuxguest` and `driver/winguest` (umbrella Decision 6). K1 moves
    none of them; K2's tier-1 check is what limits a driver to them.

## Questions

### Blocking

None.

### Assumed

- **a1. What "adding a driver touches only its own directory" means.**
  Proceeding with: no Go file outside `driver/<name>/` changes, except one
  line in `cmd/warden/drivers.go`, which K2 removes when `warden` becomes the
  dispatcher. Outside Go, a new in-tree driver still edits the reference docs
  (UX-01), `drivers.all` and `drivers.dirs` in `tools/tier1/rules.yaml`,
  `bootstrap.md`'s mapping (BOOT-07), CI and release packaging. Options: (a)
  the explicit list, as stated; (b) a generated import file; (c) a global
  registry with blank imports; (d) one binary per driver now. Recommended:
  (a). Confirm.
- **a2. The boundary with S4.** The umbrella lists "the claim connection"
  inside seam 9 and also gives it its own phase, S4, in the same wave, and
  leaves their order open. Proceeding with: K1 does not change
  `driver.Session`'s claim API (`ClaimRelay` keeps its address and token);
  S4 designs the connection type, changes `ClaimRelay` and every driver's
  claim call, and adds "the claim connection" to GLOBAL-08's seam list; the
  two are independent, neither waits, and whichever publishes second
  reconciles each driver's relay-start function in its Consolidation
  (FLOW-10). Options: (a) as stated; (b) K1 waits for S4; (c) S4 waits for
  K1; (d) K1 designs the connection too and S4 is folded in. Recommended:
  (a). Confirm.
- **a3. The relay source's scope in K1.** Proceeding with: K1 ships the
  types, the bundled, source-tree, local-path and image sources that wrap
  today's lookups, and routes the two existing overrides through them; it
  adds no flag or key. When a driver first needs a user override it is one
  kit option, `--relay` and `[driver.<name>] relay`, used exclusively. A
  pinned source (download a release form by version and verify its published
  hash) is one more `RelaySource`, built once seam 8's manifest and hash
  format are written. Options: (a) as stated; (b) also add `--relay` and
  `[driver.<name>] relay` for the four VM drivers now; (c) also build the
  pinned-download source now, unused until M6. Recommended: (a). Confirm.
- **a4. Where a driver's own configuration lives.** Proceeding with: the
  seven existing driver-owned keys keep their tables and names; a key a driver
  adds from now on lives in `[driver.<name>]`; `drivertest.Contract` refuses
  any other table. No such key exists in K1, so no user sees the table yet.
  Options: (a) as stated; (b) move the existing keys to `[driver.<name>]`
  now, with deprecated aliases; (c) let a driver declare any table.
  Recommended: (a). Confirm.
- **a5. The behaviour that changes.** Proceeding with the six differences
  listed under **Behaviour**: a flag the selected driver does not take is
  refused where five flags were silently ignored; `warden clean --driver
  qemu|vz` no longer cleans containers; `image list` and three messages name
  drivers in list order; `image prepare --help` groups each driver's text.
  Options: (a) accept all six; (b) reproduce today's behaviour exactly, with
  a per-option "ignored elsewhere" bit, a container fallback in `clean` and
  fixed orders in the frame; (c) accept some, named in the review.
  Recommended: (a). Confirm.
- **a6. One item or several.** Proceeding with one queue item and one PR,
  with the commits in **Order of the refactor** so each driver is converted
  on its own and every commit is green. Options: (a) as stated; (b) the
  contract, frame and container driver first, then one item per VM driver,
  then an item that removes the adapter and makes the rule changes.
  Recommended: (a). Confirm.
- **a7. Native validation.** Proceeding with macos-vz, windows-hyperv,
  qemu-any and macos-finch blocking, macos-qemu tracked, and firecracker
  validated by the `firecracker` CI workflow (tier 2 on a hosted KVM runner)
  with no native step. Options: (a) as stated; (b) macos-qemu blocking too;
  (c) macos-finch tracked, relying on the `integration` workflow's Docker
  run; (d) add a linux-kvm step. Recommended: (a). Confirm.
- **a8. Rule changes beyond the umbrella's table.** The umbrella lists
  GLOBAL-08 and ORCH-01 for K1 "for direction". Proceeding with those two
  plus an enforcement-only change to UX-01 and UX-03: tier 1's surface
  extraction reads driver-declared options as well as `cmd/warden`, because
  flags and keys that move into `Info.Options` would otherwise read as
  removed and stop being checked against the docs. Options: (a) as stated;
  (b) no enforcement change, a `Tier1-Allow` trailer on the move, and the
  moved flags no longer checked; (c) keep every flag literal in `cmd/warden`.
  Recommended: (a). Each rule change is approved by name in the review.
- **a9. `Info.VM` as the capability model.** Proceeding with one switch for
  the shared VM fields (Decision 4). Options: (a) as stated; (b) one
  capability per field. Recommended: (a). Confirm.

## Design

(Written in the next commit.)

## Defaults and overrides

(Written in the next commit.)

## Cross-driver impact

(Written in the next commit.)

## Rule changes

(Written in the next commit.)

## Security note

(Written in the next commit.)

## Validation plan

(Written in the next commit.)

## Implementation plan

(Written in the next commit.)

## Design review

None yet.

## Consolidation

## Rollback notes

## Archive summary

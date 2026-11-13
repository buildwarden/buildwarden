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
  - {host: macos-qemu, blocking: false, reason: "the qemu Windows prepare moves from cmd/warden into driver/qemu; a run needs an install ISO and hours, and the host's open backlog entry (qemu-windows-guest-package) shows it is not yet reliable there"}
tier2_gaps:
  - {host: macos-qemu, gap: "warden image prepare --driver qemu --os windows, then a Windows build, through the contract's PrepareImage (backlog driver-contract-qemu-windows)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10,
        GLOBAL-11, ORCH-01, ORCH-02, ORCH-03, ORCH-04, ORCH-05, RELAY-04, RELAY-07, RELAY-08,
        BOOT-02, BOOT-04, BOOT-06, BOOT-07, UX-01, UX-03, UX-04, UX-05, UX-07, UX-08, UX-09,
        TEST-01, TEST-02, TEST-03, TEST-04, TEST-07, TEST-08, LINT-05,
        FLOW-01, FLOW-02, FLOW-05, FLOW-07, FLOW-09, FLOW-10, FLOW-12, FLOW-14, FLOW-15, FLOW-17,
        FLOW-19, FLOW-20]
rule_changes:
  - {rule: GLOBAL-08, change: amend, paths: [docs/development/global.md, tools/tier1/rules.yaml]}
  - {rule: ORCH-01, change: amend, paths: [docs/development/orchestrator.md, tools/tier1/rules.yaml]}
  - {rule: UX-01, change: amend, paths: [tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt]}
  - {rule: UX-03, change: amend, paths: [tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt]}
  - {rule: ORCH-03, change: amend, paths: [tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt]}
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
| `main.go` | `newDriver`; `validateGuestOS`; `newRequest` (through `isVMDriver`); `resolveNetwork`; `runShell`; the `--driver` and `--network` help | Construct each driver and set its `Verbose` (and the container driver's `Runtime` and `Version`); allow `--guest-os windows` for `qemu` and `hyperv` only; give `[vm] network` to `hyperv` only; parse `[vm] image_refresh_after` and `image_min_grace` for every driver although only qemu and hyperv read them; answer a driver without `Exec` with a fix that names `--driver container` |
| `config.go` | `validateDriver`; `isVMDriver`; `validateFlagsForDriver`; `resolveBuildInput`; `linkRanges`; `DetectRuntime`, `runtimeStartHint` | The list of valid names; which drivers take the VM flags; `--network` for `hyperv` only, with a firecracker-specific fix; Dockerfile discovery for the container driver against script-or-Dockerfile for the rest; the build link's disjointness from `driver.ContainerBaseCIDR`, `firecracker.UpstreamCIDR` and `qemu.UserNetCIDR`; the container runtime probe |
| `host.go` | `doctorCmd`; `setupCmd`; `hypervCmd`, `hypervServiceCmd` and its three subcommands; `launcherStamps`; `hypervNetwork`; `driverName`; the `--network`, `--nat-prefix`, `--host-ip` and `--slots` flags | Doctor is a five-way switch (`hyperv.Detect` and `hyperv.Doctor`; `firecracker.Doctor`; `qemu.Preflight` and `qemu.Resolvers`; `vz.Preflight` and `vz.Resolvers`; `resolveRuntime` for the container driver). Setup calls `firecracker.RunSetup` or `hyperv.RunSetup` and prints a line for the others. `warden hyperv service install|uninstall|run` calls `hyperv.InstallService`, `UninstallService`, `RunService` |
| `image.go` | `defaultImageOS`; `runImagePrepare`; `checkImageUpdate`; `prepareVZMacos`; `prepareQemuWindows`; `prepareHyperVWindows`; `hypervImageSource`; `runImageList`; `formatVZImages`; ten `--` flags | Prepare is a switch on `driver/os` whose three largest arms (vz, qemu Windows, hyperv) are driver logic living in `cmd/warden`; list prints vz, qemu, firecracker and hyperv lines, each its own way |
| `image_verify.go` | `layerCaches`; `runImageVerify` | A map of four drivers' `LayerCache()` and a fixed order |
| `clean.go` | `runClean` | `firecracker.Clean`, `hyperv.Clean`, and otherwise the container runtime's cleanup, which is written in `cmd/warden` itself (`listResources`, `findLiveWardenProcesses`, `isOrphan`) |

`cmd/warden/main_darwin_arm64.go` pins the main thread for vz. The tests
`image_vz_test.go` and `image_hyperv_test.go` import drivers too. Besides the
code, the long help of `doctor`, `image prepare` and `clean` describes
drivers by name in prose.

Outside `cmd/warden`, shared packages name drivers as well:

- `driver.BuildRequest` is a union. `Network` is hyperv's; `RelayImage` is
  the container driver's; `ImageRefreshAfter` and `ImageMinGrace` are read by
  qemu (`ensureFreshWindowsLayer`) and hyperv (`ensureFreshBase`) only;
  `Image`, `GuestOS`, `CPUs`, `MemoryMB`, `KeepVMs`, `Script`, `BuildLink`
  and `DNS` mean something to the four VM drivers only.
- `driver/relayvm/assets.go` holds `QEMURelay`, `Firecracker` and
  `QEMUBuild`, each driver's boot asset list.
- `driver/subnet.go` holds `ContainerBaseCIDR` and `AllocateNetwork`, which
  only the container driver uses.
- `driver/buildlink/disjoint_test.go` imports `firecracker`, `hyperv` and
  `qemu`; `driver/claimlink_test.go` lists `qemu`, `firecracker` and `vz`.
- `tools/vz-image-prep/personalize.go` and `uv-build-test.go` import
  `driver/vz` (`NewMacOSVM`, `MacOSVMConfig`, `NewVirtualNetwork`, the VM
  states). They are standalone programs (`//go:build ignore`), not part of
  `warden`.
- Drivers name each other in the fix text of five messages: one in qemu,
  one in vz, two in firecracker, and the container runtime probe's `use a VM
  driver (--driver qemu or vz)`. That is prose, and it stays.
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
  route for this design's five rule changes (FLOW-14).
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
   default driver's name, and is the only Go file of `warden` and the kit
   outside `driver/<name>/` that imports a driver's package. Alternatives:
   `driver.Register` called from each driver's `init`, linked by blank
   imports (global state and init order for something K2 deletes, and the
   blank-import list still names every driver);
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
   form, a version, an origin and files with optional hashes;
   `driver/relaysource` wraps today's lookups. The form and role names are
   K1's own and provisional: seam 8, whose vocabulary they should be, is not
   written and the umbrella gives no phase the job (a11). The two existing
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
   (a third-party library's types would be part of a seam the project
   supports, so a cobra upgrade or a move off cobra would be a change to the
   contract).
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
12. **K1 adds two kit packages and moves none.** It adds
    `driver/drivertest` and `driver/relaysource` beside the packages drivers
    share today: `driver` (the contract, `Session`, the host helpers),
    `driver/buildlink`, `driver/resolvers`, `driver/script`, `driver/relayvm`,
    and the guest families `driver/linuxguest` and `driver/winguest`
    (umbrella Decision 6). Which of these the kit keeps public is K2's to
    settle, with its tier-1 check that limits a driver to them; `relayvm` in
    particular is specific to the Go relay's appliance. Alternative: fix the
    list here (it would pre-decide K2 with nothing in K1 depending on it).

## Questions

### Blocking

None.

### Assumed

- **a1. What "adding a driver touches only its own directory" means.**
  Proceeding with: no Go file outside `driver/<name>/` changes, except one
  line in `cmd/warden/drivers.go`, which K2 removes when `warden` becomes the
  dispatcher. Until then a driver from outside this repository is linked by
  patching that list. Outside Go, a new in-tree driver still edits the
  reference docs (UX-01), `drivers.all` and `drivers.dirs` in
  `tools/tier1/rules.yaml`, `bootstrap.md`'s mapping (BOOT-07), CI and
  release packaging. Options: (a) the explicit list, as stated; (b) a
  generated import file; (c) a global registry with blank imports; (d) one
  binary per driver now. Recommended: (a). Confirm.
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
  format are written (a11). Options: (a) as stated; (b) also add `--relay` and
  `[driver.<name>] relay` for the four VM drivers now; (c) also build the
  pinned-download source now, unused until M6. Recommended: (a). Confirm.
- **a4. Where a driver's own configuration lives.** Proceeding with: the
  seven existing driver-owned keys keep their tables and names; a key a driver
  adds from now on lives in `[driver.<name>]`; `drivertest.Contract` refuses
  any other table. No such key exists in K1, so no user sees the table yet.
  Options: (a) as stated; (b) move the existing keys to `[driver.<name>]`
  now, with deprecated aliases; (c) let a driver declare any table.
  Recommended: (a). Confirm.
- **a5. The behaviour that changes.** Proceeding with the seven differences
  listed under **Behaviour**: a flag the selected driver does not take is
  refused, where eleven flags were silently ignored; the existing refusals
  take one generated form; `warden clean --driver qemu|vz` no longer cleans
  containers; `image list` and two messages name drivers in list order; the
  long help of `image prepare` and `clean` groups each driver's text; a
  malformed `[vm] image_refresh_after` or `image_min_grace` is refused on
  every command that loads the configuration, where it was refused on
  `build` and `shell` only. Three help passages keep naming drivers in prose
  as frame text until K2. Options: (a) accept all seven; (b) reproduce
  today's behaviour exactly, with a per-option "ignored elsewhere" bit, a
  container fallback in `clean`, fixed orders in the frame and the age check
  on `build` and `shell` only; (c) accept some, named in the review.
  Recommended: (a). Confirm.
- **a6. One item or several.** Proceeding with one queue item and one PR,
  with the commits in **Order of the refactor** so each driver is converted
  on its own and every commit is green. Options: (a) as stated; (b) the
  contract, frame and container driver first, then one item per VM driver,
  then an item that removes the adapter and makes the rule changes.
  Recommended: (a). Confirm.
- **a7. Native validation.** Proceeding with macos-vz, windows-hyperv,
  qemu-any and macos-finch blocking, macos-qemu tracked, and two hosts left
  out. linux-kvm: firecracker is validated by the `firecracker` CI workflow
  (tier 2 on a hosted KVM runner), which this change triggers and which
  passed on its eight most recent runs (to 2026-11-12). That workflow is
  not a required check and TEST-02's list of where tiers run does not name
  it (`firecracker-driver`, Consolidation), so Publish confirms its run on
  the PR's head passed. container-any (the Windows desktop): the container
  driver is covered by the `integration` workflow on Linux and natively on
  macos-finch; its cleanup on a Windows host (`ps aux`) is covered by
  neither today, and K1 moves that code without changing it. Options: (a) as
  stated; (b) macos-qemu blocking too; (c) macos-finch tracked, relying on
  the `integration` workflow's Docker run; (d) add a blocking linux-kvm
  step; (e) add a container-any step. Recommended: (a). Confirm.
- **a8. Rule changes beyond the umbrella's table.** The umbrella lists
  GLOBAL-08 and ORCH-01 for K1 "for direction". Proceeding with those two
  plus an enforcement-only change to UX-01, UX-03 and ORCH-03, the three
  rules tier 1's surface check enforces: its extraction reads
  driver-declared options as well as `cmd/warden`, because flags and keys
  that move into `Info.Options` would otherwise read as removed and stop
  being checked against the docs. `rules.yaml`'s format does not change, so
  `main`'s tier 1 still runs against the branch (FLOW-14). Options: (a) as
  stated; (b) no enforcement change, a `Tier1-Allow` trailer on the move,
  and the moved flags no longer checked; (c) keep every flag literal in
  `cmd/warden`. Recommended: (a). Each rule change is approved by name in
  the review.
- **a9. `Info.VM` as the capability model.** Proceeding with one switch for
  the shared VM fields (Decision 4). Options: (a) as stated; (b) one
  capability per field. Recommended: (a). Confirm.
- **a10. How far K1's GLOBAL-08 wording goes.** The umbrella's table gives
  K1 "the driver contract becomes a supported seam, unstable before 1.0",
  and its Decision 18 has a rule change in the item that makes it true.
  After K1 the contract is documented and has a conformance suite, but a
  driver is still linked by a line in `cmd/warden/drivers.go` and there is
  no kit module to version. Proceeding with: K1's wording says the driver
  contract is a supported seam, unstable before 1.0, with its contract
  document and `drivertest`; the sentences "a driver the project did not
  write is a supported consumer" and "its version is the kit module's" are
  left for K2's amendment, when a driver is its own binary and the kit its
  own module. Options: (a) as stated; (b) add both sentences now, ahead of
  the code. Recommended: (a). Confirm.
- **a11. Seam 8 (relay release forms) has no phase that writes it.** The
  umbrella lists seam 8 among the seams that get a specification, and says
  M6 needs K1's relay source; but S1 writes seams 1, 2, 3, 5 and 6, S3 writes
  seam 7, seam 4 moves, and no phase is given seam 8. K1 needs its
  vocabulary now (the form names a driver declares and the file roles of a
  form) and would need its manifest and hash format for a pinned source.
  Proceeding with: K1 defines the four form names and four file roles as Go
  constants of the unstable contract, marked provisional in the contract
  document; it does not write seam 8, records no form in the ledger, and
  builds no pinned source. Who writes seam 8, and when, is the owner's to
  assign; it has to exist before M6 adopts a release. Options: (a) as
  stated, with seam 8 filed as its own S-track item; (b) seam 8's writing
  is added to S1's item; (c) K1 writes a first seam 8 text for the names it
  uses; (d) M6's first item writes it. Recommended: (a). This is a gap in the umbrella, raised here, not settled.

## Design

### Shape

```
cmd/warden/                         the frame: commands, config, flags, output
  drivers.go                        the list of in-tree drivers and the default
                                    driver's name; the only file that imports one
driver/                             the kit
  contract.go  options.go  relay.go    the driver contract (seam 9)
  session.go, hostutil.go, ...         shared lifecycle and host helpers, as today
  drivertest/                          the contract's conformance suites
  relaysource/                         where a relay comes from
  buildlink/ resolvers/ script/ relayvm/ linuxguest/ winguest/   as today
  container/ qemu/ vz/ hyperv/ firecracker/                      the drivers
```

The frame is everything `cmd/warden` does that is not a driver's: the cobra
commands, config loading, flag and option resolution, the build request,
`inspect`, and reporting. It works on a `[]driver.Driver`. In K1 it stays in
package `main` of `cmd/warden`; K2 moves it into the kit so each driver's own
binary is the same frame over a list of one (not designed here).

```go
// cmd/warden/drivers.go
var drivers = []driver.Driver{
	container.New(), qemu.New(), vz.New(), hyperv.New(), firecracker.New(),
}

const defaultDriver = "container"
```

The order is today's order in `validateDriver` and the `--driver` help. Every
message and help line that lists drivers is generated from this list.

A driver's constructor does no host work, like `Info`: the list is built
when the program starts, on every host, and a driver resolves its cache and
other directories when a method is called. `vz.New` changes for this (it
looks up the cache directory today).

### The contract

In `driver/contract.go`. `Name` becomes `Info().Name`; `StartBuild`, `Exec`
and `Close` keep their signatures and meaning.

```go
// Driver is what an orchestration backend implements.
type Driver interface {
	// Info describes the driver. It does no host work and returns the same
	// value on every call.
	Info() Info
	// StartBuild runs one build to completion and tears it down.
	StartBuild(ctx context.Context, req *BuildRequest) (*BuildResult, error)
	// Exec opens an interactive session in a build environment, or returns
	// ErrExecNotSupported.
	Exec(ctx context.Context, req *BuildRequest) error
	// Doctor reports, changing nothing on the host, whether this host can run
	// the driver's builds. It writes its findings to req.Out. blocked is true
	// when a build would be refused. err is a check that could not run, or a
	// FixableError the frame prints as one (UX-04).
	Doctor(ctx context.Context, req *DoctorRequest) (blocked bool, err error)
	// Close releases what the driver holds.
	Close() error
}

// HostSetup is implemented by a driver that needs one-time host preparation
// (warden setup).
type HostSetup interface {
	Setup(ctx context.Context, req *SetupRequest) error
}

// Images is implemented by a driver that keeps base images (warden image).
type Images interface {
	// PrepareImage acquires, verifies and readies a base image and returns
	// its path, or "" when it has already reported the outcome itself.
	PrepareImage(ctx context.Context, req *PrepareRequest) (path string, err error)
	// ListImages writes one line per prepared image and returns how many.
	ListImages(w io.Writer) (lines int, err error)
	// LayerCache is the driver's base-image hash cache on this host, or nil
	// when it keeps none here (warden image verify).
	LayerCache() *LayerCache
}

// Cleaner is implemented by a driver whose interrupted builds can leave
// resources behind (warden clean). It removes only what no running build
// holds (ORCH-04) and returns how many resources it removed.
type Cleaner interface {
	Clean(ctx context.Context, req *CleanRequest) (removed int, err error)
}

// Commander is implemented by a driver with commands of its own, mounted at
// `warden <name> ...`.
type Commander interface {
	Command() *Command
}

// Info is what a driver declares about itself.
type Info struct {
	// Name is what --driver, WARDEN_DRIVER and [runtime] driver select.
	Name string
	// VM is true for a driver that boots a separate build machine. It takes
	// the shared VM request fields (Script, Image, CPUs, MemoryMB, KeepVMs,
	// BuildLink, DNS) and builds a script or a Dockerfile; a driver without
	// it builds a Dockerfile and is refused those flags.
	VM bool
	// Shell is true when Exec opens a session (warden shell); without it Exec
	// returns ErrExecNotSupported.
	Shell bool
	// GuestOS lists the --guest-os values the driver's builds accept besides
	// the default, "linux".
	GuestOS []string
	// ImageOS lists the --os values PrepareImage takes; the first is the
	// default. Empty without Images.
	ImageOS []string
	// Options are the flags and config keys the driver owns.
	Options []Option
	// ReservedRanges are fixed address ranges the driver uses on the host; a
	// build link must not overlap any linked driver's.
	ReservedRanges []buildlink.Range
	// RelayForms are the relay release forms the driver can run; the first is
	// the one its default relay source resolves.
	RelayForms []RelayForm
	// Help is the driver's paragraph in a command's long help, and Examples
	// its example lines, keyed by command.
	Help     map[CommandName]string
	Examples map[CommandName][]string
}

// CommandName names a frame command that takes driver options.
type CommandName string

const (
	CmdBuild        CommandName = "build"
	CmdShell        CommandName = "shell"
	CmdDoctor       CommandName = "doctor"
	CmdSetup        CommandName = "setup"
	CmdImagePrepare CommandName = "image prepare"
	CmdClean        CommandName = "clean"
	// AllCommands, in Option.Flag, makes the flag global.
	AllCommands CommandName = "*"
)

// Command is a driver's own command or command group. It names no CLI
// library, so a driver and the frame need not share one.
type Command struct {
	Name, Short, Long string
	// NoArgs refuses positional arguments.
	NoArgs bool
	Sub    []*Command
	// Run is nil for a group.
	Run func(ctx context.Context, out io.Writer, args []string) error
}
```

Requests. `BuildRequest` loses its four driver-specific fields and gains
two; everything else in it stays, field for field.

```go
type BuildRequest struct {
	// Unchanged: Script, ContextDir, Image, GuestOS, Containerfile, OutputDir,
	// CaptureMode, Compress, Quiet, Timeout, CPUs, MemoryMB, KeepVMs, TempDir,
	// UpstreamCACerts, UpstreamSystemCA, BuildLink, DNS, Env, Stdin, Stdout,
	// Stderr. Removed: Network, ImageRefreshAfter, ImageMinGrace, RelayImage
	// (each is now an option of the driver that reads it).

	// Options are the driver's own settings, resolved by the frame.
	Options Values
	// Verbose is --verbose (today a field each driver's struct carries).
	Verbose bool
}

type DoctorRequest struct {
	Out io.Writer
	// BuildLink and its source ("--build-link", the config file's name, or
	// "default"), and DNS ([vm] dns), for a VM driver.
	BuildLink       buildlink.Link
	BuildLinkSource string
	DNS             []netip.Addr
	Options         Values
	Verbose         bool
}

type SetupRequest struct {
	Out       io.Writer
	BuildLink buildlink.Link // for a VM driver
	Options   Values
	Verbose   bool
}

type PrepareRequest struct {
	OS, Source string    // --os (defaulted from Info.ImageOS) and [source] / --source
	Progress   io.Writer // os.Stderr in warden
	Options    Values
	Verbose    bool
}

type CleanRequest struct {
	Out     io.Writer
	TempDir string // where builds' work dirs are made; "" is the OS temp dir
	Options Values
}
```

`driver.WardenVersion` (set by the frame, as today) replaces the container
driver's `Version` field. `ErrExecNotSupported`, `ExitError`, `FixableError`,
`Session`, `LauncherStamp`, `LayerCache` and the host helpers are unchanged.

### Options: declaring, parsing, precedence

In `driver/options.go`.

```go
// Option is one setting a driver owns: a flag, a config key, or both.
type Option struct {
	// Name identifies the option in Values and is the flag's name.
	Name string
	Kind OptionKind // OptString, OptInt, OptBool, OptAge ("7d", "36h")
	// Flag lists the commands that take --Name; empty means no flag.
	Flag []CommandName
	// Config is the key in warden.toml and the user config, "table.key";
	// empty means no key.
	Config string
	// Env is an environment variable that sets it. Only the variables that
	// exist today may be declared: UX-07 gives new settings a flag and a key.
	Env string
	// Usage is the flag's help; UsageOn overrides it for one command.
	Usage   string
	UsageOn map[CommandName]string
}

// Values are a driver's options as the frame resolved them.
type Values struct{ /* unexported */ }

func (v Values) String(name string) string
func (v Values) Int(name string) int
func (v Values) Bool(name string) bool
func (v Values) Age(name string) time.Duration
// Source says where a value came from ("--network", "WARDEN_IMAGE",
// "warden.toml", "config.toml"), or "" when nobody set it. The getters
// return the zero value for an unset option, and the driver applies its own
// default, as each does today.
func (v Values) Source(name string) string
```

What the five drivers declare is exactly what `cmd/warden` holds for them
today. No name, table or variable changes. An `Option` is written as a
composite literal with constant strings, so tier 1 can read the surface
from source (Rule changes).

| Driver | Option | Flag on | Config key | Env |
|---|---|---|---|---|
| container | `runtime` | every command (global) | `runtime.cli` | `WARDEN_CTR_CLI` |
| container | `relay-image` | none | `runtime.relay_image` | `WARDEN_IMAGE` |
| qemu | `force`, `edition`, `virtio`, `arch`, `keep-vms` | `image prepare` | none | |
| qemu, hyperv | `image-refresh-after`, `image-min-grace` (ages) | none | `vm.image_refresh_after`, `vm.image_min_grace` | |
| vz | `update` | `image prepare` | none | |
| hyperv | `network` | `build`, `shell`, `doctor`, `setup` | `vm.network` | |
| hyperv | `nat-prefix`, `host-ip`, `slots` | `setup` | none | |
| hyperv | `sha256`, `force`, `name`, `keep-vms` | `image prepare` | none | |
| hyperv | `windows-image`, `windows-image-update` | none | `vm.windows_image`, `vm.windows_image_update` | |
| firecracker | `slots` | `setup` | none | |

The two age options are declared once, in `driver/winguest`
(`winguest.FreshnessOptions()`), since the freshness rule they feed
(`winguest.CheckFreshness`) lives there; qemu and hyperv both include them.

The frame keeps what belongs to no driver: `--driver`, `--verbose`,
`--color`; `--capture`, `--output`, `--no-compress`, `--timeout`, `--tmpdir`,
`--quiet`; the VM group `--script`, `--image`, `--guest-os`, `--cpus`,
`--memory`, `--keep-vms`, `--build-link`; `image prepare`'s `--os` and
`--source`; `clean --cache`; and the keys `runtime.driver`, `build.*`,
`relay.*`, `vm.cpus`, `vm.memory_mb`, `vm.build_link`, `vm.dns`, `output.*`.

How the frame handles options:

1. **Register.** For each command, the union of the linked drivers' options
   that name it becomes that command's flags. Two drivers may declare the
   same flag on the same command (`--slots` on `setup`, `--force` and
   `--keep-vms` on `image prepare`) when the kinds agree; the frame joins
   their usage strings with `"; "`, printing identical ones once. A clash is
   per command: the same flag name on the same command with another kind,
   or with a flag the frame itself defines on that command. `--keep-vms` is
   the frame's on `build` and `shell` and qemu's and hyperv's on `image
   prepare`, and that is not a clash. Registration returns the clash as an
   error, and a test in `cmd/warden` registers `drivers` and fails on one,
   so it is found by `make test`, not at start-up. Help output is therefore
   still the whole set, whichever driver is selected, as today.
2. **Check the configuration.** When a command loads the configuration,
   the value of every config key that any linked driver declares is checked
   against the option's kind, whichever driver is selected, and a value
   that does not fit is refused before any driver work (UX-08), naming the
   file and key. Today a wrong TOML type fails the load for every driver in
   the same way; the two ages are the one case that changes (Behaviour 7).
3. **Select the driver** from `--driver`, `WARDEN_DRIVER`, `[runtime]
   driver`, else `defaultDriver`; an unknown name is today's error. This
   applies to the commands that select one today: `build`, `shell`,
   `doctor`, `setup`, `image prepare` and `clean`. `image verify` keeps its
   own rule: the `--driver` flag only, and with no flag every linked driver
   that has a layer cache on this host. `image list`, `inspect` and a
   driver's own commands select none.
4. **Refuse** a flag that was set and that the selected driver does not
   declare for this command: `--network is only used by --driver hyperv`,
   with the fix `drop --network, or use --driver hyperv`. Config keys and
   environment variables are never refused for belonging to another driver:
   one `warden.toml` serves several. A VM-group flag on a driver without
   `Info.VM` keeps today's message. On a command that selects no driver, a
   global driver flag (`--runtime`) is accepted and unused, as today.
5. **Resolve** each of the selected driver's options: the flag when set, else
   its environment variable, else `warden.toml`, else
   `~/.config/warden/config.toml`, else unset. That is today's order
   (`LoadConfig` applies the environment over the files, and a flag over
   both), and the value found is used exclusively (GLOBAL-10). A flag value
   that does not parse as the option's kind is refused here, naming the
   flag.
6. **Call** the driver with the `Values`.

The frame reads config keys by decoding each file a second time into a
generic tree, so a key a driver declares needs no field in `Config`. The
driver-owned fields leave `Config` (`RuntimeConfig.CLI` and `RelayImage`,
`VMConfig.Network`, `ImageRefreshAfter`, `ImageMinGrace`, `WindowsImage`,
`WindowsImageUpdate`). Keys that neither the frame nor a linked driver
declares are still ignored, as today.

A key a driver adds after K1 is declared as `driver.<name>.<key>`, the
`[driver.<name>]` table (a4).

### What the frame derives from `Info`

| Today (`cmd/warden`) | After |
|---|---|
| `validateDriver`'s list; the `--driver` help | `Info.Name` of each linked driver, in list order |
| `newDriver` | lookup by name; `Verbose` goes in each request |
| `isVMDriver`; `validateFlagsForDriver`'s VM flags; `vmFlagOr`; build link and `[vm] dns` for VM drivers only; doctor's `--build-link` check | `Info.VM` |
| `resolveBuildInput`: `ResolvePath` for the container driver, `resolveVMBuildInput` otherwise | `Info.VM` |
| `validateGuestOS`: `windows` for qemu and hyperv | `Info.GuestOS`; the value must be `linux` or in some linked driver's list, and, unless it is `linux`, in the selected driver's |
| `defaultImageOS`; the `driver/os` switch in `runImagePrepare`; `checkImageUpdate` | `Info.ImageOS`, then `Images.PrepareImage`; `update` is vz's option |
| `resolveNetwork`, `hypervNetwork` | hyperv's `network` option; its default `warden-dev` is applied in the driver (`requestedNetwork` already does) |
| `linkRanges` | every linked driver's `Info.ReservedRanges` |
| `launcherStamps` | the driver's `Doctor` reports its own stamps |
| `layerCaches` | `Images.LayerCache()` of each linked driver, in list order |
| `hypervCmd` and its subcommands | `Commander.Command()` mounted at `warden hyperv` |
| `runShell`'s fix, `use warden build, or --driver container for an interactive shell` | The same sentence, with the names of the linked drivers whose `Info.Shell` is true |
| `image verify`'s driver choice (`--driver` only; none means every cache) | Unchanged, over `Images.LayerCache()` |
| The per-driver paragraphs in the long help of `doctor`, `setup`, `image prepare` and `clean`, and the driver example lines | `Info.Help`, `Info.Examples`, in list order |
| `image prepare`'s `--os defaults per driver (vz: macos, ...)` | Generated from each driver's `Info.ImageOS`, in list order |

Three passages of long help describe several drivers in one sentence and
are not any one driver's paragraph. They stay in the frame as written, so
`cmd/warden` still names drivers in that prose after K1: `doctor`'s
sentence on which launcher stamps fail it (hyperv and vz) and which are
reported only (qemu and firecracker); `image prepare`'s closing paragraph
on `--sha256`, `--force` and `--update (vz only)`; and `clean`'s short
description and lead paragraph, which describe the container driver's
resources. A new driver does not edit them: its own text is its `Info.Help`.
K2 splits or drops them when each driver gets its own binary (a5).

### The operations, per driver

What each driver does today for each operation, and where it goes. "Frame
preamble" is `doctorBuildLink` (the build link and its source, the host
overlap check, the launcher stamps); it becomes a kit helper,
`driver.ReportBuildLink`, that each VM driver's `Doctor` calls first, so the
output keeps its order. `doctorResolvers` becomes `driver.ReportResolvers`.

| | container | qemu | vz | hyperv | firecracker |
|---|---|---|---|---|---|
| **Runs on** (build tags) | any host with a runtime; no tagged files | any host; `vm_unix.go`, `vm_windows.go` | macOS/arm64: `vz_darwin_arm64.go` (cgo); `vz_stub.go` elsewhere | Windows: `*_windows.go`; `*_other.go` and `driver_stub.go` elsewhere | Linux: `*_linux.go`, with amd64 checked at run time; `driver_other.go` elsewhere |
| **Doctor** | `resolveRuntime` (config, else `DetectRuntime`: finch, docker, podman by `<rt> info`); prints `container runtime: <rt>`, then `ok` | preamble; `qemu.Preflight` (qemu-system and qemu-img on PATH, relay VM assets); `qemu.Resolvers`; `ok` | preamble; `vz.Preflight` (platform, entitlement); `vz.Resolvers`; `ok` | preamble; `hyperv.Detect(network)`; `hyperv.Doctor` | preamble; `firecracker.Doctor` |
| **Setup** | none | none | none | `hyperv.RunSetup` (NAT switch, host IP, NAT rule, slot switches) | `firecracker.RunSetup` (slots, bridge table, WARDEN-FC chains; root) |
| **Image prepare** | none | linux: `linuxguest.PrepareImage`; windows: `prepareQemuWindows` in `cmd/warden` (`AcquireWindowsMedia`, `InstallWindowsImage`, `PrepareWindowsLayer`) | macos: `prepareVZMacos` in `cmd/warden` (`Preflight`, `RequireRoot`, `ImageCache.Prepare`) | windows: `prepareHyperVWindows` and `hypervImageSource` in `cmd/warden` (`FetchEvalImage`, `PrepareBase`) | linux: `firecracker.PrepareImage` |
| **Image list** | none | glob of `<cache>/images/*.qcow2` in `cmd/warden` | `ImageCache.Images` and `formatVZImages` in `cmd/warden`, on darwin | `hyperv.Status` formatted in `cmd/warden` | `firecracker.ListImages` |
| **Image verify** | none | `qemu.LayerCache` (the `linuxguest` cache) | `ImageCache.LayerCache`, on darwin | `hyperv.LayerCache` | `firecracker.LayerCache` |
| **Clean** | the runtime cleanup written in `cmd/warden/clean.go` | none; falls through to the container cleanup | none; falls through likewise | `hyperv.Clean` | `firecracker.Clean` |
| **Own commands** | none | none | none | `service install`, `uninstall`, `run` | none |
| **Relay start** | an OCI image, published on host loopback, claimed there | relay VM by direct kernel boot; claim port forwarded to host loopback | a host process in host mode on fd 3; claimed on host loopback | relay VM from a boot VHDX with a per-build seed; claimed at its per-build NAT address | relay VM by direct kernel boot; claimed at the slot's relay address |

The code in the `cmd/warden` cells moves into the driver that owns it, with
its tests (`image_vz_test.go`, `image_hyperv_test.go`, the runtime tests in
`fixable_test.go`). Each driver's exported helpers that only `cmd/warden`
called (`qemu.Preflight`, `hyperv.Detect`, `firecracker.RunSetup`, ...) stay
as that driver's internals behind its contract methods, and may lose their
export. What `tools/vz-image-prep` calls in `driver/vz` stays exported, and
those two tool files stay where they are (Rule changes, ORCH-01). Also
moving:

- `cmd/warden/main_darwin_arm64.go`'s `runtime.LockOSThread` goes to an
  `init` in `driver/vz/vz_darwin_arm64.go`; an `init` runs on the main
  goroutine, so the effect is the same.
- `relayvm.QEMURelay`, `QEMUBuild` and `Firecracker` go to `driver/qemu` and
  `driver/firecracker`; `relayvm` keeps `Asset`, `Find`, `StampPath` and
  `RelayInitrd`. `RelayInitrd` takes the relay binary's path
  (`RelayInitrd(base, relayBin, workDir)`) in place of an architecture: the
  binary now arrives in the resolved `Relay`, and `RelayInitrd` no longer
  calls `driver.GuestBinary`.
- `driver.ContainerBaseCIDR`, `WardenBaseNet` and `AllocateNetwork` go to
  `driver/container`, which declares the range in `ReservedRanges`; qemu
  declares `UserNetCIDR` and firecracker `UpstreamCIDR`.
- `driver/buildlink/disjoint_test.go` is split by owner. The default build
  link against each driver's `ReservedRanges` is checked by
  `drivertest.Contract`, and a test in `cmd/warden` checks that a link
  overlapping any linked driver's range is refused by name. The case for
  Hyper-V's default NAT prefix moves to a test in `driver/hyperv`: the NAT
  is not a reserved range (it is read back from the host and checked by the
  driver), so a test built from `ReservedRanges` would lose it.
- `driver/claimlink_test.go` becomes a helper,
  `drivertest.ClaimsWithRequestLink(t)`, which makes the same source check
  on the calling package: one `ClaimRelay` call, whose link argument is the
  request's. qemu, firecracker and vz each call it from a test in their own
  directory. The container driver does not (it passes no link on purpose),
  and hyperv does not (its claim is tested end to end over its
  `Provisioner`), as today.

Two places re-run the `warden` command line through `driver.StartDetached`:
qemu's and hyperv's background image refresh (`image prepare --driver ...`).
They keep working, since the frame still takes those arguments.

### The relay source

Today, on `main`:

| Driver | What it runs | Where it finds it | Override |
|---|---|---|---|
| container | An OCI image | Release build: pulls `ghcr.io/buildwarden/relay:latest`. Dev build (`version == "dev"`): builds `cmd/relay` for Linux and an image `warden-relay:<build id>` per build (`resolveRelayImage`, `buildRelayFromSource`) | `[runtime] relay_image` or `WARDEN_IMAGE`: an image reference, pulled; or `dev`, built from source |
| qemu | A relay VM: kernel, Alpine base initramfs, and the Go relay appended per build at `/sbin/relay` (`relayvm.RelayInitrd`) | Kernel and initramfs: beside the `warden` binary (`relay-vm-vmlinuz`, `relay-vm-initramfs.cpio.gz`), else `tools/relay-vm/output/` in the source tree, never mixed (`relayvm.Find`). Relay binary: `driver.GuestBinary("relay", "linux", GOARCH)` | none |
| firecracker | The same shape, x86_64: `firecracker-vmlinux`, `firecracker-relay-initramfs.cpio.gz`, relay appended | `relayvm.Find(relayvm.Firecracker)`; `driver.GuestBinary("relay", "linux", "amd64")` | none |
| vz | A host process: the Go relay for darwin/arm64, copied to the work dir and ad-hoc signed, run as `--mode=host --fd=3 --claim-addr=127.0.0.1:<port>` with the token in its environment (`startHostRelay`) | `driver.GuestBinary("relay", "darwin", "arm64")` | none |
| hyperv | A relay VM from a static boot VHDX that bakes kernel, init and relay, with a per-build overlay and seed (`resolveRelayBootVHDX`, `StartRelay`) | `relay-boot.vhdx` in the hyperv cache (`%LOCALAPPDATA%\warden\hyperv`, or `WARDEN_HYPERV_CACHE_DIR`), built by `tools/relay-vm/hyperv/refresh-cache.sh` | `WARDEN_RELAY_BOOT_VHDX`, a path |

`driver.GuestBinary` looks beside the `warden` binary, then in the
per-source-tree build cache, then builds from the source tree. None of the
five checks a hash, and the container default is a moving tag.

The types, in `driver/relay.go`. The form and role names are provisional
until seam 8 is written (a11):

```go
// RelayForm names a relay release form (seam 8).
type RelayForm string

const (
	// RelayHostBinary is a relay executable run on the host in host mode.
	RelayHostBinary RelayForm = "host-binary"
	// RelayOCIImage is a container image run in container mode.
	RelayOCIImage RelayForm = "oci-image"
	// RelayAppliance is a Linux relay VM on the kernel network path: a kernel
	// and an initramfs, or one boot disk.
	RelayAppliance RelayForm = "appliance"
	// RelayUnikernel is a single bootable unikernel image. No driver runs
	// one in K1; the name is reserved for M6.
	RelayUnikernel RelayForm = "unikernel"
)

// Relay is the relay one build runs, resolved to local files or an image.
type Relay struct {
	Form RelayForm
	// Version is the release it comes from, "dev" for a source-tree build,
	// or "" when unknown (an override).
	Version string
	// Origin says where it came from, for messages: "bundled", "source
	// tree", or the option or variable that named it.
	Origin string
	// Image is the reference to run, for RelayOCIImage.
	Image string
	// Files are the form's files by role.
	Files map[RelayRole]RelayFile
}

// RelayRole names a file's part in a form.
type RelayRole string

const (
	RelayBinary    RelayRole = "binary"    // host-binary; or, in an appliance, a
	                                       // relay to append at /sbin/relay
	RelayKernel    RelayRole = "kernel"    // appliance, unikernel
	RelayInitramfs RelayRole = "initramfs" // appliance
	RelayDisk      RelayRole = "disk"      // appliance as one boot disk
)

// RelayFile is one file of a relay.
type RelayFile struct {
	Path string
	// SHA256 is the published hash the file must have, or "" when none is
	// published for it. Verify checks every file that has one.
	SHA256 string
}

// Verify re-hashes each file that carries a published hash and refuses a
// mismatch with a FixableError naming the file and its origin.
func (r *Relay) Verify() error

// RelaySource resolves the relay for a build.
type RelaySource interface {
	Relay(ctx context.Context) (*Relay, error)
}
```

`driver/relaysource` holds the sources that exist today, each wrapping the
current lookup without changing it:

```go
// Bundled finds a relay's files beside the running binary, else in the
// source tree, all from one place (relayvm.Find's rule), and builds or finds
// the relay binary as driver.GuestBinary does. Relay returns an error
// wrapping ErrNotFound when neither place has every file; the driver adds
// its own fix.
func Bundled(form driver.RelayForm, files ...File) driver.RelaySource

// File is one bundled file: its release name beside the binary and its path
// in the source tree, or a cmd/ package to build for an OS and architecture.
type File struct {
	Role            driver.RelayRole
	Release, Source string
	Build           *GoBuild // cmd/relay for linux/arm64, ...
}

// Path is a relay file the user named: used as given, never looked up
// elsewhere. origin is the option or variable that named it.
func Path(form driver.RelayForm, role driver.RelayRole, path, origin string) driver.RelaySource

// Image is an OCI image reference the user named, pulled as given.
func Image(ref, origin string) driver.RelaySource
```

Each driver's relay-start step takes a `*driver.Relay` and calls
`Verify` before it starts anything:

| Driver | Form in K1 | Source in K1 | With an override |
|---|---|---|---|
| container | `oci-image` | Its own two cases stay in `driver/container` behind one `RelaySource`: the release tag for a release build, the per-build source image for a dev build | `relay-image` set: `relaysource.Image`, or the source build for `dev` |
| qemu | `appliance` (kernel, initramfs, binary to append) | `relaysource.Bundled` over today's three files | none |
| firecracker | `appliance` (the same roles) | A source in `driver/firecracker` over its one lookup of three files (below) | none |
| vz | `host-binary` | `relaysource.Bundled` with a `GoBuild` for darwin/arm64 | none |
| hyperv | `appliance` (one boot disk) | A source in `driver/hyperv` over the cache path | `WARDEN_RELAY_BOOT_VHDX` set: `relaysource.Path` |

Firecracker's relay files are not a lookup of their own. One
`relayvm.Find` resolves the kernel that both the relay VM and the build VM
boot, the relay initramfs and the direct-boot build initramfs, all from one
place, with one error (`firecracker boot assets (vmlinux, relay and
direct-boot initramfs) not found`); the launcher stamp uses the same
lookup. That stays one lookup, in `findAssets`. The driver's relay source
builds the `Relay` (kernel, initramfs, and the binary from
`driver.GuestBinary`) from its result, and the build VM takes its kernel
and initramfs from the same result, so relay and build assets are still
never mixed and the error reads as it does now. qemu already makes two
lookups, one for the relay VM and one for the build VM, so `Bundled` wraps
its relay lookup as it is, and qemu wraps `ErrNotFound` in today's message
and fix. If M6 gives firecracker a relay with a kernel of its own, the two
lookups separate then, inside `driver/firecracker`.

What this gives M6. Adopting a Rust release is a change inside one driver's
directory: its `RelayForms`, the source its build resolves, and the boot
code for a form it has not run before. A pinned source (version, URL and
published hash, downloaded to the cache and verified through `SHA256`) is
one more implementation of `RelaySource`; its manifest and hash format are
seam 8's, which is not written yet (a3). Nothing in K1 needs the Rust relay:
every source resolves the Go relay as today. A Rust appliance that carries
its relay inside the initramfs is an `appliance` with no `binary` role, and
the qemu and firecracker start code calls `RelayInitrd` only when the role
is present.

The relay source adds no relay setting. What it chooses is the artifact and
its launch wiring (`--mode`, `--fd`), which RELAY-08 calls "how the relay is
started, not configuration for a build". Every setting still travels in the
claim; the boot channel carries what it carries today.

Nothing new is recorded in the ledger. Whether a build's record should name
the relay's form and version is left to seam 8 and M6. Seam 8 has no phase
that writes it (a11); until it exists, the pinned source's manifest and hash
format, and this question, have no owner.

### The boundary with S4

K1 stops at "the relay is running, unconfigured". S4 starts at "the driver
hands the session a connection to it". K1 leaves `Session.ClaimRelay(ctx,
addr, claimToken, req, env, link, oldRelayFix)` exactly as it is, and
changes no claim call's arguments. The contract document says a driver
claims its relay through `driver.Session` and names S4 as the item that
defines the connection. Both items edit each driver's relay-start function
(K1 its input, S4 its output); the second to publish merges them (a2).

### `drivertest`

`driver/drivertest`, a public package of the kit, with two suites, one
fake, and the source check `ClaimsWithRequestLink` described above.

**`drivertest.Contract(t, d)`** needs no hypervisor, runtime or network and
runs on every host. It checks what a driver declares and what it does with
an empty cache:

- `Info`: a name of lowercase letters and digits; the same value on every
  call; `GuestOS` and `ImageOS` well formed; `ReservedRanges` that parse and
  do not overlap the default build link; `Help` and `Examples` keyed by
  known commands.
- `Options`: names unique within the driver; a known kind; usage text
  present (UX-01); a config key in `[driver.<name>]`, or one of the seven
  keys that predate the rule; an environment variable only for the two that
  exist. A clash with another driver's flag or with the frame's is not
  checked here: `drivertest` is a kit package and cannot see the frame's
  flags, which are in `cmd/warden` until K2. The frame's registration test
  checks it (Options, step 1).
- Consistency: `ImageOS` is set exactly when the driver implements `Images`;
  an option on `setup`, `image prepare` or `clean` needs `HostSetup`,
  `Images` or `Cleaner`; `RelayForms` is not empty.
- With the cache, temp and home directories pointed at empty ones:
  `ListImages` writes nothing and returns 0, and `LayerCache` is nil or a
  cache under the cache directory. This relies on a driver resolving its
  directories per call and not in its constructor (Shape).

`cmd/warden` runs it for every driver in `drivers`, so `make test` covers
all five on Linux, macOS and Windows with no per-driver wiring, and a new
driver is covered the moment it is in the list. A driver's own package may
call it too.

**`drivertest.Lifecycle(t, d, opts)`** runs real builds, so it needs a host
the driver runs on. `opts` gives the guest OS, the driver's option values
(hyperv's network, for instance) and which optional cases apply. It builds
its own contexts: a POSIX script for Linux and macOS guests, PowerShell for
Windows, and a Dockerfile with a `COPY` where the guest takes one.

| Case | Checks | Rule |
|---|---|---|
| Doctor | Not blocked; nothing in the cache or temp directory changes | UX-04 |
| Success | A script that prints a marker and exits 0 returns nil; the output directory holds the ledger, the CA certificate and `build-output.log` with the marker; the ledger's signature verifies; no name outside the documented output layout | RELAY-07, ORCH-02 |
| Inputs | A `COPY`ed file is in the ledger as a relay-served input; a translated Dockerfile leaves `inputs/Containerfile` and `inputs/build.sh` | BOOT-04, ORCH-02 |
| Failure | A script that exits 3 returns `*driver.ExitError` with code 3, and the ledger is still written | UX-09 |
| Timeout | A sleeping script with a short `Timeout` returns an error wrapping `context.DeadlineExceeded` | UX-09 |
| Cancel | Cancelling the context mid-build returns promptly | UX-09 |
| Shell | Without `Info.Shell`, `Exec` returns `ErrExecNotSupported`; with it, `Exec` runs and reports the shell's exit code as an `ExitError` | UX-09 |
| Two at once | Two concurrent builds both succeed (optional per `opts`) | ORCH-04 |
| Teardown | After each case no work directory remains, and `Clean`, when implemented, removes nothing | ORCH-04 |

It does not test isolation (S3's build-link suite), the claim's security,
or image preparation, which is driver-specific and can take hours.

A driver runs it from a test in its own directory behind the `drivertest`
build tag, so `make test` never starts a VM:

```go
//go:build drivertest

func TestLifecycle(t *testing.T) {
	drivertest.Lifecycle(t, New(), drivertest.Options{GuestOS: "windows"})
}
```

`make drivertest DRIVER=<name>` runs it (for vz it builds and signs the test
binary first, as `integration-test-vz` does today). Tagged files are not
compiled by a plain `go test` or by `make lint`, like the
`integration`-tagged vz test today, so two targets change to keep them from
rotting: `make test` also compiles them without running any (`go test -tags
drivertest -run '^$' ./driver/...`), and `make deadcode` runs with `-tags
drivertest`, so `Lifecycle` is reachable from the tests that call it.
Where it runs:

| Driver | `Contract` | `Lifecycle` |
|---|---|---|
| container | `make test`, CI matrix | The `integration` workflow (Docker on a hosted runner); macos-finch natively |
| firecracker | `make test`, CI matrix | The `firecracker` workflow (hosted runner with KVM) |
| qemu | `make test`, CI matrix | Native, qemu-any |
| vz | `make test`, CI matrix | Native, macos-vz |
| hyperv | `make test`, CI matrix | Native, windows-hyperv |

**`drivertest.Recorder`** is a fake driver that implements every interface
and records each request. The frame's tests use it to check what the frame
passes for each command, and one test links only the recorder into the
frame and drives every command: the proof that a driver can be added
without a change to `cmd/warden` beyond the list.

### Behaviour

Seven things change for a user (a5). Everything else is meant to be
identical; the cases a reader might expect to change and that do not are
listed after them.

1. **A flag the selected driver does not take is refused.** Today eleven
   flags are accepted and ignored on drivers that do not use them:
   `--runtime` on any driver but container; `--network` on `doctor` and
   `setup`, and `--nat-prefix` and `--host-ip` on `setup`, for any driver but
   hyperv; and on `image prepare`, `--sha256` and `--name` (hyperv only),
   `--edition`, `--virtio` and `--arch` (qemu only), `--force` and
   `--keep-vms` (qemu and hyperv only). Each now gets the refusal that
   `--network` on `build` and `--slots` on `setup` already get. GLOBAL-10
   asks for this: a provided value that cannot be used is an error.
2. **The existing refusals take the generated form**, `<flag> is only used
   by --driver <names>` with the fix `drop <flag>, or use --driver <names>`.
   `--network` on `build` and `--slots` on `setup` keep their message and
   gain that fix (`--network` on firecracker had a fix of its own, which
   goes); `--update` on a driver other than vz reads `--update is only used
   by --driver vz` where it read `--update is not supported for --driver
   <name> yet`. The VM-group and `--guest-os` refusals keep their text.
3. **`warden clean --driver qemu` and `--driver vz` no longer run the
   container runtime's cleanup** (Decision 10). They report nothing to clean
   and handle the cache as before. With no `--driver`, the default driver is
   still container and nothing changes.
4. **`warden image list` prints drivers in list order**: qemu, vz, hyperv,
   firecracker, where it printed vz, qemu, firecracker, hyperv. Each
   driver's lines are unchanged.
5. **Two messages list drivers in list order**: `no <os> image for --driver
   ...` and the fix of `image verify`'s `no base-image layer cache` error.
6. **The long help of `image prepare` and `clean` is assembled per driver.**
   The content is today's; qemu's two `image prepare` paragraphs become
   adjacent, `clean`'s two driver sentences swap, and `image prepare`'s list
   of default `--os` values reads in list order (`qemu: linux, vz: macos,
   hyperv: windows, firecracker: linux`). `doctor` and `setup` read as they
   do now: their per-driver paragraphs are already in list order.
7. **A malformed `[vm] image_refresh_after` or `image_min_grace` is refused
   on every command that loads the configuration.** Today `newRequest`
   parses the two ages for every driver, so a bad value is refused on
   `build` and `shell` whichever driver is selected, and on no other
   command. After K1 they are qemu's and hyperv's options, checked with
   every other driver-declared key when the configuration is loaded
   (Options, step 2): still refused whichever driver is selected, and now
   also on `doctor`, `setup`, `image prepare` and `clean`. The message keeps
   its text and fix and names the file the value was read from, where it
   always said `warden.toml`. The alternative, checking only the selected
   driver's keys, would let a bad value pass silently until its driver is
   used, which UX-08 is there to prevent.

What does not change, although the code behind it moves:

- `warden shell` on a driver without `Exec` gives today's error and fix,
  `use warden build, or --driver container for an interactive shell`. The
  driver name in the fix comes from `Info.Shell`.
- `warden image verify` selects by the `--driver` flag only, ignoring
  `WARDEN_DRIVER` and `[runtime] driver`, and checks every cache when the
  flag is absent.
- The three help passages listed under **What the frame derives from
  `Info`** keep their wording.
- The five fix messages in which one driver suggests another keep their
  wording; the container runtime probe's moves into `driver/container` with
  its text.
- A wrong TOML type for any key still fails the load, whichever driver is
  selected. The error is the frame's kind error, naming the file and key,
  where a driver-owned key's was the TOML library's.
- The environment variables the drivers read for themselves (**Defaults and
  overrides**) are untouched.

`docs/book/src/guide/cli.md` is updated for each in the same change (UX-01).
No flag, subcommand, config key or environment variable is added, renamed
or removed (UX-03), and exit codes are unchanged (UX-09): a blocked doctor
still exits 1 with no extra line.

### What guards behaviour

- **Characterization tests, written first, on unchanged code.** Before any
  production change, `cmd/warden` gains golden tests of: `--help` for every
  command; the error and fix of every refusal the frame makes, per driver;
  the `BuildRequest` each driver gets from a table of flags, environment and
  config files (extending `request_test.go`, `vmdns_test.go` and
  `buildlink_test.go`); the refusal of a malformed age and of a wrong TOML
  type, per command; and `image list` and `image verify` over fixture
  caches. The refactor's commits may change a golden only for the seven
  differences above, so each shows up as a reviewed golden diff.
- **The surface check.** Tier 1's extraction (Rule changes) must find the
  same set of flags and keys on the branch as on `main`. A flag or key lost
  in the move fails `surface-removed`, and the `--help` goldens show a flag
  that was declared but not registered.
- **The drivers' own tests are untouched.** Each driver's doctor, setup,
  clean and prepare logic keeps its internal functions and their unit tests
  (`doctor(h, out, ...)` in firecracker, `clean(...)` in hyperv, and so on);
  only the exported wrapper changes shape.
- **`drivertest`**, the `integration` and `firecracker` workflows, and the
  native steps under **Validation plan**.

### Order of the refactor

One branch, one PR, each commit green on `make test` and `make lint` for
the platforms it touches (FLOW-17).

1. Characterization tests. No production code changes.
2. The contract document, then the new types beside the existing
   four-method `driver.Driver`, which is not changed yet (`Info`, `Option`,
   `Values`, the requests, the optional interfaces, `Relay`),
   `driver/relaysource`, and the tier-1 surface change. Nothing uses them
   yet. The document comes first because the umbrella has a seam's
   specification change before its code.
3. The frame. `driver.Driver` takes its final method set, `driver/drivertest`
   is added with the recorder, and `cmd/warden` is rewritten over
   `[]driver.Driver`. A temporary `cmd/warden/legacy.go` adapts each of the
   five packages to the contract by wrapping the exported functions
   `cmd/warden` calls today. The one driver-package edit in this commit is
   `driver/container/container.go`'s compile-time assertion that it
   implements `driver.Driver`, which goes until step 4 restores it. The
   seven behaviour differences appear here, and only here.
4. to 8. One driver per commit: container, firecracker, qemu, vz, hyperv
   (the two CI can run end to end first). Each moves that driver's logic out
   of `cmd/warden` into its own directory, implements the contract there,
   routes its relay start through a relay source, adds its `drivertest`
   tests, and deletes its adapter from `legacy.go`. `BuildRequest` loses a
   field in the commit that converts its last reader.
9. Delete `legacy.go`. Move the driver names out of the shared packages. Add
   the imports test: no Go file outside `driver/<name>/` imports that
   driver's package, except `cmd/warden/drivers.go` and the two standalone
   files of `tools/vz-image-prep`, which the test lists by path.
10. The rule text (GLOBAL-08, ORCH-01), `rules.yaml`, the contract document,
    the reference docs, `structure.md` and `overview.md`.

Because each driver is converted in its own commit against a frame that is
already final, a failure found in one driver's native validation is fixed
in that driver's directory.

### What K1 leaves for K2

- The frame is still package `main`. K2 moves it into the kit and builds one
  binary per driver over it.
- Flags are the union of every linked driver's, and every linked driver's
  config keys are kind-checked at load. With one driver per binary both are
  that driver's alone; K2 decides who checks another driver's keys.
- The frame's help keeps three passages that describe several drivers
  (**What the frame derives from `Info`**). They are split or dropped when
  the frame serves one driver.
- `drivertest.Contract` cannot check a driver's flags against the frame's
  until the frame is a kit package.
- GLOBAL-08's sentences on outside drivers and on the kit module's version
  (a10).
- The build-link disjointness check covers every linked driver's reserved
  ranges. A single-driver binary knows only its own; K2 decides whether the
  dispatcher or the driver index carries the rest.
- The installed Hyper-V service's command line (`warden hyperv service
  run`) and the two `StartDetached` re-invocations name the `warden`
  binary. They are correct in K1 and must be revisited when the binary's
  name changes.
- `driver.GuestBinary` and `driver.ModuleRoot` assume the kit's module is
  the driver's. BOOT-02's "from the driver's own kit version" is K2's.

### The overview (FLOW-15)

K1 is the only item of its phase, so it makes the sweep itself. The
orchestrator's pseudocode in `concepts/overview.md` changes to:

```
warden build <context>:
    load the configuration; refuse a value of the wrong kind for any key a
        linked driver declares
    driver := the linked driver named by --driver, WARDEN_DRIVER or
              [runtime] driver, else the default (container)
    refuse a flag the driver does not declare for this command
    req := resolve the frame's flags and config (output dir, capture, ...)
    req.Options := the driver's own options: flag, else environment, else
                   warden.toml, else the user config; each used exclusively
    VM drivers (Info.VM): req.BuildLink, req.DNS as today
    driver.StartBuild(req)
    print the result; exit with the build script's exit code
```

and `StartBuild`'s skeleton gains one line before "start the relay":
`relay := the driver's relay source (bundled, source tree, or the user's
override); verify its published hashes, when it has any`. A short section
describes `doctor`, `setup`, `image` and `clean` as calls on the selected
driver's interfaces.

The dead-code sweep: `make deadcode` runs with `-tags drivertest` (see
**`drivertest`**), so the suites are reachable from the drivers' tagged
tests. FLOW-15's stated reasons for keeping an unreachable exported function
do not cover `driver/`, so a contract function the sweep still reports is
removed, not kept; widening those reasons is a rule change for K2, when the
kit has consumers outside this repository.

## Defaults and overrides

K1 adds no user-facing default. It moves existing ones behind the contract,
drops one fallback, and fixes two conventions for later items. Each is
listed; anything not listed is unchanged and frame-owned.

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The driver when none is named: `container` | Unchanged. Now the constant `defaultDriver` in `cmd/warden/drivers.go` | It is today's default and needs no host setup | `--driver`, else `WARDEN_DRIVER`, else `[runtime] driver` (today's order) |
| A driver option nobody set | Unchanged per option: the driver applies its own default (`network`: `warden-dev`; `slots`: 4 for hyperv, 16 for firecracker; `nat-prefix`: `192.168.240.0/20`; `host-ip`: `192.168.240.1`; `name`: `windows-server-2025`; `arch`: the host's; the two ages: `winguest`'s; `runtime`: the first of finch, docker, podman that answers `info`, the documented autodetection) | These are the values `cmd/warden` and the drivers apply today | The flag, else the option's environment variable, else `warden.toml`, else the user config. The first that sets it is the value; it is not merged with a lower level, and a value that does not parse or cannot be used is an error naming it (UX-08) |
| A flag the selected driver does not declare | **Changed** (Behaviour 1): refused. Eleven flags were ignored | A provided value that cannot be used is an error (GLOBAL-10) | Nothing: drop the flag or select the driver that takes it. Config keys and environment variables for other drivers are not refused for being another driver's, by design |
| A driver-declared config key whose value has the wrong kind | **Changed** for the two ages (Behaviour 7): refused when the configuration is loaded, whichever driver is selected. A wrong TOML type was already refused at load | A value that cannot be used is an error, found before any work (GLOBAL-10, UX-08) | Nothing: fix the value |
| Where a driver's relay comes from | Unchanged lookups, now behind `RelaySource` (table under **The relay source**) | Each driver runs the relay that ships with, or is built from, the same tree as `warden` | container: `[runtime] relay_image` or `WARDEN_IMAGE`; hyperv: `WARDEN_RELAY_BOOT_VHDX`. Each is used as given: an image that cannot be pulled or a path that cannot be read is an error, and the default is not tried. The other three drivers have no override (a3) |
| A relay file's published hash | **New**, inert in K1: `Relay.Verify` checks a file that carries one. No K1 source sets one | Nothing publishes relay hashes yet | When a source supplies a hash, a mismatch refuses the build; no other source is tried |
| Where a new driver-owned key lives: `[driver.<name>]` | **New convention**, with no key in it yet (a4) | It cannot collide with the frame's tables and says whose it is | The seven existing keys keep their tables |

Named lookups and fallbacks (GLOBAL-10, GLOBAL-11):

- **Kept, existing: "beside the binary, else the source tree".** The relay
  VM's kernel and initramfs come all from beside the `warden` binary or all
  from the source tree's tool output (`relayvm.Find`), and
  `driver.GuestBinary` looks beside the binary, then in the per-tree build
  cache, then builds from the tree. It is the difference between a release
  install and a checkout, not a retry after a failure; K1 moves it into
  `relaysource.Bundled` unchanged, and the driver guide already describes
  building the assets from the tree.
- **Kept, existing: container runtime autodetection** (finch, docker,
  podman), used only when no runtime is configured. It moves into
  `driver/container`.
- **Dropped: `warden clean` running the container runtime's cleanup for a
  driver that has no cleaner** (Decision 10). It was never named as a
  fallback, and it made `warden clean --driver qemu` fail for want of a
  container runtime.
- **None added.** In particular an overridden relay never falls back to the
  bundled one, and the frame never falls back to another driver.

RELAY-08: the design adds or moves no relay setting. The relay source picks
the relay artifact and its launch wiring, not configuration for a build; no
claim field, boot-channel value, flag or environment variable of the relay
changes.

`WARDEN_RELAY_BOOT_VHDX` and `WARDEN_HYPERV_CACHE_DIR` are environment
variables UX-07 would not accept as new knobs. K1 keeps them as they are and
adds none; the `relay` option of a3 is what would replace the first.

The drivers read other environment variables directly, and K1 leaves every
one as it is, outside the option model: hyperv's `WARDEN_HYPERV_BUILD_IMAGE`
(the build image when `--image` is not given), `WARDEN_HYPERV_BUILD_GEN` and
`WARDEN_HYPERV_BUILD_COM_PIPE`; qemu's `WARDEN_QEMU_BUILD_VNC`,
`WARDEN_QEMU_BUILD_MONITOR`, `WARDEN_QEMU_VNC`, `WARDEN_QEMU_MONITOR`,
`WARDEN_QEMU_VNC_PASSWORD` and `WARDEN_QEMU_INSTALL_TIMEOUT_MIN`. They are
development and debugging switches a driver reads for itself; the frame
never sees them, so "what the drivers declare is what `cmd/warden` holds
for them" is true of the flags, keys and the two declared variables only.
`drivertest.Lifecycle` runs with whatever of these the host has set.

## Cross-driver impact

`drivers: [interface]`: the change is made at the contract, and every driver
is converted the same way. Each implements `Info`, `Doctor` and the optional
interfaces it needs, reads its settings from `Options`, takes `Verbose` from
the request, resolves its relay through a `RelaySource`, and gains
`drivertest` tests. What differs:

| Driver | Optional interfaces | Moves into the driver from `cmd/warden` or the shared packages | Relay source |
|---|---|---|---|
| container (`Info.Shell`) | `Cleaner` | `DetectRuntime`, `runtimeStartHint`, `resolveRuntime`; the runtime cleanup of `clean.go`; `ContainerBaseCIDR`, `WardenBaseNet`, `AllocateNetwork`; the `runtime` and `relay-image` options. The `Runtime` and `Version` fields go | Its two cases (release tag, source build) and the `relay-image` override, behind one source |
| qemu | `Images` | `prepareQemuWindows`; the `*.qcow2` listing; `relayvm.QEMURelay` and `QEMUBuild`; `UserNetCIDR` as a reserved range; the Windows prepare options and the two ages | `relaysource.Bundled`, appliance |
| vz | `Images` | `prepareVZMacos`, `formatVZImages`, `checkImageUpdate` (as its `update` option); the main-thread pin. `New` stops resolving the cache directory | `relaysource.Bundled`, host-binary |
| hyperv | `HostSetup`, `Images`, `Cleaner`, `Commander` | `prepareHyperVWindows`, `hypervImageSource`, `hypervNetwork`, the image status lines, the three service commands; its twelve options | A source over the cache path; `WARDEN_RELAY_BOOT_VHDX` through `relaysource.Path` |
| firecracker | `HostSetup`, `Images`, `Cleaner` | `relayvm.Firecracker`; `UpstreamCIDR` as a reserved range; `slots` | Its own source over its one three-file lookup, appliance |

No driver's isolation, executor delivery, start trigger, claim, teardown or
environment record changes, so no row of `bootstrap.md`'s mapping changes
(BOOT-07). Files on the bootstrap surface are edited, which tier 1 reports
as a hint; none is a model change: `driver/container/container.go` (the
relay image and runtime), `driver/winguest/` (`FreshnessOptions` is added),
`driver/firecracker/setup.go` (`RunSetup` becomes the `Setup` method), and,
where the relay start or the image prepare takes its new input there,
`driver/firecracker/vm*.go` and `driver/vz/image.go`.

The guest families `driver/linuxguest` and `driver/winguest` change only by
`winguest.FreshnessOptions` and by reading the ages from `Options` at their
callers. `driver.Session` is not changed (a2).

## Rule changes

Five entries. The umbrella's table gives K1 the first two "for direction";
the other three change enforcement only and are asked as a8. Each is approved
by name in the design review (FLOW-19) and made in Implement (FLOW-14).

### GLOBAL-08 (amend), `docs/development/global.md`, `tools/tier1/rules.yaml`

**Why:** GLOBAL-08 says today that `driver.Driver` is "not covered by this
obligation", which is the sentence that lets `cmd/warden` name drivers.
After K1 a driver is written against a documented Go API with a conformance
suite, and linking it changes one line of `cmd/warden/drivers.go` (a1). The
wording below claims that much and no more (umbrella Decision 18). It does
not yet say that a driver from outside the project is a supported consumer,
or how the contract is versioned: an outside driver still has to patch that
list, and there is no kit module, until K2. Those two sentences are K2's
amendment (a10). The umbrella's other additions to the seam list (the
truststore endpoints, the build-link contract, the claim connection) belong
to S2, S3 and S4, and this change leaves the two existing bullets alone so
they can be amended there.

**Current wording** (from "Versioned seams" to the end of the rule; the
paragraphs before it are unchanged):

> **Versioned seams** (same pre/post-1.0 posture as the ledger format in RELAY-05):
>
> - Relay public interface: the claim protocol (`relaycfg/`: per-build config
>   pushed over a host-initiated tunnel, relay identity and `ca.cert.pem` returned),
>   the collector protocol carried over that tunnel, control plane, and the DNS /
>   artifact / heartbeat protocol. `cmd/collector` is the reference standalone
>   consumer for third-party orchestrators.
> - Executor public interface: the `initialize` invocation contract plus its side
>   of the relay protocol.
>
> Pre-1.0 these seams may still move. Post-1.0 they are stable and change rarely and
> deliberately, with explicit interface versioning. The `driver.Driver` interface
> and other orchestrator internals are **not** covered by this obligation; the
> orchestrator may treat them as internal extension points and stay rigid about its
> expected components.
>
> The automated part requires a change touching a versioned seam to also change that
> seam's contract document.

**Proposed wording** (the same span):

> **Versioned seams** (same pre/post-1.0 posture as the ledger format in RELAY-05):
>
> - Relay public interface: the claim protocol (`relaycfg/`: per-build config
>   pushed over a host-initiated tunnel, relay identity and `ca.cert.pem` returned),
>   the collector protocol carried over that tunnel, control plane, and the DNS /
>   artifact / heartbeat protocol. `cmd/collector` is the reference standalone
>   consumer for third-party orchestrators.
> - Executor public interface: the `initialize` invocation contract plus its side
>   of the relay protocol.
> - Driver contract: the Go API by which a driver plugs into the orchestrator:
>   `driver.Driver` and its optional interfaces, `driver.Info` and its options,
>   the request types and the relay source (`driver/contract.go`,
>   `driver/options.go`, `driver/relay.go`, `driver/relaysource/`). Its
>   conformance suite is `driver/drivertest` (ORCH-01) and its contract document
>   is [`contributing/driver-contract.md`](../book/src/contributing/driver-contract.md).
>
> Pre-1.0 these seams may still move. Post-1.0 they are stable and change rarely and
> deliberately, with explicit interface versioning. The driver contract is
> supported but **unstable before 1.0**: it may change in any release with no
> deprecation window, and a change to it updates its contract document and
> `drivertest` in the same change.
> The orchestrator's other internals (`cmd/warden`, and what `driver/` holds
> beyond the contract) are **not** covered by this obligation. Opening the driver
> contract does not make the orchestrator composable the other way: a driver
> chooses which release form of BuildWarden's relay it runs, through its relay
> source, and substitutes neither the relay nor the executor.
>
> The automated part requires a change touching a versioned seam to also change that
> seam's contract document.

**Severity and check:** MUST, `automated + agent-judgment`, before and after.

**Enforcement:** `seams` in `tools/tier1/rules.yaml` gains one entry, so the
existing `seam-contract` check covers the driver contract: `{name:
driver-contract, code: [driver/contract.go, driver/options.go,
driver/relay.go, driver/relaysource/], contract:
docs/book/src/contributing/driver-contract.md}`. A change to one of those
files without the contract document then fails tier 1. The comment above the
list is corrected to match. S1 fills the same list for seams 1, 2, 3, 5 and
6; the entries do not overlap, and whichever lands second merges the list.
The contract types are kept in three files of their own so that edits to
`session.go` or the host helpers do not trip the check. The contract's
signatures also use types it does not own (`buildlink.Link` and `Range`,
`LayerCache`, `FixableError`, `ExitError`, `ErrExecNotSupported`); their
files are not listed, so a breaking change to one of them is left to the
agent part of the rule.

### ORCH-01 (amend), `docs/development/orchestrator.md`, `tools/tier1/rules.yaml`

**Why:** parity is asserted today design by design ("expressed at the
`driver.Driver` interface level, or it explicitly names which driver(s)").
With `drivertest` it can be run. The rule also gains the sentence that keeps
K1's result: no driver's package imported from outside its directory. The
sentence is about imports, which a test can decide, and not about naming:
drivers are still named in prose after K1, in help text, in the fix of
messages that suggest another driver, and in tests that select a driver by
its name.

**Current wording:**

> ### ORCH-01 — Driver parity / no cross-contamination
>
> **Severity:** MUST
> **Check:** automated + agent-judgment
>
> A build-lifecycle change is expressed at the `driver.Driver` interface level, or it
> explicitly names which driver(s) it touches (container / qemu / vz, plus planned
> hyperv / firecracker) and asserts the others are unaffected. This is the standing,
> permanent form of the cross-contamination check: a change made for one driver must
> not silently alter the behavior of another.
>
> Lifecycle that every driver shares lives in `driver.Session` and the other
> `driver/` helpers; a driver re-implements it only where its platform genuinely
> differs, and the design says why.
>
> The claim is the design's `drivers` front-matter (FLOW-02). The automated part
> compares which `driver/*` trees the diff actually reaches against that claim; the
> agent part judges whether shared code was altered in a driver-specific way.

**Proposed wording:**

> ### ORCH-01 — Driver parity / no cross-contamination
>
> **Severity:** MUST
> **Check:** automated + agent-judgment
>
> Every driver implements the driver contract (GLOBAL-08) and passes
> `driver/drivertest`: `drivertest.Contract` on every host platform, and
> `drivertest.Lifecycle` on a host that can run the driver. Passing `drivertest`
> is what driver parity means: the same request gives the same build lifecycle,
> output layout and exit behavior on every driver, and a driver refuses what it
> cannot do with a fixable error (UX-04) instead of ignoring it.
>
> A build-lifecycle change is expressed in the driver contract, the shared
> `driver/` helpers and `drivertest`, or it explicitly names which driver(s) it
> touches and asserts the others are unaffected. This is the standing, permanent
> form of the cross-contamination check: a change made for one driver must not
> silently alter the behavior of another. No Go file outside a driver's own
> directory imports that driver's package, except the one list that links the
> in-tree drivers into `warden` (`cmd/warden/drivers.go`) and a driver's
> standalone maintenance programs under `tools/`.
>
> Lifecycle that every driver shares lives in `driver.Session` and the other
> `driver/` helpers; a driver re-implements it only where its platform genuinely
> differs, and the design says why.
>
> The claim is the design's `drivers` front-matter (FLOW-02). The automated part
> compares which `driver/*` trees the diff actually reaches against that claim,
> and `make test` runs `drivertest.Contract` against every driver `warden` links
> and fails on a driver imported from outside its directory; `drivertest.Lifecycle` runs
> in each driver's tier (TEST-02). The agent part judges whether shared code was
> altered in a driver-specific way.

**Severity and check:** MUST, `automated + agent-judgment`, before and after.

**Enforcement:** the `drivers-claim` check is unchanged. Its `drivers.shared`
list in `tools/tier1/rules.yaml` gains `driver/drivertest/` and
`driver/relaysource/`, and `driver/buildlink/` and `driver/resolvers/`, which
every VM driver already depends on and the list omits today; a design that
changes one of them must then declare every driver, as for the other shared
packages. The two new automated parts are ordinary tests under `make test`
(`drivertest.Contract` over `drivers`, and the imports test), not tier-1
checks, so `rules.yaml`'s `ORCH-01: {check: [drivers-claim]}` stays as it
is. The imports test scans every Go file of the module, whatever its build
tags, and allows two lists by path: `cmd/warden/drivers.go`, and
`tools/vz-image-prep/personalize.go` and `uv-build-test.go`, which import
`driver/vz` today. Those are `//go:build ignore` programs for preparing and
testing a vz image, not part of `warden`; they stay where they are and move
with the vz driver when it is extracted (K4). Alternative: move them under
`driver/vz/` now (a move of working tools, and their documented paths, for
no gain before K4).

### UX-01, UX-03 and ORCH-03 (amend, enforcement only), `tools/tier1/surface/`

**Why:** tier 1 reads the user-facing surface from the cobra flag calls and
`toml` struct tags in `cmd/warden/` (`surface.dir`). Driver-owned flags and
keys become `driver.Option` values in the drivers' packages, registered by
the frame from data. Unchanged, the check would report every moved flag and
key as removed (UX-03) and would stop comparing them with the reference docs
(UX-01, ORCH-03). One file, `surface.go`, enforces all three rules, so the
edit is a change to each (FLOW-14) and each has an entry.

**Wording:** unchanged for all three. UX-01's "The automated part is a
surface diff against a docs diff", UX-03's "The automated part flags a
removed or renamed surface with no alias or deprecation" and ORCH-03's "The
automated part is a config-key diff against a docs diff" stay word for word.

**Severity and check:** MUST, `automated + agent-judgment`, before and after,
for all three.

**Enforcement change:**

- `tools/tier1/rules.yaml` is not edited for this change: `surface.dir:
  cmd/warden/` and `surface.docs` stay as they are. FLOW-14's check builds
  tier 1 from the merge base's code with the branch's `rules.yaml`, and that
  code rejects a key it does not know, so renaming `surface.dir` or adding a
  key would make `enforcement-check.sh` exit 2 on every run of the PR.
- `tools/tier1/surface/surface.go`: besides flag calls and `toml` tags in
  `surface.dir`, `Check` reads driver-declared options. It loads, at the
  base and at HEAD, the non-test Go files of `surface.dir`, of `driver/`
  and of each directory directly under `driver/` (found by listing
  `driver/` at that revision; the root is a constant in `surface.go`). In
  them `Extract` reads composite literals of `driver.Option` (`Option`
  inside package `driver`), including the elements of a `[]driver.Option`
  literal, whose type is elided: a literal with a non-empty `Flag` yields
  flag `Name`, and one with `Config: "table.key"` yields key `key`, the
  same identity a `toml` tag gives today. Nothing else is read from the
  driver directories. The check runs when the diff touches `surface.dir` or
  `driver/`. Tests and a fixture
  (`tools/tier1/surface/testdata/driver.go.txt`) cover the new form.
- Effect on findings. The branch's tier 1 extracts the same set of flags
  and keys at the base (all in `cmd/warden`) and at HEAD (split between
  `cmd/warden` and the drivers' option literals), so it reports nothing.
  `main`'s tier 1 reads the branch's `rules.yaml`, which it can parse, and
  sees `cmd/warden` only. It reports `surface-removed` (UX-03) for each
  moved flag and key whose name no longer appears as a string literal in
  `cmd/warden`; the check takes a surviving literal for an alias, so a
  common word such as `name` or `force` may go unreported. It reports no
  `surface-docs` finding (UX-01, ORCH-03), because `cli.md` and
  `configuration.md` are in the diff and that check passes whenever a
  reference doc is touched. So the findings the branch drops are all on
  UX-03, and that entry clears them; the UX-01 and ORCH-03 entries clear
  the edit to the file, not a finding. After K1 a driver's new flag or key
  is checked against the docs exactly as a `cmd/warden` flag is.

The other two entries edit `rules.yaml` within its present format (`seams`
gains an entry and `drivers.shared` four paths), which `main`'s tier 1
reads without change.

## Security note

**Boundary:** environment to relay, declared conservatively. K1 changes how
each driver finds the relay it starts, and the relay is the build
environment's only network path.

**How the posture changes:** it does not, in K1.

- Every driver resolves the same relay artifact from the same places as on
  `main`; the relay source wraps the lookups and adds none. No file in
  `relay/`, `cmd/relay`, `relaycfg/`, `collector/` or `cmd/warden-io`
  changes, and no claim field, token or boot-channel value moves (RELAY-08,
  BOOT-06).
- No isolation code changes: the container driver's holder and iptables
  sidecar, the VM drivers' topology, firecracker's slots and posture checks
  and Hyper-V's switches are not edited beyond the function that receives
  the resolved relay.
- `Relay.Verify` is new but inert: no source in K1 supplies a hash, so it
  refuses nothing. When M6 pins releases it is the point where a tampered
  or wrong relay is refused before it starts, and it must refuse rather than
  fall back.
- The two existing overrides let the user run a relay of their choosing, as
  they do today. They are read from the user's own configuration and
  environment, which already decide what `warden` runs.
- Drivers are compiled in from a list. Nothing is loaded at run time, and no
  driver is found by searching a path; that question is K2's, with the
  dispatcher.
- An integrator's driver runs with the host user's privileges, like any
  driver. The contract and `drivertest` do not make one safe: `drivertest`
  checks lifecycle parity, and the isolation guarantee is checked by S3's
  build-link suite. The contract document says so.

**What the TEST-08 suite should exercise:** nothing new for K1 itself; the
existing isolation and measurement cases are the regression check that the
move changed nothing, on every driver. For the relay source, two cases for
when hashes exist (M6): a relay file whose bytes do not match its published
hash is refused before any VM or container starts, and an unusable override
is an error, never the bundled relay.

## Validation plan

**Tier 0** (`make test`, every platform, TEST-01):

- The characterization goldens (help, refusals, request tables, image list
  and verify), written before the refactor and diffed across it.
- `drivertest.Contract` for each of the five drivers, on Linux, macOS and
  Windows runners, through `cmd/warden`'s test over `drivers`.
- The frame's tests against `drivertest.Recorder`: option registration and
  the union of flags, a per-command clash refused and the same name on
  different commands accepted, precedence (flag, environment, project file,
  user file), kind errors at load for every linked driver's keys and at
  resolution for flags, refusal of an undeclared flag, what each command
  passes, and the recorder-only frame.
- `driver/relaysource`: one place or the other and never mixed; an override
  used exclusively; `Verify` on a matching and a mismatching hash.
- The imports test (no driver's package imported from outside its
  directory), `drivertest.ClaimsWithRequestLink` in qemu, firecracker and
  vz, the Hyper-V default-NAT disjointness test in `driver/hyperv`, and the
  tier-1 surface extraction tests.
- The `drivertest`-tagged tests compile on every platform (`make test`
  compiles them without running any).
- Each driver's existing unit tests, unchanged, plus the tests that move
  with code from `cmd/warden`.

**Tier 1** (the `integration` workflow, Docker): `make integration-test` as
today, and `drivertest.Lifecycle` for the container driver.

**Tier 2:**

- firecracker: the `firecracker` workflow on its hosted KVM runner, which
  this change triggers (`driver/firecracker/**`, `driver/*.go`,
  `cmd/warden/**`). It gains `drivertest.Lifecycle`, `warden clean --driver
  firecracker`, and `image list` and `image verify` after its builds. No
  native step (a7). The workflow is not a required check, so Publish
  confirms that its run on the PR's head passed.
- Native, blocking (FLOW-09), each on the branch after consolidation, with
  `warden doctor` output captured on `main` first for comparison:

| Host | Cases |
|---|---|
| macos-vz | (1) `warden doctor --driver vz` prints what `main` prints and exits the same. (2) `sudo warden image prepare --driver vz` refreshes the active image; `image list` shows the vz lines after qemu's; `image verify --driver vz` passes. (3) A vz build succeeds, its ledger verifies, `relay.log` is written. (4) `make drivertest DRIVER=vz` passes. (5) `--runtime docker` and `--network x` with `--driver vz` are refused with the generated message; `warden clean --driver vz` reports nothing to clean and needs no container runtime; `warden setup --driver vz` prints the no-setup line |
| windows-hyperv | (1) `warden doctor --driver hyperv`, elevated and not, prints what `main` prints; `--network` is honoured. (2) Elevated `warden setup --driver hyperv --slots 2` is idempotent and honours `--nat-prefix` and `--host-ip`. (3) `warden image prepare --driver hyperv` with no source refreshes; `image list` and `image verify --driver hyperv` pass. (4) A non-elevated durable build and an elevated ephemeral build succeed and verify; `[vm] network` is honoured and `--network` overrides it; with `[vm] image_refresh_after` set low, the detached `image prepare --driver hyperv` starts and logs. (5) `WARDEN_RELAY_BOOT_VHDX` naming a copy of the boot disk is used; naming a missing file is refused, with no fallback to the cache. (6) `warden hyperv service install`, a service-delegated build, `uninstall`. (7) `warden clean --driver hyperv` removes a `--keep-vms` build's leftovers and leaves a running build's. (8) `make drivertest DRIVER=hyperv` passes |
| qemu-any | (1) `warden doctor --driver qemu` prints what `main` prints. (2) `warden image prepare --driver qemu alpine:3.21`; `image list`; `image verify --driver qemu`. (3) A direct-boot build and a cloud-image build succeed and verify, with the relay VM's assets resolved from the source tree; then again with `tools/relay-vm/output/vmlinuz` and `initramfs.cpio.gz` copied beside `dist/warden` as `relay-vm-vmlinuz` and `relay-vm-initramfs.cpio.gz` (`make build` puts only the relay binary there), which resolves them from beside the binary. (4) `make drivertest DRIVER=qemu` passes. (5) `warden clean --driver qemu` reports nothing to clean with no container runtime running |
| macos-finch | (1) `warden doctor` prints `container runtime: finch` and `ok`. (2) A default build and a `warden shell` session work; the shell's exit code is passed through. (3) `[runtime] relay_image = "dev"` builds the relay from source; `WARDEN_IMAGE=<ref>` pulls that image; an unpullable reference fails without trying the default. (4) `warden clean` removes the containers and network of a killed build and leaves a running build's. (5) `WARDEN_CTR_CLI` and `--runtime` select the runtime; a missing one gives the fixable error. (6) `make drivertest DRIVER=container` passes on finch |

- Native, tracked: macos-qemu. The qemu Windows prepare moves from
  `cmd/warden` into `driver/qemu`. Its logic keeps its unit tests, but an
  end-to-end run needs an install ISO and hours, and that host's open
  backlog entry (`qemu-windows-guest-package`) shows a Windows build is not
  yet reliable there. Recorded in `tier2_gaps` and as backlog entry
  `driver-contract-qemu-windows` (TEST-03).

A failure that `main` already has is filed as a follow-up and does not hold
this item (FLOW-09).

## Implementation plan

Plan refines these into tasks; the order is **Order of the refactor**.

1. Add the characterization tests to `cmd/warden` on unchanged production
   code: help goldens, the refusal table, request tables per driver, `image
   list` and `image verify` fixtures (GLOBAL-04, UX-09, TEST-01, TEST-04).
2. Write `docs/book/src/contributing/driver-contract.md` (the contract in
   prose, the relay source, `drivertest`, what it does not guarantee), then
   add `driver/contract.go`, `driver/options.go` and `driver/relay.go` to
   match it, with doc comments on every exported identifier (GLOBAL-01,
   GLOBAL-08).
3. Add `driver/relaysource` (`Bundled`, `Path`, `Image`) over the existing
   lookups, with `Relay.Verify`, wrapped errors and fixable refusals
   (GLOBAL-03, GLOBAL-10, GLOBAL-11, UX-04, RELAY-08).
4. Change tier 1's surface extraction as approved, with its tests; leave
   `rules.yaml`'s `surface` keys as they are (UX-01, UX-03, ORCH-03,
   FLOW-14, LINT-05).
5. Give `driver.Driver` its final method set and add `driver/drivertest`:
   `Contract`, `Recorder`, `ClaimsWithRequestLink`, then `Lifecycle` behind
   nothing that starts a VM under `make test` (ORCH-01, TEST-01, TEST-04).
6. In the same commit as task 5, rewrite the frame in `cmd/warden` over
   `[]driver.Driver` with `drivers.go` and the temporary `legacy.go`: option
   registration with its clash test, the kind check of every linked
   driver's keys at load, resolution, the generated lists and refusals, the
   generic `doctor`, `setup`, `image`, `clean` and driver command mounts.
   Update the goldens for the seven differences only (ORCH-03, UX-03, UX-04,
   UX-07, UX-08, UX-09, GLOBAL-10).
7. Convert the container driver: `Info`, `Doctor`, `Cleaner`, its options,
   the runtime detection and cleanup moved in, its relay source; delete its
   adapter (ORCH-01, ORCH-04, BOOT-07, GLOBAL-07).
8. Convert the firecracker driver: `Info`, `Doctor`, `HostSetup`, `Images`,
   `Cleaner`, its asset list and reserved range, and its relay source over
   the one three-file lookup; `relayvm.RelayInitrd` takes the relay binary's
   path (ORCH-01, ORCH-04, GLOBAL-07).
9. Convert the qemu driver: `Info`, `Doctor`, `Images` with the Windows
   prepare moved in, its asset lists and reserved range, the freshness
   options, its relay source (ORCH-01, GLOBAL-07, UX-04).
10. Convert the vz driver: `Info`, `Doctor`, `Images` with the macOS prepare
    and list formatting moved in, the main-thread pin, a `New` that does no
    host lookup, its relay source (ORCH-01, GLOBAL-07, UX-04).
11. Convert the hyperv driver: `Info`, `Doctor`, `HostSetup`, `Images`,
    `Cleaner`, `Commander`, its twelve options, its relay source with the
    environment override (ORCH-01, ORCH-04, ORCH-05, GLOBAL-07).
12. Remove the fields `Network`, `ImageRefreshAfter`, `ImageMinGrace` and
    `RelayImage` from `BuildRequest` and the driver-owned fields from
    `Config`, each in the commit that converts its last reader (ORCH-01,
    ORCH-03).
13. Delete `legacy.go`; move the driver names out of `driver/relayvm` and
    `driver/subnet.go`; split `disjoint_test.go` (the frame's test, and the
    Hyper-V NAT case in `driver/hyperv`) and replace `claimlink_test.go`
    with the three drivers' calls to `drivertest.ClaimsWithRequestLink`; add
    the imports test and the `drivertest.Contract` test over `drivers`
    (ORCH-01, TEST-01).
14. Add each driver's `Lifecycle` test behind the `drivertest` tag, `make
    drivertest`, the tagged compile in `make test`, `-tags drivertest` in
    `make deadcode`, and the additions to the `integration` and
    `firecracker` workflows (TEST-02, TEST-07, ORCH-01, FLOW-15).
15. Make the approved rule edits: GLOBAL-08 in `global.md`, ORCH-01 in
    `orchestrator.md`, and `seams` and `drivers.shared` in `rules.yaml`
    (FLOW-14, GLOBAL-08, ORCH-01).
16. Bring the contract document up to date with what was built and link it
    from `SUMMARY.md` and `structure.md` (GLOBAL-08, UX-01).
17. Update `guide/cli.md` and `guide/configuration.md` for the seven
    differences and the `[driver.<name>]` convention,
    `concepts/overview.md` and `contributing/structure.md` for the new
    shape, and `CONTRIBUTING.md` for `make drivertest` (UX-01, FLOW-15).
18. Append the tracked gap to the native-gap backlog if Design has not
    (TEST-03, FLOW-09).

## Design review

None yet.

## Consolidation

## Rollback notes

## Archive summary

---
feature: driver-contract
status: draft
drivers: [interface]     # the build sequence moves from every driver into warden, and every
                         # driver/<name>/ tree is reached
seams: [driver-contract, relay-claim, collector, bootstrap-model]
                         # driver-contract: new, the commands warden runs on a driver binary
                         # (GLOBAL-08). relay-claim and collector, conservatively: neither wire
                         # changes, but the claimant and the collector's owner become warden
                         # for every driver. bootstrap-model: BOOT-02's wording (BOOT-07).
surfaces: [cli, config, output]  # cli: `warden driver list`, a path in --driver, per-driver flags
                         # and help; config: [driver.<name>]; output: <output>/driver.json
security_boundary: host-env  # the code that builds the isolation is chosen at run time by a
                         # lookup. Also env-relay: the driver starts the relay warden claims.
alters_relationships: true
design_review: required
native_validation:
  - {host: macos-finch, blocking: true, reason: "the container driver becomes a separate binary run through the commands; finch is the default runtime CI does not run"}
  - {host: container-any, blocking: true, reason: "the exec transport on a Windows host (lookup of a .exe, pipes, kill, reap) and the container driver's clean with keep_ids in place of the ps scan; only this host exercises them with a real runtime"}
  - {host: macos-vz, blocking: true, reason: "driver/vz is reached: its build moves onto warden's build sequence (warden stages, starts the collector and claims)"}
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv is reached: StartRelay hands its session and claim to warden, and the build-exit grace changes from 5 s to 10 s"}
  - {host: qemu-any, blocking: true, reason: "driver/qemu is reached: its build moves onto warden's build sequence"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows build through warden's build sequence; the host's open backlog entry (qemu-windows-guest-package) shows a Windows build is not yet reliable there"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows build (--guest-os windows) through warden's build sequence: warden stages build.ps1, starts the collector and claims (backlog driver-contract-qemu-windows)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10,
        GLOBAL-11, ORCH-01, ORCH-02, ORCH-03, ORCH-04, ORCH-05, RELAY-04, RELAY-07, RELAY-08,
        BOOT-01, BOOT-02, BOOT-03, BOOT-04, BOOT-06, BOOT-07, UX-01, UX-02, UX-03, UX-04, UX-05,
        UX-07, UX-08, UX-09, TEST-01, TEST-02, TEST-03, TEST-04, TEST-05, TEST-07, TEST-08,
        LINT-05, FLOW-01, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-10, FLOW-12,
        FLOW-14, FLOW-15, FLOW-17, FLOW-19, FLOW-20]
rule_changes:
  - {rule: GLOBAL-08, change: amend, paths: [docs/development/global.md, tools/tier1/rules.yaml]}
  - {rule: ORCH-01, change: amend, paths: [docs/development/orchestrator.md, tools/tier1/rules.yaml]}
  - {rule: ORCH-02, change: amend, paths: [docs/development/orchestrator.md]}
  - {rule: ORCH-04, change: amend, paths: [docs/development/orchestrator.md]}
  - {rule: TEST-05, change: amend, paths: [docs/development/testing.md, tools/tier1/rules.yaml]}
  - {rule: BOOT-02, change: amend, paths: [docs/development/bootstrap.md]}
  - {rule: UX-01, change: amend, paths: [tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt]}
  - {rule: UX-03, change: amend, paths: [tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt]}
  - {rule: ORCH-03, change: amend, paths: [tools/tier1/surface/surface.go,
      tools/tier1/surface/surface_test.go, tools/tier1/surface/testdata/driver.go.txt]}
concepts: [driver-contract, driver-binaries, driver-commands, driver-discovery, driver-conformance,
           driver-options, driver-owned-config, kit, drivercmd, build-sequence, session, claim,
           collector, collector-handoff, relay-source, relay-release-forms, reap, doctor,
           host-setup, image-prepare, clean, shell, componentization, program]
related: [componentization.md, unified-bootstrap.md, firecracker-driver.md, build-link-subnet.md,
          tier1-static-checks.md, hyperv-relay-per-build-channel.md, hyperv-driver-plan.md,
          vm-driver-release-plan.md, platform-compatibility.md, ledger-environment-types.md,
          hyperv-windows-prepare.md, vz-image-prepare-refresh.md, hyperv-clean.md,
          guest-binary-cache-per-tree.md, design-approved-rule-changes.md, design-principles.md,
          container-inputs.md, specs-migration.md]
---

# The driver command contract: `warden` runs a driver binary (componentization K1)

This is a program item (FLOW-20): phase K1 of
[`componentization`](componentization.md). It is the second draft. The first
designed a Go API that every driver links; the design review of 2026-11-14
sent it back (**Design review**), and this draft is written within the basis
agreed there:

1. The contract is the set of **commands** `warden` runs on a driver binary,
   not a Go API. A driver is its own program, found the way git finds a
   subcommand, so an existing `warden` runs a new driver without being
   rebuilt. A Go kit helps; a driver may ignore it and be written in any
   language.
2. **Primitives only.** A driver creates the isolated environment and starts
   the relay. `warden` keeps the session, the collector and the reporting.
3. **The collector is pluggable.** K1 builds only the default one (on the
   host, one build). It leaves an existing local or remote collector
   possible and does not design it.
4. The conformance suite runs against a driver **binary**.

The basis reverses the umbrella's Decision 4 ("drivers as RPC plugins"
rejected) and rewords its Decision 5 and seam 9. The umbrella is merged, so
this design proposes the new wording under **Amendments to the umbrella
design** and asks for Jeff's word on it (a18). The freeze and the program's
scope stay FLOW-20's.

It is not a move-only change in FLOW-20's sense: it adds a protocol and
moves the build sequence out of the drivers. The behaviour that changes is
listed under **Behaviour** (a5).

The delta from the first draft is not small. The Problem's inventory, the
options table, the per-driver operations and the relay source stand; the
Decisions, the contract, the suite, the collector hand-off, the rule
changes, the security note and the plan are new. It needs a review from
scratch.

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

The build itself is inside each driver. Every `StartBuild` stages the build
script, starts the collector (`driver.StartSession`), claims its relay
(`Session.ClaimRelay`), waits (`Session.Wait`) and finalizes the output, in
the driver's own process. A driver is therefore a Go package linked into
`warden`: it cannot be added to an installed `warden`, and it cannot be
written in another language.

So an integrator who wants a driver must fork `cmd/warden` and rebuild it,
and the Rust relay has nowhere to be adopted. GLOBAL-08 says the first on
purpose: `driver.Driver` is "not covered by this obligation". The umbrella
reverses that, and the design review asked for more than the first draft
gave: a driver as its own binary that an unmodified `warden` finds and
runs. K1 is the item that makes that true for one real driver (container)
and proves the build dialogue on all five.

## Prior art

- [`componentization`](componentization.md): the umbrella. Decisions 6, 7,
  8 and 18 and its answer a2 bound this design and are not reopened.
  Decisions 4 and 5, seam 9 and the K1, K2 and S4 rows are reworded by the
  review's basis; the wording is proposed under **Amendments to the
  umbrella design**. Its Consolidation records what #115 changed, which
  **Design** describes as `main` has it.
- `cmd/collector` (in the tree, no design of its own): a claimant outside
  `warden`'s process, given `-relay host:port`. It shows that the claim and
  the collector already work from a process that did not start the relay,
  which is what `warden` becomes for every driver.
- `seam-specs` (S1, in review on `feat/seam-specs`, not on `main`): the
  specs repository, the harness that plays one side of a seam against a
  binary, and specifications with numbered requirements. The driver suite
  takes its shape (a program run against a binary, skips derived from what
  the target declares, a reference target for self-test). At `618ae78` it
  already writes the collector's lifetime per session token, which the
  review's basis asked for; nothing more is proposed to it here.
- [`firecracker-driver`](firecracker-driver.md) added the fifth driver by
  editing every file in the table above, and moved the qemu relay-VM and
  Linux-guest code to `driver/relayvm` and `driver/linuxguest`. Its slot pid
  file (pid, start time, executable, keep flag) is the model for what a
  `reap` needs to find a killed build's processes. Its Decision 4 (durable
  slots made by `warden setup`) is why setup and doctor are commands of the
  contract. Its Decision 8 (a build may complete up to 10 s after its VM
  exits) becomes `warden`'s rule for every driver.
- [`hyperv-driver-plan`](hyperv-driver-plan.md): the `Provisioner` boundary
  and the privileged service, the one driver-specific command group
  (`warden hyperv service`). Reused as the model for `ext`.
- [`build-link-subnet`](build-link-subnet.md) and
  [`hyperv-relay-per-build-channel`](hyperv-relay-per-build-channel.md):
  the build link and resolvers as claim fields, `driver.LauncherStamp`,
  `driver/resolvers`, and RELAY-08. The contract carries the link and the
  resolvers from the driver to the claim unchanged; no relay setting is
  added (RELAY-08).
- [`unified-bootstrap`](unified-bootstrap.md): the bootstrap model. The
  order it fixes (relay, claim, environment) is the order of the `run`
  messages. BOOT-02's sentence on the executor's version changes
  (**Bootstrap comparison**).
- [`container-inputs`](container-inputs.md): `driver.RecordInputs` and
  ORCH-02's second paragraph. Staging and recording move from the drivers
  to `warden`, with the same files and names.
- [`tier1-static-checks`](tier1-static-checks.md): `drivers-claim`,
  `surface-docs`, `surface-removed` and `seam-contract`. K1 changes the two
  surface checks' input (Rule changes) and gives `seam-contract` an entry.
- [`specs-migration`](specs-migration.md): `specs/<name>/v<N>.md` beside
  `v<N>.schema.json`, checked by `schema-doc`. The contract's specification
  takes that form.
- [`design-approved-rule-changes`](design-approved-rule-changes.md): the
  route for this design's nine rule-change entries (FLOW-14).
- [`vm-driver-release-plan`](vm-driver-release-plan.md) (relay VM
  distribution), [`guest-binary-cache-per-tree`](guest-binary-cache-per-tree.md)
  (`driver.GuestBinary`'s lookup): the relay lookups the relay source wraps,
  unchanged.
- [`ledger-environment-types`](ledger-environment-types.md) (`LayerCache`,
  `image verify`), [`hyperv-windows-prepare`](hyperv-windows-prepare.md),
  [`vz-image-prepare-refresh`](vz-image-prepare-refresh.md),
  [`hyperv-clean`](hyperv-clean.md): the image and clean behaviour, which
  K1 leaves where it is for the four VM drivers.
- [`platform-compatibility`](platform-compatibility.md): which driver runs on
  which host. A driver binary exists only for the platforms it runs on
  (GLOBAL-07 applies to each binary).
- [`design-principles`](design-principles.md): GLOBAL-09 to GLOBAL-11, applied
  under **Defaults and overrides**.
- `remote-execution-and-measured-hermeticity` (a draft proposal) touches a
  remote backend; it is not relied on, and nothing here designs toward it.

Outside the repository, the contract borrows from tools that find and drive
plugin binaries. What each contributed:

| From | Taken |
|---|---|
| git subcommands | The name convention, and its own directory searched before `PATH`, which is what keeps a planted `git-commit` from replacing a core command |
| kubectl plugins | A `list` that shows the resolved path and every shadowed copy |
| Docker CLI plugins | One mandatory metadata command; end of input on a host-held channel as the cancel and host-death signal. Not taken: running every candidate's metadata command to draw help |
| CNI | A supported-versions list the host picks from; an error object with reserved codes; teardown that succeeds when nothing exists and may be repeated; a garbage-collection verb given the live set |
| containerd shim v2 | A separate cleanup command, given only the ID, that the host runs when it cannot reach the process that held the environment |
| csi-sanity, OCI runtime-tools | A suite run against a binary, whose skips come from what the binary declares, so "declared but broken" and "implemented but undeclared" both fail |
| go-plugin, Terraform | Not taken: gRPC (it ends "any language, no library"). Its known failure, a stray line on stdout breaking the handshake, is a suite check here |

Every design relied on already has front-matter (FLOW-12).

## Decisions

1. **The contract is commands, not a Go API.** A driver is a program named
   `warden-driver-<name>` with three required commands (`describe`, `run`,
   `reap`) and optional ones (`doctor`, `setup`, `image-prepare`,
   `image-list`, `image-verify`, `clean`, `attach`, `ext`). Requests and
   results are JSON. Alternatives: the first draft's Go interfaces linked
   through a list (an existing `warden` cannot run a new driver, and a
   driver must be Go; rejected in the review); gRPC or another RPC framework
   over a socket (a plugin host and a generated client in every language,
   where argv, JSON and the standard streams are enough).
2. **One `run` process per build; every other command runs once and
   exits.** The `run` process lives as long as the environment does, and
   closing its input means "tear down". Alternative: run-and-exit verbs
   (`create`, `start`, `state`, `stop`) as in an OCI runtime. It is nearer
   the word "commands", but vz's VM is an object inside the process that
   created it, so vz would need a holder daemon; and a killed `warden` would
   leave VMs running with nothing watching them (a14).
3. **Primitives only: `warden` keeps the session.** `warden` stages the
   build script, records altered inputs, starts the collector, sends the
   claim, waits and reports. The driver starts the relay, says where to
   claim it, and starts the environment when told. Alternative: the driver
   runs the whole build (every driver would carry the session and a
   collector, and the collector could not be chosen apart from the driver;
   ruled out by the basis).
4. **`warden` starts the default collector and sends the claim; the driver
   is handed nothing about the collector.** The driver's wiring is to make
   its relay claimable by `warden`: it reports a claim endpoint, and
   `warden` dials it with today's `Session.ClaimRelay`. Alternatives: the
   driver starts the collector, as Jeff's note first had it (every driver,
   in every language, would embed a collector, and `warden` would need a
   second protocol to reach it) (a12); the driver sends the claim, given an
   endpoint and a token (the default collector would have to listen where
   the relay can reach it, which `driver/session.go` rules out on purpose,
   or the driver would have to bridge the tunnel for the whole build)
   (a13). The second stays addable: `start` reserves a `collector` field.
5. **Lookup: an explicit path, then `warden`'s own directory, then `PATH`;
   the five bundled names never come from `PATH`.** Alternative: `PATH`
   only, as Jeff's note described (a file named `warden-driver-container`
   in any earlier `PATH` directory would replace the bundled driver, and
   `sudo warden` would not find a driver installed beside it) (a16).
6. **No auto-detection of the driver.** Selection is unchanged: `--driver`,
   `WARDEN_DRIVER`, `[runtime] driver`, else `container`. A named driver
   that is not found is an error, never the default (GLOBAL-10).
7. **A driver declares its options as data in `describe`, and `warden`
   parses them.** `warden` alone reads the files, the environment and the
   flags, with today's precedence (ORCH-03), and hands the driver resolved
   values. A driver needs no TOML parser. Alternative: hand the driver the
   raw files and argv (every driver re-implements precedence, and tier 1
   cannot read the surface).
8. **`capabilities.vm` is the one switch for the shared VM settings.** The
   VM drivers take `--script`, `--image`, `--cpus`, `--memory`,
   `--keep-vms`, `--build-link`, `[vm] cpus`, `memory_mb`, `build_link` and
   `dns` as a group today. They stay `warden`'s flags and keys, sent to a
   driver that declares `vm`. Alternative: each VM driver declares them as
   its own options (a driver from outside could not take `[vm] cpus` under
   Decision 9, and every refusal message would change) (a9).
9. **Driver-owned configuration: existing keys stay where they are; a new
   or outside driver's keys go in `[driver.<name>]`.** UX-03 forbids moving
   the seven existing keys without aliases, and K1 changes no key.
   Alternatives: move them all now with deprecated aliases (a user-facing
   change in a freeze); let a driver declare any table (collisions with
   `warden`'s tables) (a4).
10. **The kit is a Go library a driver may ignore.** `driver/drivercmd`
    dispatches the commands, builds `describe`, and offers the build
    dialogue as `driver.Host`. The reference fake is written without it, to
    show that a driver needs none.
11. **The relay source is a kit library each bundled driver resolves its
    relay through; it is not part of the contract.** A driver from outside
    finds its relay however it likes. K1 adds no user-facing relay
    override. The form and role names are K1's own and provisional: seam 8
    is not written (a3, a11).
12. **The conformance suite is a program run against a binary**, in two
    tiers: a static tier with no hypervisor, on hosted runners for every
    PR, and a lifecycle tier on the driver's own host. It imports nothing
    under `driver/`. Alternative: Go test helpers a driver's package calls,
    as in the first draft (only a Go driver built on the kit could run
    them).
13. **The specification and the suite start in this repository** and move
    to the specs repository when S1 creates it. Alternative: K1 waits for
    S1 (a17).
14. **K1 delivers the contract, the suite, the kit, a fake, `warden`'s
    calling side and the container driver as a binary. All five drivers
    move onto `warden`'s build sequence; the four VM drivers stay linked
    until K2.** Alternatives: container only, VM drivers untouched (two
    build sequences in `warden` until K2, and the dialogue unproven on vz
    and Hyper-V, the two hard cases); all five as binaries now (a Windows
    release target, the vz entitlement moved to a new binary and the
    Hyper-V service re-registered, in one PR with five native
    validations); all five "behind the commands" but shipped inside the
    `warden` file (they would not be separate binaries, and the service
    manager cannot start a service through a child of `warden`) (a15).
15. **A driver that is found is recorded, not vetted.** `warden` writes the
    driver's name, version, path and file digest to `<output>/driver.json`,
    unsigned. It cannot check that a driver it did not ship isolates the
    build or runs the project's relay. Alternatives: an opt-in key before a
    driver found on `PATH` runs; the record in the signed ledger (a schema
    version, for a value the relay cannot attest) (a19).
16. **Dropped fallback: `warden clean` for a driver with no `clean` command
    no longer runs the container runtime's cleanup.** Today `--driver qemu`
    and `--driver vz` fall through to it. Keeping it would have `warden`
    run one driver's command for another (a5, GLOBAL-11).
17. **One queue item and one PR, in ordered commits, each green.**
    Alternative: one item per step (the calling side cannot be reviewed
    without a driver that answers it) (a15).

## Questions

### Blocking

None. The review comes before Plan (FLOW-19), and every question below has
a recommended answer the design is complete under.

### Assumed

The first draft's a1 to a11 were never answered. Four are retired because
the redraft makes them moot (listed last); seven survive, reworded where
the new contract requires it; a12 to a20 are new. No retired ID is reused.

- **a3. The relay source's scope in K1.** Proceeding with: the relay source
  is a kit library and not part of the contract (Decision 11). K1 ships its
  types and the bundled, source-tree, local-path and image sources that
  wrap today's lookups, routes the two existing overrides through them
  (`[runtime] relay_image` with `WARDEN_IMAGE`, and
  `WARDEN_RELAY_BOOT_VHDX`), and adds no flag or key. The container driver
  uses it in K1; the four VM drivers adopt it in K2 with their binaries. A
  pinned source (download a release form by version and verify its
  published hash) waits for seam 8 (a11). Options: (a) as stated; (b) also
  route the four linked VM drivers through it in K1; (c) also add `--relay`
  and `[driver.<name>] relay` now; (d) also build the pinned source now,
  unused until M6. Recommended: (a). Confirm.
- **a4. Where a driver's own configuration lives.** Proceeding with: the
  seven existing driver-owned keys keep their tables and names; a key a
  bundled driver adds from now on, and every key of a driver from outside,
  lives in `[driver.<name>]`; `warden` refuses a `describe` that declares
  any other table for a name that is not bundled, and the suite's static
  tier checks it. Options: (a) as stated; (b) move the existing keys to
  `[driver.<name>]` now, with deprecated aliases; (c) let a driver declare
  any table. Recommended: (a). Confirm.
- **a5. The behaviour that changes.** Proceeding with the twelve differences
  listed under **Behaviour**. In short: flags and help follow the selected
  driver; a flag the selected driver does not declare is refused, where
  eleven were silently ignored; refusals take one generated form; "unknown
  driver" becomes "driver not found", with what was searched; `warden clean
  --driver qemu|vz` no longer cleans containers; the build-exit grace is
  10 s on every driver (it was 5 s on container and Hyper-V); a second
  Ctrl-C kills the driver and reaps, where it killed `warden`; the output
  directory gains `driver.json`; `warden driver list` is new; a path is
  accepted in `--driver` and `WARDEN_DRIVER`; a malformed `[vm]
  image_refresh_after` or `image_min_grace` is refused only when qemu or
  hyperv is selected, and then on every command; `--timeout` counts from
  the moment `warden` starts the driver.
  Options: (a) accept all twelve; (b) keep every command's flags and help
  as the union of the five bundled drivers', as today, and refuse nothing
  new; (c) accept some, named in the review. Recommended: (a). Confirm.
- **a7. Native validation.** Proceeding with macos-finch, container-any,
  macos-vz, windows-hyperv and qemu-any blocking, macos-qemu tracked, and
  linux-kvm left out. container-any is new: on a Windows host the exec
  transport (a `.exe` lookup, pipes, kill, `reap`) and the container
  driver's `clean` with `keep_ids` are exercised with a real runtime only
  there. linux-kvm: firecracker is validated by the `firecracker` CI
  workflow (tier 2 on a hosted KVM runner), which this change triggers.
  That workflow is not a required check, so Publish confirms its run on the
  PR's head passed. Options: (a) as stated; (b) container-any tracked,
  relying on tier 0's run of the fake on Windows runners; (c) macos-qemu
  blocking too; (d) add a blocking linux-kvm step; (e) the three VM hosts
  tracked, since those drivers stay linked (not recommended: their build
  path changes on every host). Recommended: (a). Confirm.
- **a8. The rule changes.** The umbrella's table gives K1 GLOBAL-08 and
  ORCH-01 "for direction". Proceeding with those two reworded for a command
  contract, plus ORCH-02 (staging moves to `warden`), ORCH-04 (teardown on
  end of input, and `reap`), TEST-05 (the driver suite), BOOT-02 (the
  executor's version is the driver's; with a Bootstrap comparison, since
  the wording differs from the one the umbrella's review accepted for K2),
  and an enforcement-only change to UX-01, UX-03 and ORCH-03 (tier 1's
  surface check also reads the bundled drivers' declared options). Options:
  (a) as stated; (b) leave BOOT-02 to K2 (the sentence "matches the
  orchestrator's version" is then false for the container driver from K1's
  merge until K2's); (c) no enforcement change, a `Tier1-Allow` trailer on
  the move, and the moved flags no longer checked against the docs.
  Recommended: (a). Each rule change is approved by name in the review, and
  the Bootstrap comparison is accepted explicitly (BOOT-07).
- **a9. `capabilities.vm` as the capability model.** Proceeding with one
  switch for the shared VM settings (Decision 8): they stay `warden`'s
  flags and keys and reach a driver that declares `vm` in `start.vm`.
  Options: (a) as stated; (b) one capability per setting; (c) each VM
  driver declares them as its own options. Recommended: (a). Confirm.
- **a11. Seam 8 (relay release forms) has no phase that writes it.** The
  umbrella lists seam 8 among the seams that get a specification, and says
  M6 needs K1's relay source; but S1 writes seams 1, 2, 3, 5 and 6, S3
  writes seam 7, seam 4 moves, and no phase is given seam 8. The relay
  source needs its vocabulary now (form names and file roles) and would
  need its manifest and hash format for a pinned source. Proceeding with:
  K1 defines the four form names and four file roles as Go constants of the
  kit library, marked provisional; they are not in the command contract; K1
  writes no seam 8 text, records no form in the ledger and builds no pinned
  source. Options: (a) as stated, with seam 8 filed as its own S-track
  item, to exist before M6 adopts a release; (b) seam 8's writing is added
  to S1's item; (c) K1 writes a first seam 8 text for the names it uses;
  (d) M6's first item writes it. Recommended: (a). This is a gap in the
  umbrella, raised here, not settled.
- **a12. Who starts the default collector?** Your note had the driver
  "instantiate an on-host endpoint that it wires in". The basis adjusted
  that, and this asks for your word on the adjustment. Proceeding with:
  `warden` starts today's in-process collector, one per build with a fresh
  token, and the driver starts none. Options: (a) as stated; (b) the driver
  starts the collector: every driver, in any language, embeds or ships a
  collector, and `warden` needs a protocol to it for console output, status
  and the output directory. Recommended: (a). Confirm.
- **a13. Who sends the claim?** The basis left it open. Proceeding with:
  `warden` dials the claim endpoint the driver reports and sends the claim,
  which is today's code and opens no listener on the host. The driver is
  then handed nothing about the collector, which is narrower than the
  basis's "an endpoint and a token". Options: (a) as stated; (b) the driver
  sends the claim, given the collector's endpoint and token: the default
  collector must then listen where the relay can reach it, which
  `driver/session.go` rules out on purpose (and firecracker's relay cannot
  open a connection to the host at all), or the driver bridges the tunnel
  for the whole build. Recommended: (a). (b) stays addable for a remote
  collector: `start` reserves `collector`, and `claim.network` is an
  enumeration. Confirm.
- **a14. One `run` process per build, or run-and-exit verbs?** Proceeding
  with one process per build that exchanges messages with `warden`
  (Decision 2). Options: (a) as stated; (b) `create`, `start`, `state`,
  `stop` verbs, each run once: nearer "commands", but vz needs a holder
  daemon, and a killed `warden` leaves VMs running. Recommended: (a).
  Confirm.
- **a15. K1's scope and done-when, against K2, K3 and S4.** Proceeding
  with Decision 14. K1 is done when an unmodified `warden` runs build,
  shell, doctor and clean through a driver binary it was not built with
  (the fake, in CI); the suite passes against the fake and
  `warden-driver-container`; `cmd/warden` names the four VM drivers only in
  `linked.go` and in their existing doctor, setup, image and clean code;
  and native validation passes on each blocking host. K2 makes each VM
  driver a binary and removes `linked.go`, then makes every driver a
  module. K3 keeps its row. S4 is absorbed, except the claim
  specification's transport section and a claim endpoint that is not TCP.
  One item and one PR. Options: (a) as stated; (b) container only, the VM
  drivers untouched: one blocking host fewer than five, and two build
  sequences in `warden` until K2; (c) all five as binaries now, with a
  Windows release target and the vz signing move; (d) as (a), split into
  two items at the point the fake passes. Recommended: (a); (b) is the
  fallback if K1 proves too large.
- **a16. Lookup and trust.** Proceeding with Decision 5 and the checks
  under **Discovery and trust**: a path in `--driver` or `WARDEN_DRIVER`;
  else `warden`'s own directory; else each absolute `PATH` entry; the five
  bundled names only from `warden`'s directory; no path from a config
  file; no batch files; on Unix a driver file writable by group or others
  is refused, and as root it must be owned by root or by the owner of the
  `warden` binary. Options: (a) as stated; (b) `PATH` only, as your note
  described: a planted bundled name wins; (c) as (a), and `PATH` is not
  searched at all when `warden` runs as root. Recommended: (a). Confirm.
- **a17. Where the contract's specification and suite live.** Proceeding
  with `specs/driver-contract/v1.md` and its schema, and
  `conformance/driver/`, in this repository, moved to
  `buildwarden/buildwarden-specs` by the item that follows S1's creation
  of it. Options: (a) as stated; (b) in the specs repository from the
  start, with K1 waiting for S1 and for Jeff to create the repository.
  Recommended: (a). Confirm.
- **a18. The umbrella's wording.** The basis changes sentences of a merged,
  archived design. Proceeding with the wording under **Amendments to the
  umbrella design**, made in this item's PR. Options: (a) as proposed, in
  K1's PR; (b) as proposed, in a process item of its own, with K1 waiting;
  (c) changed in the review, sentence by sentence. Recommended: (a). Your
  word is needed on each sentence. Nothing is proposed to `seam-specs`: its
  design already writes the collector's lifetime per session token.
- **a19. A driver from outside.** `warden` cannot check that a driver it
  did not ship isolates the build, or that it runs the project's relay and
  executor. Proceeding with Decision 15: accept that, say so in GLOBAL-08
  and the contract, and record the driver's name, version, path and file
  digest in `<output>/driver.json`, unsigned. Options: (a) as stated; (b)
  also require an opt-in key (`[runtime] allow_path_drivers = true`) before
  a driver found on `PATH` runs; (c) also record it in the ledger, at the
  cost of a schema version. Recommended: (a). Confirm.
- **a20. The contract's time limits and cleanup points.** Proceeding with:
  `describe` and a probe must answer within 10 s; after `stop`, `warden`
  waits 60 s for the driver to tear down, then kills it and runs `reap`; a
  build may still complete for 10 s after the driver reports its
  environment exited; `warden` runs `reap` after a driver that died, and
  `warden clean` runs it for every build whose lock nobody holds, but no
  sweep runs before each build. None is configurable. Options: (a) as
  stated; (b) also sweep (reap unheld builds) before every build; (c) make
  the 60 s a key. Recommended: (a). Whether 60 s covers a Hyper-V teardown
  is checked in native validation, since Hyper-V stays linked in K1.
  Confirm.

Retired by this redraft. Each is kept in the queue file, reworded as
retired, so the review confirms the retirement and no ID is reused:

- **a1** (what "adding a driver touches only its own directory" means): the
  done-when it explains is replaced. Asked now as a15.
- **a2** (the boundary with S4): K1 now settles who claims and absorbs most
  of S4. Asked now as a13 and a15.
- **a6** (one queue item or several): asked now as part of a15.
- **a10** (how far K1's GLOBAL-08 wording goes): an outside driver is a
  supported consumer from K1, so the sentence it held back is in K1's
  wording. Asked now as a8 and a19.

## Design

### Shape

```
  warden (cmd/warden)                         a driver: warden-driver-<name>
  -------------------                         ------------------------------
  config, flags, options       -- describe -->  what it is, what it takes
  stage the script, record inputs
  build ID, claim token, lock  -- run ------->  preflight; start the relay
  collector (in process)       <- relay-started: where to claim it
  tunnel + claim  ============================> relay
                               -- claimed --->  start the environment
  wait (collector signals)     <- environment-started, exited
  report, finalize             -- stop ------>  write diagnostics; tear down
                               <- stopped
  after a dead driver          -- reap <id> ->  remove what the build left
  doctor, setup, image, clean, shell, own commands: one process each
```

```
cmd/warden/                    the orchestrator: commands, config, options,
                               the build sequence, the session, reporting
  driverlookup.go              finding a driver binary (Discovery and trust)
  drivercall.go                the exec transport: running a driver's commands
  linked.go                    the four VM drivers, linked until K2; the only
                               file that names them besides their doctor,
                               setup, image and clean code
  buildseq.go                  one build sequence over either transport
cmd/warden-driver-container/   the container driver's binary (a main that
                               calls drivercmd.Main)
driver/                        the kit: Go libraries for a driver that wants them
  drivercmd/                   new: command dispatch, describe, the result
                               file, and the run dialogue as driver.Host
  relaysource/                 new: where a relay comes from
  session.go                   the session; called by warden, no longer by a driver
  hostutil.go, hostlock.go, buildlink/, resolvers/, script/, relayvm/,
  linuxguest/, winguest/       as today
  container/ qemu/ vz/ hyperv/ firecracker/   the drivers
specs/driver-contract/         v1.md and v1.schema.json: the contract
conformance/driver/            the suite (driver-conformance), the reference
                               fake (warden-driver-fake) and stubrelay
```

The contract is `specs/driver-contract/v1.md`. Everything under `driver/`
is a convenience for a driver written in Go. `warden`'s side of the
contract lives in `cmd/warden`, and the suite has its own implementation of
that side, so the two are checked against the specification and not
against each other.

Two terms used below. A **bundled** driver is one of the five in-tree
names (`container`, `qemu`, `vz`, `hyperv`, `firecracker`). A **linked**
driver is a bundled driver compiled into `warden`: the four VM drivers in
K1, none after K2. `warden` reaches a linked driver through the same
sequence and the same messages, as Go calls and not over a pipe.

### Discovery and trust

**Name.** A driver binary is `warden-driver-<name>`. `<name>` matches
`[a-z][a-z0-9]{0,31}` and is not the name of a `warden` command. On Windows
the only candidate is `warden-driver-<name>.exe`: `PATHEXT` is not
consulted, so no `.bat` or `.cmd` file is ever run.

**Selection** is unchanged: `--driver`, else `WARDEN_DRIVER`, else
`[runtime] driver`, else `container` (`resolveConfig`,
`cmd/warden/main.go`). There is no auto-detection. `image verify` keeps
reading the flag only.

**Lookup**, first match wins, resolved once per `warden` invocation to an
absolute path, which every later command of that invocation runs:

1. A `--driver` or `WARDEN_DRIVER` value that contains a path separator is
   a file. It is used as given and nothing else is searched (GLOBAL-10).
   The driver's name is what its `describe` reports, which must match the
   file's base name (`warden-driver-<name>`, with `.exe` on Windows). A
   config file may never name a path: `warden.toml` is read from the
   working directory, which may be a build context someone else wrote.
2. A linked driver, when the name is one (K1: `qemu`, `vz`, `hyperv`,
   `firecracker`).
3. The directory of `warden`'s own executable (`os.Executable()`, symlinks
   resolved). Bundled drivers ship here, so `sudo warden` still finds them
   with sudo's reset `PATH`.
4. Each absolute entry of `PATH`, in order. Relative entries and the
   current directory are skipped.

A bundled name resolves at step 1, 2 or 3 only, never from `PATH`. A name
that is not found is an error that lists what was searched; it is never
replaced by the default driver:

```
[warden] ERROR: driver "nomad" not found
  To fix: install warden-driver-nomad beside warden (/usr/local/bin) or on
          PATH, or name its file: --driver /path/to/warden-driver-nomad.
          `warden driver list` shows the drivers warden can find.
```

**Checks on the file found** (steps 1, 3 and 4), before it is run:

- It is a regular file, executable by the user.
- On Unix it is not writable by group or others. When `warden` runs as
  root, it is owned by root or by the owner of the `warden` executable
  (the binary root is already running). A refusal names the file and the
  mode or owner found.
- On Windows nothing checks who may write the file in K1 (**Risks**).

**`warden driver list`** (new) prints, for each name it can find, the path
that would run, `linked` for a linked driver, and any shadowed copies later
in the search order. It reads directories only and runs no driver. With
`--json` it prints one object, `{"drivers":[{"name","path","linked",
"shadowed":[...]}]}` (UX-02).

**What `warden` runs without being asked.** Only the selected driver. The
commands that select no driver cover the bundled ones: `image list`, and
`image verify` with no `--driver`, ask each linked driver and each bundled
driver binary in `warden`'s own directory. A driver found on `PATH` is run
only when it is named.

**Recorded.** Before the build starts, `warden` writes
`<output>/driver.json`:

```json
{"name": "container", "version": "0.9.0", "contract": 1,
 "path": "/usr/local/bin/warden-driver-container",
 "sha256": "9f2c...", "linked": false}
```

For a linked driver `path` and `sha256` are `warden`'s own. The file is
unsigned and is not part of the ledger. The environment record is not
touched: `specs/metadata-environment/v1.schema.json` already defines
`driver` as a string, set by the driver's own environment description.

**What a planted binary can and cannot do.** It cannot take a bundled
name, come from the current directory or a relative `PATH` entry, be named
by a config file, or be a batch file. A name that is not bundled is not
protected against a writable directory earlier in `PATH`: `driver list`
and `driver.json` show which file ran; they do not prevent it (a16, a19).

### The commands

The verb is `argv[1]`. Except for `attach` and `ext`, a command reads its
request as one JSON object on stdin, and the request carries `contract`,
the version `warden` chose. Secrets travel only in a request on stdin,
never in argv or the environment.

`describe` and `run` own stdout: every byte there is the contract's.
Every other command inherits the terminal for stdout and stderr, prints
its own lines, and writes its result as one JSON object to the file named
by the environment variable `WARDEN_DRIVER_RESULT`. `attach` and `ext`
inherit the terminal for stdin too, so their request is in the file named
by `WARDEN_DRIVER_REQUEST`. `warden` creates both files with mode 0600 in
its temporary directory and removes them.

| Command | Required | Request | Output | Exit |
|---|---|---|---|---|
| `describe` | yes | none | One JSON object on stdout. No side effects; answers within 10 s | 0 |
| `run` | yes | `start` on stdin, then messages | Messages on stdout, one JSON object per line; human lines on stderr | 0 after `stopped`, 1 after `failed` |
| `reap <build-id>` | yes | `{contract, build_id, tmpdir, options}` | Result `{removed}`. Removes whatever that build left, from the ID alone. Succeeds, and may be repeated, when nothing exists | 0, 1 |
| `doctor` | no | `{contract, vm, options, verbose}` | Its report on stdout. Result `{blocked}`. Changes nothing on the host | 0, 1 |
| `setup` | no | `{contract, vm, options, verbose}` | Its progress. Result `{}` | 0, 1 |
| `image-prepare` | no | `{contract, os, source, options, verbose}` | Progress on stderr. Result `{path}`; an empty path means the driver already reported the outcome | 0, 1 |
| `image-list` | no | `{contract}` | One line per prepared image on stdout. Result `{count}` | 0, 1 |
| `image-verify` | no | `{contract}` | One line per cached layer on stdout. Result `{checked, drift}` | 0, 1 |
| `clean` | no | `{contract, keep_ids, cache, tmpdir, options}` | `Removing ...` lines on stderr. Result `{removed}`. Removes its leftovers of every build whose ID is not in `keep_ids`, and its own cache when `cache` is true | 0, 1 |
| `attach <build-id>` | no (needs `capabilities.shell`) | `{contract, build_id, options}` in the request file | The interactive shell, on the inherited terminal | The shell's own code; 125 when the attach itself failed |
| `ext <args...>` | no | `{contract, options}` in the request file; the arguments in argv | The driver's own subcommands, listed in `describe.ext`. `warden hyperv service install` reaches `ext service install` | The driver's own |

**Exit codes**, for every command but `attach` and `ext`:

| Code | Meaning |
|---|---|
| 0 | Done |
| 1 | Failed; the result (or, for `run`, the `failed` message) carries an `error` object |
| 2 | The request is not valid for this contract version |
| 3 | The command is not implemented by this driver |
| 4 | No contract version in common |

Any other code, a missing result, or output that is not what the command
owes, is a driver defect, and `warden` reports it as one (**Errors**).

**Probe.** Any request may carry `"probe": true`. The driver checks
`contract` and its option values, creates nothing, and exits 0 or with its
refusal. `warden` probes `run` before it stages anything, so a bad option
value is refused before slow work (UX-08). A probe answers within 10 s.

**`describe`:**

```json
{"contract": [1],
 "name": "qemu", "version": "0.9.0", "vendor": "buildwarden",
 "bug_url": "https://github.com/buildwarden/buildwarden/issues/new",
 "commands": ["run", "reap", "doctor", "image-prepare", "image-list", "image-verify"],
 "capabilities": {"vm": true, "shell": false, "keep": "stopped", "exited": false,
   "containerfile": "translate", "guest_os": ["linux", "windows"],
   "image_os": ["linux", "windows"], "default_image_os": "linux",
   "reserved_ranges": ["10.0.2.0/24"]},
 "options": [{"name": "edition", "type": "string", "flag": "edition",
   "config": "", "env": "", "commands": ["image-prepare"],
   "usage": "Windows edition / install.wim image name (qemu windows)"}],
 "ext": [],
 "help": {"doctor": "...", "image-prepare": "..."}}
```

| Field | Meaning |
|---|---|
| `contract` | The integer contract versions the driver speaks |
| `name` | What `--driver` selects; matches the file name |
| `version`, `vendor`, `bug_url` | For `driver.json`, `--version` output and bug text |
| `commands` | The commands it implements, `describe` aside. `run` and `reap` must be present |
| `capabilities.vm` | It boots a separate build machine: it takes the shared VM settings (`start.vm`) and builds a script or a Containerfile. Without it, it builds a Containerfile only and `warden` refuses the VM flags |
| `capabilities.shell` | `run` accepts `mode: "shell"` and `attach` is implemented |
| `capabilities.keep` | What `--keep-vms` leaves: `running` (the machines), `stopped` (the work directory only) or `none` (`warden` refuses the flag) |
| `capabilities.exited` | It sends `exited` when its relay or its environment stops on its own |
| `capabilities.containerfile` | `translate`: `warden` translates a Containerfile to the build script. `ignore`: the guest cannot run one; `warden` drops it and records nothing |
| `capabilities.guest_os` | The `--guest-os` values its builds accept; the first is its default |
| `capabilities.image_os`, `default_image_os` | The `--os` values `image-prepare` takes |
| `capabilities.reserved_ranges` | Fixed address ranges it uses on the host; a build link must not overlap them |
| `options` | The flags and keys it owns (**Configuration**) |
| `ext` | Its own subcommands: `[{"path": ["service", "install"], "usage": "..."}]` |
| `help` | Its paragraph in a command's long help, keyed by command |

`warden` ignores fields it does not know, and so does a driver. Adding a
field, a command, a capability or a message type does not change the
contract version; removing or changing the meaning of one does.

### The build

`run` is one process per build. `warden` and the driver exchange messages,
one JSON object per line, UTF-8, at most 1 MiB a line: `warden` writes to
the driver's stdin and reads its stdout. Each message has a `type`. Both
sides ignore a type or a field they do not know. The driver's stderr is
its human-readable log, which `warden` passes to its own stderr.

| From | `type` | Carries |
|---|---|---|
| warden | `start` | `contract`, `build_id`, `mode` (`build` or `shell`), `claim_token`, `guest_os`, `image`, `base_image`, `build_script`, `context_dir`, `tmpdir`, `diagnostics_dir`, `keep`, `verbose`, `vm`, `options` |
| driver | `relay-started` | `claim`: `{"network": "tcp", "address": "127.0.0.1:41233"}`; `claim_values`: `{environment, build_link, upstream_dns}`; `guest_os`; `old_relay_fix` |
| warden | `claimed` | Nothing: the relay is configured and ready; start the environment |
| driver | `environment-started` | Nothing |
| driver | `exited` (optional) | `which` (`relay` or `build`) and `detail`; valid at any time after `relay-started` |
| warden | `stop` | `reason` (`completed`, `failed`, `interrupted`, `timeout`, `shell-exited`) |
| driver | `stopped` or `failed` | `stopped`: `kept`, the paths and names left behind under `keep`. `failed`: an `error` object. Exactly one of the two, as the last message, after teardown |

The fields of `start`:

| Field | Meaning |
|---|---|
| `build_id` | Eight lowercase letters and digits, made by `warden` (`driver.RandAlphaNum(8)`, as each driver makes its own today). The driver names everything it creates for the build from it, so `reap` can find it again |
| `mode` | `build` runs the script. `shell` holds the environment after setup for `attach`; only for a driver with `capabilities.shell` |
| `claim_token` | The claim token, made by `warden`. The driver puts it on the relay's boot channel, where it is one of the relay's pre-claim values (RELAY-08). It is the only secret the driver receives |
| `guest_os` | The `--guest-os` value, or the driver's default |
| `image` | The `--image` value, or empty |
| `base_image` | The `FROM` image of the Containerfile `warden` translated, or empty. It replaces each driver's own second translation (`container.stageBuildScript`, `linuxguest.ContainerfileImage`) |
| `build_script` | The absolute path of the staged build script. The driver reads it for its environment description and never serves it: the collector does |
| `context_dir` | The build context, absolute |
| `tmpdir` | Where work directories are made (`--tmpdir`, `[build] tmpdir`); empty means the system's |
| `diagnostics_dir` | The build's output directory. The driver may write `relay.log` and `console.log` there and nothing else |
| `keep` | `--keep-vms`. `warden` sends true only when `capabilities.keep` is not `none` |
| `vm` | For a driver with `capabilities.vm`: `{cpus, memory_mb, build_link, build_link_source, dns}`. Zero or empty means "not set", and the driver applies its default |
| `options` | The driver's own options (**Configuration**) |
| `collector` | Reserved, never sent in version 1 (**The collector**) |

`relay-started` carries what a driver passes into `Session.ClaimRelay`
today. `claim.address` is where the relay accepts its claim, reachable
from `warden`'s process as the user `warden` runs as. `claim.network` is an
enumeration whose only value in version 1 is `tcp`. `claim_values` are
copied into the claim unchanged, as its `environment`, `build_link` and
`upstream_dns` fields; their shape is the claim protocol's (seam 1), not
this contract's. A driver sets no other claim field: `collector_url`,
`collector_token`, `capture_mode` and the upstream CA fields are `warden`'s.
`guest_os` is the guest the environment really runs, which selects the
bootstrap config the collector serves; vz reports `darwin` whatever was
asked. `old_relay_fix` is the fix text `warden` shows if the relay refuses
a claim field as unknown. Nothing returns to the driver from the claim:
every driver discards the relay's identity today.

**The sequence.** The actor is named at each step.

1. **warden** resolves the driver (**Discovery and trust**), runs
   `describe`, picks the contract version, resolves and checks the options,
   and probes `run`.
2. **warden** creates the output directory, writes `driver.json`, and
   stages the build script in `<tmpdir>/warden-stage-<build_id>/` with
   `driver.StageBuildScript`, then records an altered input with
   `driver.RecordInputs`, as each driver does today. For
   `capabilities.containerfile: ignore` it drops the Containerfile first,
   as vz does, so nothing is translated or recorded.
3. **warden** makes `build_id` and `claim_token`, takes the host lock
   `build-<driver>-<build_id>` (`driver.HostLock`; an OS file lock,
   released if `warden` dies), starts `run` in its own process group and
   sends `start`. `--timeout` counts from here.
4. **The driver** runs its preflight, resolves its relay and its
   environment's image, describes the environment, starts the relay with
   `claim_token`, and sends `relay-started`.
5. **warden** starts the collector (`driver.StartSession`, with the
   `guest_os` the driver reported) and runs `Session.ClaimRelay` against
   `claim.address`: it opens the tunnel, sends the claim with
   `collector_url` set to the tunnel, its own collector token and the
   driver's `claim_values`, waits for the relay's `ready` (90 s from the
   start of the claim), and writes `ca.cert.pem`. An `exited` with `which:
   relay` during the claim cancels it, as firecracker does today.
6. **The relay** reaches **the collector** over that tunnel. The host
   opens no listener and the relay never dials the host (RELAY-07).
7. **warden** sends `claimed`. **The driver** isolates the environment,
   delivers the executor, starts it, and sends `environment-started`. The
   relay is claimed and configured before the environment exists (BOOT-01).
8. **The collector** takes the console output, heartbeats and completion,
   as today; the driver's messages carry none of them. **warden** runs
   `Session.Wait`: completion, 30 s without a heartbeat after the first,
   loss of the tunnel, `--timeout` or Ctrl-C.
9. **The driver** may send `exited`. A relay that exited fails the build
   at once. An environment that exited gives the build 10 s to deliver a
   completion already on its way, then fails it. This is firecracker's
   rule, applied to every driver; it needs no knowledge of the driver.
10. **warden** sends `stop`. **The driver** writes `relay.log` and
    `console.log` to `diagnostics_dir`, tears down everything it created
    (under `keep`, all but what its `capabilities.keep` leaves), sends
    `stopped`, and exits 0. **warden** closes the session, removes the
    stage directory, finalizes the output (`driver.FinalizeOutput`),
    releases and removes the lock, and reports. The exit code is the build
    script's, from the collector (UX-09).

A linked driver takes part in the same sequence. `warden` calls its `Run`
function with the `start` value and a `driver.Host`; the messages are
method calls and return values (**The kit**). Steps 1 to 10 are one piece
of code in `cmd/warden/buildseq.go` over two transports.

**Ctrl-C and `--timeout`.** The driver runs in its own process group, so
the terminal's signal reaches `warden` only. `warden` sends `stop`, closes
the driver's stdin and waits up to 60 s for `stopped`. A second Ctrl-C, or
the 60 s passing, kills the driver's process group (on Windows, the driver
process) and runs `reap <build_id>`. A driver treats `stop` and the end of
its input the same way: tear down, then exit.

**The driver fails.** Before `relay-started`, a refusal is a `failed`
message with a fixable error, and `warden` prints it as it prints a
preflight error today. If the process exits without a last message, or
writes a stdout line that is not a JSON object, `warden` fails the build,
shows the tail of the driver's stderr, and runs `reap <build_id>`.

**`warden` dies (ORCH-04).** The driver reads the end of its input and
tears down. If both died, nothing is running that `warden` started; what
is left is found later: `warden clean` runs the driver's `reap` for every
`build-<driver>-*` lock nobody holds and removes the lock file, then runs
the driver's `clean` with the IDs of the locks that are held as
`keep_ids`. No sweep runs before a build (a20).

**`--keep-vms`.** `keep: running` (hyperv, firecracker) leaves the
machines and their work directory; `stopped` (qemu, vz) leaves the work
directory only, which is what those two do today; `none` (container)
makes `warden` refuse the flag, as today.

**`warden shell`.** `warden` sends `start` with `mode: "shell"`. The
driver starts the environment held after setup and sends
`environment-started` once the shell can be entered. `warden` then runs
`attach <build_id>` with the terminal inherited, takes its exit code as
the shell's, and sends `stop`. It does not run `Session.Wait`.

**Per driver.** What each reports as its claim endpoint, what holds its
environment, and what a `reap` has to find. Only the container driver is a
binary in K1; the last column is K2's work for the others, listed here so
the contract is known to fit them.

| Driver | `claim.address` | What keeps the environment alive | `reap <id>` |
|---|---|---|---|
| container | `127.0.0.1:<port>`, the relay container's published port | The container runtime; names `warden-holder-<id>`, `warden-relay-<id>`, `warden-build-<id>`, network `warden-<id>` | **K1.** Removes those by name, the dev relay image `warden-relay:<id>`, and the work directory |
| qemu | `127.0.0.1:<port>`, forwarded by qemu to the relay VM | Two `qemu-system` children of `run` | K2. Needs a per-build pid record, which is new (firecracker's pid file is the model); how the record is checked against a reused pid on macOS is not designed |
| vz | `127.0.0.1:<port>`, the host-mode relay child | The VM is an object inside the `run` process; the relay is a child. The socketpair is the build link and never leaves the driver | K2. The relay child and the disk clones in the work directory. `run` must be the entitled process with the main thread locked, so the entitlement moves to `warden-driver-vz` |
| hyperv | `<NAT address>:8301` of the relay VM | Hyper-V, by name: `warden-relay-<id>`, `warden-build-<id>`, switch `warden-<id>` | K2. By name, as `hyperv.Clean` does. The service takes network operations only, so no build runs inside it; `InstallService` then registers the driver binary with `ext service run`, since the service manager must be that process's parent |
| firecracker | `100.64.88.(4·slot+2):8301`, dialled as the slot's owner | Two VMM children with `Pdeathsig`, and the slot and build locks, all tied to the `run` process | K2. The slot pid file, as `firecracker.Clean` does |

Three of these are behaviour K1 has to hold on the linked transport too:
firecracker's `Pdeathsig` and locks follow `warden`'s process, as today;
vz's main-thread lock stays in `cmd/warden`; and Hyper-V's `StartRelay`
gives up its `StartSession` and `ClaimRelay` calls to `warden` and keeps
the rest.

### Configuration

`warden` alone reads the config files, the environment and the flags, with
today's precedence (`LoadConfig` and `resolveConfig`; ORCH-03). A driver
declares what it owns and receives resolved values, so it needs no TOML
parser and no knowledge of where a value came from beyond its label.

An option in `describe`:

| Field | Meaning |
|---|---|
| `name` | Identifies the option in a request's `options` |
| `type` | `string`, `int`, `bool`, `age` (`7d`, `36h`), `cidr`, `ip-list` or `path` |
| `flag` | The flag's name without dashes; empty for no flag |
| `config` | The key in `warden.toml` and the user config, as `table.key`; empty for none |
| `env` | An environment variable that sets it. Only the two that exist today may be declared (`WARDEN_CTR_CLI`, `WARDEN_IMAGE`): UX-07 gives new settings a flag and a key |
| `commands` | The commands that take it: `run` (which is `warden build` and `warden shell`), `doctor`, `setup`, `image-prepare`, `clean`, `ext` |
| `usage` | The flag's help text |

A request's `options` is `{"<name>": {"value": ..., "source": "..."}}` for
each option somebody set and that applies to the command. `source` is the
label `warden` prints for it (`--network`, `WARDEN_IMAGE`, `warden.toml`,
the user file's name). An option nobody set is absent, and the driver
applies its own default, as each does today.

What the five bundled drivers declare is exactly what `cmd/warden` holds
for them today. No name, table or variable changes. In a Go driver an
option is a composite literal with constant strings, so tier 1 can read
the surface from source (Rule changes).

| Driver | Option | Flag on | Config key | Env |
|---|---|---|---|---|
| container | `runtime` | every command | `runtime.cli` | `WARDEN_CTR_CLI` |
| container | `relay-image` | none | `runtime.relay_image` | `WARDEN_IMAGE` |
| qemu | `force`, `edition`, `virtio`, `arch`, `keep-vms` | `image-prepare` | none | |
| qemu, hyperv | `image-refresh-after`, `image-min-grace` (ages) | none | `vm.image_refresh_after`, `vm.image_min_grace` | |
| vz | `update` | `image-prepare` | none | |
| hyperv | `network` | `run`, `doctor`, `setup` | `vm.network` | |
| hyperv | `nat-prefix`, `host-ip`, `slots` | `setup` | none | |
| hyperv | `sha256`, `force`, `name`, `keep-vms` | `image-prepare` | none | |
| hyperv | `windows-image`, `windows-image-update` | none | `vm.windows_image`, `vm.windows_image_update` | |
| firecracker | `slots` | `setup` | none | |

`[runtime] cli` is already run as given from a `warden.toml` in the
working directory; K1 does not change that.

`warden` keeps what belongs to no driver: `--driver`, `--verbose`,
`--color`; `--capture`, `--output`, `--no-compress`, `--timeout`,
`--tmpdir`, `--quiet`; the VM group `--script`, `--image`, `--guest-os`,
`--cpus`, `--memory`, `--keep-vms`, `--build-link`; `image prepare`'s
`--os` and `--source`; `clean --cache`; and the keys `runtime.driver`,
`build.*`, `relay.*`, `vm.cpus`, `vm.memory_mb`, `vm.build_link`,
`vm.dns`, `output.*`. The build link and `[vm] dns` are still parsed and
checked by `warden` as the files load, and the link is checked against the
reserved ranges of every bundled driver, as today: the linked drivers'
from their `describe` values and the container driver's
`driver.ContainerBaseCIDR`, which stays in the kit. K2 decides who carries
the ranges once no driver is linked.

How `warden` handles options:

1. **Find the driver before the flags are parsed.** `warden` reads
   `--driver` from the raw arguments, else `WARDEN_DRIVER`, else `[runtime]
   driver` from the config files, else `container`, and looks the driver
   up. Commands that select no driver (`inspect`, `driver list`, `image
   list`, and `image verify` without `--driver`) skip this.
2. **Describe.** `warden` runs `describe` on the selected driver only, once
   per invocation, and refuses one that does not match the schema, lists
   no common contract version, lacks `run` or `reap`, or reports a name
   other than the one selected. For a driver that is not bundled it also
   refuses a config key outside `[driver.<name>]`, an `env`, and a flag
   whose name `warden` uses on the same command; each refusal names the
   option. For the bundled drivers a test in `cmd/warden` checks the same,
   so a clash is found by `make test`.
3. **Register** the driver's flags on the commands that take them. Help
   therefore shows `warden`'s flags and the selected driver's.
4. **Check the configuration.** Every key the selected driver declares is
   checked against its type as the files load, and a value that does not
   fit is refused before any driver work, naming the file and key (UX-08).
   Keys that neither `warden` nor the selected driver declares are ignored,
   as unknown keys are today: one `warden.toml` serves several drivers.
5. **Refuse** a flag the selected driver does not declare. When a bundled
   driver does declare it, the message is `--network is only used by
   --driver hyperv`, with the fix `drop --network, or use --driver hyperv`;
   otherwise it is the unknown-flag error with the command's `--help` as
   the fix. A VM-group flag on a driver without `capabilities.vm`, and a
   `--guest-os` the driver does not list, keep today's messages.
6. **Resolve** each option that applies to the command: the flag when set,
   else its environment variable, else `warden.toml`, else
   `~/.config/warden/config.toml`, else absent. That is today's order, and
   the value found is used exclusively (GLOBAL-10).
7. **Probe, then call.** Range and consistency checks are the driver's,
   made in its probe before slow work (UX-08).

`warden` reads driver-declared keys by decoding each file a second time
into a generic tree, so a key a driver declares needs no field in
`Config`. The driver-owned fields leave `Config` (`RuntimeConfig.CLI` and
`RelayImage`, `VMConfig.Network`, `ImageRefreshAfter`, `ImageMinGrace`,
`WindowsImage`, `WindowsImageUpdate`).

For the four linked drivers, K1 moves the declarations and not the code.
Each declares its options in a Go literal that `linked.go` reads; its
doctor, setup, image and clean code stays in `cmd/warden` and reads the
resolved values in place of the flag variables.

### Errors, output and versions

**The error object** is `{"code", "msg", "fix", "details"}`:

| `code` | Meaning | `warden` prints |
|---|---|---|
| `fixable` | Today's `driver.FixableError{Msg, Fix}`: a known cause with a remedy | `[warden] ERROR: <msg>` and `To fix: <fix>`; no bug link |
| `busy` | Another process already does this (today's `ErrPrepareRunning` sentinels) | The message, as today |
| `invalid-request` | Exit 2 | A defect of `warden` or the driver: bug text |
| `unsupported-command` | Exit 3 | For an optional command the driver did not declare, `warden` never calls it; if it is returned anyway, a driver defect |
| `unsupported-contract` | Exit 4 | A fixable error naming both versions and which side to update |
| `internal` | Anything else | The message, the tail of the driver's stderr, and bug text that names the driver, its version and its `bug_url` |

A driver defect (an exit code outside the table, a missing result, bad
JSON on `run`'s stdout) prints like `internal`. `warden`'s own bug link is
shown only for `warden`'s own failures. The build's exit code always comes
from the collector; `attach`'s is the shell's; 124 and 130 keep their
meaning (UX-09).

**Output.** A command's lines are the driver's own, on the inherited
terminal, so today's text and order survive without an encoding for
launcher stamps, image lists or layer results. The result file carries
only what `warden` acts on or totals: `blocked` (doctor exits 1 with no
extra line), `path` (`Base image ready: <path>`), `count` (`No prepared
images`), `drift` (`N cached layer(s) changed or missing`) and `removed`
(`Removed N resource(s).`). Colour is `warden`'s `[warden]` prefix only, as
today. `warden driver list --json` is the one new machine-readable output
(UX-02).

**Versions.** `describe.contract` lists the versions a driver speaks.
`warden` picks the highest it also speaks and sends it in every request;
none in common is a fixable error. Version 1 is unstable before 1.0
(GLOBAL-08): it may change in any release. A driver's own `version` is
independent of `warden`'s, and `driver.json` records it.

### The kit

`driver/drivercmd` is a Go library for a driver that wants it. A driver's
`main` is one call:

```go
// cmd/warden-driver-container/main.go
func main() { drivercmd.Main(container.Driver()) }
```

```go
// Package drivercmd implements the driver command contract
// (specs/driver-contract/v1.md) for a driver written in Go.
package drivercmd

// Driver is one driver: what describe reports and a function per command.
// A nil function is a command the driver does not implement.
type Driver struct {
	Describe Describe // name, version, capabilities, options, ext, help
	Run      func(ctx context.Context, start *driver.Start, host driver.Host) (kept []string, err error)
	Reap     func(ctx context.Context, req *ReapRequest) (removed int, err error)
	Doctor   func(ctx context.Context, req *DoctorRequest) (blocked bool, err error)
	Setup    func(ctx context.Context, req *SetupRequest) error
	Prepare  func(ctx context.Context, req *PrepareRequest) (path string, err error)
	List     func(ctx context.Context) (count int, err error)
	Verify   func(ctx context.Context) (checked, drift int, err error)
	Clean    func(ctx context.Context, req *CleanRequest) (removed int, err error)
	Attach   func(ctx context.Context, req *AttachRequest) (exitCode int, err error)
	Ext      func(ctx context.Context, req *ExtRequest, args []string) error
}

// Main runs the command named by os.Args and exits with the contract's code.
func Main(d *Driver)
```

```go
// Package driver: the run dialogue as a driver built on the kit sees it.

// Host is warden's side of one build.
type Host interface {
	// ClaimRelay reports the relay as started and returns once warden has
	// claimed it and it is ready (relay-started, then claimed), or with the
	// reason the claim failed.
	ClaimRelay(ctx context.Context, r RelayStarted) error
	// Started reports that the build environment is running.
	Started() error
	// Exited reports that the relay or the environment stopped on its own.
	Exited(which ExitedPart, detail string)
	// Wait returns when warden asks the driver to stop, with the reason.
	Wait(ctx context.Context) string
}
```

What `Main` does: it dispatches `argv[1]`; builds `describe` from the
`Describe` value; decodes the request from stdin or the request file and
checks `contract`; answers a probe; writes the result file; maps a
`*driver.FixableError` to a `fixable` error object and any other error to
`internal`; and exits with the contract's code. For `run` it points the
process's stdout at stderr before any driver code runs, so a stray print
from the driver or a child cannot corrupt the message stream, and it
turns the end of stdin into the cancellation of `ctx` and the return of
`Wait`.

A driver keeps today's shape: where it called `sess.ClaimRelay(ctx, addr,
token, req, env, link, fix)` it calls `host.ClaimRelay(ctx, r)` with the
same address, environment, link and fix; where it called `sess.Wait(ctx)`
it calls `host.Wait(ctx)` and tears down when that returns. It no longer
calls `StartSession`, `StageBuildScript`, `RecordInputs` or
`FinalizeOutput`.

`warden` implements `driver.Host` twice: over its session directly, for a
linked driver; and `drivercmd` implements it over the pipe, for a binary.
A linked driver and a driver binary are thus the same `Driver` value.

The Go types are not the contract. They may change with the kit; the
specification and the suite are what a driver is held to.

**Without the kit** a driver does the same by hand: read a JSON line,
write a JSON line, exit with the right code. It needs no TOML parser, no
Dockerfile translator, no collector and no claim client. The reference
fake (**The conformance suite**) is written that way.

### The operations, per driver

What each driver does today for each operation. In K1 the container
driver's column moves behind the commands; the four VM drivers' doctor,
setup, image, clean and own-command code stays where it is, in the driver
packages and in `cmd/warden`, and K2 moves each behind the commands with
its binary. Every driver's **Relay start** row moves onto `warden`'s build
sequence in K1.

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

**The container driver, in K1.** Its code in `cmd/warden` moves into
`driver/container` with its tests, behind `drivercmd`:

| Command | From | Notes |
|---|---|---|
| `describe` | `newDriver`'s container arm; `isVMDriver` | `vm: false`, `shell: true`, `keep: none`, `containerfile: translate`, `guest_os: ["linux"]`; options `runtime` and `relay-image`; `reserved_ranges` its base range |
| `run` | `StartBuild` and `Exec`'s setup | The relay container, then on `claimed` the holder, the iptables sidecar, the build container, the executor copy and the start. `mode: shell` starts it held and waits for `/etc/warden/shell.env`, as `waitHeld` does |
| `attach` | The second half of `Exec` | `<runtime> container exec -i -t <build> sh -c ...` on the inherited terminal; the exit code from `/.warden/shell-exit`. The runtime comes from the request's `runtime` option |
| `reap` | New; `teardown`'s removals by name | Containers, network, dev relay image and work directory of one build ID |
| `doctor` | `doctorCmd`'s container arm; `resolveRuntime`, `DetectRuntime`, `runtimeStartHint` in `config.go` | Prints `container runtime: <rt>`, then `ok` |
| `clean` | `listResources`, `isOrphan` in `clean.go` | Removes `warden-*` containers and networks and `warden-relay:*` images whose build ID is not in `keep_ids`. `keep_ids` replaces `findLiveWardenProcesses`, the `ps aux` scan |

`driver.ContainerBaseCIDR`, `WardenBaseNet` and `AllocateNetwork` stay in
the kit's `driver` package in K1, since `warden` checks the build link
against the range; they move with the ranges in K2. The container driver's
`Runtime` and `Version` fields go: the runtime is an option, and the
version is the binary's own, stamped at build time.

**The four VM drivers, in K1.** Each gains a `Driver()` value with its
`Describe` and its `Run`, which is its `StartBuild` with the session calls
replaced by `driver.Host`. `vz.New` stops resolving the cache directory,
so building the value does no host work. Nothing else of theirs moves:

- Their doctor, setup, image prepare, list and verify, and clean code
  stays in `cmd/warden` (`host.go`, `image.go`, `image_verify.go`,
  `clean.go`) and their packages, reading resolved option values.
- `cmd/warden/main_darwin_arm64.go`'s main-thread lock stays.
- `warden hyperv service install|uninstall|run` stays a `warden` command,
  and the installed service keeps pointing at `warden`.
- The two `driver.StartDetached` re-invocations (qemu's and hyperv's
  background `image prepare --driver ...`) keep working, since `warden`
  still takes those arguments.
- `relayvm.QEMURelay`, `QEMUBuild` and `Firecracker`, the driver names in
  `driver/buildlink/disjoint_test.go` and `driver/claimlink_test.go`, and
  `tools/vz-image-prep`'s imports of `driver/vz` stay as they are.

`driver/claimlink_test.go` checks in source that qemu, firecracker and vz
pass the request's link to `ClaimRelay`. It is rewritten to check the same
of `host.ClaimRelay`'s `RelayStarted` value.

### The relay source

Today, on `main`:

| Driver | What it runs | Where it finds it | Override |
|---|---|---|---|
| container | An OCI image | Release build: pulls `ghcr.io/buildwarden/relay:latest`. Dev build (`version == "dev"`): builds `cmd/relay` for Linux and an image `warden-relay:<build id>` per build (`resolveRelayImage`, `buildRelayFromSource`) | `[runtime] relay_image` or `WARDEN_IMAGE`: an image reference, pulled; or `dev`, built from source |
| qemu | A relay VM: kernel, Alpine base initramfs, and the Go relay appended per build at `/sbin/relay` (`relayvm.RelayInitrd`) | Kernel and initramfs: beside the `warden` binary (`relay-vm-vmlinuz`, `relay-vm-initramfs.cpio.gz`), else `tools/relay-vm/output/` in the source tree, never mixed (`relayvm.Find`). Relay binary: `driver.GuestBinary("relay", "linux", GOARCH)` | none |
| firecracker | The same shape, x86_64: `firecracker-vmlinux`, `firecracker-relay-initramfs.cpio.gz`, relay appended | `relayvm.Find(relayvm.Firecracker)`; `driver.GuestBinary("relay", "linux", "amd64")` | none |
| vz | A host process: the Go relay for darwin/arm64, copied to the work dir and ad-hoc signed, run as `--mode=host --fd=3 --claim-addr=127.0.0.1:<port>` with the token in its environment (`startHostRelay`) | `driver.GuestBinary("relay", "darwin", "arm64")` | none |
| hyperv | A relay VM from a static boot VHDX that bakes kernel, init and relay, with a per-build overlay and seed (`resolveRelayBootVHDX`, `StartRelay`) | `relay-boot.vhdx` in the hyperv cache (`%LOCALAPPDATA%\warden\hyperv`, or `WARDEN_HYPERV_CACHE_DIR`), built by `tools/relay-vm/hyperv/refresh-cache.sh` | `WARDEN_RELAY_BOOT_VHDX`, a path |

`driver.GuestBinary` looks beside the running executable, then in the
per-source-tree build cache, then builds from the source tree. For a driver
binary the running executable is the driver, and a bundled driver ships in
`warden`'s directory, so the files are found where they are today. None of
the five checks a hash, and the container default is a moving tag.

The types, in `driver/relaysource`. They are a kit library and not part of
the command contract: a driver from outside finds its relay however it
likes, and `warden` never sees a `Relay`. The form and role names are
provisional until seam 8 is written (a11):

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

The same package holds the sources that exist today, each wrapping the
current lookup without changing it:

```go
// Bundled finds a relay's files beside the running binary, else in the
// source tree, all from one place (relayvm.Find's rule), and builds or finds
// the relay binary as driver.GuestBinary does. Relay returns an error
// wrapping ErrNotFound when neither place has every file; the driver adds
// its own fix.
func Bundled(form RelayForm, files ...File) RelaySource

// File is one bundled file: its release name beside the binary and its path
// in the source tree, or a cmd/ package to build for an OS and architecture.
type File struct {
	Role            RelayRole
	Release, Source string
	Build           *GoBuild // cmd/relay for linux/arm64, ...
}

// Path is a relay file the user named: used as given, never looked up
// elsewhere. origin is the option or variable that named it.
func Path(form RelayForm, role RelayRole, path, origin string) RelaySource

// Image is an OCI image reference the user named, pulled as given.
func Image(ref, origin string) RelaySource
```

A driver's relay-start step takes a `*relaysource.Relay` and calls `Verify`
before it starts anything. The container driver does so in K1. Each VM
driver does when it becomes a binary in K2, and until then keeps its
lookup as it is (a3); its row says what it will use:

| Driver | Form | Source | With an override |
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
directory: the source its build resolves, and the boot code for a form it
has not run before. A pinned source (version, URL and
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

### The collector

**In K1.** `warden` starts today's in-process collector, one per build
with a fresh token (`driver.StartSession`), and serves it to the relay
over the tunnel it opens on the claim connection. It hands the driver
nothing about the collector. The driver's part of "wiring the relay to
the collector" is to make its relay claimable by `warden` and to say
where. No `--collector` flag or key is added.

**What this leaves possible**, and does not design:

1. *A remote collector.* The collector is a claim field (RELAY-08), and
   the relay already accepts a `collector_url` that is not the tunnel
   (`resolveTransport`, `relaycfg/relaycfg.go`). A remote collector is
   another URL and another session token in the claim `warden` sends. No
   driver and no part of this contract changes.
2. *An existing collector on the host.* `warden` can serve it to the relay
   over the same tunnel, by proxying, which also covers relays that cannot
   open a connection to the host (firecracker's rules allow only
   established connections in).
3. *Console and status somewhere else.* They never travel in the driver's
   messages, so whoever owns the collector owns them.
4. *A claim the driver sends.* `start.collector` is reserved for an
   endpoint and a token handed to a driver that sends the claim itself,
   and `claim.network` is an enumeration, so both can be added as a
   capability without a new contract version.
5. *Several builds on one collector.* The contract has one session token
   per build and says nothing about the collector's lifetime; `seam-specs`
   already writes that lifetime per session token.

The client-facing side of a shared collector (opening a session, supplying
inputs, watching console and status, fetching outputs) is a later phase
(the umbrella's Decision 8 and a2).

### The boundary with S4 and K2

**S4** was "the claim connection: `driver.Session` takes a connection from
the driver, not a TCP address". K1 settles the question it existed to
answer: `warden` claims, at an endpoint the driver reports. What is left
of S4 is small and is not K1's: the claim specification's transport
section (S1 writes the claim over "a byte stream the claimant opens"), and
a value of `claim.network` that is not `tcp` (a Unix socket, vsock, a
named pipe), which the Rust relay on VZ will want. Until then a relay
that is reachable only over such a path needs its driver to forward it to
a loopback TCP port, as qemu already forwards its relay VM's port.

**K2** makes each VM driver a binary: its doctor, setup, image and clean
code moves behind the commands, `reap` is written for it, and it adopts
the relay source. What K2 inherits from K1 and must settle:

- `linked.go` and the linked transport go when the last VM driver is a
  binary.
- The vz entitlement and the main-thread lock move to `warden-driver-vz`;
  `warden` then needs neither cgo nor signing.
- The Hyper-V service is registered as `warden-driver-hyperv ext service
  run`, and `warden hyperv ...` reaches `ext`.
- The two `StartDetached` re-invocations name the driver binary.
- qemu's per-build pid record for `reap`.
- A Windows release target, and on Windows a job object so a killed
  driver's children die with it, and a check of who may write a driver
  file.
- Who carries the bundled drivers' reserved ranges, and whether `image
  list` runs drivers found on `PATH`.
- Then every driver becomes its own Go module, with the tier-1 check that
  limits it to the kit's public packages.

**K3** keeps its row: Hyper-V is extracted first.

### The conformance suite

`driver-conformance --driver <path>` plays `warden` against a driver
binary. It is a program, so a driver in any language can be put under it.
It has its own implementation of `warden`'s side, written from the
specification: it imports `relaycfg` and `collector` to claim the relay
and receive the build, and nothing under `driver/` or `cmd/warden`. A test
under `make test` checks those imports. Its skips come from the binary's
own `describe` and nothing else, so "declared but broken" and "implemented
but undeclared" both fail.

**Static tier.** No hypervisor, no container runtime, no network. It runs
on hosted runners for every PR, on Linux, macOS and Windows.

| Check | Rule |
|---|---|
| `describe` exits 0 within 10 s, prints one JSON object that matches the schema and nothing else, and prints the same on a second call | GLOBAL-08 |
| The name matches the file name; `run` and `reap` are listed; every option has a known type, a usage text, and (for a name that is not bundled) a key under `[driver.<name>]` and no `env` | UX-01, a4 |
| `capabilities` are consistent: `shell` exactly when `attach` is listed; `image_os` exactly when `image-prepare` is; an option names only listed commands | |
| Each listed command accepts a probe and exits 0, creating nothing in an empty temporary directory | UX-08 |
| Each command not listed exits 3 | |
| A request with a contract version the driver does not list exits 4 | |
| A request that is not JSON exits 2 | |
| `reap` of an ID that never existed exits 0 with `removed: 0`, twice | ORCH-04 |
| `run` whose input ends before `start` exits without output on stdout | ORCH-04 |
| With an empty cache: `image-list` reports `count: 0`, and `doctor` changes nothing in the cache or temporary directory | UX-04 |

**Lifecycle tier.** It runs real builds, so it needs a host the driver
runs on (tier 2, TEST-02; tier 1 for the container driver). The suite
stages a script, starts a disk collector, and drives the full sequence.
`--options <file>` gives the driver's option values for that host (the
container runtime, Hyper-V's network), `--base-image` or `--image` what
the build runs in.

| Case | Checks | Rule |
|---|---|---|
| Sequence | `relay-started`, then after `claimed` `environment-started`, then after `stop` `stopped` and exit 0; every stdout line is a JSON object; the claim endpoint accepts the claim with the token from `start` and no other | BOOT-01, GLOBAL-08 |
| Success | A script that prints a marker and exits 0: the collector receives the marker, a completion with code 0, and a ledger whose signature verifies | RELAY-07 |
| Inputs | A file the script reads from the context is in the ledger as a relay-served input | BOOT-04 |
| Failure | A script that exits 3: the completion carries 3, and the ledger is still delivered | UX-09 |
| Stop | `stop` sent after `relay-started`, after `environment-started` and mid-build: each ends in `stopped`, and a following `reap` removes nothing | ORCH-04 |
| End of input | The same three points with stdin closed and no `stop`: the same result | ORCH-04 |
| Kill | The driver killed at the same three points; `reap` then removes what was left, and a second `reap` removes nothing | ORCH-04 |
| Diagnostics | After a build, `diagnostics_dir` holds no name but `relay.log` and `console.log` | ORCH-02 |
| Two at once | Two builds with different IDs both succeed (optional per host) | ORCH-04 |
| Clean | With the running build's ID in `keep_ids`, `clean` removes nothing of it; with a killed build's ID absent, `clean` removes it (when `clean` is listed) | ORCH-04 |
| Keep | With `keep`, what `capabilities.keep` names is listed in `kept` and exists; `reap` then removes it (when not `none`) | ORCH-04 |
| Shell | `mode: shell`, then `attach` running `exit 7` on a pseudo-terminal exits 7 (when `shell`) | UX-09 |
| Exited | With the environment killed from outside, the driver sends `exited` with `which: build` (when `capabilities.exited`) | |
| Doctor | Not blocked; result present (when `doctor` is listed) | UX-04 |

It does not test isolation (S3's build-link suite), the claim's security,
or image preparation, which is driver-specific and can take hours.

**The reference fake**, `warden-driver-fake` (`conformance/driver/fake`),
is a Go program that uses the standard library only and none of the kit.
Its "relay" is `stubrelay` (`conformance/driver/stubrelay`), a separate
test program built on `relaycfg`: it accepts the claim, reports ready, and
plays the guest by reading directives from the build script the collector
serves (`# stub: output <text>`, `# stub: exit <n>`, `# stub: sleep <d>`),
then sends heartbeats and a completion. It writes no ledger, so the suite
runs the fake with `--stub-relay`, which skips the ledger checks. The fake
has three jobs:

- It shows a driver needs no kit.
- It lets `warden`'s own end-to-end tests run in CI on every platform:
  build, shell (its `attach` runs a host shell command), doctor and clean,
  through a driver `warden` was not built with.
- It has fault switches (an environment variable naming one fault: a
  stray stdout line, no teardown, end of input ignored, a wrong exit
  code, an undeclared command answered), and the suite's self-test checks
  that each of its checks fails under the matching fault.

A shell-script fake was considered and dropped: the Go relay's host mode
needs raw Ethernet frames on an inherited descriptor and is refused on
Windows, so a script could not start a real relay portably, and a script
that only pretends is `stubrelay` with less control.

**Where it runs.**

| Target | Static tier | Lifecycle tier |
|---|---|---|
| `warden-driver-fake` | `make test`, on the CI matrix | `make test`, with `stubrelay` |
| `warden-driver-container` | `make test`, on the CI matrix | The `integration` workflow (Docker on a hosted runner); natively on macos-finch and container-any |
| qemu, vz, hyperv, firecracker | Not binaries until K2 | K2, one native validation each |

`make test` builds the fake, `stubrelay` and the container driver binary
with the Go toolchain it already runs under and needs nothing else
(TEST-01); Plan chooses between building them in a `TestMain` and
re-executing the test binary. `make driver-conformance DRIVER=<path>` runs
both tiers against any binary.

**Home.** `specs/driver-contract/v1.md` with `v1.schema.json` beside it
(the `schema-doc` check holds the pair together), and `conformance/driver/`.
Both move to `buildwarden/buildwarden-specs` after S1 creates it, where
the suite joins S1's harness as a `driver` role (a17).

### Behaviour

Twelve things change for a user (a5). Everything else is meant to be
identical.

1. **Flags and help follow the selected driver.** `warden build --help`
   lists `warden`'s flags and the selected driver's (the container
   driver's, when none is named), where it listed every driver's. `warden
   build --driver hyperv --help` shows `--network`. The long descriptions
   of `doctor`, `setup`, `image prepare` and `clean` keep today's prose
   for the bundled drivers; a driver that is not bundled adds its own
   `help` paragraph.
2. **A flag the selected driver does not take is refused.** Today eleven
   flags are accepted and ignored on drivers that do not use them:
   `--runtime` on any driver but container; `--network` on `doctor` and
   `setup`, and `--nat-prefix` and `--host-ip` on `setup`, for any driver
   but hyperv; and on `image prepare`, `--sha256` and `--name` (hyperv
   only), `--edition`, `--virtio` and `--arch` (qemu only), `--force` and
   `--keep-vms` (qemu and hyperv only). GLOBAL-10 asks for this: a provided
   value that cannot be used is an error.
3. **The existing refusals take the generated form**, `<flag> is only used
   by --driver <names>` with the fix `drop <flag>, or use --driver
   <names>`. `--network` on `build` and `--slots` on `setup` keep their
   message and gain that fix (`--network` on firecracker had a fix of its
   own, which goes); `--update` on a driver other than vz reads `--update
   is only used by --driver vz`. The VM-group and `--guest-os` refusals
   keep their text.
4. **"Unknown driver" becomes "driver not found".** `unknown driver "x"
   (valid: container, qemu, vz, hyperv, firecracker)` is replaced by the
   message under **Discovery and trust**, since the set is no longer
   fixed. The `--driver` help names the five bundled drivers and says a
   name or a path of another is accepted.
5. **`warden clean --driver qemu` and `--driver vz` no longer run the
   container runtime's cleanup** (Decision 16). They remove stale build
   locks, report nothing else to clean, and handle the cache as before.
   With no `--driver` the driver is still container and nothing changes.
6. **A build may complete up to 10 s after its environment exits, on
   every driver.** It was 5 s on container and Hyper-V
   (`completionGrace`, `vmOffGrace`), 10 s on firecracker, and qemu and vz
   did not watch. A build that fails this way takes 5 s longer to say so
   on those two.
7. **A second Ctrl-C kills the driver and cleans up.** Today the second
   Ctrl-C kills `warden` and leaves whatever the driver had not yet torn
   down. After K1 it kills the driver and runs its `reap`; so does a
   teardown that takes more than 60 s. For a linked driver, which shares
   `warden`'s process, the second Ctrl-C behaves as today.
8. **The output directory gains `driver.json`.**
9. **`warden driver list` is new**, with `--json`.
10. **`--driver` and `WARDEN_DRIVER` accept a path** to a driver binary.
11. **A malformed `[vm] image_refresh_after` or `image_min_grace` is
    refused when its driver is selected, on every command.** Today
    `newRequest` parses the two ages for every driver, so a bad value is
    refused on `build` and `shell` whichever driver is selected, and on no
    other command. After K1 they are qemu's and hyperv's options: refused
    on each command of those two, and not looked at for another driver.
    The message keeps its text and fix and names the file the value was
    read from, where it always said `warden.toml`.
12. **`--timeout` counts from the moment `warden` starts the driver, on
    every driver.** Each driver started its own clock, at slightly
    different points of its preflight.

What does not change, although the code behind it moves:

- `warden shell` on a driver without `capabilities.shell` gives today's
  error and fix.
- `warden image list`, `image verify`, `image prepare`, `setup` and
  `doctor` for the four VM drivers: their code does not move in K1.
- `warden hyperv service ...`.
- The container driver's progress and error lines keep their text; they
  now come from the driver's stderr.
- The environment variables the drivers read for themselves (**Defaults
  and overrides**): a driver process inherits `warden`'s environment.
- Exit codes (UX-09): the build's code, 124, 130, and 1 for a blocked
  doctor with no extra line.

`docs/book/src/guide/cli.md`, `configuration.md`, `output.md` and
`drivers.md` are updated for each in the same change (UX-01). No existing
flag, subcommand, config key or environment variable is renamed or
removed (UX-03).

### What guards behaviour

- **Characterization tests, written first, on unchanged code.** Before any
  production change, `cmd/warden` gains golden tests of: `--help` for every
  command; the error and fix of every refusal, per driver; the
  `BuildRequest` each driver gets from a table of flags, environment and
  config files (extending `request_test.go`, `vmdns_test.go` and
  `buildlink_test.go`); the refusal of a malformed age and of a wrong TOML
  type, per command; `image list` and `image verify` over fixture caches;
  and the files `inputs/` holds after staging, per driver. The later
  commits may change a golden only for the twelve differences above, so
  each shows up as a reviewed golden diff.
- **The request tables become `start` tables.** The same table of flags,
  environment and files is asserted against the `start` message and its
  `options`, so a setting lost between `BuildRequest` and `start` fails a
  test.
- **The surface check.** Tier 1's extraction (Rule changes) must find the
  same set of flags and keys on the branch as on `main`. A flag or key
  lost in the move fails `surface-removed`.
- **The drivers' own tests.** Each driver's relay-start, environment and
  teardown functions keep their unit tests; only the calls into the
  session change. Firecracker's `waitBuild` tests move with the rule to
  `warden`'s build sequence.
- **`warden`'s end-to-end tests against the fake**, on every platform:
  success, failure, timeout, Ctrl-C, a second Ctrl-C, a driver that dies,
  a driver that prints noise, shell, doctor, clean.
- **The suite**, the `integration` and `firecracker` workflows, and the
  native steps under **Validation plan**.

### Order of the work

One branch, one PR, each commit green on `make test` and `make lint` for
the platforms it touches (FLOW-17).

1. Characterization tests. No production code changes.
2. `warden` stages the script and records inputs, and passes the staged
   path and the base image to each driver in the request. The drivers stop
   staging. Still `driver.Driver` and in-process sessions.
3. `driver.Host`, with `warden`'s build sequence over the session
   (`buildseq.go`), and `linked.go`. One commit per driver moves its
   `StartBuild` onto `Run`: container, firecracker, qemu, vz, hyperv (the
   two CI can run end to end first). The four-method `driver.Driver` goes
   when the last has moved. The grace becomes `warden`'s 10 s here.
4. The container driver's `Exec` is split into the held start (`mode:
   shell`) and the attach.
5. The specification and its schema; `driver/drivercmd`; the fake and
   `stubrelay`; the suite's static tier; the tier-1 surface change.
6. The exec transport and the lookup in `cmd/warden`; option declarations
   for all five drivers; `warden driver list`; `driver.json`; `warden`'s
   end-to-end tests against the fake.
7. `cmd/warden-driver-container`: the container driver's doctor, clean and
   runtime detection move out of `cmd/warden`; `reap`; the relay source;
   the container driver leaves `linked.go` (it is linked from step 3 until
   here); packaging.
8. The suite's lifecycle tier and its workflow runs; the rule text; the
   umbrella's wording; the reference docs, `structure.md` and
   `overview.md`.

Because every driver is on `warden`'s build sequence from step 3, a
failure found in one driver's native validation is in that driver's `Run`
or in the sequence, and the sequence is exercised by all five.

**Packaging.** `make build` also builds `dist/warden-driver-container`.
Release: one more goreleaser build with its own version stamp and the
container driver's image references in its ldflags (they are `warden`'s
today); the archive carries it beside `warden`, `warden-io` and
`warden-relay`; `install.sh` installs it beside `warden`. There is no
Windows release target today and K1 adds none; CI builds and tests the
binary for Windows as it does `warden` (GLOBAL-07, ORCH-05).

### Risks

- **Stdout pollution.** One stray line from a driver or its child on
  `run`'s stdout breaks the build. The kit redirects stdout; the suite
  checks every line.
- **A driver that ignores the end of its input.** `warden` kills it and
  calls `reap` on the same binary, which may be as broken.
- **Windows.** K1 kills the driver process, not its process tree (that
  needs a job object), and checks nothing about who may write a driver
  file. The container driver's resources are held by the runtime and found
  by `reap`, so K1 does not depend on either; K2 does, for Hyper-V.
- **A driver written without the kit may omit `exited`.** A dead
  environment is then noticed only by the loss of the tunnel or by the
  30 s heartbeat timeout, which starts after a first heartbeat.
- **Claim ports on loopback** can be dialled by any local process and are
  guarded only by the token, which qemu puts in its VM's command line.
  Unchanged by K1.
- **Three versions can differ**: `warden`'s, a driver's, and the relay's
  the driver starts. What holds them: the contract version, the relay's
  refusal of a claim field it does not know (with the driver's
  `old_relay_fix`), and the bundled drivers shipping in one archive.
- **K1 is large.** a15's option (b) is the fallback.
- **Carried over, not K1's:** seam 8's owner (a11); `GOOS=darwin
  GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/warden` fails at `b903f65`
  (both vz files are excluded), which is goreleaser's darwin/arm64
  configuration; and the release archive names `warden-io` and
  `warden-relay`, which are not the names `driver.GuestBinary` looks for.
  Each is reported to the parent session for intake as a fix.

### The overview (FLOW-15)

K1 is the only item of its phase, so it makes the sweep itself. The
orchestrator's pseudocode in `concepts/overview.md` changes to:

```
warden build <context>:
    driver := the driver named by --driver, WARDEN_DRIVER or [runtime] driver,
              else container: a path as given, else a linked driver, else
              warden-driver-<name> beside warden, else on PATH; not found is
              an error
    d := driver.describe                    # what it is and what it takes
    load the configuration; refuse a value of the wrong type for a key the
        driver declares, and a flag it does not declare
    req := resolve warden's flags and config (output dir, capture, ...)
    VM drivers (capabilities.vm): build link and [vm] dns, as today
    write <output>/driver.json
    script := translate(Containerfile, context)     # see Translator; not when
                                                    # the driver ignores one
    if script was translated: keep both under <output>/inputs/   # ORCH-02
    lock build-<driver>-<id>; start `driver run`; send start{id, claim token,
        script, base image, options, ...}
    <- relay-started{claim address, environment, build link, resolvers}
    session := StartSession(output dir, context, script, guest OS)
    session.ClaimRelay(claim address, claim token, the driver's values)
    -> claimed
    <- environment-started
    session.Wait()      # completion, a failure, or the driver's `exited`
                        # (a relay exit at once, a build exit after 10 s)
    -> stop; <- stopped                     # the driver tears down
    a driver that died or would not stop: kill it, `driver reap <id>`
    finalize output (optional zstd); print the result; exit with the build
        script's exit code
```

Each driver's skeleton replaces `StartBuild`:

```
run (one process per build, or a linked driver's Run):
    <- start
    preflight; resolve the relay and the environment's image   # per driver
    VM drivers: refuse a build link a host interface holds, and a launcher
                asset whose link-channel stamp is missing or old
    relay := start the relay, unconfigured, with the claim token # per driver
    -> relay-started; wait for claimed
    isolate the build environment                          # per driver
    deliver warden-io into it                              # per driver
    start it                                               # per driver
    -> environment-started; wait for stop or the end of input
    write relay.log and console.log; teardown
    -> stopped
```

A short section describes the other commands (`doctor`, `setup`, `image`,
`clean`, `shell`'s `attach`, a driver's own) as one process each, and the
lookup. `contributing/structure.md` gains `driver/drivercmd`,
`driver/relaysource`, `cmd/warden-driver-container`, `specs/driver-contract`
and `conformance/driver`.

The dead-code sweep: `make deadcode` runs over the new programs too, so a
kit function no driver or test calls is reported and removed. FLOW-15's
stated reasons for keeping an unreachable exported function do not cover
`driver/`; widening them is a rule change for K2, when the kit has
consumers outside this repository.

## Design review

### 2026-11-14, first sitting: changes requested

Read at `48acda9`. Outcome: **changes**, back to Design (FLOW-06). No rule
change was approved and no assumed question was answered. Jeff read the
contract and said it is not the shape he was looking for. His notes, in full
and in order:

> Okay, so the driver interface is interesting, but I think not quite what
> I'd initially been looking for. I think what I'm actually looking for is
> closer to each driver effectively being its own independent binary which
> warden finds in the same way git finds its individual subcommands (i.e.
> existing on PATH). The drivers would have a shared library (either inside
> the top-level warden or otherwise) that helps mediate pieces like
> configuration-parsing/reading and an interface-testing suite that checks
> that both required and optional commands are all specified correctly in the
> way the top-level orchestrator CLI can call into, but strictly speaking are
> allowed to be fully independent. This would allow for extensibility that an
> existing orchestrator can use without recompiling the orchestrator itself.

Asked how much a driver binary does (the whole build, or primitives that
`warden` calls while it keeps the session, the collector and the reporting):

> So I think it likely was more-similar to the primitives-only approach
> because I was thinking the collector would be effectively equally pluggable
> but the default option is that a driver instantiates an on-host endpoint
> that it wires in, but equally it could be a pre-existing local or remote
> endpoint that abides by the same rules, using session tokens to separate
> builds and allowing for listening to the console output and status via
> websockets or similar.

The session holding the sitting answered with the basis below, and Jeff
replied: "Yes, redraft K1 on that basis and show me the delta if it's small
enough, otherwise I'll re-review from scratch".

The basis for the redraft:

1. **The contract is the set of commands `warden` calls on a driver binary**,
   not a Go API. A driver is its own binary, found the way git finds a
   subcommand, so an existing `warden` runs a new driver without being
   rebuilt. The kit's library is a convenience a driver may ignore.
2. **Primitives only.** A driver creates the isolated environment, starts the
   relay, and wires the relay to a collector it was given. `warden` keeps the
   session, the collector and the reporting.
3. **The collector is pluggable.** By default it is an on-host endpoint for
   one build; it may equally be an existing local or remote endpoint that
   follows the same protocol, with a session token per build. The session's
   adjustment, which "on that basis" accepts and the redraft puts to Jeff as
   a question because his own words had the driver instantiate it: `warden`
   chooses or starts the collector and hands the driver an endpoint and a
   token; the driver only wires it in.
4. **The conformance suite runs against a driver binary** and checks its
   required and optional commands.
5. **Scope.** This program builds only the default collector (on-host, one
   build). The remote, multi-build collector and its client-facing side
   (open a session, supply inputs, watch console and status, fetch outputs)
   are a later phase. Left open for the redraft: whether the driver sends the
   claim or hands `warden` the claim connection.
6. **What it reaches outside this design:** the umbrella's Decisions 4 and 5
   and its seam 9 are reworded (it is merged, so the redraft asks for Jeff's
   word on the wording); most of S4 (the claim connection) is absorbed here;
   `seam-specs` writes the collector's lifetime as one build per session
   token, not one build per instance.

Most of the first draft's inventory stands (what `cmd/warden` and the shared
packages do per driver, the options, the relay source). The contract, the
conformance suite, the rule changes and the plan are redrafted.

## Consolidation

## Rollback notes

## Archive summary

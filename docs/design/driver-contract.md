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
  - {rule: FLOW-02, change: amend, paths: [docs/development/process.md]}
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
  route for this design's ten rule-change entries (FLOW-14).
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
  FLOW-02 (it names `driver.Driver`, which K1 removes), and an
  enforcement-only change to UX-01, UX-03 and ORCH-03 (tier 1's surface
  check also reads the bundled drivers' declared options). Options:
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
   working directory, which may be a build context someone else wrote, so
   a `[runtime] driver` value that contains a path separator is refused.
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

## Defaults and overrides

Each default, override and fallback K1 adds, changes or moves. Anything
not listed is unchanged and `warden`'s.

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The driver when none is named: `container` | Unchanged | It is today's default and needs no host setup | `--driver`, else `WARDEN_DRIVER`, else `[runtime] driver` (today's order). A named driver that is not found is an error; the default is not tried |
| Where a driver is found: linked, else `warden`'s own directory, else `PATH` | **New** (a16) | A driver installed with `warden` is the one meant; `PATH` is how git and kubectl find an added one | A path in `--driver` or `WARDEN_DRIVER`: that file and nothing else. A config file cannot name a path |
| A bundled name is never taken from `PATH` | **New** restriction (a16) | A file on `PATH` should not be able to replace a driver `warden` ships | A path in `--driver` or `WARDEN_DRIVER` |
| A driver file writable by group or others, or (as root) owned by neither root nor `warden`'s owner, is refused | **New** (a16) | It could have been replaced by another user | Nothing: fix the file's mode or owner |
| The collector: in `warden`'s process, one per build, reached over the tunnel | Unchanged in effect; now `warden`'s for every driver (a12) | It is today's, and it opens no listener | Nothing in K1; no flag or key is added (**The collector**) |
| Who sends the claim: `warden` | **Changed owner** (a13); the claim is unchanged | The collector's owner must hold the tunnel | Nothing in K1 |
| The contract version: the highest both sides speak | **New** | The newest common behaviour | Nothing; none in common is an error naming both |
| A build may complete for 10 s after its environment exits | **Changed** for container and Hyper-V, from 5 s (Behaviour 6, a20) | One rule in `warden` for every driver; 10 s is the longest any driver uses today | Nothing |
| After `stop`, the driver has 60 s to tear down; then it is killed and `reap` runs | **New** (a20) | Long enough for the slowest teardown known; short enough that Ctrl-C ends | A second Ctrl-C does it at once |
| `describe` and a probe answer within 10 s | **New** (a20) | Neither does host work | Nothing; a slower driver is reported as a defect |
| `warden clean` runs `reap` for every build of the selected driver whose lock nobody holds | **New** (a20) | Those builds are dead by construction: the lock is released when `warden` exits | Nothing; a held lock's build is never touched |
| `--timeout` counts from the start of `run` | **Changed** slightly (Behaviour 12) | One clock for every driver | `--timeout` |
| A driver option nobody set | Unchanged per option: absent from the request, and the driver applies its own default (`network`: `warden-dev`; `slots`: 4 for hyperv, 16 for firecracker; `nat-prefix`: `192.168.240.0/20`; `host-ip`: `192.168.240.1`; `name`: `windows-server-2025`; `arch`: the host's; the two ages: `winguest`'s; `runtime`: the first of finch, docker, podman that answers `info`) | These are the values `cmd/warden` and the drivers apply today | The flag, else the option's environment variable, else `warden.toml`, else the user config. The first that sets it is the value; it is not merged with a lower level, and a value that does not parse or cannot be used is an error naming it (UX-08) |
| A flag the selected driver does not declare | **Changed** (Behaviour 2): refused. Eleven flags were ignored | A provided value that cannot be used is an error (GLOBAL-10) | Nothing: drop the flag or select the driver that takes it. Config keys of other drivers are ignored by design |
| A declared config key whose value has the wrong type | **Changed** for the two ages (Behaviour 11): refused at load when its driver is selected | A value that cannot be used is an error, found before any work (UX-08) | Nothing: fix the value |
| Where a new or outside driver's keys live: `[driver.<name>]` | **New convention** (a4) | It cannot collide with `warden`'s tables and says whose it is | The seven existing keys keep their tables |
| `<output>/driver.json` | **New** (a19) | Which driver ran belongs with the build's output | Nothing; always written |
| `--keep-vms` leaves what `capabilities.keep` says | Unchanged per driver | Each driver keeps what it keeps today | Nothing |
| Where the container driver's relay comes from | Unchanged lookups, now behind the relay source | It runs the relay that ships with, or is built from, the same tree | `[runtime] relay_image` or `WARDEN_IMAGE`: used as given; an image that cannot be pulled is an error, and the default is not tried (a3) |
| A relay file's published hash | **New**, inert in K1: `Relay.Verify` checks a file that carries one. No K1 source sets one | Nothing publishes relay hashes yet | When a source supplies a hash, a mismatch refuses the build; no other source is tried |

Named lookups and fallbacks (GLOBAL-10, GLOBAL-11):

- **New, named: the driver lookup** (the second row). It is a search
  order, not a retry: the first match is used and its failure is the
  build's failure. `warden driver list` shows the order's result, and
  `driver.json` what ran. A path from the user ends the search.
- **Kept, existing: "beside the binary, else the source tree".** The relay
  VM's kernel and initramfs come all from beside the executable or all
  from the source tree's tool output (`relayvm.Find`), and
  `driver.GuestBinary` looks beside the executable, then in the per-tree
  build cache, then builds from the tree. It is the difference between a
  release install and a checkout, not a retry after a failure. For a
  driver binary "the executable" is the driver, so a bundled driver must
  be installed beside `warden-io` and the relay files, as the archive and
  `install.sh` do.
- **Kept, existing: container runtime autodetection** (finch, docker,
  podman), used only when no runtime is configured. It moves into
  `driver/container`.
- **Dropped: `warden clean` running the container runtime's cleanup for a
  driver that has no `clean`** (Decision 16). It was never named as a
  fallback, and it made `warden clean --driver qemu` fail for want of a
  container runtime.
- **None added.** A named driver that is not found is never replaced by
  the default or by another driver. A driver binary that fails is never
  retried through a linked one. An overridden relay never falls back to
  the bundled one. A driver without a command is never served by another
  driver's.

RELAY-08. The design adds no relay setting and moves three values between
processes without changing the channel each reaches the relay on:

| Value | Made by | Reaches the relay as |
|---|---|---|
| The claim token | `warden` (was the driver); sent to the driver in `start.claim_token` | A pre-claim value on the relay's boot channel, as today |
| The address the relay accepts its claim on | The driver; reported in `relay-started.claim.address` | A pre-claim value on the boot channel, as today |
| The environment record, the build link, the upstream resolvers | The driver; reported in `relay-started.claim_values` | The claim fields `environment`, `build_link`, `upstream_dns`, as today |
| The collector, capture mode, upstream CA settings | `warden` | The claim fields `collector_url`, `collector_token`, `capture_mode`, `upstream_ca_certs`, `upstream_system_ca`, as today |

No value travels on two channels, and a driver's message is never read by
the relay.

Environment variables. `WARDEN_DRIVER_RESULT` and `WARDEN_DRIVER_REQUEST`
are set by `warden` for a driver process and name files; they are part of
the calling convention, not settings a user sets (UX-07). `WARDEN_DRIVER`
gains a path as a value. `WARDEN_RELAY_BOOT_VHDX` and
`WARDEN_HYPERV_CACHE_DIR` stay as they are. The drivers read other
variables directly, and K1 leaves every one as it is, outside the option
model: hyperv's `WARDEN_HYPERV_BUILD_IMAGE`, `WARDEN_HYPERV_BUILD_GEN` and
`WARDEN_HYPERV_BUILD_COM_PIPE`; qemu's `WARDEN_QEMU_BUILD_VNC`,
`WARDEN_QEMU_BUILD_MONITOR`, `WARDEN_QEMU_VNC`, `WARDEN_QEMU_MONITOR`,
`WARDEN_QEMU_VNC_PASSWORD` and `WARDEN_QEMU_INSTALL_TIMEOUT_MIN`. A driver
process inherits `warden`'s environment, so they keep working when a
driver becomes a binary.

## Cross-driver impact

`drivers: [interface]`: the build sequence moves from every driver into
`warden`, and every driver changes the same way. Each gains a `Driver()`
value (its `describe` data and its `Run`); its `Run` is its `StartBuild`
without `StartSession`, `StageBuildScript`, `RecordInputs`, `ClaimRelay`,
`Wait` and `FinalizeOutput`, which `warden` now calls; it takes the build
ID, the claim token, the staged script and the base image from `start`;
and it declares its options. What differs:

| Driver | In K1 | Transport after K1 | What changes in its build |
|---|---|---|---|
| container | A binary, `warden-driver-container`, with `describe`, `run`, `reap`, `doctor`, `clean`, `attach`; its code leaves `cmd/warden`; its relay goes through the relay source | exec | `Exec` is split into a held `run` and `attach`. It no longer translates the Containerfile itself: the script and `base_image` come from `warden`. `completionGrace` (5 s) goes; it reports `exited` from `container wait`. `clean` takes `keep_ids` |
| qemu | `Driver()` with `Describe` and `Run` | linked | `linuxguest.ContainerfileImage` is replaced by `start.base_image`. It never watched its VMs' exit and still does not (`capabilities.exited: false`) |
| vz | `Driver()` with `Describe` and `Run`; `New` stops resolving the cache directory | linked | `containerfile: ignore`: `warden` drops the Containerfile before staging, as the driver did. It reports `guest_os: darwin`. `relay.log` is still written by the relay child into `diagnostics_dir` |
| hyperv | `Driver()` with `Describe` and `Run` | linked | `StartRelay` keeps creating and starting the relay VM and gives its `StartSession` and `ClaimRelay` calls to `warden`. `watchBuildVM` reports `exited`; `vmOffGrace` (5 s) goes. Its own `hyperv-build-<id>` lock stays, taken with `warden`'s build ID |
| firecracker | `Driver()` with `Describe` and `Run` | linked | The relay-exit watch during the claim and `waitBuild`'s rule move to `warden`; the driver reports both VMMs' exits as `exited`. Its slot and build locks and `Pdeathsig` are unchanged |

No driver's isolation, executor delivery or start trigger changes, and no
row of `bootstrap.md`'s per-driver mapping changes. BOOT-02's sentence on
the executor's version does (**Bootstrap comparison**). Files on the
bootstrap surface are edited, which tier 1 reports as a hint:
`driver/container/container.go` (the session calls, the split of `Exec`,
the relay source), `driver/linuxguest/` (`ContainerfileImage` goes), and
`driver/firecracker/vm*.go` where the VMM exit channels are handed to
`driver.Host`.

`driver.Session` keeps its methods; its callers change from the five
drivers to `warden`. The guest families `driver/linuxguest` and
`driver/winguest` change only by the removal of `ContainerfileImage`.

## Bootstrap comparison

Required because K1 changes one invariant's wording (BOOT-07). The
umbrella's review accepted a comparison for BOOT-02 and gave the change to
K2, with the wording "comes from the kit version the driver was built
from". Two things differ now: a driver need not be built from the kit, and
the first separate driver binary ships in K1, not K2. So K1 carries its
own comparison, and the review must accept this section explicitly (a8).

### Current model

The diagram in [`bootstrap.md`](../development/bootstrap.md), unchanged on
`main`. The lines that matter here:

```
  HOST (warden + collector)                    BUILD ENVIRONMENT

  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, version-matched               (seed disk, cloud-init ISO,
                                               copied-in file, ...)
```

### Proposed model

```
  HOST (warden + collector; the driver)        BUILD ENVIRONMENT

  (2) deliver the executor ----------------->  warden-io + a launcher
      per build, the driver's version          (seed disk, cloud-init ISO,
                                               copied-in file, ...)
```

Everything else in the diagram stands, including `collector <==== tunnel
==== relay`: the tunnel is still opened by the host, now always by
`warden`. The per-driver mapping and the deviation register do not change.

### Per invariant

| Invariant | Current | Proposed | Per-driver effect |
|---|---|---|---|
| BOOT-01 isolation precedes execution | As written. Each driver starts the relay, claims it, then starts the environment | Unchanged. The order is now the order of the `run` messages: the driver may not start the environment before `claimed` | None. The suite's Sequence case checks the order on a driver binary |
| BOOT-02 the executor is the only out-of-band input | "It is delivered per build and matches the orchestrator's version." | "It is delivered per build by the driver and matches the version of the driver that delivers it; a driver linked into `warden` has `warden`'s version." | container: the executor is the one beside `warden-driver-container`, which ships in the same archive as `warden`; they differ only if a user installs a driver from another release. qemu, vz, hyperv, firecracker: linked in K1, so `warden`'s version, as today. D2 (vz) is unchanged |
| BOOT-03 one start, then hands off | As written. `warden shell` is exempt once `initialize` has finished setup | Unchanged. `attach` is the shell's exec, under the same exemption | None |
| BOOT-04 build inputs come from the relay | The script and context are served by the collector through the relay | Unchanged. `warden` stages the script and the collector serves it; the driver never delivers it | None |
| BOOT-05 setup is the executor's domain | As written | Unchanged | None |
| BOOT-06 images carry hooks, not content | As written; boot-channel values come per build | Unchanged. The claim token still arrives per build; `warden` makes it and the driver puts it on the boot channel | None |

### Gains

- An installed `warden` can run a driver released later, with the executor
  that driver was tested with.
- The rule says who delivers the executor. Today "the host" hides that it
  is the driver's code.

### Costs

- **Security posture.** The executor is the one input that does not pass
  through the relay, and it is now chosen by a program `warden` found by
  lookup. For a bundled driver that is the same archive as today. For a
  driver from outside, `warden` cannot check which executor it delivers
  (**Security note**).
- **The record of what the build consumed.** Unchanged: the executor was
  not recorded in the ledger before and is not now. `driver.json` adds the
  driver's version and file digest, unsigned.
- **Three versions can differ** (`warden`, driver, relay with executor).
  The executor and the relay agree through the relay guest API; `warden`
  and the relay through the claim, where a relay that does not know a
  field refuses it by name.

## Rule changes

Ten entries. The umbrella's table gives K1 GLOBAL-08 and ORCH-01 "for
direction", worded for a Go API; both are reworded here for a command
contract. ORCH-02, ORCH-04, TEST-05, BOOT-02 and FLOW-02 are what the
redraft makes true or stale, and UX-01, UX-03 and ORCH-03 change in
enforcement only (a8). Each is approved by name in the design review
(FLOW-19) and made in Implement (FLOW-14). Each is worded so that it is
true when K1 merges, with four drivers still linked (umbrella Decision 18).

### GLOBAL-08 (amend), `docs/development/global.md`, `tools/tier1/rules.yaml`

**Why:** GLOBAL-08 says today that `driver.Driver` is "not covered by this
obligation". After K1 a driver is a program run through a written command
contract with a conformance suite, and an unmodified `warden` runs one it
was not built with. So the sentence a10 held back in the first draft, that
a driver the project did not write is a supported consumer, is true in K1.
The rule also says that the orchestrator "treats the relay and executor as
fixed, non-substitutable strong dependencies". `warden` can no longer
enforce that over a driver it did not ship; the amendment keeps the
sentence as the orchestrator's stance and states it as a requirement on a
conforming driver, with what `warden` does about it (a19). The umbrella's
other additions to the seam list (the truststore endpoints, the build-link
contract) belong to S2 and S3. `seam-specs` (S1) amends the executor
bullet and adds a paragraph before the last one; this change leaves both
alone, so the two amendments do not meet, and whichever item lands second
merges `main` and keeps both. If S1 lands first, the quotation below is
stale by S1's bullet and paragraph, and Consolidation refreshes it.

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
> - Driver contract: how `warden` finds a driver binary (`warden-driver-<name>`),
>   the commands it runs on it, and the messages of a build, as specified in
>   [`specs/driver-contract/`](../../specs/driver-contract/v1.md) and checked by
>   the driver conformance suite (`conformance/driver/`, TEST-05). A driver the
>   project did not write is a supported consumer. The Go packages under
>   `driver/` are a kit for writing a driver and are not part of the seam.
>
> Pre-1.0 these seams may still move. Post-1.0 they are stable and change rarely and
> deliberately, with explicit interface versioning. The driver contract is
> supported but **unstable before 1.0**: it may change in any release with no
> deprecation window, and a change to it updates its specification and its suite
> in the same change. The orchestrator's other internals (`cmd/warden`, and the
> Go API of the kit under `driver/`) are **not** covered by this obligation.
> Opening the driver contract does not make the orchestrator composable the
> other way: a conforming driver starts BuildWarden's relay and delivers
> BuildWarden's executor, and substitutes neither. `warden` cannot verify that
> of a driver it did not ship; it records which driver binary ran, in the
> build's output directory.
>
> The automated part requires a change touching a versioned seam to also change that
> seam's contract document.

**Severity and check:** MUST, `automated + agent-judgment`, before and after.

**Enforcement:** `seams` in `tools/tier1/rules.yaml` gains one entry, so the
existing `seam-contract` check covers the driver contract: `{name:
driver-contract, code: [driver/drivercmd/, cmd/warden/drivercall.go,
cmd/warden/driverlookup.go], contract: specs/driver-contract/v1.md}`. A
change to the kit's or `warden`'s side of the wire without the
specification then fails tier 1. The suite is not listed: a check added to
it for a requirement the specification already states is not a change to
the seam. The comment above the list is corrected to match. S1 fills the
same list for seams 1, 2, 3, 5 and 6; the entries do not overlap, and
whichever lands second merges the list.

### ORCH-01 (amend), `docs/development/orchestrator.md`, `tools/tier1/rules.yaml`

**Why:** the rule points at `driver.Driver`, which K1 removes, and says
shared lifecycle lives in `driver.Session` "and the other `driver/`
helpers", called by each driver. After K1 the shared lifecycle is
`warden`'s build sequence, a driver holds only what its platform needs,
and a driver written without the kit shares no Go code at all. Parity is
held by the contract and its suite. The parenthesis naming planned
drivers is stale and goes.

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
> A build-lifecycle change is expressed in the driver contract (GLOBAL-08) and
> held by its conformance suite (TEST-05), or it explicitly names which
> driver(s) it touches and asserts the others are unaffected. This is the
> standing, permanent form of the cross-contamination check: a change made for
> one driver must not silently alter the behavior of another.
>
> Lifecycle that every driver shares lives in `warden`: staging, the session
> (`driver.Session`) with its collector and claim, the wait, and reporting. A
> driver holds what its platform needs: starting the relay, the build
> environment, and their teardown. Helpers that drivers built on the kit share
> live under `driver/`; such a driver re-implements one only where its platform
> genuinely differs, and the design says why.
>
> The claim is the design's `drivers` front-matter (FLOW-02). The automated part
> compares which `driver/*` trees the diff actually reaches against that claim,
> and runs the driver suite against each driver binary this repository builds
> (TEST-05); the agent part judges whether shared code was altered in a
> driver-specific way.

**Severity and check:** MUST, `automated + agent-judgment`, before and after.

**Enforcement:** the `drivers-claim` check is unchanged. Its `drivers.shared`
list in `tools/tier1/rules.yaml` gains `driver/drivercmd/` and
`driver/relaysource/`, and `driver/buildlink/` and `driver/resolvers/`,
which every VM driver already depends on and the list omits today; a
design that changes one of them must then declare every driver. The suite
runs as tests (`make test` for the static tier), not as a tier-1 check, so
`rules.yaml`'s `ORCH-01: {check: [drivers-claim]}` stays as it is. The
suite's own imports (nothing under `driver/` or `cmd/warden`) are held by
a test under `make test` too: a new tier-1 check ID would make `main`'s
tier 1 reject the branch's `rules.yaml` (FLOW-14).

### ORCH-02 (amend), `docs/development/orchestrator.md`

**Why:** the rule's second paragraph has each driver record an altered
input when it stages it. Staging moves to `warden`, because `warden` owns
the collector that serves the staged script, and a driver binary should
not need a Dockerfile translator.

**Current wording** (the second paragraph; the rest of the rule is
unchanged):

> A driver that alters an input records it when it stages the input, before the
> build starts, through the shared `driver.RecordInputs`. A failed build therefore
> keeps its inputs, and a failure to record fails the build before anything runs.

**Proposed wording** (the same paragraph):

> `warden` stages the build script and records an altered input
> (`driver.RecordInputs`) before it starts the driver. A driver alters no input:
> it is given the staged script and the build context and changes neither. A
> failed build therefore keeps its inputs, and a failure to record fails the
> build before anything runs.

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. No enforcement change: `rules.yaml` lists ORCH-02 as a follow-up.

### ORCH-04 (amend), `docs/development/orchestrator.md`

**Why:** teardown "reachable from the clean path" assumed one process.
With a driver in its own process, two cases need a rule: `warden` dies and
the driver lives, and the driver dies. The contract answers both, and the
rule should hold a future driver to the answers.

**Current wording** (the rule's body; none of it changes):

> Teardown covers only the **transient** resources an operation allocates up to
> finalization: networks, subnets, containers, VMs, temp dirs, per-build
> pflash/vars, downloaded install media, mounts, and partial or aborted image builds.
> Anything the orchestrator allocates in this class has a matching teardown reachable
> from the clean path.
>
> It explicitly does **not** touch **durable outputs** that are the deliberate product
> of an operation: a finalized cached base image, or a completed ledger/output
> directory. Those are removed only via explicit commands (`warden image` management,
> an explicit clean target), never by the automatic teardown path. A freshly prepared
> image is safe from its own build's cleanup.
>
> The agent distinguishes transient allocation from durable output.

**Added,** as a new paragraph after the first:

> A driver run as a separate binary tears down what it created for a build
> when `warden` tells it to stop or when its `run` input ends, whichever
> comes first, so a killed `warden` leaves nothing running. What a killed
> driver leaves is reachable from the build ID alone: `reap <build-id>`
> removes it, succeeds when nothing is left, and is run by `warden` after a
> driver that died and by `warden clean` for every build no running `warden`
> holds.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change; the suite's Stop, End of input and Kill cases exercise
it on a driver binary.

### TEST-05 (amend), `docs/development/testing.md`, `tools/tier1/rules.yaml`

**Why:** TEST-05 lists the contract tests of the versioned seams. The
driver contract is a new seam with a suite of its own. `seam-specs` (S1)
rewrites the rule's body for the relay's and the collector's suites; this
change adds one paragraph at the rule's end and touches nothing S1
rewrites, so it reads the same after either wording.

**Current wording:**

> ### TEST-05 — Contract tests for the seams
>
> **Severity:** MUST
> **Check:** automated + agent-judgment
>
> The relay and executor public contracts (the GLOBAL-08 versioned seams) carry
> conformance / round-trip tests, so a third-party substitute has something to verify
> against:
>
> - RELAY-02 byte-exact round-trip (sent equals recorded),
> - RELAY-05 signature round-trip plus reader backward-compatibility,
> - EXEC-04 protocol conformance.

**Added,** as a new last paragraph:

> The driver contract carries a conformance suite that runs against a driver
> binary (`conformance/driver/`), so a driver written by anyone, in any
> language, has something to verify against. Its static tier runs in tier 0
> against every driver binary this repository builds; its lifecycle tier runs
> real builds on a host that can run the driver (TEST-02).

**Severity and check:** MUST, `automated + agent-judgment`, before and after.

**Enforcement:** `rules.yaml`'s owner string for TEST-05 becomes `"make
test (the round-trip tests exist as tests; the driver suite's static tier
runs there)"`. S1 changes the same string; whichever lands second keeps
both halves.

### BOOT-02 (amend), `docs/development/bootstrap.md`

**Why:** "matches the orchestrator's version" is false from the day an
installed `warden` runs a driver binary from another release, which K1
makes possible for the container driver. See **Bootstrap comparison**,
which the review accepts explicitly (BOOT-07).

**Current wording** (the rule's body):

> The host delivers exactly one thing to the build environment outside the relay:
> the executor (`warden-io`) and the minimal launcher that starts it. It is
> delivered per build and matches the orchestrator's version. The build script,
> context files, the CA and extension material never travel this way.

**Proposed wording:**

> The host delivers exactly one thing to the build environment outside the relay:
> the executor (`warden-io`) and the minimal launcher that starts it. It is
> delivered per build by the driver and matches the version of the driver that
> delivers it; a driver linked into `warden` has `warden`'s version. The build
> script, context files, the CA and extension material never travel this way.

In the model diagram, the header `HOST (warden + collector)` becomes `HOST
(warden + collector; the driver)` and `per build, version-matched` becomes
`per build, the driver's version`. The mapping and the deviation register
do not change.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change. The `bootstrap-surface` check sees the edit to
`bootstrap.md` beside a design whose `rules` cites BOOT-07.

### FLOW-02 (amend), `docs/development/process.md`

**Why:** FLOW-02 names `driver.Driver` twice, as what `interface` in a
design's `drivers` means and as a relationship `alters_relationships`
covers. K1 removes that interface. The meaning is kept and the name
updated; nothing about how designs declare impact changes.

**Current wording** (the two bullets; the rest of the rule is unchanged):

> - `drivers` — which drivers are touched, or `interface` for a change made at the
>   `driver.Driver` / `driver.Session` level (ORCH-01).

> - `alters_relationships` — `true` when the change alters a component's role or
>   how components interact: a versioned seam (GLOBAL-08), the bootstrap model
>   (BOOT rules), which component owns a responsibility (relay, collector,
>   executor, orchestrator, a driver), or the `driver.Driver` / `driver.Session`
>   contract. Changes inside one component's existing role are `false`. FLOW-05
>   limits how many such features are in flight.

**Proposed wording:**

> - `drivers` — which drivers are touched, or `interface` for a change made at the
>   level of the driver contract or `driver.Session` (ORCH-01).

> - `alters_relationships` — `true` when the change alters a component's role or
>   how components interact: a versioned seam (GLOBAL-08), the bootstrap model
>   (BOOT rules), which component owns a responsibility (relay, collector,
>   executor, orchestrator, a driver), or the driver contract or
>   `driver.Session`. Changes inside one component's existing role are `false`.
>   FLOW-05 limits how many such features are in flight.

**Severity and check:** MUST, `automated + agent-judgment`, before and
after. No enforcement change: tier 1 reads the value `interface`, not the
sentence.

### UX-01, UX-03 and ORCH-03 (amend, enforcement only), `tools/tier1/surface/`

**Why:** tier 1 reads the user-facing surface from the cobra flag calls and
`toml` struct tags in `cmd/warden/` (`surface.dir`). Driver-owned flags and
keys become option declarations in the bundled drivers' packages,
registered by `warden` from data. Unchanged, the check would report every
moved flag and key as removed (UX-03) and would stop comparing them with
the reference docs (UX-01, ORCH-03). One file, `surface.go`, enforces all
three rules, so the edit is a change to each (FLOW-14) and each has an
entry.

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
  `surface.dir`, `Check` reads the bundled drivers' declared options. It
  loads, at the base and at HEAD, the non-test Go files of `surface.dir`,
  of `driver/` and of each directory directly under `driver/` (found by
  listing `driver/` at that revision; the root is a constant in
  `surface.go`). In them `Extract` reads composite literals of
  `drivercmd.Option` (`Option` inside package `drivercmd`), including the
  elements of a `[]drivercmd.Option` literal, whose type is elided: a
  literal with a non-empty `Flag` yields that flag, and one with `Config:
  "table.key"` yields key `key`, the same identity a `toml` tag gives
  today. Nothing else is read from the driver directories. The check runs
  when the diff touches `surface.dir` or `driver/`. Tests and a fixture
  (`tools/tier1/surface/testdata/driver.go.txt`) cover the new form.
- Effect on findings. The branch's tier 1 extracts the same set of flags
  and keys at the base (all in `cmd/warden`) and at HEAD (split between
  `cmd/warden` and the drivers' option literals), plus `driver list`'s
  `--json`, so it reports nothing but the additions, which the docs in the
  diff cover. `main`'s tier 1 reads the branch's `rules.yaml`, which it can
  parse, and sees `cmd/warden` only. It reports `surface-removed` (UX-03)
  for each moved flag and key whose name no longer appears as a string
  literal in `cmd/warden`; the check takes a surviving literal for an
  alias, so a common word such as `name` or `force` may go unreported. It
  reports no `surface-docs` finding (UX-01, ORCH-03), because `cli.md` and
  `configuration.md` are in the diff and that check passes whenever a
  reference doc is touched. So the findings the branch drops are all on
  UX-03, and that entry clears them; the UX-01 and ORCH-03 entries clear
  the edit to the file, not a finding. After K1 a bundled driver's new flag
  or key is checked against the docs exactly as a `cmd/warden` flag is. A
  driver from outside documents its own surface; tier 1 does not see it.

The GLOBAL-08, ORCH-01 and TEST-05 entries edit `rules.yaml` within its
present format (`seams` gains an entry, `drivers.shared` four paths, one
owner string changes), which `main`'s tier 1 reads without change.

## Amendments to the umbrella design

[`componentization`](componentization.md) is merged and archived, and its
Decisions 1 to 17 are Jeff's. The review's basis changes some of its
sentences. Each is listed with its current and proposed wording, to be
made in this item's PR once the design review approves them (a18). Nothing
here touches the freeze, the program's scope, the track structure or any
other decision; those are FLOW-20's and the umbrella's.

A dated note is added under the umbrella's Decisions heading: "Decisions 4
and 5, seam 9 and the K1, K2 and S4 rows were reworded on 2026-11-14 by
the design review of `driver-contract` (K1)." Line numbers are the
umbrella's on `main` at `b903f65`.

**A. The decisions and seam 9.**

| Where | Current | Proposed |
|---|---|---|
| Decision 4 (116) | "**Drivers become separate binaries on a public Go kit, then separate modules, then separate repositories.** Alternatives: only complete the in-tree contract (does not let an integrator ship a driver); drivers as RPC plugins (a second protocol to version, and no gain over a binary that links the kit). Hyper-V is extracted first as the pilot." | "**Drivers become separate binaries that `warden` runs through a written command contract, then separate modules, then separate repositories.** A public Go kit implements the contract for a driver that wants it; a driver may be written without it, in any language. Alternatives: only complete the in-tree contract (does not let an integrator ship a driver); a Go API that every driver links (an existing `warden` could not run a new driver without being rebuilt); drivers as long-lived RPC plugin processes behind a plugin framework (commands, JSON and the standard streams are enough). The command contract is a second protocol to version, and that cost is accepted. Hyper-V is extracted first as the pilot." |
| Decision 5 (121) | "**A top-level `warden` dispatcher that works like git's CLI.** It finds a driver binary by name and runs it. Alternative: users call driver binaries directly; it lost because driver auto-detection and one entry point are worth keeping." | "**`warden` finds a driver binary the way git finds a subcommand, and stays the orchestrator.** It keeps the session, the collector and the reporting, and calls the driver's commands for primitives: start the relay, create the environment, tear both down. Alternatives: users call driver binaries directly (one entry point and one configuration are worth keeping); a dispatcher that hands the whole build to the driver (every driver would carry the session and a collector, and the collector could not be chosen apart from the driver)." |
| The seams, intro (262) | "Seams 1 to 8 each get a specification and a black-box conformance suite in the specs repository. Seam 9 is a Go API. Seam 10 is a document." | "Seams 1 to 9 each get a specification and a black-box conformance suite in the specs repository; seam 9's start in this repository (K1) and move there after S1. Seam 10 is a document." |
| Seam 9 (276) | "Driver contract \| Kit and drivers \| The four-method `driver.Driver` \| New Go API: registration, doctor, setup, images, clean, driver-owned configuration, the relay source and the claim connection; plus the convention by which the dispatcher finds and runs a driver binary." | "Driver contract \| `warden` and a driver binary \| The four-method `driver.Driver` \| New command contract: how `warden` finds a driver binary, and the required and optional commands it runs on it (describe, the build, cleanup after a dead driver, doctor, setup, images, clean, shell, a driver's own), with driver-owned configuration and the claim endpoint. A Go kit implements it; the relay source is the kit's." |

**B. The phases and the order.**

| Where | Current | Proposed |
|---|---|---|
| S4 (298) | "The claim connection: `driver.Session` takes a connection from the driver, not a TCP address. \| Every driver claims through it." | "Mostly absorbed into K1, which settles that `warden` claims at an endpoint the driver reports. What remains: the claim specification's transport section, and a claim endpoint that is not TCP. \| A relay reachable only over a non-TCP endpoint is claimed without a driver-side forward." |
| K1 (304) | "Complete the driver contract in-tree, including the relay source. `cmd/warden` stops naming drivers. `drivertest` exists. \| Adding a driver touches only its own directory." | "The driver command contract, its conformance suite and the kit. `warden` runs a driver binary; the container driver is one, and every driver's build runs through `warden`'s build sequence. \| An unmodified `warden` runs a driver it was not built with, and the suite passes against the container driver." |
| K2 (305) | "Each driver becomes its own Go module and binary in-tree. `warden` becomes the dispatcher. A tier-1 check limits each driver to the kit's public packages. \| The check passes for all five, and `warden` runs each driver as a separate binary." | "Each VM driver becomes a binary; then every driver becomes its own Go module in-tree. `warden` links no driver. A tier-1 check limits each driver to the kit's public packages. \| The check and the suite pass for all five, and `warden` runs each driver as a separate binary." |
| K4 (307) | "This repository holds the kit, the collector, the executor, the dispatcher and the book." | "This repository holds the kit, the collector, the executor, `warden` and the book." |
| K5 (308) | "The dispatcher can report and fetch a newer release of one driver." | "`warden` can report and fetch a newer release of one driver binary." |
| Order, wave 2 (327) | "S2, S3, S4, K1, M2 \| Driver contract complete" | "S2, S3, K1, M2 \| `warden` runs a driver binary; the suite passes" (what remains of S4 moves to wave 3) |
| Order, wave 3 | "K2, M3 \| Modules isolated; unikernel and host mode pass the suites" | "K2, M3, S4 \| Modules isolated; unikernel and host mode pass the suites" |
| What sets the order (335) | "K1 does not depend on the relay and can start as soon as P0 merges." | Unchanged, with one sentence added: "Its specification and suite start in this repository and move to the specs repository after S1." |

**C. The tables and prose that follow from A and B.**

| Where | Current | Proposed |
|---|---|---|
| Component table, Kit (251) | "Session, claim client, ledger reader, script translator, the driver contract, `drivertest`, the `warden` dispatcher, the book" | "`warden` (the orchestrator: session, claim client, collector hand-off), the kit's Go library for drivers, ledger reader, script translator, the book" |
| Component table, Drivers (254) | "One driver, as its own binary built on the kit" | "One driver, as its own binary that speaks the driver contract; the project's own are built on the kit" |
| Rule table, GLOBAL-08 (391) | "The driver contract becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints, the build-link contract and the claim connection. \| K1; S2, S3, S4 for their seams" | "The driver contract (the commands `warden` runs on a driver binary) becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints and the build-link contract. \| K1; S2, S3 for their seams" |
| Rule table, BOOT-02 (392) | "BOOT-02, BOOT-05, the `bootstrap.md` mapping \| As in the Bootstrap comparison. \| K2; S2; S3" | "... \| As in the Bootstrap comparison; BOOT-02 as in K1's own comparison. \| K1; S2; S3" |
| Rule table, TEST-05 (395) | "RELAY-05, TEST-05 \| Contract tests for the seams become the specs repository's suites. \| S1" | "... \| S1; K1 for the driver suite" |
| Rule table, ORCH-01 (396) | "Driver parity becomes passing `drivertest`. \| K1" | "Driver parity is held by the driver contract and its suite. \| K1" |
| Rule table, new rows | none | "ORCH-02 \| `warden` stages and records inputs; a driver alters none. \| K1" and "ORCH-04 \| A driver tears down when its input ends; `reap` reaches what a killed driver left. \| K1" |
| Risks (415) | "**A public Go API** has to be kept usable for integrators." | "**A command contract** has to be kept usable for integrators, and is a second protocol to version beside the kit's Go API." |
| Defaults and overrides (420) | "which driver the dispatcher picks when none is named and how it finds a driver binary (K2)" | "which driver `warden` picks when none is named and how it finds a driver binary (K1)" |
| Cross-driver impact (427) | "it implements the completed driver contract (K1), becomes a module and a binary (K2), declares its build-link class (S3), supplies its claim connection (S4)" | "its build runs through `warden`'s build sequence (K1), it becomes a binary that speaks the driver contract (K1 for container, K2 for the rest) and a module (K2), declares its build-link class (S3), reports its claim endpoint (K1)" |
| Bootstrap comparison, proposed diagram (469) | "per build, from the driver's own kit version" | "per build, the driver's version" |
| Bootstrap comparison, BOOT-02 row (490) | "It is delivered per build and comes from the kit version the driver was built from. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. \| None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. \| K2" | "It is delivered per build by the driver and matches the version of the driver that delivers it. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. \| None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. \| K1 (its own comparison)" |
| Security note (597) | "The kit cannot make an integrator's driver safe; the build-link suite and `drivertest` are what an integrator runs to show theirs is." | "`warden` runs whichever binary of that name it finds, and records it; it cannot make an integrator's driver safe. The build-link suite and the driver suite are what an integrator runs to show theirs is." |
| Validation plan (616) | "`drivertest` and the build-link suite validate each driver." | "The driver suite and the build-link suite validate each driver." |
| Archive summary, Decisions taken (755) | "Drivers as separate binaries on a public Go kit, then modules, then repositories, with Hyper-V as the pilot (4), behind a `warden` dispatcher that works like git's (5)." | "Drivers as separate binaries that `warden` runs through a written command contract, then modules, then repositories, with Hyper-V as the pilot (4); `warden` finds them as git finds a subcommand and stays the orchestrator (5); both reworded 2026-11-14." |
| Archive summary, Rejected (775) | "Completing only the in-tree driver contract; drivers as RPC plugins; users calling driver binaries directly." | "Completing only the in-tree driver contract; a Go API every driver links; drivers behind an RPC plugin framework; users calling driver binaries directly; a dispatcher that hands the whole build to the driver." |
| Front matter (8) | "the program's reach (the `warden` dispatcher)" | "the program's reach (`warden` and the driver binaries)" |

What stands: Decision 8 and a2 (the collector is specified like every
seam, with no storage boundary and no alternative collector in this
program), which the basis's fifth point repeats; Decisions 6 and 7; seam
1's row (the driver still supplies where the claim is made); seam 8; and
every other row. The umbrella's concepts gain no tag.

## Security note

**Boundary:** host to build environment, and with it environment to
relay. The driver is the code that builds the isolation between the host
and the environment and that starts the relay, the environment's only
network path. Until K1 that code is compiled into `warden`. After K1 it
is a program `warden` finds at run time and runs with the user's
privileges (root's, under `sudo warden setup` or `image prepare`).

**How the posture changes.**

- **Which code builds the boundary is decided by a lookup.** A file that
  `warden` takes for the selected driver controls the isolation, the
  relay and the executor of that build. What the lookup does about it:
  the five bundled names resolve only from a linked driver or from
  `warden`'s own directory, so nothing on `PATH` can replace a driver the
  project ships; the current directory and relative `PATH` entries are
  never searched; a config file, which may come from a build context,
  cannot name a path; on Windows only a `.exe` is run, never a batch file
  through `cmd.exe`; on Unix a file writable by group or others is
  refused, and under root so is a file owned by neither root nor the owner
  of the `warden` binary; the driver is resolved once per invocation and
  that path is run for every command, so `reap` runs the binary that ran
  the build. What it does not do: protect a name that is not bundled
  against a writable directory earlier in `PATH`. That is detected
  (`warden driver list` shows shadowed copies, `driver.json` records the
  path and digest), not prevented (a16, a19).
- **Root.** `sudo warden` runs the driver as root. The bundled drivers are
  found beside `warden`, which root already trusts by running it. A driver
  on `PATH` under root must pass the ownership check. a16's option (c)
  would not search `PATH` at all under root.
- **`warden` cannot vouch for a driver it did not ship.** A conforming
  driver isolates the environment, starts BuildWarden's relay and delivers
  BuildWarden's executor. `warden` cannot check any of the three, and the
  suite checks lifecycle, not isolation; S3's build-link suite is the
  isolation check an integrator runs. A build's ledger is signed by the
  relay the driver started, so a ledger from a build that used an outside
  driver is as trustworthy as that driver. `driver.json` says which driver
  it was, and is unsigned: it is a record for the person who ran the
  build, not evidence for a third party. GLOBAL-08's new wording says
  this.
- **Secrets across the process boundary.** One secret goes to the driver:
  the claim token, in the `start` message on stdin, never in argv or the
  environment. It lets its holder claim that build's relay once, so a
  driver that leaked it could let another local process claim the relay
  first; the build would then fail at `warden`'s claim, not run
  unobserved. The collector token never leaves `warden`: the driver is
  handed nothing about the collector (a13). The request and result files
  for the other commands are created 0600 and carry no secret.
- **The claim endpoint** is a TCP address the driver reports, and
  `warden` dials whatever it is told. A hostile driver could point it at
  something else; it could do worse directly, being the driver. The
  endpoint is on loopback or a host-only network for all five drivers and
  is guarded only by the token, as today; qemu's token is visible in its
  VM's command line, as today.
- **The driver's stdout is parsed by `warden`.** It is JSON decoded with a
  size limit per line; a driver's `claim_values` reach the claim only as
  the three named fields, so a driver cannot set the collector, the
  capture mode or the upstream trust through them.
- **What does not change.** No file in `relay/`, `cmd/relay`, `relaycfg/`,
  `collector/` or `cmd/warden-io` changes. No claim field, token or
  boot-channel value changes channel (RELAY-08, **Defaults and
  overrides**). The tunnel is still opened by the host and the host still
  opens no listener (RELAY-07). No isolation code changes: the container
  driver's holder and iptables sidecar, the VM drivers' topology,
  firecracker's slots and posture checks and Hyper-V's switches are edited
  only where they call the session. The order relay, claim, environment
  is now enforced by the message sequence as well (BOOT-01).
- **The relay source** is as in the first draft: `Relay.Verify` is new but
  inert, since no source in K1 supplies a hash; the two existing
  overrides let the user run a relay of their choosing, as today.

**What the TEST-08 suite should exercise.**

- Lookup: a `warden-driver-container` placed earlier on `PATH`, in the
  current directory, and in a relative `PATH` entry is not run; a
  group-writable driver file is refused; `[runtime] driver = "./x"` in a
  `warden.toml` is refused as a path; on Windows a `warden-driver-x.bat`
  is not run.
- The claim token appears in no process's argv or environment that
  `warden` starts.
- A driver that reports `claim_values` with extra fields does not change
  the claim's collector, capture or trust fields.
- A driver that starts the environment before `claimed` fails the suite's
  Sequence case.
- The existing isolation and measurement cases, on every driver, as the
  regression check that moving the session changed nothing.
- For the relay source, when hashes exist (M6): a relay file whose bytes
  do not match its published hash is refused before anything starts, and
  an unusable override is an error, never the bundled relay.

## Validation plan

**Tier 0** (`make test`, every platform, TEST-01):

- The characterization goldens (help, refusals, request tables, staged
  inputs, image list and verify), written before any production change and
  diffed across it.
- `warden`'s build sequence against a fake `driver.Host` peer: the order
  of the messages, the claim cancelled by a relay exit, the 10 s grace,
  `stop` on completion, failure, timeout and interrupt.
- The exec transport and the lookup: each lookup rule and each file check
  (with a table of directories, modes and owners), the result and request
  files, every exit code, a line that is not JSON, a driver that exits
  early, the 60 s limit and the 10 s limits with a fake clock.
- `warden`'s end-to-end tests through `warden-driver-fake` and
  `stubrelay`: build (success, failure, timeout, Ctrl-C, second Ctrl-C,
  killed driver, noisy driver), shell, doctor, clean with `keep_ids`,
  `driver list`, `driver.json`, a driver named by path.
- Options: registration for the selected driver, a clash refused,
  precedence (flag, environment, project file, user file), type errors at
  load for the selected driver's keys, refusal of an undeclared flag in
  both message forms, `[driver.<name>]` enforced for a driver that is not
  bundled.
- `driver/drivercmd`: dispatch, `describe` built from the Go value and
  valid against the schema, the result file, the error mapping, stdout
  redirected during `run`, end of input as cancellation.
- The suite's static tier against the fake and against
  `warden-driver-container`, on Linux, macOS and Windows runners; the
  suite's self-test with each of the fake's fault switches; the suite's
  imports test.
- `specs/driver-contract/v1.schema.json` against the examples in `v1.md`
  (`specs/specs_test.go`), and the `schema-doc` pair.
- `driver/relaysource`: one place or the other and never mixed; an
  override used exclusively; `Verify` on a matching and a mismatching hash.
- The rewritten `driver/claimlink_test.go`, and the tier-1 surface
  extraction tests.
- Each driver's existing unit tests, plus the tests that move with code
  from `cmd/warden` into `driver/container`.

**Tier 1** (the `integration` workflow, Docker): `make integration-test` as
today, now through `warden-driver-container`, and the suite's lifecycle
tier against it.

**Tier 2:**

- firecracker: the `firecracker` workflow on its hosted KVM runner, which
  this change triggers (`driver/firecracker/**`, `driver/*.go`,
  `cmd/warden/**`). It gains a build interrupted mid-way (nothing left on
  the slot) and a build whose VM is killed (fails after the grace). No
  native step (a7). The workflow is not a required check, so Publish
  confirms that its run on the PR's head passed.
- Native, blocking (FLOW-09), each on the branch after consolidation.
  Where a case says "as `main`", the output is captured on `main` first.

| Host | Cases |
|---|---|
| macos-finch | (1) `warden driver list` shows `container` at the built `warden-driver-container` and the four linked drivers; `warden doctor` prints `container runtime: finch` and `ok`. (2) A default build succeeds and its ledger verifies; `driver.json` names the driver binary and its digest; `inputs/` holds `Containerfile` and `build.sh`. (3) `warden shell` opens a shell and passes its exit code through. (4) `[runtime] relay_image = "dev"` builds the relay from source; `WARDEN_IMAGE=<ref>` pulls that image; an unpullable reference fails without trying the default. (5) Ctrl-C mid-build tears down and leaves nothing; with the driver killed mid-build `warden` fails the build and reaps; with `warden` killed mid-build the driver tears down by itself. (6) With `warden` and the driver both killed, `warden clean` removes that build's containers and network and leaves a running build's. (7) `WARDEN_CTR_CLI` and `--runtime` select the runtime; a missing one gives the fixable error; `--network x` is refused with the generated message. (8) A `warden-driver-container` earlier on `PATH` is not run. (9) `make driver-conformance DRIVER=dist/warden-driver-container` passes both tiers on finch |
| container-any | On the Windows host, with its container runtime: (1) `warden driver list` shows `warden-driver-container.exe`; `warden doctor` reports the runtime and `ok`. (2) A default build succeeds, its ledger verifies, `driver.json` is written. (3) Ctrl-C mid-build tears down; with the driver killed `warden` reaps; with `warden` killed the driver tears down on the end of its input. (4) `warden clean` leaves a running build and removes a build whose `warden` and driver were both killed. (5) A `warden-driver-container.bat` beside `warden` or on `PATH` is not run. (6) Both tiers of the suite pass against `warden-driver-container.exe` |
| macos-vz | (1) `warden doctor --driver vz` and `warden image list` print what `main` prints. (2) A vz build succeeds, its ledger verifies, `relay.log` is written, `driver.json` says `linked`; with a Containerfile in the context, it is ignored and `inputs/` is absent, as on `main`. (3) A script that exits 3 makes `warden` exit 3; `--timeout 30s` on a sleeping script exits 124. (4) Ctrl-C mid-build leaves no relay process and no work directory. (5) `--runtime docker` and `--network x` with `--driver vz` are refused with the generated message; `warden clean --driver vz` reports nothing to clean and needs no container runtime |
| windows-hyperv | (1) `warden doctor --driver hyperv`, elevated and not, prints what `main` prints; `--network` is honoured. (2) Elevated `warden setup --driver hyperv --slots 2` is idempotent. (3) A non-elevated durable build and an elevated ephemeral build succeed and verify; `[vm] network` is honoured and `--network` overrides it; with `[vm] image_refresh_after` set low, the detached `image prepare --driver hyperv` starts and logs. (4) With the service installed, a service-delegated build succeeds. (5) A build whose script powers the VM off without completing fails with the build-VM-off error about 10 s after the VM stops. (6) Ctrl-C mid-build removes both VMs and the switch; the time from Ctrl-C to exit is recorded, to check the 60 s limit against a real teardown (a20). (7) `--keep-vms` leaves both VMs; `warden clean --driver hyperv` then removes them and leaves a running build's |
| qemu-any | (1) `warden doctor --driver qemu` prints what `main` prints. (2) A direct-boot build and a cloud-image build succeed and verify; a Dockerfile whose `FROM` names a prepared image boots that image, as on `main`; `inputs/` holds `Containerfile` and `build.sh`. (3) Ctrl-C mid-build leaves no `qemu-system` process and no work directory; `--keep-vms` leaves the work directory only. (4) `--timeout 30s` on a sleeping script exits 124. (5) `warden clean --driver qemu` reports nothing to clean with no container runtime running |

Evidence goes under `native/<host>/<date>/` on the `workflow` branch.

- Native, tracked: macos-qemu. A qemu Windows build goes through `warden`'s
  build sequence like any other (`warden` stages `build.ps1`, starts the
  collector with the Windows bootstrap and claims). Tier 0 covers the
  staging and the `start` message; an end-to-end run needs a prepared
  Windows image, and that host's open backlog entry
  (`qemu-windows-guest-package`) shows a Windows build is not yet reliable
  there. Recorded in `tier2_gaps` and as backlog entry
  `driver-contract-qemu-windows` (TEST-03), whose text is updated for the
  redraft.

A failure that `main` already has is filed as a follow-up and does not hold
this item (FLOW-09).

## Implementation plan

Plan refines these into tasks; the order is **Order of the work**, and
each task leaves `make test` and `make lint` green (FLOW-17).

1. Add the characterization tests to `cmd/warden` on unchanged production
   code: help goldens, the refusal table, request tables per driver, the
   staged inputs per driver, `image list` and `image verify` fixtures
   (GLOBAL-04, UX-09, TEST-01, TEST-04).
2. Move staging to `warden`: `prepare` stages with
   `driver.StageBuildScript`, records with `driver.RecordInputs`, drops
   the Containerfile for vz, and passes the staged path and the `FROM`
   image in the request; each driver stops staging and translating
   (`container.stageBuildScript`, `linuxguest.ContainerfileImage`)
   (ORCH-02, BOOT-04, ORCH-01).
3. Add `driver.Host`, `driver.Start`, `driver.RelayStarted` and
   `warden`'s build sequence (`cmd/warden/buildseq.go`) over
   `driver.Session`, with `linked.go`; `warden` makes the build ID and the
   claim token and takes the `build-<driver>-<id>` lock (ORCH-01, ORCH-04,
   BOOT-01, RELAY-07, RELAY-08, GLOBAL-01).
4. Move the container driver's `StartBuild` onto `Run`; it reports
   `exited` from `container wait` (ORCH-01, BOOT-01).
5. Move the firecracker driver's `StartBuild` onto `Run`; its relay-exit
   watch and `waitBuild` rule move into the build sequence with their
   tests (ORCH-01, ORCH-04).
6. Move the qemu driver's `StartBuild` onto `Run` (ORCH-01, GLOBAL-07).
7. Move the vz driver's `StartBuild` onto `Run`; `vz.New` does no host
   lookup; it reports `guest_os: darwin` (ORCH-01, GLOBAL-07).
8. Move the hyperv driver's `StartBuild` onto `Run`: `StartRelay` gives up
   its session and claim; `watchBuildVM` reports `exited` (ORCH-01,
   ORCH-05, GLOBAL-07).
9. Remove the four-method `driver.Driver`, `newDriver` and the per-driver
   grace constants; rewrite `driver/claimlink_test.go` (ORCH-01, TEST-01).
10. Split the container driver's `Exec` into the held `Run` (`mode:
    shell`) and `Attach` (BOOT-03, UX-09).
11. Write `specs/driver-contract/v1.md` with numbered requirements and
    `v1.schema.json`, add it to `specs/README.md` and the book's specs
    pages (GLOBAL-08, RELAY-05's `schema-doc` pairing, UX-01).
12. Add `driver/drivercmd` to match the specification, with doc comments
    on every exported identifier (GLOBAL-01, GLOBAL-03, GLOBAL-08).
13. Add `conformance/driver/`: `warden-driver-fake`, `stubrelay`, the
    suite's static tier, its self-test with the fault switches, and the
    imports test; run the static tier under `make test` (TEST-01, TEST-05,
    GLOBAL-07).
14. Change tier 1's surface extraction as approved, with its tests; leave
    `rules.yaml`'s `surface` keys as they are (UX-01, UX-03, ORCH-03,
    FLOW-14, LINT-05).
15. Add the lookup (`driverlookup.go`), the exec transport
    (`drivercall.go`), `warden driver list` and `driver.json` (GLOBAL-06,
    GLOBAL-07, GLOBAL-10, GLOBAL-11, UX-02, UX-04, ORCH-04, ORCH-05).
16. Move every driver-owned flag and key into its driver's option
    declaration; `warden` finds the driver before parsing flags, registers
    the selected driver's, checks and resolves them; the VM drivers'
    doctor, setup, image and clean code reads resolved values; the
    driver-owned fields leave `Config`. Update the goldens for the twelve
    differences only (ORCH-03, UX-03, UX-07, UX-08, GLOBAL-10).
17. Add `warden`'s end-to-end tests against the fake, on every platform
    (TEST-01, TEST-04, UX-09, ORCH-04).
18. Add `cmd/warden-driver-container`: move the container driver's doctor,
    runtime detection and cleanup out of `cmd/warden`; write `reap` and
    `clean` with `keep_ids`; take it out of `linked.go`; `warden clean`
    reaps unheld builds (ORCH-01, ORCH-04, GLOBAL-07, GLOBAL-11).
19. Add `driver/relaysource` (`Bundled`, `Path`, `Image`, `Relay.Verify`)
    and route the container driver's relay through it (GLOBAL-03,
    GLOBAL-10, GLOBAL-11, RELAY-08).
20. Packaging: `make build`, the goreleaser build and archive, the
    container driver's ldflags, `install.sh` (GLOBAL-07, ORCH-05).
21. Add the suite's lifecycle tier, `make driver-conformance`, and its
    runs in the `integration` workflow; extend the `firecracker` workflow
    (TEST-02, TEST-05, TEST-07).
22. Make the approved rule edits: GLOBAL-08 in `global.md`; ORCH-01,
    ORCH-02 and ORCH-04 in `orchestrator.md`; TEST-05 in `testing.md`;
    BOOT-02 and the diagram in `bootstrap.md`; FLOW-02 in `process.md`;
    `seams`, `drivers.shared` and the TEST-05 owner in `rules.yaml`
    (FLOW-14, BOOT-07).
23. Make the approved amendments to `docs/design/componentization.md`
    (FLOW-12, FLOW-20).
24. Update `guide/cli.md`, `guide/configuration.md`, `guide/output.md` and
    `guide/drivers.md` for the twelve differences, the lookup and
    `[driver.<name>]`; `concepts/overview.md` and
    `contributing/structure.md` for the new shape; `CONTRIBUTING.md` for
    `make driver-conformance`; `getting-started/installation.md` for the
    second binary (UX-01, FLOW-15).
25. Update the tracked gap's backlog entry if Design has not (TEST-03,
    FLOW-09).

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

### Redraft r2, 2026-11-14: what changed since the first sitting

Not a sitting. Recorded for the next one (FLOW-06): the finding is the
basis above, and this is what it changed. The delta is not small, so the
next sitting reviews the design from scratch.

| Section | Since the first draft |
|---|---|
| Front matter | `seams`, `surfaces`, `security_boundary` (`host-env`), `native_validation` (container-any added), `rule_changes` (ten), `concepts` |
| Problem | The inventory stands; a paragraph added on the build living inside each driver |
| Prior art | `cmd/collector`, `seam-specs`, `container-inputs` and the outside prior art added |
| Decisions | Rewritten (17). Kept in substance: options as data, one switch for the VM settings, `[driver.<name>]`, the relay source, the dropped `clean` fallback |
| Questions | a1, a2, a6, a10 retired; a3, a4, a5, a7, a8, a9, a11 reworded; a12 to a20 new |
| Design | Rewritten: the contract is commands, with discovery, the build messages and sequence, the kit, the collector, and a suite run against a binary. The options table, the per-driver operations table and the relay source stand, reworded. Scope: only the container driver becomes a binary; the VM drivers' doctor, setup, image and clean code does not move |
| Defaults and overrides | Rewritten around the lookup, the collector, the time limits and RELAY-08's table |
| Cross-driver impact | Rewritten: `driver.Session`'s callers change, which the first draft said they did not |
| Bootstrap comparison | New (BOOT-02) |
| Rule changes | GLOBAL-08 and ORCH-01 rewritten; ORCH-02, ORCH-04, TEST-05, BOOT-02 and FLOW-02 new; the UX-01, UX-03 and ORCH-03 enforcement change stands, reading `drivercmd.Option` |
| Amendments to the umbrella design | New |
| Security note | Rewritten for `host-env` |
| Validation plan, Implementation plan | Rewritten |


## Consolidation

## Rollback notes

## Archive summary

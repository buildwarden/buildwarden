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
surfaces: [cli, config, output]  # cli: `warden driver list`, a path in --driver, a driver's
                         # flags declared in describe and shown in help; config:
                         # [driver] (the common keys) and [driver.<name>], read by the
                         # driver, with seven keys renamed into them; [build] teardown_timeout;
                         # output: <output>/driver.json
security_boundary: host-env  # the code that builds the isolation is chosen at run time by a
                         # lookup. Also env-relay: the driver starts the relay warden claims.
alters_relationships: true
design_review: required
native_validation:
  - {host: macos-finch, blocking: true, reason: "the container driver becomes a separate binary run through the commands; finch is the default runtime CI does not run"}
  - {host: container-any, blocking: true, reason: "the exec transport on a Windows host (lookup of a .exe, pipes, kill, reap) and the container driver's clean, where keep_ids is the only liveness signal because the process scan finds nothing on Windows; only this host exercises them with a real runtime"}
  - {host: macos-vz, blocking: true, reason: "driver/vz is reached: its build moves onto warden's build sequence (warden stages, starts the collector and claims, and owns the signals)"}
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv is reached: StartRelay hands its session and claim to warden, and the build-exit grace changes from 5 s to 10 s"}
  - {host: qemu-any, blocking: true, reason: "driver/qemu is reached: its build moves onto warden's build sequence, and its own SIGINT and SIGTERM handler goes"}
  - {host: macos-qemu, blocking: false, reason: "a qemu Windows build through warden's build sequence; the host's open backlog entry (qemu-windows-guest-package) shows a Windows build is not yet reliable there"}
tier2_gaps:
  - {host: macos-qemu, gap: "a qemu Windows build (--guest-os windows) through warden's build sequence: warden stages build.ps1, starts the collector and claims (backlog driver-contract-qemu-windows)"}
rules: [GLOBAL-01, GLOBAL-03, GLOBAL-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10,
        GLOBAL-11, ORCH-01, ORCH-02, ORCH-03, ORCH-04, ORCH-05, RELAY-04, RELAY-05, RELAY-07,
        RELAY-08,
        BOOT-01, BOOT-02, BOOT-03, BOOT-04, BOOT-06, BOOT-07, UX-01, UX-02, UX-03, UX-04, UX-05,
        UX-07, UX-08, UX-09, TEST-01, TEST-02, TEST-03, TEST-04, TEST-05, TEST-07, TEST-08,
        LINT-05, FLOW-01, FLOW-02, FLOW-05, FLOW-06, FLOW-07, FLOW-09, FLOW-10, FLOW-12,
        FLOW-14, FLOW-15, FLOW-17, FLOW-19, FLOW-20, LOOP-01, LOOP-02, LOOP-03, LOOP-04,
        LOOP-08, LOOP-09, PROFILE-01]
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
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md,
      tools/tier1/surface/surface.go, tools/tier1/surface/surface_test.go,
      tools/tier1/surface/testdata/driver.go.txt]}
  - {rule: PROFILE-01, change: amend, paths: [docs/development/loop-profile.md]}
  - {rule: UX-08, change: amend, paths: [docs/development/ux-cli-config.md]}
concepts: [driver-contract, driver-binaries, driver-commands, driver-discovery, driver-conformance,
           driver-options, driver-owned-config, driver-common-keys, driver-config-reading,
           single-config-file, config-key-aliases, teardown-timeout,
           kit, drivercmd, build-sequence, session, claim,
           collector, collector-handoff, relay-source, relay-release-forms, reap, build-record,
           build-lock, doctor,
           host-setup, image-prepare, clean, shell, componentization, program]
related: [componentization.md, unified-bootstrap.md, firecracker-driver.md, build-link-subnet.md,
          tier1-static-checks.md, hyperv-relay-per-build-channel.md, hyperv-driver-plan.md,
          vm-driver-release-plan.md, platform-compatibility.md, ledger-environment-types.md,
          hyperv-windows-prepare.md, vz-image-prepare-refresh.md, hyperv-clean.md,
          guest-binary-cache-per-tree.md, design-approved-rule-changes.md, design-principles.md,
          container-inputs.md, specs-migration.md, loop-contract.md]
---

# The driver command contract: `warden` runs a driver binary (componentization K1)

This is a program item (FLOW-20): phase K1 of
[`componentization`](componentization.md). It is the second draft. The first
designed a Go API that every driver links; the design review of 2026-11-14
sent it back (**Design review**, which holds Jeff's notes and the basis in
full), and this draft is written within the basis agreed there. Its first
four points, as recorded:

1. "The contract is the set of commands `warden` calls on a driver binary,
   not a Go API. A driver is its own binary, found the way git finds a
   subcommand, so an existing `warden` runs a new driver without being
   rebuilt. The kit's library is a convenience a driver may ignore."
2. "Primitives only. A driver creates the isolated environment, starts the
   relay, and wires the relay to a collector it was given. `warden` keeps
   the session, the collector and the reporting."
3. "The collector is pluggable. By default it is an on-host endpoint for
   one build; it may equally be an existing local or remote endpoint that
   follows the same protocol, with a session token per build. [...]
   `warden` chooses or starts the collector and hands the driver an
   endpoint and a token; the driver only wires it in."
4. "The conformance suite runs against a driver binary and checks its
   required and optional commands."

**Where the redraft was narrower than the basis or than Jeff's own words.**
Each row but the fourth was asked under **Questions**, and those questions
come first there. The middle column is what the redraft put to Jeff at
`0c29fc3`; the last says where he answered otherwise in the review's
second sitting on 2026-11-14, and the design below is written to those
answers.

| Jeff said, or the basis says | The redraft, as put to Jeff | Asked as |
|---|---|---|
| A driver is found "in the same way git finds its individual subcommands (i.e. existing on PATH)" | `warden`'s own directory is searched before `PATH`, the five bundled names never come from `PATH`, and a project's `warden.toml` may select only a bundled driver | a16. **Answered otherwise:** "overcontrolled". `PATH` is searched before what `warden` bundles, so a file earlier on `PATH` may replace a bundled driver; nothing checks owner or mode; one rule is kept, that a project's `warden.toml` names a driver and not a file. Since 2026-11-15 there is one config file and no user-level one; what that rule now is was asked as a26's second part and answered (A): no config file names a driver by path |
| "each driver effectively being its own independent binary" | Only the container driver is a binary in K1. The four VM drivers move onto `warden`'s build sequence and stay linked into `warden` until K2 | a15 |
| "a shared library [...] that helps mediate pieces like configuration-parsing/reading" | `warden` reads all configuration and hands a driver resolved values. The kit's part is the option declaration and typed access to the values; it reads no file | a21. **Answered otherwise:** (b). The kit reads the configuration inside the driver's process, and `warden` passes on the driver's flags and the paths of the config files. Since 2026-11-15 that is the path of one file (Decision 23) |
| A suite "that checks that both required and optional commands are all specified correctly" | As asked: a program run against a binary, in two tiers. `warden`'s calling side for every command is exercised in K1 through the reference fake | none |
| "commands" | Ten commands run once and exit; the build is one `run` process that exchanges messages with `warden` for as long as the environment lives | a14 |
| "a driver instantiates an on-host endpoint that it wires in" | `warden` starts the default collector; no driver starts one | a12 |
| The basis: `warden` "hands the driver an endpoint and a token; the driver only wires it in" | The driver is handed nothing about the collector. `warden` sends the claim itself, at an endpoint the driver reports | a13 |
| "a pre-existing local or remote endpoint [...] using session tokens to separate builds and allowing for listening to the console output and status" | Not built and not designed in K1 (the basis's fifth point). The design says what it leaves possible, and asks where the direction is written down | a22 |

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
| `host.go` | `doctorCmd`; `setupCmd`; `hypervCmd`, `hypervServiceCmd` and its three subcommands; `launcherStamps`; `hypervNetwork`; `driverName`; the `--network`, `--nat-prefix`, `--host-ip` and `--slots` flags | Doctor is a five-way switch (`hyperv.Detect` and `hyperv.Doctor`; `firecracker.Doctor`; `qemu.Preflight` and `qemu.Resolvers`; `vz.Preflight` and `vz.Resolvers`; `resolveRuntime` for the container driver). Setup calls `firecracker.RunSetup` or `hyperv.RunSetup` and prints a line for the others. `warden hyperv service install`, `uninstall` and `run` call `hyperv.InstallService`, `UninstallService`, `RunService` |
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

Signals are split the same way. `warden` handles the first Ctrl-C and then
stops listening (`buildContext`, `cmd/warden/main.go`), so a second one
kills it, and it does not handle SIGTERM at all. qemu and firecracker
install their own SIGINT and SIGTERM handler for the whole of `StartBuild`
(`driver/qemu/driver.go`, `driver/firecracker/driver_linux.go`). So on
those two a second Ctrl-C is ignored and SIGTERM tears the build down,
while on container, vz and hyperv a second Ctrl-C kills `warden` and
SIGTERM kills it outright, leaving whatever the build had created.

So an integrator who wants a driver must fork `cmd/warden` and rebuild it,
and the Rust relay has nowhere to be adopted. GLOBAL-08 says the first on
purpose: `driver.Driver` is "not covered by this obligation". The umbrella
reverses that, and the design review asked for more than the first draft
gave: a driver as its own binary that an unmodified `warden` finds and
runs. K1 is the item that makes that true for one real driver (container),
and puts all five on one build sequence in `warden`. For the four that
stay linked, that proves the order of the steps and nothing about running
them as separate processes (Decision 14).

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
  route for this design's eleven rule-change entries (FLOW-14).
- [`loop-contract`](loop-contract.md) (merged on 2026-11-15, after the
  review of this design began): the **Specification changes** section
  (LOOP-02), this repository's loop profile (PROFILE-01), which K1 amends,
  and FLOW-14 as amended, which lets no feature's design declare a change
  to a loop rule (**Rule changes**, on FLOW-02).
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
| git subcommands | The name convention and the search on `PATH`. Not taken, by the review's answer to a16: git searches its own exec path before `PATH`, which keeps a planted `git-commit` from replacing a core command. Here `PATH` comes first, so that a bundled driver can be replaced by a customized one without a flag |
| kubectl plugins | A `list` that shows the resolved path and every shadowed copy |
| Docker CLI plugins | One mandatory metadata command; end of input on a host-held channel as the cancel and host-death signal. Not taken: running every candidate's metadata command to draw help |
| CNI | A supported-versions list the host picks from; an error object with reserved codes; teardown that succeeds when nothing exists and may be repeated; a garbage-collection verb given the live set |
| containerd shim v2 | A separate cleanup command that the host runs when it cannot reach the process that held the environment. Not taken: that it is given only the ID (**The build record**) |
| csi-sanity, OCI runtime-tools | A suite run against a binary, whose skips come from what the binary declares, so "declared but broken" and "implemented but undeclared" both fail |
| go-plugin, Terraform | Not taken: gRPC (it ends "any language, no library"). Its known failure, a stray line on stdout breaking the handshake, is a suite check here |

Every design relied on already has front-matter (FLOW-12).

## Decisions

Five terms used from here on. A **bundled** driver is one of the five
in-tree names (`container`, `qemu`, `vz`, `hyperv`, `firecracker`). A
**linked** driver is a bundled driver compiled into `warden`: the four VM
drivers in K1, none after K2. The **bundled copy** of a name is what
`warden` carries for it: a file `warden-driver-<name>` in the directory
of `warden`'s own executable (in K1 the container driver's binary, built
and installed with `warden`), or the linked driver; a file of the same
name anywhere else is the user's. That file is the bundled copy however
the lookup reaches it, on `PATH` (where `warden`'s directory usually is)
or beside `warden`. The **exec transport** is `warden` running a
driver binary as a child process and speaking the contract to it; the
**linked transport** is the same sequence made as Go calls into a linked
driver. The **build dialogue** is the exchange of messages between `warden`
and a driver's `run` during one build.

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
5. **Lookup: an explicit path, then `PATH`, then what `warden` bundles; a
   config file may name a driver but not a file.** A path comes from
   `--driver` or `WARDEN_DRIVER`, and is used exclusively. Otherwise `warden-driver-<name>` is searched
   on `PATH`, in `PATH`'s own order, and only when `PATH` has none does
   `warden` take the copy it bundles: a file of that name in its own
   directory, else a driver linked into it. So a `warden-driver-container`
   earlier on `PATH` than the bundled one replaces it (anywhere on `PATH`,
   when `warden`'s own directory is not on it), and customizing an
   official driver needs no flag. Nothing checks who owns a driver file
   or who may write it. The one rule kept is that `warden.toml` cannot
   name a path, so a cloned repository cannot point `warden` at a driver
   binary it ships. The answer to a16 worded that rule for two files: a
   path "comes from `--driver`, `WARDEN_DRIVER` or the user's own
   config". On 2026-11-15 Jeff decided that `warden` reads one config
   file (Decision 23), so the third of those places is gone, and the
   design takes the rule as it reads without it: no config file names a
   path, whether it is `./warden.toml` or a file named with `--config`.
   That reading was asked as a26's second part and answered (A) on
   2026-11-15. Alternatives: the
   redraft's guarded lookup (`warden`'s own directory before `PATH`, the
   five bundled names never from `PATH`, a project file limited to bundled
   drivers, owner and mode checks on Unix), which the review rejected as
   "overcontrolled": the lookup is a convenience for the user, and it "is
   never outside of the user's control (i.e. never a part of the build
   environment)"; and what `warden` bundles before `PATH` (a customized
   bundled driver would then need `--driver <path>` on every command;
   asked as a follow-up and answered "PATH wins"); and `warden`'s own
   directory always searched last, even where it is a `PATH` entry (a
   copy anywhere on `PATH` would then win, but the order would no longer
   be `PATH`'s, the one order a user already knows, and the answer as
   recorded is for a copy "earlier on PATH") (a16). And, for the rule
   with one file: a path refused in the defaulted `./warden.toml` and
   accepted in a file the executor named with `--config` or
   `WARDEN_CONFIG` (the named file would stand where the user's config
   stood, but `warden` would treat one file in two ways by how it was
   found, which is the shape `project-config-trust` rejects for every
   other key, its Decision 7); a path
   accepted in any config file (a cloned repository's `warden.toml`
   could then name a binary the repository ships, which is what the
   rule was kept to prevent) (a26).
6. **No auto-detection of the driver.** None exists today and K1 adds
   none. Selection is unchanged: `--driver`, `WARDEN_DRIVER`, `[runtime]
   driver`, else `container`. A named driver that is not found is an
   error, never the default (GLOBAL-10). The umbrella's Decision 5 gave
   "driver auto-detection" as a reason; the amendment drops the phrase
   (a16, a18).
7. **A driver reads its own configuration, in its own process; the kit
   does it for a driver built on it** (and in `warden`'s process for the
   four drivers linked in K1). `warden` reads its own flags and
   keys and hands a driver three things: the path of the one config file
   it read, when there is one (Decision 23); the driver's flags from the
   command line; and its environment. The kit reads that file with the
   precedence `warden` uses (ORCH-03) and gives the driver typed values, validation
   and the error form; a driver written without the kit follows the
   contract's requirements for the same. This is a21's answer (b), "The
   kit reads it in the driver", and it is what Jeff's first note asked
   for: "a shared library [...] that helps mediate pieces like
   configuration-parsing/reading". Its costs are accepted with it:
   `warden` cannot check a driver's value or key before the driver's
   probe, help depends on the driver, tier 1 reads the surface only of
   the bundled drivers, and a driver written without the kit
   re-implements the precedence and, if it has keys, needs a parser for
   the TOML version `warden` reads (v1.1.0). Alternatives: `warden` reads everything and hands the driver
   resolved values (the redraft's recommendation, not taken in the
   review); `warden` parses the file and hands each driver its own
   table as JSON (a driver would need no TOML parser, but the seven
   existing keys sit in `warden`'s tables, so `warden` would again have
   to know which key is whose, and it is not what was answered) (a21).
8. **The shared VM settings stay `warden`'s, and `capabilities.vm` is the
   one switch for them.** The VM drivers take `--script`, `--image`,
   `--cpus`, `--memory`, `--keep-vms`, `--build-link`, `[vm] cpus`,
   `memory_mb`, `build_link` and `dns` as a group today. They have one
   meaning for every VM driver, `warden` checks the link and the
   resolvers as the config file loads, and it needs three of them itself
   (`--script` to stage, `--guest-os` for the bootstrap config,
   `--keep-vms` for `keep`). So they stay `warden`'s flags and keys,
   sent in `start.vm` to a driver that declares `vm`. Alternative, which
   is a21's answer followed to its end: each VM driver reads `--image`,
   `--cpus`, `--memory`, `--build-link` and the four keys itself, through
   a helper in the kit. `warden` would then know nothing of a VM's size
   or link, but `[vm]` would become a table several drivers read, the
   link would be checked in each driver's probe and no longer as the
   config file loads, and the refusal of a VM flag on the container driver would
   become the unknown-flag refusal. Answered (a) for K1, and considered
   again in K2 (a9).
9. **Driver-owned configuration: a small set of common keys in
   `[driver]`; a driver's overrides of those, and its own keys, in
   `[driver.<name>]`.** This is a4's answer, in Jeff's words: "A small
   subset of common keys under driver with overrides and custom options
   under driver.\<name>". The contract's specification names the common
   keys, each with its type and meaning, so that a driver from outside
   reads them as the bundled drivers do. Version 1 names two,
   `image_refresh_after` and `image_min_grace`: the only keys that more
   than one bundled driver reads today and means the same thing by. The
   list belongs to the contract version: it may change while version 1
   is unstable, and once a version is stable a new common key is a new
   version. A driver lists in `describe` the common keys it honours
   and the keys of its own; under its own table it may have any shape.
   The driver's own table is read before `[driver]`. There is one
   config file (Decision 23), so that is the whole order: a setting's
   flag, then its environment variable, then `[driver.<name>]`, then
   `[driver]`, then the driver's default. The design read at `5808805`
   had a user-level file as well and seven steps; Jeff did not confirm
   that order and decided on one file (a25). Now that a driver reads the
   file itself, nothing physical stops it reading any table, so the
   place is a rule of the contract, which `warden`, the suite and the
   kit each hold a `describe` to. The seven existing driver-owned keys take their places
   in the two tables, and their old names are still read as deprecated
   aliases (UX-03): a25's answer, (a). They are kept until 1.0: a26's
   answer, (i). Alternatives: an old name as an error at once, as two
   other changes Jeff decided on the same day are (asked as a26's
   option (ii) and rejected: it refuses configs that work, where UX-03
   gives a rename the alias route); `[driver.<name>]` alone, with the
   seven keys left where they are, which the redraft recommended and the
   review did not take; an order across a project's file and a user's
   (moot: there is one file); the shared VM settings as
   common keys now (they are `warden`'s for K1, a9); a driver reading
   any table (collisions with `warden`'s tables and between drivers,
   and nothing left to check) (a4).
10. **The kit is a Go library a driver may ignore.** `driver/drivercmd`
    dispatches the commands, builds `describe`, reads the driver's
    configuration, and offers the build dialogue as `driver.Host`. The
    reference fake is written without it, to show what a driver does
    when it uses none.
11. **The relay source is a kit library each bundled driver resolves its
    relay through; it is not part of the contract.** A driver from outside
    finds the project's relay however it likes. K1 adds no user-facing relay
    override. The form and role names are K1's own and provisional: seam 8
    is not written (a3, a11).
12. **The conformance suite is a program run against a binary**, in two
    tiers: a static tier with no hypervisor, run on Linux, macOS and
    Windows runners for every PR (Decision 20), and a lifecycle tier on
    the driver's own host. It imports nothing
    under `driver/`. Alternative: Go test helpers a driver's package calls,
    as in the first draft (only a Go driver built on the kit could run
    them).
13. **The specification and the suite start in this repository** and move
    to the specs repository in K2, once S1 has created it. Alternative: K1
    waits for S1 (a17).
14. **K1 delivers the contract, the suite, the kit, a fake, `warden`'s
    calling side for every command, and the container driver as a binary.
    All five drivers move onto `warden`'s build sequence; the four VM
    drivers stay linked until K2.** What that proves, plainly: the commands
    end to end on one real driver (container: `describe`, `run`, `reap`,
    `doctor`, `clean`, `attach`) and on the fake (all eleven); and, for
    the four linked drivers, the order of the steps in one process. It
    does not prove a VM driver as a separate process: teardown on the end
    of its input, `reap`, kill and process groups, the vz entitlement in a
    child, and the Hyper-V service's parentage are K2's. Version 1 of the
    contract may therefore still change in K2, which GLOBAL-08's "unstable
    before 1.0" allows; that risk is accepted. Alternatives: container
    only, VM drivers untouched (two build sequences in `warden` until K2,
    and no evidence that the sequence fits vz and Hyper-V, the two hard
    cases); all five as binaries now (a Windows
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
18. **`warden` keeps a record of each build beside its lock, and cleanup
    works from the record.** The record holds the driver binary that ran
    the build, the build's temporary directory, and the driver's flags
    and the path of the config file it was started with; the driver may keep a
    state file of its own beside
    it. `reap`, `attach` and `warden clean` get those values, not the
    current invocation's. Alternative: `reap` works "from the build ID
    alone", as this redraft first had it. Rejected: what the container
    driver creates lives in a runtime and a temporary directory that the
    ID does not name, so a `reap` run later, or with another `--runtime`
    or `--tmpdir`, would look in the wrong place, find nothing and report
    success (a20).
19. **`warden` owns the signals of a build, for every driver.** SIGINT and
    SIGTERM both mean "stop": tear down, then exit. qemu's and
    firecracker's own handlers go. Alternative: leave each driver's
    handling as it is (a linked driver's handler inside `Run` would end
    the build behind the build sequence's back, and a driver binary sees
    no terminal signal at all) (a5).
20. **A three-platform CI job runs K1's tests.** The `test` job of
    `main.yml` runs on Linux only, and no workflow builds `warden` for
    Windows. K1 adds code that differs per platform (process groups,
    kill, the `.exe` lookup, the pseudo-terminal of `attach`), so it adds
    one job that runs those packages' tests on
    Linux, macOS and Windows runners. Alternative: Linux in CI and the
    other two platforms only in native validation (a7).
21. **The contract's time limits are fixed, except the teardown wait,
    which is `warden`'s key `[build] teardown_timeout`** (60 s unless
    set, from 1 s to 1 h). It is `warden`'s and not the driver's: it is
    how long the caller waits before it kills, it is the same for every
    driver, one written without the kit included, and a driver that is
    stuck cannot be asked to time itself. Alternatives: every limit
    fixed, which the redraft recommended and the review did not take; a
    key of each driver (the wait would then be read by the process it is
    meant to bound); a flag as well (not added: the answer asked for a
    key, nothing about one invocation changes how long a teardown
    takes, and a flag can be added later without breaking anything; the
    key is therefore set in the config file only, as `[vm] dns` is)
    (a20). The `reap` that `warden` runs at the end of a build takes the
    same wait, so the key bounds the whole of a stop.
22. **`describe` declares a driver's flags and whether each takes a
    value, and `warden` refuses a flag that nobody declares.** `warden`
    cannot parse a command line without knowing, for each flag, whether
    the next argument is its value; so the one thing about a driver's
    configuration that `warden` is told is the names and arity of its
    flags, with a usage line for help. It follows that `warden` can
    still refuse a mistyped flag before any work, which a21's option
    text gave up; it reads no value. Alternatives: a `--` separator
    before the driver's flags (every driver flag in use today would move
    behind it); a rule that a driver's flag is written `--name=value`
    (`--runtime docker` would stop working); passing an unknown flag on
    with a guessed arity (builds the wrong thing silently). Answered (a),
    as recommended (a24).
23. **K1 is written on one config file, and waits for the item that
    makes it so.** On 2026-11-15, asked which of two config files wins
    for a driver's key, Jeff answered that there is to be one: "there
    should only be one authoritative warden.toml for any build" (a25,
    with his words in full). `warden` reads one `warden.toml`, the
    environment and the command line. The file is `./warden.toml` by
    default, or the one named by a new `--config` flag or
    `WARDEN_CONFIG` variable, which is then the only one read;
    `~/.config/warden/config.toml` stops being read, and one that exists
    is an error that names it. The queue item `project-config-trust`
    carries that change (the code, the ORCH-03 and UX-07 amendments, the
    docs; its design read at `539fd38`), and this design is written to
    fit it. So the contract hands a
    driver one `config_file` and no list, no file has a level, and the
    precedence has no step between files. K1 builds none of that item's
    part. K1's Plan and Implement start after `project-config-trust` has
    merged and `main` is merged into this branch, and K1 does not merge
    first (**What K1 takes from the two configuration items**).
    Alternatives: `config_files` kept as a list that holds one entry (a
    driver written by hand would implement an order between files that
    cannot occur, and the suite would have to test it); a `level` kept
    on the one file, or a field saying whether the file was named or
    defaulted (no bundled driver would read it, now that the container
    runtime is limited from every source; a field can be added to a
    request without a new contract version if a driver's rule comes to
    need one); K1 carrying the one-file change itself (Jeff gave it to
    `project-config-trust`, and two items would then rewrite
    `LoadConfig` and amend the same sentence of ORCH-03).

## Questions

### Blocking

None. The review comes before Plan (FLOW-19), and every question below has
a recommended answer the design is complete under.

One hold follows from Decision 23 and is not a question about the
design: Plan waits for `project-config-trust` to merge. In the queue it
takes the form of a blocking question, `q1`, with a `human` step
`f2-answer-q1` that `f2-plan` needs; Jeff answers it when he has merged
that item. The attended session wrote it into the queue file on
2026-11-15, during this review and before its outcome (workflow
`ea23718`): the step `f2-answer-q1` before `f2-plan`, and the question
`q1`, blocking and open (**What K1 takes from the two configuration
items**).

### Assumed

The first draft's a1 to a11 were never answered. Four are retired because
the redraft makes them moot (**Retired**, below); seven survive, reworded
where the new contract requires it; a12 to a26 are new. They are in two
groups: first the seven where the draft departs from Jeff's notes or from
the basis, then the rest.

**State after the review's second sitting, 2026-11-14.** Seventeen are
answered, and each carries its answer below, with the option chosen:
a16, a15, a21, a14, a12, a13, a22, a20, a19, a23, a7, a3, a11, a17, a24,
a9 and a4. Thirteen were confirmed as recommended. Four were answered
otherwise and are folded into the design: a16 (lookup and trust), a21
(who reads the configuration), a20 (the teardown wait) and a4 (where a
driver's configuration lives). Each question's text and options are kept
as they were put, so "Proceeding with" in an answered question describes
what was asked, not always what the design now does. Four are open: a5,
a18 and a8, and a25, which a4's answer raised (what becomes of the seven
existing driver-owned keys).

**State on 2026-11-15, after a25 and a18.** Nineteen are answered: a25
and a18 were answered that day, both (a). One thing asked beside a25's
letter was not confirmed, the order between the two tables across two
config files; Jeff's answer to it was that there is one config file,
and that is folded in as Decision 23. Three were then open: a5 and a8,
each reworded for that fold-in, and a26, which the fold-in and two
decisions of the same day raised. a8 now lists eleven rule changes: the
eleventh, UX-08, was added that evening, when the wording it edits was
approved in `runtime-cli-project-path`'s review.

**State on 2026-11-15, after a5, a26 and a8.** All twenty-two
are answered, and none is open: a5, a26 (both of its parts) and a8 were
answered that evening, each as recommended, after Jeff read the design
at `3f2012a`. The design was already written on those answers, so they
change no decision, requirement or rule wording. One thing they settle
that the design had left without a date: the seven old key names are
kept until 1.0 (a26). One answer carries a condition that was not yet
met when this was written: a18's, that the umbrella's list is shown
once more before the design is approved.

**Where the draft departs from your notes or the basis**

- **a16. Lookup and trust.** Your note: found "in the same way git finds
  its individual subcommands (i.e. existing on PATH)". Proceeding with
  Decisions 5 and 6 and the checks under **Discovery and trust**: a path
  in `--driver` or `WARDEN_DRIVER`; else a linked driver; else `warden`'s
  own directory; else each absolute `PATH` entry; the five bundled names
  never from `PATH`; a project's `warden.toml` may select only a bundled
  driver and never a path; no batch files; no auto-detection of the
  driver. On Unix the file found must be owned by root, by the user or by
  the owner of the `warden` executable, and must not be writable by group
  or others; a file in `warden`'s own directory is instead held to
  `warden`'s own owner and mode, and for a `PATH` hit the directory must
  not be writable by group or others either. Options: (a) as stated; (b)
  `PATH` only, as your note described: a planted bundled name wins, and
  `sudo warden` does not find a driver installed beside it; (c) as (a),
  and `PATH` is not searched at all when `warden` runs as root; (d) as
  (a), without the rule on the project file: a `warden.toml` in the
  working directory can then make `warden` run any `warden-driver-<x>` on
  `PATH`, on `--help` too. Recommended: (a). Confirm.

  **Answered 2026-11-14: changed.** None of the four as written; nearest
  is (a)'s lookup with almost every guard removed. Jeff's words: "This is
  overcontrolled. The nicety of this is for UX functionality and given
  this is never outside of the user's control (i.e. never a part of the
  build environment), I'd say let's allow for the first option with way
  less guarding around its official drivers, allowing for both customized
  implementations of existing drivers and new ones, either by absolute or
  PATH search". Two follow-ups, each asked on its own. When a name is
  both bundled and on `PATH`, which runs: **"PATH wins"**, so a
  `warden-driver-container` earlier on `PATH` replaces the bundled one and
  customizing an official driver needs no flag; `warden driver list` and
  the build's `driver.json` show which file ran. Which guards stay:
  **"Only: no path in a project file"**, so there is no owner or mode
  check and no bundled-only rule, and a project's `warden.toml` may name
  a driver but not a file; a path comes from `--driver`, `WARDEN_DRIVER`
  or the user's own config. Folded in as Decision 5 and **Discovery and
  trust**: a path, then `PATH`, then the bundled copy (a file beside
  `warden`, then a linked driver).

  *2026-11-15.* The answer names three places a path may come from, and
  the third, "the user's own config", went with the user-level file
  (a25's answer; Decision 23). The answer is not changed. What the guard
  is with one config file was asked as the second part of a26, and
  answered (A): no config file names a driver by path.
- **a15. K1's scope and done-when, against K2, K3 and S4.** Your note:
  "each driver effectively being its own independent binary". Proceeding
  with Decision 14: only the container driver is a binary in K1. K1 is
  done when an unmodified `warden` runs every command of the contract
  through a driver binary it was not built with (the fake, in CI on three
  platforms); the suite passes against the fake and
  `warden-driver-container`; `cmd/warden` names the four VM drivers only
  in `linked.go` and in their existing doctor, setup, image and clean
  code; and native validation passes on each blocking host. What K1 does
  not prove is in Decision 14: a VM driver as a separate process. K2
  makes each VM driver a binary and removes `linked.go`, moves the
  specification and the suite to the specs repository, then makes every
  driver a module. K3 keeps its row. S4 is absorbed, except the claim
  specification's transport section and a claim endpoint that is not TCP.
  One item and one PR. Options: (a) as stated; (b) container only, the VM
  drivers untouched: macos-finch and container-any blocking and the three
  VM hosts not needed, but two build sequences in `warden` until K2 and
  no evidence that the sequence fits vz and Hyper-V; (c) all five as
  binaries now, with a Windows release target and the vz signing move:
  five native validations of new process boundaries in one PR; (d) as
  (a), split into two items at the point the fake passes; (e) as (a), but
  the VM drivers' flags and keys stay in `cmd/warden` until each becomes
  a binary: their doctor, setup, image and clean code is then not touched
  twice, at the cost of two option paths in `cmd/warden` until K2, and
  Behaviour 1 to 3 and 13 arrive per driver in K2. Recommended: (a); (b)
  is the fallback if K1 proves too large.

  **Answered 2026-11-14: (a), confirmed as recommended.** Decision 14: only the container
  driver is a binary in K1; all five run on `warden`'s one build
  sequence; the four VM drivers stay linked until K2.

- **a21. Who reads the configuration?** Your note: "a shared library
  (either inside the top-level warden or otherwise) that helps mediate
  pieces like configuration-parsing/reading". Proceeding with Decision 7:
  `warden` reads the files, the environment and the flags; a driver
  declares its options in `describe` and receives resolved values; the
  kit gives a Go driver the `Option` declaration and typed access to the
  values (`req.Options.String("runtime")`), and reads no file. What
  follows from it: an option has one of seven scalar types, a driver has
  no configuration of a shape of its own, and `warden` runs `describe`
  before it parses flags. Options: (a) as stated; (b) the kit reads the
  configuration inside the driver's process, and `warden` passes through
  the flags it does not know and the paths of the config files: a driver
  may then have configuration of any shape, but `warden` cannot draw
  help, refuse an unknown flag or check a value before slow work, tier 1
  cannot read the surface, and a driver written without the kit
  re-implements the precedence. Recommended: (a). Confirm.

  **Answered 2026-11-14: (b),** "The kit reads it in the driver". This
  departs from the recommendation, and it is what your first note asked
  for. Folded in as Decision 7 and **Configuration**: `warden` reads its
  own flags and keys and hands a driver the paths of the config files,
  its flags from the command line and the environment; the kit reads
  the same files with the same precedence in the driver's process, in
  `warden`'s process too for the four linked drivers; `describe`
  declares a driver's flags for parsing and help and its keys for the
  placement rule, and nothing of their values. Three things in option
  (b)'s text turned out narrower than stated, and one wider. `warden`
  can still refuse a flag nobody declares, because it cannot parse a
  command line otherwise (a24). It can still print help for the selected
  driver's flags, from their usage lines. Tier 1 can still read the
  surface of the bundled drivers, from their Go literals. And a driver
  written without the kit needs a TOML parser as well as the
  precedence. What this reopens is asked again below: a4, a9, a5, a8,
  and a24, which is new.

  *2026-11-15.* "The paths of the config files" is now the path of one
  file, `config_file` (Decision 23).
- **a14. One `run` process per build, or run-and-exit verbs?** Your note
  spoke of "commands". Proceeding with one process per build that
  exchanges messages with `warden` (Decision 2); the other ten commands
  run once and exit. Options: (a) as stated; (b) `create`, `start`,
  `state`, `stop` verbs, each run once: nearer "commands", but vz needs a
  holder daemon, and a killed `warden` leaves VMs running. Recommended:
  (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** One `run` process
  per build; the other commands run once and exit.

- **a12. Who starts the default collector?** Your note had the driver
  "instantiate an on-host endpoint that it wires in". The basis adjusted
  that, and this asks for your word on the adjustment. Proceeding with:
  `warden` starts today's in-process collector, one per build with a fresh
  token, and the driver starts none. Options: (a) as stated; (b) the driver
  starts the collector: every driver, in any language, embeds or ships a
  collector, and `warden` needs a protocol to it for console output, status
  and the output directory. Recommended: (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** `warden` starts the
  default collector, one per build with a fresh token; the driver starts
  none.

- **a13. Who sends the claim?** The basis left it open, and said `warden`
  "hands the driver an endpoint and a token". Proceeding with: `warden`
  dials the claim endpoint the driver reports and sends the claim, which
  is today's code and opens no listener on the host. The driver is then
  handed nothing about the collector, which is narrower than the basis.
  Options: (a) as stated; (b) the driver sends the claim, given the
  collector's endpoint and token: the default collector must then listen
  where the relay can reach it, which `driver/session.go` rules out on
  purpose (and firecracker's relay cannot open a connection to the host
  at all), or the driver bridges the tunnel for the whole build.
  Recommended: (a). (b) stays addable for a remote collector: `start`
  reserves `collector`, and `claim.network` is an enumeration. Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** `warden` dials the
  claim endpoint the driver reports and sends the claim; `start` keeps
  the reserved `collector` field.

- **a22. Where the pluggable collector is written down.** Your second
  note describes a collector that may be "a pre-existing local or remote
  endpoint that abides by the same rules, using session tokens to
  separate builds and allowing for listening to the console output and
  status via websockets or similar". The basis puts it in "a later
  phase", but the umbrella has no such phase: its Decision 8 says
  alternative collectors are "a goal for later, not for this program",
  and "Later, not scheduled" names only "alternative collectors and a
  collector repository". So the direction is recorded only here.
  Proceeding with: K1 builds none of it and leaves it possible (**The
  collector**), and one clause is added to the umbrella's "Later, not
  scheduled" naming what you described (**Amendments**, section C).
  Options: (a) as stated; (b) also file a queue item for it now, on hold
  behind the freeze; (c) record nothing outside this design. Recommended:
  (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** One clause added to
  the umbrella's "Later, not scheduled"; nothing queued.

**The rest**

- **a5. The behaviour that changes.** Proceeding with the eighteen
  differences listed under **Behaviour**. In short: (1) flags and help
  follow the selected driver, and the driver's part of help comes from
  its `describe`; (2) a flag that neither `warden` nor the selected
  driver declares is refused, where eleven were silently ignored, and so
  is `--runtime` on a command that selects no driver and, under the
  container driver, on `setup` and `image prepare`, which that driver
  does not implement; (3) the three
  refusals that name another driver (`--network`, `--slots`, `--update`)
  become the one unknown-flag refusal, which names the selected driver
  only; (4) "unknown driver" becomes "driver not found", with
  what was searched, and a `warden.toml` whose `[runtime] driver` is a
  path is refused; (5) `warden clean --driver qemu|vz` no longer
  cleans containers; (6) `warden clean` first cleans up after dead builds
  of the selected driver; (7) the build-exit grace is 10 s on every
  driver (it was 5 s on container and Hyper-V); (8) a second Ctrl-C kills
  a driver binary and reaps, where it killed `warden` (a third, or a
  `reap` that outlasts the teardown wait, ends the `reap`), and on linked
  qemu and firecracker it now ends `warden`, where it was ignored; (9) SIGTERM
  tears the build down on every driver, where it killed `warden` on
  three; (10) the output directory gains `driver.json`, written with
  `inputs/` before the driver starts; (11) `warden driver list` is new;
  (12) a path is accepted in `--driver` and `WARDEN_DRIVER`; (13) a driver's keys are read only by that driver and only when
  it is selected: a malformed `[vm] image_refresh_after` or
  `image_min_grace`, and a wrong TOML type in any of the seven existing
  driver-owned keys, are refused by the driver that owns the key and
  pass unseen under another, where today the ages are refused on `build`
  and `shell` under every driver and a wrong type fails the load under
  every driver; (14)
  `--timeout` counts from the moment `warden` starts the driver; (15) a
  context or temporary path that is not valid UTF-8 is refused for a
  driver binary; (16) a `warden-driver-<name>` earlier on `PATH` than the
  bundled copy runs in place of the bundled driver of that name; (17)
  `[build] teardown_timeout` is
  new; (18) a driver's keys live in `[driver]` (the common keys) and
  `[driver.<name>]`, the seven existing driver-owned keys are renamed
  into them, each old name is still read and prints one warning line
  per command, a file that sets a key under both names is refused, and
  one driver can be given its own value of a common key.
  Re-derived on 2026-11-14 after the answers to a16, a20 and a21, and
  again after the answer to a4:
  none of the redraft's fifteen goes; 1, 3, 4, 8, 10, 12 and 13 change
  (3 loses the hint that names the driver a flag belongs to, and 13
  loses the type check of the seven keys under every driver); 16, 17
  and 18 are new. What 18 does with the old names was asked on its own
  and answered (a25, (a)).

  *Reworded on 2026-11-15, after the one-file decision* (a25's answer,
  Decision 23). Still eighteen. 4 and 12 change: there is no user-level
  file, and no config file names a driver by path (a26's second part),
  so 12 is the flag and the variable. If a26 is answered (ii) for the
  old names, 18 is rewritten to it. The list is against `main` as
  `project-config-trust` leaves it. That the user-level file is no
  longer read, and `--config` and `WARDEN_CONFIG`, are that item's
  changes and are not counted here; nor is the container runtime's limit
  to `finch`, `docker` and `podman`, which is
  `runtime-cli-project-path`'s and which K1 keeps under both names of
  the key.

  Options: (a) accept all eighteen; (b) refuse nothing new:
  `warden` also asks the other four bundled drivers what flags they
  declare and accepts and ignores those, as today (2 goes, and 3 keeps
  its hint), at the cost of running or calling every bundled driver's
  `describe` on every command; (c) for 2 and 5, UX-03's
  own route: for one release `warden` does as in (b) and warns that the
  flag is ignored for this driver, or that `clean --driver qemu` no
  longer cleans containers, and the refusal follows in the next; (d)
  for 13, `warden` keeps the seven fields in `Config` for their type
  check only, so a wrong type still fails the load under every driver,
  at the cost of a second reader in `warden` for keys it does not own;
  (e) accept some, named in the review. Recommended: (a). GLOBAL-10 asks
  for 2, and UX-03 would give it a window; the two rules pull apart
  here, and (c) is the answer if the window matters more.

  **Answered 2026-11-15: (a), confirmed as recommended.** All eighteen
  are accepted. The question's text above is as it was put. Its
  sentence "If a26 is answered (ii) for the old names, 18 is rewritten
  to it" no longer applies: a26 was answered (i), and 18 stands as
  listed, with the old names kept until 1.0. Rejected with this answer:
  (b) to (e).

- **a20. The contract's time limits and cleanup points.** Proceeding
  with: `describe` answers within 10 s and a probe within 30 s (a probe
  of `run` makes the driver's preflight); after `stop`, `warden` waits
  60 s for a driver binary to tear down, then kills it and runs `reap`; a
  build may still complete for 10 s after the driver reports its
  environment exited; `warden` runs `reap` after a driver that ended
  without `stopped` or `failed`; `warden clean` runs it for every build
  of the selected driver whose lock nobody holds, on the binary and with
  the values in that build's record (Decision 18), and removes the lock
  only when `reap` succeeds; no sweep runs before each build. None is
  configurable. What is left when both `warden` and the driver are
  killed: whatever the runtime or the hypervisor holds keeps running
  until `warden clean`, which is what a killed `warden` leaves today.
  Options: (a) as stated; (b) also sweep before every build: one listing
  of the lock directory per build, and a `reap` only when a dead build
  exists; a killed pair is then cleaned up at the next build without
  being asked, but a build removes another build's leftovers, and a
  `reap` that fails warns at a build it has nothing to do with; (c) make
  the 60 s a key. Recommended: (a). Whether 60 s covers a Hyper-V
  teardown is measured in native validation; the limit itself applies
  only to a driver binary, so K1 does not enforce it on Hyper-V. Confirm.

  **Answered 2026-11-14: (c),** the limits and cleanup points as stated,
  with the 60 s teardown wait made a configuration key. This departs
  from the recommendation. Folded in as Decision 21 and under **The
  build**, signals: the wait is `warden`'s key `[build]
  teardown_timeout`, a duration from `1s` to `1h`, `60s` when unset.
  Every other limit stays fixed.
- **a19. A driver from outside.** `warden` cannot check that a driver it
  did not ship isolates the build, or that it runs the project's relay and
  executor. Proceeding with Decision 15: accept that, say so in GLOBAL-08
  and the contract, and record the driver's name, version, path and file
  digest in `<output>/driver.json`, unsigned. Options: (a) as stated; (b)
  also require an opt-in key (`[runtime] allow_path_drivers = true`) before
  a driver found on `PATH` runs; (c) also record it in the ledger, at the
  cost of a schema version. Recommended: (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** Accept that `warden`
  cannot vet a driver from outside; say so in GLOBAL-08 and the
  contract; record name, version, path and file digest in `driver.json`,
  unsigned. No opt-in key, which agrees with the answer to a16.

- **a23. How far GLOBAL-08's wording goes in K1.** The first draft asked
  this as a10. After K1 an unmodified `warden` runs a driver it was not
  built with, so the proposed wording says "A driver the project did not
  write is a supported consumer of this contract". But a real driver from
  outside needs more than this contract (**What a driver needs besides
  this contract**): how to launch the project's relay and what to put on
  its boot channel, how to deliver and start the executor, the
  environment record, and the build-link guarantee. Those are seams 1, 5,
  7 and 8; S1 and S3 are not merged, and seam 8 has no owner (a11).
  Proceeding with: the sentence as quoted, followed by one that says the
  rest is the relay's and the executor's seams and not this contract.
  Options: (a) as stated; (b) hold the sentence back: K1's wording says
  only that `warden` runs a driver through a written contract, and
  "supported consumer" is added by the item that writes the last of
  seams 1, 5, 7 and 8. Recommended: (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** "Supported consumer
  of this contract", with the sentence that what a driver must do to the
  relay and the executor is their seams' to state, in K1.

- **a18. The umbrella's wording.** The basis changes sentences of a merged,
  archived design. Proceeding with the wording under **Amendments to the
  umbrella design**, made in this item's PR. The list covers Decisions 4
  and 5, seams 1, 6 and 9, the S4, K1, K2, K4 and K5 rows, the order, what
  M6 needs, "Later, not scheduled" (a22), and the tables and summary that
  follow from them. Options: (a) as proposed, in K1's PR; (b) as proposed,
  in a process item of its own, with K1 waiting; (c) changed in the
  review, sentence by sentence. Recommended: (a). Your word is needed on
  each sentence. Nothing is proposed to `seam-specs`: its design already
  writes the collector's lifetime per session token.

  **Answered 2026-11-15: (a), confirmed as recommended.** As proposed,
  made in place in K1's PR. As recorded with the answer: the list is
  shown to you again, checked once more against the umbrella as
  `loop-contract` amended it, before the design is approved.

- **a8. The rule changes.** The umbrella's table gives K1 GLOBAL-08 and
  ORCH-01 "for direction". Proceeding with those two reworded for a command
  contract, plus ORCH-02 (staging moves to `warden`), ORCH-04 (teardown on
  end of input, and `reap` from the build's record), TEST-05 (the driver
  suite), BOOT-02 (the executor's version is the driver's; with a
  Bootstrap comparison, since the wording differs from the one the
  umbrella's review accepted for K2), an enforcement-only change to UX-01
  and UX-03 (tier 1's surface check also reads the flags and keys the
  bundled drivers declare), PROFILE-01 (this repository's loop profile:
  four rows, below), and ORCH-03, which gets the same enforcement change,
  one paragraph (a driver reads its own flags and keys, from the config
  file `warden` names to it, with the same precedence and before any
  work, through the kit or as the contract requires; a driver run as a
  separate binary does so in its own process; the keys the contract
  names as common live in `[driver]`, and a driver's overrides of those
  and its own keys in `[driver.<name>]`, which is read before
  `[driver]`) and a clause on its last
  sentence saying where the automated part looks. GLOBAL-08's sentence on a driver
  from outside was asked on its own and confirmed (a23). Re-derived on
  2026-11-14 after the answer to a21: the list has the same ten entries;
  what differs is that ORCH-03 is now reworded and no longer
  enforcement-only, and that the surface check reads `drivercmd.Setting`
  literals. Derived once more after the answer to a4: still ten
  entries; ORCH-03's paragraph ends with the two tables and their
  order, where it said that the existing driver-owned keys keep their
  tables; UX-03's wording does not change, and the design now takes its
  route for the first time, a rename with deprecated aliases, which the
  automated part cannot see because it knows a key by the last element
  of its path (**Rule changes**; a25). a16 changes no rule, but it reaches the Bootstrap comparison
  this question accepts: BOOT-02's per-driver effect and its security
  cost now say what a file that takes a bundled name changes. UX-07 and UX-08 are kept by the design and quoted under
  **Configuration**; neither needs new words.

  *Reworded on 2026-11-15, after you read the design at `5808805`.*
  `main` at `ad4ea50` (`loop-contract`) was merged into the branch, and
  the list was derived a third time: still ten entries, but not the same
  ten.
  - *FLOW-02 is out.* It is a loop rule, and FLOW-14 and LOOP-09 as
    merged let no feature's design declare a change to one. K1's PR
    edits nothing in `process.md`.
  - *PROFILE-01 is in.* K1's PR would leave three entries of this
    repository's loop profile stale, and the rule has the change that
    does so change the entry: Specifications (it names the driver
    contract's specification and its suite), the Safety row of Checks
    (the `driver-contract` CI job), and Reference documents (the surface
    is also what the drivers declare). A fourth, Impact vocabulary,
    gains one sentence for FLOW-02's sake. That is four rows. Sweep,
    Consumables and the Specification row of Checks stay as they are
    (**Rule changes**, PROFILE-01, says why). `seam-specs` rewords the
    Specifications row as well, in other sentences of it, so you are
    asked to approve that row in two forms: as K1 leaves it alone, and
    joined with S1's words.
  - No other entry names a loop rule or edits the code of a loop rule's
    check. One list that a loop rule's check also reads, `drivers.shared`
    in `rules.yaml`, gains eight paths under ORCH-01; the check's code is
    not edited (**Rule changes**). The current wording quoted for each
    entry was compared again with `main` at `ad4ea50` and stands.

  FLOW-02 wants an answer of its own, beside the letter, because K1
  still removes `driver.Driver` and FLOW-02 names it in two bullets
  (what `interface` in a design's `drivers` means, and one of the
  relationships `alters_relationships` covers):
  (i) K1 changes nothing in FLOW-02. PROFILE-01's Impact vocabulary
  entry says what `interface` stands for in this repository, so that
  LOOP-01's reading rule, which LOOP-08 applies to a name the loop's
  detail takes from this repository, gives a design the right meaning
  from K1's merge. The two bullets are reworded by a process item, and
  K1 asks `loop-parallel` to carry them, because it already amends
  FLOW-02. Its design review has been under way since 2026-11-15: 12 of
  its 14 questions are answered (workflow `f5b7acc`), a10 and a14 are
  open, and no outcome is recorded. So the request is put to you in
  that sitting now and does not wait for this answer, because that
  sitting may end first. If that review is approved before the request
  reaches it, `loop-parallel` is not reopened for two bullets: they go
  to `loop-native-lead`, P1's fourth process item, whose design is not
  yet written, or to an item of their own as in (ii), as you choose
  then. If K1 merges first, FLOW-02's text names a Go interface that no
  longer exists from K1's merge to that item's; no check reads those
  words.
  (ii) As (i), with a process item of its own, filed now, in place of
  `loop-parallel`: one more item and one more review for two bullets.
  (iii) As (i) without the sentence in PROFILE-01: ORCH-01 as amended
  and FLOW-02's case of a versioned seam carry the meaning, and once
  FLOW-02 is made neutral nothing in this repository's rules says what
  `interface` stands for.
  (iv) K1 waits for the process item to merge before it does.
  Recommended: (i), and the design is written on it. A letter by itself
  answers the list only, and leaves (i) as a decision of the design,
  confirmed or not when the design is.

  *Reworded a second time on 2026-11-15, after the one-file decision*
  (a25's answer, Decision 23). Still ten entries, and the same ten.
  - *ORCH-03's paragraph loses its words about files.* It said "from the
    config files `warden` names to it" and ended "Within one file the
    driver's own table is read before `[driver]`; the order of the files
    does not change." It now says "the config file" and ends "which is
    read before `[driver]`". The rule's opening sentences, on the one
    file and the precedence itself, are `project-config-trust`'s to
    amend, with UX-07, and K1 changes no word of them. **Rule changes**,
    ORCH-03, says which sentences each item owns, gives the rule with
    both changes, and asks you to approve K1's paragraph and clause in
    both settings: as `main` has the rule today, and as that item leaves
    it.
  - *UX-07 is still not K1's.* It is amended by `project-config-trust`.
    UX-08's new wording is `runtime-cli-project-path`'s: the component
    that uses a setting validates it when it initializes, after the
    overrides, and only when the run uses it. K1's drivers already do
    that, and K1 changes no word of it. What K1's merge does to the list
    at the end of that wording is the next point.

  *Reworded a third time on 2026-11-15: eleven entries.* The fold-in as
  first written (`834c449`) held the UX-08 entry back, because that
  wording was not approved and its own review was still asked whether it
  should carry a list; it said the entry would be added and put to you
  here if that review approved the wording while this one was open. It
  did, seven minutes later: `runtime-cli-project-path` was approved at
  `a2afd08`, with the UX-08 rule change approved by name and the list
  kept (its a5, answered (a): "the amended UX-08 lists the four older
  checks and this item moves none").
  - *UX-08 is in, for its last paragraph only.* As approved, that
    paragraph reads: "Four checks predate this wording and are made
    whatever the run uses. As a config file loads: the TOML type of
    every key, `[vm] build_link`, and `[vm] dns`. On every `build` and
    `shell`: `[vm] image_refresh_after` and `image_min_grace`. Each
    stays until the change that moves it to its consumer, which takes it
    off this list. Nothing is added to the list." K1 is such a change
    for two of the four. The two image ages go to qemu and hyperv, which
    check them in their probe and only when selected, so that check
    leaves the list. The type of the seven driver-owned keys goes to the
    driver that owns each, so the first check narrows to `warden`'s own
    keys. `[vm] build_link` and `[vm] dns` stay (Decision 8; a9,
    answered). K1's paragraph: "Three checks predate this wording and
    are made whatever the run uses, as a config file loads: the TOML
    type of `warden`'s own keys, `[vm] build_link`, and `[vm] dns`. Each
    stays until the change that moves it to its consumer, which takes it
    off this list. Nothing is added to the list." (**Rule changes**,
    UX-08.)
  - *Nothing else of UX-08 is K1's.* Its title, its check type
    (`agent-judgment`), its first three paragraphs and the removal of
    its entry from `tools/tier1/rules.yaml` are that item's, approved
    there, and K1 changes no word of them. K1's entry has one path, the
    rule file.
  - *It is an edit of words that are not on `main` yet.* That item is in
    Plan and merges before K1's Plan starts (**What K1 takes from the
    two configuration items**), so K1's Implement finds the paragraph on
    `main`. If it does not find it as quoted, K1 edits nothing in UX-08
    and returns to Design (FLOW-06).
  - Without the entry, K1's merge would leave UX-08 naming two checks
    that are no longer made, and Implement could not correct it: FLOW-14
    clears an edit to a rule file only through an approved entry whose
    paths name the file.
  - If a5 is answered (d), `warden` keeps the type check of the seven
    keys under every driver; the first check then keeps the words "of
    every key", and only the sentence on the two ages goes.

  Options: (a) as stated;
  (b) leave BOOT-02 to K2 (the sentence "matches the orchestrator's
  version" is then false for the container driver from K1's merge until
  K2's); (c) no enforcement change, a `Tier1-Allow` trailer on the move,
  and the moved flags and keys no longer checked against the docs; (d) ORCH-03 in
  enforcement only, as the redraft had it: the rule then does not say
  that a driver's own reading keeps the precedence, and only the
  contract does. Recommended: (a). Each rule change is approved by name
  in the review, and the Bootstrap comparison is accepted explicitly
  (BOOT-07). The eleventh entry, UX-08, is in all four options.

  **Answered 2026-11-15: (a), confirmed as recommended, and (i) for
  FLOW-02.** All eleven rule changes, as stated. Each was approved by
  name the same day, on the wording at `3f2012a`, and the Bootstrap
  comparison was accepted explicitly (BOOT-07); **Design review** lists
  them, and the review's outcome was not yet recorded when this was
  written. For FLOW-02: K1 changes nothing in it. The request to
  `loop-parallel` was answered in that item's sitting the same day,
  "Yes, carry it here", so that item's FLOW-02 amendment carries the
  two bullets (**Rule changes**, FLOW-02). Rejected with this answer:
  (b), (c) and (d), and for FLOW-02 (ii), (iii) and (iv). The
  question's text above is as it was put. Its last point, "If a5 is
  answered (d)", no longer applies: a5 was answered (a), so the first
  check on UX-08's list narrows to `warden`'s own keys, as proposed.

- **a7. Native validation and the CI job.** Proceeding with macos-finch,
  container-any, macos-vz, windows-hyperv and qemu-any blocking,
  macos-qemu tracked, and linux-kvm left out. container-any is new: on a
  Windows host the exec transport (a `.exe` lookup, pipes, kill, `reap`)
  and the container driver's `clean` are exercised with a real runtime
  only there. linux-kvm: firecracker is validated by the `firecracker` CI
  workflow (tier 2 on a hosted KVM runner), which this change triggers.
  That workflow is not a required check, so Publish confirms its run on
  the PR's head passed. In CI, K1's own tests run on Linux, macOS and
  Windows runners in a new job (Decision 20); today `make test` runs in
  CI on Linux only. Options: (a) as stated; (b) container-any tracked,
  relying on the new job's run of the fake and of the static tier on a
  Windows runner, which exercises the exec transport there but no
  container runtime; (c) macos-qemu blocking too; (d) add a blocking
  linux-kvm step; (e) the three VM hosts tracked, since those drivers
  stay linked (not recommended: their build path changes on every host);
  (f) no new CI job: tier 0 runs on Linux in CI, and the macOS and
  Windows code is tested only on macos-finch and container-any.
  Recommended: (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** macos-finch,
  container-any, macos-vz, windows-hyperv and qemu-any blocking;
  macos-qemu tracked; linux-kvm left to the `firecracker` workflow; the
  new three-platform CI job.

- **a3. The relay source's scope in K1.** Proceeding with: the relay source
  is a kit library and not part of the contract (Decision 11). K1 ships its
  types and the bundled, source-tree, local-path and image sources that
  wrap today's lookups, and routes the container driver's override
  (`[runtime] relay_image` with `WARDEN_IMAGE`) through them; it adds no
  flag or key. The container driver uses it in K1. The four VM drivers
  adopt it in K2 with their binaries, and hyperv's
  `WARDEN_RELAY_BOOT_VHDX` moves then. A pinned source (download a release
  form by version and verify its published hash) waits for seam 8 (a11).
  Options: (a) as stated; (b) also route the four linked VM drivers
  through it in K1; (c) also add `--relay` and `[driver.<name>] relay`
  now; (d) also build the pinned source now, unused until M6; (e) no
  relay source in K1: the container driver keeps `resolveRelayImage`, and
  the library arrives with the first VM binary in K2, where more than one
  driver uses it (the umbrella's K1 row then loses "including the relay
  source", and M6 for the container driver waits for K2). Recommended:
  (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** The relay source is a
  kit library outside the contract; K1 ships its types and sources and
  routes the container driver's override through them; the VM drivers
  adopt it in K2.

- **a4. Where a driver's own configuration lives.** Re-derived on
  2026-11-14 after the answer to a21: the driver now reads the files
  itself, so the place of its keys is a rule and no longer something
  `warden`'s parser enforces. Proceeding with (Decision 9,
  **Configuration**): a driver's keys live in `[driver.<name>]` in
  `warden.toml` and the user's config, and under that table they may
  have any shape; the seven existing driver-owned keys keep their tables
  and names, each read only by the bundled driver named for it in the
  specification; a driver lists the keys it reads in `describe`, each
  with its TOML type and the commands that read it, and
  `warden`, the suite's static tier and the kit each refuse a listed key
  that is outside the driver's table and not one of its seven; `warden`
  defines no key under `[driver]`. What that cannot hold: a driver
  written without the kit that reads a key it did not list. Options: (a)
  as stated; (b) move the seven existing keys to `[driver.<name>]` now,
  read through deprecated aliases for a release (UX-03): one rule with
  no exception list, at the cost of a user-facing change in a freeze,
  and for `image_refresh_after` and `image_min_grace`, which qemu and
  hyperv share today, one key becoming two; (c) any shape, anywhere: a
  driver may read any table and declares no keys: nothing to list or
  check, but nothing stops a driver and `warden`, or two drivers,
  claiming one key, and a later `warden` key could change what an
  installed driver reads; (d) a file of its own per driver, beside each
  of the two config files: no shared table at all, but a second file for
  a user to find, and the seven existing keys cannot move there without
  aliases either. Recommended: (a). Confirm.

  **Answered 2026-11-14: changed.** None of the four as written. Jeff's
  words: "A small subset of common keys under driver with overrides and
  custom options under driver.\<name>". Folded in as Decision 9 and
  under **Configuration**, *Where a driver's configuration lives*:
  `[driver]` holds the common keys, which the contract's specification
  names (two in version 1, `image_refresh_after` and
  `image_min_grace`); `[driver.<name>]` holds that driver's overrides
  of them and its own keys, in any shape; in one file the driver's own
  table is read before `[driver]`, and the order of the files is
  unchanged. The sentence does not say what becomes of the seven keys
  that exist, so that is asked on its own, as a25.

- **a9. The shared VM settings: `warden`'s, passed in `start.vm`, or the
  driver's own.** Re-derived on 2026-11-14 after the answer to a21,
  which makes "the driver's own" the reading that follows the answer
  furthest. The settings are `--script`, `--image`, `--cpus`,
  `--memory`, `--keep-vms`, `--build-link`, `[vm] cpus`, `memory_mb`,
  `build_link` and `dns`. Proceeding with (Decision 8): all of them stay
  `warden`'s flags and keys; `warden` checks the build link and the
  resolvers as the files load, refuses the group for a driver without
  `capabilities.vm` with today's message, and sends the values in
  `start.vm`. Consequences: one meaning of `[vm] cpus` for every VM
  driver; a driver from outside gets them without parsing anything;
  nothing changes for a user in K1; but `warden` keeps reading four
  values it only forwards (`--image`, `--cpus`, `--memory` and their
  keys), and `[vm]` holds both `warden`'s keys and five of the seven
  driver-owned ones. Options: (a) as stated; (b) the driver's own:
  `--script`, `--guest-os` and `--keep-vms` stay `warden`'s, because it
  stages the script, picks the bootstrap config and sends `keep`; every
  VM driver declares `--image`, `--cpus`, `--memory` and `--build-link`
  as its flags and reads `[vm] cpus`, `memory_mb`, `build_link` and
  `dns` through one helper in the kit; `start.vm` goes. `warden` then
  reads nothing it does not use, and a21's rule has no exception. The
  cost: `[vm]` becomes a table that `warden` and several drivers read,
  which is the exception a4 otherwise avoids; the link and resolvers are
  checked in each driver's probe and no longer as the files load, and no
  longer against the other bundled drivers' ranges; `--cpus` on the
  container driver is refused as an unknown flag and not with `requires
  a VM driver`; and the four linked drivers' flags move in K1 where (a)
  moves none; (c) one capability per setting, with (a): a driver could
  then take `--cpus` without `--build-link`, at the cost of six
  capability names and six refusal messages where one exists, and no
  bundled driver needs it. Recommended: (a) for K1, and (b) considered
  again in K2, when each VM driver becomes a binary and its flags move
  in any case. Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** The shared VM
  settings are `warden`'s for K1, sent in `start.vm` to a driver that
  declares the `vm` capability; considered again in K2.

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

  **Answered 2026-11-14: (a), confirmed as recommended.** Provisional Go
  constants in K1; seam 8 filed as its own S-track item, to exist before
  M6 adopts a release.

- **a17. Where the contract's specification and suite live.** Proceeding
  with `specs/driver-contract/v1.md` and its schema, and
  `conformance/driver/`, in this repository, moved to
  `buildwarden/buildwarden-specs` in K2, after S1 has created it (the
  amended K2 row says so). Options: (a) as stated; (b) in the specs
  repository from the start, with K1 waiting for S1 and for Jeff to create
  the repository. Recommended: (a). Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.**
  `specs/driver-contract/v1.md` and `conformance/driver/` in this
  repository, moved to `buildwarden-specs` in K2.

- **a24. How `warden` tells a driver's flags from its own.** New on
  2026-11-14, raised by the answer to a21: once the driver reads its own
  configuration, `warden` no longer knows a driver's flags, and it
  cannot parse `warden build --x y ./ctx` without knowing whether `y`
  is the value of `--x` or the build path. Proceeding with Decision 22:
  `describe` declares each flag's name, whether it takes a value, the
  commands that take it and a usage line, for parsing and help only;
  `warden` reads no value, and refuses a flag that neither it nor the
  selected driver declares, before any work. Every flag in use today
  keeps both its spellings and its place on the command line. The cost:
  a driver cannot take a flag it did not declare, so its flags are not
  quite "of any shape", though its keys are. Options: (a) as stated; (b)
  a `--` separator: what follows it goes to the driver unread and
  undeclared (`warden build ./ctx -- --runtime docker`); `warden` then
  knows nothing of a driver's flags and help shows none, and every
  driver flag in use today moves behind `--`, which UX-03 allows only
  with an alias for each; (c) a rule of form: a driver's flag is written
  `--name=value`, and a bare `--name` is a switch; no declaration is
  needed and `warden` passes on what it does not know, but `--runtime
  docker`, the form in every script and in the docs, would take
  `docker` for the build path, and a mistyped `warden` flag would be
  passed to the driver instead of refused by `warden`; (d) as (a), and
  `--` as well, for arguments a driver cannot declare: two syntaxes,
  the second addable later without breaking anything. Recommended: (a).
  Confirm.

  **Answered 2026-11-14: (a), confirmed as recommended.** `describe`
  declares each flag's name, arity, commands and usage, for parsing and
  help only; `warden` refuses a flag nobody declares.

- **a25. What becomes of the seven existing driver-owned keys.** New on
  2026-11-14, raised by the answer to a4, which says where a driver's
  keys live and not what happens to the seven that exist: `[runtime]
  cli` and `relay_image` (container), `[vm] network`, `windows_image`
  and `windows_image_update` (hyperv), and `[vm] image_refresh_after`
  and `image_min_grace` (qemu and hyperv). UX-03 lets none of them stop
  working without an alias, a notice and a documented window.
  Proceeding with: all seven take their place in the two tables, and
  each old name is still read, as a deprecated alias. The two ages are
  the common keys `[driver] image_refresh_after` and `image_min_grace`.
  The other five become `[driver.container] cli` and `relay_image`, and
  `[driver.hyperv] network`, `windows_image` and
  `windows_image_update`. An old name counts as its new name written in
  the same file; a file that sets one key under both names is refused;
  each old name a file sets gets one warning line per command, which
  names the new one; the old names are read for at least two releases
  after the first that has the new ones, and taking them out is a
  change of its own. No flag and no environment variable changes.
  Consequences: every driver key follows one placement rule, and the
  specification's list of keys outside the two tables is seven
  deprecated names with an end. Against that: every existing config
  that sets one of the seven prints a warning until it is edited, in a
  release that already changes seventeen other things; this is the project's
  first deprecation, so the form of the notice and the length of the
  window are new conventions; and `[runtime] cli` is also the subject
  of the security fix `runtime-cli-project-path`, so whichever of the
  two merges second applies that fix's refusal under both names.
  Options: (a) as stated; (b) only the two ages move, to `[driver]`,
  with their `[vm]` names as deprecated aliases, and the five keys that
  one driver reads stay in `[runtime]` and `[vm]` for good, listed by
  driver name in the specification: five warnings fewer, but the custom
  options that exist are then not under `driver.<name>`, which holds
  only keys added from now on; (c) nothing moves: the specification
  defines `[driver]` and names no common key in version 1, both tables
  hold only keys added later, and the two ages stay one shared pair in
  `[vm]` with no override per driver: no user-facing change, and the
  shape you described has nothing in it until a key is added; (d) as
  (a) with no warning and no end: both names are read for good and
  nothing is printed, but every one of the seven has two names, and
  UX-03 asks for a notice and a window when a surface is renamed, so
  this needs your word that it is an addition and not a rename; (e) as
  (a), with another window, named in the review. Recommended: (a).
  Confirm. The five options are about the seven keys and nothing else.
  Two more things the design decides with a4's answer are not among
  them, since your sentence leaves them open as well, and each wants a
  yes or a change of its own, recorded beside the letter: (i) version
  1's common keys are the two ages and no others; (ii) in one file the
  driver's own table is read before `[driver]`, while the order of the
  files stays ORCH-03's (rejected: the more specific table winning
  across files, which would let the user's file outrank the project's
  for one setting). A letter by itself answers the seven keys only, and
  leaves (i) and (ii) as decisions of the design, confirmed or not when
  the design is.

  **Answered 2026-11-15: (a), confirmed as recommended**, for the seven
  keys: all seven move into `[driver]` and `[driver.<name>]`, each old
  name is still read as a deprecated alias and warns, and a file that
  sets a key under both names is refused. The two things asked beside
  the letter: (i) version 1's common keys are the two ages and no
  others: **confirmed**. (ii) The order between the two tables across
  the project's file and the user's: **not confirmed**. Jeff's words:
  "What do you mean by 'yours'? warden.toml is meant to be effectively
  in full control by a build executor and there shouldn't be any
  precedence necessary. Configuration should only be through a local
  'warden.toml', environment variables, and command-line arguments. By
  default, the warden.toml's location in the working directory is simply
  for ease of use, but there should only be one authoritative
  warden.toml for any build". Asked four things, each on its own, he
  chose: the user-level file goes away; the one file is `./warden.toml`
  by default, or the file named by a new `--config` flag or
  `WARDEN_CONFIG` variable, which is then the only one read; the queue
  item `project-config-trust` carries the change, and this design is
  written to fit it; and an existing `~/.config/warden/config.toml` is
  an error that names the file, with that detection removed later.
  Folded in as Decision 23: one `config_file`, no level, and the order
  flag, variable, `[driver.<name>]`, `[driver]`, default. The question's
  text above is as it was put. Its sentence on
  `runtime-cli-project-path` ("whichever of the two merges second") is
  no longer so: that item merges first, and its rule is now that the
  runtime is one of three names from every source (**What K1 takes from
  the two configuration items**).

  *2026-11-15, later.* The question's "at least two releases" is now a
  stated point: the old names are kept until 1.0 (a26, answered (i)).

- **a26. The old key names, and the one guard on the driver's file,
  after 2026-11-15.** New on 2026-11-15, raised by decisions you made
  that day after a25 was answered. It has two parts, and each wants its
  own answer.

  *The old names.* a25's answer (a) has each of the seven old key names
  keep working, with one warning line per command. The same day, for
  `runtime-cli-project-path` and for the user-level config file, you
  decided that a change that refuses what worked before is an error at
  once with a concrete message, not a warning: "Use the logic that would
  be used for the warning to instead have a concrete error message about
  what needs to be changed/updated. This logic will eventually be
  removed like the warning would be". The cases differ: those two
  changes refuse a value or a file, and an alias refuses nothing. But
  the three land close together and you may want one convention. a25's
  answer is not changed by this question; what you answer here is
  recorded here. Proceeding with (i). Options:
  (i) as answered: each old name is read as its new name and warns, for
  at least two releases, and a file that sets both is refused. (That
  evening you chose 1.0 as the removal point of
  `runtime-cli-project-path`'s transitional detection, its a4 (d). "At
  least two releases" is a floor, so keeping the old names until 1.0 as
  well is within a25's answer, and you can say so here; the design
  names no date for their removal.) No config
  stops working because of the rename. UX-03 is followed as written ("A
  rename or removal ships an alias plus a deprecation notice with a
  documented removal window"). The contract and the kit carry the
  aliases: `describe`'s `aliases`, `Setting.Aliases`, requirement C11
  with its rule for when the warning prints, and the specification's
  list of the seven old names. The release then has two conventions, a
  warning for a renamed key and an error for an unsupported runtime or a
  user-level file. And the old names change twice: taking the aliases
  out later refuses what worked, so by your rule that removal is first
  an error that names the new key, and only then is the detection
  removed;
  (ii) an error at once: a file that sets an old name is refused by the
  driver that owns the key, when that driver is selected, with a message
  that names the file, the old name and the new one, and the value is
  not used. The detection is transitional and is removed at 1.0, as
  `runtime-cli-project-path`'s is (its a4, answered (d));
  `project-config-trust` proposes that item's window for its own (its
  a3, open). One convention. The contract loses `aliases` and
  C11, and the kit loses the warning rule and the refusal of both names:
  the detection is code of the five bundled drivers, not a requirement
  on every driver. Every config that sets one of the seven fails on its
  first run with the owning driver after the upgrade, until the key is
  renamed, in a release where a user-level config must already be moved.
  An old name under another driver is not reported, since nothing reads
  it (Behaviour 13). UX-03's sentence asks for an alias, so its wording
  has to change as well, as a twelfth rule change approved by name, to
  allow an error that says what to write, with a documented removal
  window, beside the alias. Decision 9, Behaviour 18, C4, C6 and C11,
  the settings table and the validation plan are then rewritten;
  (iii) (ii) for the five keys that one driver reads and (i) for the two
  ages, or the reverse, named in the review.
  Recommended: (i). Your rule is about a change that refuses, and this
  one need not refuse; UX-03 gives a rename this route in so many words;
  and K1 is a program item under the freeze, where the fewer configs
  that break the better. (ii) is the better answer if one convention
  matters more to you than a release in which old configs keep working:
  it is simpler in the contract, and it ends in one step where (i) takes
  two.

  *The guard.* a16 kept one guard: "a project's `warden.toml` may name a
  driver, never a file path; a path comes from `--driver`,
  `WARDEN_DRIVER` or the user's own config". There is no user's own
  config now. Your reason for the guard was that a cloned repository
  must not make `warden` run a binary it ships. With one file, that
  exposure is the defaulted `./warden.toml`; a file named with
  `--config` or `WARDEN_CONFIG` is the executor's own choice.
  `project-config-trust` (its design at `539fd38`) leaves this to K1:
  "K1 decides it again". For every other key it proposes that what a
  config file may decide is not limited by where the file was found, and
  that the protection for a repository the executor does not control is
  to name the file (its Decision 7 and a4, not yet confirmed).
  Proceeding with (A). Options:
  (A) no config file names a driver by path: `[runtime] driver` is a
  name, in `./warden.toml` and in a file named with `--config` alike,
  and a path comes from `--driver` or `WARDEN_DRIVER`. One rule for
  every file, which agrees with that item's Decision 7 and with what you
  chose for the container runtime ("from every source"); `warden` need
  not know how the file was found. An executor who wants a driver by
  path every time exports `WARDEN_DRIVER` or puts the file on `PATH`. It
  gives up a path in the executor's own file, which the design at
  `5808805` allowed in the user's;
  (B) a path is refused in the defaulted `./warden.toml` and accepted,
  when absolute, in a file named with `--config` or `WARDEN_CONFIG`: the
  named file stands where the user's config stood, and is "in full
  control" of the driver. This is the nearest to a16's answer as you
  gave it. It is also what that item's Decision 7 rejects for the keys
  it covers, as "a second level by another name", so the two designs
  would then treat a defaulted file differently; and `--config
  ./warden.toml` in a cloned repository turns the guard off, by the
  executor's own act;
  (C) no guard: a path is accepted in any config file, as every other
  key of the file is under that item's Decision 7. `cd repo && warden
  build` then runs a binary that the repository's `warden.toml` names,
  at once and on `--help` too; the line that says which file was read
  stops nothing. That is what the guard was kept to prevent, and the
  kind of hole `runtime-cli-project-path` closes.
  Recommended: (A). It and (C) are the two that treat every file alike,
  as that item proposes; of the two, (A) keeps your guard, and it can be
  widened later without breaking anyone, where (C) could not be
  narrowed. A letter by itself answers the old names only, and leaves
  (A) as a decision of the design, confirmed or not when the design is.

  **Answered 2026-11-15: (i) and (A), both confirmed as recommended.**
  *The old names:* (i). Each old name works and warns, and is kept
  until 1.0 (the option as put in the sitting read "Alias and warn
  until 1.0"); a file that sets both names is refused. That is within
  a25's answer, whose "at least two releases" is a floor, and this
  answer is what names the point: 1.0, the removal point of
  `runtime-cli-project-path`'s transitional detection. Where the design
  stated the window as "at least two releases" it now says "until
  1.0"; a25's text and option (i) above are as they were put. Rejected:
  (ii), an error at once, and with it the twelfth rule change, to
  UX-03, that it would have needed; and (iii). *The guard:* (A). No
  config file names a driver by path; a path comes only from `--driver`
  or `WARDEN_DRIVER`. Rejected: (B) and (C). The design was written on
  (i) and (A), so nothing else in it changes.

### Retired

Retired by this redraft, unanswered. The queue file has no retired state
for a question, so these four are removed there and kept here; no ID is
reused. Their full text and options are in the first draft
(`git show 48acda9:docs/design/driver-contract.md`) and in the queue file
on the `workflow` branch at `3f95f20`.

- **a1** (what "adding a driver touches only its own directory" means): the
  done-when it explains is replaced. Asked now as a15.
- **a2** (the boundary with S4): K1 now settles who claims and absorbs most
  of S4. Asked now as a13 and a15.
- **a6** (one queue item or several): asked now as part of a15.
- **a10** (how far K1's GLOBAL-08 wording goes): asked again, for the
  command contract, as a23.

## Design

This item works only in this repository (LOOP-04).

### Shape

```
  warden (cmd/warden)                         a driver: warden-driver-<name>
  -------------------                         ------------------------------
  its own config and flags     -- describe -->  what it is; its flags
  the driver's flags, unread;
  the config file's path       -- run (probe)>  reads and checks its own
                                                config; preflight;
                                                creates nothing
  build ID, lock, build record
  stage the script, record inputs
  claim token                  -- run ------->  start the relay
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
cmd/warden/                    the orchestrator: commands, its own config,
                               the build sequence, the session, reporting
  driverlookup.go              finding a driver binary (Discovery and trust)
  drivercall.go                the exec transport: running a driver's commands
  linked.go                    the four VM drivers, linked until K2; the only
                               file that names them besides their doctor,
                               setup, image and clean code
  buildseq.go                  one build sequence over either transport
  buildrecord.go               the build lock's record, and cleanup from it
cmd/warden-driver-container/   the container driver's binary (a main that
                               calls drivercmd.Main)
driver/                        the kit: Go libraries for a driver that wants them
  drivercmd/                   new: command dispatch, describe, the result
                               file, reading a driver's configuration, and
                               the run dialogue
  host.go                      new: Start, Host, RelayStarted (the build
                               dialogue's Go types)
  relaysource/                 new: where a relay comes from
  session.go                   the session; warden's, no longer a driver's
  hostutil.go, hostlock.go, buildlink/, resolvers/, script/, relayvm/,
  linuxguest/, winguest/       as today (hostlock.go gains the build lock)
  container/ qemu/ vz/ hyperv/ firecracker/   the drivers
specs/driver-contract/         v1.md and v1.schema.json: the contract
conformance/driver/            the suite (driver-conformance), the reference
                               fake (warden-driver-fake) and stubrelay
```

The contract is `specs/driver-contract/v1.md`. The packages under
`driver/` are a convenience for a driver written in Go, with one
exception: `driver/session.go` (the session, its collector and the claim
client) is called only by `warden` after K1. It stays where it is in K1,
to keep the move small, and K2 moves it out of the kit when the drivers
become modules. `warden`'s side of the contract lives in `cmd/warden`, and
the suite has its own implementation of that side, so the two are checked
against the specification and not against each other.

The terms bundled, linked, bundled copy, exec transport and build
dialogue are defined at the top of **Decisions**. `warden` reaches a linked driver through the
same sequence and the same messages, as Go calls and not over a pipe.

### Discovery and trust

The lookup is a convenience for the person running `warden`, and it is
theirs to arrange: in the review's words it "is never outside of the
user's control (i.e. never a part of the build environment)" (a16). So it
searches where the user puts programs, lets a file there replace a driver
`warden` ships, and checks nothing about who owns a file. It keeps one
rule, on the one input that may not be the user's own: the config file,
which by default is the `warden.toml` of the working directory.

**Name.** A driver binary is `warden-driver-<name>`. `<name>` matches
`[a-z][a-z0-9]{0,31}` and is not the name of a `warden` command. It has no
hyphen or other punctuation because it is used as a TOML table
(`[driver.<name>]`), inside lock and record file names
(`build-<name>-<id>`) and as the `warden` subcommand that reaches the
driver's own commands. On Windows the file is `warden-driver-<name>.exe`
(below). One exception to "not the name of a `warden` command" lasts for
K1: `hyperv` is a linked driver and also the `warden` command that holds
its service helper (`warden hyperv service ...`). K2 ends it (**The
commands**, `ext`).

**Selection** is unchanged in order: `--driver`, else `WARDEN_DRIVER`,
else `[runtime] driver`, else `container` (`resolveConfig`,
`cmd/warden/main.go`). There is no auto-detection. `image verify` keeps
reading the flag only. What is new is that the value may be a path. A
value is a path when it contains a path separator (`/`, and `\` on
Windows); anything else is a name.

| Where the value is | A name | A path |
|---|---|---|
| `--driver` | yes | yes; a relative one is relative to the working directory, like any path argument |
| `WARDEN_DRIVER` | yes | yes, likewise |
| `[runtime] driver` in the config file: `./warden.toml`, or the file named with `--config` or `WARDEN_CONFIG` | yes, any driver | **no** |

- **The one rule kept: a config file may name a driver, not a file.** By
  default the file is the `warden.toml` of the working directory, and it
  may come with a build context someone else wrote. It may select any
  driver by name, which is then looked up like any other name; a value
  that is a path is refused, so a `warden.toml` cannot point `warden` at
  a driver binary, one the repository ships included. The refusal names
  the file and gives the two places a path may come from as the fix.
- The rule is the same for a file named with `--config` or
  `WARDEN_CONFIG`. a16's answer let a path come from "the user's own
  config" as well; that file is gone (Decision 23), and the design does
  not put the named file in its place: one rule for every file, and
  `warden` does not act differently by how the file was found. That is
  a26's second part, answered (A); the other reading was its option
  (B), rejected.
- A relative path in `--driver` or `WARDEN_DRIVER` is taken as typed,
  against the working directory; a user who exports a relative
  `WARDEN_DRIVER` from a shell profile gets a different file in each
  directory, and that is theirs to decide.

The rule covers the driver's file and nothing else. The container
driver's `cli` names a program too. On `main` at `ad4ea50` a
`warden.toml` in the working directory can set it to a file the
repository ships, and `warden` runs it. That is closed by the security
fix `runtime-cli-project-path`, which merges before K1: the runtime is
`finch`, `docker` or `podman`, from every source. K1 moves the reader of
the key into the container driver and keeps that rule under both names
of the key (**What K1 takes from the two configuration items**).

**Lookup**, first match wins, resolved once per `warden` invocation to an
absolute path, which every later command of that invocation runs:

1. **A path.** The file is used as given and nothing else is searched
   (GLOBAL-10). The driver's name is what its `describe` reports, which
   must match the file's base name (`warden-driver-<name>`, with `.exe`
   on Windows).
2. **`PATH`.** Each entry, in `PATH`'s order, is searched for
   `warden-driver-<name>`, and the first candidate is the driver.
   `warden`'s own directory is usually one of the entries (`install.sh`
   installs to `~/.local/bin` and asks for it on `PATH`), and the file
   beside `warden` is then met here, at that entry's place in the order.
3. **The directory of `warden`'s own executable** (`os.Executable()`,
   symlinks resolved), when `PATH` gave nothing: `warden` run by its
   path from a directory that is not on `PATH`, or under sudo's reset
   `PATH`. The container driver's binary ships here; a driver the
   project did not write may be installed here too. K1 adds no separate
   driver directory (`libexec`); whether packagers need one is K2's
   question.
4. **A driver linked into `warden`**, when the name is one (K1: `qemu`,
   `vz`, `hyperv`, `firecracker`).

**Which candidate is the bundled copy.** Candidates are compared by
identity, after symbolic links are resolved (`os.SameFile`). A candidate
that is the same file as `warden-driver-<name>` in `warden`'s own
directory is the bundled copy, and is reported as found `beside-warden`,
whichever step met it and through whatever link: a `PATH` entry that is
`warden`'s directory, a symbolic link into a package's own directory,
`/bin` beside `/usr/bin` on a system where one links to the other. Two
candidates that are one file are one candidate. So an install nobody
has touched reports `beside-warden`, and `PATH` is reported only for a
file that is not the bundled one.

**When a customized copy wins.** `PATH`'s order is literal. A
`warden-driver-container` runs in place of the bundled one when its
directory comes before `warden`'s own on `PATH`, or when `warden`'s
directory is not on `PATH` at all; nothing has to be passed to make it
so, and that is how an official driver is customized. A copy in a later
entry is passed over, as a second `git` later on `PATH` is, and `warden
driver list` shows it as shadowed. `warden driver list` and the build's
`driver.json` show which file ran (below).

**Where a linked driver sits.** Last. A file found at step 1, 2 or 3
beats code compiled into `warden`, so in K1 a `warden-driver-qemu` on
`PATH`, or beside `warden`, runs through the exec transport in place of
the linked qemu driver. Every command of the contract for that name then
goes to that binary; a command it does not list is not implemented for
that name, and the linked driver's code is not used to fill the gap
(GLOBAL-11). Two reasons for this place in the order: it gives the four
VM drivers the same rule as the container driver ("a file on `PATH`
wins"), and K2 changes nothing in it, since each VM driver then becomes
a file at step 3 and step 4 goes.

Two things about a linked name are K1's alone, and K2 ends both. A file
that replaces a linked driver must have a `reap`, like any driver
binary, although the linked driver it replaces has none (**The build**,
a linked driver). And `warden hyperv service ...` is a `warden` command,
not the hyperv driver's `ext`: it stays `warden`'s own linked code
whichever file runs for the name `hyperv`, and such a file's `ext`
cannot be reached through `warden hyperv ...` until K2 moves the
service behind `ext`.

A name that is not found is an error that says where `warden` looked. It
is never replaced by the default driver:

```
[warden] ERROR: driver "nomad" not found (searched PATH, then
                /usr/local/bin, where warden is installed)
  To fix: put warden-driver-nomad on PATH or beside warden, or name its
          file: --driver /path/to/warden-driver-nomad.
          `warden driver list` shows the drivers warden can find.
```

The message is the same for a bundled name, which is the likely case
after an upgrade that installed only `warden`. When the search passed
over a file of that name because it cannot be executed, the message
names the file and says so.

**What is still checked, and why.** Each of these is about whether the
file can be run at all, or run correctly. Nothing compares owners, modes
or directory permissions on any platform.

- *A file named by path* must exist and, after symbolic links are
  resolved, be a regular file the user may execute. Without the check
  the user would get the operating system's bare error from the first
  command; with it the error names the file and what is wrong with it
  (UX-04). Its base name must be `warden-driver-<name>` so that the
  driver's name is known without running the file, which `warden driver
  list` relies on, and so that one file is the same driver whether it is
  named by path or found on `PATH`.
- *On `PATH` and in `warden`'s directory*, a directory or a file without
  execute permission is not a candidate, and the search goes on, as in a
  shell and in Go's `exec.LookPath`. It is the one case in which the
  lookup passes over a file of the right name, so it is shown: `warden
  driver list` prints the file as skipped, with the reason, and the
  not-found error names it. A copy made to customize a bundled driver
  and left without its execute bit therefore does not run, and the
  bundled copy does (**Defaults and overrides** names this).
- *A relative `PATH` entry, or on Unix an empty one, which means the
  current directory.* A candidate reached through such an entry is not
  run, and it is not passed over either: the lookup stops with an error
  that names the entry and the file, and gives the two fixes (make the
  entry absolute; or name the file, `--driver ./bin/warden-driver-x`).
  This is what Go's `os/exec` does: `exec.LookPath` stops at such a
  candidate with `exec.ErrDot` (since Go 1.19) and does not go on to
  later entries, where a shell would run the file. `warden` starts a
  driver with `os/exec` and keeps the library's rule; skipping the entry
  silently would run a different file from the one the user's shell
  finds. The library's reason is that the current directory may not be
  the user's own, so this is the one check here whose origin is trust.
  It is kept because defeating it takes code written for the purpose,
  not as a guard this design adds. On Windows `warden` does not search
  the current directory before `PATH`, which `cmd.exe` does and
  PowerShell does not, and it skips an empty entry, as `os/exec` and
  PowerShell do.
- *On Windows the file is `warden-driver-<name>.exe`*, on `PATH`, beside
  `warden` and for a path alike; `PATHEXT` is not consulted, so a `.bat`
  or `.cmd` of that name is not a candidate. Two reasons, both
  correctness. A batch file runs under `cmd.exe`, whose quoting differs
  from every other program's, and Go's `os/exec` does not quote for it,
  so the arguments of `ext` could not be passed unchanged. And `warden`
  stops a driver by ending the process it started (**The build**), which
  for a batch file is `cmd.exe` and not the driver. A wrapper on Windows
  is therefore a small program, not a script.
- *The name `describe` reports* must be the name that was looked up. It
  names the build's lock and record and the driver's config table, so a
  driver answering to another name would share them with a different
  driver.

**`warden driver list`** (new) prints one entry per name it can find:
the five bundled names, and every `warden-driver-*` on `PATH` or beside
`warden`. Each entry gives the file that would run and where it was
found, and beneath it every other copy, in lookup order: the ones the
lookup passes over because an earlier one won (`shadows`), the bundled
copy among them, and the ones it cannot run (`skips`). Here `warden` is
installed in `/home/ana/.local/bin`, and `/home/ana/bin` comes before it
on `PATH`:

```
NAME          RUNS                                        FOUND
container  *  /home/ana/bin/warden-driver-container       PATH
                shadows /home/ana/.local/bin/warden-driver-container (beside warden)
qemu          /home/ana/bin/warden-driver-qemu            PATH
                shadows the driver linked into warden
vz            (linked into warden)                        linked
hyperv        (linked into warden)                        linked
firecracker   (linked into warden)                        linked
nomad         /opt/nomad/bin/warden-driver-nomad          PATH
                skips /home/ana/bin/warden-driver-nomad (not executable)
```

With nothing customized, the first line reads `container  *
/home/ana/.local/bin/warden-driver-container  beside warden` and has
nothing beneath it. So a bundled driver that another file has replaced
is the first thing the list shows about it.

`*` marks the driver that this invocation's `--driver`, `WARDEN_DRIVER`
and config file select, so the list answers "what would a build run
here". When the selection is a path, that name's entry shows the file
named, with `path (--driver)` or `path (WARDEN_DRIVER)` under FOUND,
and what the lookup would have found for the name beneath it as
shadowed; without this a driver named by path in the variable would run
for every build and appear nowhere in the list. A selection that cannot
be used (a path in the config file, a name that is not found) is said
in a line under the table, and the list still prints.

The command reads directories only and runs no driver; the name of a
file given by path is its base name's. With `--json` it prints one
object,
`{"drivers":[{"name","path","found","source","selected","shadowed":[{"path","found"}],"skipped":[{"path","reason"}]}]}`,
where `found` is `path`, `PATH`, `beside-warden` or `linked`, `source`
names the flag or variable behind a `path`, and a shadowed linked
driver is `{"path": <warden's own>, "found": "linked"}` (UX-02).

**What `warden` runs without being asked.** Only the selected driver.
The commands that select no driver cover the five bundled names: `image
list`, and `image verify` with no `--driver`, ask each of the five,
resolved by the lookup above, so a copy on `PATH` answers for its name
there as it would in a build. A driver of any other name is run only
when the user selects it or runs `warden <name> ...` (**The commands**,
`ext`).

**Recorded.** Before the build starts, `warden` writes
`<output>/driver.json`:

```json
{"name": "container", "version": "0.9.0", "contract": 1,
 "path": "/home/ana/bin/warden-driver-container",
 "sha256": "9f2c...", "found": "PATH",
 "shadowed": [{"path": "/home/ana/.local/bin/warden-driver-container",
               "found": "beside-warden"}]}
```

`found` is `path` (the file was named), `PATH`, `beside-warden` or
`linked`, and `shadowed` lists the copies the lookup passed over, in the
form `driver list --json` gives them. A build's output therefore says
that a bundled driver was replaced, and by which file, exactly when
`found` is `PATH` or `path` and `shadowed` holds a `beside-warden` or
`linked` entry; an install nobody has touched writes `found:
"beside-warden"` and an empty list. The redraft's `linked` boolean is
gone, since `found` says it. For a linked driver `path` and `sha256` are
`warden`'s own. The
file is unsigned and is not part of the ledger. The environment record
is not touched: `specs/metadata-environment/v1.schema.json` already
defines `driver` as a string, set by the driver's own environment
description.

**What a planted binary can and cannot do.** A file named
`warden-driver-<name>` in an absolute `PATH` directory that comes before
`warden`'s own (in any, when `warden`'s is not on `PATH`) runs in place
of the bundled driver of that name, for every command including
`--help`, `doctor` and `clean`, with the user's privileges. It then
decides how the build is isolated, which relay and executor run, and it
is given the claim token. `warden` does not prevent that; it shows it
(`driver list`) and records it (`driver.json`, a19). What a planted file
cannot do: be run from the current directory or through a relative
`PATH` entry (the lookup stops with an error there), or be named as a
driver's file by a `warden.toml`. A repository can therefore choose a
driver by name, but can supply the driver's binary only if the user has
put one of the repository's directories on `PATH` as an absolute entry
ahead of `warden`'s. It cannot name the container runtime's program
either, once `runtime-cli-project-path` has merged (**Security
note**).

### The commands

The verb is `argv[1]`. `describe` takes no request: it is what tells
`warden` which contract versions the driver speaks. Every other command
gets a request, one JSON object that carries `contract`, the version
`warden` chose, and `config_file` and `args`, from which the driver
reads its own configuration (**Configuration**). The request is on
stdin, except for `attach` and `ext`,
which inherit the terminal's stdin and read theirs from the file named by
the environment variable `WARDEN_DRIVER_REQUEST`. Secrets travel only in
a request on stdin, never in argv, the environment or a file.

`describe` and `run` own stdout: every byte there is the contract's.
Every other command inherits the terminal for stdout and stderr, prints
its own lines, and writes its result as one JSON object to the file named
by `WARDEN_DRIVER_RESULT`. `warden` creates the request and result files
with mode 0600 in its temporary directory and removes them.

| Command | Required | Request | Output | Exit |
|---|---|---|---|---|
| `describe` | yes | none | One JSON object on stdout. No side effects; answers within 10 s | 0 |
| `run` | yes | `start` on stdin, then messages | Messages on stdout, one JSON object per line; human lines on stderr | 0 after `stopped`, 1 after `failed` |
| `reap <build-id>` | yes | `{contract, build_id, tmpdir, config_file, args, state_file}`, with the values the build was started with (**The build record**). The state file is what it works from; it does not fail because the config file named here has since gone (**Configuration**, C10) | Result `{removed}`. Removes whatever that build left. Exit 0 means nothing of that build is left, and it may be repeated. When the driver cannot look where the build's leftovers would be (its runtime does not answer, it is on the wrong host), it exits 1 with a `fixable` error; it never reports success for a place it could not look in | 0, 1 |
| `doctor` | no | `{contract, vm, config_file, args, verbose}` | Its report on stdout. Result `{blocked}`. Changes nothing on the host | 0, 1 |
| `setup` | no | `{contract, vm, config_file, args, verbose}` | Its progress. Result `{}` | 0, 1 |
| `image-prepare` | no | `{contract, os, source, config_file, args, verbose}` | Progress on stderr. Result `{path}`; an empty path means the driver already reported the outcome | 0, 1 |
| `image-list` | no | `{contract, config_file}` | One line per prepared image on stdout. Result `{count}` | 0, 1 |
| `image-verify` | no | `{contract, config_file}` | One line per cached layer on stdout. Result `{checked, drift}` | 0, 1 |
| `clean` | no | `{contract, keep_ids, cache, tmpdir, config_file, args}` | `Removing ...` lines on stderr. Result `{removed}`. Removes its leftovers of every build whose ID is not in `keep_ids`, and its own cache when `cache` is true | 0, 1 |
| `attach <build-id>` | no (needs `capabilities.shell`) | `{contract, build_id, config_file, args, state_file}` in the request file, with the values the build was started with | The interactive shell, on the inherited terminal. Result `{}`, or an `error` object when the attach itself failed | The shell's own code |
| `ext <args...>` | no | `{contract, config_file}` in the request file; the arguments in argv, as typed | The driver's own subcommands, listed in `describe.ext` | The driver's own |

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

**`attach`.** Its exit code is the shell's, whatever it is, so a shell
that exits 125 is not mistaken for anything else. Whether the attach
itself failed is in the result file: when it holds an `error` object,
`warden` prints it and exits 1 (UX-09: "every other failure exits 1");
otherwise `warden` exits with the shell's code.

**`ext`, and how a user reaches it.** `warden <name> <args...>` runs the
driver `<name>`'s `ext <args...>`: a first argument that is not a flag
and not a `warden` command is looked up as a driver name, which is why a
driver's name may not be a `warden` command. "First" is literal: it is
the argument right after `warden`, so none of `warden`'s flags can come
before `<name>`, and `warden` reads nothing after it; a `--driver`
further along that line is the driver's own argument (**Configuration**,
how `warden` finds the driver). `describe.ext` lists the subcommand
paths and their usage, and `warden <name> --help` prints them. A first
argument that names no driver that can be found is the unknown-command
error, as today. In K1 `warden hyperv service ...` stays a `warden`
command (hyperv is linked), and it stays one in K1 even when a file has
replaced the linked hyperv driver (**Discovery and trust**); it has the
same form, so nothing changes for the user when K2 moves it behind `ext
service ...`.

**Commands not started by `warden`.** Two kinds exist, and the contract
allows both:

- A process the operating system starts, such as a service registered as
  `warden-driver-hyperv ext service run`. It has no request file, so
  `ext` started without `WARDEN_DRIVER_REQUEST` is given no config file
  and runs with the driver's defaults; it does not go looking for one
  by itself, in the working directory or through `WARDEN_CONFIG`. It
  must not exit 2 for the missing request.
- A process the driver starts itself, such as a detached image refresh.
  The driver then writes the request itself, as `warden` would.

**Paths** in requests are UTF-8 strings. A context, output or temporary
path that is not valid UTF-8 cannot be carried unchanged in JSON, so for a
driver binary `warden` refuses it with a fixable error before it starts
the driver (Behaviour 15). The linked transport carries paths as Go
strings, as today.

**Probe.** Any request may carry `"probe": true`. The driver checks
`contract`, reads and checks its own flags and keys from `args` and
`config_file`, makes the checks it would make before
doing any work, creates nothing, and exits 0 or with its refusal. A probe
answers within 30 s. `warden` probes `run` before it creates or stages
anything, so a bad value of a driver's flag or key, or a host that
cannot run the build, is
refused first (UX-08), and no output directory is left behind. A probe of
`run` is a `start` message with `probe: true` and without `build_id`,
`claim_token`, `build_script`, `diagnostics_dir` and `state_file`; the
driver makes its preflight (the checks each driver makes today before it
creates anything: the container driver's runtime answers, `checkPrereqs`
and the host-link check for qemu, `buildPreflight` for firecracker,
`Detect` for hyperv, the platform check for vz) and answers with one
message, `stopped` or `failed`.

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
 "flags": [{"name": "edition", "value": true, "commands": ["image-prepare"],
   "usage": "Windows edition / install.wim image name (qemu windows)"},
  {"name": "force", "value": false, "commands": ["image-prepare"],
   "usage": "re-acquire even if a valid cached copy exists"}],
 "keys": [{"key": "driver.image_refresh_after", "type": "string",
   "aliases": ["vm.image_refresh_after"], "commands": ["run"],
   "usage": "age after which a prepared Windows layer is refreshed"}],
 "ext": [],
 "help": {"doctor": "...", "image-prepare": "..."}}
```

| Field | Meaning |
|---|---|
| `contract` | The integer contract versions the driver speaks |
| `name` | What `--driver` selects; matches the file name |
| `version`, `vendor`, `bug_url` | For `driver.json` and bug text. `warden --version` prints `warden`'s version only and runs no driver |
| `commands` | The commands it implements, `describe` aside. `run` and `reap` must be present in a driver binary's. `attach` is listed exactly when `capabilities.shell` is true, and `ext` exactly when the `ext` list is not empty. A linked driver's value, in K1 only, lists `run` and each of `doctor`, `setup`, `image-prepare`, `image-list`, `image-verify` and `clean` that `warden` serves for it from its existing code, and no `reap` (**Configuration**, the four linked VM drivers) |
| `capabilities.vm` | It boots a separate build machine: it takes the shared VM settings (`start.vm`) and builds a script or a Containerfile. Without it, it builds a Containerfile only and `warden` refuses the VM flags |
| `capabilities.shell` | `run` accepts `mode: "shell"` and `attach` is implemented |
| `capabilities.keep` | What `--keep-vms` leaves: `running` (the machines), `stopped` (the work directory only) or `none` (`warden` refuses the flag) |
| `capabilities.exited` | It sends `exited` when its relay or its environment stops on its own |
| `capabilities.containerfile` | `translate`: `warden` translates a Containerfile to the build script. `ignore`: the guest cannot run one; `warden` drops it and records nothing |
| `capabilities.guest_os` | The `--guest-os` values `warden` accepts for it; the first is sent when the flag is not given. The bundled drivers list what is accepted today: container `linux`; qemu and hyperv `linux`, `windows`; firecracker `linux`; vz `linux`, which it accepts and ignores today (its guest is macOS, and it reports `darwin` in `relay-started`) |
| `capabilities.image_os`, `default_image_os` | The `--os` values `image-prepare` takes |
| `capabilities.reserved_ranges` | Fixed address ranges it uses on the host; a build link must not overlap them |
| `flags` | The flags it takes, each with its name, whether it takes a value, the commands that take it and a usage line. `warden` uses them to parse the command line and to print help, and reads no value (**Configuration**) |
| `keys` | The config keys it reads, as dotted paths: a common key it honours as `driver.<key>`, which also stands for its override `driver.<name>.<key>`, and a key of its own under `driver.<name>.`. An entry may carry `aliases`, the key's deprecated old names, each one the specification lists for that driver name. Each has a `type` (the TOML type of its value: `string`, `integer`, `float`, `boolean`, `array`, `table`, or `any` for a key whose shape the driver does not state), the `commands` that read it, and a usage line. `warden` reads none of them and no value: the list is what the placement rule is checked against, and `type` and `commands` are what let the suite give a key a wrong value and know which command must refuse it (**Configuration**, C6) |
| `ext` | Its own subcommands: `[{"path": ["service", "install"], "usage": "..."}]` |
| `help` | Its paragraph in a command's long help, keyed by command |

`warden` ignores fields it does not know, and so does a driver. Adding a
field, a command, a capability or a message type does not change the
contract version; removing or changing the meaning of one does, and so
does adding a common key (**Configuration**): a driver and a `warden`
that agree on a version must agree on what `[driver]` holds. Version 1
is the exception while it is unstable: it may change in any release,
its list of common keys with it (**Versions**).

### The build

`run` is one process per build. `warden` and the driver exchange messages,
one JSON object per line, UTF-8, at most 1 MiB a line: `warden` writes to
the driver's stdin and reads its stdout. Each message has a `type`. Both
sides ignore a type or a field they do not know. The driver's stderr is
its human-readable log; `warden` reads it through a pipe, passes it to its
own stderr and keeps its tail for error reports.

| From | `type` | Carries |
|---|---|---|
| warden | `start` | `contract`, `build_id`, `mode` (`build` or `shell`), `claim_token`, `guest_os`, `image`, `base_image`, `build_script`, `context_dir`, `tmpdir`, `diagnostics_dir`, `state_file`, `keep`, `verbose`, `vm`, `config_file`, `args` |
| driver | `relay-started` | `claim`: `{"network": "tcp", "address": "127.0.0.1:41233"}`; `claim_values`: `{environment, build_link, upstream_dns}`; `guest_os`; `old_relay_fix` |
| warden | `claimed` | Nothing: the relay is configured and ready; start the environment |
| driver | `environment-started` | Nothing |
| driver | `exited` (optional) | `which` (`relay` or `build`) and `detail`; valid at any time after `relay-started` |
| warden | `stop` | `reason` (`completed`, `failed`, `interrupted`, `timeout`, `shell-exited`) and `detail`, a sentence for the driver's log (for `failed`: what failed, such as the claim's error) |
| driver | `stopped` or `failed` | `stopped`: `kept`, the paths and names left behind under `keep`. `failed`: an `error` object. Exactly one of the two, as the driver's last message, after it has torn down. `stopped` answers `stop` or the end of input; `failed` may be sent at any time, without waiting for `stop`, when the driver itself cannot go on |

The fields of `start`:

| Field | Meaning |
|---|---|
| `build_id` | Eight lowercase letters and digits, made by `warden` (`driver.RandAlphaNum(8)`). Four of the five drivers make their own today; vz makes none and gains one. The driver names everything it creates for the build from it, so `reap` can find it again |
| `mode` | `build` runs the script. `shell` holds the environment after setup for `attach`; only for a driver with `capabilities.shell` |
| `claim_token` | The claim token, made by `warden`. The driver puts it on the relay's boot channel, where it is one of the relay's pre-claim values (RELAY-08). It is the only secret the driver receives |
| `guest_os` | The `--guest-os` value, or the first of the driver's `capabilities.guest_os` |
| `image` | The `--image` value, or empty |
| `base_image` | The `FROM` image of the build's Containerfile, as written there, whenever the build has a Containerfile and the driver's `capabilities.containerfile` is `translate`. That is so whether or not the build script was translated from it: with `--script`, or a `build.sh` named by path, the script is the user's and the image is still the Containerfile's, which is what `linuxguest.ContainerfileImage` does today. Empty when there is no Containerfile, when it cannot be parsed, or when its `FROM` is `scratch`. It replaces each driver's own second translation (`container.stageBuildScript`, `linuxguest.ContainerfileImage`); the driver still resolves the reference to what it boots or runs |
| `build_script` | The absolute path of the staged build script. The driver reads it for its environment description and never serves it: the collector does |
| `context_dir` | The build context, absolute |
| `tmpdir` | Where work directories are made (`--tmpdir`, `[build] tmpdir`); empty means the system's |
| `diagnostics_dir` | The build's output directory. The driver may write `relay.log` and `console.log` there and nothing else |
| `state_file` | The absolute path of a file that is the driver's to write for this build (**The build record**). The same path is given to `attach` and `reap` |
| `keep` | `--keep-vms`. `warden` sends true only when `capabilities.keep` is not `none` |
| `vm` | For a driver with `capabilities.vm`: `{cpus, memory_mb, build_link, build_link_source, dns}`. Zero or empty means "not set", and the driver applies its default |
| `config_file` | The absolute path of the one config file `warden` read for this invocation, absent when there is none; the driver reads its own keys from it (**Configuration**) |
| `args` | The driver's flags from the command line, passed on with their values unread (**Configuration**) |
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

**The build record.** `reap` cannot work from the build ID alone. What
the container driver creates lives in a container runtime and under a
temporary directory, and neither can be derived from the ID: a setting
nobody set is absent, so each driver process would detect the runtime
again (and find another one when the build's runtime no longer answers,
which is a likely reason the driver died), and a `reap` run by `warden
clean` would get that invocation's `--tmpdir` and `--runtime`, not the
build's. So `warden` keeps three files per build in the lock directory
(`driver.LockDir()`: `<user cache dir>/buildwarden/locks`, mode 0700):

| File | Written by | Holds |
|---|---|---|
| `build-<driver>-<id>.lock` | `warden` | Nothing. `warden` holds an OS file lock on it for the life of the build, released by the OS if `warden` dies. A held lock means the build is live |
| `build-<driver>-<id>.json` | `warden`, before it creates anything else | The record: the driver's name, the path and digest of its binary (or `linked`), the contract version, the build's temporary directory and stage directory, and the `config_file` and `args` `run` was started with. No secret: the claim token is not in it, and a driver takes no secret as a flag |
| `build-<driver>-<id>.state` | The driver, if it needs it | Whatever the driver needs to find the build again that the ID does not give, and everything it resolved from its configuration that `attach` or `reap` depends on: the container driver writes the runtime it chose; in K2 firecracker writes its slot and qemu its process record. At most 64 KiB, opaque to `warden`. The driver writes it before it creates the thing the content describes |

`attach` and `reap` get the record's `tmpdir`, `config_file` and `args`
and the state file's path, whoever runs them and whenever. The record
names the file and repeats the flags; it does not freeze what the file
said, which is why a value `reap` depends on belongs in the state file.
`warden` removes the three
files when the build ends cleanly, and otherwise only after a `reap` that
exited 0.

**The state file is authoritative for `reap` and `attach`.** They run
later than the build, sometimes much later and from another directory.
The ordinary dead build is a CI job that was killed and whose checkout,
with its `warden.toml`, was then deleted; `warden clean` meets
its record days afterwards. If `reap` re-read and re-checked the
recorded config file as `run` does, it would exit 1 for that build
every time, and its lock and record could never be removed. So, for
these two commands only:

- What the build resolved from its configuration and these commands
  depend on is in the state file (the driver wrote it before it created
  what it describes), and the driver takes it from there. It does not
  resolve it again from `args` and `config_file`.
- A `config_file` that no longer exists is read as if none had been
  named, with a line on stderr that names it. It never fails the
  command.
- When a state file exists, a recorded file that no longer parses, or
  that now holds a value the driver would refuse, does not fail the
  command either: the driver has no need of it.
- When no state file exists, the build died before the driver created
  anything a state file would describe. A driver that still wants a
  setting then resolves it from the record's `args` and from the
  recorded file, if it exists. A file that is there and cannot be used is
  a `fixable` error that names it, as in any command: the file is the
  user's to fix or remove, and the next `warden clean` goes on.

These are requirement C10 under **Configuration**, and the suite's Kill
case deletes the build's config file before `reap`.

Removing lock files is new: nothing removes one today. `driver.HostLock`
opens with `O_CREATE` and then locks, with no check that the path still
names the file it locked, so a `warden clean` that ran between a build's
open and its lock could lock, unlink and release the file, and the build
would then hold a lock on a file with no name, invisible to every later
`clean`. So build locks get their own function in `driver/hostlock.go`,
with a test: after it has the lock it compares the open file with the
path's (`os.SameFile`), and starts again with a new file when they
differ; and `warden clean` unlinks a lock file only while it holds the
lock and after the same comparison. The convention (the directory, the
name, who holds the lock, and that `keep_ids` is the IDs of the held
locks) is part of the specification, so that the suite and any other
orchestrator follow it.

The lock is `warden`'s alone. Handing the locked descriptor to the `run`
child, so that the lock stayed held until both processes were gone, was
considered and not taken: Windows has no equivalent, so a `reap` has to
be safe beside a driver that is still tearing the same build down in any
case, and the contract requires that of it.

**The sequence.** The actor is named at each step.

1. **warden** resolves the driver (**Discovery and trust**), runs
   `describe`, picks the contract version, parses the command line with
   the flags the driver declares, and probes `run`, in which the driver
   reads and checks its own configuration and makes its preflight.
2. **warden** makes `build_id`, takes the build lock
   `build-<driver>-<build_id>` and writes the build record.
3. **warden** creates the output directory, writes `driver.json`, and
   stages the build script in `<tmpdir>/warden-stage-<build_id>/` with
   `driver.StageBuildScript`, then records an altered input with
   `driver.RecordInputs`, as each driver does today. For
   `capabilities.containerfile: ignore` it drops the Containerfile first,
   as vz does, so nothing is translated or recorded.
4. **warden** makes `claim_token`, starts `run` in its own process group
   and sends `start`. `--timeout` counts from here.
5. **The driver** writes its state file if it needs one, resolves its
   relay and its environment's image, describes the environment, starts
   the relay with `claim_token`, and sends `relay-started`.
6. **warden** starts the collector (`driver.StartSession`, with the
   `guest_os` the driver reported) and runs `Session.ClaimRelay` against
   `claim.address`: it opens the tunnel, sends the claim with
   `collector_url` set to the tunnel, its own collector token and the
   driver's `claim_values`, waits for the relay's `ready` (90 s from the
   start of the claim), and writes `ca.cert.pem`. An `exited` with `which:
   relay` during the claim cancels it, as firecracker does today.
7. **The relay** reaches **the collector** over that tunnel. The host
   opens no listener and the relay never dials the host (RELAY-07).
8. **warden** sends `claimed`. **The driver** isolates the environment,
   delivers the executor, starts it, and sends `environment-started`. The
   relay is claimed and configured before the environment exists (BOOT-01).
9. **The collector** takes the console output, heartbeats and completion,
   as today; the driver's messages carry none of them. **warden** runs
   `Session.Wait`: completion, 30 s without a heartbeat after the first,
   loss of the tunnel, `--timeout` or a signal.
10. **The driver** may send `exited`. A relay that exited fails the build
    at once. An environment that exited gives the build 10 s to deliver a
    completion already on its way, then fails it. This is firecracker's
    rule, applied to every driver; it needs no knowledge of the driver.
11. **warden** sends `stop`. **The driver** writes `relay.log` and
    `console.log` to `diagnostics_dir`, tears down everything it created
    (under `keep`, all but what its `capabilities.keep` leaves), sends
    `stopped`, and exits 0. **warden** closes the session, removes the
    stage directory, finalizes the output (`driver.FinalizeOutput`),
    removes the build's state file, record and lock, and reports. The exit
    code is the build script's, from the collector (UX-09).

A linked driver takes part in the same sequence. `warden` calls its `Run`
function with the `start` value and a `driver.Host`; the messages are
method calls and return values (**The kit**). Steps 1 to 11 are one piece
of code in `cmd/warden/buildseq.go` over two transports. A linked build
has a lock and a record too, so that `warden clean` can remove its stage
directory.

**Signals and `--timeout`.** `warden` owns the signals of a build, for
every driver and both transports (Decision 19). The first SIGINT (Ctrl-C)
or SIGTERM means stop: `warden` sends `stop`, closes a driver binary's
stdin, and waits for the teardown. On Windows the signal is Ctrl-C or
Ctrl-Break. An interrupted build exits 130; one ended by SIGTERM exits 1,
which is what qemu and firecracker do today (UX-09 names only Ctrl-C).
qemu's and firecracker's own handlers go.

- *A driver binary* runs in its own process group, so the terminal's
  signal reaches `warden` only. `warden` waits for `stopped` for the
  teardown wait, 60 s unless set (below). A second signal, or the wait
  passing, kills the driver's process group (on Windows, the driver
  process) and runs `reap <build_id>`. That `reap` has the teardown wait
  in its turn: when it passes, or on a further signal, `warden` kills
  the `reap` process, says that the build's leftovers remain for `warden
  clean`, keeps the record and the lock, and exits. So the time from the
  first signal to `warden`'s exit is at most twice the wait.
- *A linked driver* shares `warden`'s process, so there is nothing to kill
  and no limit: the teardown wait applies to the exec transport only. A second
  signal ends `warden` at once, as it does today on vz and hyperv. That
  is new on qemu and firecracker, where a second Ctrl-C is ignored today:
  firecracker's VMMs die with `warden` (`Pdeathsig`); qemu's VMs do not,
  and stay until K2 gives qemu a `reap` (Behaviour 8).

A driver treats `stop` and the end of its input the same way: tear down,
then exit.

**The teardown wait** is how long `warden` waits for a driver binary to
answer `stop` with `stopped` before it kills it and runs `reap`, and
then how long it waits for that `reap`. It is
the one time limit of the contract that can be set (Decision 21, a20):

| | |
|---|---|
| Key | `teardown_timeout` in `[build]`, in the config file (ORCH-03) |
| Type | A string holding a Go duration, the form `--timeout` takes: `"90s"`, `"5m"` |
| Default | `"60s"` |
| Bounds | `1s` to `1h`, inclusive |
| Read by | `warden`, as the config file loads |
| Applies to | A driver binary, on every `stop`: a completed, failed, interrupted or timed-out build, and `warden shell`; and the `reap` that `warden` runs at the end of a build, after a kill or a driver that died. Not a `reap` run by `warden clean`, which the user started and can interrupt. A linked driver has nothing to kill, so in K1 the key is read and checked for it and then has nothing to act on |
| Flag, variable | None |

It is `warden`'s key and not the driver's, although a driver now reads
its own configuration (a21): the wait is the caller's patience, not a
setting of the thing being waited for. `warden` applies it to every
driver alike, one written without the kit included, and the driver it
is meant to bound is by definition one that has stopped answering. It
sits in `[build]` with the other per-build settings `warden` owns
(`tmpdir`, `output_dir`). The value is not sent to the driver in
version 1.

It has no flag and no variable, so it is set in the config file or not
at all. `[vm] dns` and `[relay] upstream_ca_certs` are keys without a
flag today, and this follows them; the answer asked for a key. Nothing
about one invocation changes how long a teardown takes, and a second
Ctrl-C already ends the wait at once. `--teardown-timeout` on `build`
and `shell` can be added later without breaking anything (UX-03), and
is one line if the review wants a flag now.

The bounds: below `1s` every `stop` would end in a kill and a `reap`
instead of a teardown, and above `1h` the wait bounds nothing; a second
Ctrl-C is the way to stop waiting sooner. A value that does not parse or
is out of range is refused as the file loads, before any driver runs
(UX-08), and the default is not used in its place (GLOBAL-10). So is a
value of another TOML type, such as the bare number `90`: `warden`
decodes this key apart from the typed fields of `Config`, where any
type mismatch gets the one message "fix the TOML syntax" today, so that
it gets the message below with `90 is not a duration`:

```
[warden] ERROR: [build] teardown_timeout in warden.toml: "0s" is below
                the minimum of 1s
  To fix: set it to a duration from 1s to 1h, such as "90s", or remove
          it to wait the default 60s
```

Every other limit stays fixed: `describe` within 10 s, a probe within
30 s, the relay's `ready` within 90 s of the start of the claim, 30 s
without a heartbeat, and 10 s for a completion after the environment
exits. No new limit is added for the `reap` at the end of a build: it
takes the teardown wait, since both bound the same thing, how long
`warden` waits for a driver to clean up before it gives up.

**What fails, and what follows.**

| What fails | When | Sent | What `warden` does |
|---|---|---|---|
| The driver refuses the build (preflight, or the relay cannot start) | Before `relay-started` | The driver removes what it made, sends `failed` with a `fixable` or `internal` error, exits 1 | Prints the error as it prints a preflight error today. No `reap`. Removes the stage directory, record and lock. `driver.json` and `inputs/` stay in the output directory |
| `warden`'s claim fails or times out | After `relay-started` | `warden` sends `stop` with `reason: failed` and the claim's error as `detail`; the driver tears down and sends `stopped` | Closes the session; prints the claim's error, with `old_relay_fix` when the relay refused a field; exits 1 |
| The relay exits | Any time after `relay-started` | The driver sends `exited`, `which: relay` | Cancels a claim in progress, or fails the build at once; then `stop` with `reason: failed` |
| The environment cannot be started | After `claimed` | The driver tears down, sends `failed`, exits 1 | Closes the session and prints the error. No `reap` |
| The environment exits before the build completes | After `environment-started` | The driver sends `exited`, `which: build` | Waits 10 s for a completion, then fails the build; `stop` with `reason: failed` |
| The driver's process ends with neither `stopped` nor `failed`, writes a stdout line that is not a JSON object, or does not stop within the teardown wait | Any time | Nothing | Kills the driver's process group, shows the tail of its stderr as a driver defect, and runs `reap <build_id>` with the record's values. If the collector already holds the build's completion, the build's result stands and the defect is reported beside it; otherwise the build fails. If `reap` fails, does not finish within the teardown wait, or is cut short by a further signal (it is then killed), the record and lock stay for `warden clean` |

`failed` and `stopped` both promise that the driver has torn down, so
`warden` runs `reap` only when it got neither.

**`warden` dies (ORCH-04).** The driver reads the end of its input and
tears down. Its stdout and stderr were pipes `warden` held, so they are
closed too, and a driver must finish its teardown without them. On Unix
that needs care: a process that writes to a closed pipe on its standard
output or error is killed by SIGPIPE, and for a Go program that is the
default. The container driver's teardown begins with a log line, so as
written today it would die at that line with every container still
running. The contract therefore says that after the end of its input a
driver completes its teardown whether or not it can write to stdout and
stderr, itself or through a child; the kit ignores SIGPIPE for `run` and
drops write errors on the log and the message stream; and the suite's
Orphaned case closes all three at once.

If the driver was killed too, nothing of `warden`'s or the driver's is
running, but what the runtime or the hypervisor holds is: the container
driver's relay, holder and build containers and their network (and, once
it is a binary, Hyper-V's VMs). They keep running until reaped. That is
what a killed `warden` leaves today. Nothing sweeps before a build (a20).

**`warden clean`**, for the selected driver, in this order:

1. When the driver has a `clean`, it probes it with this invocation's
   `args` and `config_file`. A flag or key of this invocation that the
   driver refuses therefore stops `warden clean` before anything is
   removed (UX-08), as a config error does today, where `runClean`
   loads the configuration first. Without the probe the driver would
   first read them in step 3, after the dead builds were reaped. A
   linked driver's clean code in `cmd/warden` resolves its settings at
   this point for the same reason. A warning for an old key name is
   printed here and not again in step 3 (**Configuration**, C11).
2. For every `build-<driver>-*.lock` nobody holds, it reads the build's
   record. For a driver binary it runs `reap <id>` on the binary the
   record names, after the check that it is a file that can be run
   (**Discovery and trust**), with the
   record's `tmpdir`, `config_file`, `args` and state file; a recorded
   config file that has gone does not fail it (above). A binary that has changed
   since the build (an upgrade) is run, with a line that says so; one
   that is gone or cannot be run is an error for that build, with the
   fix (`warden clean --driver <path>` reaps with the file named). It
   then removes the build's stage directory, which is `warden`'s own. When
   `reap` exited 0, it removes the state file, the record and the lock
   file. When it did not, all three stay, the error is shown, and `clean`
   exits 1 at the end. A lock with no record is a build that died before
   it created anything; the lock file is removed.
3. It runs the driver's `clean`, when it has one, with the IDs of the
   locks that are held as `keep_ids` and this invocation's `tmpdir`,
   config file and flags.

`keep_ids` protects the builds whose lock this user's `warden` can see.
A lock lives in the user's cache directory, so a build started by another
user, under `sudo`, with another `HOME`, or by a `warden` from before K1
is not in it. A driver whose resources are shared between users must
therefore keep a liveness check of its own, and the container driver
does: see its `clean` row under **The operations, per driver**.

**A linked driver** has no `reap` in K1. It shares `warden`'s process, so
it cannot die alone, and its `Run` returning without a result is a bug in
`warden`'s own binary. What a killed `warden` leaves behind for a linked
driver is found as it is today: `warden clean --driver hyperv` and
`--driver firecracker` by their own code, and nothing for qemu and vz
until K2 writes their `reap`. `warden clean` still removes a dead linked
build's stage directory, record and lock.

**`--keep-vms`.** `keep: running` (hyperv, firecracker) leaves the
machines and their work directory; `stopped` (qemu, vz) leaves the work
directory only, which is what those two do today; `none` (container)
makes `warden` refuse the flag, as today.

**`warden shell`.** `warden` sends `start` with `mode: "shell"`. The
driver starts the environment held after setup and sends
`environment-started` once the shell can be entered. `warden` then runs
`attach <build_id>` with the terminal inherited, takes its exit code as
the shell's unless its result holds an error, and sends `stop`. It does
not run `Session.Wait`.

**Per driver.** What each reports as its claim endpoint, what holds its
environment, and what a `reap` has to find. Only the container driver is a
binary in K1; the last column is K2's work for the others, listed here to
show where the contract is expected to fit them and where that is not yet
designed.

| Driver | `claim.address` | What keeps the environment alive | `reap <id>` |
|---|---|---|---|
| container | `127.0.0.1:<port>`, the relay container's published port | The container runtime; names `warden-holder-<id>`, `warden-relay-<id>`, `warden-build-<id>`, network `warden-<id>` | **K1.** In the runtime its state file names: removes those by name, the dev relay image `warden-relay:<id>`, and the work directory under the record's `tmpdir`. Exits 1 when that runtime does not answer |
| qemu | `127.0.0.1:<port>`, forwarded by qemu to the relay VM | Two `qemu-system` children of `run` | K2. Needs a per-build process record in its state file, which is new (firecracker's pid file is the model); how the record is checked against a reused pid on macOS is not designed |
| vz | `127.0.0.1:<port>`, the host-mode relay child | The VM is an object inside the `run` process; the relay is a child. The socketpair is the build link and never leaves the driver | K2. The relay child and the disk clones in the work directory, which vz names from the build ID from K1 on. `run` must be the entitled process with the main thread locked, so the entitlement moves to `warden-driver-vz` |
| hyperv | `<NAT address>:8301` of the relay VM | Hyper-V, by name: `warden-relay-<id>`, `warden-build-<id>`, switch `warden-<id>` | K2. By name, as `hyperv.Clean` does. The service takes network operations only, so no build runs inside it; `InstallService` then registers the driver binary with `ext service run`, since the service manager must be that process's parent (**The commands**, commands not started by `warden`) |
| firecracker | `100.64.88.(4·slot+2):8301`, dialled as the slot's owner | Two VMM children with `Pdeathsig`, and the slot and build locks, all tied to the `run` process | K2. New work: the slot pid file is keyed by slot and holds no build ID, and `firecracker.Clean` kills the VMMs of every free slot, not those of one build. `reap <id>` needs the build's slot, which the driver writes to its state file |

Three of these are behaviour K1 has to hold on the linked transport too:
firecracker's `Pdeathsig` and locks follow `warden`'s process, as today;
vz's main-thread lock stays in `cmd/warden`; and Hyper-V's `StartRelay`
gives up its `StartSession` and `ClaimRelay` calls to `warden` and keeps
the rest.

### Configuration

Two readers of one file (Decision 7, a21; Decision 23). `warden` reads
its own flags and keys. A driver reads its own, inside its own process (inside
`warden`'s, for the four drivers linked in K1): through
the kit when it is built on it, and by the contract's rules when it is
not. `warden` does not look at a driver's values, and a driver is sent
none of `warden`'s beyond the fields the contract's requests carry
(`tmpdir`, `guest_os`, `vm`, `keep`, `verbose`).

The three rules this has to keep. Each is being amended by another item
as this is written, and the design is written to the amended form
(**What K1 takes from the two configuration items**). ORCH-03 and UX-07
are quoted as proposed, from `project-config-trust`'s design at
`539fd38`, which is not approved. UX-08 is quoted as approved, from
`runtime-cli-project-path`'s design at `a2afd08`; it is not merged yet.

- ORCH-03, as `project-config-trust` proposes it: "`warden` reads one
  config file: the one `--config` names, else the one `WARDEN_CONFIG`
  names, else `warden.toml` in the working directory when it exists. It
  reads no other file and merges none; a named file that cannot be read
  is an error. Configuration precedence stays `flag > environment
  variable > the config file > default`." On `main` the sentence is
  "Configuration precedence stays `flag > warden.toml >
  ~/.config/warden`.".
- UX-07, as the same item proposes it: "any of them is easy to override
  at three levels with clear precedence, command line > environment >
  config file", and, unchanged, "A value set at any level replaces the
  default and the lower levels' value (GLOBAL-10)." The last is the
  sentence K1 leans on.
- UX-08, as approved in `runtime-cli-project-path`'s review: "A
  component validates the configuration it consumes, errors and
  mismatches alike, when it initializes: after the overrides are applied
  [...] and before any real work begins", and "Only the configuration a
  run uses is validated." On `main`: "Configuration errors or mismatches
  are validated and surfaced as early as possible, at load/parse/startup,
  before any real work begins, rather than failing deep into a build.".
  K1 edits the list of older checks that ends the approved wording
  (**Rule changes**, UX-08).

**What `warden` reads and owns.**

- Flags: `--driver`, `--verbose`, `--color`; `--capture`, `--output`,
  `--no-compress`, `--timeout`, `--tmpdir`, `--quiet`; `--config`, once
  `project-config-trust` has added it; `--guest-os`,
  which every driver takes and which is checked against
  `capabilities.guest_os`; the VM group `--script`, `--image`, `--cpus`,
  `--memory`, `--keep-vms`, `--build-link`, refused for a driver without
  `capabilities.vm` (Decision 8, a9); `image prepare`'s `--os` and
  `--source`; `clean --cache`.
- Keys: `runtime.driver`, `build.*` (with `teardown_timeout`, a20),
  `relay.*`, `vm.cpus`, `vm.memory_mb`, `vm.build_link`, `vm.dns`,
  `output.*`.
- Variables: `WARDEN_DRIVER`, `WARDEN_UPSTREAM_CA_CERTS`, `NO_COLOR`,
  `WARDEN_VERBOSE`; `WARDEN_CONFIG`, likewise that item's.

`LoadConfig` and `resolveConfig` keep the precedence `main` has for
these when K1's Implement starts, which is `project-config-trust`'s:
one file, and no user-level one. The
build link and `[vm] dns` are still parsed and checked by `warden` as the
file loads, and the link is checked against the reserved ranges of every
bundled driver, as today: the linked drivers' from their `describe`
values and the container driver's `driver.ContainerBaseCIDR`, which stays
in the kit. K2 decides who carries the ranges once no driver is linked.

`Config` loses the seven driver-owned fields (`RuntimeConfig.CLI` and
`RelayImage`; `VMConfig.Network`, `ImageRefreshAfter`, `ImageMinGrace`,
`WindowsImage`, `WindowsImageUpdate`), and `LoadConfig` stops applying
`WARDEN_CTR_CLI` and `WARDEN_IMAGE`. Each of those has one reader after
K1, its driver. The redraft kept the fields so that a wrong TOML type in
one of them still failed the load under every driver; keeping them now
would make `warden` a second reader of keys it does not own. What that
changes for a user is Behaviour 13. `warden` reads nothing in `[driver]`
or `[driver.<name>]` either, and adds no key of its own there: the
first table's keys are the contract's and the second's each driver's,
and `warden` knows the contract's names only to check a `describe`
(**Where a driver's configuration lives**, below).

**What `warden` hands a driver.** Every request, `start` included,
carries `config_file` when there is a config file; every request for a command that takes flags
carries `args` as well; and the driver process inherits `warden`'s
environment. Nothing else about configuration crosses: no resolved value,
no type, no default.

| Field | Holds |
|---|---|
| `config_file` | The absolute path of the one config file `warden` read for this invocation, as a string: the file named with `--config`, else the one `WARDEN_CONFIG` names, else `./warden.toml` when it exists (`project-config-trust`'s lookup, `configFile`; `warden` applies it and the driver does not). Absent when there is none. It is never a list, and it says nothing of how the file was found. The file is TOML v1.1.0, the version `warden`'s own parser reads, and the specification names it |
| `args` | The driver's flags from the command line. A flag declared with a value: one element per occurrence, in the order typed, as `--name=value`, whether it was typed `--name value` or `--name=value`; the value's bytes are unchanged, an empty value included. A flag declared without one (a switch): `--name`, once, when the switch is on, and nothing when it is off. Empty when the user gave none |

`args` is passed through, not interpreted: `warden` does not know what a
value means. Two things are put in one form, so that a driver written by
hand parses one syntax and not several: the split between a flag and its
value, and whether a switch is on. The command line accepts `--force`,
`--force=true` and `--force=false` today, the last occurrence winning,
and `warden` settles that itself, so a driver is never sent
`--name=false` and reads a switch by its presence. The
flags travel in the request and not in the driver's argv, where `argv[1]`
stays the command; a driver should take no secret as a flag, because the
build's `args` are kept in its record (**The build record**).

**One file, and nothing about how it was found.** The design read at
`5808805` sent a list, `config_files`, each entry with a `level`
(`project` or `user`), so that a driver could hold a key that names a
program to the user's file alone. Both are gone. There is one file
(Decision 23), so there is no list. And the rule the level was carried
for is now made without it: the container driver's runtime is one of
three names from every source (`runtime-cli-project-path`). A driver is
therefore not told whether its file is the defaulted `./warden.toml`,
which may have come with a build context someone else wrote, or a file
the executor named. A driver from outside whose key names a program or a
host path should take the file for the first; the specification says so,
and says that the safe form of such a key is a closed list of names.
`project-config-trust` proposes that what a config file may decide does
not depend on how the file was found (its Decision 7), so no rule of a
bundled driver needs the difference. If one ever does, a field can be
added to the requests without a new contract version.

**When `warden` has parsed the file it names.** On the commands where
it reads keys of its own (`build`, `shell`, `doctor`, `setup`, `image
prepare`, `clean`) `warden` parses the file before it runs the
driver, so the file was valid TOML a moment earlier. `image list`,
`image verify` and `warden <name> ...` load no configuration today and
`warden` reads no key on them after K1; there it names the file it
finds without parsing it, so a broken `warden.toml` still does not
stop those commands unless the driver reads a key of its own on them,
and no bundled driver does. The file a `reap` or `attach` is sent is
the build's record's, which this invocation may never have read.

For `run` the two fields are also written to the build's record, and
`attach` and `reap` are sent the record's, whoever runs them, `warden
clean` included, and not those of the invocation that runs them. They name the build's file and flags, not
the file's content at the time, so what `reap` and `attach` must know about
how the build was configured the driver writes to its state file during
`run`, as the container driver does with the runtime it chose, and in
those two commands the state file is what counts: a recorded file that
has since gone, or changed, does not fail them (**The build record**,
C10 below).

**How `warden` tells a driver's flags from its own.** `warden` is not
built with a driver's flags, and it cannot parse a command line without
two facts about each: that the flag exists, and whether the next
argument is its value. In `warden build --force ./ctx` the build path is
`./ctx`; in `warden build --network ./ctx` it is the value of
`--network` and there is no path; nothing on the command line says
which. So `describe` declares the driver's flags, for parsing and for
help and for nothing more (Decision 22, a24):

```json
"flags": [
  {"name": "network", "value": true, "commands": ["run", "doctor", "setup"],
   "usage": "durable build switch name (default: [driver.hyperv] network, then warden-dev)"},
  {"name": "force", "value": false, "commands": ["image-prepare"],
   "usage": "re-acquire even if a valid cached copy exists"}]
```

| Field | Meaning |
|---|---|
| `name` | The flag without its dashes, `[a-z][a-z0-9-]*`. There are no short forms |
| `value` | `true` when the flag takes a value, `false` when it stands alone |
| `commands` | The commands that take it, each one the driver lists: `run` (which is `warden build` and `warden shell`), `doctor`, `setup`, `image-prepare`, `clean`. `image-list` and `image-verify` run with no driver selected and take none. `ext` is not listed: everything after `warden <name>` is the driver's and goes to its argv as typed |
| `usage` | One line for `--help` |

`warden` places a declared flag and does nothing else with it. It does
not know its type, read its value, apply a default or relate it to a
key. What it does, in order:

1. **Find the driver before the flags are parsed.** `warden` has to
   know the driver to know the flags, so it finds the driver from the
   raw arguments, by a scan that needs no flag's arity but its own:
   - *The `ext` form first.* If the argument right after `warden` is not
     a flag and not a `warden` command, the line is `warden <name>
     <args...>`: the driver is `<name>`, and `warden` reads nothing
     after it. In `warden nomad job run --driver docker`, `--driver
     docker` is nomad's argument, not `warden`'s. No flag of `warden`'s
     can stand before `<name>`, `--config` included, so the config file
     of such a line is the one `WARDEN_CONFIG` names, else
     `./warden.toml`.
   - *Otherwise, the command.* `warden` skips its own persistent flags
     from the left (`--driver`, `--config` and `--color` with their
     values, `--verbose`, `-v`); the first argument after them that is not a
     flag names the command. A command that selects no driver
     (`inspect`, `driver list`, `image list`, and `image verify` without
     `--driver`) stops here: no driver is selected and no driver's flag
     is registered, so these work when the default driver is missing. If
     a flag `warden` does not know stands before the command's name, it
     can only be a driver's (step 3), and the command is found after
     `describe`, when that flag's arity is known.
   - *Then `--driver`.* The arguments are scanned left to right, up to
     a `--` argument or the end. `--driver <value>` takes the next
     argument as its value, whatever it looks like; `--driver=<value>`
     holds its own; the last one wins. In `warden build ./ctx --
     --driver x` there is none. `--config` is found by the same scan
     and in the same forms, because with no `--driver` the selection
     may come from the file it names.
   - *With no `--driver`:* `WARDEN_DRIVER`, else `[runtime] driver` from
     the config file (a name and never a path), else `container`.
     `warden` loads its own configuration at this point, before
     `describe`, because the selection may come from it: from the file
     the scan found for `--config`, else `WARDEN_CONFIG`'s, else
     `./warden.toml`.

   `--runtime` is a persistent flag today, accepted and ignored on the
   commands that select no driver; after K1 it is an unknown flag there
   (Behaviour 2).
2. **Describe.** `warden` runs `describe` on the selected driver only,
   once per invocation, and refuses one that does not match the schema,
   lists no common contract version, lacks `run` or (for a driver
   binary) `reap`, or reports a
   name other than the one looked up. It also refuses a flag whose name
   `warden` uses on the same command, and a key outside the driver's
   place in the config file (below); each refusal names the flag or key. For
   the bundled drivers a test in `cmd/warden` checks the same, so a
   clash is found by `make test`.
3. **Register** the declared flags on the commands that take them, as
   flags `warden` passes on. A driver's flag is accepted after the
   subcommand's name, wherever a flag of that command is accepted
   today. A flag the driver declares for every command it lists is also
   accepted before the subcommand's name, as a persistent flag is, and
   is checked against the command once the command is known. That is
   the rule that keeps `warden --runtime docker build` working:
   `--runtime` is persistent today, and it is the only such flag among
   the bundled drivers.
4. **Parse.** A flag that neither `warden` nor the selected driver
   declares for the command is refused here, before any other driver
   command runs. A VM-group flag on a driver without `capabilities.vm`,
   and a `--guest-os` the driver does not list, keep today's messages.
   The parse must also agree with the scan of step 1, which knew no
   arity but `--driver`'s. In `warden build --network --driver hyperv`
   the scan selects hyperv; hyperv declares `network` with a value, so
   the parse reads `--driver` as the value of `--network`, finds no
   `--driver` and takes `hyperv` for the build path. `warden` refuses a
   line whose parsed `--driver` is not the scanned one (`--driver was
   read as the value of --network`, with the fix: give `--network` its
   value, or write `--network=<value>`), where it would otherwise build
   on a driver the parsed line does not name. The same is done for
   `--config`: a line whose parsed `--config` is not the scanned one
   is refused, where `warden` would otherwise have selected the driver
   from one file and built with another.
5. **Split.** `warden`'s flags are resolved with its own config as
   today. The driver's are collected into `args`.
6. **Probe, then call.** The driver reads and checks its own flags and
   keys in its probe, before `warden` creates or stages anything
   (UX-08).

Three command lines:

```
warden build --driver hyperv --network warden-ci --cpus 8 ./win-project
```

hyperv declares `network` with a value for `run`. `--driver` and `--cpus`
are `warden`'s, `./win-project` is the path, and the driver gets `"args":
["--network=warden-ci"]`.

```
warden image prepare --driver qemu --os windows --force \
    --edition "Windows Server 2025 Datacenter" win.iso
```

qemu declares `force` without a value and `edition` with one, for
`image-prepare`. `--os` is `warden`'s and `win.iso` is the source. The
driver gets `"args": ["--force", "--edition=Windows Server 2025
Datacenter"]`. Without the declaration `warden` could not tell whether
`--edition` is the value of `--force`, nor which of the quoted string
and `win.iso` is the source.

```
warden build --runtime docker --regoin eu ./ctx
```

The container driver declares `runtime`; nobody declares `regoin`.
`warden` refuses the command at step 4, having run nothing but
`describe`:

```
[warden] ERROR: unknown flag --regoin for driver "container"
  To fix: see 'warden build --driver container --help' for the flags
          this driver takes
```

Rejected, and asked as a24:

- *A `--` separator*: everything after it goes to the driver unread
  (`warden build ./ctx -- --runtime docker`). It needs no declaration,
  but every driver flag in use today (`--runtime`, `--network`,
  `--slots`, the image flags) would have to move behind it, which UX-03
  forbids without an alias for each, and help could show none of them.
- *A rule that a driver's flag is written `--name=value`*, a bare
  `--name` being a switch. `warden` could then pass an unknown flag on
  without a declaration, but `--runtime docker`, the form in every
  existing script and in the docs, would turn `docker` into the build
  path.
- *Pass an unknown flag on and guess its arity.* Either guess builds the
  wrong thing for some command line, silently.
- *The declaration, and `--` as well*, for a driver that wants arguments
  it cannot declare: a second syntax for the same thing, which can be
  added later without breaking anything if a driver needs it.

**What the kit gives a driver.** A driver built on the kit declares each
setting once, as a Go literal:

```go
// driver/container
var settings = []drivercmd.Setting{
	{Name: "runtime", Type: drivercmd.String,
		Flag: "runtime", Key: "driver.container.cli",
		Aliases: []string{"runtime.cli"}, Env: "WARDEN_CTR_CLI",
		Commands: drivercmd.EveryCommand,
		Usage:    "container runtime (finch, docker, podman)"},
	{Name: "relay-image", Type: drivercmd.String,
		Key:     "driver.container.relay_image",
		Aliases: []string{"runtime.relay_image"}, Env: "WARDEN_IMAGE",
		Commands: []string{"run"},
		Usage:    "relay image reference, or dev to build it from source"},
}

// driver/qemu: a common key it honours. The kit reads it in
// [driver.qemu] and then in [driver].
var settings = []drivercmd.Setting{
	{Name: "image-refresh-after", Type: drivercmd.Age,
		Common:   "image_refresh_after",
		Aliases:  []string{"vm.image_refresh_after"},
		Commands: []string{"run"},
		Usage:    "age at which a prepared Windows layer is refreshed"},
}
```

A setting has a `Key` of the driver's own, under `driver.<name>.`, or
names a `Common` key, never both. `Aliases` are the key's old names,
where it has one (a25).

From that the kit does the rest:

- It builds `describe`'s `flags` and `keys`, a key's `type` and
  `commands` from the setting's own. `EveryCommand` means every command
  the driver lists, so the container driver's `runtime` is a flag of
  `warden build`, `shell`, `doctor` and `clean`, and not of `warden
  setup` or `image prepare`, which that driver does not implement
  (Behaviour 2).
- **It reads the same file with the same precedence `warden` uses.** For
  each setting that applies to the command: the flag in `args` when
  given (the last occurrence); else the setting's environment variable;
  else its key in `config_file`; else absent, and the driver applies its
  default. That is ORCH-03's order as `project-config-trust` leaves it,
  with the two existing variables where `LoadConfig` has them today,
  between the flag and the file. In the file a common key has two
  places, and the driver's own table is read before `[driver]` (**Where
  a driver's configuration lives**, below, with a worked example). The
  place that sets a value is the value: it is not merged with a lower
  one, and for a key that holds a table or a list the whole of it comes
  from one table (GLOBAL-10).
- **Empty values, as today.** A flag given with an empty value
  (`--runtime=`) and a variable that is set but empty are not set, and
  the next place is read: `cmd/warden` tests `flagRuntime != ""` and
  `cli != ""` today. An empty string in the file is the file's value for
  the key, and each of the seven existing keys takes it to mean its
  default, as today, under its new name and its old one. The same holds
  between the two tables: `image_refresh_after = ""` in
  `[driver.hyperv]` is hyperv's value, so it hides the `[driver]` value
  and gives hyperv its default. The contract states the first two for
  every driver (C8); what an empty string in the file means is the
  driver's.
- **Typed access.** `String`, `Int`, `Bool`, `Age` (`7d`, `36h`), `CIDR`,
  `IPList` and `Path`, read as `cfg.String("runtime")`; and `Raw`, a key
  of any shape, which the driver decodes into its own Go type with
  `cfg.Decode`. `cfg.Source(name)` is the label to print for a value:
  `--runtime`, `WARDEN_CTR_CLI`, or the file's name with the table and
  key as written there (`warden.toml [driver.hyperv] network`).
- **Validation, before any work.** The kit resolves and type-checks every
  setting that applies to a command when the request arrives: in the
  probe of `run`, and in every other command before the driver's
  function is called. A `config_file` that was named and does not exist
  is an error there, when a setting of the command has a key (C6).
  Checks of range and consistency are the driver's,
  in its `Preflight` or first in the command (UX-08).
- **Not so for `reap` and `attach`.** There the kit resolves nothing
  before the driver's function is called. The request's settings are
  resolved when the function first reads one, with a recorded file that
  no longer exists read as not listed and named on stderr. A driver
  that finds what it needs in its state file, as the container driver
  does, therefore opens no config file in those commands, and a
  deleted or broken `warden.toml` cannot stop a dead build from being
  reaped (**The build record**, C10).
- **The error form.** A value that cannot be used is a
  `driver.FixableError` that names where it came from, which the kit
  sends as a `fixable` error object and `warden` prints as it prints its
  own (UX-04). For the existing keys the text is today's, with the file
  the value was read from where today's always says `warden.toml`, and
  the table and key as the file has them, so a value under an old name
  is reported under that name:

  ```
  [warden] ERROR: warden.toml [vm] image_refresh_after: invalid age "soon"
    To fix: set it to an age such as "7d" or "36h"
  ```

- **Old names** (a25). A setting's `Aliases` are read as the key
  itself. The kit refuses a file that sets a key
  under its new name and an old one, and returns with the settings one
  warning for each old name a file sets (`Settings.Warnings`). The
  settings are resolved twice in `warden build`, `shell` and `clean`,
  once in the probe and once in the command that follows it, so the
  warnings are not printed wherever `Resolve` was called. One rule
  says when, and it is the contract's (C11): they are printed on stderr
  when a probe is answered, and in a request that is not a probe only
  for a command `warden` does not probe first (`doctor`, `setup`, the
  image commands and `ext`). A `run` or a `clean` that is not a probe
  prints none, and neither does `reap` or `attach`. `Call` applies the
  rule, for `Main` and for a linked `run`. In K1 `cmd/warden` resolves
  a linked driver's settings once for each of its other commands, for
  `clean` at the point where a driver binary is probed, and prints the
  warnings there (**Where a driver's configuration lives**).

- It refuses an `args` element the driver did not declare, and a
  declaration that breaks the placement rule: a `Key` outside
  `driver.<name>.` or with a common key's name, a `Common` that the
  contract does not name, or an alias the specification does not list
  for the driver's name.

What the five bundled drivers declare is what `cmd/warden` holds for
them today. Every flag and both variables keep their names and
meanings. The seven keys keep their names and meanings and take the
tables a4's answer gives them; each is still read under its old name
(a25).

| Driver | Setting | Flag on | Config key | Old name, still read | Env |
|---|---|---|---|---|---|
| container | `runtime` | every command it lists | `driver.container.cli` | `runtime.cli` | `WARDEN_CTR_CLI` |
| container | `relay-image` | none | `driver.container.relay_image` | `runtime.relay_image` | `WARDEN_IMAGE` |
| qemu | `force`, `edition`, `virtio`, `arch`, `keep-vms` | `image-prepare` | none | | |
| qemu, hyperv | `image-refresh-after`, `image-min-grace` (ages) | none | The common keys `driver.image_refresh_after` and `driver.image_min_grace`, each with its override in `driver.qemu.` and `driver.hyperv.` | `vm.image_refresh_after`, `vm.image_min_grace` | |
| vz | `update` | `image-prepare` | none | | |
| hyperv | `network` | `run`, `doctor`, `setup` | `driver.hyperv.network` | `vm.network` | |
| hyperv | `nat-prefix`, `host-ip`, `slots` | `setup` | none | | |
| hyperv | `sha256`, `force`, `name`, `keep-vms` | `image-prepare` | none | | |
| hyperv | `windows-image`, `windows-image-update` | none | `driver.hyperv.windows_image`, `driver.hyperv.windows_image_update` | `vm.windows_image`, `vm.windows_image_update` | |
| firecracker | `slots` | `setup` | none | | |

**What a driver written without the kit must do itself.** These are the
contract's requirements on configuration, numbered as the specification
numbers them. The last column says whether the suite can hold a binary
to each.

| | A driver must | The suite, against a binary |
|---|---|---|
| C1 | Declare every flag it takes in `describe.flags`, with its arity and its commands, and use no name `warden` uses on the same command (the specification lists them) | Static tier: the schema, the name form, the reserved names, and that each flag names only commands the driver lists |
| C2 | Take each declared flag from `args`: a flag with a value as `--name=value` (for a flag that holds one value, the last occurrence wins), a switch as `--name`, on when present and off when absent; and refuse an element it did not declare with a `fixable` error that names it | Static tier: a probe of each listed command with an undeclared element exits 1 with a `fixable` error naming it, and creates nothing |
| C3 | Read configuration only from the file named in `config_file`. It never looks for a `warden.toml` by itself, in its working directory or through `WARDEN_CONFIG`, which it inherits from `warden`; sent no `config_file`, or started with no request (a service), it reads none and uses its defaults | Static tier: with an invalid `warden.toml` in its working directory and `WARDEN_CONFIG` naming another invalid file, and no `config_file` in the request, a probe ends as it does with neither file present |
| C4 | List the keys it reads in `describe.keys`, each in its place in the config file (below) and with the commands that read it: a common key it honours as `driver.<key>`, with the type the specification gives it; a key of its own under `driver.<name>.`, with the TOML type of its value (or `any`) and a name that is no common key's. Listing a common key is also listing its override, `driver.<name>.<key>`. It reads nothing in `[driver]` that it did not list, and no override of a common key it did not list (C5) | Static tier: every listed key is `driver.<key>` for a common key of the contract version, with that key's type, or is under `driver.<name>.` and does not have a common key's name; every old name listed with a key is one the specification lists for that driver name (C11); each has a type of the allowed set and names only commands the driver lists. A key read but not listed cannot be seen from outside |
| C5 | Ignore every table and key it does not own, a common key it does not list among them, in `[driver]` and in its own table (C4); and read any file `warden` reads: the file is TOML v1.1.0 | Static tier: with a config file that holds `warden`'s tables, another driver's table, an unknown one, and every common key of the contract version that the driver does not list, written in `[driver]` and in the driver's own table with a value of another TOML type than the specification gives it, a probe ends as it does with no file; a driver that reads a common key it did not list, and checks what it reads, fails here. One of those tables is written with syntax that TOML 1.0 does not have (an inline table over several lines with a trailing comma), so a driver whose parser stops at 1.0 fails here, in the suite, and not on a user's file that `warden` accepted |
| C6 | When it lists a key: refuse a `config_file` that does not exist, cannot be read or cannot be parsed, and a key of its own whose value has the wrong type, with a `fixable` error that names the file, and the key where there is one (not in `reap` and `attach`: C10). A file that was named and is missing is never read as if none had been named, and no other file is tried in its place | Static tier, for each listed key: a `config_file` that names a path where nothing exists, and a file that is not TOML, each make a probe of a command the key names exit 1 with a `fixable` error naming the file; and, for a key whose type is not `any`, a file that gives that key a value of another TOML type makes it exit 1 naming the file and the key. All three create nothing. The check works from the key's declared `type` and `commands`, and is made at each place the key may be written: for a common key in `[driver]` and in the driver's own table, and for a key with an old name under that name too |
| C7 | Read and check all of it before any work: in the probe of `run`, and first in every other command but `reap` and `attach` (C10) | Static tier, through C2 and C6: the refusals come from a probe, which must create nothing |
| C8 | Apply the precedence: a flag, else the setting's environment variable where the specification gives it one (C9), else the key in `config_file`, else its default. A common key is read from the driver's own table, and from `[driver]` only when that table does not set it. A flag given with an empty value is not set; the place that sets a value is the value, not merged with another, and an unusable value is an error, never replaced by a lower place's value or by the default | **Not checkable against an arbitrary binary**: the suite does not know what a value means to a driver, so it cannot see which place won. Held by the kit's own tests for a driver built on it, and by the suite's self-test for the fake, whose own key and one common key are echoed back in its probe, each with the place it came from: the flag, its own table or `[driver]`. The fake has no variable; the variable's step is held by the kit's tests, for the container driver's two |
| C9 | Take its settings as flags and keys, not as environment variables of its own (UX-07). The two that exist, `WARDEN_CTR_CLI` and `WARDEN_IMAGE`, stay the container driver's | Not checkable: a process may read any variable |
| C10 | In `reap` and `attach`: take from the state file what it holds; read a `config_file` that no longer exists as if none had been named, saying so on stderr; and, when a state file exists, not fail because a recorded file cannot be parsed or holds a value it would refuse | Static tier: `reap` of an ID that never existed, sent a `config_file` that does not exist, ends as it does with none named. Lifecycle tier, Kill: before `reap`, the config file the build was started with is deleted in one run and overwritten with text that is not TOML in another, and `reap` still removes what the build left |
| C11 | Read a key under an old name only where the specification lists that name for the driver's own name, as the same key; refuse a file that sets a key under both names, with a `fixable` error that names the file and both; and say on stderr, once per old name the file sets, that the name is deprecated and what the new one is: when it answers a probe, and in a request that is not a probe only for a command `warden` does not probe first (`doctor`, `setup`, the image commands, `ext`), so not in a `run` or `clean` that is not a probe, nor in `reap` or `attach`. A driver whose name has no old names reads none | Static tier: the old names in `describe.keys` are the specification's for that name (C4), and a wrong type under an old name is refused (C6). That a value under an old name is used, that a file with both is refused, and when the warning is printed, is **not checkable against an arbitrary binary**, since the suite cannot make a value a driver accepts; held by the kit's tests, for each of the seven |

A driver that takes no flag and reads no key lists none and has nothing
to do here but C2's refusal. One that reads keys needs a parser for
TOML v1.1.0 in its own language, since it must read any file `warden`
reads; a parser that stops at TOML 1.0 is not enough. That is part of
what a21's answer costs. C11 binds only a driver that carries one of
the five bundled names: no other name has an old key.

**Where a driver's configuration lives** (Decision 9; a4, a25 and a26,
answered). The review's answer to a4, in Jeff's words: "A
small subset of common keys under driver with overrides and custom
options under driver.\<name>". So a driver's keys have two tables, in
the one config file:

| Table | Holds | Who says what a key means |
|---|---|---|
| `[driver]` | The **common keys**: a short list of keys that more than one driver reads and means the same thing by | The contract's specification, which gives each its name, type and meaning |
| `[driver.<name>]` | That driver's **overrides** of the common keys it honours, each under the common key's own name; and the driver's **own keys**, in any shape it likes: nested tables, lists, lists of tables | The specification for an override; the driver for its own |

`warden` reads neither table. `[runtime] driver` still selects the
driver and stays `warden`'s; `[driver]` configures drivers and selects
nothing.

**The common keys of contract version 1** are two:

| Key | Type | Meaning | Honoured by |
|---|---|---|---|
| `image_refresh_after` | A string holding an age, such as `"7d"` or `"36h"` | The age at which a prepared base image is prepared again in the background; the build goes on with the image it has | qemu, hyperv |
| `image_min_grace` | A string holding an age | The least evaluation grace a prepared image must still have; a build on an image with less is refused | qemu, hyperv |

They are on the list because more than one driver reads them and means
the same thing by them: qemu and hyperv both pass the two values to one
function, `winguest.CheckFreshness`, and both take its defaults (`7d`
and `1d`) when nothing is set. No other key in today's inventory
(**Problem**, and the settings table above) is read by two drivers. The
specification gives a common key its name, its type and its meaning. The
value a driver uses when nobody sets the key is the driver's to state,
as for any setting.

What was considered and left off:

| Candidate | Why it is not a common key in K1 |
|---|---|
| `cpus`, `memory_mb`, `build_link`, `dns` | Every VM driver means the same thing by them, but no driver reads them: they are `warden`'s keys in `[vm]`, sent in `start.vm` (a9) |
| The relay override, `relay_image` | One driver reads it, and its value (an image reference, or `dev`) means something to the container driver only. K1 adds no relay key (a3); the VM drivers get the relay source in K2 |
| `network` | hyperv's alone: no other driver has a durable switch to name |
| `windows_image`, `windows_image_update` | hyperv's alone: qemu is given its Windows media as the source of `image prepare` |
| `cli` | The container driver's alone |
| `slots` | hyperv and firecracker both take it, but as a flag of `setup` with no key, and with defaults of 4 and 16. A key for it would be a new surface that nobody asked for |
| `tmpdir`, `keep`, `verbose`, `guest_os`, the teardown wait | `warden`'s: sent in the requests, or kept by `warden` (Decision 21) |

**`[driver]` and `[vm]`.** In K1 the two do not overlap. `[vm]` keeps
the four shared VM settings, which are `warden`'s (a9), and the five
driver-owned keys it held take their places in the two tables above. K2
considers a9 again. If the VM drivers then read `cpus`, `memory_mb`,
`build_link` and `dns` themselves, `[driver]` is where those four would
go, as common keys with their `[vm]` names as old names, and
`[driver.<name>] cpus` would give one driver a value of its own, which
`[vm]` cannot do. K1 decides none of that and builds nothing for it.

**Who defines a common key, and who honours it.** The list belongs to
the contract version, so a driver and a `warden` that agree on a
version agree on the list. Version 1 names these two. While version 1
is unstable (**Versions**) its list may change with the rest of it, as
K2 may do for the four VM settings above without a version 2; once a
version is stable, a new common key is a new version. A driver
honours the common keys it lists, and no others. It lists one in
`describe.keys` as `driver.<key>`, with the type the specification
gives it and the commands that read it. Listing it is also listing its
override, `driver.<name>.<key>`, which has no entry of its own. A
driver that lists no common key reads nothing in `[driver]`, and one
that does not list a given key ignores it in `[driver]` and under its
own table (C5); the container driver, vz and firecracker list none in
K1. A key of the driver's own may not
take a common key's name, whether the driver honours that key or not:
in `[driver.<name>]` that name is the override and nothing else.

One name cannot be both a common key and a driver. A driver's name is
`[a-z][a-z0-9]{0,31}` and a common key's is `[a-z][a-z0-9_]*`, so
`[driver] cpus` and a driver called `cpus` would claim one place in the
file. Version 1's two names hold an underscore, which no driver's name
can. A later version that names a common key without one must also
refuse that name for a driver.

**The precedence, with two tables.** For one setting of the driver
`<name>`, the first of these that sets it is the value:

1. its flag, in `args`;
2. its environment variable (the two that exist);
3. `[driver.<name>]` in the config file;
4. `[driver]` in the config file, for a common key;
5. nothing: the driver's default.

A key of the driver's own has no place in `[driver]`, so for it step 4
does not exist. The rule in words: **the flag, then the variable, then
the file, and in the file the driver's own table before `[driver]`.**
Nothing is merged, and no step is a fallback: a value found at one step
that the driver cannot use is an error that names where it was found
(the flag, the variable, or the file with the table and the key), and
the next step is not tried (GLOBAL-10, GLOBAL-11).

The design read at `5808805` had seven steps: the two tables in the
project's `warden.toml`, then the two in the user's config. It rejected
the other order across the two files and showed the difference in a
second worked example. Jeff did not confirm that order. His answer was
that there is one authoritative `warden.toml` (a25, Decision 23), so
nothing is left between files to order, and the rejected alternative and
the second example went with the second file.

One example, with hyperv, qemu and the container driver. The two ages
have neither a flag nor a variable; qemu's and hyperv's defaults for
them are `7d` and `1d`.

```toml
# warden.toml
[driver]
image_refresh_after = "3d"

[driver.hyperv]
image_refresh_after = "12h"

[driver.qemu]
image_min_grace = "6h"

[driver.container]
cli = "podman"
```

| Driver | Setting | Value | Step |
|---|---|---|---|
| hyperv | `image_refresh_after` | `12h` | 3: its own table. `[driver]`'s `3d` is not reached |
| hyperv | `image_min_grace` | `1d` | 5: nothing sets it for hyperv, so its default. `[driver.qemu]` is not hyperv's table |
| qemu | `image_refresh_after` | `3d` | 4: `[driver]` |
| qemu | `image_min_grace` | `6h` | 3: its own table |
| container | `runtime` | `podman` | 3. With `WARDEN_CTR_CLI=docker` in the environment it is `docker` (step 2); with `--runtime finch` on the command line as well, `finch` (step 1) |
| container | the two ages | not read | It lists neither key |

**The seven existing keys** (a25, answered (a); a26, answered (i);
UX-03). Each takes its place in the two tables, and its old name is
still read, until 1.0. An old name as an error at once was asked again,
for another reason, and rejected (a26, option (ii)).

| Old name | New name | What the old name is |
|---|---|---|
| `[vm] image_refresh_after` | `[driver] image_refresh_after` | A second name for the common key, read by qemu and by hyperv, as today |
| `[vm] image_min_grace` | `[driver] image_min_grace` | The same |
| `[runtime] cli` | `[driver.container] cli` | A second name for the container driver's own key |
| `[runtime] relay_image` | `[driver.container] relay_image` | The same |
| `[vm] network` | `[driver.hyperv] network` | A second name for hyperv's own key |
| `[vm] windows_image` | `[driver.hyperv] windows_image` | The same |
| `[vm] windows_image_update` | `[driver.hyperv] windows_image_update` | The same |

An old name is a deprecated alias in UX-03's sense, which GLOBAL-10
calls "a second name for the same value, not a fallback". What that
means here:

- *It stands where its new name stands.* `[vm] image_refresh_after` is
  `[driver] image_refresh_after`: step 4 above, below the file's own
  `[driver.qemu]` or `[driver.hyperv]`. `[runtime] cli` is
  `[driver.container] cli`, step 3. A `warden.toml` that uses only old
  names therefore resolves as it does today.
- *One key, one name.* A file that sets a key under its new name and
  its old one is refused, with a `fixable` error that names the file
  and both names and says to remove the old one. The new name does not
  silently win: the file gives two values for one setting, and only
  its author knows which was meant (GLOBAL-11).
- *A warning, once per command for each old name a file sets.* The
  driver that reads the key prints it on stderr. `warden build`,
  `shell` and `clean` start the driver twice, for a probe and then for
  the command, and the driver reads its settings both times, so the
  rule is tied to the probe and not to the reading: a driver prints
  the warnings when it answers a probe, and in a request that is not a
  probe only for a command `warden` does not probe first (`doctor`,
  `setup`, the image commands and `ext`). A `run` or a `clean` that is
  not a probe prints none, and neither does `reap` or `attach`. Each
  command of `warden` therefore prints the line once:

  ```
  [warden] warning: warden.toml: [vm] network is deprecated; use [driver.hyperv] network
  ```

- *The window.* The old names are read until 1.0 (a26's answer), and
  the configuration reference says so beside each. Taking them out is a
  change of its own, with its own design.
- *Nothing else is renamed.* `--runtime`, `--network` and every other
  flag, `WARDEN_CTR_CLI` and `WARDEN_IMAGE` keep their names and their
  places in the order.

The specification lists the seven old names by driver name, marked
deprecated. It has to: a file that replaces a bundled driver from
`PATH` (a16) carries that driver's name, so it reads that driver's
keys, the old names among them. No other driver has an old name, and a
driver from outside reads nothing outside the two tables.

The container driver's `cli` is also the subject of the security fix
`runtime-cli-project-path`, which merges before K1. Its rule is about
the setting and not the key's spelling: the runtime is `finch`, `docker`
or `podman`, from the flag, the variable and the key under either name.
K1 moves that check into the driver with the setting (**What K1 takes
from the two configuration items**).

What stops two claimants of one key:

- *Two drivers.* Each owns exactly `[driver.<its own name>]`, and a name
  resolves to one binary (**Discovery and trust**). `[driver]` is shared
  on purpose, and only for the keys the specification names, with the
  meaning it gives them.
- *A driver and `warden`.* `warden` reads no key in `[driver]` or
  beneath it and adds none of its own: the keys in `[driver]` are the
  contract's, and `warden` knows their names, and the old names, only
  to check a `describe`. A driver reads nothing outside the two tables
  but the old names listed for it.
- *What holds it.* `warden` refuses a `describe` whose `keys` break the
  rule, before it uses the driver: a `driver.<key>` that is not a common
  key of a contract version `warden` speaks, a key in another driver's
  table, a key of the driver's own with a common key's name, or an old
  name the specification does not list for that driver. The suite's
  static tier checks the same (C4); the kit refuses such a declaration;
  and for the bundled drivers tier 1 reads the declarations from
  source. What nothing can hold is a driver written without the kit
  that reads a key it did not list: the specification forbids it, and
  no check can see it.

**The shared VM settings** (Decision 8; a9, answered (a)) stay
`warden`'s for K1: `warden` reads `--image`, `--cpus`, `--memory`,
`--build-link`, `[vm] cpus`, `memory_mb`, `build_link` and `dns`, checks
the link and the resolvers as the config file loads, and sends them in
`start.vm`. The other reading of a21's answer, that each VM driver reads
them itself, is considered again in K2.

**Help.**

- `warden <command> --help` prints `warden`'s flags as today, then a
  section of its own, `Flags of driver <name> (<file>, or linked):`,
  with the flags the selected driver declares for that command and their
  `usage` lines. The long descriptions of `doctor`, `setup`, `image
  prepare` and `clean` keep today's prose for the bundled drivers in K1;
  a driver may add a paragraph of its own per command (`describe.help`).
- One rule for `--help` when the driver's part cannot be had: help
  never fails and never runs a driver the selection rules refuse. When
  the selected driver cannot be found, its `describe` fails, the config
  file does not load, or the config file names a path (which is
  refused, so that file is not run for `--help` either), `--help`
  prints `warden`'s part, followed by one line that says which driver
  could not be asked and why, and exits 0. Today `--help` loads no
  configuration and cannot fail; this keeps that. The error
  `project-config-trust` adds for an old user-level file is treated
  the same way under `--help`: a line, and exit 0.
- `warden driver list` shows names and files and runs no driver, so it
  shows neither flags nor keys.
- Keys are not in `--help`, as `warden`'s own are not: a driver's keys
  are in its documentation, for the bundled drivers
  `guide/configuration.md`. `describe.keys` exists for the placement
  check above and for tooling.

What is lost: a help text that lists every driver's flags at once
(Behaviour 1); help for a driver that is broken or missing; and any
help `warden` could have generated from a type or a default, since it
knows neither. A flag's default and the form of its value are in help
only if the driver wrote them into `usage`.

**Errors.** Who reports what, when, in what form, and with which exit
code (UX-04, UX-08, UX-09):

| What is wrong | Reported by | When | Form | Exit |
|---|---|---|---|---|
| A flag that neither `warden` nor the selected driver declares for the command: a mistyped flag, or a flag of another driver | `warden` | At parse, after `describe` and before any other driver command | `unknown flag --x for driver "<name>"`, with the command's `--help` for that driver as the fix; a `FixableError` on today's flag-error path | 1 |
| A `warden` flag or key with a bad value, or a config file that is not TOML | `warden` | At load and startup, as today | As today | 1 |
| A line whose parsed `--driver` is not the one `warden` selected by its scan (`--driver` read as another flag's value) | `warden` | At parse | A `FixableError` naming the flag that took `--driver` as its value | 1 |
| A driver's flag with a value it cannot use (`--slots many`) | The driver | For `build` and `shell`, in the probe of `run`: before `warden` takes the lock, creates the output directory or stages anything. For `warden clean`, in a probe of `clean`, before any dead build is reaped. For any other command, first in that command, before it changes or downloads anything | A `fixable` error object that names the flag as typed and gives the fix; `warden` prints `[warden] ERROR:` and `To fix:` with no bug link | 1 |
| A driver's key of the wrong type, or with a value it cannot use | The driver | The same points | `fixable`, naming the file and the key | 1 |
| A `config_file` that does not exist or cannot be read, on a command for which the driver lists a key. `warden` has already refused a named file it cannot read on the commands where it opens the file itself, so this is a command where it does not (`image list`, `image verify`, `warden <name> ...`), or a file that went between `warden`'s reading and the driver's | The driver | The same points | `fixable`, naming the file. Nothing is read in its place: not `./warden.toml`, and not the defaults as if no file had been named (GLOBAL-10) | 1 |
| A file that sets one of a driver's keys under its new name and its old one | The driver | The same points | `fixable`, naming the file and both names; the fix is to remove the old one | 1 |
| A file that sets a driver's key under its old name only | Nobody fails. The driver warns | The same points, and at no other: not again in the `run` or `clean` that follows its probe, and not in `reap` or `attach` (C11) | One `[warden] warning:` line on stderr for each old name the file sets, naming the new one; the value is used | |
| A driver's key, while another driver is selected | Nobody | | It is not read (Behaviour 13). So a common key in `[driver]` is checked only by a driver that honours it, and an override in `[driver.<name>]` only when `<name>` is selected | |
| A key nobody declares: a mistyped key | Nobody, as today | | Ignored, as unknown keys are today | |
| An `args` element the driver did not declare | The driver | In the probe | `fixable`. `warden` never sends one, so it means a driver whose `describe` and parser disagree, or another caller's mistake | 1 |
| In `reap` and `attach`: a recorded config file that has gone | Nobody fails. The driver says so on stderr | | Read as if none had been named (C10) | |
| In `reap` and `attach`: a recorded config file that no longer parses or holds a value the driver would refuse | With a state file, nobody: the driver does not need the file. Without one, the driver, if it reads a setting | First in the command | `fixable`, naming the file; the lock and record stay until it is fixed or removed | 1 |

Every one of these exits 1: UX-09's "every other failure exits 1". The
contract's exit code 2 stays what it was, a request that is not valid
for the contract version, and is not used for a bad value. All of them
come before any real work (UX-08). What moved is how early: `warden`
used to refuse a bad value of a driver's flag or key as it loaded, and
now a driver process is started first, and a driver's bad key is found
only when that driver is selected.

**The four linked VM drivers: one reader, not two.** The kit has one
function that reads a driver's configuration, and both transports and
every command of a linked driver go through it:

```go
// Resolve reads the settings of the driver called name that apply to
// command, from args, the environment and the config file, with warden's
// precedence. configFile is empty when there is none.
func Resolve(name string, settings []Setting, command string, args []string,
	configFile string, environ []string) (driver.Settings, error)

// Call runs one command of d with req, as Main does in a driver process:
// it resolves and checks the driver's settings with Resolve, then calls
// the command's function.
func Call(ctx context.Context, d *Driver, command string, req *Request,
	host driver.Host) (*Result, error)
```

`Main` decodes a request from stdin or from the request file and calls
`Call`. For `run`, `linked.go` builds the same `Request` value in
memory, with the same `args` and the same `config_file`, and calls
`Call`. So for a linked driver the kit reads the config file in
`warden`'s process, a second time after `warden` read it for its own
keys, exactly as it would in a driver process, and no resolved value
passes between `cmd/warden` and a linked driver that would not cross
the pipe.

What is K1's alone, stated so that the exceptions are not mistaken for
the contract:

- *The other commands.* A linked driver's doctor, setup, image and clean
  code stays in `cmd/warden` until K2 (a15), so those commands are not
  dispatched through `Call`. That code calls `Resolve` first, with the
  driver's name and settings, the command, this invocation's `args` and
  `config_file`, where it reads flag variables and `Config` fields
  today. The reading and the checks are therefore the same function on
  every path; the dispatch is `Call` for `run` and `cmd/warden`'s
  existing switch for the rest.
- *What a linked `Describe` lists.* `run`, and each of `doctor`,
  `setup`, `image-prepare`, `image-list`, `image-verify` and `clean`
  that `cmd/warden` serves for that driver in K1; `linked.go` holds the
  list beside the `Driver` value, whose functions for those commands
  are nil. A flag declared for `doctor` or `setup` therefore names a
  command the driver lists, as C1 requires.
- *No `reap`.* It is required of a driver binary and not of a linked
  driver (**The build**, a linked driver).

K2 ends all three: each VM driver's `Driver` value gains its functions,
its binary's `Main` calls them, and `linked.go` goes.

**The reference docs and tier 1.** The bundled drivers' flags and keys
stay documented where they are, in `guide/cli.md` and
`guide/configuration.md` (UX-01, ORCH-03). What `configuration.md`
gains, in the same change:

- `[driver]` in the full reference, with the two common keys, their
  type and meaning, and which bundled drivers honour each;
- `[driver.container]` and `[driver.hyperv]` with their own keys, and
  `[driver.qemu]` and `[driver.hyperv]` as the place to override a
  common key for one driver; `[vm]` is left with `warden`'s four keys;
- the order of the five steps, in words, with the example above;
- a table of the seven renamed keys, old name beside new, with the
  warning's text, the rule that one file may not set both, and the
  window, which is until 1.0;
- the environment-variable table's key column, which names
  `driver.container.cli` and `driver.container.relay_image`.

`guide/drivers.md` names the new keys in each driver's section, and a
flag's help that names a key names the new one (`--network`). A driver
from outside documents its own keys; what the project documents for it
is the specification's list of common keys.

The bundled drivers' declarations are Go literals with constant strings
in their own packages, so tier 1's surface check reads them from source
(**Rule changes**).

- *What it can still check, for a bundled driver:* every `Flag`, `Key`,
  `Common` and `Aliases` string in a `drivercmd.Setting` literal, at the
  base and at the head. A flag or key added or changed without a
  reference doc in the diff is a finding (UX-01, ORCH-03), and one
  removed with no alias is a finding (UX-03), exactly as for a flag call
  or a `toml` tag in `cmd/warden`.
- *What it does not see: the move of the seven keys.* The check knows a
  key by the last element of its path, which is what a `toml` tag gives
  it today. `runtime.cli` and `driver.container.cli` are both `cli` to
  it, so the seven keys are the same set before and after, and neither
  the new tables nor the old names are something the automated part can
  report on. That the old names are still read, warn and are in the
  docs is held by the kit's tests and by review, the agent-judgment
  half of UX-03 and UX-01.
- *What it cannot check:* a driver from outside, which is not in this
  repository, documents its own surface and is seen by nothing here;
  whether a declaration is the whole truth, since a driver that reads a
  variable or a key around the kit is not seen (as the variables the
  drivers read directly are not seen today); and the shape beneath a
  `Raw` key, of which only the path is declared.

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
from the collector; `attach`'s is the shell's, and an attach that failed
(an `error` in its result) exits 1; 124 and 130 keep their meaning
(UX-09). UX-09's table test over the exit paths gains the failed attach,
SIGTERM, and a driver defect.

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
	Describe Describe // name, version, capabilities, settings, ext, help
	// Preflight is what a probe of run calls: the checks the driver makes
	// before it creates anything. It creates nothing.
	Preflight func(ctx context.Context, start *driver.Start) error
	Run      func(ctx context.Context, start *driver.Start, host driver.Host) (kept []string, err error)
	Reap     func(ctx context.Context, req *ReapRequest) (removed int, err error)
	Doctor   func(ctx context.Context, req *DoctorRequest) (blocked bool, err error)
	Setup    func(ctx context.Context, req *SetupRequest) error
	Prepare  func(ctx context.Context, req *PrepareRequest) (path string, err error)
	List     func(ctx context.Context, req *ListRequest) (count int, err error)
	Verify   func(ctx context.Context, req *VerifyRequest) (checked, drift int, err error)
	Clean    func(ctx context.Context, req *CleanRequest) (removed int, err error)
	// Attach returns the shell's exit code, or an error when the attach
	// itself failed; Main writes the error to the result file.
	Attach   func(ctx context.Context, req *AttachRequest) (exitCode int, err error)
	Ext      func(ctx context.Context, req *ExtRequest, args []string) error
}

// Every request type carries the command's Settings (driver.Settings), the
// ones for image-list and image-verify included: their requests name the
// config file too.

// Main runs the command named by os.Args and exits with the contract's code.
func Main(d *Driver)

// Call runs one command of d with req, as Main does in a driver process;
// warden's linked transport calls it for a driver linked into warden.
func Call(ctx context.Context, d *Driver, command string, req *Request,
	host driver.Host) (*Result, error)

// Setting is one value a driver takes: a flag, a config key, or both. A
// driver declares its settings as a literal with constant strings; the kit
// builds describe's flags and keys from it.
type Setting struct {
	Name     string   // what the driver reads it by: cfg.String("runtime")
	Type     Type     // String, Int, Bool, Age, CIDR, IPList, Path or Raw
	Flag     string   // "runtime" for --runtime; "" for no flag
	Key      string   // a key of the driver's own, under its table:
	                  // "driver.nomad.region"; "" for none
	Common   string   // a common key the driver honours, by the name the
	                  // contract gives it: "image_refresh_after". Read in
	                  // [driver.<name>], then in [driver]. Not with Key
	Aliases  []string // the key's deprecated old names, each one the
	                  // contract lists for this driver: "runtime.cli"
	Env      string   // WARDEN_CTR_CLI or WARDEN_IMAGE only (UX-07)
	Commands []string // the commands that take it; EveryCommand is every
	                  // command the driver lists
	Usage    string   // the flag's help line, and the key's
}

// Resolve reads the settings of the driver called name that apply to
// command: the flag in args, else the setting's environment variable, else
// its key in configFile, the request's config_file (ORCH-03); configFile is
// empty when the request names none. A common key is read in
// [driver.<name>] and then in [driver], and an old name counts as the key
// itself.
// It refuses an element of args that settings does not declare, a value of
// the wrong type and a file that sets a key under two names, with a
// FixableError that names where the value came from. When a setting of
// command has a key, it refuses a configFile that does not exist, cannot be
// read or is not TOML, naming the file (C6). Only for reap and attach is
// that different: it reads no file until a setting is asked for, and reads
// a file that no longer exists as if none had been named (C10).
func Resolve(name string, settings []Setting, command string, args []string,
	configFile string, environ []string) (driver.Settings, error)

// SaveState writes v as the build's state file, atomically; LoadState reads
// it back in attach and reap, and reports whether one exists.
func SaveState(stateFile string, v any) error
func LoadState(stateFile string, v any) (found bool, err error)
```

```go
// Package driver: the run dialogue as a driver built on the kit sees it.

// Settings is a driver's resolved configuration for one command. A driver
// reads a value by the name it declared; ok is false when nobody set it,
// and the driver then applies its default. drivercmd implements it.
type Settings interface {
	String(name string) (value string, ok bool)
	Int(name string) (value int, ok bool)
	Bool(name string) (value, ok bool)
	Age(name string) (value time.Duration, ok bool)   // "7d", "36h"
	CIDR(name string) (value netip.Prefix, ok bool)
	IPList(name string) (value []netip.Addr, ok bool)
	Path(name string) (value string, ok bool)
	// Decode reads a Raw setting, a key of any shape, into v.
	Decode(name string, v any) (ok bool, err error)
	// Source is the label to print for a value: --runtime, WARDEN_CTR_CLI,
	// or the file it was read from with the table and key as written there.
	Source(name string) string
	// Warnings are the lines to print on stderr: one for each deprecated
	// old name a config file sets. Call prints them, in a probe and in a
	// command warden does not probe first; a driver's function does not.
	Warnings() []string
}

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
checks `contract`; resolves and checks the driver's settings from the
request's `args` and `config_file` and from the environment, and hands
them to the command's function as a `driver.Settings` (in `driver.Start`
for `run`; for `reap` and `attach` they are resolved only when the
function reads one, C10); prints the settings' old-name warnings, in a
probe and in a command `warden` does not probe first, and not in a `run`
or `clean` that is not a probe (C11); answers a probe; writes the result
file; maps a
`*driver.FixableError` to a `fixable` error object and any other error to
`internal`; and exits with the contract's code. For `run` it points the
process's stdout at stderr before any driver code runs, so a stray print
from the driver or a child cannot corrupt the message stream, and it
turns the end of stdin into the cancellation of `ctx` and the return of
`Wait`. For `run` it also ignores SIGPIPE and drops write errors on the
log and the message stream, so that a driver whose `warden` was killed
still finishes its teardown (**The build**, `warden` dies).

Configuration is the kit's largest part after a21's answer: the
`Setting` declaration, from which it builds `describe`'s `flags` and
`keys`; `Resolve`, which reads the flags, the two existing variables and
the config file with `warden`'s precedence; and `driver.Settings`, the
typed access. All of it runs in the driver's process, or in `warden`'s
for a linked driver, and always through the one `Resolve`
(**Configuration**).

A driver keeps today's shape: where it called `sess.ClaimRelay(ctx, addr,
token, req, env, link, fix)` it calls `host.ClaimRelay(ctx, r)` with the
same address, environment, link and fix; where it called `sess.Wait(ctx)`
it calls `host.Wait(ctx)` and tears down when that returns. It no longer
calls `StartSession`, `StageBuildScript`, `RecordInputs` or
`FinalizeOutput`.

`driver.Host` has two implementations: `warden`'s, over its session, for
a linked driver; and `drivercmd`'s, over the pipe, for a driver binary.
For `run`, a linked driver and a driver binary are thus the same
`Driver` value. In K1 a linked driver's value has `Describe`,
`Preflight` and `Run` and nothing else; its other commands are served
by `cmd/warden`'s existing code (**Configuration**, the four linked VM
drivers).

The Go types are not the contract. They may change with the kit; the
specification and the suite are what a driver is held to.

**Without the kit** a driver does the same by hand: read a JSON line,
write a JSON line, exit with the right code. It needs no Dockerfile
translator, no collector and no claim client. It does need to read its
own configuration: to declare its flags, take them from `args`, and, if
it has keys, parse the TOML file it is named and apply the precedence
itself (requirements C1 to C11 under **Configuration**). A driver with no
flag and no key has only C2's refusal to write. The reference fake (**The
conformance suite**) is written without the kit and has one of each, to
show what it takes.

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
| `describe` | `newDriver`'s container arm; `isVMDriver` | `vm: false`, `shell: true`, `keep: none`, `containerfile: translate`, `guest_os: ["linux"]`; the flag `runtime`; the keys `driver.container.cli` and `driver.container.relay_image`, with their old names `runtime.cli` and `runtime.relay_image`; no common key; `reserved_ranges` its base range |
| `run` | `StartBuild` and `Exec`'s setup | Its probe reads its settings and resolves the runtime (the `runtime` setting, else detection) and refuses a missing or stopped one, as `newDriver` does today before any work. The build resolves it once more and writes it to the state file, then starts the relay container, then on `claimed` the holder, the iptables sidecar, the build container, the executor copy and the start. `mode: shell` starts it held and waits for `/etc/warden/shell.env`, as `waitHeld` does. Its isolation, delivery and start code stays in `driver/container/container.go`, so BOOT-07's surface list does not change |
| `attach` | The second half of `Exec` | `<runtime> container exec -i -t <build> sh -c ...` on the inherited terminal; the exit code from `/.warden/shell-exit`. The runtime is the one in the build's state file |
| `reap` | New; `teardown`'s removals by name | Containers, network, dev relay image and work directory of one build ID, in the runtime the state file names and under the record's `tmpdir`. With a state file it reads no config file, so a deleted checkout does not stop it. With no state file it uses the `runtime` setting as it resolves now from the record's flags and the recorded files that still exist, else detection. A runtime that does not answer is exit 1 with the fixable error, never "nothing to remove" |
| `doctor` | `doctorCmd`'s container arm; `resolveRuntime`, `DetectRuntime`, `runtimeStartHint` in `config.go` | Prints `container runtime: <rt>`, then `ok` |
| `clean` | `listResources`, `isOrphan`, `findLiveWardenProcesses` in `clean.go` | Removes `warden-*` containers and networks and `warden-relay:*` images of every build that is neither in `keep_ids` nor shown alive by the process scan. The scan (`ps aux`, for `warden-relay-<id>` and `warden-build-<id>` in a command line) stays: the driver's `container wait warden-build-<id>` child still exists in `run`, and the scan sees every user's processes, which `keep_ids` cannot (a runtime daemon is shared between users; a lock directory is not). On Windows there is no `ps`, the scan finds nothing, as today, and `keep_ids` is the only signal |

`driver.ContainerBaseCIDR`, `WardenBaseNet` and `AllocateNetwork` stay in
the kit's `driver` package in K1, since `warden` checks the build link
against the range; they move with the ranges in K2. The container driver's
`Runtime` and `Version` fields go: the runtime is a setting, and the
version is the binary's own, stamped at build time.

**The four VM drivers, in K1.** Each gains a `Driver()` value with its
`Describe` and its `Run`, which is its `StartBuild` with the session calls
replaced by `driver.Host`. `vz.New` stops resolving the cache directory,
so building the value does no host work. vz takes `start.build_id` for
its work directory and its relay child, where it made no ID. qemu's and
firecracker's `signal.NotifyContext` goes (Decision 19); the checks each
makes before it creates anything are also what its probe runs. Nothing
else of theirs moves:

- Their doctor, setup, image prepare, list and verify, and clean code
  stays in `cmd/warden` (`host.go`, `image.go`, `image_verify.go`,
  `clean.go`) and their packages, and reads each driver's settings
  through the kit's `Resolve`, where it reads flag variables and
  `Config` fields today. `linked.go` lists, for each, the commands that
  code serves, which is what its `describe` value reports; none has a
  `reap` (**Configuration**, the four linked VM drivers).
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
binary the running executable is the driver, and the bundled copy ships in
`warden`'s directory, so the files are found where they are today. A file
that replaces the bundled copy from another directory (a16) looks beside
itself: it finds `warden-io` and the relay files there, or in its own
source tree's cache, or builds them, and never looks beside `warden`. None of
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

**Two tokens**, to keep them apart. The *claim token* is made by `warden`
per build, given to the driver in `start`, and put by the driver on the
relay's boot channel; it lets `warden` claim that relay once. The
*session token* is the claim's `collector_token`: made by `warden` per
build, sent to the relay inside the claim, and never given to the driver.
It is what separates one build from another at a collector.

**In K1.** `warden` starts today's in-process collector, one per build
with a fresh session token (`driver.StartSession`), and serves it to the
relay over the tunnel it opens on the claim connection. It hands the
driver nothing about the collector. The driver's part of "wiring the relay to
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
5. *Several builds on one collector.* The claim carries one session token
   per build, and the driver contract carries none and says nothing about
   the collector's lifetime; `seam-specs` already writes that lifetime per
   session token.

The client-facing side of a shared collector (opening a session, supplying
inputs, watching console and status, fetching outputs) is what Jeff's
second note describes, and the basis puts it in "a later phase". The
umbrella has no such phase: its Decision 8 keeps alternative collectors
out of this program, and its "Later, not scheduled" names only
"alternative collectors and a collector repository". So that the
direction is written somewhere a later item will find it, this design
proposes one clause there (**Amendments**, section C) and asks (a22).

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
  run`, started by the service manager with no request file, and `warden
  hyperv ...` reaches `ext`.
- The two `StartDetached` re-invocations name the driver binary, and the
  driver writes their request itself.
- What each `reap` needs in its state file: qemu's per-build process
  record, and firecracker's slot.
- qemu's VMs after a second signal or a killed `warden`: they have no
  parent-death signal, so only its `reap` finds them.
- A Windows release target, and on Windows a job object so a killed
  driver's children die with it.
- Who carries the bundled drivers' reserved ranges, whether `image list`
  runs drivers found on `PATH`, and whether packagers need a driver
  directory apart from `warden`'s own.
- `driver/session.go` leaves the kit for `warden`'s own package.
- The specification and the suite move to the specs repository (a17).
- Then every driver becomes its own Go module, with the tier-1 check that
  limits it to the kit's public packages.

**K3** keeps its row: Hyper-V is extracted first.

### What a driver needs besides this contract

The driver contract says how `warden` runs a driver. A driver that is to
run real builds also has to start the project's relay and executor
correctly, and those are other seams, most of them not written yet. An
integrator should know that before starting (a23):

| A driver must | Stated by | Written |
|---|---|---|
| Launch the project's relay in one of its release forms, and give it its pre-claim values (the claim token and the address it accepts the claim on) on its boot channel | Seam 8 (relay release forms) for the artifact and its launch; seam 1 (the claim protocol) for the pre-claim values; RELAY-08 | Seam 1: S1, in review. Seam 8: no phase owns it (a11) |
| Report an environment record for the claim (`claim_values.environment`) | Seam 4, `specs/metadata-environment/` | Yes, today |
| Deliver the executor and start it once (`warden-io initialize`), and nothing else out of band | Seam 5 (the `initialize` contract); BOOT-02, BOOT-03 | S1, in review |
| Report the guest OS, which selects the bootstrap config | Seam 6 (bootstrap config) | S1, in review |
| Make the relay the build's only network path | Seam 7 (the build-link contract) and its suite | S3, not started |

Until those exist, the reference for each is the bundled drivers' source,
and the driver suite checks none of them: it tests the commands and the
lifecycle. The specification says so in its introduction.

### The conformance suite

`driver-conformance --driver <path>` plays `warden` against a driver
binary. It is a program, so a driver in any language can be put under it.
It has its own implementation of `warden`'s side, written from the
specification: it imports `relaycfg` and `collector` to claim the relay
and receive the build, and nothing under `driver/` or `cmd/warden`. A test
under `make test` checks those imports. Its skips come from the binary's
own `describe` and nothing else, so "declared but broken" and "implemented
but undeclared" both fail. It follows the build-lock convention as
`warden` does (**The build record**): it holds a lock for each build it
runs and passes the IDs of the held locks as `keep_ids`.

**Static tier.** No hypervisor, no container runtime, no network. It runs
under `make test`, and in CI on Linux, macOS and Windows runners in the
new `driver-contract` job (**Validation plan**).

| Check | Rule |
|---|---|
| `describe` exits 0 within 10 s, prints one JSON object that matches the schema and nothing else, and prints the same on a second call | GLOBAL-08 |
| The name matches the file name; `run` and `reap` are listed; every flag has a name of the allowed form that `warden` does not use on the same command, a `value`, a usage text and only commands the driver lists (C1); every key is a common key of the contract version, listed as `driver.<key>` with that key's type, or is under `driver.<name>.` without a common key's name; every old name is one the specification lists for that driver name; and each key has a type of the allowed set and only commands the driver lists (C4, C11) | UX-01, a4, a24, a25 |
| `capabilities` are consistent: `attach` is listed exactly when `shell`; `ext` exactly when the `ext` list is not empty; `image_os` exactly when `image-prepare` is | |
| A probe of each listed command with an element in `args` that the driver did not declare exits 1 with a `fixable` error that names the element, and creates nothing (C2) | UX-04, UX-08 |
| With an invalid `warden.toml` in the driver's working directory and `WARDEN_CONFIG` naming another invalid file, and no `config_file` in the request, a probe ends as it does with neither present: the driver reads only the file it is named (C3) | ORCH-03 |
| With a config file that holds `warden`'s tables, another driver's table and an unknown table, one of them written with syntax only TOML v1.1.0 has, and that sets every common key of the contract version the driver does not list, in `[driver]` and in the driver's own table, each to a value of another TOML type than the specification's, a probe ends as it does with no file (C4, C5). This is the ordinary case a4's answer makes: a project sets `[driver] image_refresh_after` and is built with a driver that does not honour it | ORCH-03, a4 |
| For each key the driver lists: a `config_file` that names a path where nothing exists, and one that is not TOML, each make a probe of a command the key names exit 1 with a `fixable` error naming the file; and, for a key whose type is not `any`, a file that gives the key a value of another TOML type makes it exit 1 naming the file and the key. Each creates nothing (C6, C7). The wrong type is given at each place the key may be written: a common key in `[driver]` and in the driver's own table, and a key with an old name under that name too. The container driver's `cli` is checked this way, with `[driver.container] cli = 5` and with `[runtime] cli = 5` | UX-08 |
| `reap` of an ID that never existed, sent a `config_file` that does not exist, ends as it does with no file named (C10) | ORCH-04 |
| Each listed command answers a probe within 30 s: exit 0, or exit 1 with a `fixable` error on a host that lacks what the driver needs. Either way it creates nothing in the temporary directory the suite gave it | UX-08 |
| Each command of the contract that is not listed exits 3 | |
| A request with a contract version the driver does not list exits 4 | |
| A request that is not JSON exits 2 | |
| `reap` of an ID that never existed exits 0 with `removed: 0`, or exits 1 with a `fixable` error when the driver cannot look (no runtime on this host); no other outcome, and the same on a second call | ORCH-04 |
| `run` whose input ends before `start` exits without output on stdout | ORCH-04 |
| `image-list`, when listed, exits 0 with a `count` equal to the number of lines it printed; neither it nor `doctor` changes anything under the temporary directory the suite gave | UX-04 |
| `ext` with no arguments and no request file does not exit 2, when `ext` is listed | |

**Lifecycle tier.** It runs real builds, so it needs a host the driver
runs on (tier 2, TEST-02; tier 1 for the container driver). The suite
stages a script, starts a disk collector, and drives the full sequence.
`--driver-arg <flag>`, repeated, and `--config-file <file>` give the
driver its flags and a config file for that host (the container
runtime's name, Hyper-V's network), passed on as `args` and as
`config_file`;
`--base-image` or `--image` say what the build runs in.

| Case | Checks | Rule |
|---|---|---|
| Probe | A probe of `run` exits 0 on this host and creates nothing | UX-08 |
| Sequence | The driver reports `relay-started`, then after `claimed` `environment-started`, then after `stop` `stopped` and exit 0; every stdout line is a JSON object; the claim endpoint accepts the claim with the token from `start` and no other. This is a check of the reported order: the suite cannot see when the environment really started | GLOBAL-08 |
| Success | A script that prints a marker and exits 0: the collector receives the marker, a completion with code 0, and a ledger whose signature verifies | RELAY-07 |
| Inputs | A file the script reads from the context is in the ledger as a relay-served input | BOOT-04 |
| Failure | A script that exits 3: the completion carries 3, and the ledger is still delivered | UX-09 |
| Stop | `stop` sent after `relay-started`, after `environment-started` and mid-build: each ends in `stopped`, and a following `reap` removes nothing | ORCH-04 |
| End of input | The same three points with stdin closed and no `stop`: the same result | ORCH-04 |
| Orphaned | The same three points with stdin, stdout and stderr all closed at once, as a killed `warden` leaves them: the driver exits by itself, and a following `reap` removes nothing | ORCH-04 |
| Kill | The driver killed at the same three points; `reap`, given the build's values and state file, then removes what was left, and a second `reap` removes nothing. Two more runs, when a config file was given: before `reap` the file is deleted in one and overwritten with text that is not TOML in the other, and `reap` still removes what was left (C10) | ORCH-04 |
| Reap | `reap` of an ID that never existed exits 0 with `removed: 0`, twice | ORCH-04 |
| Diagnostics | After a build, `diagnostics_dir` holds no name but `relay.log` and `console.log` | ORCH-02 |
| Two at once | Two builds with different IDs both succeed (optional per host) | ORCH-04 |
| Clean | With the running build's ID in `keep_ids`, `clean` removes nothing of it; with a killed build's ID absent, `clean` removes it (when `clean` is listed) | ORCH-04 |
| Keep | With `keep`, what `capabilities.keep` names is listed in `kept` and exists; `reap` then removes it (when not `none`) | ORCH-04 |
| Shell | `mode: shell`, then `attach` running `exit 7` on a pseudo-terminal exits 7 with no error in its result; `attach` of an ID that does not exist writes an `error` (when `shell`) | UX-09 |
| Exited | A build script that ends the environment without completing (it powers the machine off, or kills the environment's first process; `--exit-script` supplies one for a guest where neither works): the driver sends `exited` with `which: build` (when `capabilities.exited`) | |
| Doctor | Not blocked; result present (when `doctor` is listed) | UX-04 |

The Clean case removes what `warden clean` run by the same user would
remove, and no more: the `keep_ids` it passes are the IDs of every lock
held in that user's lock directory, its own build's among them. Where
other users run builds on the same host through a driver that has no
liveness check of its own, the case is skipped with `--skip clean`, for
the reason given under **The build**: locks are per user.

It does not test isolation (S3's build-link suite), the claim's security,
or image preparation, which is driver-specific and can take hours. BOOT-01
is held where it was: by each driver's code calling `host.ClaimRelay`
before it starts the environment, and by the build-link suite.

**The reference fake**, `warden-driver-fake` (`conformance/driver/fake`),
is a Go program that uses none of the kit: the standard library and a
TOML parser. It declares one flag with a value (`--greeting`), one
switch and one key of its own (`driver.fake.greeting`), and it honours
one common key, `image_refresh_after`, which it does nothing with but
echo. It honours one and not both so that the suite has a common key
the fake does not list, `image_min_grace`, to set against it (C5). It
reads them by hand with the precedence the specification states, and
in a probe echoes each value on stderr with the place it came from
(the flag, its own table or `[driver]`), which is how the suite's
self-test holds it to C8. Its cases are the steps of the order under
**Configuration** that it can reach: `--greeting` over `[driver.fake]
greeting` (step 1 over 3); and, for `image_refresh_after`, the worked
example's file with `fake` in hyperv's place (step 3), the same file
without a `[driver.fake]` table (step 4), and a file that sets it
nowhere (step 5). It has no variable, so step 2 is held by the kit's
tests for the container driver. It has no old name, since `fake` is
not a bundled name.
Its "relay" is `stubrelay` (`conformance/driver/stubrelay`), a separate
test program built on `relaycfg`: it accepts the claim, reports ready, and
plays the guest by reading directives from the build script the collector
serves (`# stub: output <text>`, `# stub: exit <n>`, `# stub: sleep <d>`,
`# stub: vanish`), then sends heartbeats and a completion. It writes no
ledger, so the suite runs the fake with `--stub-relay`, which skips the
ledger checks. The fake implements all eleven commands: its `setup`,
`image-prepare`, `image-list`, `image-verify` and `clean` work on files
under the temporary directory it is given, its `attach` runs a host shell
command, and it has one `ext` subcommand (`echo`). It has three jobs:

- It shows a driver needs no kit.
- It lets `warden`'s own end-to-end tests run in CI on three platforms
  through a driver `warden` was not built with, for every command of the
  contract: `warden build`, `shell`, `doctor`, `setup`, `image prepare`,
  `image list`, `image verify`, `clean` and `warden fake echo`. No
  bundled binary implements `setup`, the image commands or `ext` until
  K2, so the fake is what shows that K1's `warden` can call them.
- It has fault switches (an environment variable naming one fault: a
  stray stdout line, no teardown, end of input ignored, dying on a closed
  stderr, a wrong exit code, an undeclared command answered, a `reap`
  that reports success without looking, an undeclared argument accepted,
  a `warden.toml` read from the working directory, a table it does not
  own refused, a common key it does not list read from `[driver]`, a
  file found through `WARDEN_CONFIG` when none was named, `[driver]`
  read before its own table, a wrong type
  in its key accepted, a `reap` that fails when a recorded config file
  is gone), and the
  suite's self-test checks that each of its checks fails under the
  matching fault.

A shell-script fake was considered and dropped: the Go relay's host mode
needs raw Ethernet frames on an inherited descriptor and is refused on
Windows, so a script could not start a real relay portably, and a script
that only pretends is `stubrelay` with less control.

**Where it runs.**

| Target | Static tier | Lifecycle tier |
|---|---|---|
| `warden-driver-fake` | `make test`; the `driver-contract` CI job on Linux, macOS and Windows | The same, with `stubrelay` |
| `warden-driver-container` | `make test`; the `driver-contract` CI job on Linux, macOS and Windows | The `integration` workflow (Docker on a hosted Linux runner); natively on macos-finch and container-any |
| qemu, vz, hyperv, firecracker | Not binaries until K2 | K2, one native validation each |

`make test` builds the fake, `stubrelay` and the container driver binary
with the Go toolchain it already runs under and needs nothing else
(TEST-01); Plan chooses between building them in a `TestMain` and
re-executing the test binary. `make driver-conformance DRIVER=<path>` runs
both tiers against any binary.

**Home.** `specs/driver-contract/v1.md` with `v1.schema.json` beside it
(the `schema-doc` check holds the pair together, RELAY-05), and
`conformance/driver/`. Both move to `buildwarden/buildwarden-specs` in K2,
after S1 has created it, where the suite joins S1's harness as a `driver`
role (a17).

### Behaviour

Eighteen things change for a user (a5, answered (a): all eighteen
accepted). Everything else is meant
to be identical. The list was re-derived after the review's answers of
2026-11-14 to a16, a20 and a21, and again after its answer to a4. Of
the redraft's fifteen, none goes; 1, 3, 4, 8, 10, 12 and 13 change, and
16, 17 and 18 are new. Each says so. It was read once more on
2026-11-15 for the one-file decision (Decision 23): 4 and 12 change
again. The list is against `main` as `project-config-trust` and
`runtime-cli-project-path` leave it: that the user-level file is no
longer read, `--config`, `WARDEN_CONFIG`, and the runtime's limit to
three names are those items' changes, not K1's.

1. **Flags and help follow the selected driver.** `warden build --help`
   lists `warden`'s flags and, in a section of its own, the flags the
   selected driver declares (the container driver's, when none is named),
   where it listed every driver's. `warden build --driver hyperv --help`
   shows `--network`. The long descriptions of `doctor`, `setup`, `image
   prepare` and `clean` keep today's prose for the bundled drivers; a
   driver that is not bundled adds its own `help` paragraph. *Changed by
   a21:* the driver's part of help is whatever its `describe` says, so
   `--help` now runs the selected driver's `describe`, and for a driver
   that cannot be found or does not answer it prints `warden`'s part and
   a line that says so. `--help` never fails for the driver's sake, as
   it cannot fail today.
2. **A flag the selected driver does not take is refused.** Today eleven
   flags are accepted and ignored on drivers that do not use them:
   `--runtime` on any driver but container; `--network` on `doctor` and
   `setup`, and `--nat-prefix` and `--host-ip` on `setup`, for any driver
   but hyperv; and on `image prepare`, `--sha256` and `--name` (hyperv
   only), `--edition`, `--virtio` and `--arch` (qemu only), `--force` and
   `--keep-vms` (qemu and hyperv only). `--runtime` is also accepted and
   ignored today on the commands that select no driver (`inspect`, `image
   list`, and `image verify` without `--driver`), and under the
   container driver on `setup` and `image prepare`, which that driver
   does not implement; on all of those it becomes an unknown flag. On
   `build`, `shell`, `doctor` and `clean` under the container driver it
   is accepted as today, before or after the subcommand's name.
   GLOBAL-10 asks for this: a provided value that cannot be
   used is an error. UX-03 would give such a change a deprecation
   window; a5's option (c) was that route, and was rejected. *Stands.*
   After a21 it is not
   a choice `warden` makes flag by flag: a flag that neither `warden` nor
   the selected driver declares cannot be placed on the command line at
   all (**Configuration**).
3. **The driver-specific refusals become the one unknown-flag refusal.**
   Three messages name another driver today: `--network is only used by
   --driver hyperv` on `build` and `shell` (with a fix of its own on
   firecracker), `--slots is only used by --driver hyperv and
   firecracker` on `setup`, and `--update is not supported for --driver
   <d> yet` on `image prepare`. Each becomes `unknown flag --network for
   driver "vz"` with the fix `see 'warden build --driver vz --help' for
   the flags this driver takes`. The VM-group and `--guest-os` refusals
   keep their text. *Changed by a21:* the redraft generated `<flag> is
   only used by --driver <names>` from every bundled driver's
   declarations. `warden` now asks only the selected driver what it
   takes, so it cannot say which other driver a flag belongs to. The
   hint is lost.
4. **"Unknown driver" becomes "driver not found".** `unknown driver "x"
   (valid: container, qemu, vz, hyperv, firecracker)` is replaced by the
   message under **Discovery and trust**, since the set is no longer
   fixed. The `--driver` help names the five bundled drivers and says a
   name or a path of another is accepted. A `warden.toml` may name any
   driver; one whose `[runtime] driver` is a path is refused with its
   own message, where it was "unknown driver" before, and so is any
   file named with `--config`. *Changed by a16:* the redraft refused a
   project file that named a driver that is not bundled. *Changed on
   2026-11-15:* the rule is for the one config file, however it was
   found (a26).
5. **`warden clean --driver qemu` and `--driver vz` no longer run the
   container runtime's cleanup** (Decision 16). They clean up after dead
   builds as in 6, report nothing else to clean, and handle the cache as
   before. With no `--driver` the driver is still container. *Stands.*
6. **`warden clean` first cleans up after dead builds of the selected
   driver.** For each build whose lock nobody holds it runs the driver's
   `reap` with that build's own values, and removes `warden`'s stage
   directory, the record and the lock (**The build**). It prints a
   `Removing ...` line for each and counts them in its total. Then it
   runs the driver's `clean` as before. The container driver's `clean`
   keeps today's process scan and adds the held locks, so it removes
   nothing it would have kept. *Stands.*
7. **A build may complete up to 10 s after its environment exits, on
   every driver.** It was 5 s on container and Hyper-V
   (`completionGrace`, `vmOffGrace`), 10 s on firecracker, and qemu and vz
   did not watch. A build that fails this way takes 5 s longer to say so
   on those two. *Stands.*
8. **A second Ctrl-C.** Today it kills `warden` on container, vz and
   hyperv and leaves whatever the driver had not yet torn down, and is
   ignored on qemu and firecracker, which hold their own handler. After
   K1, for a driver binary (container) it kills the driver and runs its
   `reap`; so does a teardown that takes longer than the teardown wait
   (60 s unless set). A third, or a `reap` that outlasts the same wait,
   ends that `reap` and leaves the rest to `warden clean`. For a linked
   driver a second Ctrl-C ends `warden` at once: as
   today on vz and hyperv, and new on qemu and firecracker. On qemu that
   leaves its VMs running, which a second Ctrl-C could not do before;
   K2's `reap` is what finds them. *Changed by a20:* the wait can be
   set.
9. **SIGTERM tears the build down, on every driver.** Today it does on
   qemu and firecracker, and kills `warden` outright on container, vz and
   hyperv. CI runners and service managers cancel a job with SIGTERM, so
   this is the case that left containers and VMs behind. The exit code is
   1, as on qemu and firecracker today. *Stands.*
10. **The output directory gains `driver.json`**, and it and `inputs/` are
    written before the driver starts. A driver's refusal that its probe
    did not catch therefore leaves them in the output directory, where a
    few early failures left an empty directory or none. The refusals each
    driver makes today before it creates anything are in its probe, and
    still leave nothing. *Changed by a16:* the file also says where the
    driver was found and which copies it shadowed.
11. **`warden driver list` is new**, with `--json`. *Stands.*
12. **`--driver` and `WARDEN_DRIVER` accept a path** to a driver
    binary. *Changed by a16:* a path in the user's config file was
    accepted too, where the redraft allowed no config file to name one.
    *Changed on 2026-11-15:* the user-level file is gone, so no config
    file names a path, as in the redraft (a26).
13. **A driver's keys are read only by that driver, and only when it is
    selected.** Two things follow for the seven existing driver-owned
    keys. A malformed `[vm] image_refresh_after` or `image_min_grace` is
    refused today on `build` and `shell` whichever driver is selected
    (`newRequest` parses both ages for every driver) and on no other
    command; after K1 it is refused by qemu and by hyperv, on the
    commands of theirs that read it, and not looked at under another
    driver. And a wrong TOML type in any of the seven (`[runtime] cli =
    5`) fails the load today for every driver, on every command that
    loads configuration (`build`, `shell`, `doctor`, `setup`, `image
    prepare` and `clean`; `image list`, `image verify`, `inspect` and
    `hyperv service` load none); after
    K1 it is refused only by the driver that owns the key, and passes
    unseen under another. The ages' message keeps its text and fix
    (`warden.toml [vm] image_refresh_after: invalid age "soon"`) and
    names the file the value was read from, where it always said
    `warden.toml`. A wrong type gets a message that names the file and
    the key, where today's is the parser's own with the fix `fix the
    TOML syntax`; that one is a golden diff. These refusals now come from the driver's probe: after
    `warden`'s own checks, and still before anything is created.
    *Changed by a21:* the redraft kept the load-time type check of the
    seven under every driver; with the driver as their one reader it
    goes. After a4 the same holds for a key under its new name: a
    common key in `[driver]` is checked only by a driver that honours
    it, and a key in `[driver.<name>]` only when `<name>` is selected.
14. **`--timeout` counts from the moment `warden` starts the driver, on
    every driver.** Each driver started its own clock, at slightly
    different points of its preflight. *Stands.*
15. **A path that is not valid UTF-8 is refused for a driver binary.** A
    build context, output directory or `--tmpdir` whose name is not UTF-8
    cannot be sent to a driver process unchanged. In K1 that is the
    container driver; the linked drivers take it as today. *Stands.*
16. **A `warden-driver-<name>` earlier on `PATH` than the bundled copy
    runs in place of the bundled driver of that name** (anywhere on
    `PATH`, when `warden`'s own directory is not on it). Nothing has
    that name today, so nothing changes for a user who has put no such
    file there. One who has, gets it for every command, with no line at
    build time to say so; `warden driver list` and `driver.json` show
    it. A file of that name reached through a relative `PATH` entry is
    an error that names the entry, not a driver. *New, from a16.*
17. **`[build] teardown_timeout` is new**: how long `warden` waits for a
    driver binary to tear down before it kills it and reaps, and then
    for that `reap`, 60 s unless
    set. Additive; nothing waited or killed this way before. *New, from
    a20.*
18. **A driver's keys live in `[driver]` and `[driver.<name>]`, and the
    seven existing ones are renamed into them.** `[driver]` holds the
    two common keys, `image_refresh_after` and `image_min_grace`;
    `[driver.container]` holds `cli` and `relay_image`;
    `[driver.hyperv]` holds `network`, `windows_image` and
    `windows_image_update`. Three things are new for a user. The seven
    old names (`[runtime] cli` and `relay_image`, and the five in
    `[vm]`) are still read and mean what they meant, and each one a
    file sets now prints one warning line per command that names the
    new key. A file that sets a key under both names is refused. And
    one driver can be given its own value of a common key
    (`[driver.qemu] image_refresh_after`), which today's one pair in
    `[vm]` cannot express. A config that is not edited resolves to the
    same values as today. Help that names a key names the new one
    (`--network`'s usage line). *New, from a4;* what happens to the old
    names is a25's answer, which a26's answer keeps: they are read
    until 1.0.

What does not change, although the code behind it moves:

- `warden shell` on a driver without `capabilities.shell` gives today's
  error and fix.
- `warden image list`, `image verify`, `image prepare`, `setup` and
  `doctor` for the four VM drivers: their code does not move in K1.
- `warden hyperv service ...`.
- The container driver's progress and error lines keep their text; they
  now come from the driver's stderr.
- Every driver flag keeps its name, its two spellings (`--runtime docker`
  and `--runtime=docker`) and the commands it is accepted on, and every
  driver key keeps its name and meaning and is still read in the table
  it has today (18). The precedence between
  a flag, `WARDEN_CTR_CLI` or `WARDEN_IMAGE`, and the config file is the
  same; the kit applies it where `cmd/warden` did.
- The environment variables the drivers read for themselves (**Defaults
  and overrides**): a driver process inherits `warden`'s environment.
- Exit codes (UX-09): the build's code, 124, 130, and 1 for a blocked
  doctor with no extra line.
- Which image a VM build boots when the context has both a script and a
  Dockerfile: the Dockerfile's `FROM`, as today (`start.base_image`).

`docs/book/src/guide/cli.md`, `configuration.md`, `output.md` and
`drivers.md` are updated for each in the same change (UX-01). No existing
flag, subcommand or environment variable is renamed, and nothing is
removed. Seven config keys are renamed, each with its old name kept as
a deprecated alias until 1.0, which is UX-03's route (18, a25, a26).
Eleven flag-and-driver combinations that were accepted and ignored are
refused (2), which GLOBAL-10 asks for and for which UX-03 would give a
window; no window is given (a5, answered (a)).

### What guards behaviour

- **Characterization tests, written first, on unchanged code.** Before any
  production change, `cmd/warden` gains golden tests of: `--help` for every
  command; the error and fix of every refusal, per driver; the
  `BuildRequest` each driver gets from a table of flags, environment and
  the config file (extending `request_test.go`, `vmdns_test.go` and
  `buildlink_test.go`); the refusal of a malformed age and of a wrong TOML
  type, per command; `image list` and `image verify` over fixture caches;
  the files `inputs/` holds after staging, per driver; and, for each VM
  driver, the image a build resolves for a context that holds a
  Dockerfile and is built with `--script` (the choice is made inside the
  driver today, in `linuxguest.ContainerfileImage`, so a request table
  alone would not see it). The later commits may change a golden only
  for the eighteen differences above, so each shows up as a reviewed
  golden diff.
- **The request tables become two tables, over the same rows.** Today one
  table of flags, environment and files is asserted against the
  `BuildRequest`. After K1 `warden`'s half of each row is asserted
  against the `start` message, with the `args` and `config_file` it
  carries; and the driver's half is asserted against what the kit's
  `Resolve` returns for that row, per driver. A setting lost on either
  side of the hand-over fails a test, and so does a precedence that
  differs from `main`'s between a flag, a variable and the config
  file. The rows keep the seven keys under their old names,
  which shows that a config nobody edits resolves as it does today;
  each row is asserted again with the new names (Behaviour 18).
- **The surface check.** Tier 1's extraction (Rule changes) must find the
  same set of flags and keys on the branch as on `main`. A flag or key
  lost in the move fails `surface-removed`.
- **The drivers' own tests.** Each driver's relay-start, environment and
  teardown functions keep their unit tests; only the calls into the
  session change. Firecracker's `waitBuild` tests move with the rule to
  `warden`'s build sequence.
- **`warden`'s end-to-end tests against the fake**, on three platforms in
  CI: success, failure, timeout, Ctrl-C, a second Ctrl-C, SIGTERM, a
  driver that dies, a driver that prints noise, a killed `warden` whose
  driver must tear down alone, shell, doctor, setup, the image commands,
  the fake's own command, and clean after a dead build.
- **The suite**, the `integration` and `firecracker` workflows, and the
  native steps under **Validation plan**.

### What K1 takes from the two configuration items

Two items in the queue change `warden`'s configuration while K1 is in
review, and both merge before K1 does: `runtime-cli-project-path`, then
`project-config-trust`, then K1. `runtime-cli-project-path` was approved
on 2026-11-15 at `a2afd08` (its branch at `119a0e1`) and is in Plan; it
is cited here as approved. `project-config-trust` is still in Design and
is not approved; it is cited as proposed, at `539fd38`. K1 follows what
merges.

**`project-config-trust`: one config file.** Jeff decided it on
2026-11-15 (a25, with his words; Decision 23) and gave the work to that
item. What its design proposes, and what K1 does with each point:

| That design | In K1 |
|---|---|
| One file, found by `--config`, else `WARDEN_CONFIG`, else `./warden.toml` when it exists; a named file that cannot be read is an error and nothing else is tried; an empty flag or variable is not set; a named file is the only one opened (its Decision 1) | `config_file` is the path that lookup returns, made absolute, and is absent when it returns none. `warden` applies the lookup; a driver never does (C3) |
| `--config` is a global flag, accepted on every command and opened only by those that read configuration (its Decision 2) | The scan that finds the driver finds `--config` the same way, and a driver may not declare a flag `config` (C1). On `image list`, `image verify` and `warden <name> ...` `warden` names the file to the driver without opening it; a driver that reads a key there refuses a file that does not exist or that it cannot read, and does not go on as if none had been named (C6) |
| The order is flag, environment variable, the config file, default, for every key (its Decision 4) | The kit's precedence, and C8 |
| `~/.config/warden/config.toml` is not read, and one that exists is an error, transitional; one line says which file was read (its Decisions 3 and 7) | Both are `warden`'s own and stay in `cmd/warden`. K1 changes neither, and neither is sent to a driver |
| What a config file may decide is not limited by where the file was found; the protection for a repository the executor does not control is to name the file (its Decision 7, asked as its a4) | The requests carry no level and nothing about how the file was found. For `[runtime] driver` that design leaves the question to K1: "K1 decides it again". K1 decided it as a26's second part, answered (A): no config file names a driver by path |
| A relay image that is not shaped like an image reference is refused, from the file and from `WARDEN_IMAGE`, on the value in effect and only when the container driver is selected (its Decision 8) | The check moves into the container driver with the setting, and holds under both names of the key |
| A background `warden image prepare` is started with `--config` naming the file its build read (its Decision 10) | qemu and hyperv, linked in K1, start it with the `config_file` of their `start`, when there is one. K2 has the driver write the child's request itself (**The commands**) |
| Its tier-1 cases run the built `warden` | They pass on K1's branch unchanged |

Two sentences of that design, at `539fd38`, do not hold of K1. Neither
is changed from here. Both are handed to that item's Design, which is
still in progress, by the attended session, since nothing in that item's
queue file carries them yet:

- It leaves recording which config file a build used to
  "`driver-contract`'s build record". That record is in the lock
  directory and is removed when a build ends cleanly; `driver.json`, in
  the output, does not name the config file, and K1 adds nothing to the
  output for it. After both merge, the only record of which file a
  build read is the line that item prints as the build starts (its
  Decision 7). If
  the output is to name it, that is a field to add to `driver.json` and
  a nineteenth change of behaviour, which neither design proposes.
- It says "K1's `config_files` then has at most one entry and no
  `level`". K1's field is `config_file`, one string, absent when there
  is none (Decision 23).

**The order.** K1 builds none of that item's part: not the removal of
the user-level file, not `--config` or `WARDEN_CONFIG`, not the
transitional error, not the opening sentences of ORCH-03 or the levels
of UX-07. K1's code cannot be written before `main` reads one file: its
characterization goldens, its request tables and its `config_file` would
be written against two. So K1's Plan and Implement start only after
`project-config-trust` has merged to `main` and `main` has been merged
into this branch.

- *How the queue holds it.* By a blocking question (FLOW-07) that names
  the merge, the form `wf new -hold` uses for the freeze and the one
  `project-config-trust` proposes for its own wait behind
  `runtime-cli-project-path` (its a7): a question on this item, `q1`,
  whose text is that it resumes when `project-config-trust`'s PR has
  merged, with a `human` step `f2-answer-q1` that `f2-plan` needs. The
  cloud takes no step of a feature that has an open blocking question,
  so neither Plan nor Implement starts, and Jeff answers it when he has
  merged that PR.
- *When it was written: during this review, before its outcome.*
  Recording an approval marks `f2-design-review` done and does nothing
  else, and before the hold was written `f2-plan` needed only that
  step. On a copy of the queue state
  with the review marked done and nothing else changed, `wf next` offers
  `driver-contract f2-plan` to the cloud; with the question and the step
  added, `wf validate` passes and `wf next` reports the item blocked,
  waiting on `f2-answer-q1`. So a hold left to the session that records
  the outcome is a hold that one forgotten edit loses, and the next tick
  would plan K1 against a `main` that reads two files. Raising it now
  holds nothing else: an open blocking question stops Design (phase 1 is
  not done with one open), and K1's Design step is done; the review
  step's own check does not look at questions. The attended session
  wrote it on 2026-11-15, at workflow `ea23718`: the step
  `f2-answer-q1`, before `f2-plan`, which needs it, and the question
  `q1`, blocking and open. The fold-in that wrote this section could
  not, being limited in the queue file to a5, a8 and one new question,
  which was a26. The **Design review** section records it. If this
  review returns the design to Design (FLOW-06), the hold is answered or
  taken out first, and raised again when the design is approved.
- *Why before Plan.* `project-config-trust`'s design says of K1's hold
  that it is "raised when its Plan is done". This design raises it one
  step earlier, for two reasons: Plan's tasks name `LoadConfig` and the
  config tests as `main` has them, so a plan made before that merge is
  made against two files; and a K1 that has claimed its Plan holds one
  of FLOW-05's three `alters_relationships` slots (the limit in the
  workflow branch's `config.yaml`) for as long as it waits, with nothing
  to do. Nothing in that item depends on which of the two it is.
- *If K1 came to Implement first.* It must not. It would be seen at
  once, since `main` would still read `~/.config/warden/config.toml`.
  Implement then stops and reports its step blocked. It does not write a
  two-file form of the contract, and it does not carry the removal
  itself. If `project-config-trust` is withdrawn, or merges with another
  answer to which file is read, K1 returns to Design (FLOW-06). Publish
  merges `main` again and checks the same.

**`runtime-cli-project-path`: the container driver's runtime.** A
security fix (`host-env`), approved at `a2afd08`. On `main` a
`warden.toml` in the working directory can set `[runtime] cli` to a file
the repository ships, and `warden` runs it. Jeff's decision in that
review: the container driver's runtime is limited to the runtimes it
supports, `finch`, `docker` and `podman`, from every source. That item
makes the check in `cmd/warden` and merges first of the three. K1 moves
the setting into the container driver, and the check goes with it. By
the order above, that item has merged before K1's Plan starts. What K1
has to keep, which is that design's hand-off:

1. The container driver checks its `runtime` against its supported list
   when it initializes: in the probe of `run`, and first in `doctor`,
   `clean` and any other command of its own that resolves the runtime.
   The check is on the value the kit resolved, whether it came from
   `--runtime`, `WARDEN_CTR_CLI`, `[driver.container] cli` or the old
   name `[runtime] cli`. The value run is the list's own member, never
   the configured string. A value that is not on the list is that item's
   error, naming the source as `Source` labels it, exit 1, before the
   driver looks up, runs or creates anything. Nothing lower is tried in
   its place, and detection is not.
2. Only when the container driver is selected (Behaviour 13). `warden
   clean --driver qemu` and `--driver vz` stop running the container
   runtime (Behaviour 5), so they stop checking it; that item's one test
   of it takes Behaviour 5's result.
3. The state file holds a value that passed the check, and the value
   `reap` or `attach` reads back from it is compared with the list again
   before it is run, so that "the value run is the list's own" does not
   rest on the contents of a file. Where the driver resolves the setting
   again, the check runs again.
4. That item's transitional detection, which gives the forms that ran
   before the fix a message of their own, moves with the check: one file
   behind one call, with its marker and its removal point, which is 1.0
   (its a4, answered (d)).
5. That item's integration cases pass on K1's branch unchanged, but for
   the one in point 2. Its unit tables move with the code, and K1 adds
   for `[driver.container] cli` the rows they have for `[runtime] cli`:
   a K1 that checked the old name only would pass every test that item
   wrote.

The design read at `5808805` said that "whichever of the two changes
merges second applies that refusal", and carried each file's level so
that the refusal could be made for a project's file alone. Both are
overtaken: K1 is the second, and the rule has no levels.

**The general rule (UX-08, as that item amends it; approved there by
name at `a2afd08`).** In the
same review Jeff asked for a rule for the whole project, and chose that
item to write it: "configuration checking should be validated by the
consumer/user of that configuration on initialization after applying
overrides like environment variables", required of the project's own
components and "highly recommended" for others', and lazy, so that one
`warden.toml` may hold sections for drivers a run does not use, which
"aren't validated". K1's design already does this for drivers and needs
no change for it. A driver reads and checks its own settings in its
probe, after the kit has applied the flag and the variable over the file
(C7, C8). A driver's key is read only when that driver is selected
(Behaviour 13, and the row "A driver's key, while another driver is
selected" of the errors table). And a driver ignores every table it does
not own (C5). `warden`'s refusal of a flag that neither it nor the
selected driver declares (Behaviour 2) is what that wording allows of a
flag: "a command may refuse, at startup, a flag that its run cannot
use". For a driver the project did not write, the contract asks the same
and the suite checks what it can see; nothing makes such a driver run
the suite. Two things in that wording reach K1. It lists the four
checks on `main` that predate it, and has the change that moves one to
its consumer take it off the list. K1 moves two of them, the two image
ages and the type of the seven driver-owned keys, so K1 declares that
edit as a rule change of its own (**Rule changes**, UX-08; a8). And it
leaves `[vm] build_link`, `[vm] dns` and the type of `warden`'s own keys
checked as the file loads, under every driver; that item moves none of
them (its a5, answered (a)), and its hand-off notes that K1's design
keeps them where they are. It does: `warden` checks the link and the
resolvers as the file loads (Decision 8; a9, answered), and they stay
on the list unless a9 is opened again.

### Order of the work

One branch, one PR, each commit green on `make test` and `make lint` for
the platforms it touches (FLOW-17). Before step 1,
`project-config-trust` has merged and `main` is merged into the branch
(**What K1 takes from the two configuration items**), so the
characterization tests are written on a `main` that reads one config
file.

1. Characterization tests; and the specification with its schema, which
   LOOP-02 has written before the code that implements it. No production
   code changes.
2. `warden` stages the script and records inputs, and passes the staged
   path and the base image to each driver in the request. The drivers stop
   staging. Still `driver.Driver` and in-process sessions.
3. `driver.Host`, with `warden`'s build sequence over the session
   (`buildseq.go`), the build lock and record (`buildrecord.go`,
   `driver/hostlock.go`), and `linked.go`. One commit per driver moves its
   `StartBuild` onto `Run`: container, firecracker, qemu, vz, hyperv (the
   two CI can run end to end first). The four-method `driver.Driver` goes
   when the last has moved. The grace becomes `warden`'s 10 s and the
   signals become `warden`'s here.
4. The container driver's `Exec` is split into the held start (`mode:
   shell`) and the attach.
5. `driver/drivercmd`; the fake and
   `stubrelay`; the suite's static tier; the tier-1 surface change; the
   `driver-contract` CI job.
6. The exec transport and the lookup in `cmd/warden`; the setting
   declarations of all five drivers, read through the kit; the seven
   driver-owned fields leave `Config`; `warden driver list`;
   `driver.json`; `warden`'s end-to-end tests against the fake.
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
Windows release target today and K1 adds none. No CI job builds `warden`
for Windows today either; the new `driver-contract` job builds `warden`,
the container driver and the fake on a Windows runner and runs their
tests there (GLOBAL-07, ORCH-05).

### Risks

- **Stdout pollution.** One stray line from a driver or its child on
  `run`'s stdout breaks the build. The kit redirects stdout; the suite
  checks every line.
- **A driver that ignores the end of its input.** `warden` kills it and
  calls `reap` on the same binary, which may be as broken.
- **What a21's answer costs**, stated plainly:
  - *No early refusal of a bad value by `warden`.* `warden` still refuses
    a flag nobody declares, because it cannot parse a command line
    otherwise (a24). It no longer sees a driver's values at all: a
    malformed flag value or key is refused by the driver, one process
    start later than today, and a driver's key is checked only when that
    driver is selected. A wrong TOML type in `[runtime] cli` under
    `--driver qemu` goes unreported, where today it fails the load
    (Behaviour 13).
  - *Help depends on the driver.* The driver's flags in `--help` are what
    its `describe` says, and are missing when the driver is missing or
    broken. `warden` cannot add a default or a value's form that the
    driver's usage line leaves out, and the hint that a flag belongs to
    another driver is gone (Behaviour 3).
  - *Tier 1 sees the surface only of the bundled drivers*, from their
    Go literals. A driver from outside documents its own flags and keys,
    and nothing here checks that it does.
  - *A driver written without the kit re-implements the reading*: the
    flags, a TOML parser if it has keys, and the precedence, which is
    the one requirement the suite cannot check against a binary (C8).
    Two drivers can therefore differ in precedence without any check
    failing.
  - *Two readers of one file.* `warden` and the driver each parse the
    config file. A file that changes between the two reads can make
    them disagree. So could a parser for another TOML version: the
    file is TOML v1.1.0, which `warden` reads, and a driver in a
    language whose parser stops at 1.0 would refuse a file `warden`
    accepted, on syntax in a table that is not even its own. The
    specification names the version and the suite's static tier holds a
    driver to it (C5); the cost is that such a driver needs a 1.1
    parser before it can have keys at all.
  - *`reap` and `attach` run after the configuration may have changed
    or gone.* The record keeps the build's flags and file path, not the
    values resolved from them. A driver that needs a value to find its
    build again must write it to its state file, and in those two
    commands the state file is what counts: a recorded file that is
    gone is read as not listed, and with a state file no config file can
    fail them (C10). A driver written without the kit that re-checks
    its configuration in `reap` would leave a dead build's lock in place
    for good once the build's checkout is deleted; the suite's Kill case
    is what catches it.
  - *`warden` finds the driver before it can parse the line.* The scan
    for `--driver` knows no other flag's arity, and can disagree with
    the full parse on a line where another flag's value is missing.
    `warden` compares the two and refuses such a line; it never builds
    on a driver the parsed line does not name.
- **A driver that dies on a closed pipe.** After a killed `warden` a
  driver's stdout and stderr are closed, and a Go program is killed by
  SIGPIPE at its first write to either. The kit prevents it and the
  suite's Orphaned case checks it; a driver written without the kit has
  to handle it itself.
- **Locks are per user.** `keep_ids` is built from the locks in the
  user's cache directory, so it does not protect a build started by
  another user, under `sudo`, with another `HOME`, or by a `warden` from
  before K1 during an upgrade. The container driver keeps its process
  scan beside it, so nothing regresses there on Unix; on Windows the
  scan finds nothing, today and after. A driver that relies on
  `keep_ids` alone on a host shared between users can remove a live
  build of another user; the specification says so.
- **A killed pair leaves the runtime's containers running** until `warden
  clean`, as a killed `warden` does today (a20's option (b) is the
  sweep).
- **A file on `PATH` replaces a bundled driver without a word at build
  time.** That is the review's answer to a16, and the cost is stated
  here: a `warden-driver-container` that reached a `PATH` directory
  ahead of `warden`'s by accident (a stale build, another checkout's
  `bin`) or by design runs in place of the shipped one. `warden driver
  list` shows it and every build's `driver.json` records it; nothing
  warns. The reverse surprise exists too: a customized copy in a `PATH`
  directory after `warden`'s, or one without its execute bit, does not
  run, and only `warden driver list` says why.
- **Windows.** K1 kills the driver process, not its process tree (that
  needs a job object). The container driver's resources are held by the
  runtime and found by `reap`, so K1 does not depend on it; K2 does, for
  Hyper-V.
- **A driver written without the kit may omit `exited`.** A dead
  environment is then noticed only by the loss of the tunnel or by the
  30 s heartbeat timeout, which starts after a first heartbeat.
- **Claim ports on loopback** can be dialled by any local process and are
  guarded only by the token. Three drivers expose the token to other
  local processes today: the container driver puts it on the runtime
  CLI's command line (`--env WARDEN_CLAIM_TOKEN=...`) and in the relay
  container's environment, vz puts it in the relay child's environment,
  and qemu puts it in its VM's command line. Unchanged by K1.
- **The contract may change in K2.** K1 proves it on one real driver and
  a fake. vz and Hyper-V are the hard cases as separate processes
  (Decision 14), and version 1 is unstable so that K2 can change it.
- **Three-platform tests of existing code.** The new CI job runs
  `cmd/warden`'s tests on macOS and Windows runners, where CI has never
  run them. They compile for both (checked for this design); whether
  they pass there is not known. Plan's task for the job runs them first
  and fixes or build-tags what fails.
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
  Those three are to be filed as follow-up items. A fourth finding of
  the redraft's reviewers, that `[runtime] cli` in a project's
  `warden.toml` is run as given, was filed as the security fix
  `runtime-cli-project-path` and is closed there, before K1 (**What K1
  takes from the two configuration items**).
- **K1 is written to fit two designs that are not merged, and one of
  them is not approved.** `runtime-cli-project-path` was approved at
  `a2afd08`; `project-config-trust` (read at `539fd38`) is still in
  Design. What K1 takes from each is listed (**What K1 takes from the
  two configuration items**), and a point that `project-config-trust`'s
  review changes, or that either item changes before it merges, is a
  finding for Design (FLOW-06). The one that would reach the contract is
  `project-config-trust`'s
  Decision 7, asked there as a4: if a defaulted `./warden.toml` came to
  be limited in what it may set, the container and hyperv drivers would
  have to be told how their file was found, for `relay_image` and
  `windows_image`.
- **The scan finds two flags.** `--config` has to be found before the
  command line can be parsed, as `--driver` is, because the driver may
  be named in the file. The check that the scan and the parse agree
  covers both; a line where they disagree is refused.
- **What a4's answer costs.** A driver written without the kit has two
  places to read for a common key, and the easy mistake is to read
  `[driver]` first; the suite cannot see which place won (C8), so only
  the fake's self-test and the specification's example hold it. The list of
  common keys is short because today's inventory gives it two entries,
  both about prepared Windows images; a key added once version 1 is
  stable needs a new contract version. And the rename prints a warning
  for every existing config that sets one of the seven (Behaviour 18,
  a25).

### The overview (FLOW-15)

K1 is the only item of its phase, so it makes the sweep itself. The
orchestrator's pseudocode in `concepts/overview.md` changes to:

```
warden build <context>:
    load warden's own configuration from the one config file (--config,
        found by a scan of the raw arguments, else WARDEN_CONFIG, else
        ./warden.toml)                      # the driver may be named in it
    driver := the driver named by --driver (found by the same scan),
              WARDEN_DRIVER or [runtime] driver (in the config file a
              name, never a path), else container: a path as
              given, else warden-driver-<name> on PATH in PATH's order,
              else beside warden, else a linked driver; not found is an
              error
    d := driver.describe                    # what it is; the flags it takes
    parse the command line: warden's flags, and the driver's declared
        flags, whose values are passed on unread; a flag neither declares
        is refused, and so is a line whose parsed --driver or --config
        is not the scanned one
    req := resolve warden's flags and config (output dir, capture, ...)
    VM drivers (capabilities.vm): build link and [vm] dns, as today
    probe `driver run`                      # the driver reads and checks its
                                            # own flags and keys, then makes
                                            # its preflight
    lock build-<driver>-<id>; write the build record beside the lock
    write <output>/driver.json
    script := translate(Containerfile, context)     # see Translator; not when
                                                    # the driver ignores one
    if script was translated: keep both under <output>/inputs/   # ORCH-02
    start `driver run`; send start{id, claim token, script, base image,
        state file, the config file's path, the driver's flags, ...}
    <- relay-started{claim address, environment, build link, resolvers}
    session := StartSession(output dir, context, script, guest OS)
    session.ClaimRelay(claim address, claim token, the driver's values)
    -> claimed
    <- environment-started
    session.Wait()      # completion, a failure, SIGINT or SIGTERM, or the
                        # driver's `exited` (a relay exit at once, a build
                        # exit after 10 s)
    -> stop; <- stopped                     # the driver tears down
    a driver that died or would not stop: kill it, `driver reap <id>` with
        the record's values
    remove the stage directory, the record and the lock
    finalize output (optional zstd); print the result; exit with the build
        script's exit code
```

Each driver's skeleton replaces `StartBuild`:

```
run (one process per build, or a linked driver's Run):
    <- start
    read its own configuration: its flags from start, the two existing
        variables, its keys from the config file start names (its own
        table, then [driver] for a common key), with warden's
        precedence                                             # the kit
    write the state file, if reap will need more than the ID   # per driver
    resolve the relay and the environment's image              # per driver
    VM drivers: refuse a build link a host interface holds, and a launcher
                asset whose link-channel stamp is missing or old
    relay := start the relay, unconfigured, with the claim token # per driver
    -> relay-started; wait for claimed
    isolate the build environment                          # per driver
    deliver warden-io into it                              # per driver
    start it                                               # per driver
    -> environment-started; wait for stop or the end of input
    write relay.log and console.log; teardown, even with stdout and
        stderr closed
    -> stopped
```

A short section describes the other commands (`doctor`, `setup`, `image`,
`clean` with its cleanup after dead builds, `shell`'s `attach`, a
driver's own) as one process each, and the lookup. `contributing/structure.md` gains `driver/drivercmd`,
`driver/relaysource`, `cmd/warden-driver-container`, `specs/driver-contract`
and `conformance/driver`.

The dead-code sweep: `make deadcode` runs over the new programs too, so a
kit function no driver or test calls is reported and removed. FLOW-15's
stated reasons for keeping an unreachable exported function, which for
this repository are PROFILE-01's Sweep entry, do not cover `driver/`;
widening them is a change to that entry for K2, when the kit has
consumers outside this repository.

## Defaults and overrides

Each default, override and fallback K1 adds, changes or moves. Anything
not listed is unchanged and `warden`'s.

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The driver when none is named: `container` | Unchanged | It is today's default and needs no host setup | `--driver`, else `WARDEN_DRIVER`, else `[runtime] driver` (today's order). A named driver that is not found is an error; the default is not tried |
| Which file runs for a driver name: the first `warden-driver-<name>` on `PATH`, in `PATH`'s order; when `PATH` has none, the bundled copy (a file beside `warden`, else a driver linked into it). In the usual install `warden`'s directory is on `PATH`, so the bundled file is itself the first on `PATH` unless another comes before it | **New** (a16) | `PATH` is where a user puts a program they mean to run, and how git and kubectl find a command; its order is the one a user already knows; the bundled copy is what runs when the user has put nothing ahead of it, which is every install that has not been customized | A path in `--driver` or `WARDEN_DRIVER`: that file and nothing else, and if it cannot be run that is the error. The order falls through only on absence, from `PATH` to the bundled copy when `PATH` holds no candidate of the name. It never falls through on failure: a file found on `PATH` whose `describe` fails or which fails to start fails the command, and the bundled copy is not tried (GLOBAL-10, GLOBAL-11). One case is named because it looks like a failure and is treated as absence: a file of the name on `PATH` that is a directory or has no execute permission is not a candidate, as in a shell, so the search goes on and may reach the bundled copy; `warden driver list` shows the file as skipped and why |
| A config file names a driver, never a file: `./warden.toml`, and a file named with `--config` or `WARDEN_CONFIG` alike | **New** rule, the one guard kept (a16), read for one config file (a26) | The defaulted file may come with someone else's build context; a name can only select a driver the user already has; and one rule for every file needs no knowledge of how the file was found | A path comes from `--driver` or `WARDEN_DRIVER` |
| A driver reached through a relative `PATH` entry, or on Unix an empty one, is an error that names the entry; on Windows only `warden-driver-<name>.exe` is a candidate, the current directory is not searched and an empty entry is skipped | **New** (**Discovery and trust**). The first is the rule of Go's `os/exec` (`exec.ErrDot`), kept and not added; the second is what a process `warden` can start and stop requires | A silent skip of a relative entry would run a different file from the one the user's shell finds | An absolute `PATH` entry, or the file named with `--driver`; nothing for `.exe` |
| A context, output or temporary path that is not valid UTF-8, for a driver binary | **New** refusal (Behaviour 15) | It cannot be sent to another process as JSON unchanged, and altering it silently would build something else | Nothing: use a path that is UTF-8 |
| The collector: in `warden`'s process, one per build, reached over the tunnel | Unchanged in effect; now `warden`'s for every driver (a12) | It is today's, and it opens no listener | Nothing in K1; no flag or key is added (**The collector**) |
| Who sends the claim: `warden` | **Changed owner** (a13); the claim is unchanged | The collector's owner must hold the tunnel | Nothing in K1 |
| The contract version: the highest both sides speak | **New** | The newest common behaviour | Nothing; none in common is an error naming both |
| A build may complete for 10 s after its environment exits | **Changed** for container and Hyper-V, from 5 s (Behaviour 7, a20) | One rule in `warden` for every driver; 10 s is the longest any driver uses today | Nothing |
| After `stop`, a driver binary has the teardown wait to tear down, 60 s when `[build] teardown_timeout` is not set; then it is killed and `reap` runs, with the same wait; a `reap` that outlasts it is killed and the build's record and lock stay for `warden clean`. A linked driver has no limit | **New**, and a key by the review's answer (a20, Decision 21) | 60 s is long enough for the slowest teardown known and short enough that Ctrl-C ends | `[build] teardown_timeout`, from `1s` to `1h`, in the config file (no flag and no variable), and the value set is the wait, not a floor or a ceiling on the default. A value that does not parse, is out of range or is not a string is an error at load; 60 s is not used in its place. A second Ctrl-C or SIGTERM ends the wait at once, and a third ends the `reap` |
| SIGTERM stops a build as Ctrl-C does, on every driver | **Changed** for container, vz and hyperv, where it killed `warden` (Behaviour 9) | A cancelled CI job should not leave containers or VMs | Nothing |
| `describe` answers within 10 s, a probe within 30 s | **New** (a20) | `describe` does no host work; a probe makes the driver's preflight, and the slowest today is the container runtime detection (three runtimes at 3 s each) | Nothing; a slower driver is reported as a defect |
| `reap`, `attach` and `warden clean` use the values in the build's record and state file, not the current invocation's | **New** (Decision 18, a20) | They must look where the build put things. The state file is authoritative: a recorded config file that has gone or changed does not fail `reap` or `attach` (C10) | Nothing. `warden clean --driver <path>` chooses the binary that reaps, when the recorded one is gone |
| `warden clean` runs `reap` for every build of the selected driver whose lock nobody holds, and removes the lock only when `reap` succeeds | **New** (a20) | No `warden` holds the lock, so the build is dead for this user. `reap` must be safe beside a driver that is still tearing down by itself | Nothing; a held lock's build is never touched |
| What `clean` treats as live: the held locks (`keep_ids`) and, for the container driver, its process scan as today | **Changed**: the locks are added | Either sign is enough to keep a build | Nothing |
| `--timeout` counts from the start of `run` | **Changed** slightly (Behaviour 14) | One clock for every driver | `--timeout` |
| A driver setting nobody set | Unchanged per setting: the driver applies its own default (`network`: `warden-dev`; `slots`: 4 for hyperv, 16 for firecracker; `nat-prefix`: `192.168.240.0/20`; `host-ip`: `192.168.240.1`; `name`: `windows-server-2025`; `arch`: the host's; the two ages: `winguest`'s; `runtime`: the first of finch, docker, podman that answers `info`, found once per build and kept in its state file). **Changed reader** (a21): the driver resolves it, through the kit, where `cmd/warden` did | These are the values `cmd/warden` and the drivers apply today | The flag, else the setting's environment variable (the two that exist), else the config file: the order `main` has once `project-config-trust` has merged, applied by the kit to the `args` and `config_file` `warden` passes on. In the file a common key is read in the driver's own table and then in `[driver]` (the row on a common key, below). The first place that sets it is the value; it is not merged with a lower one, and a value that does not parse or cannot be used is an error naming where it came from, reported by the driver in its probe (UX-08) |
| Which config file a driver reads: the one `warden` names in `config_file`, and no other | **New** (a21); one file since 2026-11-15 (Decision 23) | `warden` and the driver must read the same file, and a driver that searched by itself, in the working directory or through `WARDEN_CONFIG`, could find another | Nothing for the driver: which file it is, `warden` decides from `--config`, `WARDEN_CONFIG` and the working directory (`project-config-trust`). A driver started with no request, such as a service, reads none and uses its defaults |
| The values the container driver's `runtime` takes: `finch`, `docker` or `podman` | Unchanged by K1: it is `runtime-cli-project-path`'s rule, which K1 moves into the driver and keeps from every source and under both names of the key | They are the runtimes the driver supports and probes for | Nothing: another value is an error, and detection is not tried in its place |
| A driver's flag given with an empty value, or a driver's variable that is set but empty: not set, and the next place is read | Unchanged in effect, **changed reader** (a21): `cmd/warden` tests for the empty string today; the contract now states it for every driver (C8) | It is today's behaviour for every such flag and both variables | Nothing; give a value |
| A driver's switch: on when present | Unchanged in effect: `--force=false` and a repeated switch are settled by `warden`, the last occurrence winning, as the command line does today | A driver reads one form | Nothing |
| A flag that neither `warden` nor the selected driver declares | **Changed** (Behaviour 2): refused by `warden` as it parses. Eleven flags were ignored | A provided value that cannot be used is an error (GLOBAL-10), and `warden` cannot place a flag whose arity it does not know (a24) | Nothing: drop the flag or select the driver that takes it |
| A driver's key whose value has the wrong type or cannot be used | **Changed** (Behaviour 13): refused by the driver that owns the key, in its probe, when that driver is selected. Under another driver it is not read; today a wrong TOML type in the seven existing keys fails the load for every driver | Each key has one reader, and one `warden.toml` serves several drivers | Nothing: fix the value |
| Where a driver's keys live: the common keys in `[driver]`; a driver's overrides of them and its own keys, in any shape, in `[driver.<name>]` | **New rule** of the contract (a4, answered) | A key says whose it is by its table; it cannot collide with `warden`'s tables or another driver's; and a value that several drivers mean the same thing by is written once | Nothing: a driver reads no other table, apart from the seven old names two rows below |
| A common key, for one driver: the value in `[driver]`, when the driver's own table does not set the key | **New** (a4). The two that exist were one pair in `[vm]` for qemu and hyperv alike | What is set for every driver holds for each driver that honours the key, until something says otherwise for that one | `[driver.<name>].<key>`: the driver's own table wins. Absent means the key is not in that table. An empty string is a value, not absence: it hides `[driver]`'s value and, for the two ages, means the driver's default. A value in the driver's own table that cannot be used is an error; `[driver]`'s is not used in its place |
| The seven existing driver-owned keys: each is read under its new name and, until 1.0, under its old one | **Changed** (Behaviour 18; a25 and a26, answered). They were read only in `[runtime]` and `[vm]` | An old name is the same key, so a `warden.toml` nobody edits resolves as today | Nothing overrides an alias: it is a second name, not a lower level (GLOBAL-10). A file that sets a key under both names is refused; the new name is not preferred. Each old name a file sets prints one warning per command |
| `<output>/driver.json` | **New** (a19) | Which driver ran belongs with the build's output | Nothing; always written |
| `--keep-vms` leaves what `capabilities.keep` says | Unchanged per driver | Each driver keeps what it keeps today | Nothing |
| Where the container driver's relay comes from | Unchanged lookups, now behind the relay source | It runs the relay that ships with, or is built from, the same tree | `[driver.container] relay_image` (old name `[runtime] relay_image`) or `WARDEN_IMAGE`: used as given; an image that cannot be pulled is an error, and the default is not tried (a3) |
| A relay file's published hash | **New**, inert in K1: `Relay.Verify` checks a file that carries one. No K1 source sets one | Nothing publishes relay hashes yet | When a source supplies a hash, a mismatch refuses the build; no other source is tried |

Named lookups and fallbacks (GLOBAL-10, GLOBAL-11):

- **New, named: the driver lookup** (the second row): `PATH` in its own
  order, then the directory of `warden`, then a linked driver. It is a
  search order, not a retry. A later place is used only when the earlier
  ones hold no candidate of that name; the first candidate is the
  driver, and its failure is the command's failure. A file of the name
  that cannot be executed is not a candidate and is passed over, which
  is named here because the bundled copy may then run where the user
  meant their own; `warden driver list` shows the order's result, what
  it shadowed and what it skipped, and `driver.json` what ran. A path
  from the user ends the search before it starts.
- **Kept, existing: "beside the binary, else the source tree".** The relay
  VM's kernel and initramfs come all from beside the executable or all
  from the source tree's tool output (`relayvm.Find`), and
  `driver.GuestBinary` looks beside the executable, then in the per-tree
  build cache, then builds from the tree. It is the difference between a
  release install and a checkout, not a retry after a failure. For a
  driver binary "the executable" is the driver, so a bundled driver must
  be installed beside `warden-io` and the relay files, as the archive and
  `install.sh` do. A copy that replaces it from another directory (a16)
  looks beside itself and never beside `warden`.
- **Kept, existing: container runtime autodetection** (finch, docker,
  podman), used only when no runtime is configured. It moves into
  `driver/container`. A build detects once and writes the result to its
  state file; `attach` and `reap` use that, so a later command never
  falls through to another runtime because the build's stopped answering.
- **New, named: `reap` without a state file.** When a build died before
  its driver wrote one, `reap` resolves its settings from the record's
  `args` and `config_file` as they are now, then the driver's default,
  and says which it used. Nothing was created before the state file, so
  there is nothing for it to miss.
- **New, named: a recorded config file that has gone, in `reap` and
  `attach`.** It is read as if none had been named, with a line on
  stderr that names it, so the driver's default may apply where
  the file's value applied to the build. This is a fallback, and it is
  named here: it runs only in those two commands, where the state file
  already holds what the build resolved, and without it a dead build
  whose checkout was deleted could never be reaped. In every other
  command the file is the current invocation's and exists.
- **Dropped: `warden clean` running the container runtime's cleanup for a
  driver that has no `clean`** (Decision 16). It was never named as a
  fallback, and it made `warden clean --driver qemu` fail for want of a
  container runtime.
- **Not a fallback: `[driver]` beneath `[driver.<name>]`, and an old
  name beside its new one.** The two tables are two places in the order
  of precedence, read on absence and never on failure: a value in the
  driver's own table that cannot be used is an error, and the value in
  `[driver]` is not tried. An old name is a second name for one key
  (GLOBAL-10's "a second name for the same value, not a fallback"), and
  a file that uses both is refused.
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
the calling convention, not settings a user sets (UX-07). `ext` started
without `WARDEN_DRIVER_REQUEST` is named no config file and runs with
the driver's defaults. `WARDEN_DRIVER`
gains a path as a value. `WARDEN_CONFIG` is `warden`'s, added by
`project-config-trust`; a driver process inherits it and does not act
on it (C3). `WARDEN_CTR_CLI` and `WARDEN_IMAGE` keep their
meaning and their place in the precedence, between the flag and the
file; the container driver reads them through the kit, where
`LoadConfig` applied them. `WARDEN_RELAY_BOOT_VHDX` and
`WARDEN_HYPERV_CACHE_DIR` stay as they are. The drivers read other
variables directly, and K1 leaves every one as it is, outside the
settings a driver declares: hyperv's `WARDEN_HYPERV_BUILD_IMAGE`, `WARDEN_HYPERV_BUILD_GEN` and
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
and it declares its flags and keys and reads them through the kit
(Decision 7). Its signals are `warden`'s (Decision 19).
What differs:

| Driver | In K1 | Transport after K1 | What changes in its build |
|---|---|---|---|
| container | A binary, `warden-driver-container`, with `describe`, `run` (and its probe), `reap`, `doctor`, `clean`, `attach`; its code leaves `cmd/warden`; its relay goes through the relay source | exec | `Exec` is split into a held `run` and `attach`. It no longer translates the Containerfile itself: the script and `base_image` come from `warden`. `completionGrace` (5 s) goes; it reports `exited` from `container wait`. It writes the runtime it chose to the state file. `clean` takes `keep_ids` beside its process scan. Its teardown must survive closed stdout and stderr |
| qemu | `Driver()` with `Describe`, `Preflight` and `Run` | linked | `linuxguest.ContainerfileImage` is replaced by `start.base_image`, which carries the Containerfile's `FROM` with `--script` too. It never watched its VMs' exit and still does not (`capabilities.exited: false`). Its SIGINT and SIGTERM handler goes: a second Ctrl-C now ends `warden` and leaves its VMs (Behaviour 8) |
| vz | `Driver()` with `Describe`, `Preflight` and `Run`; `New` stops resolving the cache directory | linked | `containerfile: ignore`: `warden` drops the Containerfile before staging, as the driver did. It reports `guest_os: darwin`. It takes the build ID from `start` for its work directory and relay child, where it made none. `relay.log` is still written by the relay child into `diagnostics_dir`. SIGTERM now tears it down |
| hyperv | `Driver()` with `Describe`, `Preflight` and `Run` | linked | `StartRelay` keeps creating and starting the relay VM and gives its `StartSession` and `ClaimRelay` calls to `warden`. `watchBuildVM` reports `exited`; `vmOffGrace` (5 s) goes. Its own `hyperv-build-<id>` lock stays, taken with `warden`'s build ID |
| firecracker | `Driver()` with `Describe`, `Preflight` and `Run` | linked | The relay-exit watch during the claim and `waitBuild`'s rule move to `warden`; the driver reports both VMMs' exits as `exited`. Its slot and build locks and `Pdeathsig` are unchanged. Its SIGINT and SIGTERM handler goes: a second Ctrl-C now ends `warden`, and `Pdeathsig` ends its VMMs |

What a4's answer and a25 change differs by driver too (the settings
table under **Configuration**; Behaviour 18):

| Driver | Keys renamed, old name still read | Common keys it honours |
|---|---|---|
| container | Two: `cli` and `relay_image`, from `[runtime]` to `[driver.container]` | None; it reads nothing in `[driver]` |
| qemu | None of its own | Both, `image_refresh_after` and `image_min_grace`, from `[vm]` to `[driver]`, each with an override in `[driver.qemu]` |
| hyperv | Three: `network`, `windows_image` and `windows_image_update`, from `[vm]` to `[driver.hyperv]` | Both, as qemu, with the override in `[driver.hyperv]` |
| vz, firecracker | None: they read no key | None |

No driver's isolation, executor delivery or start trigger changes, and no
row of `bootstrap.md`'s per-driver mapping changes. BOOT-02's sentence on
the executor's version does (**Bootstrap comparison**). Files on the
bootstrap surface are edited, which tier 1 reports as a hint:
`driver/container/container.go` (the session calls, the split of `Exec`,
the relay source; its isolation, delivery and start code stays in that
file, so the surface list in `bootstrap.md` and `rules.yaml` is not
edited), `driver/linuxguest/` (`ContainerfileImage` goes), and
`driver/firecracker/vm*.go` where the VMM exit channels are handed to
`driver.Host`.

`driver.Session` keeps its methods; its callers change from the five
drivers to `warden`. `driver/hostlock.go` gains the build lock (taken,
checked against its path, and removable), which every driver's build now
goes through. The guest families `driver/linuxguest` and
`driver/winguest` change only by the removal of `ContainerfileImage`.

## Specification changes

Added on 2026-11-15 (LOOP-02, merged at `ad4ea50`; the template gained
the section then). The table restates what the design says elsewhere
and decides nothing; the paragraph on the order in Implement and the
last paragraph are new and marked so. `seams`
declares four names; the umbrella's seam 6 is listed as well, because
the amendments reword its row.

| Seam | Its specification | What this item changes in it | Where |
|---|---|---|---|
| `driver-contract` (the umbrella's seam 9) | None today: the four-method `driver.Driver` in Go defines it. This item writes it: `specs/driver-contract/v1.md`, with numbered requirements, and `v1.schema.json` beside it | The whole document is new: how `warden` finds a driver binary; `describe`; the commands with their requests, results and exit codes; the messages of a build and their order; the build-lock convention; the failure table; the configuration requirements C1 to C11 and the common keys of version 1 (Implementation plan, task 12, has the full list). Its suite is `conformance/driver/` | Here, in this item's PR. Both move to `buildwarden/buildwarden-specs` in K2 (a17); until then no release is pinned |
| `relay-claim` (seam 1) | No written specification. `relaycfg/` on `main` defines it and GLOBAL-08 names it; `seam-specs` (S1, in review) writes it | Nothing in the protocol: no message, field or order. It is declared conservatively, because the claimant becomes `warden`'s session for every driver, at an endpoint the driver reports in `relay-started`, with three claim fields the driver supplies (`claim_values`) whose shape stays the claim protocol's. The umbrella's row for it is reworded to name those parties | No specification to change. What is left for S4 is the transport section of S1's |
| `collector` (seam 2) | No written specification. The package comment in `collector/collector.go` defines it; S1 writes it | Nothing in the protocol. It is declared conservatively, because the collector's owner becomes `warden` for every driver. The driver contract says nothing of the collector's lifetime, which S1 writes per session token | No specification to change |
| Bootstrap config (seam 6; not a name `seams` takes) | No written specification. `bootstrap/` defines it; S1 writes it | Nothing in what is served. Who selects it changes: `warden`'s session, from the `guest_os` the driver reports. The umbrella's row for it is reworded to say so | No specification to change |
| `bootstrap-model` | No specification in LOOP-02's sense. Its contract document is `docs/development/bootstrap.md` (BOOT-07) | BOOT-02's sentence on the executor's version, and two labels of the model diagram (**Bootstrap comparison**; **Rule changes**, BOOT-02) | `docs/development/bootstrap.md`, here, as a rule change the review approves |

**Order in Implement (LOOP-02).** "In Implement the specification
changes first. The code follows and is tested against the changed
specification." The specification and its schema are therefore written
at step 1 of **Order of the work**, before `driver/host.go` and
`cmd/warden/buildseq.go` (step 3), which carry the messages of a build
and are code of the seam in the `seam-contract` entry (**Rule
changes**, GLOBAL-08). The design read at `5808805` had the
specification at step 5, after them. The kit, the suite and `warden`'s
side are each tested against the specification and not against each
other (**Shape**).

One written specification is on the build's path and is not declared:
the environment record (seam 4, `specs/metadata-environment/`). A driver
reports the record it reports today, as `claim_values.environment`. No
change.

*New, for LOOP-03 as merged:* each numbered requirement of `v1.md` is
covered by a suite case that names it, or is listed in the specification
with the reason it is not machine-checked, as the table under
**Configuration** already does for C9 and for part of C11; and each case
names the requirements it checks. The Implementation plan carries it:
task 12 has the specification list what is not machine-checked, tasks
14 and 23 have each case name its requirements, and task 23, where both
tiers exist, adds the test that fails on a requirement no case names and
the specification does not list.

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
| BOOT-01 isolation precedes execution | As written. Each driver starts the relay, claims it, then starts the environment | Unchanged. The order is now the order of the `run` messages: the driver may not start the environment before `claimed` | None. Held as before by each driver's code, which starts the environment only after `host.ClaimRelay` returns. The suite's Sequence case checks the order a driver binary reports; it cannot see when the environment really started |
| BOOT-02 the executor is the only out-of-band input | "It is delivered per build and matches the orchestrator's version." | "It is delivered per build by the driver and matches the version of the driver that delivers it; a driver linked into `warden` has `warden`'s version." | container: the executor is the one beside the `warden-driver-container` that runs. For the bundled copy that is the file shipped in the same archive as `warden`, and the two differ only if a user installs a driver from another release. A file that takes the name from `PATH` or by path (a16) delivers whatever executor it finds beside itself or builds, and `warden` cannot check which; `driver.json` records that it ran. qemu, vz, hyperv, firecracker: `warden`'s version, as today, while the linked driver is what runs; a file that replaces a linked driver (a16) is in the position of a replaced container driver. D2 (vz) is unchanged |
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
  lookup. For the bundled copy (the file beside `warden`, or a linked
  driver) that is the same archive as today. A file that takes a bundled
  name from `PATH` or by path, which the review's answer to a16 allows
  without a flag, is for this purpose a driver from outside. For a
  driver from outside, `warden` cannot check which executor it delivers;
  `driver.json` records which file ran and what it shadowed (**Security
  note**).
- **The record of what the build consumed.** Unchanged: the executor was
  not recorded in the ledger before and is not now. `driver.json` adds the
  driver's version and file digest, unsigned.
- **Three versions can differ** (`warden`, driver, relay with executor).
  The executor and the relay agree through the relay guest API; `warden`
  and the relay through the claim, where a relay that does not know a
  field refuses it by name.

## Rule changes

Eleven entries. The umbrella's table gives K1 GLOBAL-08 and ORCH-01 "for
direction", worded for a Go API; both are reworded here for a command
contract. ORCH-02, ORCH-04, TEST-05 and BOOT-02 are what the
redraft makes true or stale; UX-01 and UX-03 change in enforcement only;
ORCH-03 changes in enforcement and gains one paragraph and a clause
on its last sentence; PROFILE-01, this repository's loop profile, is
amended where K1's PR would leave an entry stale; and UX-08 has the
last paragraph of its approved new wording shortened by the two checks
K1 moves to the drivers (a8). The
list was derived again after the review's answers of 2026-11-14: a16 and
a20 change no rule, and a21 changes the last three entries as said
there. It was derived a third time on 2026-11-15, after `main` at
`ad4ea50` (`loop-contract`, #119) was merged into the branch: FLOW-02
left the list and PROFILE-01 joined it, each under its own heading
below. It was read a fourth time the same day, for the decision that
`warden` reads one config file (Decision 23): the same ten, with
ORCH-03's new paragraph shorter by its words about files, and with the
rule's opening sentences left to `project-config-trust`. The eleventh,
UX-08, was added that evening, when `runtime-cli-project-path`'s review
approved the wording K1's entry edits (under its own heading, last
below). The eleven were approved by name in the design review
(FLOW-19) on 2026-11-15, on the wording at `3f2012a`, pending the
review's outcome (a8, answered; **Design review**). Each is made in
Implement (FLOW-14). Each is worded so that it is
true when K1 merges, with four drivers still linked (umbrella Decision 18).

**Checked against FLOW-14 and LOOP-09 as merged.** A feature's design
may declare a change "to the invariants and profile of a component it
works in and to their enforcement, and never to a loop rule (LOOP-\*,
FLOW-\*, LINT-\*) or its enforcement (LOOP-09)". Every entry was read
again for that. GLOBAL-08, ORCH-01 to ORCH-04, TEST-05, BOOT-02, UX-01,
UX-03, UX-08 and PROFILE-01 are this repository's own (the Owner column of the
ID scheme in `docs/development/README.md`), and their paths are their
own rule files, `tools/tier1/rules.yaml` and `tools/tier1/surface/`.
FLOW-02 was the one loop rule on the list, and it is withdrawn (below).
One path serves a loop rule as well as a project rule: `rules.yaml` maps
the `drivers-claim` check to FLOW-02 and to ORCH-01, and the ORCH-01
entry adds eight paths to its `drivers.shared` list. That list is this
repository's data and not the loop's: PROFILE-01's Impact vocabulary
entry names "the shared packages that declare every dependent driver"
as the `drivers` list of `rules.yaml`, under ORCH-01. The check's code is
not edited. The surface check serves UX-01, UX-03 and ORCH-03 only, and
`seam-contract` GLOBAL-08 and EXEC-04 only. The current wording quoted
under each entry was compared again with `main` at `ad4ea50`; the merge
changed none of those files, and every quotation stands.

### GLOBAL-08 (amend), `docs/development/global.md`, `tools/tier1/rules.yaml`

**Why:** GLOBAL-08 says today that `driver.Driver` is "not covered by this
obligation". After K1 a driver is a program run through a written command
contract with a conformance suite, and an unmodified `warden` runs one it
was not built with. So the sentence a10 held back in the first draft, that
a driver the project did not write is a supported consumer, is true of
this contract in K1. It is not yet true of everything such a driver
needs: launching the relay, delivering the executor and the build-link
guarantee are other seams, most not written (**What a driver needs
besides this contract**). The wording says both, and a23 asks whether to
hold the sentence back instead. The rule also says that the orchestrator "treats the relay and executor as
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
`docs/development/README.md` describes the orchestrator as `cmd/warden/`:
"the host CLI, driver dispatch, config, inspect, and image lifecycle".
That stays true after K1 and the README is not edited.

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
>   project did not write is a supported consumer of this contract. What such
>   a driver must do to the relay and the executor is theirs to state, in the
>   two seams above, and not this contract's. The Go packages under `driver/`
>   are a kit for writing a driver and are not part of the seam.
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
driver-contract, code: [driver/drivercmd/, driver/host.go,
cmd/warden/drivercall.go, cmd/warden/driverlookup.go,
cmd/warden/buildseq.go, cmd/warden/buildrecord.go], contract:
specs/driver-contract/v1.md}`. `driver/host.go` defines the message types
and `buildseq.go` holds what `warden` does with each message, so a change
to the kit's or `warden`'s side of the wire, or to what a message means,
without the specification then fails tier 1. The suite is not listed: a check added to
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
`driver/relaysource/`; `driver/buildlink/` and `driver/resolvers/`, which
every VM driver already depends on and the list omits today; and
`cmd/warden/buildseq.go`, `cmd/warden/buildrecord.go`,
`cmd/warden/linked.go` and `cmd/warden/drivercall.go`. The last four
follow the lifecycle to where the rule now says it lives: the claim
order, the 10 s grace and the teardown wait are in `driver/session.go` and
the drivers today, where a change is caught, and after K1 they are in
`cmd/warden`, where without these entries a change declared for one
driver would pass. A design that changes any of them must then declare
every driver. The suite
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
rule should hold a future driver to the answers. It also says what the
redraft first got wrong: a build's leftovers are not found from its ID
alone, so `warden` keeps a record (Decision 18).

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
> comes first, and finishes that teardown even when it can no longer write
> its output, so a killed `warden` alone leaves nothing running. What a
> killed driver leaves is reachable from the build ID and the record `warden`
> kept for that build: `reap <build-id>` removes it, reports success only
> when nothing is left, and is run by `warden` after a driver that died and
> by `warden clean` for every build no running `warden` holds. What `warden`
> itself staged for a build is removed the same way.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change; the suite's Stop, End of input, Orphaned and Kill
cases exercise it on a driver binary.

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

### FLOW-02: no entry; its wording is handed to a process item

The design Jeff read at `5808805` declared an amendment to FLOW-02, whose
two bullets name `driver.Driver`. FLOW-02 is a loop rule. Since `ad4ea50`
FLOW-14 lets a feature's design declare no change to one, and LOOP-09
says: "No component signs off a change, and no feature's design declares
one. A component that needs a loop rule changed asks for a process item."
The entry is withdrawn. K1's PR edits nothing in
`docs/development/process.md`.

**What K1 leaves untrue, and for how long.** K1 removes the four-method
`driver.Driver`. From K1's merge, two bullets of FLOW-02 name a Go
interface that no longer exists (current wording on `main` at `ad4ea50`;
the rest of the rule does not name it):

> - `drivers` — which drivers are touched, or `interface` for a change made at the
>   `driver.Driver` / `driver.Session` level (ORCH-01).

> - `alters_relationships` — `true` when the change alters a component's role or
>   how components interact: a versioned seam (GLOBAL-08), the bootstrap model
>   (BOOT rules), which component owns a responsibility (relay, collector,
>   executor, orchestrator, a driver), or the `driver.Driver` / `driver.Session`
>   contract. Changes inside one component's existing role are `false`. FLOW-05
>   limits how many such features are in flight.

No check reads those words: tier 1 reads the value `interface`, not the
sentence. What they mean is not lost either. The first bullet cites
ORCH-01, which K1 amends to say that a lifecycle change is expressed in
the driver contract. A change to the driver contract alters
relationships under the second bullet's first case, a versioned seam,
which K1 makes the driver contract (GLOBAL-08). `driver.Session` still
exists. And LOOP-08 says of a name the loop's detail takes from this
repository that "LOOP-01's reading rule applies, and the wording is made
neutral when the rule is next changed": a design here reads the two
bullets through this repository's Impact vocabulary entry, to which the
PROFILE-01 change below adds what `interface` stands for. The two bullets
stay as quoted until a process item changes them: from K1's merge to
that item's merge if K1 merges first, and not at all if that item
merges first.

**What is asked, and of which item (a8).** K1 asks `loop-parallel` to
carry the rewording. It is P1's third item, a process item, and its
`rule_changes` already amend FLOW-02 (at `6d432f9`: two keys added to
the list and the two paragraphs after it reworded, the two bullets above
untouched; its design does not name `driver.Driver`). Its design review
began on 2026-11-15, and Jeff settles the two bullets in that review,
so the request was put to him in that sitting, ahead of a8's answer
here. He answered it there on 2026-11-15: "Yes, carry it here". The two
bullets are therefore in `loop-parallel`'s FLOW-02 amendment (folded
into its design at `07c2993`); that design was not approved when this
was written. a8 was then answered (i) here. This design does not edit
that design or its queue file.

One case is left in which another item carries them: `loop-parallel`'s
review ends without approving its FLOW-02 amendment with the two
bullets in it. They then go to `loop-native-lead`, P1's fourth process
item, queued and with no design yet (its scope today is FLOW-04, -09,
-13, -16 and -18, and FLOW-02 would be added to it), or to a process
item of their own, as Jeff chooses then. What K1 leaves untrue then
lasts until that item merges. The case the design read at `3f2012a`
provided for, that review approved before the request reached it, did
not arise.

What K1 would have written, as a starting point for whichever item
carries it:

> - `drivers` — which drivers are touched, or `interface` for a change made at the
>   level of the driver contract or `driver.Session` (ORCH-01).

> - `alters_relationships` — `true` when the change alters a component's role or
>   how components interact: a versioned seam (GLOBAL-08), the bootstrap model
>   (BOOT rules), which component owns a responsibility (relay, collector,
>   executor, orchestrator, a driver), or the driver contract or
>   `driver.Session`. Changes inside one component's existing role are `false`.
>   FLOW-05 limits how many such features are in flight.

That wording is true before K1 merges as well as after: the umbrella
names seam 9 "Driver contract" and gives the four-method `driver.Driver`
as its form today. So the order of the two merges does not matter.
Whether it is neutral enough for LOOP-08 is for that item's review.

Considered, put under a8, and rejected there (a8 was answered (i)): a
process item of its own, filed now, in
place of `loop-parallel` (one more item and one more review for two
bullets); no sentence in PROFILE-01 (ORCH-01 as amended and the
versioned-seam case then carry the meaning, and once FLOW-02 is made
neutral nothing in this repository's rules says what `interface` stands
for); K1 waiting for the process item to merge (nothing depends on the
words, so the wait buys nothing).

### UX-01, UX-03 and ORCH-03 (amend), `tools/tier1/surface/`, `docs/development/orchestrator.md`

Derived again after a21's answer. The redraft changed these three in
enforcement only. That still holds for UX-01 and UX-03. ORCH-03 now also
gains a paragraph, because its subject, who reads configuration and with
what precedence, is what the answer changed.

**Why:** tier 1 reads the user-facing surface from the cobra flag calls and
`toml` struct tags in `cmd/warden/` (`surface.dir`). After K1 a driver's
flags and keys are not in `cmd/warden` at all: each bundled driver
declares them as `drivercmd.Setting` literals in its own package, and
reads them there (the seven driver-owned `toml` fields leave `Config`).
Unchanged, the check would report every moved flag and all seven keys as
removed (UX-03) and would stop comparing the drivers' flags and keys with
the reference docs (UX-01, ORCH-03). After a4 the seven keys are also
renamed into `[driver]` and `[driver.<name>]`, with their old names kept
as deprecated aliases; that is UX-03's own route and changes no word of
it. One file, `surface.go`, enforces all
three rules, so the edit is a change to each (FLOW-14) and each has an
entry.

ORCH-03's sentence "Configuration precedence stays `flag > warden.toml >
~/.config/warden`" has had one implementation, in `cmd/warden`. It now
has two, `warden`'s for its own keys and the kit's for a driver's, and a
driver written without the kit has a third. The rule should say that all
of them keep it, so that a later change to the kit, or a later driver, is
judged against the rule and not only against the contract. That
sentence is itself being amended, by `project-config-trust`, for one
config file. K1's paragraph is worded so that it reads the same against
the sentence as it is and as it will be.

**Wording:** unchanged for UX-01 and UX-03. UX-01's "The automated part
is a surface diff against a docs diff" and UX-03's "The automated part
flags a removed or renamed surface with no alias or deprecation" stay
word for word. UX-07 is not changed by K1 either: it is being amended
by `project-config-trust`. UX-08 is amended by
`runtime-cli-project-path`, approved at `a2afd08`, and the design keeps
both rules in either form (**Configuration** quotes them): a driver's
probe is "before any real work begins", and it is the component that
uses a setting checking it, when the run uses it. Of UX-08 K1 changes
one paragraph, the list of older checks that ends that item's wording;
that is an entry of its own, **UX-08**, the last heading of this
section.

**ORCH-03, current wording** (the rule's body):

> Configuration precedence stays `flag > warden.toml > ~/.config/warden`. Defaults
> are sane and changes are additive. A new or changed config key is documented in the
> same change; this is the config half of the docs-coupling rule (UX-01).
>
> The automated part is a config-key diff against a docs diff.

**ORCH-03, proposed wording:**

> Configuration precedence stays `flag > warden.toml > ~/.config/warden`. Defaults
> are sane and changes are additive. A new or changed config key is documented in the
> same change; this is the config half of the docs-coupling rule (UX-01).
>
> `warden` reads its own flags and keys. A driver reads its own, from the config
> file `warden` names to it, with the same precedence and before it does any
> work (UX-08): through the kit (`driver/drivercmd`), or as the driver contract
> requires of a driver written without it (GLOBAL-08). A driver run as a
> separate binary does so in its own process. The keys the driver contract
> names as common to drivers live in `[driver]`; a driver's overrides of
> those, and its own keys, live in `[driver.<name>]`, which is read before
> `[driver]`.
>
> The automated part is a config-key diff against a docs diff, over `cmd/warden`
> and the drivers this repository builds.

Two things change in the wording, then: the new second paragraph, and
the clause "over `cmd/warden` and the drivers this repository builds" at
the end of the last sentence, which says where the automated part now
looks. The paragraph does not say "in its own process" of every driver,
because four are linked into `warden` when K1 merges and read their
configuration through the kit in `warden`'s process. "Before it does any
work" is about the commands that do work; in `reap` and `attach` a
driver works from its state file (C10). The paragraph's last sentence
states a4's answer and the order between the two tables. It says nothing
of the order between a flag, a variable and the file, which is the first
sentence's, and nothing of the seven old names, which are UX-03's matter
and a25's and a26's (both answered; neither changes these words).

*Changed on 2026-11-15.* As Jeff read it at `5808805` the paragraph said
"from the config files `warden` names to it" and ended "Within one file
the driver's own table is read before `[driver]`; the order of the files
does not change." There is one file now (Decision 23), so it says "the
config file", and its last two sentences are one.

**With `project-config-trust`.** That item amends the same rule, for one
config file (its design at `539fd38`, not approved). The two change
different sentences:

| Part of ORCH-03 | This design owns | `project-config-trust` owns | Neither changes |
|---|---|---|---|
| First paragraph | Nothing | Two new sentences at its head, on the one file and how it is found; and the sentence "Configuration precedence stays `flag > warden.toml > ~/.config/warden`.", which becomes "`flag > environment variable > the config file > default`" | "Defaults are sane and changes are additive. A new or changed config key is documented in the same change; this is the config half of the docs-coupling rule (UX-01)." |
| The new second paragraph, on who reads a driver's flags and keys | All of it | Nothing | |
| Last sentence | The clause "over `cmd/warden` and the drivers this repository builds" | Nothing | "The automated part is a config-key diff against a docs diff" |

The first paragraph in the proposed wording above is `main`'s at
`ad4ea50`. It is there only to show where K1's paragraph goes; K1
proposes no word of it. K1 is the second of the two to merge (**What K1
takes from the two configuration items**). It adds its paragraph and its
clause to the rule as it finds it, and does not bring the old first
sentence back. With both changes the rule reads, as that item's design
gives its part:

> `warden` reads one config file: the one `--config` names, else the one
> `WARDEN_CONFIG` names, else `warden.toml` in the working directory when it
> exists. It reads no other file and merges none; a named file that cannot be
> read is an error. Configuration precedence stays `flag > environment variable >
> the config file > default`. Defaults are sane and changes are additive. A new
> or changed config key is documented in the same change; this is the config half
> of the docs-coupling rule (UX-01).
>
> `warden` reads its own flags and keys. A driver reads its own, from the config
> file `warden` names to it, with the same precedence and before it does any
> work (UX-08): through the kit (`driver/drivercmd`), or as the driver contract
> requires of a driver written without it (GLOBAL-08). A driver run as a
> separate binary does so in its own process. The keys the driver contract
> names as common to drivers live in `[driver]`; a driver's overrides of
> those, and its own keys, live in `[driver.<name>]`, which is read before
> `[driver]`.
>
> The automated part is a config-key diff against a docs diff, over `cmd/warden`
> and the drivers this repository builds.

The review of this design is asked to approve K1's paragraph and clause
in both settings (a8): after `main`'s first paragraph, as proposed
above, and after that item's, as here. K1's words are the same in both;
"the same precedence" means the first paragraph's, whichever it is. The
joined rule is quoted from that item's design at `539fd38`; if its
review changes its sentences, this quotation is read again against the
outcome (FLOW-10), and a change that K1's paragraph would contradict or
repeat is a finding for Design (FLOW-06). UX-07 is that item's alone.

**Severity and check:** MUST, `automated + agent-judgment`, before and after,
for all three.

**Enforcement change:**

- `tools/tier1/rules.yaml` is not edited for this change: `surface.dir:
  cmd/warden/` and `surface.docs` stay as they are. FLOW-14's check builds
  tier 1 from the merge base's code with the branch's `rules.yaml`, and that
  code rejects a key it does not know, so renaming `surface.dir` or adding a
  key would make `enforcement-check.sh` exit 2 on every run of the PR.
- `tools/tier1/surface/surface.go`: besides flag calls and `toml` tags in
  `surface.dir`, `Check` reads the settings the bundled drivers declare.
  It loads, at the base and at HEAD, the non-test Go files of
  `surface.dir`, of `driver/` and of each directory directly under
  `driver/` (found by listing `driver/` at that revision; the root is a
  constant in `surface.go`). In them `Extract` reads composite literals
  of `drivercmd.Setting` (`Setting` inside package `drivercmd`),
  including the elements of a `[]drivercmd.Setting` literal, whose type
  is elided: a literal with a non-empty `Flag` yields that flag; one
  with `Key: "driver.name.key"` yields key `key`, the last element of
  the path, which is the identity a `toml` tag gives today; one with
  `Common: "key"` yields key `key`; and each string in `Aliases` yields
  the last element of its path. Nothing else is read from the driver
  directories. The check runs
  when the diff touches `surface.dir` or `driver/`. Tests and a fixture
  (`tools/tier1/surface/testdata/driver.go.txt`) cover the new form.
- What it can and cannot check after K1. For a bundled driver, a flag or
  key added or changed in its declaration is checked against the docs
  exactly as a `cmd/warden` flag is, and one removed with no alias is a
  finding. It cannot see a driver from outside, which documents its own
  surface; it cannot see a setting a driver reads around the kit, as it
  cannot see today the variables the drivers read directly; and of a key
  of any shape it sees the path, not what lies beneath it.
- Effect on findings. The branch's tier 1 extracts the same set of flags
  and keys at the base (all in `cmd/warden`) and at HEAD (split between
  `cmd/warden` and the drivers' `Setting` literals), plus `driver list`'s
  `--json` and `teardown_timeout`, so it reports nothing but the
  additions, which the docs in the diff cover. The seven renamed keys
  are in neither list: a key's identity is the last element of its
  path, which the rename keeps (`runtime.cli` and `driver.container.cli`
  are both `cli`), so the automated part sees no change in them and
  cannot tell a kept old name from a dropped one. The aliases, their
  warning and the window are held by the kit's tests and by review, the
  agent-judgment half of UX-03. `main`'s tier 1 reads the
  branch's `rules.yaml`, which it can parse, and sees `cmd/warden` only.
  It reports `surface-removed` (UX-03) for each moved flag and each of
  the seven moved keys whose name no longer appears as a string literal
  in `cmd/warden`; the check takes a surviving literal for an alias, so a
  common word such as `name`, `force` or `network` may go unreported. It
  reports no `surface-docs` finding (UX-01, ORCH-03), because `cli.md` and
  `configuration.md` are in the diff and that check passes whenever a
  reference doc is touched. So the findings the branch drops are all on
  UX-03, and that entry clears them; the UX-01 and ORCH-03 entries clear
  the edit to the file, not a finding, and ORCH-03's also carries its
  new paragraph.

The GLOBAL-08, ORCH-01 and TEST-05 entries edit `rules.yaml` within its
present format (`seams` gains an entry, `drivers.shared` eight paths, one
owner string changes), which `main`'s tier 1 reads without change.

### PROFILE-01 (amend), `docs/development/loop-profile.md`

New on 2026-11-15: the rule did not exist when Jeff read the design at
`5808805`. It merged with `loop-contract` at `ad4ea50`.

**Why:** PROFILE-01 is this repository's profile under the Loop Contract,
and says: "An entry that no longer matches the repository is a finding,
and the change that makes it stale changes the entry." `loop-contract`'s
design gives the test: a fact an entry states needs a change to the
entry, and a fact in a file the entry points to does not. K1's PR against
that test, for the entries it could reach:

| Entry | Stale after K1? | Why |
|---|---|---|
| Specifications | Yes | It says what `specs/` holds. K1 adds `specs/driver-contract/`, and a conformance suite that is outside `specs/` (**The conformance suite**, Home). "No specifications release is pinned today" stays true of K1: both stay in this repository until K2. `seam-specs` replaces that sentence (below) |
| Checks | Yes, one row | K1 adds a CI job, `driver-contract`, that blocks (it joins `ci-ok`'s `needs`) and is none of the jobs the table accounts for; LOOP-03 has a check that a change adds carry a class and a tier, and the tier-1 Safety row gains the job. The suite needs no new words. LOOP-03 puts a conformance suite in the Specification class, whose tier-1 row names "the contract tests of TEST-05", and the TEST-05 change above names the suite there: the row points to the fact and stays true as it is. The suite's static tier runs in `make test`, and its lifecycle tier in the `integration` workflow and in native validation, all of which the Safety rows already name |
| Reference documents | Yes | It says the surface is the `surface` entry of `rules.yaml`. After K1 the surface is also the flags and keys the drivers under `driver/` declare, which that entry does not name: its `dir` stays `cmd/warden/` and the root `driver/` is a constant in `surface.go`, for the reason given under UX-01 above |
| Impact vocabulary | No | It points to the `drivers` and `seams` lists of `rules.yaml`, which the GLOBAL-08 and ORCH-01 entries edit (one seam name, eight shared paths), and to `TEMPLATE.md`, which K1 does not edit. It gains a sentence for another reason: FLOW-02, above |
| Sweep | No | K1 keeps no unreachable function: a kit function that no driver or test calls is removed (**The overview**). The kept list stands until K2 gives the kit consumers outside this repository. `make deadcode` and the overview's path do not change |
| Consumables | No | "Releases: the `v*` tags" stays true with one more binary in the archive. "Consumes: nothing" stays true of K1: the specification is in this repository and no release of it exists. `seam-specs` rewrites the entry for the release it pins |

The other thirteen entries name nothing K1 changes: the component is
still "the kit, the orchestrator, the collector, the executor, the
drivers"; the fast gate is still `make lint` and `make test` (`make
driver-conformance` is not run on every change); the release is one more
GoReleaser build in the same workflow; no host name, rule path or session
setting changes.

**Current wording** (three rows of the profile's table and one of its
Checks table; every other row and the rule's own paragraphs are
unchanged):

> | Impact vocabulary | The keys and values in `docs/design/TEMPLATE.md`'s front-matter. The driver names, the shared packages that declare every dependent driver, and the seam names are the `drivers` and `seams` lists of `tools/tier1/rules.yaml` (ORCH-01, GLOBAL-08). The `security_boundary` values are GLOBAL-06's. |

> | Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05), mirrored in the book. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. No specifications release is pinned today. |

> | Reference documents | The surface and its documents are the `surface` entry of `tools/tier1/rules.yaml` (UX-01, ORCH-03). Guides: `docs/book/src/guide/`, `docs/book/src/getting-started/`, `examples/`, `README.md`. |

> | Safety | `make lint` (`golangci-lint`, and tier 1's analyzers as `rules.yaml` maps them) and `make test`; the `integration` workflow (TEST-02's tier 1) | 1 |

**Proposed wording** (the same four rows):

> | Impact vocabulary | The keys and values in `docs/design/TEMPLATE.md`'s front-matter. The driver names, the shared packages that declare every dependent driver, and the seam names are the `drivers` and `seams` lists of `tools/tier1/rules.yaml` (ORCH-01, GLOBAL-08). In `drivers`, `interface` is a change made at the level of the driver contract or `driver.Session` (ORCH-01), and a change to either alters relationships. The `security_boundary` values are GLOBAL-06's. |

> | Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05) and the driver contract (`specs/driver-contract/`, GLOBAL-08), mirrored in the book. The driver contract's conformance suite is `conformance/driver/`. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. No specifications release is pinned today. |

> | Reference documents | The surface is the flags and keys in the directory that the `surface` entry of `tools/tier1/rules.yaml` names and those the drivers under `driver/` declare; its documents are that entry's `docs` (UX-01, ORCH-03). Guides: `docs/book/src/guide/`, `docs/book/src/getting-started/`, `examples/`, `README.md`. |

> | Safety | `make lint` (`golangci-lint`, and tier 1's analyzers as `rules.yaml` maps them) and `make test`; the `driver-contract` job of `.github/workflows/main.yml`, which runs the tests of the packages it lists on Linux, macOS and Windows; the `integration` workflow (TEST-02's tier 1) | 1 |

What changes, row by row. Impact vocabulary gains one sentence, "In
`drivers`, `interface` is ... alters relationships." Specifications
gains "and the driver contract (`specs/driver-contract/`, GLOBAL-08)"
and the sentence on the suite. Reference documents has its first
sentence rewritten. The tier-1 Safety row of Checks gains the
`driver-contract` job. Each is true when K1 merges. K2 changes
Specifications and Consumables again when the specification moves to the
specs repository, and the Sweep entry if the kit's exported API is then
kept; those are K2's to declare.

The tier-1 Specification row of Checks is not changed. The design at
`bb4d7c8` added ", the driver conformance suite (`conformance/driver/`)
among them" after "the contract tests of TEST-05". By the test above the
row was not stale, since it points to TEST-05 and TEST-05 names the
suite, so those words are withdrawn and the entry has four rows where it
had five.

**Severity and check:** MUST, `agent-judgment`, before and after. No
enforcement change: tier 1 has no check for PROFILE-01, and
`rules.yaml` gets no entry for it.

**With `seam-specs` and `loop-parallel`.** Two other items amend the
same rule, and neither is through its review.

`seam-specs` (S1, read at `e4801d6`) does not owe a PROFILE-01 change,
as this section said at `bb4d7c8`: it declares one, to three rows, the
Specifications entry, the Consumables entry and the tier-1 Specification
row of Checks. It was first committed (`1023b02`) half a minute before
this design's (`a280ba9`), so neither was written with the other in
front of it. Its section **With `driver-contract`** was then written
against this design at `bb4d7c8`, and what follows against that section.
One row is changed by both, the Specifications entry, and the two
designs change different sentences of it:

| Row | This design owns | `seam-specs` owns | Neither changes |
|---|---|---|---|
| Specifications | In its first sentence, "and the driver contract (`specs/driver-contract/`, GLOBAL-08)" after "(RELAY-05)"; and the new second sentence, "The driver contract's conformance suite is `conformance/driver/`." | The three sentences that replace its last one, "No specifications release is pinned today." | The sentence that begins "For the seams of GLOBAL-08" |
| Consumables; the tier-1 Specification row of Checks | Nothing | All of its change to each | |
| Impact vocabulary; Reference documents; the tier-1 Safety row of Checks | All of the change to each | Nothing | |

The proposed Specifications row above ends with "No specifications
release is pinned today." That sentence is S1's to replace, and is in
K1's wording only as the row reads if K1 merges first. If S1 merges
first, K1 adds its clause and its sentence to the row it finds and does
not bring the sentence back. With both changes the row reads, in either
order, as S1's design also gives it:

> | Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05) and the driver contract (`specs/driver-contract/`, GLOBAL-08), mirrored in the book. The driver contract's conformance suite is `conformance/driver/`. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. The claim protocol, the collector protocol, the relay guest API, the `initialize` contract and the bootstrap config are specified in `buildwarden/buildwarden-specs`. This repository implements them and pins one release of that repository in `tools/conformance/go.mod`, which is their contract in that list (GLOBAL-08). The pin is a release tag on `main` and on a pull request to it; a branch may pin a commit of an unmerged specifications branch while its work is in progress. |

The review of this design is asked to approve the Specifications row in
both forms (a8): as proposed above, which is what it says if K1 merges
first, and joined as here. The second to merge then finds a row its
review approved, and does not return to Design for it. The joined row is
quoted from S1's design at `e4801d6`; if S1's review changes its three
sentences, this quotation is read again against the outcome.

S1's section also gives the tier-1 Specification row of Checks joined
with the clause this design proposed at `bb4d7c8`. With that clause
withdrawn, the row with both changes is S1's proposed row alone, and its
words "the contract tests of TEST-05 that run in `make test`" take in
the driver suite's static tier. S1's design is not edited from here;
this is pointed out to its review.

`loop-parallel` (in review, read at `6d432f9`) adds an Areas row to the
profile's table and changes nothing else in the rule, where its draft at
`f706bf7` also changed the header's check line, the paragraph on the
agent part and the Process row of Checks. No row changed here is one it
touches.

Whichever lands second merges `main` and keeps both. Where a quotation
of current wording above no longer matches `main` because another item's
approved words landed first, Consolidation refreshes it (FLOW-10).

### UX-08 (amend), `docs/development/ux-cli-config.md`

New on 2026-11-15, after the fold-in of the one-file decision. The
wording this entry edits is not K1's and is not on `main` yet: it is
`runtime-cli-project-path`'s amendment of UX-08, approved by name in that
item's design review at `a2afd08` (its branch at `119a0e1`). That item is
in Plan and merges before K1's Plan starts (**What K1 takes from the two
configuration items**).

**Why:** the approved wording ends with a list of the checks on `main`
that are made more widely than the rule now allows, and gives the list a
rule of its own: "Each stays until the change that moves it to its
consumer, which takes it off this list." K1 is that change for two of
the four. That item's design hands the edit to K1 (its **What K1 has to
keep**, point 7: "That is an edit of UX-08 that K1 has to declare").
Without an entry here K1's merge would leave UX-08 naming two checks
that `warden` no longer makes, and Implement could not correct it,
because FLOW-14 clears an edit to a rule file only through an approved
entry whose paths name the file.

| Check on that list | After K1 | On the list after K1 |
|---|---|---|
| As a config file loads: the TOML type of every key | `warden` decodes only its own keys. The seven driver-owned fields leave `Config`, and the type of each of those keys is checked by the driver that owns it, in its probe and only when it is selected (Behaviour 13; C6) | Yes, narrowed to `warden`'s own keys |
| As a config file loads: `[vm] build_link` | Unchanged: the key stays `warden`'s (Decision 8; a9, answered) | Yes |
| As a config file loads: `[vm] dns` | Unchanged, likewise | Yes |
| On every `build` and `shell`: `[vm] image_refresh_after` and `image_min_grace` | The two ages are common keys read by qemu and hyperv, which check them in their probe and only when selected, under their new names and their old ones (Behaviour 13) | No |

**Current wording.** On `main` at `ad4ea50` UX-08 is one paragraph and
has no list ("Configuration errors or mismatches are validated and
surfaced as early as possible, at load/parse/startup, before any real
work begins, rather than failing deep into a build. The automated part
checks that config validation runs at startup."). The paragraph K1
edits is the last of the wording approved at `a2afd08`:

> Four checks predate this wording and are made whatever the run uses. As a
> config file loads: the TOML type of every key, `[vm] build_link`, and
> `[vm] dns`. On every `build` and `shell`: `[vm] image_refresh_after` and
> `image_min_grace`. Each stays until the change that moves it to its consumer,
> which takes it off this list. Nothing is added to the list.

**Proposed wording** (that paragraph):

> Three checks predate this wording and are made whatever the run uses, as a
> config file loads: the TOML type of `warden`'s own keys, `[vm] build_link`, and
> `[vm] dns`. Each stays until the change that moves it to its consumer, which
> takes it off this list. Nothing is added to the list.

Nothing else of UX-08 changes. Its title, its first three paragraphs
(the consumer validates when it initializes, after the overrides; only
what a run uses is validated; required of the project's components and
recommended for others') and its check type are that item's and stand
word for word. The removal of UX-08's entry from `tools/tier1/rules.yaml`
is that item's too, so this entry has one path.

**Severity and check:** MUST, `agent-judgment`, before and after, as that
item leaves them. No enforcement change: no check reads the list, and
tier 2 judges the rule from its paragraphs.

**The order, and what K1 does if it is not kept.** K1's Implement finds
the paragraph on `main`, because that item has merged by then. If UX-08
on `main` does not then end with the paragraph quoted as current (that
item withdrawn, or its wording changed before it merged), K1 makes no
edit to UX-08 and returns to Design for this entry (FLOW-06); it does
not write the list into a rule that has none.

**Rejected:** a5's option (d), under which `warden` would have kept the
seven fields in `Config` for their type check and the paragraph would
have kept "the TOML type of every key". a5 was answered (a).

**K2.** If a9 is opened again there and the VM drivers come to read
`[vm] build_link` and `[vm] dns` themselves, K2 takes those two off the
list in the same way.

## Amendments to the umbrella design

[`componentization`](componentization.md) is merged and archived, and its
Decisions 1 to 17 are Jeff's. The review's basis changes some of its
sentences. Each is listed with its current and proposed wording, to be
made in this item's PR once the design review approves them (a18). Nothing
here touches the freeze, the program's scope, the track structure or any
other decision; those are FLOW-20's and the umbrella's.

A dated entry is added to the umbrella's **Amendments** section, after
`loop-contract`'s and in its form: "**<date>, by `driver-contract`
(K1)**, approved in its design review. Decisions 4 and 5, seams 1, 6 and 9
and the K1, K2 and S4 rows were reworded, and the sentences that follow
from them. The wording each had is in
[`driver-contract`](driver-contract.md), under **Amendments to the
umbrella design**." The date is
filled in when the review approves; the sitting of 2026-11-14 sent the
design back and approved nothing. Line numbers are the umbrella's on
`main` at `ad4ea50`.

`loop-contract` amended the umbrella in place (approved on 2026-11-14,
merged on 2026-11-15) and gave it that **Amendments** section, where the
design read at `5808805` put the note under the Decisions heading. Every
sentence quoted as current below was compared again with the umbrella at
`ad4ea50`: `loop-contract` changed none of them, so each quotation
stands and only the line numbers moved (by 4 to 23 lines; the front
matter's not at all). Its amendments are to other sentences (Problem
point 3, Decision 2, a5, seam 10, the P1 and M6 rows, the heading of
Track R, the wave-1 gate, **How program items run**, the FLOW and LINT
row of the rule table, three sentences of the Archive summary), and
none of the proposals below meets one. One difference in form is Jeff's
to see under a18: `loop-contract` kept the approved sentence of Decision
2 and of a5 and added the amendment after it, while the proposals below
replace the sentences of Decisions 4 and 5, with the wording they had
kept here.

**A. The decisions and seam 9.**

| Where | Current | Proposed |
|---|---|---|
| Decision 4 (120) | "**Drivers become separate binaries on a public Go kit, then separate modules, then separate repositories.** Alternatives: only complete the in-tree contract (does not let an integrator ship a driver); drivers as RPC plugins (a second protocol to version, and no gain over a binary that links the kit). Hyper-V is extracted first as the pilot." | "**Drivers become separate binaries that `warden` runs through a written command contract, then separate modules, then separate repositories.** A public Go kit implements the contract for a driver that wants it; a driver may be written without it, in any language. Alternatives: only complete the in-tree contract (does not let an integrator ship a driver); a Go API that every driver links (an existing `warden` could not run a new driver without being rebuilt); drivers as long-lived RPC plugin processes behind a plugin framework (commands, JSON and the standard streams are enough). The command contract is a second protocol to version, and that cost is accepted. Hyper-V is extracted first as the pilot." |
| Decision 5 (125) | "**A top-level `warden` dispatcher that works like git's CLI.** It finds a driver binary by name and runs it. Alternative: users call driver binaries directly; it lost because driver auto-detection and one entry point are worth keeping." | "**`warden` finds a driver binary the way git finds a subcommand, and stays the orchestrator.** It keeps the session, the collector and the reporting, and calls the driver's commands for primitives: start the relay, create the environment, tear both down. Alternatives: users call driver binaries directly (one entry point and one configuration are worth keeping); a dispatcher that hands the whole build to the driver (every driver would carry the session and a collector, and the collector could not be chosen apart from the driver)." This drops "driver auto-detection" from the reason: `warden` has none today (the driver is the one named, else `container`) and K1's Decision 6 adds none (a16) |
| The seams, intro (269) | "Seams 1 to 8 each get a specification and a black-box conformance suite in the specs repository. Seam 9 is a Go API. Seam 10 is a document." | "Seams 1 to 9 each get a specification and a black-box conformance suite in the specs repository; seam 9's start in this repository (K1) and move there in K2. Seam 10 is a document." Seam 8 still has no phase that writes it; that gap is a11's and this sentence does not close it |
| Seam 1 (275) | "Claim protocol (`relay-claim`) \| Driver and relay \| No \| Written down. The connection it runs over is supplied by the driver (TCP or vsock)." | "Claim protocol (`relay-claim`) \| Claimant (`warden`'s session) and relay \| No \| Written down. The claimant opens the connection at an endpoint the driver reports (TCP in K1; other transports are what remains of S4)." |
| Seam 6 (280) | "Bootstrap config \| Driver and executor, through the relay \| Code only (`bootstrap/`) \| Written down." | "Bootstrap config \| `warden`'s session and executor, through the collector and the relay; the driver reports the guest OS \| Code only (`bootstrap/`) \| Written down." |
| Seam 9 (283) | "Driver contract \| Kit and drivers \| The four-method `driver.Driver` \| New Go API: registration, doctor, setup, images, clean, driver-owned configuration, the relay source and the claim connection; plus the convention by which the dispatcher finds and runs a driver binary." | "Driver contract \| `warden` and a driver binary \| The four-method `driver.Driver` \| New command contract: how `warden` finds a driver binary, and the required and optional commands it runs on it (describe, the build, cleanup after a dead driver, doctor, setup, images, clean, shell, a driver's own), with driver-owned configuration and the claim endpoint. A Go kit implements it; the relay source is the kit's." |

**B. The phases and the order.**

| Where | Current | Proposed |
|---|---|---|
| S4 (305) | "The claim connection: `driver.Session` takes a connection from the driver, not a TCP address. \| Every driver claims through it." | "Mostly absorbed into K1, which settles that `warden` claims at an endpoint the driver reports. What remains: the claim specification's transport section, and a claim endpoint that is not TCP. \| A relay reachable only over a non-TCP endpoint is claimed without a driver-side forward." |
| K1 (311) | "Complete the driver contract in-tree, including the relay source. `cmd/warden` stops naming drivers. `drivertest` exists. \| Adding a driver touches only its own directory." | "The driver command contract, its conformance suite and the kit, including the relay source. `warden` runs a driver binary; the container driver is one, and every driver's build runs through `warden`'s build sequence. \| An unmodified `warden` runs a driver it was not built with, and the suite passes against the container driver." (If a3's option (e) is chosen, "including the relay source" moves to K2's row.) |
| K2 (312) | "Each driver becomes its own Go module and binary in-tree. `warden` becomes the dispatcher. A tier-1 check limits each driver to the kit's public packages. \| The check passes for all five, and `warden` runs each driver as a separate binary." | "Each VM driver becomes a binary and adopts the relay source; then every driver becomes its own Go module in-tree. `warden` links no driver. The driver specification and suite move to the specs repository. A tier-1 check limits each driver to the kit's public packages. \| The check and the suite pass for all five, and `warden` runs each driver as a separate binary." |
| K4 (314) | "This repository holds the kit, the collector, the executor, the dispatcher and the book." | "This repository holds the kit, the collector, the executor, `warden` and the book." |
| K5 (315) | "The dispatcher can report and fetch a newer release of one driver." | "`warden` can report and fetch a newer release of one driver binary." |
| Order, wave 2 (334) | "S2, S3, S4, K1, M2 \| Driver contract complete" | "S2, S3, K1, M2 \| `warden` runs a driver binary; the suite passes" (what remains of S4 moves to wave 3) |
| Order, wave 3 (335) | "K2, M3 \| Modules isolated; unikernel and host mode pass the suites" | "K2, M3, S4 \| Modules isolated; unikernel and host mode pass the suites" |
| What sets the order (342) | "K1 does not depend on the relay and can start as soon as P0 merges." | Unchanged, with one sentence added: "Its specification and suite start in this repository and move to the specs repository in K2." |
| What sets the order (343) | "M6 needs K1's relay source, not driver extraction." | "M6 needs the relay source, not repository extraction: K1's for the container driver, and K2's adoption of it for the four VM drivers." |
| Later, not scheduled (346) | "alternative collectors and a collector repository (seam 2; the collector's storage boundary is designed with the first of them);" | "alternative collectors and a collector repository (seam 2; the collector's storage boundary is designed with the first of them), among them a shared local or remote collector with a session token per build and a client-facing side for opening a session, supplying inputs, watching console and status, and fetching outputs;" (a22) |

**C. The tables and prose that follow from A and B.**

| Where | Current | Proposed |
|---|---|---|
| Component table, Kit (258) | "Session, claim client, ledger reader, script translator, the driver contract, `drivertest`, the `warden` dispatcher, the book" | "`warden` (the orchestrator: session, claim client, collector hand-off), the kit's Go library for drivers, ledger reader, script translator, the book" |
| Component table, Drivers (261) | "One driver, as its own binary built on the kit" | "One driver, as its own binary that speaks the driver contract; the project's own are built on the kit" |
| Rule table, GLOBAL-08 (401) | "The driver contract becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints, the build-link contract and the claim connection. \| K1; S2, S3, S4 for their seams" | "The driver contract (the commands `warden` runs on a driver binary) becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints and the build-link contract. \| K1; S2, S3 for their seams" |
| Rule table, BOOT-02 (402) | "BOOT-02, BOOT-05, the `bootstrap.md` mapping \| As in the Bootstrap comparison. \| K2; S2; S3" | "... \| As in the Bootstrap comparison; BOOT-02 as in K1's own comparison. \| K1; S2; S3" |
| Rule table, TEST-05 (405) | "RELAY-05, TEST-05 \| Contract tests for the seams become the specs repository's suites. \| S1" | "... \| S1; K1 for the driver suite" |
| Rule table, ORCH-01 (406) | "Driver parity becomes passing `drivertest`. \| K1" | "Driver parity is held by the driver contract and its suite. \| K1" |
| Rule table, new rows | none | "ORCH-02 \| `warden` stages and records inputs; a driver alters none. \| K1"; "ORCH-04 \| A driver tears down when its input ends; `reap` reaches what a killed driver left. \| K1"; "UX-01, UX-03 \| Enforcement only: tier 1 also reads the flags and keys the bundled drivers declare. \| K1"; "ORCH-03 \| The same enforcement change; and one paragraph and a clause: a driver reads its own flags and keys, with the same precedence. \| K1"; and "PROFILE-01 \| The profile names the driver contract's specification, its suite and its CI job, the surface the drivers declare, and what `interface` stands for. \| K1" |
| Risks (425) | "**A public Go API** has to be kept usable for integrators." | "**A command contract** has to be kept usable for integrators, and is a second protocol to version beside the kit's Go API." |
| Defaults and overrides (430) | "which driver the dispatcher picks when none is named and how it finds a driver binary (K2)" | "which driver `warden` picks when none is named and how it finds a driver binary (K1)" |
| Cross-driver impact (438) | "it implements the completed driver contract (K1), becomes a module and a binary (K2), declares its build-link class (S3), supplies its claim connection (S4)" | "its build runs through `warden`'s build sequence (K1), it becomes a binary that speaks the driver contract (K1 for container, K2 for the rest) and a module (K2), declares its build-link class (S3), reports its claim endpoint (K1)" |
| Bootstrap comparison, proposed diagram (480) | "per build, from the driver's own kit version" | "per build, the driver's version" |
| Bootstrap comparison, BOOT-02 row (500) | "It is delivered per build and comes from the kit version the driver was built from. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. \| None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. \| K2" | "It is delivered per build by the driver and matches the version of the driver that delivers it. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. \| None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. \| K1 (its own comparison)" |
| Security note (608) | "The kit cannot make an integrator's driver safe; the build-link suite and `drivertest` are what an integrator runs to show theirs is." | "`warden` runs whichever binary of that name it finds, and records it; it cannot make an integrator's driver safe. The build-link suite and the driver suite are what an integrator runs to show theirs is." |
| Validation plan (626) | "`drivertest` and the build-link suite validate each driver." | "The driver suite and the build-link suite validate each driver." |
| Archive summary, Decisions taken (777) | "Drivers as separate binaries on a public Go kit, then modules, then repositories, with Hyper-V as the pilot (4), behind a `warden` dispatcher that works like git's (5)." | "Drivers as separate binaries that `warden` runs through a written command contract, then modules, then repositories, with Hyper-V as the pilot (4); `warden` finds them as git finds a subcommand and stays the orchestrator (5); both reworded by the design review of `driver-contract`, approved <date>." |
| Archive summary, the comparison (793) | "The Bootstrap comparison is accepted for three model changes, BOOT-01 (S3), BOOT-02 (K2) and BOOT-05 (S2), each made later by the item named (a12)." | "The Bootstrap comparison is accepted for two model changes, BOOT-01 (S3) and BOOT-05 (S2), each made later by the item named (a12); BOOT-02's change is made in K1 under K1's own comparison." |
| Archive summary, rule changes (819) | "That includes the three `bootstrap.md` edits, whose comparison is already accepted here (BOOT-07)." | "That includes the BOOT-01 and BOOT-05 edits to `bootstrap.md`, whose comparison is accepted here (BOOT-07); BOOT-02's is accepted in K1's design." |
| Archive summary, Rejected (797) | "Completing only the in-tree driver contract; drivers as RPC plugins; users calling driver binaries directly." | "Completing only the in-tree driver contract; a Go API every driver links; drivers behind an RPC plugin framework; users calling driver binaries directly; a dispatcher that hands the whole build to the driver." |
| Front matter (8) | "the program's reach (the `warden` dispatcher)" | "the program's reach (`warden` and the driver binaries)" |

What stands: Decision 8 and a2 (the collector is specified like every
seam, with no storage boundary and no alternative collector in this
program), which the basis's fifth point repeats and to which "Later, not
scheduled" now adds what Jeff described; Decisions 6 and 7; seams 2 to 5,
7 and 8 (seam 5's "Driver and executor" still holds: the driver starts
the executor); and every row not listed above. The umbrella's concepts
gain no tag. The rule table there is "listed for direction"; the rows
added keep it in step with K1's `rule_changes` and approve nothing.

## Security note

**Boundary:** host to build environment, and with it environment to
relay. The driver is the code that builds the isolation between the host
and the environment and that starts the relay, the environment's only
network path. Until K1 that code is compiled into `warden`. After K1 it
is a program `warden` finds at run time and runs with the user's
privileges (root's, under `sudo warden setup` or `image prepare`).

**How the posture changes.**

- **Which code builds the boundary is decided by a lookup, and the
  lookup trusts `PATH`.** A file that `warden` takes for the selected
  driver controls the isolation, the relay and the executor of that
  build. The redraft guarded the lookup (bundled names never from
  `PATH`, a project file limited to bundled drivers, owner and mode
  checks). The review removed those guards. The reasoning is Jeff's and
  is recorded in his words: "This is overcontrolled. The nicety of this
  is for UX functionality and given this is never outside of the user's
  control (i.e. never a part of the build environment), I'd say let's
  allow for the first option with way less guarding around its official
  drivers, allowing for both customized implementations of existing
  drivers and new ones, either by absolute or PATH search". Asked which
  wins when a name is both bundled and on `PATH`, he chose "PATH wins";
  asked which guards stay, "Only: no path in a project file". The
  posture that follows:
  - *The lookup is part of the host, not of the build.* Nothing inside
    the build environment can reach `PATH`, `warden`'s directory, the
    config file on the host or `warden`'s command line. Whoever can write a
    directory on the user's `PATH` can already replace `git`, the
    container runtime or `warden` itself, and so can already do
    whatever a driver could. The driver lookup adds no new way in for
    such a writer; it adds one more program they could replace.
  - *What a planted binary can now do.* A `warden-driver-<name>` in an
    absolute `PATH` directory that comes before `warden`'s own (in any,
    when `warden`'s directory is not on `PATH`, as under sudo's reset
    `PATH`) runs in place of the bundled driver of that name, on every
    command, `--help`, `doctor` and `clean` included, and as root under
    `sudo warden` if it is on root's `PATH`. It chooses the isolation,
    the relay and the executor, and it receives the claim token. It can
    therefore run a build with no isolation, or with a relay of its own
    that signs a ledger of its choosing. `warden` cannot tell (a19).
  - *What a build context can and cannot do.* A `warden.toml` that
    comes with a build context is read when `warden` is run there with
    no `--config` and no `WARDEN_CONFIG`. It
    can select a driver by name. It cannot name a driver's file, and a
    file reached through the current directory or a relative `PATH`
    entry is not run, so a cloned repository cannot supply a driver
    binary by itself. It can where the user's `PATH` holds an absolute
    entry inside the repository ahead of `warden`'s directory, which
    some setups create (a per-project `bin` added by an environment
    hook): then the repository ships the file and its own `warden.toml`,
    or merely the default driver's name, selects it. This is the
    exposure the old "bundled names never from `PATH`" rule closed for
    the five bundled names, and it is now open by decision. A user in
    that position passes `--driver <path>` or keeps such directories
    off `PATH`; the reference docs say so (UX-01). A file the executor
    names with `--config` is held to the same rule (a26), and with it
    the repository's own `warden.toml` is not read at all, which is how
    a repository that is not trusted is built
    (`project-config-trust`). Separately: on `main` at `ad4ea50` a
    `warden.toml` can make `warden` run a program the repository
    ships, through `[runtime] cli` under the default driver. The rule
    kept here is about the driver's file and does not close that.
    `runtime-cli-project-path` does, before K1, and K1 keeps its rule
    in the container driver (**What K1 takes from the two
    configuration items**).
  - *What shows it.* `warden driver list` shows which file runs for
    each name, every copy it shadows and every file it skipped, and
    marks the driver the current flag, variable and config select, a
    path included; every build's `driver.json` records the path, the
    file's digest, where it was found and what it shadowed (a19). Both
    are for the person who ran the build. Neither is signed, and
    neither prevents anything.
  - *What stays fixed.* Within one invocation the driver is resolved
    once and that path is run for every command, `reap` included.
    `warden clean` reaps a dead build with the binary its record names,
    and says so when that file has changed since.
- **Root.** `sudo warden` runs the driver as root. Under sudo's reset
  `PATH` the search covers root's `PATH`, then `warden`'s directory,
  where the bundled copy is. No ownership check is made for root either;
  the review's answer does not distinguish it.
- **The build record and state file** live in the user's lock directory
  (mode 0700), carry no secret, and are read back by `warden clean`,
  which runs the binary a record names. Whoever can write that directory
  is the user.
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
- **A driver reads the configuration itself** (a21). The driver's
  process is named the one config file and parses it. That gives it
  nothing new: it runs as the user and could read the file. What changes
  is who interprets a value. A key under `[driver]` or
  `[driver.<name>]`, and each of the seven existing driver-owned keys
  under its old name, is read and acted on by the driver with no check
  by `warden`. A common key in `[driver]` reaches every driver that
  honours it; version 1's two are ages and name no program or path. The
  file is `./warden.toml` unless the executor named another, so it may
  have come with a cloned repository, and the contract does not tell the
  driver which. The design read at `5808805` carried a level with each
  file, so that a driver could hold a key that names a program to the
  user's file alone. The level is gone with the second file, and the one
  key it was carried for no longer needs it: the container driver's
  `cli`, which names a program to run, takes one of three names from
  every source (`runtime-cli-project-path`), and K1 keeps that check in
  the driver, under both names of the key. Two more driver-owned keys
  take a value from that file that reaches further than a name:
  `relay_image` (container) and `windows_image` (hyperv).
  `project-config-trust` proposes that a file found in the working
  directory decides through them what a named file decides, with one
  line on every run that says which file was read, and that the
  protection for a repository the executor does not control is to name
  the file; it also has a relay image that is not shaped like a
  reference refused. That posture is that item's to state and its
  review's to accept. K1 moves the reader of both keys into the two
  drivers, with the relay image's check, and changes nothing in it. A
  driver from outside whose key names a program or a host path is where
  `cli` was: the specification tells it to take its file for one that
  may have come with the build context, and to hold such a key to a
  closed list of names. A driver's flags travel in the request, not in
  its argv, and are kept in the build's record, so the contract tells a
  driver to take no secret as a flag.
- **Secrets across the process boundary.** One secret goes to the driver:
  the claim token, in the `start` message on stdin, never in the argv or
  the environment of the driver process, and never in the build record.
  What the driver then does with it is unchanged: the container driver
  passes it to the runtime CLI as `--env` and into the relay container's
  environment, vz puts it in the relay child's environment, and qemu puts
  it in its VM's command line, as each does today. It lets its holder
  claim that build's relay once, so a
  driver that leaked it could let another local process claim the relay
  first; the build would then fail at `warden`'s claim, not run
  unobserved. The collector token never leaves `warden`: the driver is
  handed nothing about the collector (a13). The request and result files
  for the other commands are created 0600 and carry no secret.
- **The claim endpoint** is a TCP address the driver reports, and
  `warden` dials whatever it is told. A hostile driver could point it at
  something else; it could do worse directly, being the driver. The
  endpoint is on loopback or a host-only network for all five drivers and
  is guarded only by the token, as today, and the token is visible to
  other local processes on container, vz and qemu, as today (above).
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
  is held as before by each driver's code; the message sequence states it
  and the suite checks the order a driver reports (BOOT-01).
- **Signals.** SIGTERM now tears a build down on every driver, where on
  three it killed `warden` and left the environment and its relay
  running. A second signal on a linked qemu build can now leave its VMs
  running, which it could not before (Behaviour 8).
- **The relay source** is as in the first draft: `Relay.Verify` is new but
  inert, since no source in K1 supplies a hash; the two existing
  overrides let the user run a relay of their choosing, as today.

**What the TEST-08 suite should exercise.**

- Lookup: a `warden-driver-container` in the current directory is not
  run; one reached through a relative `PATH` entry, or on Unix an empty
  one, is not run and the command fails naming the entry; one in an
  absolute `PATH` entry ahead of `warden`'s directory is run in place of
  the bundled copy, and `warden driver list` and `driver.json` name it
  and the copy it shadows; one in an entry after `warden`'s directory is
  not run and is listed as shadowed; the bundled file met through `PATH`
  or through a symbolic link is reported `beside-warden` and once;
  `[runtime] driver = "./x"` or an absolute path in `./warden.toml`
  is refused before any driver runs, and under `--help`
  the file is not run either, and the same value in a file named with
  `--config` or `WARDEN_CONFIG` is refused too; `[runtime] driver = "x"`
  in the config file selects `warden-driver-x` by lookup; on Windows
  a `warden-driver-x.bat` is not taken for the driver. No case asserts
  an owner or a mode: those checks are gone (a16).
- The container driver's runtime, once the check has moved into the
  driver: `runtime-cli-project-path`'s cases, with the key under its
  new name and its old one. A `warden.toml` whose `cli` is a file the
  context ships, or `sh` beside a file named `pull`, runs nothing and
  exits 1.
- A driver sent no `config_file` reads none: not the `warden.toml` of
  its working directory, and not the file `WARDEN_CONFIG` names.
- The claim token appears in the argv or environment of no driver process
  that `warden` starts (`run` and every other command), and in no build
  record.
- A driver that reports `claim_values` with extra fields does not change
  the claim's collector, capture or trust fields.
- A driver that reports `environment-started` before `claimed` fails the
  suite's Sequence case.
- A `reap` that cannot reach its runtime exits 1 and the lock and record
  stay; a `warden clean` run with another `--runtime` and `--tmpdir` than
  the dead build's still removes that build's containers and work
  directory, and so does one run after the build's checkout, with its
  `warden.toml`, was deleted.
- `warden clean` with a key of this invocation that the driver refuses
  (`[runtime] cli = 5`) fails before any dead build is reaped.
- A build whose lock is in another user's cache directory is not removed
  by the container driver's `clean` while its `run` process lives.
- A killed `warden` (its pipes closed) leaves no container of the build
  once the driver has exited.
- The existing isolation and measurement cases, on every driver, as the
  regression check that moving the session changed nothing.
- For the relay source, when hashes exist (M6): a relay file whose bytes
  do not match its published hash is refused before anything starts, and
  an unusable override is an error, never the bundled relay.

## Validation plan

**Where tier 0 runs.** `make test` runs wherever a developer runs it
(TEST-01). In CI it runs on Linux only: `main.yml`'s `test` job is
`ubuntu-latest`, the `build` job builds on macOS and Linux and runs
`warden --help`, and no job builds `warden` for Windows. K1 adds code that
differs per platform, so it adds one job (Decision 20):

- **`driver-contract`**, in `.github/workflows/main.yml`: a matrix over
  `ubuntu-latest`, `macos-latest` and `windows-latest` that runs `go test`
  for `./cmd/warden/...`, `./conformance/...`, `./driver`,
  `./driver/drivercmd/...`, `./driver/relaysource/...` and
  `./driver/container/...`, under the same `changes` filter as `test`,
  and is added to `ci-ok`'s `needs`. The `test` job is not edited.
- It is not a rule change (FLOW-14). The guarded jobs are those that run
  tier 1, `golangci-lint` or `tools/workflow`, and the `needs:` entries
  that name them; this job runs none of the three, and FLOW-14's check
  (`tools/workflow/check/workflows.go`) allows a new `needs:` entry "so a
  new job can join the required check". `rule_changes` therefore has no
  entry for it.
- `cmd/warden`'s and `driver`'s existing tests have never run on macOS or
  Windows in CI. They compile for both; whether they pass is not known
  (**Risks**). The task that adds the job runs them there first.

**Tier 0:**

- The characterization goldens (help, refusals, request tables, staged
  inputs, image list and verify, and the image a VM build resolves for a
  script-plus-Dockerfile context), written before any production change
  and diffed across it.
- `warden`'s build sequence against a fake `driver.Host` peer: the order
  of the messages, the claim cancelled by a relay exit, the 10 s grace,
  `stop` on completion, failure, timeout, SIGINT and SIGTERM, and each
  row of the failure table.
- The build lock and record: the lock is taken before anything is
  created; a lock unlinked between open and lock is detected and retaken
  (the race `warden clean` could otherwise win); the record holds no
  secret; `reap` and `attach` get the record's values and not the
  invocation's; a failed `reap` keeps the lock and record; `warden clean`
  removes a dead build's stage directory, for a linked driver too.
- The exec transport and the lookup: the order (a path, `PATH`,
  `warden`'s directory, a linked driver) over a table of directories,
  with a bundled name shadowed from an earlier `PATH` entry, not
  shadowed from a later one, and a linked name replaced by a file;
  `warden`'s own directory on `PATH`, once and twice, and reached
  through a symbolic link, each reported `beside-warden` and listed
  once; a directory and a file with no execute bit passed over and
  listed as skipped; a candidate in a relative `PATH` entry refused with
  the entry named; a symbolic link followed; the rule that no config
  file names a path, for `./warden.toml` and for a file named with
  `--config`; `driver list` with its
  `shadowed` and `skipped` entries, its mark on the selected driver and
  its entry for a path from the flag and the variable; the result and
  request files, every
  exit code, a line that is not JSON, a driver that exits early, a path
  that is not UTF-8, the teardown wait (its default, a value from the
  config file, a value that does not parse, one out
  of range and one that is a bare number, each refused at load with the
  key's own message; the same wait applied to the `reap` after a kill,
  and a `reap` killed when it passes or on a third signal, with the
  record and lock kept) and the 10 s and 30 s limits with a fake clock.
- `warden`'s end-to-end tests through `warden-driver-fake` and
  `stubrelay`: build (success, failure, timeout, Ctrl-C, second Ctrl-C,
  SIGTERM, killed driver, noisy driver, a `warden` killed with its pipes
  closed), shell and a failed attach, doctor, setup, image prepare, list
  and verify, the fake's `ext` command through `warden fake echo`, clean
  after a dead build and with `keep_ids`, `driver list`, `driver.json`, a
  driver named by path.
- Configuration, `warden`'s side: the selected driver's declared flags
  registered on the commands that take them and shown in `--help`; a
  clash with one of `warden`'s flags refused; a flag in both spellings
  and, for one declared for every command the driver lists, before the
  subcommand's name; the scan for the driver, with three lines in
  particular: `warden nomad job run --driver docker` (the `ext` form:
  nomad is run and `--driver docker` is its argument), `warden build
  --network --driver hyperv` (refused: the parsed `--driver` is not the
  scanned one) and `warden build ./ctx -- --driver x` (the scan stops
  at `--`); `--config` found by the same scan, the driver selected from
  the file it names, and `warden build --network --config x` refused;
  a command that selects no driver run with the default
  driver missing; the `args` built from a command line, in order and
  with repeats, with a switch given as `--force=false` and as `--force
  --force=false` left out, and a flag with an empty value passed on as
  `--name=`; a flag nobody declares refused before
  any driver command but `describe`; `--runtime` on a command that
  selects no driver, and on `setup` under the container driver;
  `config_file` naming the file `warden` itself read, as an absolute
  path, and absent when there is none; `image list` run beside a `warden.toml` that is not
  TOML; a `describe` with a key outside the driver's two tables, with
  a `driver.<key>` the contract does not name, or with an old name not
  listed for that driver, refused; `--help` for a driver that cannot be described, with a
  config file that does not load and with a path in the config file,
  each printing `warden`'s part and exiting 0; `warden clean` probing
  the driver's `clean` before it reaps; and that
  `attach` and `reap` receive the build's `args` and `config_file` from
  its record.
- Configuration, the kit's side (`drivercmd.Resolve`), per bundled
  driver: precedence (flag, variable, file) over the
  rows of `main`'s request tables, written once with the old names and
  once with the new, to the same result; the five steps for a common
  key, with the worked example as cases, a value in the driver's
  own table that cannot be used refused without `[driver]`'s being
  tried, and an empty string in the driver's own table hiding
  `[driver]`'s value; `[driver]` not read by a driver that lists no
  common key; for each of the seven keys, the old name read as the new
  one, one warning for each old name a file sets and
  none in `reap` or `attach`, `build`, `shell` and `clean` with an old
  name printing exactly one line each (in the probe, and none in the
  `run` or `clean` that follows), `doctor` printing one with no probe
  before it, and a file with both names refused with
  both named; a declaration with a `Key` outside the driver's table,
  with a common key's name, or with an alias the specification does
  not list, refused; a value of the wrong type refused with
  the file and key named; an undeclared argument refused; another
  driver's table and `warden`'s tables ignored; no file read that was
  not named; a named file that does not exist, on a command with a
  setting that has a key, refused with the file named, and not read as
  if none had been named (C6); a `Raw` key decoded whole from one
  table; an
  empty flag value and an empty variable falling through to the next
  place; in
  `reap` and `attach`, no file opened until a setting is read, and a
  named file that does not exist read as if none had been named; and the same
  result through `Main`, through `Call` and through `Resolve` called
  from `cmd/warden`, so the linked drivers and the container binary are
  shown to read alike.
- UX-09's exit-path table, with the failed attach, SIGTERM and a driver
  defect added.
- `driver/drivercmd`: dispatch, `describe` built from the Go value and
  valid against the schema, the result file, the error mapping, stdout
  redirected during `run`, end of input as cancellation, teardown
  completing with stdout and stderr closed, `Setting`, `Resolve` and
  `driver.Settings`, and the state file helpers.
- The suite's static tier against the fake and against
  `warden-driver-container`; the suite's self-test with each of the
  fake's fault switches; the suite's imports test.
- `specs/driver-contract/v1.schema.json` against the examples in `v1.md`
  (`specs/specs_test.go`), and the `schema-doc` pair.
- `driver/relaysource`: one place or the other and never mixed; an
  override used exclusively; `Verify` on a matching and a mismatching hash.
- The rewritten `driver/claimlink_test.go`, and the tier-1 surface
  extraction tests.
- Each driver's existing unit tests, plus the tests that move with code
  from `cmd/warden` into `driver/container`. Among those are
  `runtime-cli-project-path`'s tables for the runtime, with the rows
  they have for `[runtime] cli` repeated for `[driver.container] cli`,
  the marker-file case included.

**Tier 1** (the `integration` workflow, Docker): `make integration-test` as
today, now through `warden-driver-container`, and the suite's lifecycle
tier against it. `runtime-cli-project-path`'s cases in that script run
unchanged, but for `warden clean` under qemu and vz (Behaviour 5). The workflow's `paths` gain `driver/drivercmd/**`,
`driver/relaysource/**`, `conformance/driver/**` and
`specs/driver-contract/**`, so a later change to the code the binary is
built from still triggers it (the TEST-02 caveat); `driver/*.go` does not
match subdirectories. The `firecracker` workflow's `paths` gain
`driver/drivercmd/**` for the same reason. Neither edit touches a job
that runs the rules. TEST-02's sentence on where tier 1 runs is a summary
that already omits `relaycfg/` and `driver/buildlink/`; it is left as it
is, and the workflow file is the list.

**Tier 2:**

- firecracker: the `firecracker` workflow on its hosted KVM runner, which
  this change triggers (`driver/firecracker/**`, `driver/*.go`,
  `cmd/warden/**`). It gains a build interrupted mid-way (nothing left on
  the slot), one ended by SIGTERM, and a build whose VM is killed (fails
  after the grace). No native step (a7). The workflow is not a required
  check, so Publish confirms that its run on the PR's head passed.
- Native, blocking (FLOW-09), each on the branch after consolidation.
  Where a case says "as `main`", the output is captured on `main` first.

| Host | Cases |
|---|---|
| macos-finch | (1) `warden driver list` shows `container` at the built `warden-driver-container`, found beside `warden` and marked as selected, and the four linked drivers; `warden doctor` prints `container runtime: finch` and `ok`. (2) A default build succeeds and its ledger verifies; `driver.json` names the driver binary and its digest; `inputs/` holds `Containerfile` and `build.sh`. (3) `warden shell` opens a shell and passes its exit code through. (4) `[driver.container] relay_image = "dev"` builds the relay from source, and so does the old name `[runtime] relay_image = "dev"`, with one warning line that names the new key; a `warden.toml` that sets both is refused before anything is created; `WARDEN_IMAGE=<ref>` pulls that image; an unpullable reference fails without trying the default. (5) Ctrl-C mid-build tears down and leaves nothing; SIGTERM mid-build does the same and exits 1; with the driver killed mid-build `warden` fails the build and reaps; with `warden` killed (SIGKILL) mid-build the driver tears down by itself and no container of the build is left. (6) With `warden` and the driver both killed, the build's containers are still running; `warden clean`, run with another `--tmpdir` than the build's and after the build's `warden.toml` was deleted, removes that build's containers, network, work directory, stage directory, lock and record, and leaves a running build's; a running build started with another `HOME`, whose lock is elsewhere, is left alone too. (7) `WARDEN_CTR_CLI` and `--runtime` select the runtime; a missing one gives the fixable error and leaves no output directory; `--network x` is refused by `warden` as an unknown flag for the container driver; `[runtime] cli = 5` in `warden.toml` is refused by the driver, naming the file, and leaves no output directory, and so is `[driver.container] cli = 5`. (8) With nothing customized, a build's `driver.json` has `found: beside-warden` and nothing shadowed. A copy of `warden-driver-container` placed in a directory that comes before `warden`'s on `PATH` runs in place of the one beside `warden`: `warden driver list` shows it and the copy it shadows, and the build's `driver.json` has `found: PATH`, its digest and the bundled copy under `shadowed`; the same copy in a directory after `warden`'s on `PATH` does not run and is listed as shadowed; `[runtime] driver = "./x"` in the project's `warden.toml` is refused. (9) `make driver-conformance DRIVER=dist/warden-driver-container` passes both tiers on finch |
| container-any | On the Windows host, with its container runtime: (1) `warden driver list` shows `warden-driver-container.exe`; `warden doctor` reports the runtime and `ok`. (2) A default build succeeds, its ledger verifies, `driver.json` is written. (3) Ctrl-C mid-build tears down; with the driver killed `warden` reaps; with `warden` killed the driver tears down on the end of its input. (4) `warden clean` leaves a running build and removes a build whose `warden` and driver were both killed, from its record. (5) A `warden-driver-container.bat` beside `warden` or on `PATH` is not taken for the driver; a `warden-driver-container.exe` in a `PATH` directory ahead of `warden`'s replaces the one beside `warden`. (6) Both tiers of the suite pass against `warden-driver-container.exe` |
| macos-vz | (1) `warden doctor --driver vz` and `warden image list` print what `main` prints. (2) A vz build succeeds, its ledger verifies, `relay.log` is written, `driver.json` says `linked`; with a Containerfile in the context, it is ignored and `inputs/` is absent, as on `main`. (3) A script that exits 3 makes `warden` exit 3; `--timeout 30s` on a sleeping script exits 124. (4) Ctrl-C mid-build leaves no relay process and no work directory; SIGTERM mid-build does the same and exits 1, where on `main` it kills `warden`. (5) `--runtime docker` and `--network x` with `--driver vz` are each refused as an unknown flag for driver `vz`; `[runtime] cli = 5` in `warden.toml` does not stop a vz build (Behaviour 13); `warden clean --driver vz` reports nothing to clean and needs no container runtime |
| windows-hyperv | (1) `warden doctor --driver hyperv`, elevated and not, prints what `main` prints; `--network` is honoured. (2) Elevated `warden setup --driver hyperv --slots 2` is idempotent. (3) A non-elevated durable build and an elevated ephemeral build succeed and verify; `[driver.hyperv] network` is honoured and `--network` overrides it; the old name `[vm] network` is honoured too, with one warning line that names the new key; with `[driver] image_refresh_after` set low, the detached `image prepare --driver hyperv` starts and logs, and with `[driver.hyperv] image_refresh_after = "30d"` beside it in the same file, it does not. (4) With the service installed, a service-delegated build succeeds. (5) A build whose script powers the VM off without completing fails with the build-VM-off error about 10 s after the VM stops. (6) Ctrl-C mid-build removes both VMs and the switch; the time from Ctrl-C to exit is recorded, to judge the 60 s default of the teardown wait against a real teardown before K2 applies it to Hyper-V (a20). (7) `--keep-vms` leaves both VMs; `warden clean --driver hyperv` then removes them and leaves a running build's |
| qemu-any | (1) `warden doctor --driver qemu` prints what `main` prints. (2) A direct-boot build and a cloud-image build succeed and verify; a Dockerfile whose `FROM` names a prepared image boots that image, as on `main`, and still does when the build is given `--script`, as on `main`; `inputs/` holds `Containerfile` and `build.sh`. (3) Ctrl-C mid-build leaves no `qemu-system` process and no work directory; SIGTERM mid-build does the same; `--keep-vms` leaves the work directory only. (4) `--timeout 30s` on a sleeping script exits 124. (5) `warden clean --driver qemu` reports nothing to clean with no container runtime running. (6) A second Ctrl-C during teardown ends `warden`; what it leaves is recorded (Behaviour 8) |

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
each task leaves `make test` and `make lint` green (FLOW-17). Plan
starts after `project-config-trust` has merged and `main` is merged
into the branch; a `main` that still reads
`~/.config/warden/config.toml` stops the first task (**What K1 takes
from the two configuration items**).

1. Add the characterization tests to `cmd/warden` on unchanged production
   code: help goldens, the refusal table, request tables per driver, the
   staged inputs per driver, the image a VM build resolves for a
   script-plus-Dockerfile context, `image list` and `image verify`
   fixtures (GLOBAL-04, UX-09, TEST-01, TEST-04).
2. Move staging to `warden`: `prepare` stages with
   `driver.StageBuildScript`, records with `driver.RecordInputs`, drops
   the Containerfile for vz, and passes the staged path and the
   Containerfile's `FROM` image in the request, with `--script` too; each
   driver stops staging and translating (`container.stageBuildScript`,
   `linuxguest.ContainerfileImage`) (ORCH-02, BOOT-04, ORCH-01).
3. Add the build lock to `driver/hostlock.go` (taken, compared with its
   path, retaken when it was unlinked; removable under the lock) with its
   test, and `cmd/warden/buildrecord.go`: the record, the state file's
   path, and cleanup of a dead build from them (ORCH-04, GLOBAL-07,
   GLOBAL-04).
4. Add `driver.Host`, `driver.Start`, `driver.RelayStarted`
   (`driver/host.go`) and `warden`'s build sequence
   (`cmd/warden/buildseq.go`) over `driver.Session`, with `linked.go`;
   `warden` makes the build ID and the claim token, takes the lock and
   writes the record before it stages, handles SIGINT and SIGTERM for
   every driver, and follows the failure table (ORCH-01, ORCH-04,
   BOOT-01, RELAY-07, RELAY-08, GLOBAL-01, UX-09).
5. Move the container driver's `StartBuild` onto `Run`; it reports
   `exited` from `container wait` (ORCH-01, BOOT-01).
6. Move the firecracker driver's `StartBuild` onto `Run`; its relay-exit
   watch and `waitBuild` rule move into the build sequence with their
   tests; its signal handler goes (ORCH-01, ORCH-04).
7. Move the qemu driver's `StartBuild` onto `Run`; its signal handler
   goes (ORCH-01, GLOBAL-07).
8. Move the vz driver's `StartBuild` onto `Run`; `vz.New` does no host
   lookup; it reports `guest_os: darwin` and names its work directory and
   relay child from the build ID (ORCH-01, GLOBAL-07).
9. Move the hyperv driver's `StartBuild` onto `Run`: `StartRelay` gives up
   its session and claim; `watchBuildVM` reports `exited` (ORCH-01,
   ORCH-05, GLOBAL-07).
10. Remove the four-method `driver.Driver`, `newDriver` and the per-driver
   grace constants; rewrite `driver/claimlink_test.go` (ORCH-01, TEST-01).
11. Split the container driver's `Exec` into the held `Run` (`mode:
    shell`) and `Attach` (BOOT-03, UX-09).
12. Write `specs/driver-contract/v1.md` with numbered requirements and
    `v1.schema.json`, add it to `specs/README.md` and the book's specs
    pages. It is done at step 1 of **Order of the work**, ahead of tasks
    2 to 11 (LOOP-02). It lists each requirement that is not
    machine-checked, with the reason (LOOP-03). It includes the
    build-lock convention, the failure table, the
    probe form of `start`, commands not started by `warden`, UTF-8 paths,
    teardown with closed output, the configuration requirements C1 to
    C11 (`config_file`, the TOML version of the
    file, `args` and the form of a switch, `describe`'s `flags` and
    `keys` with their types and commands, the two tables, the common
    keys of version 1 with their type and meaning, the precedence with
    its five steps and the worked example, the seven old names by
    driver name, and the state file's authority in `reap` and
    `attach`), how `warden` finds the driver
    before it parses, `warden`'s reserved flag names, and an introduction that says
    what a
    driver needs besides this contract (GLOBAL-08, RELAY-05's
    `schema-doc` pairing, UX-01).
13. Add `driver/drivercmd` to match the specification, with doc comments
    on every exported identifier: dispatch through `Main` and `Call`,
    `Preflight` for a probe, `Setting`, `Resolve` and `driver.Settings`
    (the flags from `args`, the two existing variables, the keys from
    `config_file`, with `warden`'s precedence; a common key read in the
    driver's own table and then in `[driver]`; an old name read as its
    key, with its warning, printed once per command (C11), and the
    refusal of a file that sets both),
    the state file helpers,
    and `run` ignoring SIGPIPE (GLOBAL-01, GLOBAL-03, GLOBAL-08,
    GLOBAL-10, ORCH-03, ORCH-04, UX-08).
14. Add `conformance/driver/`: `warden-driver-fake` with all eleven
    commands and a flag, a key of its own and a common key read by
    hand, `stubrelay`, the suite's
    static tier with the configuration checks, its self-test with the
    fault switches, and the imports test; run the static tier under `make
    test`. Each case names the requirements of `v1.md` it checks
    (TEST-01, TEST-05, GLOBAL-07, LOOP-03).
15. Add the `driver-contract` job to `.github/workflows/main.yml` (three
    platforms) and to `ci-ok`'s `needs`, leaving the `test` job as it is;
    run `cmd/warden`'s and `driver`'s existing tests on macOS and Windows
    first and fix or build-tag what fails there (TEST-01, GLOBAL-07,
    FLOW-14).
16. Change tier 1's surface extraction as approved, to read
    `drivercmd.Setting` literals (`Flag`, `Key`, `Common`, `Aliases`),
    with its tests; leave `rules.yaml`'s
    `surface` keys as they are (UX-01, UX-03, ORCH-03, FLOW-14, LINT-05).
17. Add the lookup (`driverlookup.go`: a path, `PATH`, `warden`'s
    directory, a linked driver) with the rule that no config file
    names a path, the exec
    transport (`drivercall.go`) for every
    command of the contract (`run` and its probe, `reap`, `doctor`,
    `setup`, the image commands, `clean`, `attach`, and `ext` as `warden
    <name> ...`), the refusal of a path that is not UTF-8, `warden driver
    list`, `driver.json` and the teardown wait read from `[build]
    teardown_timeout` (GLOBAL-06, GLOBAL-07, GLOBAL-10, GLOBAL-11,
    UX-02, UX-04, UX-09, ORCH-04, ORCH-05).
18. Move every driver-owned flag and key into its driver's `Setting`
    declaration, the seven keys under their new names in `[driver]` and
    `[driver.<name>]` with their old names as `Aliases`; `warden` finds
    the driver before parsing flags (the `ext` form, the command, then
    the scan for `--driver` and `--config`), registers
    the flags the selected driver declares, refuses one nobody declares
    and a line whose parsed `--driver` or `--config` is not the scanned
    one, and
    passes the driver's on in `args` with `config_file`; `warden clean`
    probes the driver's `clean` before it reaps;
    each driver reads its settings through the kit, the linked ones
    through `Call` in `warden`'s process for `run`, and the VM drivers' doctor,
    setup, image and clean code through `Resolve`; qemu and hyperv
    start their background image prepare with the `config_file` of
    `start`; the seven driver-owned
    fields and the two variables leave `Config` and `LoadConfig`. Update
    the goldens for the eighteen differences only (ORCH-03, UX-03,
    UX-07, UX-08, GLOBAL-10).
19. Add `warden`'s end-to-end tests against the fake, for every command,
    run on three platforms by the `driver-contract` job (TEST-01, TEST-04,
    UX-09, ORCH-04).
20. Add `cmd/warden-driver-container`: move the container driver's doctor,
    runtime detection and cleanup out of `cmd/warden`; its `Preflight`;
    the runtime checked against the driver's supported list, with
    `runtime-cli-project-path`'s transitional detection and its tests
    moved here and the rows for `[driver.container] cli` added, and
    written to the state file; `project-config-trust`'s check of the
    relay image's shape, moved with `relay_image`; `reap` (exit 1 when
    the runtime does not answer) and `clean` with `keep_ids` beside the process scan;
    take it out of `linked.go`; `warden clean` cleans up after dead builds
    from their records (ORCH-01, ORCH-04, GLOBAL-06, GLOBAL-07,
    GLOBAL-10, GLOBAL-11).
21. Add `driver/relaysource` (`Bundled`, `Path`, `Image`, `Relay.Verify`)
    and route the container driver's relay through it (GLOBAL-03,
    GLOBAL-10, GLOBAL-11, RELAY-08).
22. Packaging: `make build`, the goreleaser build and archive, the
    container driver's ldflags, `install.sh` (GLOBAL-07, ORCH-05).
23. Add the suite's lifecycle tier, `make driver-conformance`, and its
    runs in the `integration` workflow; add `driver/drivercmd/**`,
    `driver/relaysource/**`, `conformance/driver/**` and
    `specs/driver-contract/**` to that workflow's `paths` and
    `driver/drivercmd/**` to the `firecracker` workflow's; extend the
    `firecracker` workflow's cases. Each lifecycle case names the
    requirements it checks, and a test under `make test` fails on a
    requirement of `v1.md` that no case of either tier names and the
    specification does not list as not machine-checked (TEST-02,
    TEST-05, TEST-07, LOOP-03).
24. Make the approved rule edits: GLOBAL-08 in `global.md`; ORCH-01,
    ORCH-02, ORCH-03 and ORCH-04 in `orchestrator.md`; TEST-05 in
    `testing.md`;
    BOOT-02 and the diagram in `bootstrap.md`; PROFILE-01's four rows in
    `loop-profile.md`; the last paragraph of UX-08 in
    `ux-cli-config.md`, as `runtime-cli-project-path` left it on `main`
    (if it is not there as **Rule changes** quotes it, no edit, and back
    to Design);
    `seams`, `drivers.shared` and the TEST-05 owner in `rules.yaml`
    (FLOW-14, BOOT-07). Nothing in `process.md` is edited (LOOP-09).
25. Make the approved amendments to `docs/design/componentization.md`
    (FLOW-12, FLOW-20).
26. Update `guide/cli.md`, `guide/configuration.md`, `guide/output.md` and
    `guide/drivers.md` for the eighteen differences, the lookup, what a
    file on `PATH` replaces, `[driver]` with its common keys,
    `[driver.<name>]`, the order between the two tables, the table of
    the seven renamed keys with the window (until 1.0), and
    `[build] teardown_timeout`; `concepts/overview.md` and
    `contributing/structure.md` for the new shape; `CONTRIBUTING.md` for
    `make driver-conformance`; `getting-started/installation.md` for the
    second binary (UX-01, FLOW-15).
27. Update the tracked gap's backlog entry if Design has not (TEST-03,
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
next sitting reviews the design from scratch. Before it was put to Jeff
the redraft was read by three independent reviewers (against the code;
against the umbrella, the template, the rules and the queue file; and
against Jeff's notes), and their findings are folded in: the build record
(Decision 18), the signals (Decision 19), the three-platform CI job
(Decision 20), the file checks, the project-file rule, the failure table,
the completed list of umbrella amendments, and the table at the top that
sets Jeff's words beside what the draft does.

| Section | Since the first draft |
|---|---|
| Front matter | `seams`, `surfaces`, `security_boundary` (`host-env`), `native_validation` (container-any added), `tier2_gaps`, `rules`, `rule_changes` (ten), `concepts`, `related` |
| Opening | New: the basis as recorded, and the table of where the draft is narrower than it or than Jeff's words |
| Problem | The inventory stands; added: the build living inside each driver, and the split signal handling |
| Prior art | `cmd/collector`, `seam-specs`, `container-inputs` and the outside prior art added |
| Decisions | Rewritten (20). Kept in substance: options as data, one switch for the VM settings, `[driver.<name>]`, the relay source, the dropped `clean` fallback |
| Questions | a1, a2, a6, a10 retired; a3, a4, a5, a7, a8, a9, a11 reworded; a12 to a23 new |
| Design | Rewritten: the contract is commands, with discovery and its file checks, the build messages, the build record, the sequence, signals and the failure table, the kit, the collector, what a driver needs besides the contract, and a suite run against a binary. The options table, the per-driver operations table and the relay source stand, reworded. Scope: only the container driver becomes a binary; the VM drivers' doctor, setup, image and clean code does not move |
| Defaults and overrides | Rewritten around the lookup, the collector, the record, the time limits and RELAY-08's table |
| Cross-driver impact | Rewritten: `driver.Session`'s callers change, which the first draft said they did not |
| Bootstrap comparison | New (BOOT-02) |
| Rule changes | GLOBAL-08 and ORCH-01 rewritten; ORCH-02, ORCH-04, TEST-05, BOOT-02 and FLOW-02 new; the UX-01, UX-03 and ORCH-03 enforcement change stands, reading `drivercmd.Option` |
| Amendments to the umbrella design | New |
| Security note | Rewritten for `host-env` |
| Validation plan, Implementation plan | Rewritten |


### 2026-11-14, second sitting on the redraft: fourteen questions answered; three answers folded in

Read at `0c29fc3`. The sitting is not finished and has no outcome yet.
Jeff answered fourteen of the nineteen assumed questions. No rule change
and no amendment to the umbrella has been put to him yet.

Eleven answers confirm the recommendation and change no text:

- a15 (a): only the container driver is a binary in K1; all five run on
  `warden`'s one build sequence; the four VM drivers stay linked until
  K2.
- a14 (a): one `run` process per build; the other commands run once and
  exit.
- a12 (a): `warden` starts the default collector, one per build with a
  fresh token; the driver starts none.
- a13 (a): `warden` dials the claim endpoint the driver reports and
  sends the claim; `start` keeps the reserved `collector` field.
- a22 (a): one clause added to the umbrella's "Later, not scheduled";
  nothing queued.
- a19 (a): `warden` cannot vet a driver from outside; GLOBAL-08 and the
  contract say so; name, version, path and file digest go in
  `driver.json`, unsigned.
- a23 (a): "supported consumer of this contract", with the sentence that
  the relay's and the executor's requirements are their seams', in K1.
- a7 (a): macos-finch, container-any, macos-vz, windows-hyperv and
  qemu-any blocking; macos-qemu tracked; linux-kvm left to the
  `firecracker` workflow; the new three-platform CI job.
- a3 (a): the relay source is a kit library outside the contract; K1
  ships its types and sources and routes the container driver's
  override through them; the VM drivers adopt it in K2.
- a11 (a): provisional Go constants in K1; seam 8 filed as its own
  S-track item, to exist before M6 adopts a release.
- a17 (a): `specs/driver-contract/v1.md` and `conformance/driver/` in
  this repository, moved to `buildwarden-specs` in K2.

Three answers depart from the redraft and are folded into the design,
one commit each, and nothing else is changed:

- **a16, lookup and trust: changed.** Jeff: "This is overcontrolled. The
  nicety of this is for UX functionality and given this is never outside
  of the user's control (i.e. never a part of the build environment), I'd
  say let's allow for the first option with way less guarding around its
  official drivers, allowing for both customized implementations of
  existing drivers and new ones, either by absolute or PATH search". Two
  follow-ups: a name that is both bundled and on `PATH`: "PATH wins";
  which guards stay: "Only: no path in a project file". *Folded in:*
  Decision 5 and **Discovery and trust** are rewritten. The order is a
  path, then `PATH` in `PATH`'s own order, then the bundled copy (a file
  beside `warden`, then a linked driver, which is last so that a file
  always beats code compiled into `warden`). `warden`'s own directory is
  usually on `PATH`, so a customized copy wins when it is earlier on
  `PATH` than that directory; the bundled file is known by identity and
  reported `beside-warden` however it was reached. The owner, mode and
  directory checks, the rule that a bundled name never comes from `PATH`
  and the rule that a project file selects only a bundled driver are
  gone. Kept: a project's `warden.toml` names a driver and never a file;
  this is about the driver's file only, and `[runtime] cli`, which a
  project's file can already point at a program, is left as it is and
  filed as a follow-up. Kept, each said to be for correctness or a
  choice about meaning and not a guard: a named file must be one that
  can be run and must be called `warden-driver-<name>`; a relative path
  in the user's config file is refused (new with this answer, since the
  redraft let no config file name a path); a directory or a file
  without execute permission is passed over, as in a shell, and shown as
  skipped; and on Windows only a `.exe` is a candidate. One more is kept
  and is trust in origin: a driver reached through a relative `PATH`
  entry is an error that names the entry, which is the rule of Go's
  `os/exec` (`exec.ErrDot`), not one this design adds. `warden driver
  list` and `driver.json` show a shadowed bundled copy, a linked one
  included, and the list marks the driver the current flag, variable
  and config select. Also changed: the Security note (his reasoning in
  his words, and what a planted binary can now do), Defaults and
  overrides, the prior-art row on git, Behaviour 4, 12 and the new 16,
  the tier-0 and TEST-08 cases that asserted the old guards, the
  macos-finch and container-any cases, Risks, the Bootstrap comparison
  (BOOT-02's per-driver effect and its security cost, for a file that
  takes a bundled name) and the relay lookup. The umbrella amendment
  for its Decision 5 did not speak of the guards and is unchanged.
- **a21, who reads the configuration: (b), "The kit reads it in the
  driver".** *Folded in:* Decision 7 and **Configuration** are rewritten.
  `warden` reads its own flags and keys and hands a driver the config
  files' paths in precedence order, each with its level (project or
  user), the driver's flags and the environment; the kit reads them in
  the driver's process with `warden`'s precedence, and in `warden`'s
  process for the four linked drivers through the same function
  (`Resolve`), so there is one reader. `describe` declares a driver's
  flags with their arity, for parsing and help (Decision 22), and its
  keys, with a type and the commands that read each, for the placement
  rule and the suite; `options` leaves `describe` and every request,
  which carry `config_files` and `args` in its place; the build record
  keeps those two. `warden` finds the driver before it parses, by a
  scan of the raw arguments that the parse must then agree with. In
  `reap` and `attach` the state file is authoritative, so a recorded
  config file that has gone cannot stop a dead build from being reaped.
  The requirements on a driver written without the kit are numbered C1
  to C10, with what the suite can and cannot check. Also changed: the
  kit's API, the suite's static tier and the fake, the errors table,
  help, Behaviour 1, 2, 3 and 13, What guards behaviour, Risks (the
  costs, stated), Defaults and overrides, the Security note, the
  overview, the ORCH-03 rule change (now a paragraph and a clause of
  wording as well as enforcement), the validation and implementation
  plans, and the front matter. The seven driver-owned fields leave
  `Config`.
- **a20, time limits: (c), with the 60 s teardown wait made a key.**
  *Folded in:* Decision 21. The wait is `warden`'s key `[build]
  teardown_timeout`, a Go duration from `1s` to `1h`, `60s` when unset,
  checked as the files load, with no flag (two of UX-07's three levels,
  as `[vm] dns` has). It is `warden`'s and not the driver's because it
  is the caller's wait and bounds a driver that has stopped answering.
  The `reap` that `warden` runs after a kill takes the same wait. Every
  other limit stays fixed. Also changed: the signals text, the failure
  table, Defaults and overrides, Behaviour 8 and the new 17, and the
  tier-0 cases.

Before it was put back to Jeff, the folded-in text was read by two
independent reviewers, one against the code and one against the three
answers. What their findings changed is in the bullets above. The ones
that are new statements and not corrections: the literal `PATH` order
and the identity of the bundled file (the usual install has `warden`'s
directory on `PATH`); the error for a relative `PATH` entry, where the
text had a silent skip; the K1 exceptions for a linked name (`warden
hyperv service ...` stays `warden`'s command, a linked driver lists no
`reap`, and its doctor, setup, image and clean code calls `Resolve` and
is not dispatched through the kit); the scan for `--driver` and its
check against the parse; a switch reaching a driver as `--name` only;
empty values counting as not set, as today; `warden clean` probing the
driver's `clean` before it reaps; a key's `type` and `commands` in
`describe`; TOML v1.1.0 named as the version of the config files; each
config file's level; the state file's authority in `reap` and `attach`
(C10); the same wait for the `reap` after a kill; and one rule for
`--help` when the driver's part cannot be had. Not fixed here, because
this step may not edit them: the notes of the four `f6-validate` queue
steps whose cases changed, which are rewritten from the Validation
plan's table with the queue's next write.

Still to come in this sitting: a5 (the behaviour that changes, now
seventeen items), a18 (the umbrella's wording, sentence by sentence), a8
(the rule changes, each by name, and the Bootstrap comparison), a4 and
a9 (both derived again after a21's answer), and a24, which a21's answer
raised (how `warden` tells a driver's flags from its own); then the
outcome.

### 2026-11-14, second sitting, continued: a24, a9 and a4 answered; a4 folded in

Read at `6d73ac2`. The sitting is still not finished and has no outcome.
Jeff answered three more of the assumed questions, which makes seventeen
of twenty answered; this fold-in adds a twenty-first. No rule change and
no amendment to the umbrella has been put to him yet.

Two answers confirm the recommendation and change no text but their own
status:

- a24 (a): `describe` declares each flag's name, arity, commands and
  usage, for parsing and help only; `warden` refuses a flag nobody
  declares (Decision 22).
- a9 (a): the shared VM settings are `warden`'s for K1, sent in
  `start.vm`; considered again in K2 (Decision 8).

One answer departs from the redraft and is folded in:

- **a4, where a driver's configuration lives: changed.** The redraft
  recommended `[driver.<name>]` in any shape, with the seven existing
  keys left where they are. Jeff: "A small subset of common keys under
  driver with overrides and custom options under driver.\<name>".
  *Folded in:* Decision 9 and **Configuration**, *Where a driver's
  configuration lives*, are rewritten. `[driver]` holds the common
  keys, which the contract's specification names with their type and
  meaning, and a driver lists in `describe` the ones it honours.
  Version 1 names two, `image_refresh_after` and `image_min_grace`,
  the only keys two bundled drivers read today with one meaning; the
  candidates left off, and why, are in a table. `[driver.<name>]`
  holds that driver's overrides of the common keys and its own keys.
  The shared VM settings stay `warden`'s in `[vm]` (a9), and the
  section says how `[driver]` would relate to them if K2 answers a9
  otherwise. *Decided here, and to be confirmed with the rest:* in one
  file the driver's own table is read before `[driver]`, and the order
  of the files is ORCH-03's, unchanged; the other order, the more
  specific table across files, is rejected because the user's file
  would then outrank the project's for one setting. Two worked
  examples with both files show the difference. *Asked, because his
  sentence does not settle it:* what becomes of the seven existing
  keys (a25, new). The design is written on its recommended answer:
  all seven take their places in the two tables and their old names
  are read as deprecated aliases, with a warning, a refusal of a file
  that sets both names, and a window of at least two releases. Also
  changed: `describe`'s `keys`, the kit's `Setting` (`Common`,
  `Aliases`), `Resolve` and `Settings`, the settings table, the
  requirements C4, C6 and C8 and a new C11, the errors table, the
  suite's static tier and the fake, Behaviour 13 and a new 18, Defaults
  and overrides, ORCH-03's proposed wording (its last sentence
  followed a4) and what the surface check can see of a rename, the
  Security note, Risks, the validation and implementation plans, the
  overview, the front matter, and a5 and a8 where they list what
  changed.

The fold-in was read by one independent reviewer, against the code and
against the three answers. What the findings changed:

- *One warning per command.* `warden build`, `shell` and `clean` start
  the driver twice, for a probe and for the command, and the driver
  resolves its settings both times, so "whoever called `Resolve` prints
  the warnings" printed each old name twice. One rule now, in C11, the
  kit and the errors table: a driver prints them when it answers a
  probe, and in a request that is not a probe only for a command
  `warden` does not probe first; a `run` or `clean` that is not a
  probe prints none. The kit's tests gain the count.
- *An unlisted common key is ignored, and the suite checks it.* C5's
  file now sets every common key the driver does not list, in
  `[driver]` and in the driver's own table, with a wrong type, and the
  probe must end as it does with no file. The fake honours one common
  key and not both so that the check has a key to set against it, and
  has a fault switch for it.
- *The list of common keys and the contract version.* While version 1
  is unstable its list may change with it, as K2 may do; once a
  version is stable, a new common key is a new version (Decision 9,
  **The commands**, **Configuration**, Risks).
- Smaller: `warden` "reads no key in `[driver]` and adds none of its
  own" where the text said it defines none, since it holds the
  contract's names to check a `describe`; the fake's precedence cases
  are spelled out, since it has one name and one common key; Cross-driver
  impact says which keys are renamed and which are common for each
  driver; and a25 says that its five options are about the seven keys
  only, and asks for the list of common keys and the order of the two
  tables each by itself.

Also recorded: the `[runtime] cli` finding (a project's file can name a
program to run) was filed on Jeff's word as its own security fix, queue
item `runtime-cli-project-path`. K1 does not carry it; K1 renames the
key it is about, so the design says that whichever change merges second
applies the refusal under both names.

Still to come in this sitting: a5 (the behaviour that changes, now
eighteen items), a18 (the umbrella's wording, sentence by sentence), a8
(the rule changes, each by name, and the Bootstrap comparison) and a25
(the seven existing keys); then the outcome.

### 2026-11-15: `main` at `ad4ea50` merged; what changed after Jeff read `5808805`

No sitting, and no outcome. The review is still in progress: a25, a5, a8
and a18 are open, no rule change is approved and no amendment to the
umbrella is approved. `loop-contract` merged on 2026-11-15 (`ad4ea50`,
#119) with the Loop Contract (LOOP-01 to LOOP-09), this repository's
profile (PROFILE-01), FLOW-01, FLOW-03 and FLOW-14 amended, a
**Specification changes** section in the template, and the umbrella
amended in place. `main` was merged into the branch (`f91aef3`), and the
design was brought into line with it. Everything below is new since
`5808805`, the commit Jeff read. No recorded answer is touched.

Rule changes: the list is still ten entries, with one out and one in.

- **FLOW-02: the entry is withdrawn.** It is a loop rule, and FLOW-14 and
  LOOP-09 as merged let no feature's design declare a change to one. The
  section that held its current and proposed wording now says what K1
  leaves untrue in FLOW-02 (two bullets name `driver.Driver`), for how
  long, and that the rewording is asked of `loop-parallel`, whose review
  is under way, with where the two bullets go if that review ends first.
  Nothing in `process.md` is edited by K1. Put to Jeff inside a8, with
  three other ways to do it.
- **PROFILE-01: a new entry**, `{rule: PROFILE-01, change: amend, paths:
  [docs/development/loop-profile.md]}`. Four rows of the profile:
  Specifications, Reference documents and the tier-1 Safety row of
  Checks, which K1's PR would leave stale; and Impact vocabulary, which
  gains one sentence on what `interface` stands for. Current and
  proposed wording are under **Rule changes**. Sweep, Consumables, the
  Specification row of Checks and the other thirteen entries are not
  changed, and the section says why. `seam-specs` changes the
  Specifications row too; the section says which words are whose and
  gives the row with both.
- **The other nine entries** were checked against FLOW-14 and LOOP-09:
  all are this repository's rules. Their wording, current and proposed,
  is as Jeff read it. Every "current wording" was compared again with
  `main` at `ad4ea50` and none needed a change: the merge did not touch
  `global.md`, `orchestrator.md`, `testing.md`, `bootstrap.md`,
  `ux-cli-config.md`, `tools/tier1/` or `.github/`.
- **a8 is reworded** in the design, and the queue file's a8 is edited
  to match: its first option lists PROFILE-01 where it listed FLOW-02,
  and it asks for FLOW-02's route beside the letter. Its options (a) to
  (d) are otherwise as they were. The workflow branch carries the first
  rewording at `f090401`. The corrections of the pass below are an edit
  to the queue file that the review session commits; this branch's
  agent does not commit to the workflow branch.

The umbrella amendments (a18):

- Every "Current" cell was compared again with the umbrella at
  `ad4ea50`, which `loop-contract` amended in place. All 34 quotations
  still match. The line numbers were renumbered; six that were one
  line off are corrected, and the wave-3 row gains its number.
- The dated note moves: it was to go under the umbrella's Decisions
  heading, and is now an entry in the umbrella's new **Amendments**
  section, after `loop-contract`'s and in its form.
- In "Rule table, new rows", the FLOW-02 row is replaced by a PROFILE-01
  row.
- One difference from `loop-contract`'s amendments is pointed out for
  a18 and not changed: those kept the approved sentence of Decision 2 and
  added to it, while these replace the sentences of Decisions 4 and 5.

Also added or changed:

- A **Specification changes** section (LOOP-02), before the Bootstrap
  comparison. Its table restates the design. Its last paragraph is new:
  every numbered requirement is covered by a suite case that names it or
  is listed as not machine-checked (LOOP-03). Tasks 12, 14 and 23 of
  the Implementation plan carry it, task 23 with a test that fails on a
  requirement nothing covers.
- **The specification is written first.** LOOP-02 has the specification
  change before the code that implements it. **Order of the work** had
  the specification at step 5, after `driver/host.go` and `buildseq.go`
  (step 3), which the `seam-contract` entry lists as code of the seam.
  It moves to step 1, beside the characterization tests; step 5 keeps
  the rest of its work, no step is renumbered, and task 12 of the
  Implementation plan says so. Nobody asked for this change by name; it
  follows from LOOP-02, and it is in a commit of its own so that it can
  be taken back alone.
- One line under **Design**: the item works only in this repository
  (LOOP-04).
- The sentence on the dead-code sweep says that the kept list is
  PROFILE-01's Sweep entry, and that widening it in K2 is a change to
  that entry.
- Front matter: `rule_changes` as above; `rules` gains LOOP-01 to
  LOOP-04, LOOP-08, LOOP-09 and PROFILE-01; `related` gains
  `loop-contract.md`. Prior art gains one entry for it. Implementation
  plan, task 24, names `loop-profile.md` where it named `process.md`.

**Corrected the same day, after an independent review of this update**
(read at `bb4d7c8`). Each finding was checked against its source, and
the list above is written as the design now stands. What the pass
changed from `bb4d7c8`:

- *`loop-parallel`'s review is not still to come.* The FLOW-02 section
  and a8 said it was. It began on 2026-11-15 and 12 of its 14 questions
  are answered (workflow `f5b7acc`); no outcome is recorded. Both places
  now say so, say that the request is put to Jeff in that sitting and
  does not wait for a8, and say where the two bullets go if that review
  is approved first: `loop-native-lead` or an item of their own, and
  `loop-parallel` is not reopened. `loop-parallel`'s design is cited at
  `6d432f9`, where its FLOW-02 change is the same as at `f706bf7`.
- *PROFILE-01 is four rows, not five.* The tier-1 Specification row of
  Checks is withdrawn from the change: it points to TEST-05, and the
  TEST-05 change names the suite, so K1 does not leave it stale. The
  other four rows are word for word as at `bb4d7c8`. a8, the section
  and task 24 say four.
- *`seam-specs` declares a PROFILE-01 change; it does not owe one.* Its
  `1023b02` was committed half a minute before this design's `a280ba9`.
  The section's last part is rewritten for it (read at `e4801d6`): which
  sentences of the Specifications row each design owns, that row with
  both changes, and a request under a8 to approve it in both forms.
  The sentences on "No specifications release is pinned today" and on
  "Consumes: nothing" now say they are true of K1 alone. The sentence on
  `loop-parallel`'s PROFILE-01 change is corrected to what it is at
  `6d432f9`, one added row.
- *The LOOP-03 sentence had no task.* Tasks 12, 14 and 23 gain it.
- *a8 names the shared list.* The ORCH-01 entry adds eight paths to
  `drivers.shared`, which the `drivers-claim` check reads for FLOW-02 as
  well as for ORCH-01. **Rule changes** already said so; a8 now does.

Not changed by the pass: `rule_changes` (ten entries, PROFILE-01's path
the same), the other nine entries, the umbrella amendments, any option
(a) to (d) of a8, any other question.

Not changed: `seams`, `drivers`, `surfaces`, `security_boundary`,
`alters_relationships`, `native_validation` and `tier2_gaps`; every
decision; every answered question; the Bootstrap comparison.

Still to come: a25, a5, a18 and a8, each rule change by name and the
Bootstrap comparison; then the outcome.

### 2026-11-15, second sitting, continued: a25 and a18 answered; one config file folded in

The sitting is still not finished and has no outcome. No rule change and
no amendment to the umbrella is approved. Read at `8f61932` for this
fold-in; Jeff last read the design at `5808805`.

Two answers, both as recommended:

- a25 (a): all seven existing driver-owned keys move into `[driver]` and
  `[driver.<name>]`, and each old name is read as a deprecated alias
  that warns. Asked beside the letter, the common keys of version 1 are
  the two image ages and no others: confirmed.
- a18 (a): the amendments to the umbrella are made in K1's PR. The list
  is shown to Jeff again before the design is approved.

One thing asked beside a25's letter was not confirmed, and is folded in:

- **The order between the two tables across two config files.** The
  design had the project's `warden.toml` before the user's
  `~/.config/warden/config.toml`, each resolved by itself, in seven
  steps. Jeff: "What do you mean by 'yours'? warden.toml is meant to be
  effectively in full control by a build executor and there shouldn't be
  any precedence necessary. Configuration should only be through a local
  'warden.toml', environment variables, and command-line arguments. By
  default, the warden.toml's location in the working directory is simply
  for ease of use, but there should only be one authoritative
  warden.toml for any build". Asked four things, each on its own, he
  chose: the user-level file goes away; the one file is `./warden.toml`
  by default, or the file named by a new `--config` flag or
  `WARDEN_CONFIG` variable, which is then the only one read; the queue
  item `project-config-trust` carries the change (the code, the ORCH-03
  and UX-07 amendments, the docs), and this design and
  `runtime-cli-project-path` are written to fit it; and an existing
  `~/.config/warden/config.toml` is an error that names it, a detection
  removed later.

*Folded in.* Decision 23 is new. The contract hands a driver one
`config_file`, a string, absent when there is none, where it handed a
list `config_files` with a `level` on each entry: in the requests of
every command, the build record, `reap` and `attach`, requirements C3,
C6, C8 and C10, the kit's `Resolve` (the `ConfigFile` type and
`Settings.Level` go), the fake and the suite. The precedence is a
setting's flag, its environment variable, `[driver.<name>]`, `[driver]`,
the driver's default: five steps and one worked example, where there
were seven and two. Also changed for it: Decisions 5, 7, 9, 18 and 21;
**Discovery and trust**; the scan that finds the driver, which now finds
`--config` too; the teardown wait's table; Behaviour 4 and 12; Risks;
the overview; Defaults and overrides; ORCH-03's proposed paragraph,
which loses its words about files; the Security note; the validation and
implementation plans; and a5 and a8, both reworded, in the queue file as
well. A new section, **What K1 takes from the two configuration items**,
gathers what follows.

Decided in the fold-in, and to be confirmed with the rest:

- *K1 waits for `project-config-trust`.* K1 builds none of that item's
  part, so its Plan and Implement start after that item has merged. The
  queue holds it by a blocking question that names the merge and that
  Jeff answers when he has merged that PR (`q1`, with the step
  `f2-answer-q1` that `f2-plan` needs); it is written into the queue
  file during this review, before the outcome is recorded. K1 does not
  merge first, and an Implement that finds `main` still reading two
  files stops.
- *No level, and no field for how the file was found.* Rejected: a list
  of one, a level on the one file. The key the level was carried for is
  settled another way (next point).
- *The container driver's runtime.* Jeff decided in
  `runtime-cli-project-path`'s review that it is limited to `finch`,
  `docker` and `podman` from every source. That item merges first; K1
  moves the check into the driver with the setting and keeps it under
  both names of the key. The design said before that K1 does not carry
  the fix and that whichever change merges second applies it; that is
  overtaken.
- *The general rule on configuration* that Jeff asked for in the same
  review, written there as an amendment to UX-08 and approved by name at
  `a2afd08`: K1's drivers already validate their own settings before
  work and only when selected. One paragraph says so.
- *ORCH-03.* `project-config-trust` owns the rule's opening sentences,
  on the one file and the precedence; K1 owns its new paragraph and the
  clause on its last sentence. K1 is the second to merge and adds its
  two parts to the rule as it finds it. The rule with both changes is
  quoted, and the review is asked to approve K1's parts in both
  settings.

*Asked, as a26, in two parts.* Whether the seven old key names keep
working with a warning, as a25 was answered, or become an error at once
that names the new key, as the two changes Jeff decided on the same day
are; recommended: as answered. And what the one guard kept under a16 is,
now that there is no user's own config: no config file names a driver by
path (recommended, and what the design is written on), or a path is
accepted in a file the executor named with `--config`, or no guard.
a25's and a16's recorded answers are not touched.

Not settled by this fold-in, and said where it arises:

- Of the two designs K1 now leans on, one is approved and neither is
  merged. Their commits appeared while this fold-in was being written,
  and it was brought into line with them. `runtime-cli-project-path` has
  its approval, at `a2afd08` (below). `project-config-trust`, read at
  `539fd38`, is still in Design. What K1 takes from each is in the new
  section. `project-config-trust` proposes that a config file found in
  the working directory may decide what a named file decides (its
  Decision 7, its a4, not confirmed); if that changes, the drivers that
  read `relay_image` and `windows_image` would have to be told how their
  file was found, and the requests would gain a field.
- `project-config-trust` says of K1's hold that it is raised when K1's
  Plan is done; this design raises it before Plan, and says why.

*After an independent review of the fold-in* (it read `834c449`), the
same day:

- **An eleventh rule change, UX-08.** The fold-in held it back because
  the wording it edits was not approved. Seven minutes after `834c449`,
  `runtime-cli-project-path`'s review approved that wording by name at
  `a2afd08`, with its list of four older checks kept. K1 moves two of
  the four to the drivers, and that item's hand-off gives K1 the edit to
  declare. Added: the `rule_changes` entry (one path,
  `docs/development/ux-cli-config.md`), the heading **UX-08** under
  **Rule changes** with the approved paragraph and K1's, and a third
  rewording of a8, in the queue file as well. The same item's a5 was
  answered (a): it moves none of the four, and K1 still leaves `[vm]
  build_link` and `[vm] dns` where they are (a9, answered).
  `runtime-cli-project-path` is now cited as approved wherever it was
  cited as proposed; `project-config-trust` is still cited as proposed.
- **a26.** That item's transitional detection is kept until 1.0 (its a4,
  answered (d)). Option (ii) now says so, option (i) notes that "at
  least two releases" is a floor, and the UX-03 change (ii) would need
  is a twelfth rule change. In the queue file as well.
- **The hold.** The design said the hold on Plan would be written into
  the queue file when this review's outcome is recorded. Nothing in
  recording an outcome writes it, and without it the next tick takes
  `f2-plan` (shown on a copy of the queue state). It is now to be
  written during the review, as `q1` with the step `f2-answer-q1`.
  **It is not in the queue file at this commit**: the fold-in could add
  one question there, which was a26, so writing it is the attended
  session's. The reason given for raising it before Plan that spoke of
  FLOW-05's "one" `alters_relationships` slot is corrected: the limit is
  three.
- **The contract's requirements.** C6 now covers a `config_file` that
  was named and does not exist, outside `reap` and `attach`: it is
  refused, and not read as if none had been named. With one named file
  and commands on which `warden` does not open it, that case can reach a
  driver, and the requirements did not state it. The Errors table, the
  comment on `Resolve`, the suite's static tier and the kit's tests
  follow. C8's order for a driver written without the kit gains the
  environment variable's step, which the five-step order and ORCH-03
  already had.
- Six places that still said "the files" say "the config file".
- Two sentences of `project-config-trust`'s design that do not hold of
  K1 (the build record as the place a build's config file is recorded;
  `config_files` with one entry) are listed under **What K1 takes from
  the two configuration items**, for the attended session to hand to
  that item's Design.

Not changed: the first ten `rule_changes` entries and their
wording, the umbrella amendments, `seams`, `drivers`, `surfaces`,
`security_boundary`, `alters_relationships`, `design_review`,
`native_validation`, `tier2_gaps`, the native cases and the queue's
`f6-validate` notes, the Bootstrap comparison, and every recorded
answer. The front matter gains one concept, `single-config-file`, and
the eleventh rule change.

Still to come: a5, a8 with each of the eleven rule changes by name and
the Bootstrap comparison, a26, the umbrella's list once more (a18), the
hold written into the queue file; then the outcome.

### 2026-11-15, second sitting, continued: a5, a26 and a8 answered; eleven rule changes approved by name

This entry records three answers and the rule changes Jeff approved by
name. It is not the review's outcome, and the design is not approved by
it: the umbrella's list shown once more (a18), the Security note's
acceptance and the outcome are still to come in this sitting and are
recorded when they are given. Jeff read the design at `3f2012a`.

Three answers, each the recommended option, on which the design was
already written:

- a5 (a): all eighteen changes of behaviour are accepted.
- a26, in its two parts. The old names, (i): each old name works and
  warns, and is kept until 1.0 (the option as put read "Alias and warn
  until 1.0"); a file that sets both names is refused. The guard, (A):
  no config file names a driver by path; a path comes only from
  `--driver` or `WARDEN_DRIVER`.
- a8 (a): all eleven rule changes. And route (i) for FLOW-02: K1
  changes nothing in FLOW-02. In `loop-parallel`'s design review the
  same day Jeff answered K1's request: "Yes, carry it here". That
  item's FLOW-02 amendment carries the two bullets (its design at
  `07c2993`, which is not approved yet).

The eleven rule changes, each approved by name on the wording the
design had when Jeff read it:

- GLOBAL-08.
- ORCH-01.
- ORCH-02.
- ORCH-04.
- TEST-05.
- BOOT-02, with the Bootstrap comparison accepted explicitly (BOOT-07).
- UX-01, in enforcement only.
- UX-03, in enforcement only.
- ORCH-03: K1's paragraph and clause, in both settings, after `main`'s
  first paragraph and after `project-config-trust`'s.
- PROFILE-01: four rows, the Specifications row in both forms, K1's
  alone and joined with `seam-specs`' sentences.
- UX-08: its last paragraph only.

The line FLOW-19 reads for each rule change, and for the design, is
written with the outcome and not here.

*Recorded in the design* (a clerical pass; no decision, requirement of
the contract or rule wording changes, and every proposed wording under
**Rule changes**, the Bootstrap comparison and the Security note are as
Jeff read them):

- a5, a26 and a8 carry their answers under **Questions**, and the
  places that treated them as open or gave what an unchosen option
  would do now say what was answered: the a16 row of the table at the
  head of the design ("Where the redraft was narrower"), Decisions 5
  and 9, **Discovery and trust**, **Configuration**, **Behaviour** (its
  head, 2, 18 and its closing paragraph), **What K1 takes from the two
  configuration items**, **Defaults and overrides**, the head of **Rule
  changes**, ORCH-03's commentary, UX-08's last note, and task 26 of
  the Implementation plan. a16 and a25 each carry a dated note pointing
  to a26's answer.
- The old names' window is "until 1.0" where the design said "at least
  two releases" or "a stated window" (a26's answer, within a25's).
  Nothing else is said of the removal: "Taking them out is a change of
  its own, with its own design" stands as Jeff read it. How that
  removal is kept track of until 1.0 is not stated by this design. The
  first state of this pass (`f830755`) gave the seven names
  `runtime-cli-project-path`'s convention for code with a removal
  point, a `TRANSITIONAL(<name>)` marker and a removal item filed at
  Publish on hold until 1.0; that was more than the answer says and
  Jeff had not read it, so it was taken out again. It is Plan's to
  raise, or Jeff's to ask for.
- The hold is in the queue file: `q1`, blocking and open, with the step
  `f2-answer-q1` before `f2-plan`, written by the attended session at
  workflow `ea23718`. The places that said it was still to be written
  are corrected.
- FLOW-02's section under **Rule changes** says where the two bullets
  now are. Its fallback paragraph is rewritten, and this is a case
  Jeff has not read: the case it provided for (that item's review
  approved before the request reached it) did not arise, and the
  paragraph now provides for the one still possible (that review ends
  without approving the FLOW-02 amendment with the two bullets in it).
  The two destinations are the ones he read, `loop-native-lead` or a
  process item of their own, and the choice is still his, made then.
- One sentence of the previous entry was reworded ("was approved at" to
  "has its approval, at", for `runtime-cli-project-path` at `a2afd08`)
  so that FLOW-19's check does not read it as an approval of this
  design; its meaning is unchanged.
- Three sentences still say what the review "is asked" or "must" do
  for a8 (the Bootstrap comparison's first paragraph, and the last
  paragraphs on ORCH-03's two settings and on the Specifications row's
  two forms). They are left as Jeff read them; the list above is the
  record of what he approved.

Still to come: the umbrella's list once more (a18); the Security note's
acceptance; then the outcome.

### 2026-11-15, second sitting, outcome: approved

- **r2, 2026-11-15: approved at `a6c8f0b`** (Jeff; read at `5808805` and,
  with one config file folded in, at `3f2012a`; page
  https://claude.ai/artifact/AGW8qkxVncdMhjna1bPsGH). `a6c8f0b` records
  the last answers and changes no decision, contract clause or rule
  wording since `3f2012a` (the entry above; checked by an independent
  reviewer). Every question is answered: a5 (a), a26 (i) and (A), and a8
  (a) with (i) for FLOW-02, in this sitting; the others as the entries
  above record. Asked in the sitting and answered by him: the amendments
  to the umbrella design were put to him once more, as a18's answer
  required, and "they stand"; the date the umbrella's **Amendments**
  entry and its Archive summary carry is 2026-11-15. The Bootstrap
  comparison is accepted explicitly (BOOT-07), with BOOT-02. The Security
  note is accepted (GLOBAL-06). Told to him with the request for the
  outcome: the removal of the seven old key names at 1.0 is not tracked
  by this design and falls to Plan; and the FLOW-02 fallback paragraph
  covers a case he had not read. Plan stays held by `q1` until
  `project-config-trust`'s PR has merged (Decision 23).
- **rule change GLOBAL-08 approved at `a6c8f0b`**, as worded under **Rule
  changes**.
- **rule change ORCH-01 approved at `a6c8f0b`**, as worded there.
- **rule change ORCH-02 approved at `a6c8f0b`**, as worded there.
- **rule change ORCH-04 approved at `a6c8f0b`**, as worded there.
- **rule change TEST-05 approved at `a6c8f0b`**, as worded there.
- **rule change BOOT-02 approved at `a6c8f0b`**, as worded there, with
  the Bootstrap comparison.
- **rule change UX-01 approved at `a6c8f0b`**: enforcement only.
- **rule change UX-03 approved at `a6c8f0b`**: enforcement only.
- **rule change ORCH-03 approved at `a6c8f0b`**: K1's paragraph and
  clause, in both settings (after `main`'s first paragraph, and after
  `project-config-trust`'s).
- **rule change PROFILE-01 approved at `a6c8f0b`**: four rows, the
  Specifications row in both forms (K1's alone, and joined with
  `seam-specs`' sentences).
- **rule change UX-08 approved at `a6c8f0b`**: the last paragraph only.

## Consolidation

## Rollback notes

## Archive summary

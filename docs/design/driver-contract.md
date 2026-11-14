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
                         # [driver.<name>], read by the driver; [build] teardown_timeout;
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
  - {rule: ORCH-03, change: amend, paths: [docs/development/orchestrator.md,
      tools/tier1/surface/surface.go, tools/tier1/surface/surface_test.go,
      tools/tier1/surface/testdata/driver.go.txt]}
concepts: [driver-contract, driver-binaries, driver-commands, driver-discovery, driver-conformance,
           driver-options, driver-owned-config, driver-config-reading, teardown-timeout,
           kit, drivercmd, build-sequence, session, claim,
           collector, collector-handoff, relay-source, relay-release-forms, reap, build-record,
           build-lock, doctor,
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
| A driver is found "in the same way git finds its individual subcommands (i.e. existing on PATH)" | `warden`'s own directory is searched before `PATH`, the five bundled names never come from `PATH`, and a project's `warden.toml` may select only a bundled driver | a16. **Answered otherwise:** "overcontrolled". `PATH` is searched before what `warden` bundles, so a file on `PATH` may replace a bundled driver; nothing checks owner or mode; one rule is kept, that a project's `warden.toml` names a driver and not a file |
| "each driver effectively being its own independent binary" | Only the container driver is a binary in K1. The four VM drivers move onto `warden`'s build sequence and stay linked into `warden` until K2 | a15 |
| "a shared library [...] that helps mediate pieces like configuration-parsing/reading" | `warden` reads all configuration and hands a driver resolved values. The kit's part is the option declaration and typed access to the values; it reads no file | a21. **Answered otherwise:** (b). The kit reads the configuration inside the driver's process, and `warden` passes on the driver's flags and the paths of the config files |
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
name anywhere else is the user's. The **exec transport** is `warden` running a
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
   project's `warden.toml` may name a driver but not a file.** A path
   comes from `--driver`, `WARDEN_DRIVER` or the user's own config file,
   and is used exclusively. Otherwise `warden-driver-<name>` is searched
   on `PATH`, and only when `PATH` has none does `warden` take the copy it
   bundles: a file of that name in its own directory, else a driver linked
   into it. So a `warden-driver-container` on `PATH` replaces the bundled
   one, and customizing an official driver needs no flag. Nothing checks
   who owns a driver file or who may write it. The one rule kept is that
   the project's `warden.toml` cannot name a path, so a cloned repository
   cannot make `warden` run a binary it ships. Alternatives: the
   redraft's guarded lookup (`warden`'s own directory before `PATH`, the
   five bundled names never from `PATH`, a project file limited to bundled
   drivers, owner and mode checks on Unix), which the review rejected as
   "overcontrolled": the lookup is a convenience for the user, and it "is
   never outside of the user's control (i.e. never a part of the build
   environment)"; and what `warden` bundles before `PATH` (a customized
   bundled driver would then need `--driver <path>` on every command;
   asked as a follow-up and answered "PATH wins") (a16).
6. **No auto-detection of the driver.** None exists today and K1 adds
   none. Selection is unchanged: `--driver`, `WARDEN_DRIVER`, `[runtime]
   driver`, else `container`. A named driver that is not found is an
   error, never the default (GLOBAL-10). The umbrella's Decision 5 gave
   "driver auto-detection" as a reason; the amendment drops the phrase
   (a16, a18).
7. **A driver reads its own configuration, in its own process; the kit
   does it for a driver built on it.** `warden` reads its own flags and
   keys and hands a driver three things: the paths of the config files it
   found, in precedence order; the driver's flags from the command line;
   and its environment. The kit reads the same files with the precedence
   `warden` uses (ORCH-03) and gives the driver typed values, validation
   and the error form; a driver written without the kit follows the
   contract's requirements for the same. This is a21's answer (b), "The
   kit reads it in the driver", and it is what Jeff's first note asked
   for: "a shared library [...] that helps mediate pieces like
   configuration-parsing/reading". Its costs are accepted with it:
   `warden` cannot check a driver's value or key before the driver's
   probe, help depends on the driver, tier 1 reads the surface only of
   the bundled drivers, and a driver written without the kit
   re-implements the precedence and, if it has keys, needs a TOML
   parser. Alternatives: `warden` reads everything and hands the driver
   resolved values (the redraft's recommendation, not taken in the
   review); `warden` parses the files and hands each driver its own
   table as JSON (a driver would need no TOML parser, but the seven
   existing keys sit in `warden`'s tables, so `warden` would again have
   to know which key is whose, and it is not what was answered) (a21).
8. **The shared VM settings stay `warden`'s, and `capabilities.vm` is the
   one switch for them.** The VM drivers take `--script`, `--image`,
   `--cpus`, `--memory`, `--keep-vms`, `--build-link`, `[vm] cpus`,
   `memory_mb`, `build_link` and `dns` as a group today. They have one
   meaning for every VM driver, `warden` checks the link and the
   resolvers as the files load, and it needs three of them itself
   (`--script` to stage, `--guest-os` for the bootstrap config,
   `--keep-vms` for `keep`). So they stay `warden`'s flags and keys,
   sent in `start.vm` to a driver that declares `vm`. Alternative, which
   is a21's answer followed to its end: each VM driver reads `--image`,
   `--cpus`, `--memory`, `--build-link` and the four keys itself, through
   a helper in the kit. `warden` would then know nothing of a VM's size
   or link, but `[vm]` would become a table several drivers read, the
   link would be checked in each driver's probe and no longer as the
   files load, and the refusal of a VM flag on the container driver would
   become the unknown-flag refusal. Open (a9).
9. **Driver-owned configuration: a driver's keys live in
   `[driver.<name>]`, where they may take any shape; the seven existing
   keys stay where they are, each read by the driver that owns it.**
   UX-03 forbids moving the seven without aliases, and K1 changes no
   key. Now that a driver reads the files itself, nothing physical stops
   it reading any table, so the place is a rule of the contract: a
   driver lists its keys in `describe`, and `warden`, the suite and the
   kit each refuse a listed key outside the driver's own table.
   Alternatives: move the seven now with deprecated aliases (a
   user-facing change in a freeze); let a driver read any table
   (collisions with `warden`'s tables and between drivers, and nothing
   left to check). Open (a4).
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
    and the config file paths it was started with; the driver may keep a
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
    meant to bound); a flag as well (not added: the wait is a property
    of a host and its drivers, set once, and a flag can be added later
    without breaking anything) (a20).
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
    with a guessed arity (builds the wrong thing silently) (a24).

## Questions

### Blocking

None. The review comes before Plan (FLOW-19), and every question below has
a recommended answer the design is complete under.

### Assumed

The first draft's a1 to a11 were never answered. Four are retired because
the redraft makes them moot (**Retired**, below); seven survive, reworded
where the new contract requires it; a12 to a23 are new. They are in two
groups: first the seven where the draft departs from Jeff's notes or from
the basis, then the rest.

**State after the review's second sitting, 2026-11-14.** Fourteen are
answered, and each carries its answer below, with the option chosen:
a16, a15, a21, a14, a12, a13, a22, a20, a19, a23, a7, a3, a11 and a17.
Eleven were confirmed as recommended. Three were answered otherwise and
are folded into the design: a16 (lookup and trust), a21 (who reads the
configuration) and a20 (the teardown wait). Each question's text and
options are kept as they were put, so "Proceeding with" in an answered
question describes what was asked, not always what the design now does.
Six are open: a5, a18 and a8, and a4 and a9, which a21's answer made it
necessary to derive again, and a24, which that answer raised.

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
  `warden-driver-container` on `PATH` replaces the bundled one and
  customizing an official driver needs no flag; `warden driver list` and
  the build's `driver.json` show which file ran. Which guards stay:
  **"Only: no path in a project file"**, so there is no owner or mode
  check and no bundled-only rule, and a project's `warden.toml` may name
  a driver but not a file; a path comes from `--driver`, `WARDEN_DRIVER`
  or the user's own config. Folded in as Decision 5 and **Discovery and
  trust**: a path, then `PATH`, then the bundled copy (a file beside
  `warden`, then a linked driver).
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

- **a5. The behaviour that changes.** Proceeding with the seventeen
  differences listed under **Behaviour**. In short: (1) flags and help
  follow the selected driver, and the driver's part of help comes from
  its `describe`; (2) a flag that neither `warden` nor the selected
  driver declares is refused, where eleven were silently ignored, and so
  is `--runtime` on a command that selects no driver; (3) the three
  refusals that name another driver (`--network`, `--slots`, `--update`)
  become the one unknown-flag refusal, which names the selected driver
  only; (4) "unknown driver" becomes "driver not found", with
  what was searched, and a project file whose `[runtime] driver` is a
  path is refused; (5) `warden clean --driver qemu|vz` no longer
  cleans containers; (6) `warden clean` first cleans up after dead builds
  of the selected driver; (7) the build-exit grace is 10 s on every
  driver (it was 5 s on container and Hyper-V); (8) a second Ctrl-C kills
  a driver binary and reaps, where it killed `warden`, and on linked qemu
  and firecracker it now ends `warden`, where it was ignored; (9) SIGTERM
  tears the build down on every driver, where it killed `warden` on
  three; (10) the output directory gains `driver.json`, written with
  `inputs/` before the driver starts; (11) `warden driver list` is new;
  (12) a path is accepted in `--driver`, `WARDEN_DRIVER` and the user's
  config; (13) a driver's keys are read only by that driver and only when
  it is selected: a malformed `[vm] image_refresh_after` or
  `image_min_grace`, and a wrong TOML type in any of the seven existing
  driver-owned keys, are refused by the driver that owns the key and
  pass unseen under another, where today the ages are refused on `build`
  and `shell` under every driver and a wrong type fails the load under
  every driver; (14)
  `--timeout` counts from the moment `warden` starts the driver; (15) a
  context or temporary path that is not valid UTF-8 is refused for a
  driver binary; (16) a `warden-driver-<name>` on `PATH` runs in place of
  the bundled driver of that name; (17) `[build] teardown_timeout` is
  new. Re-derived on 2026-11-14 after the answers to a16, a20 and a21:
  none of the redraft's fifteen goes; 1, 3, 4, 8, 10, 12 and 13 change
  (3 loses the hint that names the driver a flag belongs to, and 13
  loses the type check of the seven keys under every driver); 16 and 17
  are new. Options: (a) accept all seventeen; (b) refuse nothing new:
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
  here, and (c) is the answer if the window matters more. Open.
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
- **a8. The rule changes.** The umbrella's table gives K1 GLOBAL-08 and
  ORCH-01 "for direction". Proceeding with those two reworded for a command
  contract, plus ORCH-02 (staging moves to `warden`), ORCH-04 (teardown on
  end of input, and `reap` from the build's record), TEST-05 (the driver
  suite), BOOT-02 (the executor's version is the driver's; with a
  Bootstrap comparison, since the wording differs from the one the
  umbrella's review accepted for K2), FLOW-02 (it names `driver.Driver`,
  which K1 removes), an enforcement-only change to UX-01 and UX-03
  (tier 1's surface check also reads the flags and keys the bundled
  drivers declare), and ORCH-03, which gets the same enforcement change
  and one paragraph: a driver reads its own flags and keys, with the
  same precedence and before any work. GLOBAL-08's sentence on a driver
  from outside was asked on its own and confirmed (a23). Re-derived on
  2026-11-14 after the answer to a21: the list has the same ten entries;
  what differs is that ORCH-03 is now reworded and no longer
  enforcement-only, and that the surface check reads `drivercmd.Setting`
  literals. UX-07 and UX-08 are kept by the design and quoted under
  **Configuration**; neither needs new words. Options: (a) as stated;
  (b) leave BOOT-02 to K2 (the sentence "matches the orchestrator's
  version" is then false for the container driver from K1's merge until
  K2's); (c) no enforcement change, a `Tier1-Allow` trailer on the move,
  and the moved flags no longer checked against the docs; (d) ORCH-03 in
  enforcement only, as the redraft had it: the rule then does not say
  that a driver's own reading keeps the precedence, and only the
  contract does. Recommended: (a). Each rule change is approved by name
  in the review, and the Bootstrap comparison is accepted explicitly
  (BOOT-07). Open.
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
  specification; a driver lists the keys it reads in `describe`, and
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
  aliases either. Recommended: (a). Open.
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
  in any case. Open.
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
  Open.

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

### Shape

```
  warden (cmd/warden)                         a driver: warden-driver-<name>
  -------------------                         ------------------------------
  its own config and flags     -- describe -->  what it is; its flags
  the driver's flags, unread;
  the config files' paths      -- run (probe)>  reads and checks its own
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
rule, on the one input that may not be the user's own.

**Name.** A driver binary is `warden-driver-<name>`. `<name>` matches
`[a-z][a-z0-9]{0,31}` and is not the name of a `warden` command. It has no
hyphen or other punctuation because it is used as a TOML table
(`[driver.<name>]`), inside lock and record file names
(`build-<name>-<id>`) and as the `warden` subcommand that reaches the
driver's own commands. On Windows the file is `warden-driver-<name>.exe`
(below).



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
| `[runtime] driver` in the user's config (`~/.config/warden/config.toml`) | yes | yes, absolute only |
| `[runtime] driver` in the project's `warden.toml` | yes, any driver | **no** |

- **The one rule kept: a project's `warden.toml` may name a driver, not a
  file.** That file is read from the working directory and may come with
  a build context someone else wrote. It may select any driver by name,
  which is then looked up like any other name; a value that is a path is
  refused, so a cloned repository cannot make `warden` run a binary it
  ships. The refusal names the file and gives the three places a path
  may come from as the fix.
- A relative path in the user's config file is refused too. This is
  correctness, not trust: that file is read from every working
  directory, and a relative path would name a different file in each.

`[runtime] cli` is already run as given from a project's `warden.toml`.
K1 does not change that; it is filed as a follow-up (**Risks**).

**Lookup**, first match wins, resolved once per `warden` invocation to an
absolute path, which every later command of that invocation runs:

1. **A path.** The file is used as given and nothing else is searched
   (GLOBAL-10). The driver's name is what its `describe` reports, which
   must match the file's base name (`warden-driver-<name>`, with `.exe`
   on Windows).
2. **`PATH`.** Each absolute entry, in order, is searched for
   `warden-driver-<name>`.
3. **The directory of `warden`'s own executable** (`os.Executable()`,
   symlinks resolved). The container driver's binary ships here, so
   `sudo warden` still finds it under sudo's reset `PATH`; a driver the
   project did not write may be installed here too. K1 adds no separate
   driver directory (`libexec`); whether packagers need one is K2's
   question.
4. **A driver linked into `warden`**, when the name is one (K1: `qemu`,
   `vz`, `hyperv`, `firecracker`).

Steps 3 and 4 are the bundled copy. `PATH` comes before it, so a
`warden-driver-container` on `PATH` runs in place of the one beside
`warden`, and nothing has to be passed to make it so: that is how an
official driver is customized. `warden driver list` and the build's
`driver.json` show which file ran (below).

**Where a linked driver sits.** Last. A file found at step 1, 2 or 3
beats code compiled into `warden`, so in K1 a `warden-driver-qemu` on
`PATH`, or beside `warden`, runs through the exec transport in place of
the linked qemu driver. Every command for that name then goes to that
binary; a command it does not list is not implemented for that name, and
the linked driver's code is not used to fill the gap (GLOBAL-11). Two
reasons for this place in the order: it gives the four VM drivers the
same rule as the container driver ("a file on `PATH` wins"), and K2
changes nothing in it, since each VM driver then becomes a file at step
3 and step 4 goes.

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
after an upgrade that installed only `warden`.

**What is still checked, and why.** Each of these is about whether the
file can be run at all, or run correctly. None is about who put it
there, and nothing compares owners, modes or directory permissions on
any platform.

- *A file named by path* must exist and, after symbolic links are
  resolved, be a regular file the user may execute. Without the check
  the user would get the operating system's bare error from the first
  command; with it the error names the file and what is wrong with it
  (UX-04).
- *On `PATH` and in `warden`'s directory*, a directory or a file without
  execute permission is not a candidate, and the search goes on. This is
  what a shell and Go's `exec.LookPath` do.
- *A relative or empty `PATH` entry*, which means the current directory,
  is skipped. Go's `os/exec` refuses to run a program it resolved through
  such an entry (`exec.ErrDot`, since Go 1.19), and `warden` keeps the
  standard library's rule instead of adding code to defeat it. On Windows
  it likewise does not search the current directory first, which
  `cmd.exe` does and `os/exec` refuses.
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
found, and beneath it every copy the lookup passes over, in lookup
order, the bundled copy among them:

```
NAME         RUNS                                         FOUND
container    /home/ana/bin/warden-driver-container        PATH
               shadows /usr/local/bin/warden-driver-container (beside warden)
qemu         (linked into warden)                         linked
vz           (linked into warden)                         linked
hyperv       (linked into warden)                         linked
firecracker  (linked into warden)                         linked
nomad        /opt/nomad/bin/warden-driver-nomad           PATH
```

So a bundled driver that a file on `PATH` has replaced is the first
thing the list shows about it. The command reads directories only and
runs no driver. With `--json` it prints one object,
`{"drivers":[{"name","path","found","shadowed":[{"path","found"}]}]}`,
where `found` is `PATH`, `beside-warden` or `linked` (UX-02).

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
 "sha256": "9f2c...", "linked": false, "found": "PATH",
 "shadowed": ["/usr/local/bin/warden-driver-container"]}
```

`found` is `path` (the file was named), `PATH`, `beside-warden` or
`linked`, and `shadowed` lists the copies the lookup passed over, so a
build's output says when a bundled driver was replaced and by which
file. For a linked driver `path` and `sha256` are `warden`'s own. The
file is unsigned and is not part of the ledger. The environment record
is not touched: `specs/metadata-environment/v1.schema.json` already
defines `driver` as a string, set by the driver's own environment
description.

**What a planted binary can and cannot do.** A file named
`warden-driver-<name>` in any absolute `PATH` directory runs in place of
the bundled driver of that name, for every command including `--help`,
`doctor` and `clean`, with the user's privileges. It then decides how
the build is isolated, which relay and executor run, and it is given
the claim token. `warden` does not prevent that; it shows it (`driver
list`) and records it (`driver.json`, a19). What a planted file cannot
do: be reached through the current directory or a relative `PATH`
entry, or be named as a file by a project's `warden.toml`. A repository
can therefore choose a driver by name, but can supply the binary only
if the user has put one of the repository's directories on `PATH` as an
absolute entry (**Security note**).

### The commands

The verb is `argv[1]`. `describe` takes no request: it is what tells
`warden` which contract versions the driver speaks. Every other command
gets a request, one JSON object that carries `contract`, the version
`warden` chose, and `config_files` and `args`, from which the driver
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
| `reap <build-id>` | yes | `{contract, build_id, tmpdir, config_files, args, state_file}`, with the values the build was started with (**The build record**) | Result `{removed}`. Removes whatever that build left. Exit 0 means nothing of that build is left, and it may be repeated. When the driver cannot look where the build's leftovers would be (its runtime does not answer, it is on the wrong host), it exits 1 with a `fixable` error; it never reports success for a place it could not look in | 0, 1 |
| `doctor` | no | `{contract, vm, config_files, args, verbose}` | Its report on stdout. Result `{blocked}`. Changes nothing on the host | 0, 1 |
| `setup` | no | `{contract, vm, config_files, args, verbose}` | Its progress. Result `{}` | 0, 1 |
| `image-prepare` | no | `{contract, os, source, config_files, args, verbose}` | Progress on stderr. Result `{path}`; an empty path means the driver already reported the outcome | 0, 1 |
| `image-list` | no | `{contract, config_files}` | One line per prepared image on stdout. Result `{count}` | 0, 1 |
| `image-verify` | no | `{contract, config_files}` | One line per cached layer on stdout. Result `{checked, drift}` | 0, 1 |
| `clean` | no | `{contract, keep_ids, cache, tmpdir, config_files, args}` | `Removing ...` lines on stderr. Result `{removed}`. Removes its leftovers of every build whose ID is not in `keep_ids`, and its own cache when `cache` is true | 0, 1 |
| `attach <build-id>` | no (needs `capabilities.shell`) | `{contract, build_id, config_files, args, state_file}` in the request file, with the values the build was started with | The interactive shell, on the inherited terminal. Result `{}`, or an `error` object when the attach itself failed | The shell's own code |
| `ext <args...>` | no | `{contract, config_files}` in the request file; the arguments in argv, as typed | The driver's own subcommands, listed in `describe.ext` | The driver's own |

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
driver `<name>`'s `ext <args...>`: a first argument that is not a `warden`
command is looked up as a driver name, which is why a driver's name may
not be a `warden` command. `describe.ext` lists the subcommand paths and
their usage, and `warden <name> --help` prints them. A first argument that
names no driver that can be found is the unknown-command error, as today.
In K1 `warden hyperv service ...` stays a `warden` command (hyperv is
linked); it has the same form, so nothing changes for the user when K2
moves it behind `ext service ...`.

**Commands not started by `warden`.** Two kinds exist, and the contract
allows both:

- A process the operating system starts, such as a service registered as
  `warden-driver-hyperv ext service run`. It has no request file, so
  `ext` started without `WARDEN_DRIVER_REQUEST` is given no config files
  and runs with the driver's defaults; it does not go looking for a
  config file by itself. It must not exit 2 for the missing request.
- A process the driver starts itself, such as a detached image refresh.
  The driver then writes the request itself, as `warden` would.

**Paths** in requests are UTF-8 strings. A context, output or temporary
path that is not valid UTF-8 cannot be carried unchanged in JSON, so for a
driver binary `warden` refuses it with a fixable error before it starts
the driver (Behaviour 15). The linked transport carries paths as Go
strings, as today.

**Probe.** Any request may carry `"probe": true`. The driver checks
`contract`, reads and checks its own flags and keys from `args` and
`config_files`, makes the checks it would make before
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
 "keys": [{"key": "vm.image_refresh_after",
   "usage": "age after which a prepared Windows layer is refreshed"}],
 "ext": [],
 "help": {"doctor": "...", "image-prepare": "..."}}
```

| Field | Meaning |
|---|---|
| `contract` | The integer contract versions the driver speaks |
| `name` | What `--driver` selects; matches the file name |
| `version`, `vendor`, `bug_url` | For `driver.json` and bug text. `warden --version` prints `warden`'s version only and runs no driver |
| `commands` | The commands it implements, `describe` aside. `run` and `reap` must be present. `attach` is listed exactly when `capabilities.shell` is true, and `ext` exactly when the `ext` list is not empty |
| `capabilities.vm` | It boots a separate build machine: it takes the shared VM settings (`start.vm`) and builds a script or a Containerfile. Without it, it builds a Containerfile only and `warden` refuses the VM flags |
| `capabilities.shell` | `run` accepts `mode: "shell"` and `attach` is implemented |
| `capabilities.keep` | What `--keep-vms` leaves: `running` (the machines), `stopped` (the work directory only) or `none` (`warden` refuses the flag) |
| `capabilities.exited` | It sends `exited` when its relay or its environment stops on its own |
| `capabilities.containerfile` | `translate`: `warden` translates a Containerfile to the build script. `ignore`: the guest cannot run one; `warden` drops it and records nothing |
| `capabilities.guest_os` | The `--guest-os` values `warden` accepts for it; the first is sent when the flag is not given. The bundled drivers list what is accepted today: container `linux`; qemu and hyperv `linux`, `windows`; firecracker `linux`; vz `linux`, which it accepts and ignores today (its guest is macOS, and it reports `darwin` in `relay-started`) |
| `capabilities.image_os`, `default_image_os` | The `--os` values `image-prepare` takes |
| `capabilities.reserved_ranges` | Fixed address ranges it uses on the host; a build link must not overlap them |
| `flags` | The flags it takes, each with its name, whether it takes a value, the commands that take it and a usage line. `warden` uses them to parse the command line and to print help, and reads no value (**Configuration**) |
| `keys` | The config keys it reads, as dotted paths with a usage line: each under `driver.<name>.`, or one of the seven existing keys listed for that name. `warden` reads none of them; the list is what the placement rule is checked against (**Configuration**) |
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
its human-readable log; `warden` reads it through a pipe, passes it to its
own stderr and keeps its tail for error reports.

| From | `type` | Carries |
|---|---|---|
| warden | `start` | `contract`, `build_id`, `mode` (`build` or `shell`), `claim_token`, `guest_os`, `image`, `base_image`, `build_script`, `context_dir`, `tmpdir`, `diagnostics_dir`, `state_file`, `keep`, `verbose`, `vm`, `config_files`, `args` |
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
| `config_files` | The config files `warden` found, highest precedence first; the driver reads its own keys from them (**Configuration**) |
| `args` | The driver's flags from the command line, passed on unread (**Configuration**) |
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
| `build-<driver>-<id>.json` | `warden`, before it creates anything else | The record: the driver's name, the path and digest of its binary (or `linked`), the contract version, the build's temporary directory and stage directory, and the `config_files` and `args` `run` was started with. No secret: the claim token is not in it, and a driver takes no secret as a flag |
| `build-<driver>-<id>.state` | The driver, if it needs it | Whatever the driver needs to find the build again that the ID does not give, and everything it resolved from its configuration that `attach` or `reap` depends on: the container driver writes the runtime it chose; in K2 firecracker writes its slot and qemu its process record. At most 64 KiB, opaque to `warden`. The driver writes it before it creates the thing the content describes |

`attach` and `reap` get the record's `tmpdir`, `config_files` and `args`
and the state file's path, whoever runs them and whenever. The record
names the files and repeats the flags; it does not freeze what the files
said, which is why a value `reap` depends on belongs in the state file.
`warden` removes the three
files when the build ends cleanly, and otherwise only after a `reap` that
exited 0.

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
  process) and runs `reap <build_id>`.
- *A linked driver* shares `warden`'s process, so there is nothing to kill
  and no limit: the teardown wait applies to the exec transport only. A second
  signal ends `warden` at once, as it does today on vz and hyperv. That
  is new on qemu and firecracker, where a second Ctrl-C is ignored today:
  firecracker's VMMs die with `warden` (`Pdeathsig`); qemu's VMs do not,
  and stay until K2 gives qemu a `reap` (Behaviour 8).

A driver treats `stop` and the end of its input the same way: tear down,
then exit.

**The teardown wait** is how long `warden` waits for a driver binary to
answer `stop` with `stopped` before it kills it and runs `reap`. It is
the one time limit of the contract that can be set (Decision 21, a20):

| | |
|---|---|
| Key | `teardown_timeout` in `[build]`, in `warden.toml` or the user's config file; the project's file wins over the user's (ORCH-03) |
| Type | A string holding a Go duration, the form `--timeout` takes: `"90s"`, `"5m"` |
| Default | `"60s"` |
| Bounds | `1s` to `1h`, inclusive |
| Read by | `warden`, as the config files load |
| Applies to | A driver binary, on every `stop`: a completed, failed, interrupted or timed-out build, and `warden shell`. A linked driver has nothing to kill, so in K1 the key is read and checked for it and then has nothing to act on |
| Flag, variable | None |

It is `warden`'s key and not the driver's, although a driver now reads
its own configuration (a21): the wait is the caller's patience, not a
setting of the thing being waited for. `warden` applies it to every
driver alike, one written without the kit included, and the driver it
is meant to bound is by definition one that has stopped answering. It
sits in `[build]` with the other per-build settings `warden` owns
(`tmpdir`, `output_dir`). It has no flag: how long a teardown takes is a
property of a host and its drivers, set once and not per invocation, as
`[vm] dns` and `[relay] upstream_ca_certs` are keys without flags today;
a flag can be added later without breaking anything (UX-03). The value
is not sent to the driver in version 1.

The bounds: below `1s` every `stop` would end in a kill and a `reap`
instead of a teardown, and above `1h` the wait bounds nothing; a second
Ctrl-C is the way to stop waiting sooner. A value that does not parse or
is out of range is refused as the file loads, before any driver runs
(UX-08), and the default is not used in its place (GLOBAL-10):

```
[warden] ERROR: [build] teardown_timeout in warden.toml: "0s" is below
                the minimum of 1s
  To fix: set it to a duration from 1s to 1h, such as "90s", or remove
          it to wait the default 60s
```

Every other limit stays fixed: `describe` within 10 s, a probe within
30 s, the relay's `ready` within 90 s of the start of the claim, 30 s
without a heartbeat, and 10 s for a completion after the environment
exits.

**What fails, and what follows.**

| What fails | When | Sent | What `warden` does |
|---|---|---|---|
| The driver refuses the build (preflight, or the relay cannot start) | Before `relay-started` | The driver removes what it made, sends `failed` with a `fixable` or `internal` error, exits 1 | Prints the error as it prints a preflight error today. No `reap`. Removes the stage directory, record and lock. `driver.json` and `inputs/` stay in the output directory |
| `warden`'s claim fails or times out | After `relay-started` | `warden` sends `stop` with `reason: failed` and the claim's error as `detail`; the driver tears down and sends `stopped` | Closes the session; prints the claim's error, with `old_relay_fix` when the relay refused a field; exits 1 |
| The relay exits | Any time after `relay-started` | The driver sends `exited`, `which: relay` | Cancels a claim in progress, or fails the build at once; then `stop` with `reason: failed` |
| The environment cannot be started | After `claimed` | The driver tears down, sends `failed`, exits 1 | Closes the session and prints the error. No `reap` |
| The environment exits before the build completes | After `environment-started` | The driver sends `exited`, `which: build` | Waits 10 s for a completion, then fails the build; `stop` with `reason: failed` |
| The driver's process ends with neither `stopped` nor `failed`, writes a stdout line that is not a JSON object, or does not stop within the teardown wait | Any time | Nothing | Kills the driver's process group, shows the tail of its stderr as a driver defect, and runs `reap <build_id>` with the record's values. If the collector already holds the build's completion, the build's result stands and the defect is reported beside it; otherwise the build fails. If `reap` fails, the record and lock stay for `warden clean` |

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

1. For every `build-<driver>-*.lock` nobody holds, it reads the build's
   record. For a driver binary it runs `reap <id>` on the binary the
   record names, after the check that it is a file that can be run
   (**Discovery and trust**), with the
   record's `tmpdir`, `config_files`, `args` and state file. A binary that has changed
   since the build (an upgrade) is run, with a line that says so; one
   that is gone or cannot be run is an error for that build, with the
   fix (`warden clean --driver <path>` reaps with the file named). It
   then removes the build's stage directory, which is `warden`'s own. When
   `reap` exited 0, it removes the state file, the record and the lock
   file. When it did not, all three stay, the error is shown, and `clean`
   exits 1 at the end. A lock with no record is a build that died before
   it created anything; the lock file is removed.
2. It runs the driver's `clean`, when it has one, with the IDs of the
   locks that are held as `keep_ids` and this invocation's `tmpdir`,
   config files and flags.

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

Two readers of one set of files (Decision 7, a21). `warden` reads its own
flags and keys. A driver reads its own, inside its own process: through
the kit when it is built on it, and by the contract's rules when it is
not. `warden` does not look at a driver's values, and a driver is sent
none of `warden`'s beyond the fields the contract's requests carry
(`tmpdir`, `guest_os`, `vm`, `keep`, `verbose`).

The three rules this has to keep, quoted:

- ORCH-03: "Configuration precedence stays `flag > warden.toml >
  ~/.config/warden`."
- UX-07: "any of them is easy to override at three levels with clear
  precedence, invocation > project > user (`flag > warden.toml >
  ~/.config/warden`, the same order as ORCH-03)", and "A value set at any
  level replaces the default and the lower levels' value (GLOBAL-10)."
- UX-08: "Configuration errors or mismatches are validated and surfaced
  as early as possible, at load/parse/startup, before any real work
  begins, rather than failing deep into a build."

**What `warden` reads and owns.**

- Flags: `--driver`, `--verbose`, `--color`; `--capture`, `--output`,
  `--no-compress`, `--timeout`, `--tmpdir`, `--quiet`; `--guest-os`,
  which every driver takes and which is checked against
  `capabilities.guest_os`; the VM group `--script`, `--image`, `--cpus`,
  `--memory`, `--keep-vms`, `--build-link`, refused for a driver without
  `capabilities.vm` (Decision 8, a9); `image prepare`'s `--os` and
  `--source`; `clean --cache`.
- Keys: `runtime.driver`, `build.*` (with `teardown_timeout`, a20),
  `relay.*`, `vm.cpus`, `vm.memory_mb`, `vm.build_link`, `vm.dns`,
  `output.*`.
- Variables: `WARDEN_DRIVER`, `WARDEN_UPSTREAM_CA_CERTS`, `NO_COLOR`,
  `WARDEN_VERBOSE`.

`LoadConfig` and `resolveConfig` keep today's precedence for these. The
build link and `[vm] dns` are still parsed and checked by `warden` as the
files load, and the link is checked against the reserved ranges of every
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
changes for a user is Behaviour 13.

**What `warden` hands a driver.** Every request, `start` included,
carries `config_files`; every request for a command that takes flags
carries `args` as well; and the driver process inherits `warden`'s
environment. Nothing else about configuration crosses: no resolved value,
no type, no default.

| Field | Holds |
|---|---|
| `config_files` | The absolute paths of the config files `warden` found, highest precedence first: the project's `warden.toml`, then the user's `~/.config/warden/config.toml`. A file that does not exist is not listed. `warden` has parsed each for its own keys before it runs the driver, so a listed file was valid TOML a moment earlier |
| `args` | The driver's flags from the command line, in the order typed, one element per occurrence: `--name` for a flag typed without a value, `--name=value` for one typed with a value, whether as `--name value` or as `--name=value`. The value's bytes are unchanged. Empty when the user gave none |

`args` is passed through, not interpreted: `warden` does not know what a
value means. Only the split between a flag and its value is put in one
form, so that a driver written by hand parses one syntax and not two. The
flags travel in the request and not in the driver's argv, where `argv[1]`
stays the command; a driver should take no secret as a flag, because the
build's `args` are kept in its record (**The build record**).

For `run` the two fields are also written to the build's record, and
`attach` and `reap` are sent the record's, whoever runs them, `warden
clean` included, and not those of the invocation that runs them. They name the build's files and flags, not
their content at the time, so what `reap` and `attach` must know about
how the build was configured the driver writes to its state file during
`run`, as the container driver does with the runtime it chose.

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
   "usage": "durable build switch name (default: [vm] network, then warden-dev)"},
  {"name": "force", "value": false, "commands": ["image-prepare"],
   "usage": "re-acquire even if a valid cached copy exists"}]
```

| Field | Meaning |
|---|---|
| `name` | The flag without its dashes, `[a-z][a-z0-9-]*`. There are no short forms |
| `value` | `true` when the flag takes a value, `false` when it stands alone |
| `commands` | The commands that take it: `run` (which is `warden build` and `warden shell`), `doctor`, `setup`, `image-prepare`, `clean`. `image-list` and `image-verify` run with no driver selected and take none. `ext` is not listed: everything after `warden <name>` is the driver's and goes to its argv as typed |
| `usage` | One line for `--help` |

`warden` places a declared flag and does nothing else with it. It does
not know its type, read its value, apply a default or relate it to a
key. What it does, in order:

1. **Find the driver before the flags are parsed.** `warden` reads
   `--driver` from the raw arguments, else `WARDEN_DRIVER`, else
   `[runtime] driver` from the config files (from the project's file, a
   name and never a path), else `container`, and looks the driver up.
   Commands that select no driver (`inspect`, `driver list`, `image
   list`, and `image verify` without `--driver`) skip this, so no
   driver's flag is registered on them. `--runtime` is a persistent flag
   today, accepted and ignored on those commands; after K1 it is an
   unknown flag there (Behaviour 2).
2. **Describe.** `warden` runs `describe` on the selected driver only,
   once per invocation, and refuses one that does not match the schema,
   lists no common contract version, lacks `run` or `reap`, or reports a
   name other than the one looked up. It also refuses a flag whose name
   `warden` uses on the same command, and a key outside the driver's
   place in the files (below); each refusal names the flag or key. For
   the bundled drivers a test in `cmd/warden` checks the same, so a
   clash is found by `make test`.
3. **Register** the declared flags on the commands that take them, as
   flags `warden` passes on. A driver's flag is accepted wherever a flag
   of that command is accepted today; `--runtime`, a persistent flag
   today, is still accepted before the subcommand's name.
4. **Parse.** A flag that neither `warden` nor the selected driver
   declares for the command is refused here, before any other driver
   command runs. A VM-group flag on a driver without `capabilities.vm`,
   and a `--guest-os` the driver does not list, keep today's messages.
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
Datacenter"]`. Without the declaration `win.iso` could as well be the
value of `--force`.

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
var settings = []drivercmd.Setting{
	{Name: "runtime", Type: drivercmd.String,
		Flag: "runtime", Key: "runtime.cli", Env: "WARDEN_CTR_CLI",
		Commands: drivercmd.EveryCommand,
		Usage:    "container runtime (finch, docker, podman)"},
	{Name: "relay-image", Type: drivercmd.String,
		Key: "runtime.relay_image", Env: "WARDEN_IMAGE",
		Commands: []string{"run"},
		Usage:    "relay image reference, or dev to build it from source"},
}
```

From that the kit does the rest:

- It builds `describe`'s `flags` and `keys`.
- **It reads the same files with the same precedence `warden` uses.**
  For each setting that applies to the command: the flag in `args` when
  given (the last occurrence); else the setting's environment variable;
  else the first file in `config_files` that sets its key, which is the
  project's `warden.toml` before the user's config; else absent, and the
  driver applies its default. That is ORCH-03's and UX-07's order, with
  the two existing variables where `LoadConfig` has them today, between
  the flag and the files. The level that sets a value is the value: it
  is not merged with a lower level's, and for a key that holds a table
  or a list the whole of it comes from one file (GLOBAL-10).
- **Typed access.** `String`, `Int`, `Bool`, `Age` (`7d`, `36h`), `CIDR`,
  `IPList` and `Path`, read as `cfg.String("runtime")`; and `Raw`, a key
  of any shape, which the driver decodes into its own Go type with
  `cfg.Decode`. `cfg.Source(name)` is the label to print for a value:
  `--runtime`, `WARDEN_CTR_CLI`, or the file's name, as `warden` labels
  its own.
- **Validation, before any work.** The kit resolves and type-checks every
  setting that applies to a command when the request arrives: in the
  probe of `run`, and in every other command before the driver's
  function is called. Checks of range and consistency are the driver's,
  in its `Preflight` or first in the command (UX-08).
- **The error form.** A value that cannot be used is a
  `driver.FixableError` that names where it came from, which the kit
  sends as a `fixable` error object and `warden` prints as it prints its
  own (UX-04):

  ```
  [warden] ERROR: [vm] image_refresh_after in warden.toml: "soon" is not
                  an age
    To fix: set it to an age such as "7d" or "36h"
  ```

- It refuses an `args` element the driver did not declare, and a
  declaration whose key is outside the driver's place in the files.

What the five bundled drivers declare is exactly what `cmd/warden` holds
for them today. No name, table or variable changes.

| Driver | Setting | Flag on | Config key | Env |
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

**What a driver written without the kit must do itself.** These are the
contract's requirements on configuration, numbered as the specification
numbers them. The last column says whether the suite can hold a binary
to each.

| | A driver must | The suite, against a binary |
|---|---|---|
| C1 | Declare every flag it takes in `describe.flags`, with its arity and its commands, and use no name `warden` uses on the same command (the specification lists them) | Static tier: the schema, the name form, the reserved names, and that each flag names only commands the driver lists |
| C2 | Take each declared flag from `args` as `--name` or `--name=value` (for a flag that holds one value, the last occurrence wins), and refuse an element it did not declare with a `fixable` error that names it | Static tier: a probe of each listed command with an undeclared element exits 1 with a `fixable` error naming it, and creates nothing |
| C3 | Read configuration only from the files in `config_files`. It never looks for `warden.toml` or a user config by itself; started with no request (a service), it reads none and uses its defaults | Static tier: with an invalid `warden.toml` in its working directory and an invalid config under its `HOME`, and `config_files` empty, a probe ends as it does with neither file present |
| C4 | List the keys it reads in `describe.keys`, each in its own place in the files (below) | Static tier: every listed key is under `driver.<name>.`, or is one of the seven existing keys listed for that name. A key read but not listed cannot be seen from outside |
| C5 | Ignore every table and key it does not own | Static tier: with a config file that holds `warden`'s tables, another driver's table and an unknown one, a probe ends as it does with no file |
| C6 | When it lists a key: refuse a file in `config_files` that it cannot parse, and a key of its own whose value has the wrong type, with a `fixable` error that names the file, and the key where there is one | Static tier, for a driver that lists a key: a file that is not TOML, and a file in which `driver.<name>` is a number and not a table (a wrong type for every key beneath it), each make a probe exit 1 with a `fixable` error naming the file |
| C7 | Read and check all of it before any work: in the probe of `run`, and first in every other command | Static tier, through C2 and C6: the refusals come from a probe, which must create nothing |
| C8 | Apply the precedence: a flag, else the first file in `config_files` that sets the key, else its default; the level that sets a value is the value, not merged with a lower one, and an unusable value is an error, never replaced by the default | **Not checkable against an arbitrary binary**: the suite does not know what a value means to a driver, so it cannot see which level won. Held by the kit's own tests for a driver built on it, and by the suite's self-test for the fake, whose one key is echoed back in its probe |
| C9 | Take its settings as flags and keys, not as environment variables of its own (UX-07). The two that exist, `WARDEN_CTR_CLI` and `WARDEN_IMAGE`, stay the container driver's | Not checkable: a process may read any variable |

A driver that takes no flag and reads no key lists none and has nothing
to do here but C2's refusal. One that reads keys needs a TOML parser in
its own language; that is part of what a21's answer costs.

**Where a driver's configuration lives** (Decision 9, a4, open). A
driver's keys live in its own table, `[driver.<name>]`, in the same two
files, and under that table it may have any shape it likes: nested
tables, lists, lists of tables. The seven keys that exist today stay
where they are, each read only by the bundled driver that owns it, and
the specification lists them by driver name:

| Key | Read by the driver named |
|---|---|
| `runtime.cli`, `runtime.relay_image` | `container` |
| `vm.network`, `vm.windows_image`, `vm.windows_image_update` | `hyperv` |
| `vm.image_refresh_after`, `vm.image_min_grace` | `qemu` and `hyperv`, with one meaning |

What stops two claimants of one key:

- *Two drivers.* Each owns exactly `[driver.<its own name>]`, and a name
  resolves to one binary (**Discovery and trust**). The one shared pair,
  the two ages, is shared on purpose and listed.
- *A driver and `warden`.* `warden` defines no key under `[driver]` and
  never will; a driver reads none outside its table but the listed
  seven.
- *What holds it.* `warden` refuses a `describe` whose `keys` break the
  rule, before it uses the driver; the suite's static tier checks the
  same (C4); the kit refuses such a declaration; and for the bundled
  drivers tier 1 reads the declarations from source. What nothing can
  hold is a driver written without the kit that reads a key it did not
  list: the specification forbids it, and no check can see it.

A copy of a bundled driver that replaces it from `PATH` (a16) has that
driver's name, and so reads that driver's keys.

**The shared VM settings** (Decision 8, a9, open) stay `warden`'s under
this design: `warden` reads `--image`, `--cpus`, `--memory`,
`--build-link`, `[vm] cpus`, `memory_mb`, `build_link` and `dns`, checks
the link and the resolvers as the files load, and sends them in
`start.vm`. a9 puts the other reading of a21's answer, that each VM
driver reads them itself, as an option.

**Help.**

- `warden <command> --help` prints `warden`'s flags as today, then a
  section of its own, `Flags of driver <name> (<file>, or linked):`,
  with the flags the selected driver declares for that command and their
  `usage` lines. The long descriptions of `doctor`, `setup`, `image
  prepare` and `clean` keep today's prose for the bundled drivers in K1;
  a driver may add a paragraph of its own per command (`describe.help`).
- When the selected driver cannot be found or its `describe` fails,
  `--help` still prints `warden`'s part, followed by one line that says
  which driver could not be asked and why, and exits 0.
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
| A driver's flag with a value it cannot use (`--slots many`) | The driver | For `build` and `shell`, in the probe of `run`: before `warden` takes the lock, creates the output directory or stages anything. For any other command, first in that command, before it changes or downloads anything | A `fixable` error object that names the flag as typed and gives the fix; `warden` prints `[warden] ERROR:` and `To fix:` with no bug link | 1 |
| A driver's key of the wrong type, or with a value it cannot use | The driver | The same points | `fixable`, naming the file and the key | 1 |
| A driver's key, while another driver is selected | Nobody | | It is not read (Behaviour 13) | |
| A key nobody declares: a mistyped key | Nobody, as today | | Ignored, as unknown keys are today | |
| An `args` element the driver did not declare | The driver | In the probe | `fixable`. `warden` never sends one, so it means a driver whose `describe` and parser disagree, or another caller's mistake | 1 |

Every one of these exits 1: UX-09's "every other failure exits 1". The
contract's exit code 2 stays what it was, a request that is not valid
for the contract version, and is not used for a bad value. All of them
come before any real work (UX-08). What moved is how early: `warden`
used to refuse a bad value of a driver's flag or key as it loaded, and
now a driver process is started first, and a driver's bad key is found
only when that driver is selected.

**The four linked VM drivers: one path, not two.** The kit has one entry
that both transports use:

```go
// Call runs one command of d with req, as Main does in a driver process:
// it resolves and checks the driver's settings from req.Args, the
// environment and req.ConfigFiles, then calls the command's function.
func Call(ctx context.Context, d *Driver, command string, req *Request,
	host driver.Host) (*Result, error)
```

`Main` decodes a request from stdin or from the request file and calls
`Call`. `linked.go` builds the same `Request` value in memory, with the
same `args` and the same `config_files`, and calls `Call`. So for a
linked driver the kit reads the config files in `warden`'s process, a
second time after `warden` read them for its own keys, exactly as it
would in a driver process, and no resolved value passes between
`cmd/warden` and a linked driver that would not cross the pipe. The VM
drivers' doctor, setup, image and clean code that stays in `cmd/warden`
until K2 gets its values from the function `Call` uses,
`drivercmd.Resolve(settings, command, args, configFiles, environ)`,
where it reads flag variables and `Config` fields today.

**The reference docs and tier 1.** For the bundled drivers nothing
changes in what is documented or where: `guide/cli.md` and
`guide/configuration.md` list their flags and keys (UX-01, ORCH-03).
Their declarations are Go literals with constant strings in their own
packages, so tier 1's surface check reads them from source (**Rule
changes**).

- *What it can still check, for a bundled driver:* every `Flag` and
  `Key` in a `drivercmd.Setting` literal, at the base and at the head. A
  flag or key added or changed without a reference doc in the diff is a
  finding (UX-01, ORCH-03), and one removed with no alias is a finding
  (UX-03), exactly as for a flag call or a `toml` tag in `cmd/warden`.
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
	List     func(ctx context.Context) (count int, err error)
	Verify   func(ctx context.Context) (checked, drift int, err error)
	Clean    func(ctx context.Context, req *CleanRequest) (removed int, err error)
	// Attach returns the shell's exit code, or an error when the attach
	// itself failed; Main writes the error to the result file.
	Attach   func(ctx context.Context, req *AttachRequest) (exitCode int, err error)
	Ext      func(ctx context.Context, req *ExtRequest, args []string) error
}

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
	Key      string   // "driver.nomad.region", or one of the seven existing
	                  // keys ("runtime.cli"); "" for no key
	Env      string   // WARDEN_CTR_CLI or WARDEN_IMAGE only (UX-07)
	Commands []string // the commands that take it
	Usage    string   // the flag's help line, and the key's
}

// Resolve reads the settings that apply to command: the flag in args, else
// the setting's environment variable, else the first of configFiles that
// sets its key (ORCH-03). It refuses an element of args that settings does
// not declare, and a value of the wrong type, with a FixableError that names
// where the value came from.
func Resolve(settings []Setting, command string, args, configFiles,
	environ []string) (driver.Settings, error)

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
	// Decode reads a Raw setting, a key of any shape, into v.
	Decode(name string, v any) (ok bool, err error)
	// Source is the label to print for a value: --runtime, WARDEN_CTR_CLI,
	// or the name of the file it was read from.
	Source(name string) string
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
request's `args` and `config_files` and from the environment, and hands
them to the command's function as a `driver.Settings` (in `driver.Start`
for `run`); answers a probe; writes the result file; maps a
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
the config files with `warden`'s precedence; and `driver.Settings`, the
typed access. All of it runs in the driver's process, or in `warden`'s
for a linked driver, through the one `Call` (**Configuration**).

A driver keeps today's shape: where it called `sess.ClaimRelay(ctx, addr,
token, req, env, link, fix)` it calls `host.ClaimRelay(ctx, r)` with the
same address, environment, link and fix; where it called `sess.Wait(ctx)`
it calls `host.Wait(ctx)` and tears down when that returns. It no longer
calls `StartSession`, `StageBuildScript`, `RecordInputs` or
`FinalizeOutput`.

`driver.Host` has two implementations: `warden`'s, over its session, for
a linked driver; and `drivercmd`'s, over the pipe, for a driver binary. A
linked driver and a driver binary are thus the same `Driver` value.

The Go types are not the contract. They may change with the kit; the
specification and the suite are what a driver is held to.

**Without the kit** a driver does the same by hand: read a JSON line,
write a JSON line, exit with the right code. It needs no Dockerfile
translator, no collector and no claim client. It does need to read its
own configuration: to declare its flags, take them from `args`, and, if
it has keys, parse the TOML files it is named and apply the precedence
itself (requirements C1 to C9 under **Configuration**). A driver with no
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
| `describe` | `newDriver`'s container arm; `isVMDriver` | `vm: false`, `shell: true`, `keep: none`, `containerfile: translate`, `guest_os: ["linux"]`; the flag `runtime`; the keys `runtime.cli` and `runtime.relay_image`; `reserved_ranges` its base range |
| `run` | `StartBuild` and `Exec`'s setup | Its probe reads its settings and resolves the runtime (the `runtime` setting, else detection) and refuses a missing or stopped one, as `newDriver` does today before any work. The build resolves it once more and writes it to the state file, then starts the relay container, then on `claimed` the holder, the iptables sidecar, the build container, the executor copy and the start. `mode: shell` starts it held and waits for `/etc/warden/shell.env`, as `waitHeld` does. Its isolation, delivery and start code stays in `driver/container/container.go`, so BOOT-07's surface list does not change |
| `attach` | The second half of `Exec` | `<runtime> container exec -i -t <build> sh -c ...` on the inherited terminal; the exit code from `/.warden/shell-exit`. The runtime is the one in the build's state file |
| `reap` | New; `teardown`'s removals by name | Containers, network, dev relay image and work directory of one build ID, in the runtime the state file names and under the record's `tmpdir`. With no state file it uses the `runtime` setting as it resolves now, else detection. A runtime that does not answer is exit 1 with the fixable error, never "nothing to remove" |
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
  `Config` fields today.
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
| The name matches the file name; `run` and `reap` are listed; every flag has a name of the allowed form that `warden` does not use on the same command, a `value`, a usage text and only commands the driver lists (C1); every key is under `driver.<name>.` or is one of the seven existing keys listed for that name (C4) | UX-01, a4, a24 |
| `capabilities` are consistent: `attach` is listed exactly when `shell`; `ext` exactly when the `ext` list is not empty; `image_os` exactly when `image-prepare` is | |
| A probe of each listed command with an element in `args` that the driver did not declare exits 1 with a `fixable` error that names the element, and creates nothing (C2) | UX-04, UX-08 |
| With an invalid `warden.toml` in the driver's working directory and an invalid config under its `HOME`, and `config_files` empty, a probe ends as it does with neither present: the driver reads only the files it is named (C3) | ORCH-03 |
| With a config file that holds `warden`'s tables, another driver's table and an unknown table, a probe ends as it does with no file (C5) | ORCH-03 |
| For a driver that lists a key: a file in `config_files` that is not TOML, and a file in which `driver.<name>` is a number and not a table, each make a probe exit 1 with a `fixable` error naming the file, and create nothing (C6, C7) | UX-08 |
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
driver its flags and a config file for that host (the container runtime,
Hyper-V's network), passed on as `args` and `config_files`;
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
| Kill | The driver killed at the same three points; `reap`, given the build's values and state file, then removes what was left, and a second `reap` removes nothing | ORCH-04 |
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
switch and one key (`driver.fake.greeting`), reads them by hand with the
precedence the specification states, and echoes the greeting and the
level it came from on stderr in a probe, which is how the suite's
self-test holds it to C8.
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
  own refused, the user's file read before the project's), and the
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

Seventeen things change for a user (a5, open). Everything else is meant
to be identical. The list was re-derived after the review's answers of
2026-11-14 to a16, a20 and a21. Of the redraft's fifteen, none goes;
1, 3, 4, 8, 10, 12 and 13 change, and 16 and 17 are new. Each says so.

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
   a line that says so.
2. **A flag the selected driver does not take is refused.** Today eleven
   flags are accepted and ignored on drivers that do not use them:
   `--runtime` on any driver but container; `--network` on `doctor` and
   `setup`, and `--nat-prefix` and `--host-ip` on `setup`, for any driver
   but hyperv; and on `image prepare`, `--sha256` and `--name` (hyperv
   only), `--edition`, `--virtio` and `--arch` (qemu only), `--force` and
   `--keep-vms` (qemu and hyperv only). `--runtime` is also accepted and
   ignored today on the commands that select no driver (`inspect`, `image
   list`, and `image verify` without `--driver`); there it becomes an
   unknown flag. GLOBAL-10 asks for this: a provided value that cannot be
   used is an error. UX-03 would give such a change a deprecation
   window; a5's option (c) is that route. *Stands.* After a21 it is not
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
   name or a path of another is accepted. A project `warden.toml` may
   name any driver; one whose `[runtime] driver` is a path is refused
   with its own message, where it was "unknown driver" before. *Changed
   by a16:* the redraft refused a project file that named a driver that
   is not bundled.
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
   (60 s unless set). For a linked driver it ends `warden` at once: as
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
12. **`--driver`, `WARDEN_DRIVER` and `[runtime] driver` in the user's
    config accept a path** to a driver binary (in the user's config, an
    absolute one). *Changed by a16:* the redraft allowed no config file
    to name a path.
13. **A driver's keys are read only by that driver, and only when it is
    selected.** Two things follow for the seven existing driver-owned
    keys. A malformed `[vm] image_refresh_after` or `image_min_grace` is
    refused today on `build` and `shell` whichever driver is selected
    (`newRequest` parses both ages for every driver) and on no other
    command; after K1 it is refused by qemu and by hyperv, on the
    commands of theirs that read it, and not looked at under another
    driver. And a wrong TOML type in any of the seven (`[runtime] cli =
    5`) fails the load today for every driver and every command; after
    K1 it is refused only by the driver that owns the key, and passes
    unseen under another. The message keeps its text and fix and names
    the file the value was read from, where the ages' always said
    `warden.toml`. These refusals now come from the driver's probe: after
    `warden`'s own checks, and still before anything is created.
    *Changed by a21:* the redraft kept the load-time type check of the
    seven under every driver; with the driver as their one reader it
    goes.
14. **`--timeout` counts from the moment `warden` starts the driver, on
    every driver.** Each driver started its own clock, at slightly
    different points of its preflight. *Stands.*
15. **A path that is not valid UTF-8 is refused for a driver binary.** A
    build context, output directory or `--tmpdir` whose name is not UTF-8
    cannot be sent to a driver process unchanged. In K1 that is the
    container driver; the linked drivers take it as today. *Stands.*
16. **A `warden-driver-<name>` on `PATH` runs in place of the bundled
    driver of that name.** Nothing has that name today, so nothing
    changes for a user who has put no such file there. One who has,
    gets it for every command, with no line at build time to say so;
    `warden driver list` and `driver.json` show it. *New, from a16.*
17. **`[build] teardown_timeout` is new**: how long `warden` waits for a
    driver binary to tear down before it kills it and reaps, 60 s unless
    set. Additive; nothing waited or killed this way before. *New, from
    a20.*

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
  driver key keeps its table, name and meaning. The precedence between
  a flag, `WARDEN_CTR_CLI` or `WARDEN_IMAGE`, the project's file and the
  user's is the same; the kit applies it where `cmd/warden` did.
- The environment variables the drivers read for themselves (**Defaults
  and overrides**): a driver process inherits `warden`'s environment.
- Exit codes (UX-09): the build's code, 124, 130, and 1 for a blocked
  doctor with no extra line.
- Which image a VM build boots when the context has both a script and a
  Dockerfile: the Dockerfile's `FROM`, as today (`start.base_image`).

`docs/book/src/guide/cli.md`, `configuration.md`, `output.md` and
`drivers.md` are updated for each in the same change (UX-01). No existing
flag, subcommand, config key or environment variable is renamed or
removed. Eleven flag-and-driver combinations that were accepted and
ignored are refused (2), which GLOBAL-10 asks for and for which UX-03
would give a window (a5).

### What guards behaviour

- **Characterization tests, written first, on unchanged code.** Before any
  production change, `cmd/warden` gains golden tests of: `--help` for every
  command; the error and fix of every refusal, per driver; the
  `BuildRequest` each driver gets from a table of flags, environment and
  config files (extending `request_test.go`, `vmdns_test.go` and
  `buildlink_test.go`); the refusal of a malformed age and of a wrong TOML
  type, per command; `image list` and `image verify` over fixture caches;
  the files `inputs/` holds after staging, per driver; and, for each VM
  driver, the image a build resolves for a context that holds a
  Dockerfile and is built with `--script` (the choice is made inside the
  driver today, in `linuxguest.ContainerfileImage`, so a request table
  alone would not see it). The later commits may change a golden only
  for the seventeen differences above, so each shows up as a reviewed
  golden diff.
- **The request tables become two tables, over the same rows.** Today one
  table of flags, environment and files is asserted against the
  `BuildRequest`. After K1 `warden`'s half of each row is asserted
  against the `start` message, with the `args` and `config_files` it
  carries; and the driver's half is asserted against what the kit's
  `Resolve` returns for that row, per driver. A setting lost on either
  side of the hand-over fails a test, and so does a precedence that
  differs from today's between a flag, a variable, the project's file
  and the user's.
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

### Order of the work

One branch, one PR, each commit green on `make test` and `make lint` for
the platforms it touches (FLOW-17).

1. Characterization tests. No production code changes.
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
5. The specification and its schema; `driver/drivercmd`; the fake and
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
    config files. A file that changes between the two reads, or a parser
    that differs (a driver in another language), can make them disagree
    about a file `warden` has already accepted.
  - *`reap` and `attach` re-read configuration that may have changed.*
    The record keeps the build's flags and file paths, not the values
    resolved from them; a driver that needs a value to find its build
    again must write it to its state file.
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
  here: a `warden-driver-container` that reached a `PATH` directory by
  accident (a stale build, another checkout's `bin`) or by design runs in
  place of the shipped one. `warden driver list` shows it and every
  build's `driver.json` records it; nothing warns.
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
  And `[runtime] cli` in a project's `warden.toml` is run as given
  (`cmd/warden/config.go`, `driver/container/container.go`), so a build
  context can already choose a program `warden` runs; K1 keeps a project
  file from naming a driver's file and leaves this one as it is. Each is
  to be filed as a follow-up item.

### The overview (FLOW-15)

K1 is the only item of its phase, so it makes the sweep itself. The
orchestrator's pseudocode in `concepts/overview.md` changes to:

```
warden build <context>:
    driver := the driver named by --driver, WARDEN_DRIVER or [runtime] driver
              (a project file: a name, never a path), else container: a
              path as given, else warden-driver-<name> on PATH, else beside
              warden, else a linked driver; not found is an error
    d := driver.describe                    # what it is; the flags it takes
    parse the command line: warden's flags, and the driver's declared
        flags, which are passed on unread; a flag neither declares is
        refused
    load warden's own configuration
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
        state file, the config files' paths, the driver's flags, ...}
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
        variables, its keys from the config files start names, with
        warden's precedence                                    # the kit
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
stated reasons for keeping an unreachable exported function do not cover
`driver/`; widening them is a rule change for K2, when the kit has
consumers outside this repository.

## Defaults and overrides

Each default, override and fallback K1 adds, changes or moves. Anything
not listed is unchanged and `warden`'s.

| Default, override or fallback | Status | Why it is the obvious choice (GLOBAL-09) | What overrides it, exclusively (GLOBAL-10) |
|---|---|---|---|
| The driver when none is named: `container` | Unchanged | It is today's default and needs no host setup | `--driver`, else `WARDEN_DRIVER`, else `[runtime] driver` (today's order). A named driver that is not found is an error; the default is not tried |
| Which file runs for a driver name: the first `warden-driver-<name>` on `PATH`; when `PATH` has none, the bundled copy (a file beside `warden`, else a driver linked into it) | **New** (a16) | `PATH` is where a user puts a program they mean to run, and how git and kubectl find a command; the bundled copy is what runs when the user has put nothing there, which is every install that has not been customized | A path in `--driver`, `WARDEN_DRIVER` or the user's config file: that file and nothing else, and if it cannot be run that is the error. The order falls through only on absence, from `PATH` to the bundled copy when no file of the name is on `PATH`. It never falls through on failure: a file found on `PATH` that cannot be run, or whose `describe` fails, fails the command, and the bundled copy is not tried (GLOBAL-10, GLOBAL-11) |
| A project `warden.toml` names a driver, never a file | **New** rule, the one guard kept (a16) | That file may come with someone else's build context; a name can only select a program the user already has | A path comes from `--driver`, `WARDEN_DRIVER` or the user's config file |
| A relative path in the user's config file is refused; relative and empty `PATH` entries are skipped; on Windows only `warden-driver-<name>.exe` is a candidate | **New**, each for correctness and none for trust (**Discovery and trust**) | A relative path in a file read from every directory names a different file in each; the other two are what Go's `os/exec` and a process `warden` can stop require | An absolute path; an absolute `PATH` entry or a path to the file; nothing for `.exe` |
| A context, output or temporary path that is not valid UTF-8, for a driver binary | **New** refusal (Behaviour 15) | It cannot be sent to another process as JSON unchanged, and altering it silently would build something else | Nothing: use a path that is UTF-8 |
| The collector: in `warden`'s process, one per build, reached over the tunnel | Unchanged in effect; now `warden`'s for every driver (a12) | It is today's, and it opens no listener | Nothing in K1; no flag or key is added (**The collector**) |
| Who sends the claim: `warden` | **Changed owner** (a13); the claim is unchanged | The collector's owner must hold the tunnel | Nothing in K1 |
| The contract version: the highest both sides speak | **New** | The newest common behaviour | Nothing; none in common is an error naming both |
| A build may complete for 10 s after its environment exits | **Changed** for container and Hyper-V, from 5 s (Behaviour 7, a20) | One rule in `warden` for every driver; 10 s is the longest any driver uses today | Nothing |
| After `stop`, a driver binary has the teardown wait to tear down, 60 s when `[build] teardown_timeout` is not set; then it is killed and `reap` runs. A linked driver has no limit | **New**, and a key by the review's answer (a20, Decision 21) | 60 s is long enough for the slowest teardown known and short enough that Ctrl-C ends | `[build] teardown_timeout`, from `1s` to `1h`: the project's file, else the user's, and the value set is the wait, not a floor or a ceiling on the default. A value that does not parse or is out of range is an error at load; 60 s is not used in its place. A second Ctrl-C or SIGTERM ends the wait at once |
| SIGTERM stops a build as Ctrl-C does, on every driver | **Changed** for container, vz and hyperv, where it killed `warden` (Behaviour 9) | A cancelled CI job should not leave containers or VMs | Nothing |
| `describe` answers within 10 s, a probe within 30 s | **New** (a20) | `describe` does no host work; a probe makes the driver's preflight, and the slowest today is the container runtime detection (three runtimes at 3 s each) | Nothing; a slower driver is reported as a defect |
| `reap`, `attach` and `warden clean` use the values in the build's record and state file, not the current invocation's | **New** (Decision 18, a20) | They must look where the build put things | Nothing. `warden clean --driver <path>` chooses the binary that reaps, when the recorded one is gone |
| `warden clean` runs `reap` for every build of the selected driver whose lock nobody holds, and removes the lock only when `reap` succeeds | **New** (a20) | No `warden` holds the lock, so the build is dead for this user. `reap` must be safe beside a driver that is still tearing down by itself | Nothing; a held lock's build is never touched |
| What `clean` treats as live: the held locks (`keep_ids`) and, for the container driver, its process scan as today | **Changed**: the locks are added | Either sign is enough to keep a build | Nothing |
| `--timeout` counts from the start of `run` | **Changed** slightly (Behaviour 14) | One clock for every driver | `--timeout` |
| A driver setting nobody set | Unchanged per setting: the driver applies its own default (`network`: `warden-dev`; `slots`: 4 for hyperv, 16 for firecracker; `nat-prefix`: `192.168.240.0/20`; `host-ip`: `192.168.240.1`; `name`: `windows-server-2025`; `arch`: the host's; the two ages: `winguest`'s; `runtime`: the first of finch, docker, podman that answers `info`, found once per build and kept in its state file). **Changed reader** (a21): the driver resolves it, through the kit, where `cmd/warden` did | These are the values `cmd/warden` and the drivers apply today | The flag, else the setting's environment variable (the two that exist), else the project's `warden.toml`, else the user's config: today's order, applied by the kit to the `args` and `config_files` `warden` passes on. The first level that sets it is the value; it is not merged with a lower level, and a value that does not parse or cannot be used is an error naming where it came from, reported by the driver in its probe (UX-08) |
| Which config files a driver reads: those `warden` names in `config_files`, in that order, and no other | **New** (a21) | `warden` and the driver must read the same files, and a driver that searched by itself could find others | Nothing. A driver started with no request, such as a service, reads none and uses its defaults |
| A flag that neither `warden` nor the selected driver declares | **Changed** (Behaviour 2): refused by `warden` as it parses. Eleven flags were ignored | A provided value that cannot be used is an error (GLOBAL-10), and `warden` cannot place a flag whose arity it does not know (a24) | Nothing: drop the flag or select the driver that takes it |
| A driver's key whose value has the wrong type or cannot be used | **Changed** (Behaviour 13): refused by the driver that owns the key, in its probe, when that driver is selected. Under another driver it is not read; today a wrong TOML type in the seven existing keys fails the load for every driver | Each key has one reader, and one `warden.toml` serves several drivers | Nothing: fix the value |
| Where a driver's keys live: `[driver.<name>]`, in any shape | **New rule** of the contract (a4, open) | It cannot collide with `warden`'s tables or another driver's, and says whose it is | The seven existing keys keep their tables, each read by the driver the specification names for it |
| `<output>/driver.json` | **New** (a19) | Which driver ran belongs with the build's output | Nothing; always written |
| `--keep-vms` leaves what `capabilities.keep` says | Unchanged per driver | Each driver keeps what it keeps today | Nothing |
| Where the container driver's relay comes from | Unchanged lookups, now behind the relay source | It runs the relay that ships with, or is built from, the same tree | `[runtime] relay_image` or `WARDEN_IMAGE`: used as given; an image that cannot be pulled is an error, and the default is not tried (a3) |
| A relay file's published hash | **New**, inert in K1: `Relay.Verify` checks a file that carries one. No K1 source sets one | Nothing publishes relay hashes yet | When a source supplies a hash, a mismatch refuses the build; no other source is tried |

Named lookups and fallbacks (GLOBAL-10, GLOBAL-11):

- **New, named: the driver lookup** (the second row): `PATH`, then the
  directory of `warden`, then a linked driver. It is a search order, not
  a retry. A later place is used only when the earlier ones hold no file
  of that name; the first file found is the driver, and its failure is
  the command's failure. `warden driver list` shows the order's result
  and what it passed over, and `driver.json` what ran. A path from the
  user ends the search before it starts.
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
  `driver/container`. A build detects once and writes the result to its
  state file; `attach` and `reap` use that, so a later command never
  falls through to another runtime because the build's stopped answering.
- **New, named: `reap` without a state file.** When a build died before
  its driver wrote one, `reap` resolves its settings from the record's
  `args` and `config_files` as they are now, then the driver's default,
  and says which it used. Nothing was created before the state file, so
  there is nothing for it to miss.
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
the calling convention, not settings a user sets (UX-07). `ext` started
without `WARDEN_DRIVER_REQUEST` is named no config file and runs with
the driver's defaults. `WARDEN_DRIVER`
gains a path as a value. `WARDEN_CTR_CLI` and `WARDEN_IMAGE` keep their
meaning and their place in the precedence, between the flag and the
files; the container driver reads them through the kit, where
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
redraft makes true or stale; UX-01 and UX-03 change in enforcement only;
and ORCH-03 changes in enforcement and gains one paragraph (a8). The
list was derived again after the review's answers of 2026-11-14: a16 and
a20 change no rule, and a21 changes the last three entries as said
there. No rule was approved in that sitting. Each is approved by name in
the design review
(FLOW-19) and made in Implement (FLOW-14). Each is worded so that it is
true when K1 merges, with four drivers still linked (umbrella Decision 18).

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
the reference docs (UX-01, ORCH-03). One file, `surface.go`, enforces all
three rules, so the edit is a change to each (FLOW-14) and each has an
entry.

ORCH-03's sentence "Configuration precedence stays `flag > warden.toml >
~/.config/warden`" has had one implementation, in `cmd/warden`. It now
has two, `warden`'s for its own keys and the kit's for a driver's, and a
driver written without the kit has a third. The rule should say that all
of them keep it, so that a later change to the kit, or a later driver, is
judged against the rule and not only against the contract.

**Wording:** unchanged for UX-01 and UX-03. UX-01's "The automated part
is a surface diff against a docs diff" and UX-03's "The automated part
flags a removed or renamed surface with no alias or deprecation" stay
word for word. UX-07 and UX-08 are not changed either: the design keeps
both (**Configuration** quotes them), and a driver's probe is "before
any real work begins".

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
> `warden` reads its own flags and keys. A driver reads its own, in its own
> process, from the config files `warden` names to it, with the same precedence
> and before it does any work (UX-08): through the kit (`driver/drivercmd`), or
> as the driver contract requires of a driver written without it (GLOBAL-08). A
> driver's keys live in `[driver.<name>]`; the driver-owned keys that existed
> before the driver contract keep their tables.
>
> The automated part is a config-key diff against a docs diff, over `cmd/warden`
> and the drivers this repository builds.

The paragraph's last sentence states a4's recommended answer; if a4 is
answered otherwise, it is reworded to that answer before it is approved.

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
  is elided: a literal with a non-empty `Flag` yields that flag, and one
  with `Key: "table.key"` or `Key: "driver.name.key"` yields key `key`,
  the last element of the path, which is the identity a `toml` tag gives
  today. Nothing else is read from the driver directories. The check runs
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
  additions, which the docs in the diff cover. `main`'s tier 1 reads the
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

## Amendments to the umbrella design

[`componentization`](componentization.md) is merged and archived, and its
Decisions 1 to 17 are Jeff's. The review's basis changes some of its
sentences. Each is listed with its current and proposed wording, to be
made in this item's PR once the design review approves them (a18). Nothing
here touches the freeze, the program's scope, the track structure or any
other decision; those are FLOW-20's and the umbrella's.

A dated note is added under the umbrella's Decisions heading: "Decisions 4
and 5, seams 1, 6 and 9 and the K1, K2 and S4 rows were reworded by the
design review of `driver-contract` (K1), approved <date>." The date is
filled in when the review approves; the sitting of 2026-11-14 sent the
design back and approved nothing. Line numbers are the umbrella's on
`main` at `b903f65`.

**A. The decisions and seam 9.**

| Where | Current | Proposed |
|---|---|---|
| Decision 4 (116) | "**Drivers become separate binaries on a public Go kit, then separate modules, then separate repositories.** Alternatives: only complete the in-tree contract (does not let an integrator ship a driver); drivers as RPC plugins (a second protocol to version, and no gain over a binary that links the kit). Hyper-V is extracted first as the pilot." | "**Drivers become separate binaries that `warden` runs through a written command contract, then separate modules, then separate repositories.** A public Go kit implements the contract for a driver that wants it; a driver may be written without it, in any language. Alternatives: only complete the in-tree contract (does not let an integrator ship a driver); a Go API that every driver links (an existing `warden` could not run a new driver without being rebuilt); drivers as long-lived RPC plugin processes behind a plugin framework (commands, JSON and the standard streams are enough). The command contract is a second protocol to version, and that cost is accepted. Hyper-V is extracted first as the pilot." |
| Decision 5 (121) | "**A top-level `warden` dispatcher that works like git's CLI.** It finds a driver binary by name and runs it. Alternative: users call driver binaries directly; it lost because driver auto-detection and one entry point are worth keeping." | "**`warden` finds a driver binary the way git finds a subcommand, and stays the orchestrator.** It keeps the session, the collector and the reporting, and calls the driver's commands for primitives: start the relay, create the environment, tear both down. Alternatives: users call driver binaries directly (one entry point and one configuration are worth keeping); a dispatcher that hands the whole build to the driver (every driver would carry the session and a collector, and the collector could not be chosen apart from the driver)." This drops "driver auto-detection" from the reason: `warden` has none today (the driver is the one named, else `container`) and K1's Decision 6 adds none (a16) |
| The seams, intro (262) | "Seams 1 to 8 each get a specification and a black-box conformance suite in the specs repository. Seam 9 is a Go API. Seam 10 is a document." | "Seams 1 to 9 each get a specification and a black-box conformance suite in the specs repository; seam 9's start in this repository (K1) and move there in K2. Seam 10 is a document." Seam 8 still has no phase that writes it; that gap is a11's and this sentence does not close it |
| Seam 1 (268) | "Claim protocol (`relay-claim`) \| Driver and relay \| No \| Written down. The connection it runs over is supplied by the driver (TCP or vsock)." | "Claim protocol (`relay-claim`) \| Claimant (`warden`'s session) and relay \| No \| Written down. The claimant opens the connection at an endpoint the driver reports (TCP in K1; other transports are what remains of S4)." |
| Seam 6 (273) | "Bootstrap config \| Driver and executor, through the relay \| Code only (`bootstrap/`) \| Written down." | "Bootstrap config \| `warden`'s session and executor, through the collector and the relay; the driver reports the guest OS \| Code only (`bootstrap/`) \| Written down." |
| Seam 9 (276) | "Driver contract \| Kit and drivers \| The four-method `driver.Driver` \| New Go API: registration, doctor, setup, images, clean, driver-owned configuration, the relay source and the claim connection; plus the convention by which the dispatcher finds and runs a driver binary." | "Driver contract \| `warden` and a driver binary \| The four-method `driver.Driver` \| New command contract: how `warden` finds a driver binary, and the required and optional commands it runs on it (describe, the build, cleanup after a dead driver, doctor, setup, images, clean, shell, a driver's own), with driver-owned configuration and the claim endpoint. A Go kit implements it; the relay source is the kit's." |

**B. The phases and the order.**

| Where | Current | Proposed |
|---|---|---|
| S4 (298) | "The claim connection: `driver.Session` takes a connection from the driver, not a TCP address. \| Every driver claims through it." | "Mostly absorbed into K1, which settles that `warden` claims at an endpoint the driver reports. What remains: the claim specification's transport section, and a claim endpoint that is not TCP. \| A relay reachable only over a non-TCP endpoint is claimed without a driver-side forward." |
| K1 (304) | "Complete the driver contract in-tree, including the relay source. `cmd/warden` stops naming drivers. `drivertest` exists. \| Adding a driver touches only its own directory." | "The driver command contract, its conformance suite and the kit, including the relay source. `warden` runs a driver binary; the container driver is one, and every driver's build runs through `warden`'s build sequence. \| An unmodified `warden` runs a driver it was not built with, and the suite passes against the container driver." (If a3's option (e) is chosen, "including the relay source" moves to K2's row.) |
| K2 (305) | "Each driver becomes its own Go module and binary in-tree. `warden` becomes the dispatcher. A tier-1 check limits each driver to the kit's public packages. \| The check passes for all five, and `warden` runs each driver as a separate binary." | "Each VM driver becomes a binary and adopts the relay source; then every driver becomes its own Go module in-tree. `warden` links no driver. The driver specification and suite move to the specs repository. A tier-1 check limits each driver to the kit's public packages. \| The check and the suite pass for all five, and `warden` runs each driver as a separate binary." |
| K4 (307) | "This repository holds the kit, the collector, the executor, the dispatcher and the book." | "This repository holds the kit, the collector, the executor, `warden` and the book." |
| K5 (308) | "The dispatcher can report and fetch a newer release of one driver." | "`warden` can report and fetch a newer release of one driver binary." |
| Order, wave 2 (327) | "S2, S3, S4, K1, M2 \| Driver contract complete" | "S2, S3, K1, M2 \| `warden` runs a driver binary; the suite passes" (what remains of S4 moves to wave 3) |
| Order, wave 3 | "K2, M3 \| Modules isolated; unikernel and host mode pass the suites" | "K2, M3, S4 \| Modules isolated; unikernel and host mode pass the suites" |
| What sets the order (335) | "K1 does not depend on the relay and can start as soon as P0 merges." | Unchanged, with one sentence added: "Its specification and suite start in this repository and move to the specs repository in K2." |
| What sets the order (336) | "M6 needs K1's relay source, not driver extraction." | "M6 needs the relay source, not repository extraction: K1's for the container driver, and K2's adoption of it for the four VM drivers." |
| Later, not scheduled (338) | "alternative collectors and a collector repository (seam 2; the collector's storage boundary is designed with the first of them);" | "alternative collectors and a collector repository (seam 2; the collector's storage boundary is designed with the first of them), among them a shared local or remote collector with a session token per build and a client-facing side for opening a session, supplying inputs, watching console and status, and fetching outputs;" (a22) |

**C. The tables and prose that follow from A and B.**

| Where | Current | Proposed |
|---|---|---|
| Component table, Kit (251) | "Session, claim client, ledger reader, script translator, the driver contract, `drivertest`, the `warden` dispatcher, the book" | "`warden` (the orchestrator: session, claim client, collector hand-off), the kit's Go library for drivers, ledger reader, script translator, the book" |
| Component table, Drivers (254) | "One driver, as its own binary built on the kit" | "One driver, as its own binary that speaks the driver contract; the project's own are built on the kit" |
| Rule table, GLOBAL-08 (391) | "The driver contract becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints, the build-link contract and the claim connection. \| K1; S2, S3, S4 for their seams" | "The driver contract (the commands `warden` runs on a driver binary) becomes a supported seam, unstable before 1.0. The seam list gains the truststore endpoints and the build-link contract. \| K1; S2, S3 for their seams" |
| Rule table, BOOT-02 (392) | "BOOT-02, BOOT-05, the `bootstrap.md` mapping \| As in the Bootstrap comparison. \| K2; S2; S3" | "... \| As in the Bootstrap comparison; BOOT-02 as in K1's own comparison. \| K1; S2; S3" |
| Rule table, TEST-05 (395) | "RELAY-05, TEST-05 \| Contract tests for the seams become the specs repository's suites. \| S1" | "... \| S1; K1 for the driver suite" |
| Rule table, ORCH-01 (396) | "Driver parity becomes passing `drivertest`. \| K1" | "Driver parity is held by the driver contract and its suite. \| K1" |
| Rule table, new rows | none | "ORCH-02 \| `warden` stages and records inputs; a driver alters none. \| K1"; "ORCH-04 \| A driver tears down when its input ends; `reap` reaches what a killed driver left. \| K1"; "FLOW-02 \| Names the driver contract where it named `driver.Driver`. \| K1"; "UX-01, UX-03 \| Enforcement only: tier 1 also reads the flags and keys the bundled drivers declare. \| K1"; and "ORCH-03 \| The same enforcement change; and a driver reads its own flags and keys, with the same precedence. \| K1" |
| Risks (415) | "**A public Go API** has to be kept usable for integrators." | "**A command contract** has to be kept usable for integrators, and is a second protocol to version beside the kit's Go API." |
| Defaults and overrides (420) | "which driver the dispatcher picks when none is named and how it finds a driver binary (K2)" | "which driver `warden` picks when none is named and how it finds a driver binary (K1)" |
| Cross-driver impact (427) | "it implements the completed driver contract (K1), becomes a module and a binary (K2), declares its build-link class (S3), supplies its claim connection (S4)" | "its build runs through `warden`'s build sequence (K1), it becomes a binary that speaks the driver contract (K1 for container, K2 for the rest) and a module (K2), declares its build-link class (S3), reports its claim endpoint (K1)" |
| Bootstrap comparison, proposed diagram (469) | "per build, from the driver's own kit version" | "per build, the driver's version" |
| Bootstrap comparison, BOOT-02 row (490) | "It is delivered per build and comes from the kit version the driver was built from. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. \| None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. \| K2" | "It is delivered per build by the driver and matches the version of the driver that delivers it. The relay is versioned separately; the executor and the relay agree through the relay guest API's version. \| None in what is delivered. Each driver binary carries its own executor version. D2 (vz) is unchanged. \| K1 (its own comparison)" |
| Security note (597) | "The kit cannot make an integrator's driver safe; the build-link suite and `drivertest` are what an integrator runs to show theirs is." | "`warden` runs whichever binary of that name it finds, and records it; it cannot make an integrator's driver safe. The build-link suite and the driver suite are what an integrator runs to show theirs is." |
| Validation plan (616) | "`drivertest` and the build-link suite validate each driver." | "The driver suite and the build-link suite validate each driver." |
| Archive summary, Decisions taken (755) | "Drivers as separate binaries on a public Go kit, then modules, then repositories, with Hyper-V as the pilot (4), behind a `warden` dispatcher that works like git's (5)." | "Drivers as separate binaries that `warden` runs through a written command contract, then modules, then repositories, with Hyper-V as the pilot (4); `warden` finds them as git finds a subcommand and stays the orchestrator (5); both reworded by the design review of `driver-contract`, approved <date>." |
| Archive summary, the comparison (770) | "The Bootstrap comparison is accepted for three model changes, BOOT-01 (S3), BOOT-02 (K2) and BOOT-05 (S2), each made later by the item named (a12)." | "The Bootstrap comparison is accepted for two model changes, BOOT-01 (S3) and BOOT-05 (S2), each made later by the item named (a12); BOOT-02's change is made in K1 under K1's own comparison." |
| Archive summary, rule changes (796) | "That includes the three `bootstrap.md` edits, whose comparison is already accepted here (BOOT-07)." | "That includes the BOOT-01 and BOOT-05 edits to `bootstrap.md`, whose comparison is accepted here (BOOT-07); BOOT-02's is accepted in K1's design." |
| Archive summary, Rejected (775) | "Completing only the in-tree driver contract; drivers as RPC plugins; users calling driver binaries directly." | "Completing only the in-tree driver contract; a Go API every driver links; drivers behind an RPC plugin framework; users calling driver binaries directly; a dispatcher that hands the whole build to the driver." |
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
    user's config or `warden`'s command line. Whoever can write a
    directory on the user's `PATH` can already replace `git`, the
    container runtime or `warden` itself, and so can already do
    whatever a driver could. The driver lookup adds no new way in for
    such a writer; it adds one more program they could replace.
  - *What a planted binary can now do.* A `warden-driver-<name>` in any
    absolute `PATH` directory runs in place of the bundled driver of
    that name, on every command, `--help`, `doctor` and `clean`
    included, and as root under `sudo warden` if it is on root's
    `PATH`. It chooses the isolation, the relay and the executor, and
    it receives the claim token. It can therefore run a build with no
    isolation, or with a relay of its own that signs a ledger of its
    choosing. `warden` cannot tell (a19).
  - *What a build context can and cannot do.* A project's `warden.toml`
    can select a driver by name. It cannot name a file, and the current
    directory and relative `PATH` entries are not searched, so a cloned
    repository cannot supply the binary by itself. It can where the
    user's `PATH` holds an absolute entry inside the repository, which
    some setups create (a per-project `bin` added by an environment
    hook): then the repository ships the file and its own `warden.toml`,
    or merely the default driver's name, selects it. This is the
    exposure the old "bundled names never from `PATH`" rule closed for
    the five bundled names, and it is now open by decision. A user in
    that position passes `--driver <path>` or keeps such directories
    off `PATH`; the reference docs say so (UX-01).
  - *What shows it.* `warden driver list` shows which file runs for
    each name and every copy it shadows; every build's `driver.json`
    records the path, the file's digest, where it was found and what it
    shadowed (a19). Both are for the person who ran the build. Neither
    is signed, and neither prevents anything.
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
- **A driver reads the user's configuration itself** (a21). The driver's
  process is named the project's `warden.toml` and the user's config
  file and parses them. That gives it nothing new: it runs as the user
  and could read both. What changes is who interprets a value. A key
  under `[driver.<name>]`, and each of the seven existing driver-owned
  keys, is read and acted on by the driver with no check by `warden`,
  the project's file included; `[runtime] cli`, which names a program to
  run, is such a key today and stays one (**Risks**). A driver's flags
  travel in the request, not in its argv, and are kept in the build's
  record, so the contract tells a driver to take no secret as a flag.
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

- Lookup: a `warden-driver-container` in the current directory, or in a
  relative or empty `PATH` entry, is not run; one in an absolute `PATH`
  entry is run in place of the bundled copy, and `warden driver list`
  and `driver.json` name it and the copy it shadows; `[runtime] driver
  = "./x"` or an absolute path in the project's `warden.toml` is refused
  before any driver runs, for `--help` too, and the same value in the
  user's config file is accepted when absolute; `[runtime] driver = "x"`
  in the project's file selects `warden-driver-x` by lookup; on Windows
  a `warden-driver-x.bat` is not taken for the driver. No case asserts
  an owner or a mode: those checks are gone (a16).
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
  directory.
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
  with a bundled name shadowed from `PATH` and a linked name replaced by
  a file; a directory, a file with no execute bit and a relative `PATH`
  entry passed over; a symbolic link followed; the project-file rule and
  the relative path in the user's config; `driver list` and its
  `shadowed` entries; the result and request files, every
  exit code, a line that is not JSON, a driver that exits early, a path
  that is not UTF-8, the teardown wait (its default, a value from the
  project's file over the user's, a value that does not parse and one
  out of range, each refused at load) and the 10 s and 30 s limits with
  a fake clock.
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
  and before the subcommand's name; the `args` built from a command
  line, in order and with repeats; a flag nobody declares refused before
  any driver command but `describe`; `--runtime` on a command that
  selects no driver; `config_files` in precedence order and without a
  missing file; a `describe` with a key outside the driver's table
  refused; `--help` for a driver that cannot be described; and that
  `attach` and `reap` receive the build's `args` and `config_files` from
  its record.
- Configuration, the kit's side (`drivercmd.Resolve`), per bundled
  driver: precedence (flag, variable, project file, user file) over the
  rows of today's request tables; a value of the wrong type refused with
  the file and key named; an undeclared argument refused; another
  driver's table and `warden`'s tables ignored; no file read that was
  not named; a `Raw` key decoded whole from one file; and the same
  result through `Main` and through `Call`, so the linked drivers and
  the container binary are shown to read alike.
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
  from `cmd/warden` into `driver/container`.

**Tier 1** (the `integration` workflow, Docker): `make integration-test` as
today, now through `warden-driver-container`, and the suite's lifecycle
tier against it. The workflow's `paths` gain `driver/drivercmd/**`,
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
| macos-finch | (1) `warden driver list` shows `container` at the built `warden-driver-container` and the four linked drivers; `warden doctor` prints `container runtime: finch` and `ok`. (2) A default build succeeds and its ledger verifies; `driver.json` names the driver binary and its digest; `inputs/` holds `Containerfile` and `build.sh`. (3) `warden shell` opens a shell and passes its exit code through. (4) `[runtime] relay_image = "dev"` builds the relay from source; `WARDEN_IMAGE=<ref>` pulls that image; an unpullable reference fails without trying the default. (5) Ctrl-C mid-build tears down and leaves nothing; SIGTERM mid-build does the same and exits 1; with the driver killed mid-build `warden` fails the build and reaps; with `warden` killed (SIGKILL) mid-build the driver tears down by itself and no container of the build is left. (6) With `warden` and the driver both killed, the build's containers are still running; `warden clean`, run with another `--tmpdir` than the build's, removes that build's containers, network, work directory, stage directory, lock and record, and leaves a running build's; a running build started with another `HOME`, whose lock is elsewhere, is left alone too. (7) `WARDEN_CTR_CLI` and `--runtime` select the runtime; a missing one gives the fixable error and leaves no output directory; `--network x` is refused by `warden` as an unknown flag for the container driver; `[runtime] cli = 5` in `warden.toml` is refused by the driver, naming the file, and leaves no output directory. (8) A copy of `warden-driver-container` placed in a directory on `PATH` runs in place of the one beside `warden`: `warden driver list` shows it and the copy it shadows, and the build's `driver.json` has `found: PATH` and its digest; `[runtime] driver = "./x"` in the project's `warden.toml` is refused. (9) `make driver-conformance DRIVER=dist/warden-driver-container` passes both tiers on finch |
| container-any | On the Windows host, with its container runtime: (1) `warden driver list` shows `warden-driver-container.exe`; `warden doctor` reports the runtime and `ok`. (2) A default build succeeds, its ledger verifies, `driver.json` is written. (3) Ctrl-C mid-build tears down; with the driver killed `warden` reaps; with `warden` killed the driver tears down on the end of its input. (4) `warden clean` leaves a running build and removes a build whose `warden` and driver were both killed, from its record. (5) A `warden-driver-container.bat` beside `warden` or on `PATH` is not taken for the driver; a `warden-driver-container.exe` on `PATH` replaces the one beside `warden`. (6) Both tiers of the suite pass against `warden-driver-container.exe` |
| macos-vz | (1) `warden doctor --driver vz` and `warden image list` print what `main` prints. (2) A vz build succeeds, its ledger verifies, `relay.log` is written, `driver.json` says `linked`; with a Containerfile in the context, it is ignored and `inputs/` is absent, as on `main`. (3) A script that exits 3 makes `warden` exit 3; `--timeout 30s` on a sleeping script exits 124. (4) Ctrl-C mid-build leaves no relay process and no work directory; SIGTERM mid-build does the same and exits 1, where on `main` it kills `warden`. (5) `--runtime docker` and `--network x` with `--driver vz` are each refused as an unknown flag for driver `vz`; `[runtime] cli = 5` in `warden.toml` does not stop a vz build (Behaviour 13); `warden clean --driver vz` reports nothing to clean and needs no container runtime |
| windows-hyperv | (1) `warden doctor --driver hyperv`, elevated and not, prints what `main` prints; `--network` is honoured. (2) Elevated `warden setup --driver hyperv --slots 2` is idempotent. (3) A non-elevated durable build and an elevated ephemeral build succeed and verify; `[vm] network` is honoured and `--network` overrides it; with `[vm] image_refresh_after` set low, the detached `image prepare --driver hyperv` starts and logs. (4) With the service installed, a service-delegated build succeeds. (5) A build whose script powers the VM off without completing fails with the build-VM-off error about 10 s after the VM stops. (6) Ctrl-C mid-build removes both VMs and the switch; the time from Ctrl-C to exit is recorded, to judge the 60 s default of the teardown wait against a real teardown before K2 applies it to Hyper-V (a20). (7) `--keep-vms` leaves both VMs; `warden clean --driver hyperv` then removes them and leaves a running build's |
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
each task leaves `make test` and `make lint` green (FLOW-17).

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
    pages. It includes the build-lock convention, the failure table, the
    probe form of `start`, commands not started by `warden`, UTF-8 paths,
    teardown with closed output, the configuration requirements C1 to C9
    (`config_files`, `args`, `describe`'s `flags` and `keys`, the
    precedence, the place of a driver's keys and the seven existing
    ones), `warden`'s reserved flag names, and an introduction that says
    what a
    driver needs besides this contract (GLOBAL-08, RELAY-05's
    `schema-doc` pairing, UX-01).
13. Add `driver/drivercmd` to match the specification, with doc comments
    on every exported identifier: dispatch through `Main` and `Call`,
    `Preflight` for a probe, `Setting`, `Resolve` and `driver.Settings`
    (the flags from `args`, the two existing variables, the keys from
    `config_files`, with `warden`'s precedence), the state file helpers,
    and `run` ignoring SIGPIPE (GLOBAL-01, GLOBAL-03, GLOBAL-08,
    GLOBAL-10, ORCH-03, ORCH-04, UX-08).
14. Add `conformance/driver/`: `warden-driver-fake` with all eleven
    commands and a flag and a key read by hand, `stubrelay`, the suite's
    static tier with the configuration checks, its self-test with the
    fault switches, and the imports test; run the static tier under `make
    test` (TEST-01, TEST-05, GLOBAL-07).
15. Add the `driver-contract` job to `.github/workflows/main.yml` (three
    platforms) and to `ci-ok`'s `needs`, leaving the `test` job as it is;
    run `cmd/warden`'s and `driver`'s existing tests on macOS and Windows
    first and fix or build-tag what fails there (TEST-01, GLOBAL-07,
    FLOW-14).
16. Change tier 1's surface extraction as approved, to read
    `drivercmd.Setting` literals, with its tests; leave `rules.yaml`'s
    `surface` keys as they are (UX-01, UX-03, ORCH-03, FLOW-14, LINT-05).
17. Add the lookup (`driverlookup.go`: a path, `PATH`, `warden`'s
    directory, a linked driver) with the project-file rule, the exec
    transport (`drivercall.go`) for every
    command of the contract (`run` and its probe, `reap`, `doctor`,
    `setup`, the image commands, `clean`, `attach`, and `ext` as `warden
    <name> ...`), the refusal of a path that is not UTF-8, `warden driver
    list`, `driver.json` and the teardown wait read from `[build]
    teardown_timeout` (GLOBAL-06, GLOBAL-07, GLOBAL-10, GLOBAL-11,
    UX-02, UX-04, UX-09, ORCH-04, ORCH-05).
18. Move every driver-owned flag and key into its driver's `Setting`
    declaration; `warden` finds the driver before parsing flags,
    registers the flags the selected driver declares, refuses one nobody
    declares, and passes the driver's on in `args` with `config_files`;
    each driver reads its settings through the kit, the linked ones
    through `Call` in `warden`'s process, and the VM drivers' doctor,
    setup, image and clean code through `Resolve`; the seven driver-owned
    fields and the two variables leave `Config` and `LoadConfig`. Update
    the goldens for the seventeen differences only (ORCH-03, UX-03,
    UX-07, UX-08, GLOBAL-10).
19. Add `warden`'s end-to-end tests against the fake, for every command,
    run on three platforms by the `driver-contract` job (TEST-01, TEST-04,
    UX-09, ORCH-04).
20. Add `cmd/warden-driver-container`: move the container driver's doctor,
    runtime detection and cleanup out of `cmd/warden`; its `Preflight`;
    the runtime written to the state file; `reap` (exit 1 when the runtime
    does not answer) and `clean` with `keep_ids` beside the process scan;
    take it out of `linked.go`; `warden clean` cleans up after dead builds
    from their records (ORCH-01, ORCH-04, GLOBAL-07, GLOBAL-11).
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
    `firecracker` workflow's cases (TEST-02, TEST-05, TEST-07).
24. Make the approved rule edits: GLOBAL-08 in `global.md`; ORCH-01,
    ORCH-02, ORCH-03 and ORCH-04 in `orchestrator.md`; TEST-05 in
    `testing.md`;
    BOOT-02 and the diagram in `bootstrap.md`; FLOW-02 in `process.md`;
    `seams`, `drivers.shared` and the TEST-05 owner in `rules.yaml`
    (FLOW-14, BOOT-07).
25. Make the approved amendments to `docs/design/componentization.md`
    (FLOW-12, FLOW-20).
26. Update `guide/cli.md`, `guide/configuration.md`, `guide/output.md` and
    `guide/drivers.md` for the seventeen differences, the lookup, what a
    file on `PATH` replaces, `[driver.<name>]` and `[build]
    teardown_timeout`; `concepts/overview.md` and
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
  path, then `PATH`, then the bundled copy (a file beside `warden`, then
  a linked driver, which is last so that a file always beats code
  compiled into `warden`). The owner, mode and directory checks, the
  rule that a bundled name never comes from `PATH` and the rule that a
  project file selects only a bundled driver are gone. Kept: a project's
  `warden.toml` names a driver and never a file. Kept for correctness
  and said to be so: a named file must be one that can be run, relative
  `PATH` entries are skipped as Go's `os/exec` requires, and on Windows
  only a `.exe` is a candidate. `warden driver list` and `driver.json`
  show a shadowed bundled copy. Also changed: the Security note (his
  reasoning in his words, and what a planted binary can now do),
  Defaults and overrides, the prior-art row on git, Behaviour 4, 12 and
  the new 16, the tier-0 and TEST-08 cases that asserted the old guards,
  the macos-finch and container-any cases, and Risks. The umbrella
  amendment for its Decision 5 did not speak of the guards and is
  unchanged.
- **a21, who reads the configuration: (b), "The kit reads it in the
  driver".** *Folded in:* Decision 7 and **Configuration** are rewritten.
  `warden` reads its own flags and keys and hands a driver the config
  files' paths in precedence order, the driver's flags and the
  environment; the kit reads them in the driver's process with
  `warden`'s precedence, and in `warden`'s process for the four linked
  drivers through the same entry, so there is one path. `describe`
  declares a driver's flags with their arity, for parsing and help
  (Decision 22), and its keys, for the placement rule; `options` leaves
  `describe` and every request, which carry `config_files` and `args`
  in its place; the build record keeps those two. The requirements on a
  driver written without the kit are numbered C1 to C9, with what the
  suite can and cannot check. Also changed: the kit's API, the suite's
  static tier and the fake, the errors table, help, Behaviour 1, 3 and
  13, What guards behaviour, Risks (the costs, stated), Defaults and
  overrides, the Security note, the overview, the ORCH-03 rule change
  (now one paragraph of wording as well as enforcement), the validation
  and implementation plans, and the front matter. The seven
  driver-owned fields leave `Config`.
- **a20, time limits: (c), with the 60 s teardown wait made a key.**
  *Folded in:* Decision 21. The wait is `warden`'s key `[build]
  teardown_timeout`, a Go duration from `1s` to `1h`, `60s` when unset,
  checked as the files load, with no flag. It is `warden`'s and not the
  driver's because it is the caller's wait and bounds a driver that has
  stopped answering. Every other limit stays fixed. Also changed: the
  signals text, the failure table, Defaults and overrides, Behaviour 8
  and the new 17, and the tier-0 cases.

Still to come in this sitting: a5 (the behaviour that changes, now
seventeen items), a18 (the umbrella's wording, sentence by sentence), a8
(the rule changes, each by name, and the Bootstrap comparison), a4 and
a9 (both derived again after a21's answer), and a24, which a21's answer
raised (how `warden` tells a driver's flags from its own); then the
outcome.

## Consolidation

## Rollback notes

## Archive summary

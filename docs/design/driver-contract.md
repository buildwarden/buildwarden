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
  refused, where eleven flags were silently ignored; the existing refusals
  take one generated form; `warden clean --driver qemu|vz` no longer cleans
  containers; `image list` and two messages name drivers in list order; the
  long help of `image prepare` and `clean` groups each driver's text.
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

### Shape

```
cmd/warden/                         the frame: commands, config, flags, output
  drivers.go                        the list of in-tree drivers and the default
                                    driver's name; the only file that names one
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
today. No name, table or variable changes.

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
   their usage strings with `"; "`. A clash of kinds, or with a frame flag,
   is a start-up panic that `drivertest` catches first. Help output is
   therefore still the whole set, whichever driver is selected, as today.
2. **Select the driver** from `--driver`, `WARDEN_DRIVER`, `[runtime]
   driver`, else `defaultDriver`; an unknown name is today's error.
3. **Refuse** a flag that was set and that the selected driver does not
   declare for this command: `--network is only used by --driver hyperv`,
   with the fix `drop --network, or use --driver hyperv`. Config keys and
   environment variables are never refused: one `warden.toml` serves several
   drivers. A VM-group flag on a driver without `Info.VM` keeps today's
   message.
4. **Resolve** each of the selected driver's options: the flag when set, else
   its environment variable, else `warden.toml`, else
   `~/.config/warden/config.toml`, else unset. That is today's order
   (`LoadConfig` applies the environment over the files, and a flag over
   both), and the value found is used exclusively (GLOBAL-10). A value that
   does not parse as the option's kind is refused here, before any driver
   work (UX-08), naming the flag or the file and key.
5. **Call** the driver with the `Values`.

The frame reads config keys by decoding each file a second time into a
generic tree, so a key a driver declares needs no field in `Config`. The
driver-owned fields leave `Config` (`RuntimeConfig.CLI` and `RelayImage`,
`VMConfig.Network`, `ImageRefreshAfter`, `ImageMinGrace`, `WindowsImage`,
`WindowsImageUpdate`). Unknown keys are still ignored, as today.

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
| The per-driver paragraphs in the long help of `doctor`, `setup`, `image prepare` and `clean`, and the driver example lines | `Info.Help`, `Info.Examples`, in list order |

### The operations, per driver

What each driver does today for each operation, and where it goes. "Frame
preamble" is `doctorBuildLink` (the build link and its source, the host
overlap check, the launcher stamps); it becomes a kit helper,
`driver.ReportBuildLink`, that each VM driver's `Doctor` calls first, so the
output keeps its order. `doctorResolvers` becomes `driver.ReportResolvers`.

| | container | qemu | vz | hyperv | firecracker |
|---|---|---|---|---|---|
| **Runs on** (build tags) | any host with a runtime; no tagged files | any host; `vm_unix.go`, `vm_windows.go` | macOS/arm64: `vz_darwin_arm64.go` (cgo); `vz_stub.go` elsewhere | Windows: `*_windows.go`; `*_other.go` and `driver_stub.go` elsewhere | Linux/amd64: `*_linux.go`; `driver_other.go` elsewhere |
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
as that driver's internals behind its contract methods. Also moving:

- `cmd/warden/main_darwin_arm64.go`'s `runtime.LockOSThread` goes to an
  `init` in `driver/vz/vz_darwin_arm64.go`; an `init` runs on the main
  goroutine, so the effect is the same.
- `relayvm.QEMURelay`, `QEMUBuild` and `Firecracker` go to `driver/qemu` and
  `driver/firecracker`; `relayvm` keeps `Asset`, `Find`, `StampPath` and
  `RelayInitrd`.
- `driver.ContainerBaseCIDR`, `WardenBaseNet` and `AllocateNetwork` go to
  `driver/container`, which declares the range in `ReservedRanges`; qemu
  declares `UserNetCIDR` and firecracker `UpstreamCIDR`.
- `driver/buildlink/disjoint_test.go` moves to `cmd/warden`, which links
  every driver; `driver/claimlink_test.go` scans every `driver/*/` directory
  instead of a list.

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

The types, in `driver/relay.go`:

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
// the relay binary as driver.GuestBinary does.
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
| firecracker | `appliance` (the same roles) | `relaysource.Bundled` | none |
| vz | `host-binary` | `relaysource.Bundled` with a `GoBuild` for darwin/arm64 | none |
| hyperv | `appliance` (one boot disk) | A source in `driver/hyperv` over the cache path | `WARDEN_RELAY_BOOT_VHDX` set: `relaysource.Path` |

What this gives M6. Adopting a Rust release is a change inside one driver's
directory: its `RelayForms`, the source its build resolves, and the boot
code for a form it has not run before. A pinned source (version, URL and
published hash, downloaded to the cache and verified through `SHA256`) is
one more implementation of `RelaySource`; its manifest and hash format are
seam 8's, which is not written yet (a3). Nothing in K1 needs the Rust relay:
every source resolves the Go relay as today. A Rust appliance that carries
its relay inside the initramfs is an `appliance` with no `binary` role, and
the qemu and firecracker start code appends a binary only when the role is
present.

The relay source adds no relay setting. What it chooses is the artifact and
its launch wiring (`--mode`, `--fd`), which RELAY-08 calls "how the relay is
started, not configuration for a build". Every setting still travels in the
claim; the boot channel carries what it carries today.

Nothing new is recorded in the ledger. Whether a build's record should name
the relay's form and version is left to seam 8 and M6.

### The boundary with S4

K1 stops at "the relay is running, unconfigured". S4 starts at "the driver
hands the session a connection to it". K1 leaves `Session.ClaimRelay(ctx,
addr, claimToken, req, env, link, oldRelayFix)` exactly as it is, and
changes no claim call's arguments. The contract document says a driver
claims its relay through `driver.Session` and names S4 as the item that
defines the connection. Both items edit each driver's relay-start function
(K1 its input, S4 its output); the second to publish merges them (a2).

### `drivertest`

`driver/drivertest`, a public package of the kit, with two suites and one
fake.

**`drivertest.Contract(t, d)`** needs no hypervisor, runtime or network and
runs on every host. It checks what a driver declares and what it does with
an empty cache:

- `Info`: a name of lowercase letters and digits; the same value on every
  call; `GuestOS` and `ImageOS` well formed; `ReservedRanges` that parse and
  do not overlap the default build link; `Help` and `Examples` keyed by
  known commands.
- `Options`: unique names; no clash with a frame flag or key; a known kind;
  usage text present (UX-01); a config key in `[driver.<name>]`, or one of
  the seven keys that predate the rule; an environment variable only for the
  two that exist.
- Consistency: `ImageOS` is set exactly when the driver implements `Images`;
  an option on `setup`, `image prepare` or `clean` needs `HostSetup`,
  `Images` or `Cleaner`; `RelayForms` is not empty.
- With the cache, temp and home directories pointed at empty ones:
  `ListImages` writes nothing and returns 0, and `LayerCache` is nil or a
  cache under the cache directory.

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
| Shell | `Exec` returns `ErrExecNotSupported`, or, where `opts` says it is supported, runs and reports the shell's exit code as an `ExitError` | UX-09 |
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
binary first, as `integration-test-vz` does today). Where it runs:

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

Six things change for a user (a5). Everything else is meant to be identical.

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
   adjacent, and `clean`'s two driver sentences swap. `doctor` and `setup`
   read as they do now.

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
  `buildlink_test.go`); and `image list` and `image verify` over fixture
  caches. The refactor's commits may change a golden only for the six
  differences above, so each shows up as a reviewed golden diff.
- **The surface check.** Tier 1's extraction (Rule changes) must find the
  same set of flags and keys on the branch as on `main`. A flag or key lost
  in the move fails `surface-removed`.
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
2. The contract types, `driver/relaysource`, `driver/drivertest` with the
   recorder, and the tier-1 surface change. Nothing uses them yet.
3. The frame. `cmd/warden` is rewritten over `[]driver.Driver`. A temporary
   `cmd/warden/legacy.go` adapts each of the five packages to the contract
   by wrapping the exported functions `cmd/warden` calls today, so no driver
   package changes in this commit. The six behaviour differences appear
   here, and only here.
4. to 8. One driver per commit: container, firecracker, qemu, vz, hyperv
   (the two CI can run end to end first). Each moves that driver's logic out
   of `cmd/warden` into its own directory, implements the contract there,
   routes its relay start through a relay source, adds its `drivertest`
   tests, and deletes its adapter from `legacy.go`. `BuildRequest` loses a
   field in the commit that converts its last reader.
9. Delete `legacy.go`. Move the driver names out of the shared packages. Add
   the test that no Go file outside `driver/<name>/` imports or names a
   driver package except `cmd/warden/drivers.go`.
10. The rule text (GLOBAL-08, ORCH-01), `rules.yaml`, the contract document,
    the reference docs, `structure.md` and `overview.md`.

Because each driver is converted in its own commit against a frame that is
already final, a failure found in one driver's native validation is fixed
in that driver's directory.

### What K1 leaves for K2

- The frame is still package `main`. K2 moves it into the kit and builds one
  binary per driver over it.
- Flags are the union of every linked driver's. With one driver per binary
  the union is that driver's.
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

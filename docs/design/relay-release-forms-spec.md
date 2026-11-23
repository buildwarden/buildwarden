---
feature: relay-release-forms-spec
status: draft
drivers: [interface]     # the driver contract gains the compatible-shapes declaration and the
                         # preference key (seam 9). No driver's behaviour changes: none declares
                         # or reads them before its M6 item; the container driver's code changes
                         # only by the kit's renamed identifiers (Cross-driver impact)
seams: [relay-release-forms, driver-contract, ledger-format]
                         # seam 8, written for the first time, in the specs repository; seam 9,
                         # specs/driver-contract/v1.md and its suite; seam 4, one optional member
                         # of the environment metadata (Specification changes)
surfaces: [config]       # one common key of the driver contract, relay_shapes, documented in the
                         # book's configuration page; no driver reads it yet (UX-01)
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
                         # home first: the driver contract, its suite, the kit, the environment
                         # metadata, the umbrella's amendments and this design; the second holds
                         # seam 8's specification and suite, and has no area map, so it is one
                         # exclusive area (FLOW-05)
merge_first: [buildwarden/buildwarden-specs]
                         # the driver contract cites seam 8's document, which merges on the specs
                         # program trunk first
areas: [designs, specs, conformance, driver-core, driver/container, cli, book]
                         # designs: this design and the umbrella's amendments; specs:
                         # specs/driver-contract/ and specs/metadata-environment/; conformance:
                         # conformance/driver/ (cases and the fake); driver-core:
                         # driver/relaysource/ and driver/drivercmd/; driver/container: the
                         # kit's renamed identifiers in two files; cli: the copy of the driver
                         # contract's schema that cmd/warden embeds; book: the common key in
                         # docs/book/src/guide/configuration.md
security_boundary: none  # no code on a boundary changes behaviour: no driver resolves its relay
                         # through the new kit code before its M6 item. The trust root and the
                         # environment requirements this design states set M6's posture, so they
                         # are flagged (b1, b3; Security note)
alters_relationships: true   # seam 8 becomes a contract between relay releases and every driver;
                         # building a relay's environment moves from relay releases to drivers'
                         # recipes; the driver contract gains the relay's shapes
design_review: required  # the umbrella test's U1 and U2 fail: the ownership of a relay's
                         # environment moves to the drivers, and b1 to b8 are component design,
                         # b14 privileged work, all flagged (Design, The umbrella test)
native_validation: []    # nothing runs natively: the suites run in Linux CI, and no driver's
                         # behaviour changes (Validation plan)
tier2_gaps: []           # no implementation of seam 8 exists yet; what no case checks is listed
                         # in each specification's not-machine-checked section (Validation plan)
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-02, RELAY-03,
        RELAY-04, RELAY-05, RELAY-07, RELAY-08, BOOT-01, BOOT-06, BOOT-07, ORCH-01, UX-01, UX-04,
        TEST-02, TEST-03, TEST-05, FLOW-01, FLOW-02, FLOW-03, FLOW-05, FLOW-06, FLOW-07, FLOW-08,
        FLOW-09, FLOW-12, FLOW-14, FLOW-19, FLOW-20, LOOP-02, LOOP-04, LOOP-07, LOOP-08]
rule_changes: []         # no rule moves here: RELAY-04's restatement stays M6's, and GLOBAL-08's
                         # list of seams the adoption item's (Rule changes)
concepts: [relay-release-forms, release-manifest, published-hash, native-binary, relay-shape,
           self-built, relay-recipe, shape-preference, relay-launch-interface,
           relay-environment-requirements, relay-source, driver-contract, environment-metadata,
           seams, specs-repo, conformance, program]
related: [componentization.md, driver-contract.md, seam-specs.md, program-north-star.md,
          hyperv-relay-per-build-channel.md, firecracker-driver.md, unified-bootstrap.md]
---

# Seam 8: the relay's release, its native binary and its shapes

## Problem

The componentization umbrella lists seam 8, the relay's release forms, among
the seams that get a specification and a conformance suite in
`buildwarden/buildwarden-specs`: "each form (host-mode binary per platform,
unikernel image, appliance image, OCI image) with a published hash, and the
devices a relay VM needs. The list is open-ended." No phase row writes it.
`driver-contract` (K1) raised the gap as its a11, and Jeff answered (a) on
2026-11-14: K1 defines four form names and four file roles as provisional Go
constants of the kit (`driver/relaysource`), and seam 8 is its own S-track
item, "to exist before M6 adopts a release". This is that item.

Its first design (read at `40f6c52`) wrote the four forms down as the seam,
each with its launch interface and a relay VM's device list. Jeff sent it
back in review r1 (2026-11-23) to start over from first principles: *drivers
dictate the relay environment they need to enforce the boundary on their
build environment.* The relay's release is a native binary per platform,
always; every other shape a relay can take is an optional prebuilt
convenience, specified when the relay can output it, and a driver that finds
none it can use builds its own environment around the binary. His notes are
quoted in full under **Design review**, r1.

What is missing, concretely:

- **No release to pin.** Nothing names a relay release or publishes a hash
  for it. K1's `RelayFile.SHA256` and `Relay.Verify` exist and check
  nothing: no driver checks a hash, and the container driver's default is
  the moving tag `ghcr.io/buildwarden/relay:latest`. K1 left the pinned
  source's "manifest and hash format" to seam 8.
- **No written interface for the binary.** How a launcher starts the relay
  executable (its mode, its claim address, its frame socket, its claim
  token) is code in five drivers and the relay VM's init.
- **No statement of what the relay needs from its environment.** The
  firecracker and qemu relay VMs carry an entropy device so that the CRNG is
  seeded before the relay makes its keys (RELAY-03), and the M0 spike found
  that Hermit fails closed without one; nothing says that every environment
  a relay runs in must give it one, or what else it must give.
- **No way for a driver to say which shapes it can run, or for a user to
  choose.** K1's `RelayForm` is a closed list of four names that no release
  publishes, and the driver contract says nothing of the relay's shape.
- **No record of which relay ran.** K1 left "whether a build's record should
  name the relay's form and version" to seam 8 and M6.

Today's drivers already build their relay's environment themselves: qemu
and firecracker boot an Alpine kernel and initramfs with the Go relay
appended per build, Hyper-V builds its boot disk on the host
(`refresh-cache.sh`), vz runs the binary as a host process, and the
container driver's development build makes an image from the binary
(`driver-contract`, The relay source). What they lack is a release to build
from, a pin to check it against and an interface to start it on.

M6 is wave 5, but the relay's M2 and M3 produce releases now, and the relay
repository's profile is to declare seam 8 among its consumables (LOOP-08
step 5). They need the core before their first release.

## Prior art

- [`componentization`](componentization.md): seam 8's row; Decisions 3
  (specifications with black-box suites in the specs repository), 11 (native
  binaries usable at host level on every supported OS and architecture, and
  unikernel images; more forms may follow), 12 (the appliance and the OCI
  image on the kernel network path; the unikernel and host mode on the
  relay's own stack); a3 (host mode Linux first, then macOS); the
  Cross-driver table's column "Relay form expected at M6", which this design
  amends (**Amendments to the umbrella design**). Its Defaults and overrides
  names "which relay release form each driver uses and how it is
  overridden (K1, M6)" as a default the program adds; Decision 8 is it.
- [`driver-contract`](driver-contract.md) (K1): Decision 11 and a3 (the relay
  source is a kit library; "K1 adds no relay key", which this design departs
  from on Jeff's r1 note (8), b6); a11 (this item); the relay source's
  per-driver table, the evidence that drivers already self-build; the
  precedence and the common keys (DRV-C4, DRV-C8), which the preference key
  follows; its **Amendments to the umbrella design**, whose form this
  design's amendments take.
- [`seam-specs`](seam-specs.md) (S1): the specs repository's conventions
  (document sections, numbered requirements, `not-machine-checked.txt`,
  `cases/`, `bw-conformance-<directory>`, target files, the Linux sandbox,
  the `exec` launcher, the in-process attachment, per-directory tags),
  reused as they are (Decision 1).
- [`hyperv-relay-per-build-channel`](hyperv-relay-per-build-channel.md):
  RELAY-08's pre-claim values and BOOT-06's boot channel, the relay seed and
  the `warden.<key>=<value>` words. Under this design those are a VM
  environment's channel, which a recipe or a shape's document defines; the
  binary's own interface (Decision 5) is what the channel feeds.
- [`firecracker-driver`](firecracker-driver.md): the entropy device on the
  relay VM "so the CRNG seeds before the relay makes its keys" (RELAY-03),
  reused as Decision 6's first requirement.
- [`unified-bootstrap`](unified-bootstrap.md): BOOT-06 as it stands. This
  item changes no BOOT rule.
- [`program-north-star`](program-north-star.md): the umbrella test, the
  question kinds on a program item (FLOW-07, FLOW-19, FLOW-20), the
  pre-release tags `<directory>/vX.Y.Z-dev.N`.
- `prebuilt-artifacts` (approved at `7a85a33`, held): "how a relay states its
  version becomes seam 8's to say" (b10).
- The VZ spike report, `spike/REPORT.md` on `buildwarden-relay` `main`: a
  virtio entropy device on every relay VM, the kernel refusing to produce
  random data without one.
- Python's source distribution and binary wheels, the analogy Jeff's r1 note
  uses: the source is always published, wheels are optional, and an
  installer that finds no wheel for its platform builds from the source,
  saying so. Decisions 2, 7 and 8 follow it, with the difference that a
  self-built relay is not compiled: the binary is the same, and only its
  environment is built.
- Standards: GLOBAL-08 (versioned seams), GLOBAL-09 to GLOBAL-11 (the
  preference list's default and its named fallback), RELAY-03 (keys made in
  memory at start, which needs entropy), RELAY-04 (three modes), RELAY-07
  (no build state on the relay's disk), RELAY-08 and BOOT-06 (pre-claim
  values only on the boot channel), TEST-05 (seams tested by the specs
  repository's suites), LOOP-02 (specification first), LOOP-07 (adoption by
  a reconciliation item).

## Decisions

1. **Seam 8 is one specification, `relay-release-forms`, in the specs
   repository, and version 1 is its core only.** Requirement prefix `FORM`;
   two roles, the *release* (what the relay's release publishes) and the
   *launcher* (a driver, the harness, or an integrator's tool that starts a
   relay from a release). The core is the manifest and its published hash
   (Decisions 3 and 4), the native binaries and their roles (Decision 2), the
   binary's launch interface (Decision 5) and what the relay requires of any
   environment it runs in (Decision 6). It defines no shape (Decision 7).
   The directory has the document `v1.md`, the manifest's schema
   `v1.manifest.schema.json`, `not-machine-checked.txt`, the suite in
   `cases/`, `bw-conformance-relay-release-forms` and a `CHANGELOG.md` from
   `v0.1.0`.
   - *Rejected: version 1 with the four forms of K1 and their devices*, the
     first design. Review r1: a shape is specified only once the relay's
     own release can output it, and none can today.
   - *Rejected: a section of `relay-claim`.* That document puts "who starts
     a relay" outside its scope, and a claimant is not a launcher.

2. **The native binary per operating system and architecture is the
   source distribution: every release carries it.** A release publishes the
   relay executable for each platform it supports, as a plain file (not an
   archive), with the role `binary`. It MUST carry `linux/amd64` and
   `linux/arm64`, the platforms of every relay environment a driver builds
   (a container, a Linux VM) and of host mode on Linux; it MAY carry
   others (`darwin/arm64` once host mode reaches macOS, umbrella a3). Each
   Linux binary is statically linked and needs no file but itself, so that
   a recipe can put it in an empty root file system. Platform names are Go's
   (`linux`, `darwin`, `windows`; `amd64`, `arm64`), as the specs
   repository's own binaries use. The manifest says, per binary, which
   modes it runs (`host`, `container`, `vm`). Roles are an open
   vocabulary: version 1 defines `binary`, a shape's document may define
   more, and a launcher ignores a file whose role it does not use (b4).
   - *Rejected: archives (`.tar.gz`) as the released files.* A launcher
     would unpack before it can hash what it runs, and the hash would name
     the archive, not the executable.
   - *Rejected: a binary per mode.* The relay chooses its mode from its
     arguments; one executable per platform is what is tested.

3. **A release is described by one manifest.** `relay-release.json`, valid
   against `v1.manifest.schema.json`, published beside the release's files.
   Its members: `manifest_version` (1); `relay` (the implementation's name)
   and `version`; `specs`, the specification releases the relay implements
   (`relay-claim/v0.1.0`, ...); `binaries`, each with `os`, `arch`, `modes`,
   the least memory it runs in, and its file (a name relative to the
   manifest, the size, the SHA-256); and `shapes`, an open list whose entries
   each name a shape (`<name>/v<N>`, Decision 7), its files by role in the
   same form, or for an image its reference by digest, and whatever else
   that shape's document defines. Version 1's schema checks the common
   members of a shape entry and nothing of a particular shape. A relay
   states its version here and nowhere this specification requires (b10).
   - *Rejected: a `SHA256SUMS` file alone.* It says nothing of platform,
     mode or shape.
   - *Rejected: a sidecar per file*, or an OCI artifact for every file: a
     launcher would fetch and trust N small files, or need a registry client
     to start a host binary.

4. **The manifest's SHA-256 is the release's published hash and the trust
   root; a self-built environment is first-class.** A release publishes the
   manifest's hash with it (`relay-release.json.sha256`, and its release
   notes). A launcher that adopts a release holds its version and that hash
   in its own release (M6's "adopt relay version X" sets both), checks the
   manifest it fetched against the pin before it reads it, checks each file
   it uses against the manifest before it runs anything, and never takes
   the hash to check from the place it fetched the manifest. A shape and a
   self-built environment rest on the same pin: a shape's files are
   checked against the manifest; a self-built environment is the pinned
   binary, checked the same way, plus the driver's recipe, which ships in
   the driver's own release and is trusted as the driver is. Neither is a
   lesser path, and reproducing a self-built environment bit for bit is not
   required, for validation or otherwise (Jeff's r1 notes (5) and (6)). A
   signature over the manifest is allowed and not required in version 1
   (b1).
   - *Rejected: per-file hashes pinned by the consumer.* One value per
     release is what an adopt item can review.
   - *Rejected: the hash fetched beside the manifest.* It detects a damaged
     download and not a replaced one.
   - *Rejected for version 1: a required signature.* A pinned hash already
     ties a driver's release to one relay release; a signature helps a
     launcher that pins nothing, which version 1 does not have. It can be
     added compatibly.
   - *Rejected: reproducible self-built environments*, with a digest of the
     built environment to compare. Review r1, note (5).

5. **The binary's launch interface is its arguments and one environment
   variable, the same on every platform and fixed by the specification.**
   `relay --mode=<host|container|vm> --claim-addr=<address>:<port>`, and
   per mode: `--fd=<n>` in host mode (the inherited frame socket of the
   build link), `--build-if=<name>` in vm mode (the build link's interface).
   The claim token is in `WARDEN_CLAIM_TOKEN`. Nothing else configures it:
   these are RELAY-08's pre-claim values and the wiring it calls "how the
   relay is started"; everything else is a claim field. Every argument the
   mode needs is required: there is no detected mode and no default
   descriptor, address or subnet. The relay writes its log to standard
   error; on a missing or malformed value it says which on standard error
   and exits non-zero before it serves anything. Its own address, prefix and
   gateway in vm mode are the environment's to configure before the binary
   starts (Decision 6); how they and the token reach that environment, a
   relay seed or a kernel command line, is the recipe's or the shape's,
   under RELAY-08 and BOOT-06 (b2).
   - *Rejected: the VM boot channel (`warden.*` words, the relay seed) in
     the core.* It is how one kind of environment feeds the binary, and the
     environment is the driver's (Decision 9) or a shape's (Decision 7).
   - *Rejected: argument templates in the manifest.* Every launcher would
     carry a template engine, and RELAY-08 could no longer be checked by
     reading one document.
   - *Rejected: the Go relay's detected mode and its defaults* (`--fd=3`,
     `--subnet`, a claim address on every interface). Each is a guess
     GLOBAL-11 rules out; they are the Go relay's divergences (Decision 12).

6. **What the relay requires of any environment it runs in.** Stated once,
   for every launcher, recipe and shape, and checked by none of the relay's
   own code beyond refusing to start (b3):
   1. *Entropy.* The environment's random source is seeded before the
      relay starts: on a VM, an entropy device, or the processor's or
      firmware's source where the hypervisor gives none (`RDSEED`, the UEFI
      RNG protocol). The relay blocks or refuses rather than make a key
      from an unseeded source (RELAY-03).
   2. *Time.* The clock is within 5 minutes of UTC when the relay starts,
      since it issues and checks certificates.
   3. *Network, by mode.* Host: the frame socket on `--fd`. Container: the
      build link's interface in the relay's network namespace and a route
      upstream. Vm: the interface `--build-if` names, configured with the
      relay's own pre-claim address, prefix and gateway, and an upstream
      interface with a route, before the binary starts. The claim address is
      reachable from the claimant (DRV-42). What else the build link must
      be is seam 7's.
   4. *Privileges.* In container and vm mode, binding ports 53, 80 and 443
      on the build-link address.
   5. *Storage.* No persistent writable storage: the relay keeps no build
      state on disk (RELAY-07), and an environment gives it none to keep;
      a memory-backed temporary directory is allowed.
   6. *Memory.* At least what the manifest states for the binary.
   7. *Output.* Its standard error kept by the launcher (a driver's
      `relay.log`).
   - *Rejected: devices per hypervisor in the core* (two NICs in order, a
     console, an optical drive). They are one recipe's or one shape's
     choices; the core states the need, not the device.
   - *Rejected: entropy optional where the kernel has other sources.* A
     relay that makes its CA and signing keys at start is exactly what
     RELAY-03 guards; one rule for every environment is simpler to check.

7. **Shapes are the wheels: each is its own versioned addition, specified
   once the relay's own release outputs it.** A *shape* is a prebuilt
   environment for the relay (an OCI image, a VM image for one hypervisor,
   a unikernel image), named `<name>/v<N>` as the ledger names environment
   types (`oci-image/v1`). Each is specified by its own document, in its own
   directory of the specs repository (`relay-shape-<name>/v<N>.md`), with
   its own tag, so that it is released when it is ready and changes without
   a release of the core. A shape's document says which roles its files
   have, how a launcher starts it (its boot channel, which carries only the
   pre-claim values), which `relay-claim` mode's requirements it meets, and
   how it meets each of Decision 6's requirements. A new major version of a
   shape is a new name. Version 1 of the core names none, and a launcher
   ignores a shape entry it does not run (b5).
   - *Rejected: a minor version of the core per shape.* Every shape's
     release would wait on, and be versioned with, the core's.
   - *Rejected: unversioned shape names.* A driver declares what it can
     run, and an incompatible change to a shape would make that declaration
     false without anything changing in the driver.

8. **A driver resolves its relay by a preference list of shapes, and falls
   back, by name, to building its own environment from the binary.** The
   driver contract (seam 9, `specs/driver-contract/v1.md`) gains:
   - `describe.capabilities.relay_shapes`: the shapes the driver can run,
     its *compatible shapes*, as `<name>/v<N>`; absent means none.
   - A common key, `relay_shapes` (an array of strings): the preference
     list, in order, under `[driver]` or `[driver.<name>]` with the
     precedence every common key has. Each entry is a compatible shape or
     the reserved name `self-built`. The driver refuses, with a `fixable`
     error that names the file, the key and its compatible shapes, an entry
     that is neither, an entry given twice, and an entry after
     `self-built`, which could never be reached.
   - Resolution: the first listed shape that the pinned release carries for
     the environment's platform. When none is, the driver builds its
     environment from the binary: the named fallback (GLOBAL-10,
     GLOBAL-11), which it says on standard error, naming the shapes it
     looked for and the release, and records (Decision 10). Listing
     `self-built` chooses it explicitly at its place in the list; the
     fallback is that same path, taken at the end of the list.
   - A listed shape that the release carries and that does not verify, or
     a recipe that fails, is an error that names it, never a reason to try
     the next entry.
   - Unset, the list is the driver's default, which it states in the key's
     `usage` as every common key's default is stated. At M6 each bundled
     driver's default is `[]`: it self-builds (Decision 11).
   A driver that runs a release of seam 8 honours the key; a driver that
   runs no relay of the project's (an integrator's, with its own) lists
   neither. New requirements follow `DRV-70` and `DRV-C12` (b6).
   - *Rejected: a key of each driver's own.* One name with one meaning for
     every driver is what a common key is, and Jeff's r1 note (8) puts the
     list in the contract.
   - *Rejected: `warden` resolves the relay.* The orchestrator reads none of
     a driver's configuration (Configuration), and only the driver knows
     what it can build.
   - *Rejected: no fallback when the user lists shapes and omits
     `self-built`.* GLOBAL-10 would allow it, but Jeff's r1 note (3) has the
     list "falling back to self-building"; a user who wants a shape or
     nothing has no way to say so in version 1, which b6 asks.
   - *Rejected: a silent fallback.* Review r1, note (3).

9. **Self-build recipes live with the drivers, or in the kit where they are
   shared; seam 8 does not specify them.** A *recipe* is the driver's code
   that builds a relay environment around the pinned binary and starts it:
   today's `tools/relay-vm` initramfs with the relay appended, Hyper-V's
   boot disk, the container driver's image built from the binary, vz's host
   process. A recipe meets Decision 6, starts the binary on Decision 5's
   interface, and gives it its pre-claim values under RELAY-08 and BOOT-06.
   It is identified by the driver's name and version and the recipe's name.
   A recipe two drivers would share (a Linux VM's initramfs, an image from
   the binary) belongs in the kit. No recipe is written or moved in this
   item; each driver's M6 item writes or adopts its own (Jeff's r1 note
   (4)).

10. **The ledger records which relay ran, as metadata the driver supplies.**
    The `environment` metadata schema (seam 4,
    `specs/metadata-environment/v1.md`) gains an optional member `relay`:
    `version` and `manifest_sha256` of the pinned release, `path` (the
    shape's name, or `self-built`), and for a self-built environment
    `recipe` (`<driver>/<recipe>@<driver version>`); for a relay a user's
    override named, `origin` (the key or variable) and no hash. The driver
    puts it in `claim_values.environment.metadata`, which the relay already
    writes into the environment record unchanged (CLAIM-28); no claim field
    and no relay code changes. It is unsigned and driver-attested, as the
    rest of that metadata is, and says so. The driver contract requires it
    of a driver that runs a release of seam 8. This revisits the first
    design's a6 (b7).
    - *Rejected: nothing recorded, left to M6.* With self-built
      environments first-class, the record is how a reader of a ledger
      tells a shape from a recipe, and the metadata already travels.
    - *Rejected: a claim field the relay records itself.* It changes seam 1
      and the relay for a value the relay cannot check either.
    - *Rejected: part of a signed environment payload.* The payload
      describes the build's environment, not the relay's, and a new payload
      type is a larger change to seam 4.

11. **Program order: M6 self-builds first, and shapes are adopted driver by
    driver later.** Each driver's M6 item pins the Rust relay's binary
    release and builds its environment from it with its own or the kit's
    recipe; its default preference list is `[]`. A shape becomes a choice
    once its document is released and the relay outputs it, and a driver
    adopts it in an item of its own, adding it to its compatible shapes and,
    if it is the better default, to its default list. The relay's M2 and M3
    still build and validate the appliance, the OCI image and the unikernel
    against the suites; they are shapes only once each is specified (Jeff's
    r1 note (7); **Amendments to the umbrella design**).

12. **The kit gets the shape vocabulary and the resolver; the manifest
    reader, fetching and recipes come with the first adoption.** In
    `driver/relaysource`, K1's four forms become the binary plus an
    extensible set of shapes: `RelayForm` becomes `Shape`, an open string
    `<name>/v<N>` with the reserved `SelfBuilt`; `Relay.Form` becomes
    `Relay.Shape`; the four roles keep only `binary` as the core's, the
    others becoming the provisional roles of the container driver's and the
    VM drivers' existing relays. A resolver takes the preference list, the
    compatible shapes and what a release carries for a platform (an
    interface), and returns the path and, for the fallback, its reason. In
    `driver/drivercmd`, `Capabilities` gains `RelayShapes`, and the common
    key `relay_shapes` is checked at the points DRV-C7 gives. The container
    driver's Go relay image is named by a provisional shape local to it,
    `go-relay-oci` (a known divergence, not a shape of seam 8). Reading and
    verifying a manifest, fetching a release and the shared recipes are the
    first M6 adoption's: until a release of seam 8 exists there is nothing
    to fetch (b9).

13. **The Go relay is a known divergence.** Its releases publish no
    manifest; its default image is `:latest`; its binary detects its mode
    and defaults `--fd`, `--subnet` and its claim address; its VM init
    takes `warden.net=dhcp` when the word is absent. The specification lists
    each under Known divergences, and nothing of the Go relay changes: M6
    retires it (b11).

14. **This repository adopts seam 8 later, by a reconciliation item.**
    Pinning it (`tools/conformance/relay-release-forms/go.mod`), adding it to
    GLOBAL-08's list of versioned seams and tier 1's `seams` (with
    `driver/relaysource/` as its code), its row in the book's seams page, and
    the manifest reader of Decision 12 are `adopt relay-release-forms at
    <version>` (LOOP-07), which FLOW-20 lets the loop file once a
    pre-release is tagged. The driver contract and the environment metadata,
    which live here, change in this item (b12).

15. **Seam 8's first pre-release waits for the specs repository's
    workflows to know its directory.** The `release` and `implementations`
    workflows name the six directories in their own text, on the specs
    repository's `main` as on its program trunk, and changing them is CI and
    release configuration, which is Jeff's. This item's pull request there
    does not touch `.github/`; it merges into the specs program trunk, and
    `relay-release-forms/v0.1.0-dev.1` is tagged once the workflows know the
    directory (b14).

## Questions

### Blocking

None. Every question that is component design or privileged work is
flagged below and answered in the design review this design requires
(FLOW-19, FLOW-20).

### Assumed

Each says what this design proceeds with, the options weighed and the
recommendation. The first design's a1 to a12 are withdrawn, as review r1
said.

- **b1. The trust root: the manifest's hash, pinned; self-built
  first-class.** Proceeding with Decision 4: one manifest per release, its
  SHA-256 the published hash; a launcher pins version and hash in its own
  release, checks the manifest and every file it uses before it runs
  anything, never takes the hash from where it fetched the manifest; a
  self-built environment is the pinned binary plus the driver's recipe,
  trusted as the driver is; no reproducibility; a signature allowed, not
  required. Options: (a) as stated; (b) as (a), with a required signature
  over the manifest (cosign keyless, as `warden`'s checksums are signed);
  (c) per-file pins; (d) the hash fetched beside the manifest. Recommended:
  (a): it is r1's notes (1), (5) and (6) made precise, one reviewable value
  per adopted release, and (b) can be added compatibly. **Flagged:**
  component design, the security posture of the component that signs the
  ledger, and a new contract.
- **b2. The binary's launch interface.** Proceeding with Decision 5:
  `--mode`, `--claim-addr`, `--fd` (host), `--build-if` (vm),
  `WARDEN_CLAIM_TOKEN`; every value required, no detected mode or default;
  the environment configures the vm-mode addresses; a boot channel is the
  recipe's or the shape's. Options: (a) as stated; (b) the Go relay's
  interface as it is, defaults and detection included; (c) a configuration
  file in place of arguments; (d) the VM boot channel (`warden.*` words, the
  relay seed) in the core as well. Recommended: (a): it is what the Go relay
  takes, less its guesses (GLOBAL-11), and (d) fixes one kind of
  environment in the core. **Flagged:** component design, the contract
  between every release and every launcher.
- **b3. What the relay requires of any environment.** Proceeding with
  Decision 6: a seeded random source (an entropy device, or the processor's
  or firmware's), a clock within 5 minutes of UTC, the network of its mode,
  the privilege to bind 53, 80 and 443, no persistent storage, the stated
  memory, and its standard error kept. Options: (a) as stated; (b) as (a),
  with a VM device list in the core (two NICs build link first, a console,
  an entropy device); (c) entropy required for VM environments only; (d) a
  different clock tolerance. Recommended: (a): r1's note (2) names entropy
  for RELAY-03 as the example, and devices are a recipe's or a shape's.
  **Flagged:** component design, the seam's contract and the relay's key
  posture (RELAY-03).
- **b4. The platforms and files of the binary.** Proceeding with Decision
  2: `linux/amd64` and `linux/arm64` required, others optional; each a plain
  file; Linux binaries statically linked; modes stated per binary; the
  roles open, `binary` the only one of the core. Options: (a) as stated;
  (b) no required platform; (c) every platform the relay supports required;
  (d) archives. Recommended: (a): every self-built environment runs a Linux
  binary, and an empty root file system is the simplest recipe.
  **Flagged:** component design, a requirement on every relay release.
- **b5. Shapes: names and where each is specified.** Proceeding with
  Decision 7: `<name>/v<N>`, each in its own directory
  (`relay-shape-<name>`) with its own tag, saying its roles, start, claim
  mode and how it meets Decision 6; none in version 1. Options: (a) as
  stated; (b) unversioned names; (c) each shape a section added to the core
  in a minor release. Recommended: (a): a shape is released when the relay
  can output it (r1 note (2)) and changes on its own. **Flagged:** component
  design, how the seam grows.
- **b6. The driver contract: compatible shapes and the preference list.**
  Proceeding with Decision 8: `capabilities.relay_shapes`, the common key
  `relay_shapes` with the reserved `self-built`, refused unless a subset of
  the compatible shapes with no repeat and nothing after `self-built`; the
  first listed shape the release carries, else the named, reported
  fallback; a shape that fails to verify is an error; the default each
  driver's, `[]` at M6. Options: (a) as stated; (b) as (a), with the
  fallback only when the key is unset or lists `self-built` (an explicit
  list without it fails when no listed shape is in the release); (c) a key
  of each driver's own; (d) `warden` resolves the relay. Recommended: (a):
  r1's notes (3) and (8); (b) is the stricter reading of GLOBAL-10 and is
  Jeff's to choose. It departs from K1's a3 ("K1 adds no relay key") on his
  newer direction. **Flagged:** component design, a change to the driver
  contract.
- **b7. The ledger records which relay ran.** Proceeding with Decision 10:
  an optional `relay` member of the `environment` metadata (version,
  manifest hash, path, recipe or origin), supplied by the driver in
  `claim_values.environment.metadata`, unsigned and driver-attested,
  required of a driver that runs a release of seam 8. Options: (a) as
  stated; (b) nothing recorded, left to M6 (the first design's a6); (c) a
  claim field the relay records; (d) a signed payload. Recommended: (a): r1
  asked to revisit, and it needs no relay or claim change. **Flagged:**
  component design, a change to seam 4's content.
- **b8. The umbrella's amendments.** Proceeding with the wording under
  **Amendments to the umbrella design**: seam 8's row, the Cross-driver
  column and its sentence, the M6 row, Defaults and overrides, the
  "Later" sentence and the RELAY-04 row. Options: (a) as proposed; (b) also
  amend Decision 11 to name the native binary as the source distribution
  and the unikernel as a shape; (c) amend nothing and let this design
  stand beside the umbrella. Recommended: (a): Decision 11 still holds read
  as shapes, and (c) leaves the umbrella stating forms r1 rejected.
  **Flagged:** component design, the program's design.
- **b9. The kit's scope in this item.** Proceeding with Decision 12: the
  shape vocabulary, the resolver over an interface and the key's check
  here; the manifest reader, fetching and recipes with the first adoption.
  Options: (a) as stated; (b) the manifest reader and verification here as
  well; (c) no kit change until M6. Recommended: (a): r1's note (8) names
  `driver/relaysource`, and (b) implements a seam this repository has not
  pinned (LOOP-07). Strategy: the order of work inside the program.
- **b10. Names and how a relay states its version.** Proceeding with:
  directory `relay-release-forms`, prefix `FORM`, manifest
  `relay-release.json`; the version in the manifest only. Options: (a) as
  stated; (b) rename the directory `relay-release`, now that "forms" are a
  binary and shapes; (c) each file also states its version
  (`prebuilt-artifacts` Decision 4). Recommended: (a): the seam's name in
  the umbrella and the queue, and a kernel or image cannot carry a version
  line. Strategy.
- **b11. The Go relay as a known divergence** (Decision 13). Options: (a) as
  stated; (b) its release publishes a manifest now (release configuration,
  Jeff's). Recommended: (a): M6 retires it. Strategy.
- **b12. Adoption of seam 8 here by a reconciliation item** (Decision 14),
  while the driver contract and the environment metadata change in this
  item. Options: (a) as stated; (b) this item also pins seam 8 and amends
  GLOBAL-08 and tier 1's `seams`. Recommended: (a): (b) pins a
  specification before any release of it, and adds a rule change. Strategy:
  the order LOOP-07 gives.
- **b13. What the suites check.** Proceeding with: seam 8's suite checks a
  release (the manifest against its schema and the published hash; each
  file's size and hash; the required platforms; and, for the binary built
  for the harness's host, a start in host mode on Decision 5's interface in
  the Linux sandbox with the `exec` launcher, serving its claim endpoint and
  refusing a request without its token, and refusing to start without a
  required argument); Decision 6 and every launcher requirement are listed
  as not machine-checked. The driver suite checks `relay_shapes` in
  `describe` and the key's refusals in a probe, against the fake; the
  resolution, the fallback's message and the metadata are not
  machine-checked, since the suite sees no relay. Options: (a) as stated;
  (b) static release checks only; (c) a recipe case. Recommended: (a).
  Strategy.
- **b14. The specs workflows' directory list** (Decision 15). Options: (a)
  Jeff changes the specs repository's `release` and `implementations`
  workflows to read the list from the `Makefile`'s `SPECS`, so that no later
  specification needs the change; (b) this item's specs pull request changes
  them and goes to the specs repository's `main` by his merge; (c) he adds the
  directory by hand. Recommended: (a). **Flagged:** privileged work (CI
  and release configuration), Jeff's.

### Settled, not asked

- Native binary as the source distribution, shapes as later versioned
  additions, the preference list with a named self-built fallback, recipes
  with the drivers or the kit, no reproducibility, the manifest as trust
  root, M6 self-building first, and the driver contract changing: settled
  by review r1's notes (1) to (8), not asked. b1 to b8 ask only what this
  design adds to them.
- Seam 8 belongs in the specs repository with a black-box suite: settled
  by umbrella Decision 3, not asked.

## Design

### What the item builds

**In `buildwarden/buildwarden-specs`**, on `feat/relay-release-forms-spec`
from the specs program trunk (`70457f1`), merged into it by the loop
(FLOW-20) before the home's pull request (`merge_first`). That repository has
not adopted the Loop Contract (no profile, LOOP-08), so the fast gate and the
reference-document checks do not run there (FLOW-03); its CI (`make check`)
does, and the conventions in its `README.md` are the invariants the change
keeps (LOOP-04, LOOP-08). It has no area map, so FLOW-05 treats it as one
exclusive area. `relay-conformance-timeout` also changes its harness, against
its `main`; whichever merges second takes the next harness version.

- `relay-release-forms/`: `v1.md`; `v1.manifest.schema.json` with an example
  manifest the document's examples are checked against;
  `not-machine-checked.txt`; `CHANGELOG.md` (`## v0.1.0`); the directory's
  package (`doc.go`, `spec.go`, `spec_test.go`, embedding the document as the
  other six do); `cases/`; `cmd/bw-conformance-relay-release-forms/`; `go.mod`.
- `conformance/`: the `release` role in the target format (a `[release]`
  table naming the manifest, a path or an `https` URL, and optionally the
  published hash), a release model, and in-process fixture releases, one
  that conforms and one per fault the cases look for; the harness's next
  minor version.
- `Makefile`'s `SPECS`, `go.work`'s `use` list, and `README.md`'s index row
  (seam 8, `relay-release-forms`, `FORM`, not yet released), the `release`
  role, and the Implementations table (none yet).

**In `buildwarden/buildwarden`**, on this branch, after the specs merge:

- `specs/driver-contract/v1.md` and `v1.schema.json` (Decision 8): the
  capability, the common key, the new requirements, the "What a driver
  needs besides this contract" row for seam 8 pointing at the new document,
  and the not-machine-checked rows; `cmd/warden/driver-contract-v1.schema.json`,
  the copy `cmd/warden` embeds and its test compares.
- `specs/metadata-environment/v1.md` and `v1.schema.json` (Decision 10):
  the optional `relay` member; tier 1's `schema-doc` check holds the pair
  together (RELAY-05). The `$id` does not change.
- `conformance/driver/`: the common key in its list, cases for the
  capability and the key's refusals, and the fake driver declaring two
  compatible shapes and reading the key.
- `driver/relaysource/` and `driver/drivercmd/` (Decision 12), and
  `driver/container/container.go` and `relay_source_test.go` for the
  renamed identifiers; no behaviour of the container driver changes.
- `docs/book/src/guide/configuration.md`: the common key in the `[driver]`
  example and the sentence that lists the common keys (UX-01). No driver
  reads it yet, which the page says.
- `docs/design/componentization.md`: the amendments below, and a dated
  entry in its **Amendments** section, once the review approves them.

No area is needed that the map lacks, and no link. `cli` is reached only by
the embedded schema copy.

### Seam 8's document, section by section

The sections are the specs repository's, in its order.

- **Scope and roles.** The release and the launcher. Outside it: what the
  relay does once claimed (`relay-claim`, `relay-guest-api`, the collector
  documents); each shape (its own document); recipes (the drivers'); the
  build link's isolation (seam 7); where a launcher keeps its pin; a claim
  transport other than TCP (S4).
- **Transport.** A release is a set of files at one base location with
  `relay-release.json` among them; names in the manifest are relative to
  it. Nothing binds a release to GitHub.
- **Messages and fields.** The manifest, member by member (Decision 3); the
  published hash (Decision 4); the binaries, platforms and roles (Decision
  2); the shape entry's common members and the naming of shapes (Decision
  7); the launch interface (Decision 5); the environment requirements
  (Decision 6).
- **Ordering.** A launcher checks the manifest against its pin, then each
  file it uses, then builds or starts the environment, then starts the
  binary with its values; the release accepts the claim (`relay-claim`).
- **Errors.** A launcher refuses a manifest whose hash is not the pin, an
  unknown `manifest_version`, a file whose size or hash differs, an image
  reference without a digest, each naming the file and where it came from.
  The binary refuses a missing or malformed argument or token, and an
  unseeded random source.
- **Limits.** The manifest's size, names' lengths, hashes as 64 lowercase
  hexadecimal digits.
- **Versioning.** `manifest_version` 1; while releases are `v0.x.y` the
  document may change incompatibly; a new member, role or platform is
  additive; a launcher ignores a member, role or shape it does not use.
- **Conformance.** Which requirements a release meets, which a launcher
  meets, and what a shape's document must state.
- **Known divergences.** The Go relay's (Decision 13).

### The driver contract's change

In `specs/driver-contract/v1.md`, numbered by Plan after `DRV-70` and
`DRV-C12`:

- In `describe`'s table: `capabilities.relay_shapes`, the compatible
  shapes. A requirement: each entry matches `<name>/v<N>` and none repeats,
  and `self-built` is not one.
- In **The common keys of version 1**: `relay_shapes`, an array of strings,
  the preference list (Decision 8). Adding a common key is a change the
  **Versions** section lets version 1 make while it is unstable.
- A configuration requirement: a driver that lists `driver.relay_shapes`
  refuses an entry that is neither a compatible shape nor `self-built`, a
  repeat, and an entry after `self-built`, with a `fixable` error naming the
  file, the key and its compatible shapes, at the points DRV-C7 gives.
- A build requirement: a driver that runs a release of seam 8 lists the key,
  resolves as Decision 8 says, reports the fallback on standard error before
  `relay-started`, and supplies the `relay` metadata of Decision 10.
- In "What a driver needs besides this contract": seam 8's row names
  `relay-release-forms` for the binary, the release and the environment,
  and the shapes' documents for each shape.

The schema gains the capability and nothing else: the key is a common key's
entry in `describe.keys`, which the schema already describes.

### The umbrella test, read by Design

Design judged the points before choosing the review (FLOW-20; the
independent session judges them again):

- U1 fails. Seam 8 is in the umbrella's seam table and this is the item Jeff
  filed for it (`driver-contract` a11), but the design moves building a
  relay's environment from the relay's releases (the umbrella's "appliance
  image", Track R's M2 and M3 deliverables as release forms) to the
  drivers' recipes, a component's ownership, and it changes seam 9 and seam
  4 besides, on Jeff's r1 direction.
- U2 fails: b1 to b8 are component design and b14 privileged work, flagged.
- U3 holds: `rule_changes` is empty, and the areas (`designs`, `specs`,
  `conformance`, `driver-core`, `driver/container`, `cli`, `book`) reach no
  rule or machinery path of the profile (`docs/development/`,
  `tools/tier1/`, `tools/workflow/`, `.claude/`).
- U4 holds: `security_boundary: none`; no code on a boundary changes
  behaviour (Security note).
- U5 holds: the breaks are the Go API of `driver/relaysource` (`RelayForm`,
  `Relay.Form`) and additions to the driver contract, both of which FLOW-20's
  Interfaces bullet allows; no CLI flag or config key is removed, and the
  ledger's wire format and every `$id` are unchanged.
- U6: `repos` match intake's; intake recorded no areas, so the session
  checks the seven above against the map.

So `design_review: required`.

## Amendments to the umbrella design

[`componentization`](componentization.md) is archived, and its decisions are
Jeff's. Review r1's direction changes these sentences; each is made in this
item's pull request once the review approves it (b8), with an entry in its
**Amendments** section in the form of `driver-contract`'s: "**<date>, by
`relay-release-forms-spec`**, approved in its design review. Seam 8's row,
the M6 row, the Cross-driver table's relay column, and the sentences that
follow from them were reworded. The wording each had is in
[`relay-release-forms-spec`](relay-release-forms-spec.md), under
**Amendments to the umbrella design**." Line numbers are the umbrella's on
the program trunk at `5b0a000`.

| Where | Current | Proposed |
|---|---|---|
| Seam 8 (291) | "Relay release forms \| Relay releases and drivers \| No \| New: each form (host-mode binary per platform, unikernel image, appliance image, OCI image) with a published hash, and the devices a relay VM needs. The list is open-ended." | "Relay release forms \| Relay releases and drivers \| No \| New: a release manifest and its published hash; the native binary per platform, always released, with its launch interface; and what the relay requires of any environment. Prebuilt shapes (an OCI image, VM images, a unikernel) are each a versioned addition once the relay's release outputs them; a driver that runs none of them builds the relay's environment from the binary. The list of shapes is open-ended." |
| M6 (335) | "Reconciliation: one "adopt relay version X" item per driver, each an item in the one queue that names that driver's repository. Then the Go relay is retired." | "Reconciliation: one "adopt relay version X" item per driver, each an item in the one queue that names that driver's repository, in which the driver pins the Rust relay's release and builds its relay's environment from the native binary. Then the Go relay is retired. Shapes are adopted driver by driver afterwards." |
| Later, not scheduled (363-364) | "more relay release forms such as cloud images (seam 8)" | "more relay shapes such as cloud images, and the drivers' adoption of each shape (seam 8)" |
| Rule table, RELAY-04 (418) | "Three-mode parity is restated for the Rust relay's release forms and its two network paths." | "Three-mode parity is restated for the Rust relay's native binary in each mode, its shapes, and its two network paths." |
| Defaults and overrides (453) | "which relay release form each driver uses and how it is overridden (K1, M6)," | "which relay shape each driver prefers and how the preference is set, falling back to an environment built from the binary (seam 8's item, M6)," |
| Cross-driver table heading (465) | "Relay form expected at M6" | "Relay shape a driver may adopt after M6" |
| Below the table (473-474) | "The relay form is each driver's own decision in its M6 item; the column is what the relay's milestones are built to allow." | "At M6 every driver builds its relay's environment from the native binary. Which shapes it adopts later, and in what order of preference, is each driver's own decision; the column is what the relay's milestones are built to allow." |

The Cross-driver table's cells are unchanged under the new heading. Decision
11 is not amended: its native binaries and unikernel images still hold, read
as the binary and a shape.

## Defaults and overrides

- **`relay_shapes` unset: the driver's default list**, stated in its key's
  `usage`. At M6 every bundled driver's is `[]`, so it self-builds: the
  binary is always released, so it is the one path every driver has
  (GLOBAL-09). Overridden by the key in `[driver.<name>]`, else `[driver]`,
  used exclusively (GLOBAL-10, DRV-C8).
- **The named fallback: self-built when no listed shape is in the pinned
  release for the environment's platform.** Triggered only by the release's
  manifest carrying no entry for any listed shape on that platform. The
  user sees one line on standard error before the build starts, naming the
  shapes looked for, the release and the path taken, and the ledger's
  environment metadata records `path: self-built` (Decision 10). Its
  reference documentation is the driver contract and the book's
  configuration page (UX-01). This item names it; no driver takes it until
  its M6 item.
- **Not fallbacks:** a listed shape that does not verify, a recipe that
  fails, a manifest that does not match the pin, a file that does not
  match the manifest: each is an error that names it and its fix (UX-04),
  and nothing else is tried.
- **`capabilities.relay_shapes` absent: no compatible shapes**, so only
  `self-built` is accepted in the key. The obvious reading of a driver that
  declares nothing.
- **The binary's arguments have no defaults** (Decision 5); the Go relay's
  are known divergences.
- **The pin is the launcher's**, set by its adopt item; there is no default
  location or hash, and no unpinned path. A user's override of the relay
  (the container driver's `relay_image`, a path a driver takes) stays that
  driver's own key, used as given, recorded with its `origin` and no hash.
- **Fixed values:** the clock tolerance (5 minutes) and the required
  platforms; nothing overrides them.
- **The signature** is optional, so its absence is not a fallback, and a
  present signature never stands in for the pin.

## Cross-driver impact

No driver's behaviour changes. The container driver's code changes only by
the kit's renamed identifiers (`RelayForm` to `Shape`, `RelayOCIImage` to
its local provisional shape); qemu, firecracker, vz and hyperv do not use
`driver/relaysource` yet (K2). From each driver's M6 item on, it declares
its compatible shapes (none at first), honours `relay_shapes` with the
default `[]`, builds its environment from the pinned binary with its recipe
(qemu and firecracker: the initramfs with the binary; hyperv: its boot disk
and seed; vz: a host process, or a VM; container: an image from the binary),
and supplies the `relay` metadata. Shapes follow driver by driver.

## Specification changes

- **`relay-release-forms` (seam 8): new.** `relay-release-forms/v1.md` in
  `buildwarden/buildwarden-specs`, requirements `FORM-1` onward, with the
  manifest schema and suite, made in that repository's pull request from
  `feat/relay-release-forms-spec` into `program/componentization`. Its first
  release is `relay-release-forms/v0.1.0-dev.1`, once the specs workflows
  accept the directory (Decision 15). This repository pins it later
  (Decision 14). Until it lands, seam 8 is defined by `driver/relaysource`
  (the names), the five drivers' relay start code and
  `tools/relay-vm/README.md`.
- **`driver-contract` (seam 9): changed.** `specs/driver-contract/v1.md`
  and `v1.schema.json` here: `capabilities.relay_shapes`, the common key
  `relay_shapes`, and new requirements after `DRV-70` and `DRV-C12`
  (above); the seam 8 row of "What a driver needs besides this contract";
  the not-machine-checked rows. Its suite, `conformance/driver/`, gains the
  cases. The contract version stays 1 (Versions: version 1 may change its
  common keys while unstable).
- **`ledger-format` (seam 4): changed additively.**
  `specs/metadata-environment/v1.md` and `v1.schema.json`: the optional
  `relay` member (Decision 10). The ledger's wire format, the environment
  record and every `$id` are unchanged.
- **`relay-claim` (seam 1): no change.** Seam 8 refers to CLAIM-2 (the claim
  endpoint), CLAIM-5 and CLAIM-10 (the token before the claim) and CLAIM-28
  (the environment's metadata written as given).
- **`bootstrap-model`: no change.** The boot channel stays BOOT-06's; a
  recipe or a shape is the launcher that reads it. No Bootstrap comparison
  is needed (BOOT-07).

## Rule changes

None. RELAY-04's restatement for the Rust relay stays M6's (the umbrella's
rule table, whose wording b8 amends), RELAY-08 already covers the binary's
flags and variable as "how the relay is started", and adding seam 8 to
GLOBAL-08's list of versioned seams is the adoption item's (Decision 14).

## Security note

`security_boundary: none`. No code on the host-environment,
environment-relay or relay boundary changes behaviour: the specification,
the contract's additions and the kit's resolver are used by no driver before
its M6 item, and the container driver's change is a rename. What the item
sets is the posture M6 will have: a launcher that checks a pinned manifest
and every file before it runs a relay (Decision 4), environments that seed
the relay's random source before it makes its keys (Decision 6), and a
fallback that can never be reached by a failed verification (Decision 8).
Each is flagged (b1, b3, b6). A self-built environment is trusted as the
driver that builds it is; the ledger says which path ran, as driver-attested
metadata (Decision 10). For TEST-08, at M6: a launcher refuses a manifest or
file that does not match its pin; a binary refuses to start without its
token or a seeded random source; a shape that fails verification fails the
build rather than falling back.

## Validation plan

- **In the specs repository:** `make check` (document checks over seven
  directories, `go vet`, `go test` with seam 8's suite against the
  in-process releases, coverage, module and tidy checks), and its CI on the
  pull request into the program trunk. The launch case runs in the Linux
  sandbox against a fixture binary built from the harness's own relay
  stand-in; no relay's release conforms yet (Decision 13).
- **In this repository:** `make test` and `make lint`: the driver suite
  against the fake (the capability and the key's refusals), the kit's tests
  for the resolver (every order, the fallback and its message, a failed
  verification that does not fall back) and for the key's check, the
  container driver's tests unchanged in behaviour, `cmd/warden`'s schema
  copy test, and tier 1's `schema-doc` check over the environment metadata.
- **Tier 2:** none native. Decision 6 and the launcher requirements are
  listed, with reasons, in seam 8's `not-machine-checked.txt`; they are
  checked when a driver adopts a release (M6).
- **Native validation:** none, blocking or tracked: no driver, host or
  platform behaviour changes.

## Implementation plan

Plan writes the tasks from the design as reviewed. In outline, specification
first (LOOP-02):

1. Specs repository: `relay-release-forms/v1.md` with every requirement
   numbered, the schema and its example, `not-machine-checked.txt`, the
   changelog and the package; `make check-docs` green.
2. Specs repository: the harness's `release` role, model and fixtures; the
   suite and its command; `Makefile`, `go.work`, `README.md`; `make check`
   green; the pull request into the specs program trunk.
3. Here: `specs/driver-contract/v1.md` and its schema, and the embedded copy
   in `cmd/warden`; `specs/metadata-environment/v1.md` and its schema.
4. Here: `conformance/driver/` cases and the fake.
5. Here: `driver/relaysource/` and `driver/drivercmd/`, and the container
   driver's renamed identifiers (rework of K1's code, FLOW-06).
6. Here: the book's configuration page (UX-01); the umbrella's amendments
   and its **Amendments** entry.
7. At Publish: the design archived with its summary. Once seam 8's first
   pre-release is tagged, the loop files `adopt relay-release-forms at
   v0.1.0-dev.1` for this repository (Decision 14).

## Design review

- r1, 2026-11-23: changes. Read at 40f6c52, in the attended session
  (workflow-human). Jeff sent the design back before a1 to a12 were asked:

  Start over from first principles (Jeff, 2026-11-23 review r1). Drivers
  dictate the relay environment they need to enforce the boundary on their
  build environment. (1) The native relay binary per OS/arch is the source
  distribution: always released, pinned by the release manifest's hash, the
  case of last resort. (2) Prebuilt shapes (OCI image, VM-specific images,
  unikernel) are the binary wheels: each is specified as its own versioned
  addition only once it is straightforward to output as part of the relay's
  own releases; version 1 defines the core (manifest and hash, the native
  binaries and roles, the binary's launch interface, what the relay requires
  of any environment, e.g. entropy for RELAY-03) and no shapes up front. (3)
  A driver resolves its relay by a configurable preference list of shapes,
  which configuration requires to be a subset of the shapes known compatible
  with that driver, falling back to self-building its environment from the
  native binary; self-built is also selectable explicitly; the fallback is a
  named default, never silent (GLOBAL-09 to GLOBAL-11). (4) Self-build
  recipes live with the drivers, or in the drivers' shared utilities library
  (the kit) where shareable. (5) Reproducibility of self-built environments
  is not required for validation. (6) Trust: the manifest of release
  artifacts and hashes stands as the root; a self-built environment is
  first-class (the pinned binary's hash plus the driver's recipe). (7)
  Program order: M6 first means every driver self-builds from the Rust
  relay's binary release, shapes adopted driver by driver later. (8) The
  driver contract changes by design to carry the compatible-shapes
  declaration and the preference list: specs/driver-contract/v1.md,
  conformance/driver/ and the kit's driver/relaysource (K1's four form names
  become the binary plus an extensible set of shapes). Also revisit a6
  (whether the ledger records which relay path ran) and the umbrella's seam
  8 wording in componentization.md. Effectively starting over: the questions
  a1 to a12 are replaced; budget raised on Jeff's word.

  What changed in the redo (f1-design-r2, from first principles): every
  Decision and Question is new (b1 to b14 replace a1 to a12). Version 1 is
  the core only, with no shape (Decisions 1 to 7); the driver contract gains
  the compatible shapes and the preference key with the named self-built
  fallback (Decision 8); recipes are the drivers' or the kit's (Decision 9);
  the ledger records the relay's path as environment metadata (Decision 10,
  revisiting a6); the umbrella's seam 8, M6 and Cross-driver wording is
  amended (b8); `repos` unchanged, `areas` widened from `designs` to seven,
  `seams` from one to three.

## Consolidation

## Rollback notes

## Archive summary

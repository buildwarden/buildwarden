---
feature: relay-release-forms-spec
status: draft
drivers: []              # no driver tree changes. Every driver that starts a relay will be a
                         # launcher of this specification, from its M6 "adopt relay version X"
                         # item on; none adopts it here (Cross-driver impact)
seams: [relay-release-forms]
                         # seam 8 of the umbrella, written for the first time: no requirement
                         # of another seam changes (Specification changes)
surfaces: []
repos: [buildwarden/buildwarden, buildwarden/buildwarden-specs]
                         # home first (this design); the specification and its suite live in
                         # the second, which has no area map and so is one exclusive area (FLOW-05)
merge_first: [buildwarden/buildwarden-specs]
                         # the home's PR carries only this design, archived; it names the
                         # specification's merge on the specs program trunk
areas: [designs]         # in buildwarden/buildwarden this item edits its own design only;
                         # the adoption here is a later reconciliation item (Decision 11)
security_boundary: none  # no code on a boundary changes and nothing adopts the specification
                         # here; the integrity requirements it states are flagged (a1) and
                         # change a posture only when a driver adopts them (Security note)
alters_relationships: true   # a new versioned seam between relay releases and every launcher
                         # of a relay; the kit's relay source takes its names from it once adopted
design_review: required  # the umbrella test's U2 fails: a1 to a3 are component design (a new
                         # contract's trust root, launch interface and device set), flagged
native_validation: []    # nothing native: the suite runs in the specs repository's Linux CI
tier2_gaps: []           # no implementation conforms yet; what no case checks is listed in the
                         # specification's not-machine-checked.txt (Validation plan)
rules: [GLOBAL-06, GLOBAL-07, GLOBAL-08, GLOBAL-09, GLOBAL-10, GLOBAL-11, RELAY-02, RELAY-03,
        RELAY-04, RELAY-08, BOOT-01, BOOT-06, BOOT-07, TEST-02, TEST-03, TEST-05, UX-01,
        FLOW-01, FLOW-02, FLOW-03, FLOW-05, FLOW-07, FLOW-08, FLOW-09, FLOW-12, FLOW-14,
        FLOW-19, FLOW-20, LOOP-02, LOOP-04, LOOP-07, LOOP-08]
rule_changes: []
concepts: [relay-release-forms, release-manifest, published-hash, relay-source, boot-channel,
           relay-seed, relay-vm-devices, host-mode, unikernel, appliance, oci-image, seams,
           specs-repo, conformance, program]
related: [componentization.md, driver-contract.md, seam-specs.md, program-north-star.md,
          hyperv-relay-per-build-channel.md, firecracker-driver.md, unified-bootstrap.md]
---

# Seam 8: the relay's release forms

## Problem

The componentization umbrella lists seam 8, the relay's release forms, among
the seams that get a specification and a conformance suite in
`buildwarden/buildwarden-specs`: "each form (host-mode binary per platform,
unikernel image, appliance image, OCI image) with a published hash, and the
devices a relay VM needs. The list is open-ended." No phase row writes it.
`driver-contract` (K1) raised the gap as its a11, and Jeff answered (a) on
2026-11-14: K1 defines the four form names and four file roles as
provisional Go constants of the kit (`driver/relaysource`), writes no seam 8
text, and seam 8 is filed as its own S-track item, "to exist before M6
adopts a release". This is that item (filed 2026-11-20, task 38 of
`driver-contract`; a program item since `program-north-star`'s state step).

What is missing, concretely:

- **No written vocabulary.** `driver/relaysource` names the forms
  (`host-binary`, `oci-image`, `appliance`, `unikernel`) and the roles
  (`binary`, `kernel`, `initramfs`, `disk`), and its package comment says
  they "may change with" seam 8.
- **No way to name a release and check it.** K1's `RelayFile.SHA256` and
  `Relay.Verify` exist, but nothing publishes the hash they check. Today no
  driver checks a hash, and the container driver's default is the moving
  tag `ghcr.io/buildwarden/relay:latest`. K1 left a *pinned source*
  (version, location and published hash, downloaded and verified) to seam
  8: "its manifest and hash format are seam 8's".
- **No written launch interface.** How a launcher gives each form its
  pre-claim values and its links is code in five drivers and the relay
  VM's init (`tools/relay-vm/README.md` is the only document, for the
  appliance). `relay-claim` puts the boot channel outside its scope
  ("Who starts a relay, and how its claim token reaches it before the
  claim"), and `seam-specs` a6 says the VM's init and boot channel "belong
  to seam 8".
- **No list of what a relay VM needs.** qemu, firecracker and Hyper-V each
  give their relay VM two NICs, build link first, and a console; qemu and
  firecracker add a virtio entropy device; and the M0 VZ spike found that
  Hermit fails closed without an entropy source. The
  Rust relay's unikernel (M3) and appliance (M2) need a stated target, and
  each M6 item needs one to adopt against.

M6 is wave 5, but M2 and M3 build release forms now, and the relay
repository's profile is to declare "the relay's release forms (the
umbrella's seam 8)" as its consumables (`loop-contract`, LOOP-08 step 5).
They need the specification before their first release, not after.

## Prior art

- [`componentization`](componentization.md): seam 8's row; Decisions 3
  (specifications with black-box suites in the specs repository), 11 (host
  mode on every supported OS and architecture, plus unikernel images; more
  forms may follow), 12 (the appliance and the OCI image on the kernel
  network path; the unikernel and host mode on the relay's own stack over
  raw frames), 15 (Hermit forks); a3 (host mode Linux first, then macOS, not
  Windows); the Cross-driver table (the form each driver is expected to run
  at M6, each driver's own decision). Reused as the scope of this item.
- [`driver-contract`](driver-contract.md) (K1): Decision 11 and a3 (the
  relay source is a kit library, not part of the command contract), a11
  (this item), and the relay source's types and per-driver table. The form
  and role names are taken from it unchanged (Decision 2); its pinned
  source is what Decision 4's published hash is for. It left "whether a
  build's record should name the relay's form and version" to "seam 8 and
  M6" (a6 below).
- [`seam-specs`](seam-specs.md) (S1): the specs repository's conventions
  (document sections, numbered requirements, `not-machine-checked.txt`,
  `cases/`, `bw-conformance-<directory>`, target files, the Linux sandbox,
  the in-process attachment, per-directory tags), reused as they are
  (Decision 1). Its a6: the VM-image launcher is built by the relay's M2 or
  M3 item, "or by seam 8's item, whichever first has an image and a CI that
  boots it" (a9 below). Its Decision 6 rejected harness flags per form
  because "each relay form needs its own launch wiring", which is the
  interface this item writes down.
- [`hyperv-relay-per-build-channel`](hyperv-relay-per-build-channel.md):
  the relay seed (an ISO 9660 image `WARDENRELAY` holding
  `warden-relay.conf`), RELAY-08's pre-claim values and BOOT-06's second
  paragraph. The seed and the `warden.<key>=<value>` vocabulary are taken as
  the VM forms' boot channel (Decision 5).
- [`firecracker-driver`](firecracker-driver.md): the relay VM with a static
  upstream on its slot and an entropy device "so the CRNG seeds before the
  relay makes its keys" (RELAY-03), reused in Decision 6.
- [`unified-bootstrap`](unified-bootstrap.md): BOOT-06 as it stands. This
  item changes no BOOT rule.
- [`program-north-star`](program-north-star.md): the umbrella test, the
  question kinds on a program item (FLOW-07, FLOW-19, FLOW-20), the
  pre-release tags `<directory>/vX.Y.Z-dev.N`, and `specs-prerelease-tags`.
- `prebuilt-artifacts` (approved at `7a85a33`, held; on
  `feat/prebuilt-artifacts`): it gives executables a version line, rejects
  sidecars and per-directory manifests for files found on `PATH`, and says
  "how a relay states its version becomes seam 8's to say". Its rejection is
  about files a user installs on `PATH`; a release manifest is published
  with the release and pinned by hash, so the reason does not carry over
  (a11).
- The VZ spike report, `spike/REPORT.md` on `buildwarden-relay` `main`:
  the unikernel boots as Hermit's loader with the relay as its initrd; a
  virtio entropy device on every relay VM, the kernel refusing to produce
  random data without one; a virtio-net upstream; the build link as a
  second NIC fed by a datagram socket pair; vsock for the claim recommended.
- `tools/relay-vm/README.md` and `init` (program trunk `1e6a32d`): the
  appliance's boot channel as the Go relay VM has it, which Decision 5
  writes down.
- Standards: GLOBAL-08 (versioned seams), RELAY-04 (three modes), RELAY-08
  and BOOT-06 (pre-claim values only on the boot channel), RELAY-03 (keys
  made in memory at start, which needs entropy), TEST-05 (seams tested by
  the specs repository's suites), LOOP-02 (spec-first), LOOP-07 (artifact
  forms are consumables; adoption through a reconciliation item).

## Decisions

1. **One specification, `relay-release-forms`, in the specs repository,
   under its conventions.** Requirement prefix `FORM`. Two roles: the
   *release*, which publishes forms, and the *launcher*, which starts one
   (a driver, the harness, or an integrator's orchestrator). Its directory
   has the document `v1.md`, the manifest's schema
   `v1.manifest.schema.json`, `not-machine-checked.txt`, a suite in
   `cases/`, the command `bw-conformance-relay-release-forms`, and a
   `CHANGELOG.md` starting at `v0.1.0`.
   - *Rejected: one specification per form.* The manifest, the published
     hash and the device vocabulary are shared, and four documents would
     restate them.
   - *Rejected: a section of `relay-claim`.* That document puts the boot
     channel outside its scope on purpose, and a claimant is not a
     launcher: `cmd/collector` claims a relay it did not start.
   - *Rejected: `specs/` in this repository.* Umbrella Decision 3 puts every
     seam's specification but the ledger's and, until K2, the driver
     contract's in the specs repository.

2. **The forms and roles are K1's names, unchanged, and the list is
   open.** Forms: `host-binary` (host mode, one per OS and architecture),
   `oci-image` (container mode), `appliance` (a Linux relay VM on the
   kernel network path: a kernel and an initramfs, or one boot disk),
   `unikernel` (a relay VM on the relay's own stack over raw frames).
   Roles: `binary`, `kernel`, `initramfs`, `disk`. A release names, per
   form, the roles it carries: `host-binary` `binary`; `appliance` `kernel`
   and `initramfs`, or `disk`; `unikernel` `kernel`, and `initramfs` when
   its kernel loads the relay as an initial image (Hermit's loader with the
   relay as its initrd, as on VZ in M0); `oci-image` none, it names an
   image. A released appliance is complete: the `binary` role in an
   appliance, K1's relay appended at `/sbin/relay`, is how a launcher
   assembles one from a source tree and is no part of a release. A new
   form name is an additive change; a launcher passes over a form it does
   not run (a4).
   - *Rejected: new names.* K1's are in use in the kit and match the
     umbrella's words; renaming buys nothing.
   - *Rejected: a role per form* (`unikernel-image`, `appliance-kernel`).
     The four roles already say what a file is; the form says what it is
     for.

3. **A release is described by one manifest.** `relay-release.json`, JSON
   valid against `v1.manifest.schema.json`, is published beside the
   release's files. It names the relay and its version, and lists each form
   of the release: the form, the OS and architecture (`linux`, `darwin`,
   `windows`; `amd64`, `arm64`, as Go and the specs repository's binaries
   name them), the `relay-claim` mode whose requirements it meets
   (Decision 7), the boot channels it reads (Decision 5), for a VM form the
   devices it needs (Decision 6), and either its files by role (a name
   relative to the manifest, the size and the SHA-256) or, for `oci-image`,
   an image reference that carries its digest (`name@sha256:<hex>`). It
   also lists the specification releases the relay implements
   (`relay-claim/v0.1.0`, ...). A relay states its version there and
   nowhere this specification requires (a11).
   - *Rejected: a `SHA256SUMS` file alone.* It says nothing of which file
     is which form and role, which platform it is for, or what a VM needs.
   - *Rejected: a sidecar per file.* A launcher would fetch and trust N
     small files, and a file copied without its sidecar loses its identity.
   - *Rejected: an OCI artifact for every form.* Every launcher of a host
     binary would need a registry client, and the specs repository's own
     binaries are released as plain files.

4. **The published hash is the SHA-256 of the manifest, and it is what a
   consumer pins.** A release publishes the manifest's hash with the
   release (`relay-release.json.sha256`, and its release notes). A launcher
   that adopts a release holds the version and that hash in its own
   configuration or code (M6's "adopt relay version X" sets both), checks
   the manifest it fetched against it before reading it, then checks each
   file against the manifest before it starts anything, and runs an image
   only by the digest the manifest names. A launcher never takes the hash
   to check from the place it fetched the manifest. A signature over the
   manifest is allowed and not required in version 1 (a1, flagged).
   - *Rejected: per-file hashes pinned by the consumer.* One value per
     release is what an adopt item can review; per-file pins multiply with
     every form and platform.
   - *Rejected: the hash fetched beside the manifest.* It detects a
     damaged download and not a replaced one.
   - *Rejected for v1: a required signature (cosign or similar).* A pinned
     hash already ties a driver's release to one relay release; a signature
     adds a key or identity to manage and helps a launcher that pins
     nothing, which version 1 does not have. It can be added compatibly.

5. **The launch interface is fixed by the specification per form, not
   described by the manifest.** What the launcher gives, and on which
   channel, is only RELAY-08's pre-claim values (the claim token, and the
   relay's own address, prefix and gateway where the form needs them) and
   the launch wiring RELAY-08 calls "how the relay is started":
   - `host-binary`: the arguments `--mode=host`, `--fd=<n>` (the inherited
     frame socket of the build link) and `--claim-addr=<host:port>`, and
     the claim token in the environment variable `WARDEN_CLAIM_TOKEN`;
     nothing else configures it.
   - `oci-image`: run with the image's own entrypoint and no arguments, the
     claim token in `WARDEN_CLAIM_TOKEN`, the claim endpoint on TCP port
     8301 of the container.
   - `appliance` and `unikernel`: `warden.<key>=<value>` words, the keys
     `warden.net` (`dhcp` or `static`), `warden.nat_ip`,
     `warden.nat_prefix`, `warden.nat_gw` and `warden.claim_token`, on the
     kernel command line, or in the relay seed (an ISO 9660 image with
     volume label `WARDENRELAY` holding `warden-relay.conf`, one word to a
     line, the first `warden.seed=1`) attached as a read-only optical
     drive, for a hypervisor that gives no per-boot command line. The
     manifest says which of the two channels a VM form reads; a form that
     reads the seed refuses a `warden.*` word on its command line, so the
     two never both supply a value. The claim endpoint is TCP port 8301 on
     the upstream address.
   A release refuses to start on a malformed or missing value and says
   which (a2, flagged; a12).
   - *Rejected: argument and command-line templates in the manifest*, as
     the harness's target files have. Every launcher would carry a
     template engine, a release could ask for any value on any channel,
     and RELAY-08's "only the pre-claim values" could no longer be checked
     by reading one document.
   - *Rejected: a new vocabulary for the Rust relay.* The Go relay, its
     init and five drivers use these names, and the Rust relay implements
     the same seams.

6. **A VM form declares the devices it needs, from a vocabulary this
   specification defines, and the launcher provides exactly those.** For
   `appliance` and `unikernel`: how it boots (a kernel, with the
   `initramfs` when the form has one; or the `disk` under UEFI); its NICs
   in order, the build link first and the upstream second, each a
   virtio-net device (or the hypervisor's synthetic adapter where it has
   no virtio, Hyper-V); an entropy source, required for every VM form in
   version 1 and named by the form: a virtio entropy device, or the
   processor's or firmware's source where the hypervisor offers no device
   (`RDSEED` on x86-64, the UEFI RNG protocol of a Hyper-V Gen2 VM), because
   Hermit's kernel fails closed without one (M0) and the Linux appliance
   must seed its CRNG before the relay makes its keys (RELAY-03); a console (serial or virtio console), which carries the
   relay's log and its refusals; the optical drive for the relay seed when
   the form reads the seed; and the least memory and vCPUs it runs with.
   What may be attached to the build-link NIC is the build-link contract's
   (seam 7, phase S3) and is not restated here. A claim over vsock or any
   transport but TCP is `relay-claim`'s transport section and phase S4's;
   version 1 declares no vsock device (a3, flagged).
   - *Rejected: devices left to each driver.* The umbrella names "the
     devices a relay VM needs" as part of the seam, and an M6 item has
     nothing to adopt against without them.
   - *Rejected: a virtio entropy device in every case.* Hyper-V has none,
     and its appliance seeds from the firmware and the processor today.
   - *Rejected: entropy optional for the appliance.* The Linux kernel has
     other sources, but a relay that makes its CA and signing keys at start
     on a VM with none is exactly what RELAY-03 guards against; one rule for
     every VM form is simpler to check.

7. **Each form states which `relay-claim` mode's requirements it meets.**
   `host-binary` meets host mode's, `oci-image` container mode's,
   `appliance` vm mode's, and `unikernel` host mode's build-link
   requirements (CLAIM-39), because it serves the build on a frame link
   with its own stack (umbrella Decision 12). `relay-claim`'s Conformance
   section already asks a relay that is none of its three modes to state
   which mode's requirements it meets; the manifest's `claim_mode` is where
   a release states it (a5).

8. **Nothing is recorded in the ledger.** Whether a build's record names
   the relay's form, version or published hash is a change to seam 4 (the
   environment record), which the umbrella names for no phase before M6.
   This specification makes the question answerable, since one hash names
   one release, and leaves it to M6 or an item of its own (a6).

9. **The suite checks a release; a launcher's requirements are stated and
   not machine-checked in version 1.** The suite's role is `release`. A
   target file gains `role = "release"` and a `[release]` table naming the
   manifest (a path, or an `https` URL) and, optionally, the published hash
   to check it against. Its cases check: the manifest against the schema;
   the manifest against the published hash; each file's presence, size and
   SHA-256; that each form carries exactly its roles; that an image
   reference carries a digest; the platform names; and, for a
   `host-binary` built for the harness's host and for an `oci-image`, that
   the form started on Decision 5's interface in the harness's Linux
   sandbox, with the `exec` and `oci` launchers as they are, serves its
   claim endpoint and refuses a request without its token. The in-process
   attachment builds releases in memory, one that conforms and one with
   each fault the cases look for, so `go test` checks the suite with no
   network. The VM forms' launch and devices, and every launcher
   requirement (check before start, refuse a mismatch, provide the declared
   devices, give only the pre-claim values), are listed in
   `not-machine-checked.txt` with their reasons: a VM launcher is the
   relay's M2 or M3 item's (`seam-specs` a6), and a launcher is checked by
   the driver suite once a driver adopts this seam (a9).

10. **The Go relay is a known divergence, not an implementation.** Its
    releases publish no manifest; its default image is the moving tag
    `:latest`; its appliance is not released (qemu and firecracker build
    it from `tools/relay-vm`, Hyper-V's boot disk is built on the host by
    `refresh-cache.sh`) and appends the relay per build; its init takes
    `warden.net=dhcp` when the word is absent. The specification lists each
    under Known divergences. Making the Go relay's release publish a
    manifest is a change to this repository's release configuration
    (privileged work) for a relay M6 retires (a7).

11. **This repository adopts the specification later, through a
    reconciliation item.** Pinning it (`tools/conformance/relay-release-forms/go.mod`),
    adding it to GLOBAL-08's list of versioned seams and to tier 1's
    `seams` (with `driver/relaysource/` as its code), adding its row to the
    book's seams page and dropping "provisional" from `driver/relaysource`
    is the item `adopt relay-release-forms at <version>` (LOOP-07), which
    FLOW-20 lets the loop file once a pre-release of it is tagged. That
    item carries the GLOBAL-08 rule change and its own review. Here the
    home changes nothing but this design (a8).

12. **The specification is released once the specs repository's release
    workflow knows its directory.** The `release` and `implementations`
    workflows name the six directories in their own text, and a change to
    either is CI and release configuration, which is Jeff's (FLOW-20,
    privileged work). This item's pull request in the specs repository does
    not touch `.github/`. It merges into the specs program trunk untagged
    if the workflows do not know the directory yet, and its first
    pre-release, `relay-release-forms/v0.1.0-dev.1`, waits for them (a10).

## Questions

### Blocking

None. The questions below that need Jeff are flagged and answered in his
design review, which this design requires.

### Assumed

Each says what this design proceeds with, the options weighed, and the
recommendation. On a program item only component design is flagged
(FLOW-19, FLOW-20).

- **a1. The release's manifest and its published hash, as the trust root
  for a relay release.** Proceeding with Decisions 3 and 4: one manifest
  per release, its SHA-256 the published hash; a launcher pins version and
  hash, checks the manifest against the pin and every file or image against
  the manifest before starting anything, and never takes the hash from
  where it fetched the manifest; a signature allowed, not required.
  Options: (a) as stated; (b) as (a), with a required signature over the
  manifest that launchers verify (cosign keyless, as `warden`'s checksums
  are signed today); (c) per-file hashes pinned by the consumer, no
  manifest hash; (d) hashes published beside the files and fetched with
  them. Recommended: (a): one reviewable value per adopted release, and it
  binds every file; (b) can be added compatibly when a launcher that pins
  nothing exists. **Flagged:** it sets what a launcher must check before it
  runs a relay, the supply path of the component that signs the ledger: a
  security posture and a new contract the umbrella names ("with a
  published hash") without designing.
- **a2. The launch interface: fixed per form by the specification.**
  Proceeding with Decision 5: the Go relay's flags and variable for
  `host-binary` and `oci-image`, and the relay VM's `warden.<key>` words on
  the kernel command line or in the relay seed for the VM forms, the
  manifest naming which VM channel a form reads. Options: (a) as stated;
  (b) templates in the manifest with placeholders for the pre-claim
  values, as target files have; (c) a new, form-neutral vocabulary for the
  Rust relay. Recommended: (a): drop-in releases, no template engine in
  launchers, and RELAY-08 checkable in one place. **Flagged:** it is the
  contract between every relay release and every driver.
- **a3. The devices a relay VM needs.** Proceeding with Decision 6: boot
  by kernel (and initramfs) or UEFI disk; two NICs, build link first; an
  entropy source required for every VM form, named by the form (a virtio
  device, or the processor's or firmware's where the hypervisor has none);
  a console; the seed's optical drive when the seed is read; least memory
  and vCPUs declared; no vsock in version 1. Options: (a) as stated; (b)
  entropy required for the unikernel only; (c) a virtio entropy device
  required in every case; (d) declare vsock now as an optional device for
  the claim, ahead of S4; (e) NIC order upstream first. Recommended: (a):
  it is what qemu, firecracker and Hyper-V give the Go relay VM today and
  what M0 found Hermit needs; (c) rules out Hyper-V; vsock belongs with the
  claim's transport (S4); (e) would reorder every driver for nothing.
  **Flagged:** it is part of the seam's contract, and the entropy
  requirement is part of the relay's key posture (RELAY-03).
- **a4. Form and role names, and the roles each form carries.** Settled
  by `driver-contract` Decision 11 and a11 for the names, not asked.
  Proceeding with Decision 2 for the roles: a unikernel is `kernel` with an
  optional `initramfs` (M0's loader and relay), and a released appliance
  carries no `binary`. Options: (a) as stated; (b) a unikernel as one
  `kernel` file only, as K1's comment reads; (c) a released appliance may
  carry `binary` for the launcher to append. Recommended: (a): Hermit on VZ
  boots as loader plus initrd; an appliance that needs assembling is not a
  release whose hash means anything. A question of strategy: it implements
  the forms the umbrella names.
- **a5. The `relay-claim` mode each form meets** (Decision 7), unikernel
  as host mode's build link. Settled by `relay-claim`'s Conformance
  section ("states which mode's requirements it meets") and umbrella
  Decision 12, not asked beyond confirming where it is stated. Options:
  (a) in the manifest, per form; (b) in this document only, per form name.
  Recommended: (a): a form the list gains later states its own. Strategy.
- **a6. No ledger record of the form, version or hash** (Decision 8).
  Options: (a) none here, left to M6 or its own item; (b) add them to the
  environment record now. Recommended: (a): (b) changes seam 4, which the
  umbrella names for no phase before M6 and whose change would be
  component design of its own. Strategy: declining a seam change.
- **a7. The Go relay as a known divergence** (Decision 10). Options:
  (a) as stated; (b) the Go relay's release publishes a manifest now (a
  change to this repository's `.goreleaser.yaml` and `relay-image.yml`,
  Jeff's). Recommended: (a): it is retired at M6, and the Rust relay's M2
  and M3 releases are the first producers. Strategy.
- **a8. Adoption here later, by a reconciliation item** (Decision 11).
  Options: (a) as stated; (b) this item also pins the specification,
  amends GLOBAL-08 and tier 1's `seams`, and adds the book row.
  Recommended: (a): (b) pins a specification before any release of it,
  and adds a rule change to an item that otherwise needs none. Strategy:
  the order LOOP-07 already gives.
- **a9. What the suite checks** (Decision 9). The VM launcher is settled
  by `seam-specs` a6 (Jeff, 2026-11-15: built by the relay's M2 or M3
  item, or by this item if it first has an image and a CI that boots it;
  it has neither), not asked. Options for the rest: (a) as stated; (b)
  release-side static checks only, no launch case; (c) also a launcher
  case in the driver suite now. Recommended: (a): the launch case is what
  makes Decision 5 more than prose for the two forms the sandbox can run,
  and (c) belongs with the first driver that adopts the seam. Strategy.
- **a10. The specs workflows' directory list** (Decision 12). Options:
  (a) Jeff adds the directory to the specs repository's `release` and
  `implementations` workflows, best inside `specs-prerelease-tags` (his
  setup item that already changes `release.yml`), reading the list from
  the `Makefile`'s `SPECS` so that no later specification needs the same
  change; (b) this item's specs pull request changes the workflows and
  goes to the specs repository's `main` by his merge, not to the program
  trunk; (c) the directory list is added by hand when he next touches the
  workflows. Recommended: (a). Not component design; it needs privileged
  work, which is Jeff's, and is asked in the review this design needs
  anyway.
- **a11. How a relay states its version, and the names.** Proceeding
  with: the manifest's `version` is the release's, and this specification
  asks a file for no version of its own (a host binary may carry
  `prebuilt-artifacts`' version line); the directory `relay-release-forms`,
  the prefix `FORM`, the manifest file `relay-release.json`. Options: (a)
  as stated; (b) each file states the version in itself as well, as
  `prebuilt-artifacts` Decision 4 does for executables. Recommended: (a):
  a kernel, a disk image or an OCI image cannot carry that line, and the
  hash already binds the file to the manifest. `prebuilt-artifacts` leaves
  this to seam 8. Strategy.
- **a12. No default for a missing boot-channel value.** Proceeding with:
  a launcher gives `warden.net` and every key its form needs, and a release
  refuses to start without one; the Go init's `dhcp` default is a known
  divergence (Decision 10). Options: (a) as stated; (b) the specification
  keeps `dhcp` as the default. Recommended: (a): every driver already
  passes the word, and a boot value that defaults is the kind of guess
  GLOBAL-11 rules out. Strategy (Defaults and overrides).

## Design

### What the item builds

All of the item's work but this design is in `buildwarden/buildwarden-specs`,
on its branch `feat/relay-release-forms-spec` from the specs program trunk
(`70457f1`), merged into that trunk by the loop (FLOW-20). That repository
has not adopted the Loop Contract (no profile, LOOP-08), so the fast gate
and the reference-document checks do not run there (FLOW-03); its CI
(`make check`) does, and its conventions in `README.md` are the invariants
the change keeps (LOOP-04, LOOP-08). It has no area map, so FLOW-05 treats
it as one exclusive area: this item collides there with
`relay-conformance-timeout` and `relay-collector-failure-fails-open`, both
in progress with that repository, and the queue orders them.

In `buildwarden-specs`:

- `relay-release-forms/`: `v1.md`; `v1.manifest.schema.json`, with an
  example manifest the document's examples are checked against;
  `not-machine-checked.txt`; `CHANGELOG.md` (`## v0.1.0`); `doc.go`,
  `spec.go`, `spec_test.go` (the directory's package, embedding the
  document as the other six do); `cases/` (the suite); `cmd/bw-conformance-relay-release-forms/`;
  `go.mod`, naming the harness version this item releases with.
- `conformance/`: the `release` role in the target format
  (`conformance/target`), a `release` model that fetches and reads a
  manifest written from the document, and its in-process fixtures; the
  harness's next minor version at the head of its `CHANGELOG.md`, and
  `go.work`'s `replace` line raised to it. `relay-conformance-timeout`
  also changes the harness; whichever merges second takes the next
  version in its refresh.
- `Makefile`'s `SPECS`, `go.work`'s `use` list, and `README.md`: the
  index row (seam 8, `relay-release-forms`, `FORM`, not yet released),
  the `release` role among the target roles, and the Implementations
  table, which names no implementation for it yet.

In `buildwarden/buildwarden`: this design only. No area is needed that the
map lacks, and no link.

### The document, section by section

The sections are the repository's, in its order.

- **Scope and roles.** The release and the launcher. Outside it: what the
  relay does once started (`relay-claim`, `relay-guest-api`, the collector
  documents), the build link's isolation (seam 7), where a launcher keeps
  its pin (a driver's own), a claim transport other than TCP (S4).
- **Transport.** A release is a set of files at one base location with
  `relay-release.json` among them; file names in the manifest are relative
  to it. An `oci-image` is pulled by its digest reference. Nothing binds a
  release to GitHub; a GitHub release is one such location.
- **Messages and fields.** The manifest, field by field (Decision 3), the
  published hash (Decision 4), the forms and roles (Decision 2), the claim
  mode (Decision 7), each form's launch interface and boot channels
  (Decision 5), and the device vocabulary (Decision 6).
- **Ordering.** A launcher checks the manifest against its pin, then every
  file of the form it runs, then starts it; it gives the pre-claim values
  at start and nothing after; the release accepts the claim on its claim
  endpoint (`relay-claim`).
- **Errors.** A launcher refuses a manifest whose hash is not the pinned
  one, an unknown `manifest_version`, a file whose size or hash differs, an
  image reference without a digest, and a form with a role it does not
  know, each naming the file and where it came from (the message K1's
  `Relay.Verify` already gives). It passes over a form it does not run. A
  release refuses a missing or malformed boot value on its console, and
  starts no relay (a12).
- **Limits.** The manifest's size, the length of names, and hashes as 64
  lowercase hexadecimal digits.
- **Versioning.** `manifest_version` 1; while releases are `v0.x.y` the
  document may change incompatibly, as the other six say; a new form, role
  or device is an additive change, and a launcher ignores a member it does
  not know outside the ones it must check.
- **Conformance.** Which requirements a release meets, per form, and which
  a launcher meets.
- **Known divergences.** The Go relay's (Decision 10).

### The umbrella test, read by Design

Design judged the points before choosing the review (FLOW-20; the
independent session would judge them again):

- U1 holds: seam 8 is in the umbrella's seam table, and this is the item
  Jeff filed for it under `driver-contract` a11 (a filed follow-up); it
  adds no component and moves no boundary or ownership.
- U2 fails: a1, a2 and a3 are component design (a new contract's trust
  root, its launch interface and its device set), flagged.
- U3 holds: no rule change; the only area here is `designs`.
- U4 holds: `security_boundary: none`; nothing on a boundary changes.
- U5 holds: no interface is broken; the specification is new and no
  release, flag or key changes.
- U6: `repos` match intake's; intake recorded no areas, so the session
  checks `[designs]` against the map.

So `design_review: required`: the design goes to Jeff, who settles a1 to
a3 (and a10's privileged change) in one sitting.

## Defaults and overrides

- **No default for a boot-channel value.** A release starts only when
  every key its form reads is given; `warden.net` has no default (a12).
  The Go init's `dhcp` default is listed as a known divergence.
- **Fixed values, not defaults:** the claim endpoint on TCP port 8301 for
  `oci-image` and the VM forms, and the build link as the first NIC. A
  launcher has no way to change them, so nothing overrides them.
- **No fallback between forms or sources.** A launcher that finds no form
  for its platform, or a file that does not match, stops with the error
  (GLOBAL-11); it does not try another form, another location, or the
  same release unpinned. The named fallback this drops is "verify against
  the hash published beside the manifest when no pin is configured"
  (Decision 4, rejected).
- **The signature** is optional, so its absence is not a fallback: no
  launcher in version 1 checks one, and none may treat a present
  signature as a substitute for the pin.

No default of `warden`, a driver or the kit changes in this item.

## Cross-driver impact

No driver changes. When each driver adopts a relay release in its M6 item
it becomes a launcher of this specification for the form it chose
(umbrella Cross-driver table): container `oci-image`; qemu and firecracker
`unikernel` or `appliance`; vz `unikernel` in a second VM, with
`host-binary` still available; hyperv `appliance`, which reads the relay
seed. The kit's `driver/relaysource` already has the four names, the
`SHA256` field and `Verify`; its pinned source and the drop of
"provisional" come with the adoption item (Decision 11).

## Specification changes

- **`relay-release-forms` (seam 8): new.** `relay-release-forms/v1.md` in
  `buildwarden/buildwarden-specs`, requirements `FORM-1` onward, with its
  manifest schema and suite, made in that repository's pull request from
  `feat/relay-release-forms-spec` into `program/componentization`. Its first
  release is the loop's pre-release `relay-release-forms/v0.1.0-dev.1`
  once the specs repository's release workflow accepts it (a10). This
  repository pins no release of it in this item; the reconciliation item
  does (Decision 11). Until this lands, seam 8 is defined by
  `driver/relaysource` (the names), the five drivers' relay start code and
  `tools/relay-vm/README.md` (the appliance's boot channel).
- **`relay-claim` (seam 1): no change.** Seam 8 refers to CLAIM-2 (the
  claim endpoint), CLAIM-5 and CLAIM-10 (the token on the boot channel)
  and the Conformance clause on modes (Decision 7).
- **`ledger-format` (seam 4): no change** (Decision 8).
- **`bootstrap-model`: no change.** Decision 5 writes down BOOT-06's boot
  channel as it stands; no Bootstrap comparison is needed (BOOT-07).

## Security note

`security_boundary: none`, and no code on the host-environment,
environment-relay or relay boundary changes: the item adds a document, a
schema and a suite in the specs repository. The specification states
integrity requirements (Decision 4) and a device requirement that guards
key generation (Decision 6) that bind a launcher only once it adopts them.
Each M6 adoption that makes a driver check a pinned hash tightens that
driver's posture and states it in its own security note. Because those
requirements set what that posture will be, a1 and a3 are flagged and go to
Jeff. For TEST-08, later: a launcher refuses a manifest or file that does
not match its pin, and a VM form refuses to start without its entropy
source.

## Validation plan

- **Tier 0 and 1, in the specs repository:** `make check` on the branch
  (document checks over seven directories, `go vet`, `go test` with the
  suite against the in-process releases, the coverage check, the module
  and tidy checks), and its CI on the pull request into the program trunk.
- **Tier 2:** none native. The suite's launch case runs in the Linux
  sandbox in CI against fixture forms the cases build (a host binary
  built from the harness's own relay, and its image), not against any
  relay's release: no release conforms yet (Decision 10). The VM forms and
  the launcher requirements are listed, with reasons, in
  `not-machine-checked.txt`; they are checked when M2 or M3 builds a VM
  launcher and when a driver adopts the seam.
- **In this repository:** `make lint` and `make test` on the branch,
  which change nothing but this design.
- **Native validation:** none, blocking or tracked: no driver, host or
  platform behaviour changes, and nothing here runs natively.

## Implementation plan

Plan writes the tasks from the design as reviewed. In outline, in the specs
repository, specification first (LOOP-02):

1. `relay-release-forms/v1.md` with every requirement numbered, the
   manifest schema and its example, `not-machine-checked.txt`, the
   `CHANGELOG.md`, and the directory's package; `make check-docs` green.
2. The harness: the `release` role and `[release]` table in the target
   format, the release model, the in-process fixtures, the version bump.
3. The suite and its command; `make check` green; every requirement cited
   by a case or listed.
4. `Makefile`, `go.work`, `README.md`.
5. At Publish, in this repository: the design archived with its summary.
   Once the specification's first pre-release is tagged, the loop files
   `adopt relay-release-forms at v0.1.0-dev.1` for this repository
   (Decision 11).

## Design review

## Consolidation

## Rollback notes

## Archive summary

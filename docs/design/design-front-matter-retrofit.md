---
feature: design-front-matter-retrofit
status: draft
round: parallel
drivers: []
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [FLOW-01, FLOW-02, FLOW-07, FLOW-09, FLOW-12, FLOW-14, GLOBAL-06, GLOBAL-08, RELAY-03, RELAY-04, TEST-02, TEST-03, TEST-08, LINT-02]
concepts: [design-archive, front-matter, retrofit, archive-retrieval]
related: [workflow-bootstrap.md, firecracker-driver-plan.md, hyperv-driver-plan.md, platform-compatibility.md, remote-execution-and-measured-hermeticity.md, vm-driver-release-plan.md, windows-driver-plan.md, hyperv-windows-prepare.md, ledger-environment-types.md, unified-bootstrap.md]
---

# Retrofit FLOW-02 front-matter onto the older designs

## Problem

FLOW-12 makes `docs/design/` the archive and retrieves from it by front-matter
(`concepts`, `drivers`, `seams`, `rules`). Six documents there predate FLOW-02
and carry none, so a design phase searching the front-matter cannot find them:

| Document | What it is |
|---|---|
| `firecracker-driver-plan.md` | Plan for a Firecracker driver; not started |
| `hyperv-driver-plan.md` | Hyper-V driver plan; implemented, partly superseded by the integration pass |
| `platform-compatibility.md` | Host x target x driver matrix; a living reference |
| `remote-execution-and-measured-hermeticity.md` | Vision for an RE API backend; not started |
| `vm-driver-release-plan.md` | Cross-driver roadmap; active (Phase 5a done, later phases open) |
| `windows-driver-plan.md` | Windows guest plan; Phase 1 merged, Phase 2 delivered through Hyper-V |

Three archived designs (`hyperv-windows-prepare.md`,
`ledger-environment-types.md`, `unified-bootstrap.md`) have front-matter but
predate the `alters_relationships` key, so their front-matter is incomplete
against today's template.

FLOW-12 retrofits lazily, "when a new design first relies on them", and the
README follow-up says the same. Jeff moved this item from priority 7 to 3
("retrofit early"), so the archive is made searchable before the queued
features (Firecracker, security suite, tier-1 checks) start designing against
it.

## Prior art

- FLOW-02 (the keys and the conservative-derivation rule), FLOW-12 (archive and
  retrofit), and `TEMPLATE.md` (key order and value vocabulary).
- The archived designs with front-matter set the vocabulary reused here:
  seams `relay-claim`, `collector`, `ledger-format`, `executor-initialize`,
  `relay-guest-api`, `bootstrap-model`, `image-manifest`, `windows-guest-seed`;
  hosts `windows-hyperv`, `macos-vz`, `macos-qemu`. No new seam names are
  introduced; one new host name, `linux-kvm`, is needed for Firecracker.
- `workflow-bootstrap.md` (active) already has complete front-matter and is
  untouched.
- `tools/workflow/check` (`Front.problems`) is the mechanical definition of
  "complete front-matter" that the retrofit is validated against.
- The `firecracker-driver` queue item notes that its design "retrofits
  front-matter (FLOW-12)" onto `firecracker-driver-plan.md`. This feature does
  it first; that feature amends it (see Assumed a7).

## Decisions

1. **Retrofit every pre-standard design now, not lazily** (a1). The lazy rule
   leaves each new design to discover and retrofit the same documents, which
   duplicates work and risks two branches writing conflicting front-matter for
   one file. Rejected: lazy only (the README wording; slower to make the archive
   searchable).
2. **Existing lines are never edited; the retrofit only adds** (a4). Each
   document gets a front-matter block prepended. The two documents whose
   `security_boundary` is not `none` and whose body has no security section
   (`windows-driver-plan.md`, `remote-execution-and-measured-hermeticity.md`)
   also get a short `## Security note (retrofit)` section appended at the end
   of the file. No existing line changes, so the diff is still added lines
   only and mechanically checkable. Stale status banners, missing Archive
   summaries and the template's other section headings are not added.
   Why the notes rather than an exception (review 7b11f043ab61, MUST 3-4):
   FLOW-02 is a MUST ("any value other than `none` requires a security note in
   the design"), and confirming an assumption cannot waive it. An exception is
   a rule change, which is Jeff's to make (FLOW-14), not something a design
   grants itself. Lowering the boundary to `none` would understate it, against
   conservative derivation. And `remote-execution-and-measured-hermeticity` is
   a draft that the next design will build on; it changes the relay itself, so
   GLOBAL-06 makes it high-severity, and a boundary with no note would be
   inherited silently. The notes record what each document's own text says
   about the boundary and what TEST-08 should exercise; they add no new
   design. Rejected: front-matter only with a recorded FLOW-02 exception (a4
   (b)); rewriting the documents into template shape (large, opinionated, and
   history the archive should keep as written).
3. **`feature` is the file's base name** (a6), for example
   `feature: firecracker-driver-plan`. Rejected: the feature's queue slug, which
   collides where a queue item names a different design
   (`firecracker-driver` uses `firecracker-spike-plan.md`).
4. **`status` reflects the document's current standing** (a2): `archived` when
   its work is merged and it is no longer edited (`hyperv-driver-plan`,
   `windows-driver-plan`); `active` when it is still maintained
   (`vm-driver-release-plan`, `platform-compatibility`); `draft` when nothing it
   describes is built (`firecracker-driver-plan`,
   `remote-execution-and-measured-hermeticity`). `round: parallel` for all six,
   since none went through a merge round.
5. **Values are derived conservatively from the text as written** (FLOW-02).
   Where a plan reaches a shared package (`relay/`, `ledger/`,
   `cmd/warden-io`), `drivers` lists every dependent driver. Where it is unclear
   whether a plan alters relationships, `alters_relationships: true`. `rules`
   maps the plan's subject to the current rule IDs that govern it, since the
   plans predate rule IDs.
6. **`security_boundary` takes the single most severe boundary the document
   alters** (a5), ordered `host-env` > `env-relay` > `env-beyond`. Where a
   document's existing body has a security section, a YAML comment points to it
   as the security note. Where it has none, the comment points to the appended
   `## Security note (retrofit)` (Decision 2, a4).
   Rejected: proposing a list-valued `security_boundary` (a FLOW-02 change for
   human sign-off; not needed to make the archive searchable).
7. **`native_validation` and `tier2_gaps` on retrofitted documents are
   descriptive**: they record which host the plan's work needs and which gaps
   the document itself names. They create no queue steps and no backlog entries;
   the one tracked gap named (`vm-driver-release-plan.md`, qemu Windows guest
   package) is already `qemu-windows-guest-package` in the native-gap backlog.
   Hosts follow `drivers` (review 7b11f043ab61, SHOULD 6): the primary hosts a
   plan targets keep the blocking value the plan gives them, and every further
   driver that `drivers` lists because of a shared-package change (Decision 5)
   gets one `blocking: false` regression entry on its host: `container` on
   `container-any`, `qemu` on `qemu-any`, `vz` on `macos-vz`, `hyperv` on
   `windows-hyperv`. This covers the VZ and Linux+qemu regression check that
   `windows-driver-plan.md` names in its Phase 2 hand-off, and the "no
   regression on container/qemu/vz" condition on `hyperv-driver-plan.md`'s
   relay output sink. `vm-driver-release-plan` (a roadmap whose phases name
   their own hosts) and `platform-compatibility` (a reference, not a change)
   are not widened this way. The host names are ones already used in the
   archive and the native-gap backlog; `linux-kvm` remains the one new name.
8. **The three archived designs gain `alters_relationships: true`** (a3): each
   changes a versioned seam or moves a responsibility between components
   (`unified-bootstrap`: bootstrap model; `ledger-environment-types`: ledger
   format and claim; `hyperv-windows-prepare`: Windows guest code moved from two
   drivers into the shared `driver/winguest`). No other key is changed.
9. **Rework after review 7b11f043ab61** (p1-design-r2). The findings were
   against values this design chose, so they are fixed here and Implement
   re-applies the blocks:
   - `firecracker-driver-plan`: `drivers` widens to every relay-dependent
     driver, because the plan changes the relay binary (relay-to-host vsock
     "CA ready" and "ledger complete" signals) and shares `util.go` with qemu.
     `seams` gains `relay-claim`, because the plan injects a per-build
     `relay.env`, extracts the CA from the relay, and hands the warm pool's
     resumed relay its build config over vsock, which is the claim GLOBAL-08
     names. Decision 5 now applies to it as to the Hyper-V and Windows plans.
     `rules` gains RELAY-04 (a relay change states its modes) and RELAY-03:
     the warm pool snapshots a relay after "CA generated, listeners up", so
     every restored relay would share one CA key, which RELAY-03 forbids. The
     retrofit records this in a YAML comment for the `firecracker-driver`
     design to settle and does not edit the plan (Decision 2).
   - `windows-driver-plan`, `remote-execution-and-measured-hermeticity`:
     appended security notes (Decision 2, a4).
   - `hyperv-driver-plan`: keeps `archived` (a2), with a comment on `status`
     naming its unbuilt scope, Phase 1's Linux build guest
     (`vm-driver-release-plan.md` Phase 5: "Linux guests on Hyper-V not
     started"), so a search on `status` does not read it as complete.
   - `windows-driver-plan`, `hyperv-driver-plan`: regression hosts added to
     `native_validation` (Decision 7). The same rule adds them to
     `firecracker-driver-plan` (its widened `drivers`) and
     `remote-execution-and-measured-hermeticity` (no primary host yet).
10. **Rework after review fdd14b44f1f8** (p1-design-r3). All three findings
    were in the security-note text this design prescribes, so they are fixed
    here and Implement re-applies only the two notes. The front-matter blocks
    are unchanged. Every claim in a note must be one its own document
    supports (Decision 2: the notes add no design).
    - `windows-driver-plan` (MUST): the note no longer says administrator
      rights inside the guest cannot open a path. The plan argues isolation
      from host-enforced topology only, its guest has an autologon admin, and
      its adversarial isolation check (Phasing chunk 4, graduation gate) is
      not in Phase 1's "Done & validated" list. The note now says the
      argument relies on no in-guest control, and leaves the
      guest-administrator question to that gate.
    - `windows-driver-plan` (SHOULD): the prepared image is "input fixed at
      prepare" (BOOT-06), not "trusted input"; the plan calls the guest
      "untrusted by design".
    - `remote-execution-and-measured-hermeticity` (SHOULD): the Boundary
      bullet adds CAS input staging outside the relay (lifecycle step 4,
      against BOOT-02 today) and the RE API services as a new network-facing
      surface; the TEST-08 bullet adds digest-mismatched CAS blobs and
      unauthenticated Execute or ActionCache writes. `security_boundary`
      stays `env-relay`: both changes are about what reaches the build
      environment and the ledger, not the host-to-environment boundary.
11. **Out of scope**: `docs/book/src/design/` (the ledger spec, philosophy
    and initial proposal are book pages governed by RELAY-05, not archive
    designs); `firecracker-spike-plan.md` and `container-inputs.md`, which are
    on other feature branches and get front-matter from their own design
    phases; `TEMPLATE.md`.

## Questions

### Blocking

None.

### Assumed

- **a1. Scope.** Proceeding with retrofitting all six pre-standard designs in
  this feature; confirm. Options: (a) all six now; (b) only those a queued
  feature relies on; (c) lazily per FLOW-12 wording. Recommended: (a).
- **a2. Status mapping.** Proceeding with `archived` for `hyperv-driver-plan`
  and `windows-driver-plan`, `active` for `vm-driver-release-plan` and
  `platform-compatibility`, `draft` for `firecracker-driver-plan` and
  `remote-execution-and-measured-hermeticity`; confirm. `hyperv-driver-plan`'s
  `status` carries a comment naming its unbuilt Linux-guest phase (Decision 9).
  Options: (a) this mapping; (b) as (a) but `hyperv-driver-plan` `active`
  until its Linux-guest phase is built or dropped; (c) every retrofitted
  document `archived`; (d) every one `active`. Recommended: (a); the
  Windows-guest work it planned is merged and the Linux-guest phase is a
  `vm-driver-release-plan` item, which is `active`.
- **a3. Complete the archived designs.** Proceeding with adding
  `alters_relationships: true` to `hyperv-windows-prepare.md`,
  `ledger-environment-types.md` and `unified-bootstrap.md`; confirm. Options:
  (a) add `true` to all three (conservative); (b) judge each, possibly `false`
  for `hyperv-windows-prepare`; (c) leave archived designs untouched.
  Recommended: (a).
- **a4. Add only; no existing line edited.** Proceeding with a prepended
  front-matter block on every document, plus a short appended
  `## Security note (retrofit)` on the two whose `security_boundary` is not
  `none` and whose body has no security section (`windows-driver-plan.md`,
  `remote-execution-and-measured-hermeticity.md`), as FLOW-02 requires. No
  Archive summary; stale banners such as `windows-driver-plan.md`'s "Phase 2
  pending" are left as written; confirm. Options: (a) front-matter plus the
  two appended security notes; (b) front-matter only, recording a FLOW-02
  exception for the two documents (a rule change for Jeff, FLOW-14; leaves a
  MUST unmet on a draft the next design builds on); (c) as (a), plus a
  one-line retrofit banner; (d) as (a), plus Archive summaries for the two
  archived plans. Recommended: (a) (Decision 2).
- **a5. One security boundary per document.** Proceeding with the single most
  severe boundary, others named in a YAML comment that also points to the
  document's security note (its own security section, or the appended note
  under a4); confirm. Options: (a) single value plus comment; (b) propose a
  FLOW-02 amendment allowing a list. Recommended: (a).
- **a6. Feature slug.** Proceeding with `feature` = the file's base name;
  confirm. Options: (a) base name; (b) the matching queue slug where one exists.
  Recommended: (a).
- **a7. Firecracker overlap.** Proceeding with retrofitting
  `firecracker-driver-plan.md` here; the `firecracker-driver` design then
  amends it (for example to `archived` once `firecracker-spike-plan.md`
  supersedes it) instead of writing it from scratch; confirm. Options: (a)
  retrofit here; (b) skip it and leave it to `firecracker-driver`.
  Recommended: (a); this item is priority 3, that one 6.
- **a8. Keeping the archive complete.** Proceeding without a permanent
  archive-wide front-matter check in this feature, because `tools/workflow/`
  is not edited unattended (FLOW-14); the check is proposed to
  `tier1-static-checks` (LINT-02); confirm. Options: (a) hand it to
  `tier1-static-checks`; (b) add a test under `tools/workflow/check` in an
  attended step of this feature; (c) no permanent check. Recommended: (a).
- **a9. README follow-up.** Proceeding with leaving the "Follow-up - Design
  front-matter retrofit" bullet in `docs/development/README.md` for the human
  to remove at sign-off (FLOW-14: agents do not edit the standards unattended);
  the PR description gives the exact edit; confirm. Options: (a) human removes
  it at sign-off; (b) an attended step removes it in this branch.
  Recommended: (a).

## Design

Each listed document gets the block below prepended, followed by a blank line,
before its existing first line. Keys follow `TEMPLATE.md` order. A leading
comment marks the block as a retrofit. `windows-driver-plan.md` and
`remote-execution-and-measured-hermeticity.md` also get the security note
given under their block appended after their existing last line, separated by
one blank line. The note's heading is written outside its fence, because
`check.ParseDesign` splits sections on any line starting `## `, fenced or not.
No existing line changes (Decision 2).

The code branch at fdd14b4 carries the round-2 blocks, which match those
below, and the round-2 notes; the two notes below replace the round-2 notes
(Decision 10, Implementation plan task 0).

### `firecracker-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: firecracker-driver-plan
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv, firecracker]  # relay/ gains vsock "CA ready"/"ledger complete" signals; util.go shared with qemu
seams: [relay-claim, relay-guest-api, collector]     # per-build relay.env and CA extraction; warm-pool config injected over vsock; outputs read from relay rootfs predate RELAY-07
surfaces: [cli, config]
security_boundary: host-env           # also env-relay; see "Security Considerations"
alters_relationships: true            # warm-pool manager, vsock signaling, relay claim over vsock
native_validation:
  - {host: linux-kvm, blocking: true, reason: "driver/firecracker runs only on a Linux KVM host"}
  - {host: container-any, blocking: false, reason: "regression: relay/ vsock signaling change"}
  - {host: qemu-any, blocking: false, reason: "regression: relay/ change and util.go shared with qemu"}
  - {host: macos-vz, blocking: false, reason: "regression: relay/ vsock signaling change"}
  - {host: windows-hyperv, blocking: false, reason: "regression: relay/ vsock signaling change"}
tier2_gaps: []
rules: [ORCH-01, ORCH-03, ORCH-04, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-03, RELAY-04, RELAY-07, BOOT-01, BOOT-02, BOOT-04, TEST-02, TEST-08]  # RELAY-03: warm-pool snapshot is taken after CA generation, so restored relays would share a CA key
concepts: [firecracker, microvm, vsock, mmds, jailer, warm-pool, snapshot-resume, tap-networking, two-vm-topology]
related: [vm-driver-release-plan.md]
---
```

### `hyperv-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; body unchanged.
feature: hyperv-driver-plan
status: archived                        # Windows-guest path merged; Phase 1 Linux build guest not built (vm-driver-release-plan Phase 5)
round: parallel
drivers: [container, qemu, vz, hyperv]  # output sink added to relay/ (shared package)
seams: [relay-claim, collector, relay-guest-api]
surfaces: [cli, config]
security_boundary: host-env             # also env-relay; see "Security Considerations", "Privilege & Elevation Model"
alters_relationships: true
native_validation:
  - {host: windows-hyperv, blocking: true, reason: "driver/hyperv runs only on Hyper-V"}
  - {host: container-any, blocking: false, reason: "regression: relay/ output sink; localSink must stay the default"}
  - {host: qemu-any, blocking: false, reason: "regression: relay/ output sink; localSink must stay the default"}
  - {host: macos-vz, blocking: false, reason: "regression: relay/ output sink; localSink must stay the default"}
tier2_gaps: []
rules: [ORCH-01, ORCH-03, ORCH-04, ORCH-05, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-07, BOOT-01, BOOT-02, UX-04, TEST-02]
concepts: [hyperv, windows-host, vswitch, nat, elevation, privileged-service, unattend, differencing-disk, output-egress, collector, uki-relay-boot]
related: [vm-driver-release-plan.md, windows-driver-plan.md, hyperv-windows-prepare.md]
---
```

### `platform-compatibility.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard reference; body unchanged.
feature: platform-compatibility
status: active
round: parallel
drivers: [container, qemu, vz, hyperv]
seams: []
surfaces: []
security_boundary: none
alters_relationships: false
native_validation: []
tier2_gaps: []
rules: [ORCH-01, ORCH-05, GLOBAL-07, TEST-02, TEST-03, TEST-08]
concepts: [platform-matrix, host-target, driver-availability, licensing, network-isolation, graduation-criteria]
related: [windows-driver-plan.md, vm-driver-release-plan.md]
---
```

### `remote-execution-and-measured-hermeticity.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard proposal; existing lines unchanged, security note appended.
feature: remote-execution-and-measured-hermeticity
status: draft
round: parallel
drivers: [container, qemu, vz, hyperv]  # ledger/ and relay changes reach every driver
seams: [ledger-format]
surfaces: [cli, config, output]
security_boundary: env-relay            # per-action relay ledgers and cache provenance; see "Security note (retrofit)"
alters_relationships: true              # BuildWarden as an RE API backend; ledgers composed from cached results
native_validation:
  - {host: container-any, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
  - {host: qemu-any, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
  - {host: macos-vz, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
  - {host: windows-hyperv, blocking: false, reason: "regression: ledger/ and relay changes reach every driver"}
tier2_gaps: []
rules: [RELAY-01, RELAY-02, RELAY-03, RELAY-05, GLOBAL-05, GLOBAL-06, GLOBAL-08, TEST-08]
concepts: [remote-execution, re-api, measured-hermeticity, action-cache, provenance, environment-record, cache-invalidation, bazel]
related: [ledger-environment-types.md]
---
```

Appended after the existing last line: one blank line, the heading line
`## Security note (retrofit)`, one blank line, then this text:

```markdown
Added when front-matter was retrofitted (FLOW-02, FLOW-12). It restates what
this proposal's text implies for GLOBAL-06; it is not a security design. The
first design that builds on this proposal writes its own note and settles the
open points below.

- **Boundary:** env-relay, and the relay itself. A parent build's ledger
  incorporates environment and network records from cached sub-ledgers that
  its own relay did not observe ("Cache hit behavior"), and each worker relay
  writes an environment record as the first ledger entry. Both change what a
  signed ledger attests, which GLOBAL-06 makes high-severity. The proposal
  also implies two further boundary changes. Inputs are staged from
  content-addressed storage into the sandbox outside the relay ("Action
  execution lifecycle", step 4), which BOOT-02 does not allow today; only the
  action's cache key (`input_root_digest`) records them. And the RE API
  services (Execute, ContentAddressableStorage, ActionCache) are a new
  network-facing surface whose contents feed parent ledgers.
- **How the posture changes:** a parent ledger's claims now rest on the action
  cache and on other relays' signatures, as well as on its own observation. A
  forged, tampered or replayed cache entry (`ledger_digest`,
  `environment_digest`, `hermetic`) would put false provenance into a validly
  signed parent ledger. Open points: verify each sub-ledger's signature chain
  and the worker relay's key before incorporating it; recompute `hermetic`
  from the verified sub-ledger, never from the stored flag; mark incorporated
  records as incorporated rather than observed; keep one fresh relay per
  action (RELAY-03), as "Worker model" already requires.
- **What TEST-08 should exercise:** a sub-ledger with a broken signature, a
  wrong `ledger_digest` or an unknown signing key is rejected, not
  incorporated; a cache entry whose stored `hermetic` disagrees with its
  ledger; deduplication by hash block cannot hide a record whose content
  differs; an action marked `requires-network` reaches the network only
  through a relay (its worker's, or the parent build's under `no-remote`);
  CAS blobs that differ from their digests; an unauthenticated Execute or
  ActionCache write.
```

### `vm-driver-release-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard roadmap; body unchanged.
feature: vm-driver-release-plan
status: active
round: parallel
drivers: [container, qemu, vz, hyperv, firecracker]
seams: [relay-claim, collector, ledger-format, image-manifest, windows-guest-seed]
surfaces: [cli, config, output]
security_boundary: host-env             # also env-relay; see "Security Testing Framework"
alters_relationships: true
native_validation:
  - {host: macos-vz, blocking: true, reason: "Phase 1 vz driver runs only on macOS/arm64"}
  - {host: windows-hyperv, blocking: true, reason: "Phase 5 and 5a Hyper-V Windows-guest lifecycle"}
  - {host: macos-qemu, blocking: false, reason: "shared Windows guest package on qemu, tracked as qemu-windows-guest-package"}
tier2_gaps:
  - "shared Windows guest package (driver/winguest) on qemu under macOS: not yet validated natively"
rules: [ORCH-01, ORCH-04, ORCH-05, GLOBAL-06, GLOBAL-07, GLOBAL-08, RELAY-05, RELAY-07, TEST-02, TEST-03, TEST-08, SEC-01, UX-04, UX-07]
concepts: [release-plan, driver-matrix, security-testing, escape-vectors, image-lifecycle, relay-vm-distribution, ledger-compatibility, image-prepare, claim, collector]
related: [hyperv-driver-plan.md, windows-driver-plan.md, firecracker-driver-plan.md, hyperv-windows-prepare.md, platform-compatibility.md]
---
```

### `windows-driver-plan.md`

```yaml
---
# Retrofitted (FLOW-12) onto a pre-standard plan; existing lines unchanged, security note appended.
feature: windows-driver-plan
status: archived
round: parallel
drivers: [container, qemu, vz, hyperv]  # cmd/warden-io changes reach every driver
seams: [executor-initialize, relay-guest-api, windows-guest-seed]
surfaces: [cli, config]
security_boundary: env-relay            # Windows CA trust install and topological isolation; see "Security note (retrofit)"
alters_relationships: true              # executor gains a Windows guest; build.ps1 input model
native_validation:
  - {host: macos-qemu, blocking: true, reason: "Phase 1 win-arm64 under qemu on Apple Silicon"}
  - {host: windows-hyperv, blocking: true, reason: "Phase 2 native win-64 on Hyper-V"}
  - {host: macos-vz, blocking: false, reason: "regression check the plan requires: VZ path untouched"}
  - {host: qemu-any, blocking: false, reason: "regression check the plan requires: Linux+qemu disk, seed and serial unchanged"}
  - {host: container-any, blocking: false, reason: "regression: cmd/warden-io changes reach the container driver"}
tier2_gaps: []
rules: [EXEC-01, EXEC-02, EXEC-03, EXEC-04, ORCH-01, ORCH-05, BOOT-02, BOOT-04, GLOBAL-06, GLOBAL-07, TEST-02, TEST-08]
concepts: [windows-guest, warden-io-windows, build-ps1, conda-forge, image-prepare, autounattend, licensing, win-arm64]
related: [hyperv-driver-plan.md, vm-driver-release-plan.md, platform-compatibility.md, hyperv-windows-prepare.md]
---
```

Appended after the existing last line: one blank line, the heading line
`## Security note (retrofit)`, one blank line, then this text:

```markdown
Added when front-matter was retrofitted (FLOW-02, FLOW-12). It gathers what
this plan already says about GLOBAL-06; it adds no design.

- **Boundary:** env-relay. The Windows guest is a new kind of inner build
  environment behind the same relay.
- **How the posture changes:** `warden-io` on Windows imports the relay's
  per-build CA into the machine root store (`certutil -addstore -f Root`) and
  sets the CA environment variables, so every Schannel and OpenSSL client in
  the guest trusts the relay's MITM CA for that build. Isolation stays
  topological and host-enforced: the guest's single NIC faces the relay VM,
  which redirects :80, :443 and :53 and drops forwarding. There is no
  in-guest firewall, so the isolation argument does not rely on any in-guest
  control; whether a guest administrator can open a path is what the
  adversarial gate below must show, and the plan does not record that gate
  as run. The prepared Windows image (autounattend, first-boot task) is input fixed at
  prepare, before the build (BOOT-06: it carries the startup hook, not build
  content); per build, only the executor and its seed cross outside the
  relay (BOOT-02).
- **What TEST-08 should exercise:** egress from the Windows guest that
  bypasses the relay (direct IP, other ports, IPv6, SMB/NetBIOS, ICMP,
  DNS-over-HTTPS); that the per-build CA does not persist into the prepared
  image or a later build; that the seed carries nothing beyond the executor,
  launcher and network configuration; and the adversarial "escape the build
  env" agent the plan's own Phasing and graduation gates require.
```

### Archived designs

`hyperv-windows-prepare.md`, `ledger-environment-types.md` and
`unified-bootstrap.md` each gain one line, `alters_relationships: true`,
after `security_boundary` (template order). Nothing else changes.

## Cross-driver impact

None. The change touches only Markdown under `docs/design/` (front-matter,
plus two appended security notes); no driver tree, shared package, seam or user-facing surface changes, so `drivers`,
`seams` and `surfaces` are empty (ORCH-01). The `drivers` values written into
the retrofitted documents describe those documents, not this change.

## Validation plan

- **Tier 0.** Every `docs/design/*.md` except `TEMPLATE.md` parses with
  `check.ParseDesign` and reports no `Front.problems` other than the expected
  feature-versus-queue mismatch (the retrofitted documents have no queue item).
  `problems` is unexported, so this runs as a scratch test inside the package
  that is never committed (a8); its output goes in the implementation commit
  message. (Phase 1 already parsed each proposed block with `ParseDesign`.)
- A diff check: for each retrofitted document, `git diff` shows only added
  lines: the front-matter at the top of the file, plus the appended security
  note at the end for `windows-driver-plan.md` and
  `remote-execution-and-measured-hermeticity.md` (Decision 2).
- `make test` and `make lint` stay green (nothing they cover changes).
- **Tier 1 / 2.** None. No native host validates this change: it reaches no
  driver tree and alters no host's behavior, so FLOW-09 adds no
  `m6-validate-<host>` step and no tracked gap.

## Implementation plan

Round 3 (p1-design-r3, after review fdd14b44f1f8; p2-plan-r3 may refine
it). Each task carries one status. **Done** means it shipped and round 3
does not change it: Implement does not touch it again, and the task names
the commit. **Rework** means it shipped and p1-design-r3 changed its text:
Implement redoes it through task 0 (FLOW-06). **To do** means it is not done
yet, or is a check that must run again on the round-3 tree.

Each task names the assumed questions it rests on (a1-a9, all still open). A
different answer touches only the tasks that name it. No task edits an
existing body line of a retrofitted document, `tools/workflow/`,
`docs/development/` or `.claude/` (Decision 2, FLOW-14). The round-2 tree
(fdd14b4) passed the parse and diff checks (review fdd14b44f1f8, tier 1).
Round 3 changes only text inside the two appended notes; the new note text
has no line starting `## ` or `---`, so the section split and front-matter
parse are unaffected, and task 8 re-confirms this.

0. **Rework**, the round-3 procedure (FLOW-06, FLOW-02, GLOBAL-06; review
   fdd14b44f1f8, Decision 10). In each of `windows-driver-plan.md` and
   `remote-execution-and-measured-hermeticity.md`, replace the round-2 note
   text, that is every line after the blank line that follows the
   `## Security note (retrofit)` heading, through the end of the file, with
   that file's note from Design, verbatim, ending in a newline. Leave the
   front-matter block, the heading line, the blank lines around it, and
   every original body line as they are. Make one commit per file, citing
   this task, the file's task number and the review. Tasks 4 and 6 give
   each file's end state and acceptance. The other seven touched files are
   **Done** and stay as they are. Assumes a4 (a) and a5.
1. **Done** (round 2: ba605ac): `firecracker-driver-plan.md` begins with
   its Design block, then one blank line, then the unchanged original first
   line. The block widens `drivers`, adds `relay-claim` to `seams`, adds
   RELAY-03 and RELAY-04 with a comment to `rules`, and adds four regression
   hosts (FLOW-02, FLOW-12, GLOBAL-08, RELAY-03, RELAY-04). Assumes a1, a2
   (`draft`), a5, a6 and a7. Under a7 (b), the file reverts to its
   `origin/main` content and this task is dropped.
2. **Done** (round 2: 4805056): `hyperv-driver-plan.md` begins with its
   Design block. The block adds the comment on `status` and three regression
   hosts (FLOW-02, FLOW-12, GLOBAL-06). Assumes a1, a2 (`archived`), a5 and
   a6. Under a2 (b), `status` becomes `active` and its comment is dropped.
3. **Done** (51aedd3): `platform-compatibility.md` block (FLOW-02,
   FLOW-12). Assumes a1, a2 (`active`) and a6.
4. **Rework** (round 2: d5f83c3; round 3 changes the note only):
   `remote-execution-and-measured-hermeticity.md` begins with its Design
   block, whose header comment and `security_boundary` comment point to the
   note (FLOW-02, FLOW-12, GLOBAL-06); the block is already on the branch.
   After the existing last line of the original body come, in this order:
   - one blank line;
   - the line `## Security note (retrofit)`;
   - one blank line;
   - the note text from Design, verbatim, ending in a newline. Its Boundary
     bullet names CAS input staging outside the relay and the RE API
     surface, and its TEST-08 bullet the two added cases (Decision 10).

   This satisfies FLOW-02, GLOBAL-06 and TEST-08 (the note names what TEST-08
   should exercise; this feature writes no test of it). Assumes a1, a2
   (`draft`), a4 (a), a5 and a6. Under a4 (b), no note is appended, and the
   `security_boundary` comment reads "no security section; FLOW-02 exception
   (a4)". That option is a rule change for Jeff (FLOW-14), so Implement
   does not apply it unattended.
5. **Done** (894629d): `vm-driver-release-plan.md` block (FLOW-02, FLOW-12,
   TEST-03). Its `tier2_gaps` entry names the gap already tracked as
   `qemu-windows-guest-package`, so no backlog entry is added. Assumes a1,
   a2 (`active`), a5 and a6.
6. **Rework** (round 2: fdd14b4; round 3 changes the note only):
   `windows-driver-plan.md` begins with its Design block, already on the
   branch. The block's header comment and `security_boundary` comment point
   to the note, and it adds the `macos-vz`, `qemu-any` and `container-any`
   regression hosts (FLOW-02, FLOW-12, GLOBAL-06). Its security note follows
   the original body exactly as in task 4 (FLOW-02, GLOBAL-06, TEST-08). The
   note relies on no in-guest control, leaves the guest-administrator
   question to the adversarial gate, and calls the prepared image input
   fixed at prepare (BOOT-06) (Decision 10). The
   "Phase 2 pending" banner stays. Assumes a1, a2 (`archived`), a4 (a), a5
   and a6. Under a4 (b), as for task 4.
7. **Done** (7b11f04): rework of the three archived designs (FLOW-06,
   FLOW-02). `alters_relationships: true` follows `security_boundary` in
   `hyperv-windows-prepare.md`, `ledger-environment-types.md` and
   `unified-bootstrap.md`. Assumes a3 (a). Under a3 (b), the value changes
   per file; under a3 (c), the three lines are reverted.
8. **To do**, again on the round-3 tree: the tier-0 parse check (TEST-02
   tier 0, TEST-01, FLOW-02).
   - Run `check.ParseDesign` and `Front.problems(<base name>)` over every
     `docs/design/*.md` on the branch except `TEMPLATE.md`. Use the real
     parser through `go test -overlay` with a scratch `_test.go` kept in the
     session scratchpad, so nothing under `tools/workflow/` is created or
     edited (FLOW-14, a8).
   - Pass: no problems in any of the eleven documents, and
     `alters_relationships` set on each.
   - For the two noted files, `Sections` also contains
     `Security note (retrofit)`.
   - `native-host-loop.md` is on `origin/main` only and is not in scope.
   - Paste the output into the last rework commit's message.

   Assumes a6 (the base-name comparison) and a8.
9. **To do**, again on the round-3 tree: the diff checks (TEST-02 tier 0,
   FLOW-12, FLOW-02). `origin/main` has moved since the branch point (to
   137feae; it touched no file here), so diff against the merge base.
   - (a) For each of the nine files, `git diff -U0 origin/main...HEAD --
     <file>` shows 0 removed lines.
     - `windows-driver-plan.md` and
       `remote-execution-and-measured-hermeticity.md` have exactly two
       hunks. The first is the leading block plus its trailing blank line.
       The second, at the end of the file, is the blank line, the heading
       line, the blank line and the note.
     - Each of the other four retrofits has exactly one hunk: the block plus
       its blank line.
     - The three archived designs each have a single added
       `alters_relationships` line.
   - (b) The six blocks and two notes are byte-identical to Design.
   - (c) `git diff --stat fdd14b4..HEAD` lists only the two reworked files
     and this design (and any later plan commit to it), so the seven
     **Done** files are untouched.
   - (d) In each reworked file, `git diff -U0 fdd14b4..HEAD` touches only
     lines after the `## Security note (retrofit)` heading.

   Record the result with task 8's. Assumes a4 (the second hunk).
10. **To do**: run `make test` and `make lint` (TEST-01, FLOW-17). Put the
    go1.26.3 toolchain that golangci-lint needs on `PATH`. No build-tagged
    file changes, so no per-platform lint run is owed. Push
    `feat/design-front-matter-retrofit` after the rework commits, without
    force (FLOW-16, FLOW-17). Assumes nothing open.
11. **To do**, reference docs and overview (UX-01, FLOW-15): none are owed.
    The change has no user-facing surface (`surfaces: []`) and changes no
    component flow, so `docs/book/src/guide/` and
    `docs/book/src/concepts/overview.md` stay as they are. Record
    "UX-01/FLOW-15: none" in the last rework commit's message so that Review
    can check it. Assumes nothing open.
12. **To do**, native validation (FLOW-09, TEST-03): none. This feature adds
    no `m6-validate-<host>` step and no backlog entry. The retrofitted
    `native_validation` values, including `linux-kvm`, which no machine in
    `hosts.yaml` serves yet, are descriptive only (Decision 7). Assumes
    nothing open.
13. **To do**, for the PR description written at Publish (FLOW-08, FLOW-14,
    LINT-02). It contains:
    - the exact `docs/development/README.md` edit that retires the
      "Follow-up - Design front-matter retrofit" bullet, for the human to
      apply at sign-off (a9);
    - the archive-wide front-matter check proposed to
      `tier1-static-checks` (a8);
    - the RELAY-03 warm-pool CA point recorded in the firecracker block's
      `rules` comment, for the `firecracker-driver` design (a7);
    - the review's informational suggestion of a follow-up item to refresh
      the stale `platform-compatibility.md` body, for intake (FLOW-12).

    Under a9 (b) or a8 (b), the first two become attended steps on this
    branch instead. Assumes a7, a8 and a9.

## Consolidation (merge round)

Filled in during the merge round (FLOW-10).

## Rollback notes

None.

## Archive summary

Filled in on merge (FLOW-12).
